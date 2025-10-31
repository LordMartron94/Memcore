package primitives

//go:inline
func alignIdxUp(idx uint64, alignment uint64) uint64 {
	mask := alignment - 1
	return (idx + mask) &^ mask
}

// Note: alignIdxUp is also in memforge, but duplicated here for inlining benefits.
