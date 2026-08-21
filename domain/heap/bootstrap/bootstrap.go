// Package bootstrap declares Heap's Target/Link bootstrap raw-presence seed.
// It owns neither a second heap image nor an initial-state composition root.
package bootstrap

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// Root is one sealed complete bootstrap image for one actor-local BootRoot.
// Entries are deliberately aggregated before execution: a table's entries
// coexist within its one WorldExact object and must never be emitted as
// sibling whole-world alternatives for Factor Join to reconcile.
type Root struct {
	schema   heapdomain.Schema
	key      heapdomain.Key
	kind     heapdomain.RootKind
	entries  []heapdomain.BootEntry
	id       identity.ContentID
	value    heapdomain.Value
	admitted bool
}

// NewRoot aggregates every canonical BootEntry for key in a deterministic
// semantic order. It is the only bootstrap operand constructor.
func NewRoot(schema heapdomain.Schema, key heapdomain.Key) (Root, bool) {
	if !schema.ContentID().Available() || key.Kind() != heapdomain.RootBoot {
		return Root{}, false
	}
	bootID, bootIDOK := key.BootID()
	if !bootIDOK {
		return Root{}, false
	}
	entries := make([]heapdomain.BootEntry, 0)
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		if !entryOK || !keyOK {
			return Root{}, false
		}
		if entryKey == key {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		leftID, _ := entries[left].ID()
		rightID, _ := entries[right].ID()
		return compareID(leftID, rightID) < 0
	})
	for index, entry := range entries {
		id, ok := entry.ID()
		if !ok {
			return Root{}, false
		}
		if index != 0 {
			previous, previousOK := entries[index-1].ID()
			if !previousOK || compareID(previous, id) >= 0 {
				return Root{}, false
			}
		}
	}
	id := rootID(schema.ContentID(), bootID)
	if !id.Available() {
		return Root{}, false
	}
	value, valueOK := buildRootValue(schema, key, entries)
	if !valueOK {
		return Root{}, false
	}
	return Root{
		schema: schema, key: key, kind: heapdomain.RootBoot,
		entries: entries, id: id, value: value, admitted: true,
	}, true
}

func (root Root) ID() (identity.ContentID, bool) {
	if !root.id.Available() {
		return identity.ContentID{}, false
	}
	return root.id, true
}

func (root Root) fencedTo(schema heapdomain.Schema) bool {
	return schema.Valid() && root.schema == schema && root.kind == heapdomain.RootBoot && root.id.Available()
}

func (root Root) entryCount() int {
	return len(root.entries)
}

func (root Root) keyValue() (heapdomain.Key, bool) {
	if !root.id.Available() || root.kind != heapdomain.RootBoot {
		return heapdomain.Key{}, false
	}
	return root.key, true
}

func compareID(left, right identity.ContentID) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func rootID(schemaID, bootID identity.ContentID) identity.ContentID {
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

func contentForSchema(schema heapdomain.Schema, root Root) (Root, [32]byte, bool) {
	id, ok := root.ID()
	if !ok || !root.fencedTo(schema) {
		return Root{}, [32]byte{}, false
	}
	return root, [32]byte(id), true
}

func resultForSchema(schema heapdomain.Schema, root Root) (heapdomain.Key, heapdomain.Value, bool) {
	if !root.fencedTo(schema) || !root.admitted {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	return root.key, root.value, true
}

func buildRootValue(schema heapdomain.Schema, key heapdomain.Key, entries []heapdomain.BootEntry) (heapdomain.Value, bool) {
	none, noneOK := schema.ContainmentNone()
	frozen, frozenOK := schema.BootFrozen(key)
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, frozen, none)
	if !initializerOK || !noneOK || !frozenOK {
		return heapdomain.Value{}, false
	}
	for _, entry := range entries {
		slot, slotOK := entry.Slot()
		raw, payload, projectionOK := entry.Projection()
		selector, selectorOK := schema.SelectorForSlot(slot)
		if !slotOK || !projectionOK || !selectorOK {
			return heapdomain.Value{}, false
		}
		var state heapdomain.CellState
		var stateOK bool
		switch raw {
		case heapdomain.RawAbsent:
			state, stateOK = schema.CellAbsent()
		case heapdomain.RawPresent:
			valueChild, childOK := entry.ValueContainment()
			if !childOK {
				return heapdomain.Value{}, false
			}
			state, stateOK = schema.CellPresent(slot, payload, valueChild, none)
		default:
			return heapdomain.Value{}, false
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return heapdomain.Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	world, worldOK := schema.Exact(key, object)
	value, relationOK := schema.Relation(key, world)
	if !objectOK || !worldOK || !relationOK {
		return heapdomain.Value{}, false
	}
	return value, schema.Admits(key, value)
}
