// Package owner declares Ownership's cold Factor schema. Ownership remains
// engine-free; only its closed origin-duty coordinate/value law is retained.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/ownership"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

// Owner contains cold Factor-issued forms, never concrete carrier handles.
type Owner struct {
	schema ownership.Schema
	rank   ownership.WidenRank
	keys   map[ownership.Coordinate]coordinate
	factor *engine.Factor[coordinate, ownership.Value]
	output engine.Output[ownership.Value]
	read   engine.ReadForm[ownership.Value, engine.OrderedCells[ownership.Value]]
	write  engine.WriteForm[ownership.Value]
	carry  engine.CarryForm
}

// Declare retains Ownership's exact (AnalysisRoot, Origin, Role) schema as
// the sole coordinate authority. Empty duty universes are lawful.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema ownership.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.CoordinateCount()) {
		return nil, false
	}
	lattice, latticeOK := schema.Lattice()
	defaultValue, defaultOK := schema.Default()
	rank, rankOK := schema.WidenRank()
	if !latticeOK || !defaultOK || !rankOK {
		return nil, false
	}
	keys, keysOK := exactKeys(schema)
	if !keysOK {
		return nil, false
	}
	owner := &Owner{schema: schema, rank: rank, keys: keys}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, ownership.Value]{
		Semantic: semantic, KeyEnd: uint64(schema.CoordinateCount()), Lattice: lattice, Default: defaultValue,
		AdmitAt: owner.admits,
		Fingerprint: func(value ownership.Value) uint64 {
			fingerprint, ok := schema.Fingerprint(value)
			if !ok {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, ownership.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, ownership.Value]) bool {
		read, readOK := engine.ExactReadForm(factor)
		write, writeOK := engine.ExactWriteForm(factor)
		carry, carryOK := engine.Carry(factor)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output, owner.read, owner.write, owner.carry = factor.Output(), read, write, carry
		return true
	})
	if !ok || factor == nil {
		return nil, false
	}
	// Factor.Ref is the only issuer of a usable exact coordinate and fences
	// issuance until the whole Composition seals.
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Schema() ownership.Schema {
	if owner == nil {
		return ownership.Schema{}
	}
	return owner.schema
}

func (owner *Owner) Output() engine.Output[ownership.Value] {
	if owner == nil {
		return engine.Output[ownership.Value]{}
	}
	return owner.output
}

func (owner *Owner) Read() engine.ReadForm[ownership.Value, engine.OrderedCells[ownership.Value]] {
	if owner == nil {
		return engine.ReadForm[ownership.Value, engine.OrderedCells[ownership.Value]]{}
	}
	return owner.read
}

func (owner *Owner) Write() engine.WriteForm[ownership.Value] {
	if owner == nil {
		return engine.WriteForm[ownership.Value]{}
	}
	return owner.write
}

func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues this Owner's exact Factor coordinate for one Schema-issued
// Ownership coordinate. It accepts neither dense ordinals nor reconstructed
// origin tuples: the declaration-time map is the sole local reverse lookup.
func (owner *Owner) Locate(value ownership.Coordinate) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || !owner.schema.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.keys[value]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))
}

func exactKeys(schema ownership.Schema) (map[ownership.Coordinate]coordinate, bool) {
	if !schema.Valid() {
		return nil, false
	}
	keys := make(map[ownership.Coordinate]coordinate, schema.CoordinateCount())
	for index := 0; index < schema.CoordinateCount(); index++ {
		key, ok := schema.CoordinateAt(index)
		if !ok || !key.Valid() {
			return nil, false
		}
		if _, duplicate := keys[key]; duplicate {
			return nil, false
		}
		keys[key] = coordinate(index)
	}
	return keys, true
}

func (owner *Owner) coordinateAt(index coordinate) (ownership.Coordinate, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.CoordinateCount()) {
		return ownership.Coordinate{}, false
	}
	return owner.schema.CoordinateAt(int(index))
}

func (owner *Owner) admits(index coordinate, value ownership.Value) bool {
	key, ok := owner.coordinateAt(index)
	return ok && owner.schema.Admit(key, value)
}

func (owner *Owner) widenRank(index coordinate, value ownership.Value, component int) uint64 {
	key, ok := owner.coordinateAt(index)
	if !ok {
		return 0
	}
	measure, ok := owner.rank.At(key, value, component)
	if !ok {
		return 0
	}
	return measure
}
