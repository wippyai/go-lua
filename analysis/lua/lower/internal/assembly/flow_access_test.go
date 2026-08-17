package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestAssemblyExactLensRetainsNameKeyProvenance(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	base := c.String(assemblyTestSpan(), body, "object")
	key := c.Name(assemblyTestSpan(), body, "field")
	lens := c.LensExact(assemblyTestSpan(), body, base, key, kind.FieldName)
	if lens == 0 {
		t.Fatal("LensExact rejected an authored Name key")
	}
}
