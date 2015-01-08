package common

import (
	"io/ioutil"
	"os"
)

func init() {
	InitLogging(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
}
