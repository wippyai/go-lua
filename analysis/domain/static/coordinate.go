package static

// Coordinate is one Authority-local dense Static factor key.  It deliberately
// carries authority identity rather than a portable static type reference: a
// portable reference names authored meaning, while a Coordinate names one
// exact solver cell in one sealed Authority.
//
// Its fields stay private so no caller can manufacture a coordinate or turn a
// foreign-but-equal reference into a local Factor key.
type Coordinate struct {
	authority *Authority
	index     uint32
}

func (a *Authority) coordinateFor(key coordinateKey) (Coordinate, bool) {
	if a == nil || key.reference.Valid() == false {
		return Coordinate{}, false
	}
	index, ok := a.coordinateIndex[key]
	if !ok {
		return Coordinate{}, false
	}
	return Coordinate{authority: a, index: index}, true
}
