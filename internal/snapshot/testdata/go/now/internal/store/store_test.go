package store

import "testing"

func TestStore(t *testing.T) {
	_ = Put("a")
	_ = Put("b")
}
