package a

func flat(a, b, c bool) int {
	if a && b || c {
		return 1
	} else if b {
		return 2
	} else {
		return 3
	}
}

func deep(xs []int, ok bool) int {
	n := 0
	for _, x := range xs {
		if x > 0 {
			switch {
			case x > 10:
				if ok {
					n++
				}
			}
		}
	}
	f := func() {
		if ok {
			n--
		}
	}
	f()
	return n
}
