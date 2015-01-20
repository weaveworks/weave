package testing

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

func AssertNoErr(t *testing.T, err error) {
	if err != nil {
		Fatalf(t, "Unexpected error: %s", err)
	}
}

func AssertEqualuint64(t *testing.T, got, wanted uint64, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertEqualInt(t *testing.T, got, wanted int, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertEqualString(t *testing.T, got, wanted string, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s '%s' but got '%s'", desc, wanted, got)
	}
}

func AssertStatus(t *testing.T, got int, wanted int, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertErrorInterface(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if !gotT.Implements(wantedT) {
		Fatalf(t, "Expected %s but got %s (%s)", wantedT.String(), gotT.String(), desc)
	}
}

func AssertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if gotT != wantedT {
		Fatalf(t, "Expected %s but got %s (%s)", wantedT.String(), gotT.String(), desc)
	}
}

func AssertType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted)
	if gotT != wantedT {
		Fatalf(t, "Expected %s but got %s (%s)", wantedT.String(), gotT.String(), desc)
	}
}

// Like testing.Fatalf, but adds the stack trace of the current call
func Fatalf(t *testing.T, format string, args ...interface{}) {
	t.Fatalf(format+"\n%s", append(args, StackTrace())...)
}

func StackTrace() string {
	buf := make([]byte, 1<<20)
	stacklen := runtime.Stack(buf, false)
	return string(buf[:stacklen])
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
