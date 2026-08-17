package assembly

import (
	"math"
	"testing"
)

func TestAssemblyStaticAdmissionValidatesPathsAndFloatPayloads(t *testing.T) {
	if !staticPathValid(nil, []string{"pkg", "Type"}) {
		t.Fatal("staticPathValid rejected a nonempty path")
	}
	if staticPathValid(nil, []string{"pkg", ""}) {
		t.Fatal("staticPathValid accepted an empty path component")
	}
	if staticFloatBitsValid(nil, math.Float64bits(math.NaN())) {
		t.Fatal("staticFloatBitsValid accepted NaN")
	}
}
