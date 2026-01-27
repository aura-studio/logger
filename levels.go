package logger

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var (
	ErrUnknownLogLevel = errors.New("unknown log level")
	ErrLevelNotSet     = errors.New("log level not set")
)

type LogLevel struct {
	level logrus.Level
}

func NewLogLevel(s string) *LogLevel {
	if gjson.Valid(s) {
		result := gjson.Parse(s)
		if result.IsObject() {
			s = gjson.Get(s, "level").String()
		} else {
			s = result.String()
		}
	}

	var level logrus.Level

	switch strings.ToLower(s) {
	case "panic":
		level = logrus.PanicLevel
	case "fatal":
		level = logrus.FatalLevel
	case "error":
		level = logrus.ErrorLevel
	case "warn", "warning":
		level = logrus.WarnLevel
	case "info", "print":
		level = logrus.InfoLevel
	case "debug":
		level = logrus.DebugLevel
	case "trace":
		level = logrus.TraceLevel
	default:
		panic(fmt.Errorf("%w: %s", ErrUnknownLogLevel, s))
	}

	return &LogLevel{level}
}

func (l *LogLevel) ToLogrus() logrus.Level {
	return l.level
}

type LogLevels struct {
	logLevels []*LogLevel
}

func NewLogLevels(i interface{}) *LogLevels {
	switch i := i.(type) {
	case string:
		s := i

		if gjson.Valid(s) {
			r := gjson.Get(s, "level")
			switch {
			case r.IsArray():
				strs := make([]string, 0, len(r.Array()))
				for _, str := range gjson.Get(s, "level").Array() {
					strs = append(strs, str.String())
				}
				return NewLogLevels(strs)
			case r.Type == gjson.String:
				s = r.String()
			default:
				panic(fmt.Errorf("%w: %s", ErrUnknownLogLevel, s))
			}
		}

		logLevel := NewLogLevel(s)
		logLevels := make([]*LogLevel, 0, logLevel.ToLogrus()+1)

		for i := logrus.Level(0); i <= logLevel.ToLogrus(); i++ {
			logLevels = append(logLevels, NewLogLevel(i.String()))
		}

		return &LogLevels{logLevels}

	case []string:
		strs := i

		logLevels := make([]*LogLevel, 0, len(strs))
		for _, str := range strs {
			logLevels = append(logLevels, NewLogLevel(str))
		}

		return &LogLevels{logLevels}
	default:
		panic(fmt.Errorf("%w: %T", ErrUnknownLogLevel, i))
	}
}

func (l *LogLevels) ToLogrus() []logrus.Level {
	logrusLevels := make([]logrus.Level, 0, len(l.logLevels))
	for _, level := range l.logLevels {
		logrusLevels = append(logrusLevels, level.ToLogrus())
	}

	return logrusLevels
}
