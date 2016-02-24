package ipam

type consense struct {
	resultChan chan<- struct{}
}

func (c *consense) Try(alloc *Allocator) bool {
	if !alloc.ring.Empty() {
		close(c.resultChan)
		return true
	}

	alloc.establishRing()

	return false
}

func (c *consense) Cancel() {
	close(c.resultChan)
}

func (c *consense) ForContainer(ident string) bool {
	return false
}
