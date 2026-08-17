package assembly

import (
	"testing"

	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func TestAssemblyStaticAPIAdmitsPrimitiveRows(t *testing.T) {
	c := newAssemblyCollector()
	primitive := c.Primitive(assemblyTestSpan(), programstatic.PrimitiveString)
	if primitive == 0 {
		t.Fatal("Primitive did not create a static type row")
	}
}
