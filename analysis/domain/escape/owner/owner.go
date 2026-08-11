// Package owner declares Escape's cold Factor schema. Escape itself remains
// engine-free; this package retains only the sealed coordinate/value law and
// exports Factor-issued capabilities for the later template compiler.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/escape"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

// Owner contains no concrete units, targets, or Link occurrence binding.
// The exact forms are deliberately unanchored: Wave E supplies coordinates
// from canonical Program/Link structure once, when it builds templates.
type Owner struct {
	schema escape.Schema
	rank   escape.WidenRank
	factor *engine.Factor[coordinate, escape.Value]
	locate map[escape.Coordinate]coordinate
	output engine.Output[escape.Value]
	read   engine.ReadForm[escape.Value, engine.OrderedCells[escape.Value]]
	write  engine.WriteForm[escape.Value]
	carry  engine.CarryForm
}

// Declare installs Escape's finite, sealed coordinate/value algebra in the
// open Composition. The empty coordinate universe is lawful.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema escape.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.CoordinateCount()) {
		return nil, false
	}
	lattice, latticeOK := schema.Lattice()
	defaultValue, defaultOK := schema.Default()
	rank, rankOK := schema.WidenRank()
	if !latticeOK || !defaultOK || !rankOK {
		return nil, false
	}
	locations, located := locateCoordinates(schema)
	if !located {
		return nil, false
	}
	owner := &Owner{schema: schema, rank: rank, locate: locations}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, escape.Value]{
		Semantic: semantic, KeyEnd: uint64(schema.CoordinateCount()), Lattice: lattice, Default: defaultValue,
		AdmitAt: owner.admits,
		Fingerprint: func(value escape.Value) uint64 {
			fingerprint, ok := schema.Fingerprint(value)
			if !ok {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, escape.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, escape.Value]) bool {
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

// Schema returns the immutable Escape semantic algebra.
func (owner *Owner) Schema() escape.Schema {
	if owner == nil {
		return escape.Schema{}
	}
	return owner.schema
}

// Output is Escape's sole typed Rule-output capability.
func (owner *Owner) Output() engine.Output[escape.Value] {
	if owner == nil {
		return engine.Output[escape.Value]{}
	}
	return owner.output
}

// Read is Escape's exact, unanchored observation form.
func (owner *Owner) Read() engine.ReadForm[escape.Value, engine.OrderedCells[escape.Value]] {
	if owner == nil {
		return engine.ReadForm[escape.Value, engine.OrderedCells[escape.Value]]{}
	}
	return owner.read
}

// Write is Escape's exact, unanchored write form.
func (owner *Owner) Write() engine.WriteForm[escape.Value] {
	if owner == nil {
		return engine.WriteForm[escape.Value]{}
	}
	return owner.write
}

// Carry is Escape's whole-Factor predecessor form.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Escape's exact sealed coordinate capability for one canonical
// Schema-local boundary. The reverse relation is fixed by CoordinateAt's
// complete sealed order and contains no Link handle scan or mutable cache.
func (owner *Owner) Locate(location escape.Coordinate) (engine.Ref[coordinate], bool) {
	if owner == nil || !owner.schema.Valid() || owner.factor == nil || !location.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.locate[location]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func locateCoordinates(schema escape.Schema) (map[escape.Coordinate]coordinate, bool) {
	if !schema.Valid() || !validCoordinateCount(schema.CoordinateCount()) {
		return nil, false
	}
	locations := make(map[escape.Coordinate]coordinate, schema.CoordinateCount())
	for index := 0; index < schema.CoordinateCount(); index++ {
		location, ok := schema.CoordinateAt(index)
		if !ok || !location.Valid() || uint64(index) > uint64(^uint32(0)) {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := locations[location]; duplicate {
			return nil, false
		}
		locations[location] = coordinate
	}
	return locations, len(locations) == schema.CoordinateCount()
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))
}

func (owner *Owner) coordinateAt(index coordinate) (escape.Coordinate, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.CoordinateCount()) {
		return escape.Coordinate{}, false
	}
	return owner.schema.CoordinateAt(int(index))
}

func (owner *Owner) admits(index coordinate, value escape.Value) bool {
	key, ok := owner.coordinateAt(index)
	return ok && owner.schema.Admit(key, value)
}

func (owner *Owner) widenRank(index coordinate, value escape.Value, component int) uint64 {
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
