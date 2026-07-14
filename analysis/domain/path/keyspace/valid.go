package keyspace

// Valid reports whether ks is the live KeySpace instance that owns its intern
// universe. In particular, a shallow copy is invalid even though its copied
// intern tables may look usable: keys minted by the copy would carry the
// original instance's authority and must not cross analysis boundaries.
func (ks *KeySpace) Valid() bool {
	return ks != nil && ks.validSpace()
}
