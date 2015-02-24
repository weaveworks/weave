package common

func CheckFatal(e error) {
	if e != nil {
		Error.Fatal(e)
	}
}

func CheckWarn(e error) {
	if e != nil {
		Warning.Println(e)
	}
}
