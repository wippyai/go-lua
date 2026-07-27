package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	leftSlots, err := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{
		keys.FromPath(pathdom.NewPath(9911, "selector")),
	})
	if err != nil {
		t.Fatal(err)
	}
	independentlySealedLeft, err := domain.SealCoordinateFactorInventory(keys, leftSlots)
	if err != nil {
		t.Fatal(err)
	}
	leftByContent, err := catalog.intern(1, independentlySealedLeft)
	if err != nil {
		t.Fatal(err)
	}
	right, err := catalog.intern(1, rightInventory)
	if err != nil {
		t.Fatal(err)
	}
	if left != leftAgain || left != leftByContent || left == right || left.ordinal != 1 || right.ordinal != 2 {
		t.Fatalf("selector ordinals left=%#v duplicate=%#v content=%#v right=%#v", left, leftAgain, leftByContent, right)
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

func BenchmarkFormalApplyCoordinateSelectorCatalogInternDuplicate(b *testing.B) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	program := &RelationProgram{bodies: []relationProgramBody{{variable: 1, productDomain: domain}}}
	forest := &formalFiberInventory{program: program}
	catalog, err := newFormalApplyCoordinateSelectorCatalog(forest, 1)
	if err != nil {
		b.Fatal(err)
	}
	var duplicate state.CoordinateFactorInventory
	for id := symbol.ID(1); id <= 64; id++ {
		slots, slotErr := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{keys.FromPath(pathdom.NewPath(id, "selector"))})
		if slotErr != nil {
			b.Fatal(slotErr)
		}
		inventory, sealErr := domain.SealCoordinateFactorInventory(keys, slots)
		if sealErr != nil {
			b.Fatal(sealErr)
		}
		if _, internErr := catalog.intern(1, inventory); internErr != nil {
			b.Fatal(internErr)
		}
		if id == 64 {
			duplicate = inventory
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, internErr := catalog.intern(1, duplicate); internErr != nil {
			b.Fatal(internErr)
		}
	}
}

// BenchmarkFormalApplyCoordinateSelectorCatalogLinearDuplicate models the
// pre-interning duplicate path: exact equality against every prior selector.
// Keep it beside the fast-path benchmark so a regression cannot quietly
// restore quadratic canonicalization during freeze.
func BenchmarkFormalApplyCoordinateSelectorCatalogLinearDuplicate(b *testing.B) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	program := &RelationProgram{bodies: []relationProgramBody{{variable: 1, productDomain: domain}}}
	forest := &formalFiberInventory{program: program}
	catalog, err := newFormalApplyCoordinateSelectorCatalog(forest, 1)
	if err != nil {
		b.Fatal(err)
	}
	var duplicate state.CoordinateFactorInventory
	for id := symbol.ID(1); id <= 64; id++ {
		slots, slotErr := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{keys.FromPath(pathdom.NewPath(id, "selector"))})
		if slotErr != nil {
			b.Fatal(slotErr)
		}
		inventory, sealErr := domain.SealCoordinateFactorInventory(keys, slots)
		if sealErr != nil {
			b.Fatal(sealErr)
		}
		if _, internErr := catalog.intern(1, inventory); internErr != nil {
			b.Fatal(internErr)
		}
		if id == 64 {
			duplicate = inventory
		}
	}
	entries := catalog.byBody[0]
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		found := false
		for _, prior := range entries {
			equal, equalErr := formalCoordinateInventoriesEqual(domain, prior, duplicate)
			if equalErr != nil {
				b.Fatal(equalErr)
			}
			if equal {
				found = true
				break
			}
		}
		if !found {
			b.Fatal("duplicate selector was not found")
		}
	}
}
