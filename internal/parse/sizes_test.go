package parse_test

// Lines of goVars, measured once on the Go grammar (2026-11-16): 10,000 lines (0.2 MB) need 29 MiB of grammar memory,
// 60,000 (1.1 MB) need 85 MiB, between the recycle size of 64 MiB and the cap, and 100,000 lines already exceed the
// 128 MiB cap.
const (
	wellUnderTheCap    = 10_000
	overTheRecycleSize = 60_000
	overTheCap         = 150_000
)
