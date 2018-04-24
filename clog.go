package zlog

import (
	"os"
	"fmt"
	"time"
	"runtime"
  "strings"
	"io/ioutil"
  
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
  "github.com/satori/go.uuid"
	"github.com/json-iterator/go"

	"jinygo/core/utils"
)

const (
	LOG = "log"

	DEBUG = "debug"
	INFO = "info"
	ERROR = "error"
	WARN = "warn"

	DEFAULT = "default"
	CONSOLE = "console"
)

var (
	Coo *CooLogger
	cooConf *CooLogConfig
  encoderConfig zapcore.EncoderConfig
)

type CooLogger struct {
	core        zapcore.Core
	development bool
	name        string
	errorOutput zapcore.WriteSyncer
	addCaller   bool
	addStack    zapcore.LevelEnabler
	callerSkip  int
	appName     string
}

type CooLogConfig struct {
	Dev     bool               `json:"dev" yaml:"dev"`
	Level   string             `json:"level" yaml:"level"`
	Encoder []string           `json:"encoder" yaml:"encoder"`
	Encode  map[string]string  `json:"encode" yaml:"encode"`
	Key     map[string]string  `json:"key" yaml:"key"`
	OutPuts []string           `json:"outputs" yaml:"outputs"`
}

func defaultLogConfig() {
	cooConf = &CooLogConfig{
		Dev:         true,
		Level:       DEBUG,
		Encoder:     []string{CONSOLE},
		Encode:      map[string]string{"time": "local", "level": "capital", "duration": "string", "caller": "short"},
		Key:         map[string]string{
			"name": "logger",
			"time": "time",
			"level": "level",
			"caller": "caller",
			"message": "msg",
			"stacktrace": "stacktrace",
		},
		OutPuts:     []string{"stderr"},
	}
}

func (clog *CooLogConfig) lvlEncoder() {
	lvl := clog.Encode["level"]
	switch lvl {
	case "capital":
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	case "capitalColor":
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	case "color":
		encoderConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	default:
		encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	}
}

func (clog *CooLogConfig) timeEncoder() {
	time := clog.Encode["time"]
	switch time {
	case "local":
		encoderConfig.EncodeTime = logEncodeTime
	case "iso8601", "ISO8601":
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	case "millis":
		encoderConfig.EncodeTime = zapcore.EpochMillisTimeEncoder
	case "nanos":
		encoderConfig.EncodeTime = zapcore.EpochNanosTimeEncoder
	default:
		encoderConfig.EncodeTime = zapcore.EpochTimeEncoder
	}
}

func (clog *CooLogConfig) durEncoder() {
	dur := clog.Encode["duration"]
	switch dur {
	case "string":
		encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	case "nanos":
		encoderConfig.EncodeDuration = zapcore.NanosDurationEncoder
	default:
		encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	}
}

func (clog *CooLogConfig) callerEncoder() {
	caller := clog.Encode["caller"]
	switch caller {
	case "full":
		encoderConfig.EncodeCaller = zapcore.FullCallerEncoder
	default:
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}
}

func New(app string) *CooLogger {

	encoderConfig.NameKey       = cooConf.Key["name"]
	encoderConfig.TimeKey       = cooConf.Key["time"]
	encoderConfig.LevelKey      = cooConf.Key["level"]
	encoderConfig.CallerKey     = cooConf.Key["caller"]
	encoderConfig.MessageKey    = cooConf.Key["message"]
	encoderConfig.StacktraceKey = cooConf.Key["stacktrace"]

	encoderConfig.LineEnding = zapcore.DefaultLineEnding

	cooConf.timeEncoder()
	cooConf.lvlEncoder()
	cooConf.durEncoder()
	cooConf.callerEncoder()

	var lvl zap.AtomicLevel
	switch cooConf.Level {
	case DEBUG:
		lvl = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case WARN:
		lvl = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case ERROR:
		lvl = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		lvl = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	var outputs []string
	for _, p := range cooConf.OutPuts {
		if p == DEFAULT {
			logfile := fmt.Sprintf("%s_%s.log", app, time.Now().Format("2006-01-02_15-04-05"))
			outputs = append(outputs, logfile)
		} else {
			outputs = append(outputs, p)
		}
	}

	sink, _, _ := zap.Open(outputs...)

	var cores []zapcore.Core
	for _, e := range cooConf.Encoder {
		switch e {
		case CONSOLE:
			consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
			cores = append(cores, zapcore.NewCore(consoleEncoder, sink, lvl))
		case utils.JSON:
			jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
			cores = append(cores, zapcore.NewCore(jsonEncoder, sink, lvl))
		}
	}

	//errSink, _, err := zap.Open("stderr")
	//if err != nil {
	//	closeOut()
	//}

	log := &CooLogger{
		core:        zapcore.NewTee(cores...),
		development: cooConf.Dev,
		errorOutput: zapcore.Lock(os.Stderr),
		addStack:    zapcore.FatalLevel + 1,
		addCaller:   true,
		appName: app,
	}
	return log
}
func buildLogConfig(runMode, profileType string) {
	logfile := "log." + profileType
	conf, err := utils.AppFile("config", runMode, profileType, logfile)
	if err != nil {
		defaultLogConfig()
	} else {
		buf, err := ioutil.ReadFile(conf)
		if err != nil {
			fmt.Errorf("Yaml log file read error")
		}
		switch profileType {
		case utils.JSON:
			jsoniter.Unmarshal(buf, &cooConf)
		case utils.YAML:
			yaml.Unmarshal(buf, &cooConf)
		default:
			fmt.Errorf("This file type is not supported")
			return
		}
	}
}

func Init(appName, runMode, profileType string) {
	buildLogConfig(runMode, profileType)
	Coo = New(appName)
}

func (clog *CooLogger) check(lvl zapcore.Level, msg string) *zapcore.CheckedEntry {
	const callerSkipOffset = 2
	ent := zapcore.Entry{
		LoggerName: Coo.name + uuid.Must(uuid.NewV4()).String(),
		Time:       time.Now(),
		Level:      lvl,
		Message:    msg,
	}
	ce := Coo.core.Check(ent, nil)
	willWrite := ce != nil

	switch ent.Level {
	case zapcore.PanicLevel:
		ce = ce.Should(ent, zapcore.WriteThenPanic)
	case zapcore.FatalLevel:
		ce = ce.Should(ent, zapcore.WriteThenFatal)
	case zapcore.DPanicLevel:
		if Coo.development {
			ce = ce.Should(ent, zapcore.WriteThenPanic)
		}
	}

	if !willWrite {
		return ce
	}

	ce.ErrorOutput = Coo.errorOutput
	if Coo.addCaller {
		ce.Entry.Caller = zapcore.NewEntryCaller(runtime.Caller(Coo.callerSkip + callerSkipOffset))
		if !ce.Entry.Caller.Defined {
			fmt.Fprintf(Coo.errorOutput, "%v Logger.check error: failed to get caller\n", time.Now().Local())
			Coo.errorOutput.Sync()
		}
	}
	if Coo.addStack.Enabled(ce.Entry.Level) {
		ce.Entry.Stack = zap.Stack("").String
	}

	return ce
}
func Debug(details ...interface{}) {
	if ce := Coo.check(zapcore.DebugLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Info(details ...interface{}) {
	if ce := Coo.check(zapcore.InfoLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Warn(details ...interface{}) {
	if ce := Coo.check(zapcore.WarnLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Error(details ...interface{}) {
	if ce := Coo.check(zapcore.ErrorLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func DPanic(details ...interface{}) {
	if ce := Coo.check(zapcore.DPanicLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Panic(details ...interface{}) {
	if ce := Coo.check(zapcore.PanicLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Fatal(details ...interface{}) {
	if ce := Coo.check(zapcore.FatalLevel, fmt.Sprint(details...)); ce != nil {
		ce.Write()
	}
}

func Sync() error {
	return Coo.core.Sync()
}

func logEncodeTime(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("[2006-01-02 15:04:05] "))
}
