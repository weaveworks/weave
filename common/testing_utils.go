package common

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
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
