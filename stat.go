package stat

//内部提供最终的输出控制,输出的控制不交给外部完成
//为了避免使用double buffer机制,数据接收和重置放到协程中统一事件处理
//定义外部与内部协程之间的通信数据的格式
//通信协议包括
//  错误码、延时、成功与失败
import (
	"container/list"
	"fmt"
	"net"
	"time"

	"github.com/astaxie/beego/logs"
)

const (
	ERRNUM         = 5
	IPNUM          = 3
	STR_FMT        = "%-11s"
	IP_FMT         = "%-17s"
	PER_FMT        = "%-10f%%"
	MINI_FLOAT_FMT = "%-5f"
	MINI_INT_FMT   = "%-4d"
	MINI_STR_FMT   = "%-4s"
	SPT            = " | "

	STAT_DELAY_END_COUNT  = "tcount"
	STAT_DELAY_TOTAL_TIME = "de_total_ms"
	STAT_DELAY_MAX        = "de_max_ms"
	STAT_MAX_IP           = "max_ip"
	STAT_DELAY_UP         = "de_up"
	STAT_DELAY_UP2        = "de_up_2"
	STAT_DELAY_UP3        = "de_up_3"

	STATIN          = "MsgIn"
	STATOUT         = "MsgOut"
	INDEGREERecive  = "INDEGREE_Recive(B)"
	INDEGREESend    = "INDEGREE_Send(B)"
	OUTDEGREERecive = "OUTDEGREE_Recive(B)"
	OUTDEGREESend   = "OUTDEGREE_Send(B)"
)

type LoggerParam struct {
	Level       string
	Path        string
	NamePrefix  string
	Filename    string
	Maxfilesize int
	Maxdays     int
	Maxlines    int
	Chanlen     int64
}

type StatItem struct {
	Name      string //统计的接口名
	Delay     uint   //接口执行的延时,单位ms
	Errcode   int    //当次接口请求的错误码,0--成功
	Ipsrc     net.IP //请求的来源ip
	Payload   uint   //请求的载荷
	Direction int    //上行or下行   1 --- 上行   0 ----下行
	InOrOut   int    //入度还是出度   1 ---- in  0 ----out
}
type StatInter interface {
	AddReportHeadItem(itemName string)
	AddReportBodyRowItem(itemName string)
	AddReportBodyColItem(itemName string)
	AddReportTailItem(itemName string)
	AddReportErrorItem(itemName string)
	AddReportIpError()

	IncStat(itemName string, val uint)
	IncKey(itemName string)
	IncStatByTab(rowName string, colName string, val uint)
	IncErrnoStat(errno int, val uint)
	IncErrnoStatByItem(itemName string, errno int, val uint)
	IncErrnoIp(ip uint, errno int, val uint)

	SetStat(itemName string, val uint)
	GetStat(itemName string) int
	GetStatValueByTab(itemName string, colName string)
	TimeStatGet(rowName string) (count uint, avgDelay float32, maxDelay float32, upDelay uint, upDelay2 uint, upDelay3 uint)

	NoCheckAndPrint()
	Print()
	PrintHeader()
	PrintBody()
	PrintTail()
	PrintRowError()
	PrintIpError()

	ClearAll()
	Reset()
}

type Mystat struct {
	sampleChan    chan StatItem
	ctrlChan      chan int
	vHeadItems    *list.List
	vBodyRowItems *list.List
	vBodyColItems *list.List
	vTailItems    *list.List
	statlog       *logs.BeeLogger
	timeout       uint
	statGap       time.Duration
	delayUp       uint
	delayUp2      uint
	delayUp3      uint
	mapErrNum     map[string]map[int]uint //request,errnode,count
	mapErrIp      map[int]map[int]uint    //retcode,ip,count
	isHadIpErr    bool
	countMap      map[string]uint //requests,count
	errnoCountMap map[int]uint    //errno,count
	timeOutMap    map[string]uint
}

var GStat *Mystat

//初始化调用,线程不安全
func Init(logconfig LoggerParam, statgap time.Duration) {
	Logger := logs.NewLogger(logconfig.Chanlen)
	logConfig := fmt.Sprintf(`{
                  "filename":"%s/%s_%s",
                  "maxlines":%d,
                  "maxsize":%d,
                  "maxDays":%d,
		  "blankprefix":true}`,
		logconfig.Path,
		logconfig.NamePrefix,
		logconfig.Filename,
		logconfig.Maxlines,
		logconfig.Maxfilesize,
		logconfig.Maxdays)

	var level int
	switch logconfig.Level {
	case "debug":
		level = logs.LevelDebug
	case "info":
		level = logs.LevelInformational
	case "notice":
		level = logs.LevelNotice
	case "warn":
		level = logs.LevelWarning
	case "error":
		level = logs.LevelError
	case "critical":
		level = logs.LevelCritical
	case "alert":
		level = logs.LevelAlert
	case "emergency":
		level = logs.LevelEmergency
	}

	Logger.SetLogger("file", logConfig)
	Logger.SetLevel(level)
	Logger.BlankPrefix()
	Logger.Async()
	GStat = new(Mystat)
	GStat.statGap = statgap
	GStat.statlog = Logger
	GStat.vHeadItems = list.New()
	GStat.vBodyRowItems = list.New()
	GStat.vBodyColItems = list.New()
	GStat.vTailItems = list.New()
	GStat.countMap = make(map[string]uint)
	GStat.errnoCountMap = make(map[int]uint)
	GStat.mapErrIp = make(map[int]map[int]uint)
	GStat.mapErrNum = make(map[string]map[int]uint)
	GStat.timeOutMap = make(map[string]uint)

	GStat.sampleChan = make(chan StatItem, 1024)
	GStat.ctrlChan = make(chan int)
	GStat.AddReportHeadItem(STATIN)
	GStat.AddReportHeadItem(STATOUT)
	GStat.AddReportTailItem(INDEGREERecive)
	GStat.AddReportTailItem(INDEGREESend)

}

//初始化调用,线程不安全
func SetDelayUp(delayUp uint, delayUp2 uint, delayUp3 uint) {
	GStat.delayUp = delayUp
	GStat.delayUp2 = delayUp2
	GStat.delayUp3 = delayUp3
}

func PushStat(itemName string, procTime int, requestIp net.IP, payload int, retcode int) {
	statItem := new(StatItem)
	statItem.Name = itemName
	statItem.Delay = uint(procTime)
	statItem.Errcode = retcode
	statItem.Ipsrc = requestIp
	statItem.Payload = uint(payload)
	statItem.Direction = 1
	statItem.InOrOut = 1
	GStat.sampleChan <- *statItem

}

func StatProc() {
	go func() {
		t1 := time.NewTimer(time.Second * GStat.statGap)
		for {
			select {
			case <-t1.C:
				GStat.NoCheckAndPrint()
				t1.Reset(time.Second * GStat.statGap)

			case <-GStat.ctrlChan:
				//退出
				return

			case elem := <-GStat.sampleChan:
				GStat.addSampleStat(&elem)
			}
		}
	}()

}

func Exit() {
	GStat.ctrlChan <- 0
	close(GStat.sampleChan)
	close(GStat.ctrlChan)
}

func (stat *Mystat) addSampleStat(elem *StatItem) {
	stat.IncKey(elem.Name + STAT_DELAY_END_COUNT)             //延时的总请求数
	stat.IncStat(elem.Name+STAT_DELAY_TOTAL_TIME, elem.Delay) //延时总和
	if elem.Direction == 1 {
		stat.IncStat(STATIN, 1)
	} else {
		stat.IncStat(STATOUT, 1)
	}

	if elem.InOrOut == 1 {
		if elem.Direction == 1 {
			stat.IncStat(INDEGREERecive, elem.Payload)
		} else {
			stat.IncStat(INDEGREESend, elem.Payload)
		}
	} else {
		if elem.Direction == 1 {
			stat.IncStat(OUTDEGREERecive, elem.Payload)
		} else {
			stat.IncStat(OUTDEGREESend, elem.Payload)
		}
	}

	max := stat.GetStat(elem.Name + STAT_DELAY_MAX)
	if elem.Delay > max {
		stat.SetStat(elem.Name+STAT_DELAY_MAX, elem.Delay)               //最大延时
		stat.SetStat(elem.Name+STAT_MAX_IP, uint(inet_aton(elem.Ipsrc))) //最大延时的ip
	}

	//分段延时统计
	if elem.Delay > stat.delayUp {
		stat.IncKey(elem.Name + STAT_DELAY_UP)
	}
	if elem.Delay > stat.delayUp2 {
		stat.IncKey(elem.Name + STAT_DELAY_UP2)
	}
	if elem.Delay > stat.delayUp3 {
		stat.IncKey(elem.Name + STAT_DELAY_UP3)
	}

	stat.IncStat(elem.Name, 1) //请求数累计
	if elem.Errcode != 0 {
		stat.IncErrnoIp(elem.Ipsrc, elem.Errcode, 1)        //错误码和ip统计
		stat.IncErrnoStatByItem(elem.Name, elem.Errcode, 1) //请求相关的错误码统计
	}

	if elem.Delay > stat.timeout {
		stat.IncTimeout(elem.Name)
	}

}

//初始化调用,线程不安全
func (stat *Mystat) SetTimeOut(timeout uint) {
	stat.timeout = timeout

}

func (stat *Mystat) AddReportHeadItem(itemName string) {
	stat.vHeadItems.PushBack(itemName)
}
func (stat *Mystat) AddReportBodyRowItem(itemName string) {
	stat.vBodyRowItems.PushBack(itemName)
}
func (stat *Mystat) AddReportBodyColItem(itemName string) {
	stat.vBodyColItems.PushBack(itemName)
}
func (stat *Mystat) AddReportTailItem(itemName string) {
	stat.vTailItems.PushBack(itemName)
}
func (stat *Mystat) AddReportErrorItem(itemName string) {
	delete(stat.mapErrNum, itemName)
	stat.mapErrNum[itemName] = make(map[int]uint)

}
func (stat *Mystat) AddReportIpError() {
	stat.isHadIpErr = true
}

func (stat *Mystat) IncTimeout(itemName string) {
	count, ok := stat.timeOutMap[itemName]
	if !ok {
		stat.timeOutMap[itemName] = 1
	} else {
		stat.timeOutMap[itemName] = count + 1
	}

}

func (stat *Mystat) GetTimeOut(itemName string) uint {
	val, ok := stat.timeOutMap[itemName]
	if !ok {
		return 0
	}
	return val
}

func (stat *Mystat) SetStat(itemName string, val uint) {
	stat.countMap[itemName] = val
}

func (stat *Mystat) IncKey(itemName string) {
	count, ok := stat.countMap[itemName]
	if !ok {
		stat.countMap[itemName] = 1
	} else {
		stat.countMap[itemName] = count + 1
	}

}

func (stat *Mystat) IncStat(itemName string, val uint) {
	count, ok := stat.countMap[itemName]
	if !ok {
		stat.countMap[itemName] = val
	} else {
		stat.countMap[itemName] = count + val
	}

}
func (stat *Mystat) IncStatByTab(rowName string, colName string, val uint) {
	count, ok := stat.countMap[rowName+colName]
	if !ok {
		stat.countMap[rowName+colName] = val
	} else {
		stat.countMap[rowName+colName] = count + val
	}

}
func (stat *Mystat) IncErrnoStat(errno int, val uint) {
	count, ok := stat.errnoCountMap[errno]
	if !ok {
		stat.errnoCountMap[errno] = val
	} else {
		stat.errnoCountMap[errno] = count + val
	}

}
func (stat *Mystat) IncErrnoStatByItem(itemName string, errno int, val uint) {
	stat.IncErrnoStat(errno, val) //错误码统计
	errcodeMap, ok := stat.mapErrNum[itemName]
	if !ok {
		return
	}
	//到底是引用还是值
	count, ok2 := errcodeMap[errno]
	if !ok2 {
		errcodeMap[errno] = val
	} else {
		errcodeMap[errno] = val + count
	}
}
func (stat *Mystat) IncErrnoIp(ip net.IP, errno int, val uint) {
	ipMap, ok := stat.mapErrIp[errno]
	ipint := inet_aton(ip)
	if !ok {
		ipMap := make(map[int]uint)
		ipMap[ipint] = val
		stat.mapErrIp[errno] = ipMap
	} else {
		count, ok := ipMap[ipint]
		if !ok {
			ipMap[ipint] = val
		} else {
			ipMap[ipint] = val + count
		}
	}

}

func (stat *Mystat) GetStat(itemName string) uint {
	val, ok := stat.countMap[itemName]
	if !ok {
		return 0
	}
	return val
}

func (stat *Mystat) TimeStatGet(rowName string) (count uint, avgDelay float32, maxDelay float32, upDelay uint, upDelay2 uint, upDelay3 uint) {
	ok := false
	iMaxDelay := uint(0)
	count, ok = stat.countMap[rowName+STAT_DELAY_END_COUNT]
	if !ok {
		fmt.Printf("no key:%s\n", rowName+STAT_DELAY_END_COUNT)
	}
	delayTotalTime := uint(0)
	delayTotalTime, ok = stat.countMap[rowName+STAT_DELAY_TOTAL_TIME]
	if ok {
		avgDelay = float32(delayTotalTime) / float32(count)
		fmt.Printf("no key:%s\n", rowName+STAT_DELAY_TOTAL_TIME)
	}
	iMaxDelay, ok = stat.countMap[rowName+STAT_DELAY_MAX]
	maxDelay = float32(iMaxDelay)
	if !ok {
		fmt.Printf("no key:%s\n", rowName+STAT_DELAY_MAX)
	}
	upDelay2, ok = stat.countMap[rowName+STAT_DELAY_UP2]
	if !ok {
		fmt.Printf("no key:%s\n", rowName+STAT_DELAY_UP2)
	}
	upDelay3, ok = stat.countMap[rowName+STAT_DELAY_UP3]
	if !ok {
		fmt.Printf("no key:%s\n", rowName+STAT_DELAY_UP3)
	}
	return

}

func (stat *Mystat) GetStatValueByTab(itemName string, colName string) uint {
	count, ok := stat.countMap[itemName+colName]
	if !ok {
		return 0
	}
	return count
}

func (stat *Mystat) Print() {
	stat.PrintHeader()
	stat.PrintBody()
	stat.PrintRowError()
	stat.PrintIpError()
	stat.PrintTail()
	stat.statlog.Info("")
	return

}
func (stat *Mystat) NoCheckAndPrint() {
	stat.Print()
	stat.Reset()
}
func (stat *Mystat) PrintHeader() {
	t2 := time.Now()
	stat.statlog.Info("Statistic in %ds,  CTime: %s", stat.statGap, t2.Format("2006-01-02 15:04:05"))
	stat.statlog.Info("\n---------------------\nHead Information\n---------------------\n")

	line := fmt.Sprintf("%18s", "")
	line1 := fmt.Sprintf("%-18s", "total:")
	line2 := fmt.Sprintf("%-18s", "count /1s:")
	name := ""
	value := uint(0)
	for e := stat.vHeadItems.Front(); e != nil; e = e.Next() {
		name = e.Value.(string)
		value = stat.GetStat(name)
		line = fmt.Sprintf("%s|%9s", line, name)
		line1 = fmt.Sprintf("%s|%9d", line1, value)
		line2 = fmt.Sprintf("%s|%9d", line2, value/uint(stat.statGap))
	}
	stat.statlog.Info("%s", line)
	stat.statlog.Info("%s", line1)
	stat.statlog.Info("%s", line2)

}
func (stat *Mystat) PrintBody() {
	stat.statlog.Info("\n---------------------\nOperation Information\n---------------------")

	line := fmt.Sprintf("%-18s", "Op")

	for e := stat.vBodyColItems.Front(); e != nil; e = e.Next() {
		line = fmt.Sprintf("%s|%8s ", line, e.Value)

	}

	line = fmt.Sprintf("%s|%8s|%8s|%8s|%15s|>%3d(ms)|>%3d(ms)|>%3d(ms)|%4d(ms)|", line, "tcount", "avg_de_ms", "de_max_ms", "max_ip",
		stat.delayUp,
		stat.delayUp2,
		stat.delayUp3,
		stat.timeout)

	stat.statlog.Info("%s", line)
	for eRow := stat.vBodyRowItems.Front(); eRow != nil; eRow = eRow.Next() {
		rowTotalCount := uint(0)
		name := eRow.Value.(string)
		line = fmt.Sprintf("%-18s", name+":")
		colname := ""
		value := uint(0)
		for eCol := stat.vBodyColItems.Front(); eCol != nil; eCol = eCol.Next() {
			colname = eCol.Value.(string)
			value = stat.GetStatValueByTab(name, colname)
			rowTotalCount += value
			line = fmt.Sprintf("%s|%8d ", line, value)
		}

		tcount, avg, max, up, up2, up3 := stat.TimeStatGet(name)
		maxIp := stat.GetStat(name + STAT_MAX_IP)
		if rowTotalCount == 0 && tcount == 0 {
			continue
		}

		line = fmt.Sprintf("%s|%8d|%8.3f|%8.3f|%15s|%8d|%8d|%8d",
			line,
			tcount,
			avg,
			max,
			inet_ntoa(int(maxIp)).String(),
			up,
			up2,
			up3)

		line = fmt.Sprintf("%s|%8d|", line, stat.GetTimeOut(name))

		stat.statlog.Info("%s", line)
	}
}
func (stat *Mystat) PrintTail() {
	if stat.vTailItems.Len() > 0 {
		stat.statlog.Info("\n---------------------\nTail Information\n---------------------")
		name := ""
		for e := stat.vTailItems.Front(); e != nil; e = e.Next() {
			name = e.Value.(string)
			stat.statlog.Info("%-17s | %8d", name+"#", stat.GetStat(name))
		}
	}
}

func (stat *Mystat) PrintRowError() {
	stat.statlog.Info("\n---------------------\nError Information\n---------------------")
	str := fmt.Sprintf("%-17s", "Op")
	format1 := ""
	format2 := ""
	for i := 0; i < ERRNUM; i++ {
		format1 = fmt.Sprintf("%s%d", "Err", i+1)
		format2 = fmt.Sprintf(STR_FMT, format1)
		str += SPT
		str += format2
	}

	format1 = fmt.Sprintf("%s", "total count")
	format2 = fmt.Sprintf(STR_FMT, format1)
	str += SPT
	str += format2
	stat.statlog.Info("%s", str)

	allcount := 0
	count := 0
	for k, v := range stat.mapErrNum {
		count = 0
		if len(v) == 0 {
			continue
		}
		str = ""
		format1 = fmt.Sprintf("%-17s", k+"_E")
		str += format1
		topkArray := GetTopn(v, ERRNUM)
		for _, item := range topkArray {
			format1 = fmt.Sprintf("%d/%d", item, v[item])
			format2 = fmt.Sprintf(STR_FMT, format1)
			count += int(v[item])
			str += SPT
			str += format2
		}
		for i := len(topkArray); i < ERRNUM; i++ {
			format1 = fmt.Sprintf("%d/%d", 0, 0)
			format2 = fmt.Sprintf(STR_FMT, format1)
			str += SPT
			str += format2
		}

		allcount += count
		format1 = fmt.Sprintf("%d", count)
		format2 = fmt.Sprintf(STR_FMT, format1)
		str += SPT
		str += format2
		stat.statlog.Info("%s", str)

	}
	stat.statlog.Info("---------------------")
	str = fmt.Sprintf("%-17s", "TOTAL")
	topnErrnoArray := GetTopn(stat.errnoCountMap, ERRNUM)
	for _, val := range topnErrnoArray {
		format1 = fmt.Sprintf("%d/%d", val, stat.errnoCountMap[val])
		format2 = fmt.Sprintf(STR_FMT, format1)
		str += SPT
		str += format2
	}

	for i := len(topnErrnoArray); i < ERRNUM; i++ {
		format1 = fmt.Sprintf("%d/%d", 0, 0)
		format2 = fmt.Sprintf(STR_FMT, format1)
		str += SPT
		str += format2
	}
	format1 = fmt.Sprintf("%d", allcount)
	format2 = fmt.Sprintf(STR_FMT, format1)
	str += SPT
	str += format2
	stat.statlog.Info("%s", str)

}
func (stat *Mystat) PrintIpError() {
	if !stat.isHadIpErr {
		return
	}
	stat.statlog.Info("\n---------------------\nIP Information\n---------------------")
	str := ""
	format1 := fmt.Sprintf("%-17s", "retcode")
	format2 := ""
	str += format1

	for i := 0; i < IPNUM; i++ {
		format1 = fmt.Sprintf("%s%d", "ip", i+1)
		format2 = fmt.Sprintf(IP_FMT, format1)
		str += SPT
		str += format2
	}
	stat.statlog.Info("%s", str)
	for retcode, v := range stat.mapErrIp {
		count := 0
		if len(v) == 0 {
			continue
		}
		format1 = fmt.Sprintf("%-17d", retcode)
		str = ""
		str += format1
		topkArrayIp := GetTopn(v, IPNUM)
		for _, ip := range topkArrayIp {
			format1 = fmt.Sprintf("%s/%d", inet_ntoa(ip).String(), v[ip])
			format2 = fmt.Sprintf(IP_FMT, format1)
			count += int(v[ip])
			str += SPT
			str += format2

		}
		for i := len(topkArrayIp); i < IPNUM; i++ {
			format1 = fmt.Sprintf("%d/%d", 0, 0)
			format2 = fmt.Sprintf(IP_FMT, format1)
			str += SPT
			str += format2
		}
		stat.statlog.Info("%s", str)

	}

}

func (stat *Mystat) ClearAll() {
	for k, _ := range stat.timeOutMap {
		delete(stat.timeOutMap, k)
	}
	for k, _ := range stat.countMap {
		delete(stat.countMap, k)
	}
	for k, _ := range stat.errnoCountMap {
		delete(stat.errnoCountMap, k)
	}
	for k, v := range stat.mapErrNum {
		for k1, _ := range v {
			delete(v, k1)
		}
		delete(stat.mapErrNum, k)
	}
	for k, v := range stat.mapErrIp {
		for k1, _ := range v {
			delete(v, k1)
		}
		delete(stat.mapErrIp, k)
	}

}
func (stat *Mystat) Reset() {
	for k, _ := range stat.timeOutMap {
		delete(stat.timeOutMap, k)
	}

	for k, _ := range stat.countMap {
		delete(stat.countMap, k)
	}
	for k, _ := range stat.errnoCountMap {
		delete(stat.errnoCountMap, k)
	}

	for k, v := range stat.mapErrNum {
		for k1, _ := range v {
			delete(v, k1)
		}
		delete(stat.mapErrNum, k)

	}

	for k, v := range stat.mapErrIp {
		for k1, _ := range v {
			delete(v, k1)
		}
		delete(stat.mapErrIp, k)
	}

}
