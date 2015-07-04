package common

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
)

type textFormatter struct {
}

// Based off logrus.TextFormatter, which behaves completely
// differently when you don't want colored output
func (f *textFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	levelText := strings.ToUpper(entry.Level.String())[0:4]
	timeStamp := entry.Time.Format("2006/01/02 15:04:05.000000")
	if len(entry.Data) > 0 {
		fmt.Fprintf(b, "%s: %s %-44s ", levelText, timeStamp, entry.Message)
		for k, v := range entry.Data {
			fmt.Fprintf(b, " %s=%v", k, v)
		}
	} else {
		// No padding when there's no fields
		fmt.Fprintf(b, "%s: %s %s", levelText, timeStamp, entry.Message)
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

var (
	standardTextFormatter = &textFormatter{}
)

var (
	Log *logrus.Logger
)

func init() {
	Log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: standardTextFormatter,
		Hooks:     make(logrus.LevelHooks),
	}
}

func SetLogLevel(levelname string) {
	level, err := logrus.ParseLevel(levelname)
	if err != nil {
		Log.Fatal(err)
	}
	Log.Level = level
}

// For backwards-compatibility with code that only has a boolean
func EnableDebugLogging(flag bool) {
	switch {
	case flag && Log.Level < logrus.DebugLevel:
		Log.Level = logrus.DebugLevel
	case !flag && Log.Level >= logrus.DebugLevel:
		Log.Level = logrus.InfoLevel
	}
}
