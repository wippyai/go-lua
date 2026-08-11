// Package owner declares Module's sealed cache relation as one cold Factor.
// Module remains engine-free: this package translates its complete Schema
// algebra into Factor capabilities, never into concrete analysis coordinates.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/module"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
)

// coordinate is the Factor-private dense index for Schema's already-sealed
// key table. It is not part of the owner API and cannot be named by Rules.
type coordinate uint32

// Owner retains Module's semantic authority and only the typed cold
// capabilities subsequently consumed by Rule declarations. Its private
// key-to-layout snapshot is not a public coordinate plane: no caller can
// obtain a Unit, Target, or carrier-facing handle from this owner.
type Owner struct {
	schema module.Schema
	factor *engine.Factor[coordinate, module.Value]
	locate map[module.Key]coordinate

	output engine.Output[module.Value]
	read   engine.ReadForm[module.Value, engine.OrderedCells[module.Value]]
	write  engine.WriteForm[module.Value]
	carry  engine.CarryForm
}

// Declare adds Module's one Factor to an open Composition. The Schema has
// already fixed the complete cache-coordinate universe; an empty universe is
// lawful and has vacuous default-admission proof.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema module.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.KeyCount()) {
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
	owner := &Owner{schema: schema, locate: locations}
	factor, declared := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, module.Value]{
		Semantic: semantic,
		KeyEnd:   uint64(schema.KeyCount()),
		Lattice:  lattice,
		Default:  defaultValue,
		AdmitAt:  owner.admits,
		Fingerprint: func(value module.Value) uint64 {
			fingerprint, ok := schema.Fingerprint(value)
			if !ok {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, module.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, module.Value]) bool {
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

// Schema exposes Module's immutable cache vocabulary, not an engine view.
func (owner *Owner) Schema() module.Schema {
	if owner == nil {
		return module.Schema{}
	}
	return owner.schema
}

// Link returns the exact sealed Link that issued this Module owner's keys.
// It is a provenance fence only; Module values retain no live Link pointer.
func (owner *Owner) Link() *link.Link {
	if owner == nil {
		return nil
	}
	return owner.schema.Link()
}

// Output is Module's sole typed Rule-output capability.
func (owner *Owner) Output() engine.Output[module.Value] {
	if owner == nil {
		return engine.Output[module.Value]{}
	}
	return owner.output
}

// Read is the Factor-issued exact read shape. Rule templates supply
// coordinates later; this owner never exposes a concrete cell.
func (owner *Owner) Read() engine.ReadForm[module.Value, engine.OrderedCells[module.Value]] {
	if owner == nil {
		return engine.ReadForm[module.Value, engine.OrderedCells[module.Value]]{}
	}
	return owner.read
}

// Write is the Factor-issued exact output shape.
func (owner *Owner) Write() engine.WriteForm[module.Value] {
	if owner == nil {
		return engine.WriteForm[module.Value]{}
	}
	return owner.write
}

// Carry is Module's explicit whole-Factor predecessor capability.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Module's exact sealed coordinate capability for one canonical
// Schema-local key. The owner records only the schema's complete KeyAt order;
// callers cannot address Factor layout directly.
func (owner *Owner) Locate(key module.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || !owner.schema.Valid() || owner.factor == nil || !key.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.locate[key]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func locateCoordinates(schema module.Schema) (map[module.Key]coordinate, bool) {
	if !schema.Valid() || !validCoordinateCount(schema.KeyCount()) {
		return nil, false
	}
	locations := make(map[module.Key]coordinate, schema.KeyCount())
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
		if !ok || !key.Valid() || uint64(index) > uint64(^uint32(0)) {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := locations[key]; duplicate {
			return nil, false
		}
		locations[key] = coordinate
	}
	return locations, len(locations) == schema.KeyCount()
}

func (owner *Owner) keyAt(index coordinate) (module.Key, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return module.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

// admits is the sole key-specific ingress fence. It remains owned by the
// Module Schema and is passed unchanged to FactorSpec.
func (owner *Owner) admits(index coordinate, value module.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.schema.Admits(key, value)
}

func (owner *Owner) widenRank(index coordinate, value module.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	rank, ok := owner.schema.WidenRank()
	if !ok {
		return 0
	}
	measure, ok := rank.At(key, value, component)
	if !ok {
		return 0
	}
	return measure
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
