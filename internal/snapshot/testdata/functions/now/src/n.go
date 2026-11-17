package a

type T struct{}

func (t T) Method() {}

func Plain() { f := func() {}; f() }
