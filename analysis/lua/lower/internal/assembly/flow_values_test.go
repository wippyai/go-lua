package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblyValuesPreserveFixedOccurrenceOrder(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	first := c.Integer(assemblyTestSpan(), body, 1)
	second := c.Integer(assemblyTestSpan(), body, 2)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{first, second}, 0)
	if values == 0 {
		t.Fatal("Values rejected an authored fixed occurrence range")
	}
	row, ok := c.flow.ValueAt(0)
	if !ok || row.Owner != body || row.Fixed.Start != 0 || row.Fixed.End != 2 {
		t.Fatalf("Values row = %#v/%t, want owner/body and fixed range [0,2)", row, ok)
	}
	if got, ok := c.flow.ValueTermAt(0); !ok || got != first {
		t.Fatalf("first Values term = %d/%t, want %d", got, ok, first)
	}
	if got, ok := c.flow.ValueTermAt(1); !ok || got != second {
		t.Fatalf("second Values term = %d/%t, want %d", got, ok, second)
	}
}
