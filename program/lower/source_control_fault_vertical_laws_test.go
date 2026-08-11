package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestSourceControlFaultVerticalResolvesEachLexicalBodyIndependently(t *testing.T) {
	p := parseBindLower(t, `do
  goto inner
  local inner = 1
  ::inner::
  inner = inner
end
goto outer
local outer = 2
::outer::
outer = outer`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("Entry is absent")
	}
	innerBody := controlSourceAt(t, p, entry, 0)
	if parent, ok := p.Source().Index().BodyParent(innerBody); !ok || parent != entry {
		t.Fatalf("inner Body parent = %v/%v, want %v", parent, ok, entry)
	}
	assertEnteredLocalFault(t, p, innerBody, 0)
	assertEnteredLocalFault(t, p, entry, 1)
	if p.Source().Faults().Count() != 2 {
		t.Fatalf("ControlFaultCount = %d, want one per lexical Body", p.Source().Faults().Count())
	}
	if p.Flow().Authored().Control().Gotos().Count() != 0 {
		t.Fatalf("invalid controls became executable Gotos: %d", p.Flow().Authored().Control().Gotos().Count())
	}
}

func assertEnteredLocalFault(t *testing.T, p *program.Program, body keyspace.Term, sourceIndex int) {
	t.Helper()
	fault := controlSourceAt(t, p, body, sourceIndex)
	bind := controlSourceAt(t, p, body, sourceIndex+1)
	label := controlSourceAt(t, p, body, sourceIndex+2)
	blocker := boundCell(t, p, bind, 0)
	got, ok := p.Source().Faults().At(fault)
	if !ok || got.Owner != body || got.Kind != source.ControlFaultGotoEntersLocal ||
		got.Label != label || got.Blocker != blocker {
		t.Fatalf(
			"ControlFault(%v) = owner %v kind %v label %v blocker %v ok %v, want owner %v kind %v label %v blocker %v",
			fault, got.Owner, got.Kind, got.Label, got.Blocker, ok,
			body, source.ControlFaultGotoEntersLocal, label, blocker,
		)
	}
}
