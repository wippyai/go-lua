package value

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"

	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/domain/heap"
)

// ClosedOperandCoordinates is the ordered coordinate vector one closed scalar
// table constructor reads: every Value coordinate its fields consume, each
// named once, in this axis's own dense order.
//
// It is answered here because this is the only axis that can answer it. The
// fields are Heap's - their descriptors, slots and payloads are Heap topology
// - but the vector is a list of VALUE coordinates in VALUE's numbering, and
// that numbering exists nowhere else: a coordinate's dense index is the
// position this schema's normalizer assigned it. Heap is upstream of this
// axis and holds no index into it, so a constructor cannot state its own
// operand vector, and a second derivation of it somewhere else would be a
// second numbering of the same reads.
//
// The vector is the DENOMINATOR a whole-vector read over these operands
// spans, and the constructor's own field topology addresses positions in it,
// so both come from this one derivation rather than from two that agree by
// luck. The order is this axis's dense order because that is the order a
// vector read is delivered in; duplicates collapse because a coordinate read
// by two fields is one cell observed once, not two.
func (schema *Schema) ClosedOperandCoordinates(root heap.Key) ([]Coordinate, bool) {
	if schema == nil || !schema.heap.Valid() {
		return nil, false
	}
	heaps := schema.heap
	module, _, _, _, _, rootOK := heaps.AllocationOriginForKey(root)
	if !rootOK || !module.Available() {
		return nil, false
	}
	fieldCount := heaps.FieldCount(root)
	coordinates := make([]Coordinate, 0, fieldCount*2)
	appendCoordinate := func(coordinate Coordinate) bool {
		if _, ok := schema.CoordinateIndex(coordinate); !ok {
			return false
		}
		coordinates = append(coordinates, coordinate)
		return true
	}
	for index := 0; index < fieldCount; index++ {
		field, fieldOK := heaps.FieldAt(root, index)
		descriptor, descriptorOK := heaps.ArtifactFieldFor(field)
		program, values, valuesOK := heaps.ArtifactValuesForField(field)
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		memberOffset, memberCount, memberSpanOK := values.MemberSpan()
		member, memberOK := programschema.ValuesMemberFamily().At(&program.Frozen, catalog, int(memberOffset))
		value, valueRefOK := schema.CoordinateForMountedSemantic(module, member.ID())
		_, width, finalOpen, geometryOK := descriptor.Values()
		if !fieldOK || !descriptorOK || !valuesOK || !program.Available() || !catalogOK || !memberSpanOK || memberCount != 1 || !geometryOK || finalOpen || width != 1 || !memberOK || !valueRefOK {
			return nil, false
		}
		switch descriptor.Kind() {
		case heapallocation.FieldKindKey:
			if normalized, normalizedOK := descriptor.NormalizedKey(); !normalizedOK || normalized != 0 {
				return nil, false
			}
			// A dynamic key that shares the first value cell reads one
			// coordinate in both roles; one that does not reads a second, the
			// slot's own origin, and both are operands of this constructor.
			if descriptor.SharesFirstValueCell() {
				if !appendCoordinate(value) {
					return nil, false
				}
				continue
			}
			if !appendCoordinate(value) {
				return nil, false
			}
			sourceSlot, sourceSlotOK := heaps.SlotForField(field)
			if !sourceSlotOK {
				return nil, false
			}
			_, _, dynamicID, dynamicOK := sourceSlot.Origin()
			coordinate, coordinateOK := schema.CoordinateForID(dynamicID)
			if !dynamicOK || !coordinateOK || !appendCoordinate(coordinate) {
				return nil, false
			}
		case heapallocation.FieldKindList, heapallocation.FieldKindName, heapallocation.FieldKindExact:
			// A zero normalized key is the non-storable nil/NaN outcome. It
			// selects nothing and must not fall through to the dynamic branch.
			normalized, normalizedOK := descriptor.NormalizedKey()
			if !normalizedOK || normalized == 0 {
				return nil, false
			}
			if !appendCoordinate(value) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	sort.Slice(coordinates, func(left, right int) bool {
		leftIndex, leftOK := schema.CoordinateIndex(coordinates[left])
		rightIndex, rightOK := schema.CoordinateIndex(coordinates[right])
		return leftOK && rightOK && leftIndex < rightIndex
	})
	unique := coordinates[:0]
	for _, coordinate := range coordinates {
		if len(unique) == 0 || unique[len(unique)-1] != coordinate {
			unique = append(unique, coordinate)
		}
	}
	if len(unique) == 0 {
		return nil, false
	}
	return unique, true
}

// ClosedOperandKeyCount and ClosedOperandKeyAt publish that same vector as
// this axis's dense keys: the span a whole-vector read over one constructor's
// operands is taken over, and the coordinate each of its cells sits at.
//
// They are the pair a reading rule's declaration names. Nothing is derived
// here that ClosedOperandCoordinates did not already decide - the keys are its
// coordinates in its order - so the read spans exactly the operands the
// constructor consumes, and neither side can drift from the other.
//
// A root that is not an admissible closed constructor publishes no vector, and
// the count says so as a negative rather than as an empty one: zero is the
// width of a vector that exists and is empty.
func (schema *Schema) ClosedOperandKeyCount(root heap.Key) int {
	coordinates, ok := schema.ClosedOperandCoordinates(root)
	if !ok {
		return -1
	}
	return len(coordinates)
}

func (schema *Schema) ClosedOperandKeyAt(root heap.Key, index int) (uint32, bool) {
	coordinates, ok := schema.ClosedOperandCoordinates(root)
	if !ok || index < 0 || index >= len(coordinates) {
		return 0, false
	}
	return schema.CoordinateIndex(coordinates[index])
}

// ClosedOperands is one closed constructor seen from this axis: the row that
// carries the operand vector published above.
//
// It exists because the vector needs a row to hang off. The constructor itself
// is a Heap row and Heap cannot hold this axis's numbering, so a reader that
// wants the span asks the axis that states it, at a row addressed by the same
// occurrence the Heap row is. Nothing about the constructor is re-decided
// here: the row is a handle onto the schema and the allocation, and every
// answer it gives is the published vector's.
type ClosedOperands struct {
	schema *Schema
	root   heap.Key
}

// Root is the allocation this row is the operand view of.
func (operands ClosedOperands) Root() heap.Key { return operands.root }

// KeyVectorCount and KeyVectorAt are the span a whole-vector read over this
// constructor's operands is taken over, in this axis's dense keys. They are
// the pair the reading rule's declaration names, and they answer exactly what
// ClosedOperandKeyCount and ClosedOperandKeyAt answer - the row holds no
// second version of the vector.
func (operands ClosedOperands) KeyVectorCount() int {
	if operands.schema == nil {
		return -1
	}
	return operands.schema.ClosedOperandKeyCount(operands.root)
}

func (operands ClosedOperands) KeyVectorAt(index int) (uint32, bool) {
	if operands.schema == nil {
		return 0, false
	}
	return operands.schema.ClosedOperandKeyAt(operands.root, index)
}

// ClosedOperand is one cell of that vector: the coordinate the constructor
// reads at one position. It carries the coordinate and nothing else, because
// that is all a cell of this vector is - the operands are Value coordinates
// this axis already issued, and giving them a second identity of their own
// would number the same reads twice.
type ClosedOperand struct {
	coordinate Coordinate
}

// Coordinate is this operand's key: the coordinate its cell is read at.
func (operand ClosedOperand) Coordinate() Coordinate { return operand.coordinate }

// ClosedOperandsCount and ClosedOperandsAt are this axis's directory of
// constructor operand rows. Its order is the Heap closed-allocation order,
// taken from the Heap schema this one was sealed against rather than built
// beside it: the two directories enumerate the same constructors, and a
// correspondence between them is only true if neither renumbers.
func (schema *Schema) ClosedOperandsCount() int {
	if schema == nil || !schema.heap.Valid() {
		return 0
	}
	return schema.heap.ClosedAllocationCount()
}

func (schema *Schema) ClosedOperandsAt(index int) (ClosedOperands, bool) {
	if schema == nil || !schema.heap.Valid() {
		return ClosedOperands{}, false
	}
	root, rootOK := schema.heap.ClosedAllocationAt(index)
	if !rootOK {
		return ClosedOperands{}, false
	}
	return ClosedOperands{schema: schema, root: root}, true
}

// ClosedOperandsOrdinal is the position one operand row holds in that
// directory. A row of another schema is refused rather than located by its
// allocation: the row is an owner handle, and two schemas sealed from one Link
// are deliberately not interchangeable.
func (schema *Schema) ClosedOperandsOrdinal(operands ClosedOperands) (uint32, bool) {
	if schema == nil || operands.schema != schema || !schema.heap.Valid() {
		return 0, false
	}
	return schema.heap.ClosedAllocationOrdinal(operands.root)
}

// ClosedOperandsForMountedOccurrence resolves the operand row of the
// constructor one mounted occurrence denotes. It is the address both
// directories share, which is what lets a rule whose candidate is the Heap
// constructor reach this axis's row for the same one.
func (schema *Schema) ClosedOperandsForMountedOccurrence(module, occurrence identity.ContentID) (ClosedOperands, bool) {
	if schema == nil || !schema.heap.Valid() {
		return ClosedOperands{}, false
	}
	root, rootOK := schema.heap.ClosedAllocationForMountedOccurrence(module, occurrence)
	if !rootOK {
		return ClosedOperands{}, false
	}
	return ClosedOperands{schema: schema, root: root}, true
}
