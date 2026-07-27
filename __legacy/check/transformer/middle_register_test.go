package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestMiddleRegisterSchemaIsCanonicalTypedAndStableAcrossWrites(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("typed-middle-registers")))
	build := func(reverse bool) *Arena {
		arena := NewArena(standard.Registry())
		if !arena.bindLexicalOwner(owner) {
			t.Fatal("bind lexical owner")
		}
		if reverse {
			arena.bindExpressionValue(factflow.ExprRef(31))
			arena.bindCallResult(cfg.Point(17), 2)
			arena.bindEnvironmentSymbol(symbol.ID(9))
		} else {
			arena.bindEnvironmentSymbol(symbol.ID(9))
			arena.bindCallResult(cfg.Point(17), 2)
			arena.bindExpressionValue(factflow.ExprRef(31))
		}
		// Rebinding is a write/use of the same lexical storage coordinate, not
		// permission to mint another formal root.
		arena.bindEnvironmentSymbol(symbol.ID(9))
		if err := arena.sealMiddleRegisterSchema(); err != nil {
			t.Fatal(err)
		}
		return arena
	}

	left, right := build(false), build(true)
	slots := []statekey.Value{
		statekey.SymbolValue(9),
		statekey.CallResult(17, 2),
		statekey.ExpressionValue(31),
	}
	for index, slot := range slots {
		leftRoot, leftOK := left.middleRoot(slot)
		rightRoot, rightOK := right.middleRoot(slot)
		if !leftOK || !rightOK || leftRoot != rightRoot || leftRoot != (Root{Kind: RootMiddle, Index: uint32(index)}) {
			t.Fatalf("slot %d Middle root = %#v/%v and %#v/%v", slot, leftRoot, leftOK, rightRoot, rightOK)
		}
		formalRoot, ok := left.middle.formalRoot(owner, leftRoot)
		if !ok || formalRoot.Ordinal() != uint64(index+1) {
			t.Fatalf("slot %d formal root = %#v/%v", slot, formalRoot, ok)
		}
	}

	symbolRoot, _ := left.middleRoot(statekey.SymbolValue(9))
	scalar, scalarOK := left.middleValue(statekey.SymbolValue(9))
	path := left.middleSymbolPath(symbol.ID(9))
	if !scalarOK || scalar == 0 || path == 0 || left.values[scalar].root != symbolRoot || left.paths[path].root != symbolRoot {
		t.Fatalf("symbol scalar/path do not share Middle root: root=%#v value=%#v path=%#v", symbolRoot, left.values[scalar], left.paths[path])
	}
	if left.middleSymbolPath(symbol.ID(31)) != 0 {
		t.Fatal("non-Symbol register became path-addressable")
	}
}

func TestMiddleRegisterInventoryIncludesDeclarationWithoutExecutableTerm(t *testing.T) {
	arena := NewArena(standard.Registry())
	declaration := statekey.SymbolValue(symbol.ID(41))
	if arena.validEnvironmentSlot(declaration) {
		t.Fatal("declaration unexpectedly occurred in executable term inventory")
	}
	if err := arena.includeMiddleRegisterInventory([]statekey.Value{declaration}); err != nil {
		t.Fatal(err)
	}
	if len(arena.values) != 1 {
		t.Fatalf("inventory admission minted executable syntax: %d values", len(arena.values))
	}
	if err := arena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	root, exact := arena.middleRoot(declaration)
	if !exact || root != (Root{Kind: RootMiddle, Index: 0}) {
		t.Fatalf("declaration-only slot has no canonical MID root: %#v/%t", root, exact)
	}
}
