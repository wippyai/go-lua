package value

import "math"

// sourceFloatRepresentable reports whether Value can retain the source in its
// exact non-NaN numeric class. Numeric/equality still owns IEEE payloads;
// Value needs only the one key-invalid NaN distinction for Heap projection.
func sourceFloatRepresentable(value float64) bool {
	return !math.IsNaN(value)
}
