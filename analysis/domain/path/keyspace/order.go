package keyspace

// Less orders keys identically to the old string comparison
// Format(a) < Format(b). Ordering is by structural spelling, never by intern id,
// so sort results are deterministic across KeySpaces that hold the same values.
//
// The comparison streams the root spelling then the segment suffix without
// materializing the full Format string, which keeps lexicographic order exact
// over the concatenation root+suffix.
func (ks *KeySpace) Less(a, b Key) bool {
	if ks == nil || !ks.validKey(a) || !ks.validKey(b) {
		return false
	}
	return ks.compare(a, b) < 0
}

func (ks *KeySpace) compare(a, b Key) int {
	ar := ks.rootSpelling(a)
	br := ks.rootSpelling(b)
	as := ks.suffix(a.Segs)
	bs := ks.suffix(b.Segs)
	return compareConcat(ar, as, br, bs)
}

// compareConcat compares a0+a1 with b0+b1 lexicographically by byte, without
// building either concatenation.
func compareConcat(a0, a1, b0, b1 string) int {
	ai, bi := 0, 0
	for {
		ab, aok := concatByteAt(a0, a1, ai)
		bb, bok := concatByteAt(b0, b1, bi)
		if !aok || !bok {
			switch {
			case !aok && !bok:
				return 0
			case !aok:
				return -1
			default:
				return 1
			}
		}
		if ab != bb {
			if ab < bb {
				return -1
			}
			return 1
		}
		ai++
		bi++
	}
}

func concatByteAt(s0, s1 string, i int) (byte, bool) {
	if i < len(s0) {
		return s0[i], true
	}
	i -= len(s0)
	if i < len(s1) {
		return s1[i], true
	}
	return 0, false
}
