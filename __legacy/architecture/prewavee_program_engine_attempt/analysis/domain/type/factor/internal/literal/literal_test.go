package literal_test

import (
	"testing"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/literal"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestAdmissionRecordsOnlyColdHandlesAndProjectsAfterSeal(t *testing.T) {
	source := linked(t, `return nil, true, 7, 1.5, "ok"`)
	table, err := typedomain.NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := literal.Admit(source, table)
	if err != nil {
		t.Fatal(err)
	}
	if set.Count() != 5 {
		t.Fatalf("literal count=%d, want 5", set.Count())
	}
	entry, ok := set.At(0)
	if !ok {
		t.Fatal("first literal absent")
	}
	universe, ok := origin.Build(source)
	if !ok {
		t.Fatal("literal origin universe")
	}
	if _, ok := entry.Carrier(table, universe); ok {
		t.Fatal("literal carrier constructed before Table seal")
	}
	table.Seal()
	for index := 0; index < set.Count(); index++ {
		entry, ok := set.At(index)
		if !ok {
			t.Fatalf("literal %d absent", index)
		}
		value, ok := entry.Carrier(table, universe)
		if !ok {
			t.Fatalf("literal %d carrier", index)
		}
		if origins, ok := value.Origins(); !ok || origins.Count() != 0 {
			t.Fatalf("literal %d propagated origins=%d/%v", index, origins.Count(), ok)
		}
		pack, ok := value.Data()
		if !ok || pack.IsBottom() || pack.IsTop() {
			t.Fatalf("literal %d Pack=%#v/%v", index, pack, ok)
		}
	}
}

func linked(t testing.TB, text string) *link.Link {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "literal.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "literal", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
