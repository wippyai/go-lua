// Package source owns the structural classification of one existing Program
// allocation root. It deliberately produces no Heap fact: construction Rules
// consume this fenced source description and own every semantic transition.
package source

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Form is the complete source constructor disposition. Closed means that all
// fields are scalar Values; FinalOpen is reserved for the Pack-owned path.
type Form uint8

const (
	FormInvalid Form = iota
	FormEmpty
	FormClosed
	FormFinalOpen
)

// KeyKind is Heap allocation-field geometry. It is deliberately owned beside
// Field: neither Link nor a runtime-type projection classifies Heap geometry.
type KeyKind uint8

const (
	KeyInvalid KeyKind = iota
	KeyExact
	KeyDynamic
)

// Root is one schema-fenced, existing Program allocation root. Its fields are
// private so no consumer can pair a source root with a key from a different
// Heap schema.
type Root struct {
	schema heap.Schema
	key    heap.Key
	id     keyspace.ContentID
	kind   heap.AllocationKind
	form   Form
}

// New classifies exactly one source Program root. Fresh Target roots have no
// Program origin and are rejected: their guarded creation has a separate
// semantic owner.
func New(schema heap.Schema, key heap.Key) (Root, bool) {
	linked := schema.Link()
	if linked == nil {
		return Root{}, false
	}
	_, _, kind, originOK := key.ProgramAllocation()
	id, idOK := key.ContentID()
	if !originOK || !schema.OwnsKey(key) || !idOK || !id.Available() {
		return Root{}, false
	}
	form, formOK := classify(schema, key, kind)
	if !formOK {
		return Root{}, false
	}
	return Root{schema: schema, key: key, id: id, kind: kind, form: form}, true
}

func classify(schema heap.Schema, root heap.Key, kind heap.AllocationKind) (Form, bool) {
	switch kind {
	case heap.AllocationClosure:
		return FormEmpty, true
	case heap.AllocationTable:
		rootShard, rootTerm, _, rootOK := root.ProgramAllocation()
		rootProgram, rootProgramOK := schema.Link().Project().Mounts().Program(rootShard)
		if !rootOK || !rootProgramOK || rootProgram == nil || !rootProgram.Flow().AccessGeometry().Available() {
			return FormInvalid, false
		}
		count := schema.FieldCount(root)
		if count == 0 {
			return FormEmpty, true
		}
		finalOpen := false
		for index := 0; index < count; index++ {
			field, fieldOK := schema.FieldAt(root, index)
			fieldRoot, shard, fieldTerm, fieldOrigin := schema.FieldOrigin(field)
			p, programOK := schema.Link().Project().Mounts().Program(shard)
			if !fieldOK || !fieldOrigin || fieldRoot != root || !programOK || p == nil {
				return FormInvalid, false
			}
			flowView := p.Flow()
			if !flowView.AccessGeometry().Available() {
				return FormInvalid, false
			}
			table, _, values, _, fieldOK := flowView.Authored().Fields().Get(fieldTerm)
			resolvedValues, open, valuesOK := flowView.Authored().Fields().Values(fieldTerm)
			_, tableOwnerOK := flowView.Authored().Tables().Get(table)
			if !fieldOK || table != rootTerm || !tableOwnerOK || resolvedValues != values || !valuesOK {
				return FormInvalid, false
			}
			if open {
				finalOpen = true
				continue
			}
			width, widthOK := p.Flow().Authored().Values().Len(values)
			if !widthOK || width != 1 {
				return FormInvalid, false
			}
		}
		if finalOpen {
			return FormFinalOpen, true
		}
		return FormClosed, true
	default:
		return FormInvalid, false
	}
}

func (root Root) ID() (keyspace.ContentID, bool) { return root.id, root.id.Available() }
func (root Root) Key() heap.Key                  { return root.key }
func (root Root) Kind() heap.AllocationKind      { return root.kind }
func (root Root) Form() Form                     { return root.form }

// Revalidate refuses stale, forged, and cross-schema copies before a Rule
// admits an operand into its semantic or evidence path.
func (root Root) Revalidate(schema heap.Schema) bool {
	canonical, ok := New(schema, root.key)
	return root.FencedTo(schema) && ok && canonical == root
}

// FencedTo is Root's hot owner/capability fence. A Root that has already
// passed Revalidate at the content, instance, or evidence boundary carries
// its exact Heap schema issuer; recurrent transfer must not reconstruct Link
// allocation topology for every Product row.
func (root Root) FencedTo(schema heap.Schema) bool {
	_, rootIDOK := root.ID()
	return root.schema == schema && schema.Link() != nil && rootIDOK && root.Key().Kind() == heap.RootAllocation && root.Form() != FormInvalid
}

// Closed is the complete schema-fenced structural view of one scalar table
// constructor. It is the only field source accepted by the closed Rule.
type Closed struct {
	root   Root
	heap   heap.Schema
	values *valuedomain.Schema
	coords []valuedomain.Coordinate
	fields []Field
}

// NewClosed admits only a scalar closed source table. No caller can use this
// descriptor for an empty/closure root or a final-open table.
func NewClosed(schema heap.Schema, valueSchema *valuedomain.Schema, allocation heap.Key) (Closed, bool) {
	root, rootOK := New(schema, allocation)
	if !rootOK || root.form != FormClosed || valueSchema == nil || valueSchema.Link() == nil || schema.Link() == nil ||
		valueSchema.Link() != schema.Link() || !valueSchema.OwnsHeapSchema(schema) {
		return Closed{}, false
	}
	fields, coords, fieldsOK := closedFields(schema, valueSchema, root)
	if !fieldsOK || len(coords) == 0 {
		return Closed{}, false
	}
	return Closed{root: root, heap: schema, values: valueSchema, coords: coords, fields: fields}, true
}

func (closed Closed) ID() (keyspace.ContentID, bool) { return closed.root.ID() }
func (closed Closed) Key() heap.Key                  { return closed.root.Key() }

// Count is source-order field count. It is defined only after complete
// descriptor revalidation, never as a loose Link query.
func (closed Closed) Count() int {
	if !closed.valid() {
		return 0
	}
	return len(closed.fields)
}

// Revalidate reconstructs this complete descriptor from the owner-issued
// Heap and Value schemas before any Rule uses it.
func (closed Closed) Revalidate() bool {
	canonical, ok := NewClosed(closed.heap, closed.values, closed.root.key)
	return ok && closed.root == canonical.root && closed.heap == canonical.heap && closed.values == canonical.values && equalCoordinates(closed.coords, canonical.coords) && equalFields(closed.fields, canonical.fields)
}

// RevalidateFor additionally fences this descriptor to the exact Heap and
// Value schema instances selected by its owner.  Equal schemas independently
// sealed from the same Link are intentionally not interchangeable here:
// coordinates are local owner-issued handles.
func (closed Closed) RevalidateFor(heapSchema heap.Schema, values *valuedomain.Schema) bool {
	return closed.FencedTo(heapSchema, values) && closed.Revalidate()
}

// FencedTo is the hot-path owner/capability fence for a descriptor that was
// already admitted through RevalidateFor.  It deliberately does not rebuild
// Link field topology: transfer and evidence replay may inspect the immutable
// descriptor many times, while complete topology reconstruction remains a
// cold admission-boundary check.
func (closed Closed) FencedTo(heapSchema heap.Schema, values *valuedomain.Schema) bool {
	return closed.heap == heapSchema && closed.values == values && closed.valid()
}

// CoordinateCount and CoordinateAt issue the unique, global-index-sorted
// local input vector for this constructor. Field use ordinals are indexes into
// this vector, never global Value-factor indexes.
func (closed Closed) CoordinateCount() int {
	if !closed.valid() {
		return 0
	}
	return len(closed.coords)
}

func (closed Closed) CoordinateAt(index int) (valuedomain.Coordinate, bool) {
	if !closed.valid() || index < 0 || index >= len(closed.coords) {
		return valuedomain.Coordinate{}, false
	}
	return closed.coords[index], true
}

// Field is one source-ordered scalar constructor input. It carries only cold
// topology projections; applying values to Heap remains the closed Rule's
// semantic work.
type Field struct {
	ordinal  uint32
	slot     heap.Slot
	payload  heap.Payload
	selector heap.KeySelector
	value    valuedomain.Coordinate
	valueOrd uint32
	key      valuedomain.Coordinate
	keyOrd   uint32
	keyKind  KeyKind
	exact    linkproject.Key
	id       keyspace.ContentID
}

// At returns one canonical scalar field. Dynamic source keys retain a Value
// coordinate for equality consumers; a Flow-authored direct same-Cell Read
// pair uses that one key-read coordinate for both roles. Heap receives only
// its typed selector, while static source keys retain their exact Project key
// identity.
func (closed Closed) At(index int) (Field, bool) {
	if index < 0 || !closed.valid() || index >= len(closed.fields) {
		return Field{}, false
	}
	return closed.fields[index], true
}

func (closed Closed) valid() bool {
	_, rootIDOK := closed.root.ID()
	return closed.heap.Link() != nil && closed.heap.ContentID().Available() && closed.values != nil && closed.values.Link() != nil && closed.values.Link() == closed.heap.Link() && closed.values.OwnsHeapSchema(closed.heap) && rootIDOK && closed.root.Key().Kind() == heap.RootAllocation && closed.root.Form() == FormClosed && len(closed.coords) != 0 && len(closed.fields) != 0
}

func closedFields(schema heap.Schema, valueSchema *valuedomain.Schema, root Root) ([]Field, []valuedomain.Coordinate, bool) {
	linked := schema.Link()
	if linked == nil {
		return nil, nil, false
	}
	if linked.Boundary() == nil {
		return nil, nil, false
	}
	boundaryValues := linked.Boundary().Values()
	_, rootTerm, _, rootOK := root.key.ProgramAllocation()
	if !rootOK {
		return nil, nil, false
	}
	fieldCount := schema.FieldCount(root.key)
	coordinates := make([]valuedomain.Coordinate, 0, fieldCount*2)
	appendCoordinate := func(coordinate valuedomain.Coordinate) bool {
		if _, ok := valueSchema.CoordinateIndex(coordinate); !ok {
			return false
		}
		coordinates = append(coordinates, coordinate)
		return true
	}
	for index := 0; index < fieldCount; index++ {
		field, fieldOK := schema.FieldAt(root.key, index)
		fieldRoot, shard, fieldTerm, originOK := schema.FieldOrigin(field)
		mounts := linked.Project().Mounts()
		if _, shardOK := mounts.Index(shard); !shardOK {
			return nil, nil, false
		}
		projectShard := shard
		programValue, programOK := mounts.Program(projectShard)
		if !fieldOK || !originOK || fieldRoot != root.key || !programOK || programValue == nil {
			return nil, nil, false
		}
		flowView := programValue.Flow()
		if !flowView.AccessGeometry().Available() {
			return nil, nil, false
		}
		table, sourceTerm, values, fieldKind, ok := flowView.Authored().Fields().Get(fieldTerm)
		authoredValues, finalOpen, valuesOK := flowView.Authored().Fields().Values(fieldTerm)
		tableOwner, tableOwnerOK := flowView.Authored().Tables().Get(table)
		width, widthOK := flowView.Authored().Values().Len(values)
		member, memberOK := flowView.Authored().Values().Member(values, 0)
		valueRef, valueRefOK := boundaryValues.Of(projectShard, member)
		normalized, geometryOK := flowView.AccessGeometry().TableFields().Get(fieldTerm)
		if !ok || table != rootTerm || !tableOwnerOK || authoredValues != values || !valuesOK || finalOpen || !widthOK || width != 1 || !memberOK || !valueRefOK || !geometryOK {
			return nil, nil, false
		}
		switch fieldKind {
		case flowkind.FieldKey:
			if normalized != 0 {
				return nil, nil, false
			}
			if sameDirectCellRead(flowView.Authored(), tableOwner, sourceTerm, member) {
				coordinate, coordinateOK := valueSchema.CoordinateFor(valueRef)
				if !coordinateOK || !appendCoordinate(coordinate) {
					return nil, nil, false
				}
				continue
			}
			value, valueOK := valueSchema.CoordinateFor(valueRef)
			if !valueOK || !appendCoordinate(value) {
				return nil, nil, false
			}
			dynamicRef, dynamicOK := boundaryValues.Of(projectShard, sourceTerm)
			coordinate, coordinateOK := valueSchema.CoordinateFor(dynamicRef)
			if !dynamicOK || !coordinateOK || !appendCoordinate(coordinate) {
				return nil, nil, false
			}
		case flowkind.FieldList, flowkind.FieldName, flowkind.FieldExact:
			// FieldExact nil/NaN is represented by a zero normalized key. It is
			// non-storable and must not fall through to the dynamic branch.
			if normalized == 0 {
				return nil, nil, false
			}
			value, valueOK := valueSchema.CoordinateFor(valueRef)
			if !valueOK || !appendCoordinate(value) {
				return nil, nil, false
			}
		default:
			return nil, nil, false
		}
	}
	sort.Slice(coordinates, func(left, right int) bool {
		leftIndex, leftOK := valueSchema.CoordinateIndex(coordinates[left])
		rightIndex, rightOK := valueSchema.CoordinateIndex(coordinates[right])
		return leftOK && rightOK && leftIndex < rightIndex
	})
	unique := coordinates[:0]
	for _, coordinate := range coordinates {
		if len(unique) == 0 || unique[len(unique)-1] != coordinate {
			unique = append(unique, coordinate)
		}
	}
	if len(unique) == 0 {
		return nil, nil, false
	}
	fields := make([]Field, 0, fieldCount)
	for index := 0; index < fieldCount; index++ {
		field, fieldOK := schema.FieldAt(root.key, index)
		slot, slotOK := schema.SlotForField(field)
		payload, payloadOK := schema.PayloadForField(field)
		fieldRoot, shard, fieldTerm, originOK := schema.FieldOrigin(field)
		mounts := linked.Project().Mounts()
		_, shardOK := mounts.Index(shard)
		projectShard := shard
		programValue, programOK := mounts.Program(shard)
		if !fieldOK || !slotOK || !payloadOK || !originOK || fieldRoot != root.key || !shardOK || !programOK || programValue == nil {
			return nil, nil, false
		}
		flowView := programValue.Flow()
		if !flowView.AccessGeometry().Available() {
			return nil, nil, false
		}
		table, sourceTerm, values, fieldKind, ok := flowView.Authored().Fields().Get(fieldTerm)
		authoredValues, finalOpen, valuesOK := flowView.Authored().Fields().Values(fieldTerm)
		tableOwner, tableOwnerOK := flowView.Authored().Tables().Get(table)
		width, widthOK := flowView.Authored().Values().Len(values)
		member, memberOK := flowView.Authored().Values().Member(values, 0)
		valueRef, valueRefOK := boundaryValues.Of(projectShard, member)
		normalized, geometryOK := flowView.AccessGeometry().TableFields().Get(fieldTerm)
		if !ok || table != rootTerm || !tableOwnerOK || authoredValues != values || !valuesOK || finalOpen || !widthOK || width != 1 || !memberOK || !valueRefOK || !geometryOK {
			return nil, nil, false
		}
		kind := KeyExact
		var exact linkproject.Key
		switch fieldKind {
		case flowkind.FieldKey:
			if normalized != 0 {
				return nil, nil, false
			}
			kind = KeyDynamic
		case flowkind.FieldList, flowkind.FieldName, flowkind.FieldExact:
			// A zero exact key is the non-storable nil/NaN outcome, never a
			// dynamic selector.
			if normalized == 0 {
				return nil, nil, false
			}
			var exactOK bool
			exact, exactOK = linked.Project().Keys().ForProgram(projectShard, programValue, normalized)
			if !exactOK {
				return nil, nil, false
			}
		default:
			return nil, nil, false
		}
		value, valueOK := valueSchema.CoordinateFor(valueRef)
		valueOrd, valueOrdOK := coordinateOrdinal(unique, value)
		if !valueOK || !valueOrdOK {
			return nil, nil, false
		}
		var key valuedomain.Coordinate
		var keyOrd uint32
		var selector heap.KeySelector
		if kind == KeyDynamic {
			if sameDirectCellRead(flowView.Authored(), tableOwner, sourceTerm, member) {
				key, keyOrd = value, valueOrd
			} else {
				dynamicRef, dynamicOK := boundaryValues.Of(projectShard, sourceTerm)
				if !dynamicOK {
					return nil, nil, false
				}
				var keyOK bool
				key, keyOK = valueSchema.CoordinateFor(dynamicRef)
				var keyOrdOK bool
				keyOrd, keyOrdOK = coordinateOrdinal(unique, key)
				if !keyOK || !keyOrdOK {
					return nil, nil, false
				}
			}
		} else {
			var selectorOK bool
			selector, selectorOK = schema.SelectorForSlot(slot)
			if !selectorOK {
				return nil, nil, false
			}
		}
		id, idOK := fieldID(root.id, uint32(index+1))
		if !idOK {
			return nil, nil, false
		}
		fields = append(fields, Field{ordinal: uint32(index + 1), slot: slot, payload: payload, selector: selector, value: value, valueOrd: valueOrd, key: key, keyOrd: keyOrd, keyKind: kind, exact: exact, id: id})
	}
	return fields, unique, true
}

// sameDirectCellRead recognizes only two authored Read rows that both read
// the same lexical Cell under the table's owner. Terms are occurrence
// identities, so equal source expressions must not be compared directly.
func sameDirectCellRead(authored flow.Authored, owner, left, right keyspace.Term) bool {
	if keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
		keyspace.TermFamily(left) != keyspace.FamilyRead || keyspace.TermOrdinal(left) == 0 ||
		keyspace.TermFamily(right) != keyspace.FamilyRead || keyspace.TermOrdinal(right) == 0 {
		return false
	}
	reads := authored.Storage().Reads()
	leftOwner, leftSource, _, leftOK := reads.Get(left)
	rightOwner, rightSource, _, rightOK := reads.Get(right)
	if !leftOK || !rightOK || leftOwner != owner || rightOwner != owner ||
		keyspace.TermFamily(leftSource) != keyspace.FamilyCell || keyspace.TermOrdinal(leftSource) == 0 ||
		keyspace.TermFamily(rightSource) != keyspace.FamilyCell || keyspace.TermOrdinal(rightSource) == 0 {
		return false
	}
	return leftSource == rightSource
}

func equalCoordinates(left, right []valuedomain.Coordinate) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func coordinateOrdinal(coords []valuedomain.Coordinate, coordinate valuedomain.Coordinate) (uint32, bool) {
	for ordinal, current := range coords {
		if current == coordinate {
			return uint32(ordinal), true
		}
	}
	return 0, false
}

func equalFields(left, right []Field) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].id != right[index].id || left[index].ordinal != right[index].ordinal || left[index].slot != right[index].slot || left[index].payload != right[index].payload || left[index].value != right[index].value || left[index].valueOrd != right[index].valueOrd || left[index].key != right[index].key || left[index].keyOrd != right[index].keyOrd || left[index].keyKind != right[index].keyKind || left[index].exact != right[index].exact {
			return false
		}
		leftSelector, leftStatic := left[index].ExactSelector()
		rightSelector, rightStatic := right[index].ExactSelector()
		if leftStatic != rightStatic {
			return false
		}
		if leftStatic {
			leftKey, leftOK := leftSelector.ExactAt(0)
			rightKey, rightOK := rightSelector.ExactAt(0)
			if leftSelector.Kind() != heap.KeySelectorAtom || rightSelector.Kind() != heap.KeySelectorAtom || leftSelector.ExactCount() != 1 || rightSelector.ExactCount() != 1 || !leftOK || !rightOK || leftKey != left[index].exact || rightKey != right[index].exact || leftKey != rightKey {
				return false
			}
		}
	}
	return true
}

func fieldID(root keyspace.ContentID, ordinal uint32) (keyspace.ContentID, bool) {
	if !root.Available() || ordinal == 0 {
		return keyspace.ContentID{}, false
	}
	var image [48]byte
	copy(image[:32], root[:])
	binary.BigEndian.PutUint64(image[32:40], 0x686561702d666c64)
	binary.BigEndian.PutUint64(image[40:48], uint64(ordinal))
	return sha256.Sum256(image[:]), true
}

func (field Field) ID() (keyspace.ContentID, bool) { return field.id, field.id.Available() }
func (field Field) Ordinal() uint32                { return field.ordinal }
func (field Field) Slot() heap.Slot                { return field.slot }
func (field Field) Payload() heap.Payload          { return field.payload }
func (field Field) Value() valuedomain.Coordinate  { return field.value }
func (field Field) ValueOrdinal() uint32           { return field.valueOrd }
func (field Field) KeyKind() KeyKind               { return field.keyKind }
func (field Field) ExactKey() (linkproject.Key, bool) {
	return field.exact, field.keyKind == KeyExact
}
func (field Field) ExactSelector() (heap.KeySelector, bool) {
	return field.selector, field.keyKind == KeyExact
}
func (field Field) DynamicKey() (valuedomain.Coordinate, bool) {
	return field.key, field.keyKind == KeyDynamic
}
func (field Field) DynamicKeyOrdinal() (uint32, bool) {
	return field.keyOrd, field.keyKind == KeyDynamic
}
