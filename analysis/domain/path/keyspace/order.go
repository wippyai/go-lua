package keyspace

// Less orders keys identically to the old string comparison
// Format(a) < Format(b). Ordering is by structural spelling, never by intern id,
// so sort results are deterministic across KeySpaces that hold the same values.
//
// Each order spelling is materialized exactly once at the KeySpace minting
// boundary. Sorting performs two map reads and one allocation-free string
// comparison regardless of root or path depth.
func (ks *KeySpace) Less(a, b Key) bool {
	if ks == nil || !ks.validKey(a) || !ks.validKey(b) {
		return false
	}
	return ks.compare(a, b) < 0
}

func (ks *KeySpace) compare(a, b Key) int {
	left := ks.formatByKey[a]
	right := ks.formatByKey[b]
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
