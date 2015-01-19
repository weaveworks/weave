package testing

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func CallSite(level int) string {
	_, file, line, ok := runtime.Caller(level)
	if ok {
		// Truncate file name at last file name separator.
		if index := strings.LastIndex(file, "/"); index >= 0 {
			file = file[index+1:]
		} else if index = strings.LastIndex(file, "\\"); index >= 0 {
			file = file[index+1:]
		}
	} else {
		file = "???"
		line = 1
	}
	return fmt.Sprintf("%s:%d: ", file, line)
}

func AssertNoErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(CallSite(2), err)
	}
}

func AssertEqualuint64(t *testing.T, got, wanted uint64, desc string, level int) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(level), desc, wanted, got)
	}
}

func AssertEqualInt(t *testing.T, got, wanted int, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(2), desc, wanted, got)
	}
}

func AssertEqualString(t *testing.T, got, wanted string, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s '%s' but got '%s'", CallSite(2), desc, wanted, got)
	}
}

func AssertStatus(t *testing.T, got int, wanted int, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(2), desc, wanted, got)
	}
}

func AssertErrorInterface(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if !gotT.Implements(wantedT) {
		t.Fatalf("%s: Expected %s but got %s (%s)", CallSite(2), wantedT.String(), gotT.String(), desc)
	}
}

func AssertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if gotT != wantedT {
		t.Fatalf("%s: Expected %s but got %s (%s)", CallSite(2), wantedT.String(), gotT.String(), desc)
	}
}

func AssertType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted)
	if gotT != wantedT {
		t.Fatalf("%s: Expected %s but got %s (%s)", CallSite(2), wantedT.String(), gotT.String(), desc)
	}
}

func StackTraceAll() string {
	buf := make([]byte, 1<<20)
	stacklen := runtime.Stack(buf, true)
	return string(buf[:stacklen])
}

// Borrowed from net/http tests:
// goTimeout runs f, failing t if f takes more than d to complete.
func RunWithTimeout(t *testing.T, d time.Duration, f func()) {
	ch := make(chan bool, 2)
	timer := time.AfterFunc(d, func() {
		t.Errorf("Timeout expired after %v: stacks:\n%s", d, StackTraceAll())
		ch <- true
	})
	defer timer.Stop()
	go func() {
		defer func() { ch <- true }()
		f()
	}()
	<-ch
}
