package router

type Interaction struct {
	code       int
	resultChan chan<- interface{}
}
