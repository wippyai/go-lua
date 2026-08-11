// Package owner declares Residence's cold Factor schema. The private dense
// coordinate is cold layout only, never an actor, allocation decision, or
// persisted identity.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/residence"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

type Owner struct {
	schema      residence.Schema
	rank        residence.WidenRank
	factor      *engine.Factor[coordinate, residence.Value]
	coordinates map[residence.Key]coordinate
	output      engine.Output[residence.Value]
	read        engine.ReadForm[residence.Value, engine.OrderedCells[residence.Value]]
	write       engine.WriteForm[residence.Value]
	carry       engine.CarryForm
}

// Declare retains Residence's schema-issued key/value fence. The schema's
// AnalysisRoot-sensitive applicability is consumed only when Wave E binds a
// concrete Program/Link occurrence; no root is captured in this cold owner.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema residence.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !schema.ContentID().Available() || !validCoordinateCount(schema.KeyCount()) {
		return nil, false
	}
	rank, rankOK := residence.NewWidenRank(schema)
	if !rankOK {
		return nil, false
	}
	coordinates, indexed := locateCoordinates(schema)
	if !indexed {
		return nil, false
	}
	domain := schema.Domain()
	defaultValue := schema.Default()
	if !domain.Equal(defaultValue, defaultValue) {
		return nil, false
	}
	owner := &Owner{schema: schema, rank: rank, coordinates: coordinates}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, residence.Value]{
		Semantic: semantic, KeyEnd: uint64(schema.KeyCount()), Lattice: domain, Default: defaultValue,
		AdmitAt: owner.admits,
		Fingerprint: func(value residence.Value) uint64 {
			fingerprint, ok := schema.Fingerprint(value)
			if !ok {
				return 0
			}
			return fingerprint
		},
		WidenRank: engine.Measure[coordinate, residence.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, residence.Value]) bool {
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
	// The Factor becomes a retained issuer only after its declaration callback
	// has closed. Ref still fences Locate until the whole Composition seals.
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Schema() residence.Schema {
	if owner == nil {
		return residence.Schema{}
	}
	return owner.schema
}

func (owner *Owner) Output() engine.Output[residence.Value] {
	if owner == nil {
		return engine.Output[residence.Value]{}
	}
	return owner.output
}

func (owner *Owner) Read() engine.ReadForm[residence.Value, engine.OrderedCells[residence.Value]] {
	if owner == nil {
		return engine.ReadForm[residence.Value, engine.OrderedCells[residence.Value]]{}
	}
	return owner.read
}

func (owner *Owner) Write() engine.WriteForm[residence.Value] {
	if owner == nil {
		return engine.WriteForm[residence.Value]{}
	}
	return owner.write
}

func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Residence's exact sealed Factor capability for one Key from
// this owner's immutable Schema range. The reverse index is built once from
// KeyAt order during declaration; it neither scans Link nor accepts a dense
// coordinate from callers.
func (owner *Owner) Locate(key residence.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || owner.coordinates == nil {
		return engine.Ref[coordinate]{}, false
	}
	selector, found := owner.coordinates[key]
	if !found {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(selector)
}

func locateCoordinates(schema residence.Schema) (map[residence.Key]coordinate, bool) {
	count := schema.KeyCount()
	if !validCoordinateCount(count) {
		return nil, false
	}
	coordinates := make(map[residence.Key]coordinate, count)
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

func (owner *Owner) coordinateAt(index coordinate) (residence.Key, bool) {
	if owner == nil || !owner.schema.ContentID().Available() || uint64(index) >= uint64(owner.schema.KeyCount()) {
		return residence.Key{}, false
	}
	return owner.schema.KeyAt(int(index))
}

func (owner *Owner) admits(index coordinate, value residence.Value) bool {
	_, ok := owner.coordinateAt(index)
	return ok && owner.schema.Admits(value)
}

func (owner *Owner) widenRank(index coordinate, value residence.Value, component int) uint64 {
	key, ok := owner.coordinateAt(index)
	if !ok {
		return 0
	}
	return owner.rank.At(key, value, component)
}
