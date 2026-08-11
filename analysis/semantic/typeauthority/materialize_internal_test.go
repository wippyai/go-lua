package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRecursivePlaceholderIsAllocatedOnlyAtActualBackedge(t *testing.T) {
	nonrecursive := internalAuthority(t, `type Name = string`)
	if _, ok := nonrecursive.materialize(1); !ok {
		t.Fatal("nonrecursive alias did not materialize")
	}
	if nonrecursive.recursive[0] != nil {
		t.Fatal("nonrecursive retained a recursive placeholder")
	}

	recursive := internalAuthority(t, `type Node = Node?`)
	if _, ok := recursive.materialize(1); !ok {
		t.Fatal("recursive alias did not materialize")
	}
	if recursive.recursive[0] == nil {
		t.Fatal("recursive alias missed actual Mu backedge")
	}
}

func internalAuthority(t testing.TB, source string) *Authority {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "authority_internal.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "authority", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	authority, ok := Seal(linked)
	if !ok {
		t.Fatal("authority did not seal")
	}
	return authority
}
