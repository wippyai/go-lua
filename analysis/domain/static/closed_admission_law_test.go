package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestStaticClosedAdmissionRejectsDanglingRecursiveGraphs(t *testing.T) {
	authority := &Authority{
		results:       []resultRow{{kind: KindBottom}, {kind: KindTop}},
		closedByBytes: make(map[string]Value),
	}
	dangling := typ.NewRecursivePlaceholder("Dangling")
	for _, value := range []struct {
		name  string
		value typ.Type
	}{
		{name: "recursive", value: dangling},
		{name: "nested", value: typ.NewArray(dangling)},
	} {
		if _, err := authority.addClosed(value.value); err == nil {
			t.Fatalf("%s dangling graph was admitted", value.name)
		}
	}
}

func TestStaticClosedAdmissionRejectsUnissuedProductiveRecursiveGraph(t *testing.T) {
	authority := &Authority{
		results:       []resultRow{{kind: KindBottom}, {kind: KindTop}},
		closedByBytes: make(map[string]Value),
	}
	closed := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewArray(self)
	})
	if _, err := authority.addClosed(closed); err == nil {
		t.Fatal("unissued productive recursive graph was admitted")
	}
}
