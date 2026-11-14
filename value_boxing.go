package lua

import "math"

const preloadLimit = 256
const intPreloadLimit = 65536 // Common loop bounds

var preloadedNumbers [preloadLimit]LValue
var preloadedIntegers [intPreloadLimit * 2]LValue // [-65536, 65536)

func init() {
	for i := 0; i < preloadLimit; i++ {
		preloadedNumbers[i] = LNumber(i)
	}
	for i := -intPreloadLimit; i < intPreloadLimit; i++ {
		preloadedIntegers[i+intPreloadLimit] = LInteger(i)
	}
}

// lnumberToValue boxes v, sharing a preallocated value when v is bit-for-bit
// one of the cached floats; -0.0 equals 0.0 numerically but keeps its sign.
func lnumberToValue(v LNumber) LValue {
	iv := int(v)
	if iv >= 0 && iv < preloadLimit && math.Float64bits(float64(v)) == math.Float64bits(float64(iv)) {
		return preloadedNumbers[iv]
	}
	return v
}

func lintegerToValue(v LInteger) LValue {
	iv := int(v)
	if iv >= -intPreloadLimit && iv < intPreloadLimit {
		return preloadedIntegers[iv+intPreloadLimit]
	}
	return v
}
