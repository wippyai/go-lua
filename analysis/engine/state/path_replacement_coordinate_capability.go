package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// PathReplacementCoordinateCapability is the registered declaration of the
// coupled path-evidence coordinates emitted by one PathReplacement
// transaction. It is sealed from the same target/source syntax used by the
// transaction, so a frozen Effect footprint cannot hand-spell, widen, or omit
// its target refinement or equality publication.
type PathReplacementCoordinateCapability struct {
	seal   *productDomainSeal
	keys   *keyspace.KeySpace
	family CoordinateFamily
	slots  []CoordinateSlot
}

func (c PathReplacementCoordinateCapability) ValidFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	if !d.Valid() || c.seal != d.seal || c.keys != keys || keys == nil || !keys.Valid() {
		return false
	}
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || c.family != family {
		return false
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return false
	}
	for _, slot := range c.slots {
		if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
			return false
		}
	}
	return true
}

// EmittedSlots returns precisely the coordinate slots that the corresponding
// PathReplacement transaction may publish. The returned slice is detached so
// callers cannot alter the sealed declaration.
func (c PathReplacementCoordinateCapability) EmittedSlots() []CoordinateSlot {
	return append([]CoordinateSlot(nil), c.slots...)
}

// SealPathReplacementCoordinateCapability derives PathReplacement's coupled
// path-evidence publication directly from the registered path-evidence family.
// Every replacement writes its exact target refinement; a non-identity source
// additionally publishes its exact source-to-target equality proof.
func (d ProductDomain) SealPathReplacementCoordinateCapability(
	keys *keyspace.KeySpace,
	target, source keyspace.Key,
	hasSource bool,
) (PathReplacementCoordinateCapability, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || target.Kind == keyspace.KindInvalid ||
		hasSource != (source.Kind != keyspace.KindInvalid) {
		return PathReplacementCoordinateCapability{}, fmt.Errorf("%w: path replacement coordinate capability is unowned", ErrInvalidLaneFactor)
	}
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || family.Lane().ID() != LanePathEvidence {
		return PathReplacementCoordinateCapability{}, fmt.Errorf("%w: path replacement path-evidence factor is not registered", ErrInvalidLaneFactor)
	}
	capability := PathReplacementCoordinateCapability{seal: d.seal, keys: keys, family: family}
	targetSlot, err := d.PathRefinementCoordinateSlot(keys, target)
	if err != nil {
		return PathReplacementCoordinateCapability{}, err
	}
	capability.slots = append(capability.slots, targetSlot)
	if !hasSource || source == target {
		return capability, nil
	}
	equality, err := d.PathBranchProofCoordinateSlot(keys, pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathEqual, Path: target, Other: source,
	})
	if err != nil {
		return PathReplacementCoordinateCapability{}, err
	}
	capability.slots = append(capability.slots, equality)
	return capability, nil
}
