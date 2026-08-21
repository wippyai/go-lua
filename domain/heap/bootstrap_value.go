package heap

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// BootRootID returns the stable identity of the sealed bootstrap image owned
// by this Heap schema. It preserves the pre-cut bootstrap image identity while
// moving its storage out of bootstrap's duplicate catalog.
func (schema Schema) BootRootID(key Key) (identity.ContentID, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootBoot {
		return identity.ContentID{}, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok || row.kind != RootBoot || !row.bootID.Available() || !row.bootContentID.Available() {
		return identity.ContentID{}, false
	}
	return row.bootContentID, true
}

// BootValue returns the complete immutable bootstrap image sealed for key.
// The image is a canonical Heap row, not a transient rule operand or a
// per-solve reconstruction.
func (schema Schema) BootValue(key Key) (Value, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootBoot {
		return Value{}, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok || row.kind != RootBoot || !row.bootValue.valid() || row.bootValue.owner != schema.owner {
		return Value{}, false
	}
	return row.bootValue, true
}

// sealBootRows publishes each bootstrap image once, after all Heap source
// rows and rank bounds are sealed. It deliberately uses the existing typed
// BootEntry projections; no Host/Target traversal remains on the hot path.
func (owner *schema) sealBootRows() bool {
	if owner == nil || !owner.id.Available() {
		return false
	}
	if len(owner.roots) == 0 {
		return true
	}
	schema := Schema{owner: owner}
	for index := range owner.roots {
		row := &owner.roots[index]
		if row.kind != RootBoot || !row.bootID.Available() {
			continue
		}
		keySlot := owner.bootIndex[row.bootID]
		key := Key{owner: owner, slot: keySlot}
		value, valueOK := buildBootValue(schema, key)
		contentID := bootRootContentID(schema.ContentID(), row.bootID)
		if !valueOK || !contentID.Available() {
			return false
		}
		row.bootValue = value
		row.bootContentID = contentID
	}
	return true
}

func bootRootContentID(schemaID, bootID identity.ContentID) identity.ContentID {
	if !schemaID.Available() || !bootID.Available() {
		return identity.ContentID{}
	}
	var image [32 + 32 + 16]byte
	copy(image[:32], schemaID[:])
	copy(image[32:64], bootID[:])
	binary.BigEndian.PutUint64(image[64:72], 0x686561702d626f6f) // "heap-boo"
	binary.BigEndian.PutUint64(image[72:80], 3)
	return sha256.Sum256(image[:])
}

func buildBootValue(schema Schema, key Key) (Value, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootBoot {
		return Value{}, false
	}
	none, noneOK := schema.ContainmentNone()
	frozen, frozenOK := schema.BootFrozen(key)
	initializer, initializerOK := schema.BeginObject(ShapeEligible, frozen, none)
	if !initializerOK || !noneOK || !frozenOK {
		return Value{}, false
	}
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		if !entryOK {
			return Value{}, false
		}
		if !keyOK || entryKey != key {
			continue
		}
		slot, slotOK := entry.Slot()
		selector, selectorOK := schema.SelectorForSlot(slot)
		raw, payload, projectionOK := entry.Projection()
		if !slotOK || !selectorOK || !projectionOK {
			return Value{}, false
		}
		var state CellState
		var stateOK bool
		switch raw {
		case RawAbsent:
			state, stateOK = schema.CellAbsent()
		case RawPresent:
			containment, containmentOK := entry.ValueContainment()
			if !containmentOK {
				return Value{}, false
			}
			state, stateOK = schema.CellPresent(slot, payload, containment, none)
		default:
			return Value{}, false
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	world, worldOK := schema.Exact(key, object)
	if !objectOK || !worldOK {
		return Value{}, false
	}
	return schema.Relation(key, world)
}
