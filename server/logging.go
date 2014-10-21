package weavedns

import (
	"io"
	"io/ioutil"
	"log"
	"os"
)

const (
	standard_log_flags = log.Ldate | log.Ltime | log.Lshortfile
)

// Inspired by http://www.goinggo.net/2013/11/using-log-package-in-go.html

var (
	Debug   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func InitLogging(debugHandle io.Writer,
	infoHandle io.Writer,
	warningHandle io.Writer,
	errorHandle io.Writer) {

	Debug = log.New(debugHandle, "DEBUG: ", standard_log_flags)
	Info = log.New(infoHandle, "INFO: ", standard_log_flags)
	Warning = log.New(warningHandle, "WARNING: ", standard_log_flags)
	Error = log.New(errorHandle, "ERROR: ", standard_log_flags)
}

func init() {
	InitLogging(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
}
