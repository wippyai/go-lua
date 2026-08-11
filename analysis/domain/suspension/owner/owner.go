// Package owner declares Suspension's sealed continuation relation as one
// cold Factor. Suspension itself remains independent of engine execution.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
)

// coordinate is private dense indexing over Schema's exact generation keys.
type coordinate uint32

// Owner exposes only Suspension's semantic Schema and cold typed Factor
// forms. It does not retain or disclose concrete Units, Targets, Points, or
// generation-specific engine handles.
type Owner struct {
	schema suspension.Schema
	keys   map[suspension.Key]coordinate
	factor *engine.Factor[coordinate, suspension.Value]

	output engine.Output[suspension.Value]
	read   engine.ReadForm[suspension.Value, engine.OrderedCells[suspension.Value]]
	write  engine.WriteForm[suspension.Value]
	carry  engine.CarryForm
}

// Declare adds the exact sealed occurrence/module-init generation range to
// an open Composition. Empty ranges are lawful; their default admission
// obligation is vacuous rather than a reason to omit the Factor.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema suspension.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.Valid() || !validCoordinateCount(schema.KeyCount()) {
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
	owner := &Owner{schema: schema, keys: keys}
	factor, declared := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, suspension.Value]{
		Semantic: semantic,
		KeyEnd:   uint64(schema.KeyCount()),
		Lattice:  lattice,
		Default:  defaultValue,
		AdmitAt:  owner.admits,
		Fingerprint: func(value suspension.Value) uint64 {
			fingerprint, ok := schema.Fingerprint(value)
			if !ok {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, suspension.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, suspension.Value]) bool {
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

// Schema exposes Suspension's immutable continuation vocabulary.
func (owner *Owner) Schema() suspension.Schema {
	if owner == nil {
		return suspension.Schema{}
	}
	return owner.schema
}

// Link returns the exact sealed Link that issued this Suspension owner's
// generation keys. It is a construction provenance fence; lifecycle Values
// remain Link-free.
func (owner *Owner) Link() *link.Link {
	if owner == nil {
		return nil
	}
	return owner.schema.Link()
}

// Output is Suspension's sole typed Rule-output capability.
func (owner *Owner) Output() engine.Output[suspension.Value] {
	if owner == nil {
		return engine.Output[suspension.Value]{}
	}
	return owner.output
}

// Read is the exact read shape, still unanchored to an occurrence.
func (owner *Owner) Read() engine.ReadForm[suspension.Value, engine.OrderedCells[suspension.Value]] {
	if owner == nil {
		return engine.ReadForm[suspension.Value, engine.OrderedCells[suspension.Value]]{}
	}
	return owner.read
}

// Write is the exact output shape, still unanchored to a generation.
func (owner *Owner) Write() engine.WriteForm[suspension.Value] {
	if owner == nil {
		return engine.WriteForm[suspension.Value]{}
	}
	return owner.write
}

// Carry is Suspension's explicit whole-Factor predecessor capability.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues this Owner's exact Factor coordinate for one Schema-issued
// continuation-generation key. It never accepts a raw ordinal or falls back
// to a scan over occurrence identities.
func (owner *Owner) Locate(key suspension.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || !owner.schema.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.keys[key]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func (owner *Owner) keyAt(index coordinate) (suspension.Key, bool) {
	if owner == nil || !owner.schema.Valid() || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return suspension.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

// admits preserves the Schema's per-generation subject fence at ingress.
func (owner *Owner) admits(index coordinate, value suspension.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.schema.Admits(key, value)
}

func (owner *Owner) widenRank(index coordinate, value suspension.Value, component int) uint64 {
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

func exactKeys(schema suspension.Schema) (map[suspension.Key]coordinate, bool) {
	if !schema.Valid() {
		return nil, false
	}
	keys := make(map[suspension.Key]coordinate, schema.KeyCount())
	for index := 0; index < schema.KeyCount(); index++ {
		key, ok := schema.KeyAt(index)
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
