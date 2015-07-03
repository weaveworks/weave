package common

func CheckFatal(e error) {
	if e != nil {
		Log.Fatal(e)
	}
}

func CheckWarn(e error) {
	if e != nil {
		Log.Warningln(e)
	}
}

// Assert test is true, panic otherwise
func Assert(test bool) {
	if !test {
		panic("Assertion failure")
	}
}
