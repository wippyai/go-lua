package factor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestFactorBodyRootsAreOwnerFenced(t *testing.T) {
	contract, linked, packs := factorFixture(t)
	algebra, ok := New(linked, packs, contract)
	if !ok || algebra == nil || algebra.RootCount() < 2 {
		t.Fatal("seal factor roots")
	}
	first, firstOK := algebra.RootAt(0)
	second, secondOK := algebra.RootAt(1)
	if !firstOK || !secondOK || first == second {
		t.Fatal("distinct body roots")
	}
	if !algebra.Admit(first, algebra.Bottom()) || !algebra.Admit(second, algebra.Top()) {
		t.Fatal("extreme body admission")
	}
	foreign, ok := New(linked, packs, contract)
	if !ok || foreign.Owns(algebra.Bottom()) {
		t.Fatal("same Link factor accepted a foreign live owner value")
	}
}

func factorFixture(t testing.TB) (*target.Contract, *link.Link, *pack.Schema) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "effect_factor.lua", Text: []byte(`
local function first() return 1 end
local function second() return 2 end
first()
second()
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "effect_factor", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("seal type authority")
	}
	statics, _, err := staticdomain.Seal(linked, types)
	if err != nil {
		t.Fatal(err)
	}
	packs, ok := pack.Seal(linked, statics)
	if !ok {
		t.Fatal("seal pack")
	}
	return contract, linked, packs
}
