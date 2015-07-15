package common

// Assert test is true, panic otherwise
func Assert(test bool) {
	if !test {
		panic("Assertion failure")
	}
}

func OnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
