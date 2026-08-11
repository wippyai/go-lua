// Package owner declares Footprint's cold Factor schema. Footprint Keys stay
// the semantic source authority; dense coordinates are never exposed.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/footprint"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

type Owner struct {
	schema      footprint.Schema
	algebra     footprint.Algebra
	factor      *engine.Factor[coordinate, footprint.Value]
	coordinates map[footprint.Key]coordinate
	output      engine.Output[footprint.Value]
	read        engine.ReadForm[footprint.Value, engine.OrderedCells[footprint.Value]]
	write       engine.WriteForm[footprint.Value]
	carry       engine.CarryForm
}

// Declare retains the complete Schema-derived key/value fence without
// declaring a concrete engine unit, target, or Program/Link occurrence.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema footprint.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.KeyCount()) {
		return nil, false
	}
	algebra, algebraOK := footprint.NewAlgebra(schema)
	if !algebraOK {
		return nil, false
	}
	coordinates, indexed := locateCoordinates(schema)
	if !indexed {
		return nil, false
	}
	domain := algebra.Lattice()
	defaultValue := algebra.Default()
	if !domain.Equal(defaultValue, defaultValue) {
		return nil, false
	}
	owner := &Owner{schema: schema, algebra: algebra, coordinates: coordinates}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, footprint.Value]{
		Semantic: semantic, KeyEnd: uint64(schema.KeyCount()), Lattice: domain, Default: defaultValue,
		AdmitAt: owner.admits, Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, footprint.Value]{Width: 1, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, footprint.Value]) bool {
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
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Schema() footprint.Schema {
	if owner == nil {
		return footprint.Schema{}
	}
	return owner.schema
}

func (owner *Owner) Output() engine.Output[footprint.Value] {
	if owner == nil {
		return engine.Output[footprint.Value]{}
	}
	return owner.output
}

func (owner *Owner) Read() engine.ReadForm[footprint.Value, engine.OrderedCells[footprint.Value]] {
	if owner == nil {
		return engine.ReadForm[footprint.Value, engine.OrderedCells[footprint.Value]]{}
	}
	return owner.read
}

func (owner *Owner) Write() engine.WriteForm[footprint.Value] {
	if owner == nil {
		return engine.WriteForm[footprint.Value]{}
	}
	return owner.write
}

func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Footprint's exact sealed Factor capability for one Key from
// this exact Heap-derived Schema. Its map is populated only from KeyAt order;
// a caller cannot provide or recover the private dense coordinate.
func (owner *Owner) Locate(key footprint.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || owner.coordinates == nil {
		return engine.Ref[coordinate]{}, false
	}
	selector, found := owner.coordinates[key]
	if !found {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(selector)
}

func locateCoordinates(schema footprint.Schema) (map[footprint.Key]coordinate, bool) {
	count := schema.KeyCount()
	if !validCoordinateCount(count) {
		return nil, false
	}
	coordinates := make(map[footprint.Key]coordinate, count)
	for index := 0; index < count; index++ {
		key, ok := schema.KeyAt(index)
		selector := coordinate(index)
		if !ok || uint64(selector) != uint64(index) {
			return nil, false
		}
		if _, duplicate := coordinates[key]; duplicate {
			return nil, false
		}
		coordinates[key] = selector
	}
	return coordinates, len(coordinates) == count
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}

func (owner *Owner) coordinateAt(index coordinate) (footprint.Key, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return footprint.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

func (owner *Owner) admits(index coordinate, value footprint.Value) bool {
	_, ok := owner.coordinateAt(index)
	if !ok {
		return false
	}
	return owner.algebra.Admits(value)
}

func (owner *Owner) widenRank(index coordinate, value footprint.Value, component int) uint64 {
	key, ok := owner.coordinateAt(index)
	if !ok {
		return 0
	}
	return owner.algebra.WidenRank(key, value, component)
}
