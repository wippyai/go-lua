package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalApplyCoordinateSelectorCatalogUsesExactOrdinalIdentity(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	selector := func(id symbol.ID) state.CoordinateFactorInventory {
		slots, err := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{
			keys.FromPath(pathdom.NewPath(id, "selector")),
		})
		if err != nil {
			t.Fatal(err)
		}
		inventory, err := domain.SealCoordinateFactorInventory(keys, slots)
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}

	program := &RelationProgram{bodies: []relationProgramBody{{variable: 1, productDomain: domain}}}
	forest := &formalFiberInventory{program: program}
	catalog, err := newFormalApplyCoordinateSelectorCatalog(forest, 1)
	if err != nil {
		t.Fatal(err)
	}
	leftInventory, rightInventory := selector(9911), selector(9912)
	left, err := catalog.intern(1, leftInventory)
	if err != nil {
		t.Fatal(err)
	}
	leftAgain, err := catalog.intern(1, leftInventory)
	if err != nil {
		t.Fatal(err)
	}
	right, err := catalog.intern(1, rightInventory)
	if err != nil {
		t.Fatal(err)
	}
	if left != leftAgain || left == right || left.ordinal != 1 || right.ordinal != 2 {
		t.Fatalf("selector ordinals left=%#v duplicate=%#v right=%#v", left, leftAgain, right)
	}

	// The old key used a lossy selector hash and compared only the product
	// vector inside a hash bucket. Model that worst case directly: both product
	// vectors have the same hash, while exact catalog ordinals must still name
	// two entries. No coordinate hash participates in the new key.
	const collidedProductVectorHash uint64 = 0
	body := &formalComponentTerminalBody{}
	entries := map[formalFactorExecutionCapabilityKey]string{
		{body: body, hash: collidedProductVectorHash, selector: left}:  "left",
		{body: body, hash: collidedProductVectorHash, selector: right}: "right",
	}
	if len(entries) != 2 || entries[formalFactorExecutionCapabilityKey{body: body, hash: collidedProductVectorHash, selector: left}] != "left" ||
		entries[formalFactorExecutionCapabilityKey{body: body, hash: collidedProductVectorHash, selector: right}] != "right" {
		t.Fatal("exact selector ordinals aliased under a colliding product-vector hash")
	}
}
