package testing

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

func AssertTrue(t *testing.T, cond bool, desc string) {
	if !cond {
		Fatalf(t, "Expected %s", desc)
	}
}

func AssertFalse(t *testing.T, cond bool, desc string) {
	if cond {
		Fatalf(t, "Unexpected %s", desc)
	}
}

func AssertNil(t *testing.T, p interface{}, desc string) {
	val := reflect.ValueOf(p)
	if val.IsValid() && !val.IsNil() {
		Fatalf(t, "Expected nil pointer for %s but got a \"%s\": \"%s\"",
			desc, reflect.TypeOf(p), val.Interface())
	}
}

func AssertNotNil(t *testing.T, p interface{}, desc string) {
	val := reflect.ValueOf(p)
	if !val.IsValid() || val.IsNil() {
		Fatalf(t, "Unexpected nil pointer for %s", desc)
	}
}

func AssertNoErr(t *testing.T, err error) {
	if err != nil {
		Fatalf(t, "Unexpected error: %s", err)
	}
}

func AssertBool(t *testing.T, got, wanted bool, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %t but got %t", desc, wanted, got)
	}
}

func AssertEqualUint32(t *testing.T, got, wanted uint32, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertEqualuint64(t *testing.T, got, wanted uint64, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertEqualInt64(t *testing.T, got, wanted int64, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}
func AssertEqualInt(t *testing.T, got, wanted int, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertNotEqualInt(t *testing.T, got, wanted int, desc string) {
	if got == wanted {
		Fatalf(t, "Expected %s %d to be different to %d", desc, wanted, got)
	}
}

func AssertEqualString(t *testing.T, got, wanted string, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s '%s' but got '%s'", desc, wanted, got)
	}
}

func AssertNotEqualString(t *testing.T, got, wanted string, desc string) {
	if got == wanted {
		Fatalf(t, "Expected %s unlike '%s'", desc, wanted)
	}
}

func AssertEquals(t *testing.T, a1, a2 interface{}) {
	if !reflect.DeepEqual(a1, a2) {
		Fatalf(t, "Expected %+v == %+v", a1, a2)
	}
}

func AssertStatus(t *testing.T, got int, wanted int, desc string) {
	if got != wanted {
		Fatalf(t, "Expected %s %d but got %d", desc, wanted, got)
	}
}

func AssertErrorInterface(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if got == nil {
		Fatalf(t, "Expected %s but got nil (%s)", wantedT.String(), desc)
	}
	if !gotT.Implements(wantedT) {
		Fatalf(t, "Expected %s but got %s (%s)", wantedT.String(), gotT.String(), desc)
	}
}

func AssertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if got == nil {
		Fatalf(t, "Expected %s but got nil (%s)", wantedT.String(), desc)
	}
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

func AssertEmpty(t *testing.T, array interface{}, desc string) {
	if reflect.ValueOf(array).Len() != 0 {
		Fatalf(t, "Expected empty %s but got %s", desc, array)
	}
}

func AssertSuccess(t *testing.T, err error) {
	if err != nil {
		Fatalf(t, "Expected success, got '%s'", err.Error())
	}
}

func AssertPanic(t *testing.T, f func()) {
	wrapper := func() (paniced bool) {
		defer func() {
			if err := recover(); err != nil {
				paniced = true
			}
		}()

		f()
		return
	}
	AssertTrue(t, wrapper(), "Expected panic")
}

// Like testing.Fatalf, but adds the stack trace of the current call
func Fatalf(t *testing.T, format string, args ...interface{}) {
	t.Fatalf(format+"\n%s", append(args, StackTrace())...)
}

func StackTrace() string {
	return stackTrace(false)
}

func stackTrace(all bool) string {
	buf := make([]byte, 1<<20)
	stacklen := runtime.Stack(buf, all)
	return string(buf[:stacklen])
}

// Borrowed from net/http tests:
// goTimeout runs f, failing t if f takes more than d to complete.
func RunWithTimeout(t *testing.T, d time.Duration, f func()) {
	ch := make(chan bool, 2)
	timer := time.AfterFunc(d, func() {
		t.Errorf("Timeout expired after %v: stacks:\n%s", d, stackTrace(true))
		ch <- true
	})
	defer timer.Stop()
	go func() {
		defer func() { ch <- true }()
		f()
	}()
	<-ch
}
