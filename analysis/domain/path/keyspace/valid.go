package keyspace

// Valid reports whether ks is the live KeySpace instance that owns its intern
// universe. In particular, a shallow copy is invalid even though its copied
// intern tables may look usable: keys minted by the copy would carry the
// original instance's authority and must not cross analysis boundaries.
func (ks *KeySpace) Valid() bool {
	return ks != nil && ks.validSpace()
}

// InternSize reports the total number of retained non-sentinel intern entries.
// It is a read-only growth invariant for Freeze/Apply boundaries.
func (ks *KeySpace) InternSize() int {
	if !ks.Valid() {
		return 0
	}
	return len(ks.segEntries) + len(ks.rootEntries) + len(ks.formalRootEntries) + len(ks.existentialEntries) - 4
}
