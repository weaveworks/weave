package testing

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func CallSite() string {
	_, file, line, ok := runtime.Caller(2)
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
		t.Fatal(CallSite(), err)
	}
}

func AssertStatus(t *testing.T, got int, wanted int, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(), desc, wanted, got)
	}
}

func AssertEqualUint32(t *testing.T, got uint32, wanted uint32, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(), desc, wanted, got)
	}
}

func AssertEqualUint64(t *testing.T, got uint64, wanted uint64, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s %d but got %d", CallSite(), desc, wanted, got)
	}
}

func AssertEqualStr(t *testing.T, got string, wanted string, desc string) {
	if got != wanted {
		t.Fatalf("%s: Expected %s '%s' but got '%s'", CallSite(), desc, wanted, got)
	}
}

func AssertErrorInterface(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if !gotT.Implements(wantedT) {
		t.Fatalf("%s: Expected %s but got %s (%s)", CallSite(), wantedT.String(), gotT.String(), desc)
	}
}

func AssertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if gotT != wantedT {
		t.Fatalf("%s: Expected %s but got %s (%s)", CallSite(), wantedT.String(), gotT.String(), desc)
	}
}
