# Stat Manuals
## About

	golang application local business monitor
	application can integrate stat module by follow directions
### Stat Log Style
  <br>
  Statistic in 60s,  CTime: 2017-09-07 06:57:23
  <br>
  ---------------------
  <br>
  Head Information
  <br>
  ---------------------
  <br>
         	     |    MsgIn|   MsgOut 
   <br>
   total:            |        0|        0
   <br>
   count /1s:        |        0|        0
   <br>
   ---------------------
   <br>
   Operation Information
   <br>
   ---------------------
   <br>
   Op                |  tcount|avg_de_ms|de_max_ms| max_ip|> 20(ms)|>   50(ms)|>100(ms)| 180(ms)|
   <br>
   ---------------------
   <br>
   Error Information
   <br>
   ---------------------
   <br>
   Op                | Err1        | Err2        | Err3        | Err4        |    Err5        | total count
   <br>
   ---------------------
   <br>
   TOTAL             | 0/0         | 0/0         | 0/0         | 0/0         | 0/0         | 0
   <br>
   ---------------------
   <br>
   IP Information
   <br>
   ---------------------
   <br>
   retcode           | ip1               | ip2               | ip3
   <br>
   ---------------------
   <br>
   Tail Information
   <br>
   ---------------------
   <br>
   INDEGREE_Recive(B)#   |        0
   <br>
   INDEGREE_Send(B)#     |        0
   <br>
   
## Dependences
  <br>
  github.com/astaxie/beego/logs
  <br>
  original beego  still need to modify to support stat module
  <br>
  
### First Change
   <br>
   add blankprefix support
   <br>
   modify logs/file.go as show below
   <br>
   
#### Json Config Support

```golang
    type fileLogWriter struct {
        sync.RWMutex
        Filename         string `json:"filename"`
        BlankPrefix      bool   `json:"blankprefix"`
    }
```
<br>

#### FileWriter Default BlankPrefix Support

```golang
    func newFileWriter() Logger {
        w := &fileLogWriter{
                Daily:      true,
                MaxDays:    7,
                Rotate:     true,
                BlankPrefix: false,
                RotatePerm: "0440",
                Level:      LevelTrace,
                Perm:       "0660",
        }
        return w
    }
```
<br>

#### Real Write Support

```golang
       func (w *fileLogWriter) WriteMsg(when time.Time, msg string, level int)  error {
             if level > w.Level {
                return nil
            }
            h, d := formatTimeHeader(when)
           if !w.BlankPrefix {
             msg = string(h) + msg + "\n"
           }else{
             msg = msg + "\n"
           }
	   ....
       }
```
<br>

### Second Change
<br>
Remove the logger level prefix in log line,such as [I],[D],...

#### BeeLogger Add Member attribute
   
   attribute blankPrefix
   <br>
```golang
    type BeeLogger struct {
        lock                sync.Mutex
        level               int
        init                bool
        enableFuncCallDepth bool
        loggerFuncCallDepth int
        asynchronous        bool
        blankPrefix         bool
        msgChanLen          int64
        msgChan             chan *logMsg
        signalChan          chan string
        wg                  sync.WaitGroup
        outputs             []*nameLogger
    }
```
#### BeeLogger Member Default Value
   
   blankPrefix set default value
   <br>
```golang
    func NewLogger(channelLens ...int64) *BeeLogger {
         bl := new(BeeLogger)
         bl.level = LevelDebug
         bl.loggerFuncCallDepth = 2
        bl.blankPrefix = false
        bl.msgChanLen = append(channelLens, 0)[0]
        if bl.msgChanLen <= 0 {
                bl.msgChanLen = defaultAsyncMsgLen
        }
        bl.signalChan = make(chan string, 1)
        bl.setLogger(AdapterConsole)
        return bl
    }
```


#### BeeLogger Add Interface

```golang
    func (bl *BeeLogger) BlankPrefix() {
        bl.blankPrefix = true
    }
```

#### BeeLogger WriteMsg Modify
   
   clause msg = levelPrefix[logLevel] + msg add condition
   <br>
```golang
    func (bl *BeeLogger) writeMsg(logLevel int, msg string, v ...interface{})         error {
        if !bl.init {
                bl.lock.Lock()
                bl.setLogger(AdapterConsole)
                bl.lock.Unlock()
        }

        if len(v) > 0 {
                msg = fmt.Sprintf(msg, v...)
        }
        when := time.Now()
        if bl.enableFuncCallDepth {
                _, file, line, ok := runtime.Caller(bl.loggerFuncCallDepth)
                if !ok {
                        file = "???"
                        line = 0
                }
                _, filename := path.Split(file)
                msg = "[" + filename + ":" + strconv.Itoa(line) + "] " + msg
        }
        //set level info in front of filename info
        if logLevel == levelLoggerImpl {
                logLevel = LevelEmergency
        } else {
             if !bl.blankPrefix {
                     msg = levelPrefix[logLevel] + msg
             }
        }
        ....
     }
```

<br>

## Using Help

### Base Initialize
<br>
```golang
	logconfig := make(stat.LoggerParm)
	logconfig.level = "info"
	logconfig.path = "./stat"
	logconfig.namePrefix = "test"
	logconfig.filename = "stat.log"
	logconfig.maxfilesize = 10000
	logconfig.maxdays = 7
	logconfig.maxlines = 10000
	logconfig.chanlen = 10000
	stat.Init(logconfig, 60)
	stat.StatProc()
```
### Application Initialize
```golang
    stat.SetDelayUp(20,50,100)
```

### Add Stat Record Data
```golang
    type StatItem struct {
	  Name      string //统计的接口名
	  Delay     uint   //接口执行的延时,单位ms
	  Errcode   int    //当次接口请求的错误码,0--成功
	  Ipsrc     net.IP //请求的来源ip
	  Payload   uint   //请求的载荷
	  Direction int    //上行or下行          1 --- 上行   0 ----下行
	  InOrOut   int    //入度请求还是出度请求  1 ---- in  0 ----out
    }

    stat.Push(elem)
```
### Exit Must Call
```golang
    stat.Exit()
```    
### Already Include Stat Options
```golang
    const (
       STAT_IN			        = "MsgIn"
       STAT_OUT				    = "MsgOut"
       INDEGREE_Recive 		    = "InDegree_Recive(MB)"
       INDEGREE_Send			= "InDegree_Send(MB)"
       OUTDEGREE_Recive 		= "OutDegree_Recive(MB)"
       OUTDEGREE_Send			= "OutDegree_Send(MB)"
    )
```
### User How to Add Stat Options
   
   user can add user define option and call interface below to tag itemName to Stat Module
 ```golang
	AddReportHeadItem(itemName string)
	AddReportBodyRowItem(itemName string)
	AddReportBodyColItem(itemName string)
	AddReportTailItem(itemName string)
	AddReportErrorItem(itemName string)
```

