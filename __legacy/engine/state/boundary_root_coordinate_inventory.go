package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// BoundaryRootCoordinateSlots returns the exact registered coordinates minted
// by root publication at the supplied destination paths.  It is the static
// topology counterpart of BoundaryFactorTransportPlan.PrepareFactor's root
// application: formal tuple inventories call it before descriptor sealing so
// Apply can never grow a coordinate fiber at execution time.
func (d ProductDomain) BoundaryRootCoordinateSlots(keys *keyspace.KeySpace, targets []keyspace.Key) ([]CoordinateSlot, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return nil, fmt.Errorf("%w: boundary root coordinate inventory is unowned", ErrInvalidLaneFactor)
	}
	closure := emptyBoundaryClosure()
	for _, target := range targets {
		if target.Kind == keyspace.KindInvalid || keys.FormatReadOnly(target) == "" {
			return nil, fmt.Errorf("%w: boundary root coordinate target is foreign", ErrInvalidLaneFactor)
		}
		closure.paths[target] = struct{}{}
	}
	ctx := &boundaryApplyContext{reg: d.reg, keys: keys, closure: closure}
	var out []CoordinateSlot
	for laneIndex := range d.factorLanes {
		runtime := &d.factorLanes[laneIndex]
		for familyIndex := range runtime.coordinates {
			coordinate := &runtime.coordinates[familyIndex]
			for _, target := range targets {
				key, claimed, ok := coordinate.boundary.rootSlot(ctx, BoundaryFactorTarget{Path: target})
				if !ok {
					return nil, fmt.Errorf("%w: family %q rejected boundary root inventory", ErrInvalidLaneFactor, coordinate.family.id)
				}
				if claimed {
					out = append(out, CoordinateSlot{family: coordinate.family, keys: keys, key: key})
				}
			}
		}
	}
	inventory, err := d.SealCoordinateFactorInventory(keys, out)
	if err != nil {
		return nil, err
	}
	return inventory.Slots(), nil
}
