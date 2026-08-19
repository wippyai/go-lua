package assembly

import (
	"math"
	"testing"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
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

func TestAssemblyStaticAPIAdmitsPrimitiveRows(t *testing.T) {
	c := newAssemblyCollector()
	primitive := c.Primitive(assemblyTestSpan(), statictypes.PrimitiveString)
	if primitive == 0 {
		t.Fatal("Primitive did not create a static type row")
	}
}
