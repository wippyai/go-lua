package hash

// FNV-1a constants.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// MixHash combines two hashes using FNV-1a style mixing.
func MixHash(h, v uint64) uint64 {
	h ^= v
	h *= fnvPrime64

	return h
}

// FnvString hashes a string using FNV-1a.
func FnvString(s string) uint64 {
	var h uint64 = fnvOffset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}

	return h
}
