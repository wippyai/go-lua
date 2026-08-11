package collector

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/source"
)

func TestSourceOrderAdmitsNestedBodyAsDirectRoot(t *testing.T) {
	c := New("nested-body.lua", 0, bind.GlobalCensus{})
	order := c.Source().Order()
	span := source.Span{File: "nested-body.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	entry := order.Body(span)
	nested := order.Body(span)
	if entry == 0 || nested == 0 || entry == nested {
		t.Fatalf("Body identities = %v/%v, want distinct nonzero terms", entry, nested)
	}
	if !order.SetBody(nested) {
		t.Fatalf("nested Body fill failed: %v", failure(c))
	}
	if !order.SetBody(entry, nested) || !order.SetEntry(entry) {
		t.Fatalf("entry Body fill failed: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare nested Body = %v", err)
	}
	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("Assemble nested Body = %v/%v", assembly, err)
	}
}
