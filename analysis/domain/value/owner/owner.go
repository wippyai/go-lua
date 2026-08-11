// Package owner declares Value's cold Factor schema.  Value itself remains
// engine-free: this package retains only its reviewed schema and typed
// Factor-issued forms, never carrier coordinates or runtime handles.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

// Owner is the declaration-time authority for the Link-wide Value relation.
// Correlation remains in value.Value; this owner does not create a second
// registry, state plane, or coordinate vocabulary.
type Owner struct {
	schema  *value.Schema
	factor  *engine.Factor[coordinate, value.Value]
	output  engine.Output[value.Value]
	read    engine.ReadForm[value.Value, engine.OrderedCells[value.Value]]
	summary engine.ReadForm[value.Value, engine.OrderedCells[value.Value]]
	write   engine.WriteForm[value.Value]
	carry   engine.CarryForm
}

// Declare attaches the existing Value schema once to composition.  Exact
// program coordinates are deliberately absent: Wave E binds those to the
// forms retained below.
func Declare(composition *engine.Composition, semantic, summarySemantic engine.SemanticKey, schema *value.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !summarySemantic.Available() || semantic == summarySemantic || schema == nil || schema.Link() == nil || !validCoordinateCount(schema.CoordinateCount()) {
		return nil, false
	}
	owner := &Owner{schema: schema}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, value.Value]{
		Semantic:    semantic,
		KeyEnd:      uint64(schema.CoordinateCount()),
		Lattice:     schema.Domain(),
		Default:     schema.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: schema.Fingerprint,
		WidenRank:   engine.Measure[coordinate, value.Value]{Width: 1, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, value.Value]) bool {
		var valid bool
		owner.output = factor.Output()
		owner.read, valid = engine.ExactReadForm(factor)
		if !valid {
			return false
		}
		normalizer, valid := engine.DeclareIdentityNormalizer(factor, summarySemantic)
		if !valid {
			return false
		}
		owner.summary, valid = engine.SummaryReadForm(normalizer)
		if !valid {
			return false
		}
		owner.write, valid = engine.ExactWriteForm(factor)
		if !valid {
			return false
		}
		owner.carry, valid = engine.Carry(factor)
		return valid
	})
	if !ok {
		return nil, false
	}
	// Keep the typed issuer only after its declaration callback has closed.
	// Locate still cannot issue a capability until the Composition seals.
	owner.factor = factor
	return owner, true
}

// Schema returns Value's immutable semantic vocabulary.
func (owner *Owner) Schema() *value.Schema {
	if owner == nil {
		return nil
	}
	return owner.schema
}

func (owner *Owner) Output() engine.Output[value.Value] {
	if owner == nil {
		return engine.Output[value.Value]{}
	}
	return owner.output
}

func (owner *Owner) ExactRead() engine.ReadForm[value.Value, engine.OrderedCells[value.Value]] {
	if owner == nil {
		return engine.ReadForm[value.Value, engine.OrderedCells[value.Value]]{}
	}
	return owner.read
}

// SummaryRead is Value's sole variable-arity identity observation form. It
// preserves canonical ordered cells exactly; users supply only schema-owned
// Coordinates through NewSummaryRefs and AppendSummaryCoordinate.
func (owner *Owner) SummaryRead() engine.ReadForm[value.Value, engine.OrderedCells[value.Value]] {
	if owner == nil {
		return engine.ReadForm[value.Value, engine.OrderedCells[value.Value]]{}
	}
	return owner.summary
}

func (owner *Owner) ExactWrite() engine.WriteForm[value.Value] {
	if owner == nil {
		return engine.WriteForm[value.Value]{}
	}
	return owner.write
}

func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Value's exact Wave-E coordinate capability for one
// Schema-local coordinate. Schema owns the sole reverse lookup and Factor
// owns the sealed-composition fence.
func (owner *Owner) Locate(location value.Coordinate) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.schema == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.CoordinateIndex(location)
	if !ok || uint64(index) >= uint64(owner.schema.CoordinateCount()) {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(coordinate(index))
}

// AddActivationPortRead binds one exact Value coordinate to a source-owned
// activation import slot.  The Factor remains owner-private; the engine still
// authenticates the issued Ref against this exact issuer.
func (owner *Owner) AddActivationPortRead(port *engine.ActivationPort, slot engine.SemanticKey, ref engine.Ref[coordinate]) bool {
	return owner != nil && owner.factor != nil && engine.AddActivationPortRead(port, slot, owner.factor, ref)
}

// NewSummaryRefs begins one seal-once vector for this Value owner. The
// private Factor coordinate remains opaque to consumers: a caller can retain
// and pass this value to engine binding APIs but can add entries only through
// AppendSummaryCoordinate.
func (owner *Owner) NewSummaryRefs() *engine.ClosedRefs[coordinate] {
	if owner == nil || owner.factor == nil {
		return nil
	}
	return owner.factor.NewClosedRefs()
}

// AppendSummaryCoordinate resolves one schema-local Value coordinate and
// admits its exact Factor Ref to this owner's summary vector. Foreign,
// unavailable, duplicate, open, and sealed vectors fail through the one
// underlying Factor capability path.
func (owner *Owner) AppendSummaryCoordinate(refs *engine.ClosedRefs[coordinate], location value.Coordinate) bool {
	if owner == nil || refs == nil {
		return false
	}
	ref, ok := owner.Locate(location)
	return ok && refs.Append(ref)
}

// CloseSummaryRefs fixes canonical ascending Factor-coordinate order. A
// closed vector is the only form accepted by a SummaryRead binding.
func (owner *Owner) CloseSummaryRefs(refs *engine.ClosedRefs[coordinate]) bool {
	return owner != nil && refs != nil && refs.Close()
}

func (owner *Owner) coordinateAt(index coordinate) (value.Coordinate, bool) {
	if owner == nil || owner.schema == nil {
		return value.Coordinate{}, false
	}
	return owner.schema.CoordinateAt(int(index))
}

func (owner *Owner) admits(index coordinate, fact value.Value) bool {
	issued, ok := owner.coordinateAt(index)
	return ok && owner.schema.AdmitsCoordinate(issued, fact)
}

func (owner *Owner) widenRank(index coordinate, fact value.Value, component int) uint64 {
	if component != 0 {
		return 0
	}
	if _, ok := owner.coordinateAt(index); !ok {
		return 0
	}
	return owner.schema.WidenMeasure(fact)
}

// Coordinate is one-based in Value's semantic vocabulary, so the largest
// lawful dense position range has MaxUint32 entries.  An empty schema remains
// a lawful dormant Factor.
func validCoordinateCount(count int) bool { return count >= 0 && uint64(count) <= uint64(^uint32(0)) }
