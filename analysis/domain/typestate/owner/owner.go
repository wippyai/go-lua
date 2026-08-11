// Package owner declares Typestate's sealed relation family as one cold
// Factor. Typestate owns the protocol algebra; this package owns only the
// declarative engine boundary.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine"
)

// coordinate is Typestate's private dense selector over the sealed Schema.
type coordinate uint32

// Owner retains the exact schema and algebra plus cold typed capabilities.
// No public method exposes concrete carrier/target state or reconstructed
// protocol coordinates.
type Owner struct {
	schema      typestate.Schema
	algebra     typestate.Algebra
	factor      *engine.Factor[coordinate, typestate.Relation]
	coordinates map[typestate.Key]coordinate

	output engine.Output[typestate.Relation]
	read   engine.ReadForm[typestate.Relation, engine.OrderedCells[typestate.Relation]]
	write  engine.WriteForm[typestate.Relation]
	carry  engine.CarryForm
}

// Declare adds the exact D0 relation family to an open Composition. The
// Typestate schema/algebra remain the sole source of coordinate admission and
// well-founded widening rank.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema typestate.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.KeyCount()) {
		return nil, false
	}
	algebra, algebraOK := typestate.NewAlgebra(schema)
	if !algebraOK {
		return nil, false
	}
	coordinates, indexed := locateCoordinates(schema)
	if !indexed {
		return nil, false
	}
	lattice := algebra.Lattice()
	defaultValue := algebra.Default()
	if !lattice.Equal(defaultValue, defaultValue) {
		return nil, false
	}
	owner := &Owner{schema: schema, algebra: algebra, coordinates: coordinates}
	factor, declared := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, typestate.Relation]{
		Semantic:    semantic,
		KeyEnd:      uint64(schema.KeyCount()),
		Lattice:     lattice,
		Default:     defaultValue,
		AdmitAt:     owner.admits,
		Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, typestate.Relation]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, func(factor *engine.Factor[coordinate, typestate.Relation]) bool {
		read, readOK := engine.ExactReadForm(factor)
		write, writeOK := engine.ExactWriteForm(factor)
		carry, carryOK := engine.Carry(factor)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output = factor.Output()
		owner.read = read
		owner.write = write
		owner.carry = carry
		return true
	})
	if !declared || factor == nil {
		return nil, false
	}
	owner.factor = factor
	return owner, true
}

// Schema exposes Typestate's immutable Link-derived vocabulary.
func (owner *Owner) Schema() typestate.Schema {
	if owner == nil {
		return typestate.Schema{}
	}
	return owner.schema
}

// Algebra exposes Typestate's immutable semantic algebra, never Factor
// machinery. It is retained so future Rule declarations can use proven
// protocol operations without rederiving the Schema.
func (owner *Owner) Algebra() typestate.Algebra {
	if owner == nil {
		return typestate.Algebra{}
	}
	return owner.algebra
}

// Output is Typestate's sole typed Rule-output capability.
func (owner *Owner) Output() engine.Output[typestate.Relation] {
	if owner == nil {
		return engine.Output[typestate.Relation]{}
	}
	return owner.output
}

// Read is the Factor-issued exact read shape.
func (owner *Owner) Read() engine.ReadForm[typestate.Relation, engine.OrderedCells[typestate.Relation]] {
	if owner == nil {
		return engine.ReadForm[typestate.Relation, engine.OrderedCells[typestate.Relation]]{}
	}
	return owner.read
}

// Write is the Factor-issued exact output shape.
func (owner *Owner) Write() engine.WriteForm[typestate.Relation] {
	if owner == nil {
		return engine.WriteForm[typestate.Relation]{}
	}
	return owner.write
}

// Carry is Typestate's explicit whole-Factor predecessor capability.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Typestate's exact sealed Factor capability for one admitted
// resource Key. The private map is the one KeyAt-order image of the already
// sealed Schema; it creates no protocol or holder vocabulary.
func (owner *Owner) Locate(key typestate.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || owner.coordinates == nil {
		return engine.Ref[coordinate]{}, false
	}
	selector, found := owner.coordinates[key]
	if !found {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(selector)
}

func locateCoordinates(schema typestate.Schema) (map[typestate.Key]coordinate, bool) {
	count := schema.KeyCount()
	if !validCoordinateCount(count) {
		return nil, false
	}
	coordinates := make(map[typestate.Key]coordinate, count)
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

func (owner *Owner) keyAt(index coordinate) (typestate.Key, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return typestate.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

// admits delegates all protocol-state/holder/duty compatibility to the
// proven Typestate algebra; there is no duplicate owner-side predicate.
func (owner *Owner) admits(index coordinate, value typestate.Relation) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.algebra.AdmitsAt(key, value)
}

func (owner *Owner) widenRank(index coordinate, value typestate.Relation, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	return owner.algebra.WidenRank(key, value, component)
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
