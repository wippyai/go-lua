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
// its target refinement (including its transaction-produced canonical alias)
// or equality publication.
type PathReplacementCoordinateCapability struct {
	seal   *productDomainSeal
	keys   *keyspace.KeySpace
	family CoordinateFamily
	slots  []CoordinateSlot
}

// PathReplacementCoordinateWrite is one sealed PathStore emission.  A direct
// replacement, a literal-key index write, and an object-entry write all use
// the same PathEvidence publication law; their syntax differs, but none may
// hand-assemble the emitted coordinate set.
type PathReplacementCoordinateWrite struct {
	Target    keyspace.Key
	Source    keyspace.Key
	HasSource bool
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
// Every replacement writes its exact target refinement and, where the keyspace
// has one, its exact FieldCanonical alias; a non-identity source additionally
// publishes its exact source-to-target equality proof.
func (d ProductDomain) SealPathReplacementCoordinateCapability(
	keys *keyspace.KeySpace,
	target, source keyspace.Key,
	hasSource bool,
) (PathReplacementCoordinateCapability, error) {
	return d.SealPathReplacementCoordinateCapabilityForWrites(keys, []PathReplacementCoordinateWrite{{
		Target: target, Source: source, HasSource: hasSource,
	}})
}

// SealPathReplacementCoordinateCapabilityForWrites seals the complete,
// ordered PathStore publication set. It admits only declared writes: direct
// replacement, literal/dynamic-index evidence, and object-entry members. A
// source equality is present only for an individual write that emits it.
func (d ProductDomain) SealPathReplacementCoordinateCapabilityForWrites(
	keys *keyspace.KeySpace,
	writes []PathReplacementCoordinateWrite,
) (PathReplacementCoordinateCapability, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || len(writes) == 0 {
		return PathReplacementCoordinateCapability{}, fmt.Errorf("%w: path replacement coordinate capability is unowned", ErrInvalidLaneFactor)
	}
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || family.Lane().ID() != LanePathEvidence {
		return PathReplacementCoordinateCapability{}, fmt.Errorf("%w: path replacement path-evidence factor is not registered", ErrInvalidLaneFactor)
	}
	capability := PathReplacementCoordinateCapability{seal: d.seal, keys: keys, family: family}
	appendSlot := func(slot CoordinateSlot) error {
		for _, existing := range capability.slots {
			equal, err := d.CoordinateSlotEqual(existing, slot)
			if err != nil {
				return err
			}
			if equal {
				return nil
			}
		}
		capability.slots = append(capability.slots, slot)
		return nil
	}
	appendTargetRefinements := func(target keyspace.Key) error {
		targets := []keyspace.Key{target}
		if canonical, ok := keys.FieldCanonical(target); ok && canonical != target {
			targets = append(targets, canonical)
		}
		for _, target := range targets {
			targetSlot, err := d.PathRefinementCoordinateSlot(keys, target)
			if err != nil {
				return err
			}
			if err := appendSlot(targetSlot); err != nil {
				return err
			}
		}
		return nil
	}
	for _, write := range writes {
		if write.Target.Kind == keyspace.KindInvalid || write.HasSource != (write.Source.Kind != keyspace.KindInvalid) {
			return PathReplacementCoordinateCapability{}, fmt.Errorf("%w: path replacement coordinate write is malformed", ErrInvalidLaneFactor)
		}
		if err := appendTargetRefinements(write.Target); err != nil {
			return PathReplacementCoordinateCapability{}, err
		}
		if !write.HasSource || write.Source == write.Target {
			continue
		}
		equality, err := d.PathBranchProofCoordinateSlot(keys, pathevidence.BranchProof{
			Kind: pathevidence.BranchProofPathEqual, Path: write.Target, Other: write.Source,
		})
		if err != nil {
			return PathReplacementCoordinateCapability{}, err
		}
		if err := appendSlot(equality); err != nil {
			return PathReplacementCoordinateCapability{}, err
		}
	}
	return capability, nil
}
