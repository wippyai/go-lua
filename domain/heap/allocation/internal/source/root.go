// Package source owns the structural classification of one existing Program
// allocation root. It deliberately produces no Heap fact: construction Rules
// consume this fenced source description and own every semantic transition.
package source

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
	id     identity.ContentID
	kind   heap.AllocationKind
	form   Form
}

// New classifies exactly one source Program root. Fresh Target roots have no
// Program origin and are rejected: their guarded creation has a separate
// semantic owner.
func New(schema heap.Schema, key heap.Key) (Root, bool) {
	_, _, _, kind, _, originOK := schema.AllocationOriginForKey(key)
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
	sealed, sealedOK := schema.AllocationFormForKey(root)
	if !sealedOK {
		return FormInvalid, false
	}
	switch kind {
	case heap.AllocationClosure:
		return formFromProgram(sealed)
	case heap.AllocationTable:
		return formFromProgram(sealed)
	default:
		return FormInvalid, false
	}
}

func formFromProgram(form heap.AllocationForm) (Form, bool) {
	switch form {
	case heap.AllocationFormEmpty:
		return FormEmpty, true
	case heap.AllocationFormClosed:
		return FormClosed, true
	case heap.AllocationFormFinalOpen:
		return FormFinalOpen, true
	default:
		return FormInvalid, false
	}
}

func (root Root) ID() (identity.ContentID, bool) { return root.id, root.id.Available() }
func (root Root) Key() heap.Key                  { return root.key }
func (root Root) Kind() heap.AllocationKind      { return root.kind }
func (root Root) Form() Form                     { return root.form }

// Revalidate refuses stale, forged, and cross-schema copies before a Rule
// admits an operand into its semantic or evidence path.
func (root Root) Revalidate(schema heap.Schema) bool {
	canonical, ok := New(schema, root.key)
	return root.FencedTo(schema) && ok && root.same(canonical)
}

func (root Root) same(other Root) bool {
	return root.schema == other.schema && root.key == other.key && root.id == other.id && root.kind == other.kind && root.form == other.form
}

// FencedTo is Root's hot owner/capability fence. A Root that has already
// passed Revalidate at the content, instance, or evidence boundary carries
// its exact Heap schema issuer; recurrent transfer must not reconstruct Link
// allocation topology for every Product row.
func (root Root) FencedTo(schema heap.Schema) bool {
	_, _, _, kind, _, originOK := schema.AllocationOriginForKey(root.key)
	return schema.Valid() && root.schema == schema && root.id.Available() && root.kind != heap.AllocationInvalid && root.form != FormInvalid &&
		schema.OwnsKey(root.key) && root.key.Kind() == heap.RootAllocation && originOK && kind == root.kind
}

// Closed is the complete schema-fenced structural view of one scalar table
// constructor. It is the only field source accepted by the closed Rule.
type Closed struct {
	root   Root
	heap   heap.Schema
	values *valuedomain.Schema
	coords []valuedomain.Coordinate
	keys   []uint64
	fields []Field
}

// NewClosed admits only a scalar closed source table. No caller can use this
// descriptor for an empty/closure root or a final-open table.
func NewClosed(schema heap.Schema, valueSchema *valuedomain.Schema, allocation heap.Key) (Closed, bool) {
	root, rootOK := New(schema, allocation)
	if !rootOK || root.form != FormClosed || valueSchema == nil || !valueSchema.LinkOwner().Matches(schema.LinkOwner()) ||
		!valueSchema.OwnsHeapSchema(schema) {
		return Closed{}, false
	}
	fields, coords, fieldsOK := closedFields(schema, valueSchema, root)
	if !fieldsOK || len(coords) == 0 {
		return Closed{}, false
	}
	keys, keysOK := denseSummaryKeys(valueSchema, allocation, coords)
	if !keysOK {
		return Closed{}, false
	}
	return Closed{root: root, heap: schema, values: valueSchema, coords: coords, keys: keys, fields: fields}, true
}

func (closed Closed) ID() (identity.ContentID, bool) { return closed.root.ID() }
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
	return ok && closed.root.same(canonical.root) && closed.heap == canonical.heap && closed.values == canonical.values && equalCoordinates(closed.coords, canonical.coords) && equalDenseKeys(closed.keys, canonical.keys) && equalFields(closed.fields, canonical.fields)
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

// SummaryKeyCount and SummaryKeyAt expose the canonical dense Value-factor
// coordinates as an immutable scalar range. Closed remains the sole owner of
// the key vector; rule issuance can validate and hash it without allocating a
// copied slice for every mounted constructor occurrence. An unauthenticated
// operand owns no vector at all, and the seam spells that absence as a
// negative count: zero is the length of an authenticated empty range.
func (closed Closed) SummaryKeyCount() int {
	if !closed.valid() {
		return -1
	}
	return len(closed.keys)
}

func (closed Closed) SummaryKeyAt(index int) (uint64, bool) {
	if !closed.valid() || index < 0 || index >= len(closed.keys) {
		return 0, false
	}
	return closed.keys[index], true
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
	exact    heap.ExactKey
	id       identity.ContentID
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
	return closed.heap.Valid() && closed.heap.ContentID().Available() && closed.values != nil && closed.values.LinkOwner().Matches(closed.heap.LinkOwner()) && closed.values.OwnsHeapSchema(closed.heap) && rootIDOK && closed.root.Key().Kind() == heap.RootAllocation && closed.root.Form() == FormClosed && len(closed.coords) != 0 && len(closed.coords) == len(closed.keys) && len(closed.fields) != 0
}

// closedFields composes one constructor's field topology over the operand
// vector its Value axis published.
//
// The two halves belong to different owners and are kept that way. WHICH Value
// coordinates this constructor reads, in which order, is a fact in Value's own
// numbering and is answered by the axis that mints it; a constructor cannot
// state it, and deriving it a second time here would be a second numbering of
// the same reads. What this owner composes is the part that is Heap's: the
// slot, payload, selector and exact key of each field, and the position each
// field's coordinate holds in that published vector.
func closedFields(schema heap.Schema, valueSchema *valuedomain.Schema, root Root) ([]Field, []valuedomain.Coordinate, bool) {
	if valueSchema == nil {
		return nil, nil, false
	}
	module, _, _, _, _, rootOK := schema.AllocationOriginForKey(root.key)
	if !rootOK || !module.Available() {
		return nil, nil, false
	}
	unique, uniqueOK := valueSchema.ClosedOperandCoordinates(root.key)
	if !uniqueOK || len(unique) == 0 {
		return nil, nil, false
	}
	fieldCount := schema.FieldCount(root.key)
	fields := make([]Field, 0, fieldCount)
	for index := 0; index < fieldCount; index++ {
		field, fieldOK := schema.FieldAt(root.key, index)
		slot, slotOK := schema.SlotForField(field)
		payload, payloadOK := schema.PayloadForField(field)
		descriptor, descriptorOK := schema.ArtifactFieldFor(field)
		program, values, valuesOK := schema.ArtifactValuesForField(field)
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		memberOffset, memberCount, memberSpanOK := values.MemberSpan()
		member, memberOK := programschema.ValuesMemberFamily().At(&program.Frozen, catalog, int(memberOffset))
		value, valueRefOK := valueSchema.CoordinateForMountedSemantic(module, member.ID())
		payloadModule, payloadValues, payloadOffset, payloadSourceOK := payload.Source()
		_, width, finalOpen, geometryOK := descriptor.Values()
		if !fieldOK || !slotOK || !payloadOK || !descriptorOK || !valuesOK || !program.Available() || !catalogOK || !memberSpanOK || memberCount != 1 || !geometryOK || finalOpen || width != 1 || !memberOK || !valueRefOK || !payloadSourceOK || payloadModule != module || payloadValues != descriptor.ValuesID() || payloadOffset != 0 {
			return nil, nil, false
		}
		kind := KeyExact
		var exact heap.ExactKey
		switch descriptor.Kind() {
		case heapallocation.FieldKindKey:
			if normalized, normalizedOK := descriptor.NormalizedKey(); !normalizedOK || normalized != 0 {
				return nil, nil, false
			}
			kind = KeyDynamic
		case heapallocation.FieldKindList, heapallocation.FieldKindName, heapallocation.FieldKindExact:
			// A zero exact key is the non-storable nil/NaN outcome, never a
			// dynamic selector.
			normalized, normalizedOK := descriptor.NormalizedKey()
			if !normalizedOK || normalized == 0 {
				return nil, nil, false
			}
			originKind, sourceExact, _, exactOK := slot.Origin()
			if !exactOK || originKind != heap.SlotExact {
				return nil, nil, false
			}
			exact = sourceExact
		default:
			return nil, nil, false
		}
		valueOrd, valueOrdOK := coordinateOrdinal(unique, value)
		if !valueRefOK || !valueOrdOK {
			return nil, nil, false
		}
		var key valuedomain.Coordinate
		var keyOrd uint32
		var selector heap.KeySelector
		if kind == KeyDynamic {
			if descriptor.SharesFirstValueCell() {
				key, keyOrd = value, valueOrd
			} else {
				_, _, dynamicID, dynamicOK := slot.Origin()
				if !dynamicOK {
					return nil, nil, false
				}
				var keyOK bool
				key, keyOK = valueSchema.CoordinateForID(dynamicID)
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

func equalDenseKeys(left, right []uint64) bool {
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

// denseSummaryKeys takes this constructor's operand vector in the dense
// currency the Value axis publishes it in. The keys are read off that axis
// rather than converted here: a coordinate's dense index is Value's numbering,
// and computing it beside the published vector would be a second statement of
// the same order that agrees only while both are correct.
//
// Strictly ascending is checked because it is what the vector MEANS - one cell
// per coordinate, in dense order - and a violation renumbers every later cell
// silently rather than failing.
func denseSummaryKeys(schema *valuedomain.Schema, root heap.Key, coordinates []valuedomain.Coordinate) ([]uint64, bool) {
	if schema == nil || len(coordinates) == 0 {
		return nil, false
	}
	if schema.ClosedOperandKeyCount(root) != len(coordinates) {
		return nil, false
	}
	keys := make([]uint64, len(coordinates))
	for index := range coordinates {
		dense, ok := schema.ClosedOperandKeyAt(root, index)
		if !ok {
			return nil, false
		}
		if index != 0 && keys[index-1] >= uint64(dense) {
			return nil, false
		}
		keys[index] = uint64(dense)
	}
	return keys, true
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

func fieldID(root identity.ContentID, ordinal uint32) (identity.ContentID, bool) {
	if !root.Available() || ordinal == 0 {
		return identity.ContentID{}, false
	}
	var image [48]byte
	copy(image[:32], root[:])
	binary.BigEndian.PutUint64(image[32:40], 0x686561702d666c64)
	binary.BigEndian.PutUint64(image[40:48], uint64(ordinal))
	return sha256.Sum256(image[:]), true
}

func (field Field) ID() (identity.ContentID, bool) { return field.id, field.id.Available() }
func (field Field) Ordinal() uint32                { return field.ordinal }
func (field Field) Slot() heap.Slot                { return field.slot }
func (field Field) Payload() heap.Payload          { return field.payload }
func (field Field) Value() valuedomain.Coordinate  { return field.value }
func (field Field) ValueOrdinal() uint32           { return field.valueOrd }
func (field Field) KeyKind() KeyKind               { return field.keyKind }
func (field Field) ExactKey() (heap.ExactKey, bool) {
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
