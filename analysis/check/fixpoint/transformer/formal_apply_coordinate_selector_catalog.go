package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalApplyCoordinateSelectorOrdinal is a stable, body-local ordinal in a
// frozen catalog. Zero is invalid so an unbound footprint cannot accidentally
// address the first selector.
type formalApplyCoordinateSelectorOrdinal uint32

// formalApplyCoordinateSelectorRef is exact selector identity. The catalog
// pointer and target body make same-numbered ordinals from different frozen
// programs structurally distinct.
type formalApplyCoordinateSelectorRef struct {
	catalog *formalApplyCoordinateSelectorCatalog
	target  relationVar
	ordinal formalApplyCoordinateSelectorOrdinal
}

func (r formalApplyCoordinateSelectorRef) valid() bool {
	return r.catalog != nil && r.target != 0 && r.ordinal != 0 &&
		int(r.target) <= len(r.catalog.byBody) && int(r.ordinal) <= len(r.catalog.byBody[r.target-1])
}

// formalApplyCoordinateSelectorCatalog owns every distinct Apply source cone
// exactly once per target body. Entries are ordered by the canonical formal
// region cell order. Equality is the ProductDomain's exact coordinate-slot
// equality; hashes are deliberately absent from both interning and identity.
type formalApplyCoordinateSelectorCatalog struct {
	forest      *formalFiberInventory
	byBody      [][]state.CoordinateFactorInventory
	refs        [][]formalApplyCoordinateSelectorRef
	byInventory []map[state.CoordinateFactorInventory]formalApplyCoordinateSelectorRef
	byCell      map[formalRelationCell]formalApplyCoordinateSelectorRef
}

func newFormalApplyCoordinateSelectorCatalog(forest *formalFiberInventory, bodyCount int) (*formalApplyCoordinateSelectorCatalog, error) {
	if forest == nil || forest.program == nil || bodyCount <= 0 || bodyCount != len(forest.program.bodies) {
		return nil, fmt.Errorf("transformer: Apply coordinate selector catalog is unowned")
	}
	return &formalApplyCoordinateSelectorCatalog{
		forest:      forest,
		byBody:      make([][]state.CoordinateFactorInventory, bodyCount),
		refs:        make([][]formalApplyCoordinateSelectorRef, bodyCount),
		byInventory: make([]map[state.CoordinateFactorInventory]formalApplyCoordinateSelectorRef, bodyCount),
		byCell:      make(map[formalRelationCell]formalApplyCoordinateSelectorRef),
	}, nil
}

func (c *formalApplyCoordinateSelectorCatalog) intern(target relationVar, inventory state.CoordinateFactorInventory) (formalApplyCoordinateSelectorRef, error) {
	if c == nil || c.forest == nil || c.forest.program == nil || target == 0 || int(target) > len(c.byBody) {
		return formalApplyCoordinateSelectorRef{}, fmt.Errorf("transformer: Apply coordinate selector has foreign catalog ownership")
	}
	body := &c.forest.program.bodies[target-1]
	spanKeys := inventory.KeySpace()
	if spanKeys == nil || !inventory.ValidFor(body.productDomain, spanKeys) {
		return formalApplyCoordinateSelectorRef{}, fmt.Errorf("transformer: Apply coordinate selector is malformed")
	}
	// CoordinateFactorInventory is an immutable, comparable authority. Most
	// selector cones arrive here by structural sharing from coordinate closure,
	// so preserve that identity as a zero-scan fast path. Inventories sealed
	// independently remain lawful: they still use the exact domain equality
	// fallback below before receiving the same ordinal.
	identities := c.byInventory[target-1]
	if ref, found := identities[inventory]; found {
		return ref, nil
	}
	entries := c.byBody[target-1]
	for index, prior := range entries {
		equal, err := formalCoordinateInventoriesEqual(body.productDomain, prior, inventory)
		if err != nil {
			return formalApplyCoordinateSelectorRef{}, err
		}
		if equal {
			ref := formalApplyCoordinateSelectorRef{catalog: c, target: target, ordinal: formalApplyCoordinateSelectorOrdinal(index + 1)}
			if identities == nil {
				identities = make(map[state.CoordinateFactorInventory]formalApplyCoordinateSelectorRef)
				c.byInventory[target-1] = identities
			}
			identities[inventory] = ref
			return ref, nil
		}
	}
	ref := formalApplyCoordinateSelectorRef{catalog: c, target: target, ordinal: formalApplyCoordinateSelectorOrdinal(len(entries) + 1)}
	c.byBody[target-1] = append(entries, inventory)
	c.refs[target-1] = append(c.refs[target-1], ref)
	if identities == nil {
		identities = make(map[state.CoordinateFactorInventory]formalApplyCoordinateSelectorRef)
		c.byInventory[target-1] = identities
	}
	identities[inventory] = ref
	return ref, nil
}

func (c *formalApplyCoordinateSelectorCatalog) inventory(ref formalApplyCoordinateSelectorRef) (state.CoordinateFactorInventory, error) {
	if c == nil || ref.catalog != c || !ref.valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("transformer: Apply coordinate selector reference is unowned")
	}
	return c.byBody[ref.target-1][ref.ordinal-1], nil
}

func (c *formalApplyCoordinateSelectorCatalog) references(target relationVar) []formalApplyCoordinateSelectorRef {
	if c == nil || target == 0 || int(target) > len(c.refs) {
		return nil
	}
	// The catalog is immutable after coordinate closure publication. Returning
	// its retained reference slice makes producer prefreeze a direct, zero-allocation
	// lookup rather than rebuilding ordinals for every product spelling.
	return c.refs[target-1]
}

func freezeFormalApplyCoordinateSelectorCatalog(c *formalCoordinateDependencyClosure) (*formalApplyCoordinateSelectorCatalog, error) {
	if c == nil || c.forest == nil || c.program == nil || len(c.cellValue) != len(c.cells) || len(c.cellFrames) != len(c.cells) {
		return nil, fmt.Errorf("transformer: Apply coordinate selector closure is malformed")
	}
	catalog, err := newFormalApplyCoordinateSelectorCatalog(c.forest, len(c.program.bodies))
	if err != nil {
		return nil, err
	}
	for index, cell := range c.cells {
		value := c.cellValue[index]
		if value.source.KeySpace() == nil {
			continue
		}
		if len(c.cellFrames[index]) == 0 {
			return nil, fmt.Errorf("transformer: Apply source selector has no target frame")
		}
		targetIndex := c.frames[c.cellFrames[index][0]].target
		for _, frameIndex := range c.cellFrames[index][1:] {
			if c.frames[frameIndex].target != targetIndex {
				return nil, fmt.Errorf("transformer: one Apply selector names multiple target bodies")
			}
		}
		ref, internErr := catalog.intern(relationVar(targetIndex+1), value.source)
		if internErr != nil {
			return nil, internErr
		}
		catalog.byCell[cell] = ref
	}
	return catalog, nil
}
