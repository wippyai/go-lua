// Package owner declares Pack's cold Factor schema.  Pack's relation stays
// engine-free; exact root positions are bound only by the later template
// compiler.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

// SummaryRefs is the Pack owner's opaque, seal-once vector of exact Factor
// coordinates.  The source side may retain the vector as an instance witness,
// but it cannot inspect or manufacture the private coordinate type.
type SummaryRefs struct {
	owner *Owner
	refs  *engine.ClosedRefs[coordinate]
}

type Owner struct {
	schema      *pack.Schema
	coordinates map[pack.Root]coordinate
	factor      *engine.Factor[coordinate, pack.Value]
	output      engine.Output[pack.Value]
	read        engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]]
	summary     engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]]
	hasSummary  bool
	write       engine.WriteForm[pack.Value]
	carry       engine.CarryForm
}

func Declare(composition *engine.Composition, semantic engine.SemanticKey, schema *pack.Schema) (*Owner, bool) {
	return declare(composition, semantic, engine.SemanticKey{}, schema)
}

// DeclareWithSummary closes the same Pack Factor with its one identity
// summary form.  It is used by Pack's variable-arity outcome transfer; the
// ordinary Declare surface remains exact-read-only for domains that do not
// need a summary vector.
func DeclareWithSummary(composition *engine.Composition, semantic, summarySemantic engine.SemanticKey, schema *pack.Schema) (*Owner, bool) {
	return declare(composition, semantic, summarySemantic, schema)
}

func declare(composition *engine.Composition, semantic, summarySemantic engine.SemanticKey, schema *pack.Schema) (*Owner, bool) {
	if composition == nil || !semantic.Available() || schema == nil || !validCount(schema.RootCount()) {
		return nil, false
	}
	if summarySemantic.Available() && summarySemantic == semantic {
		return nil, false
	}
	coordinates, ok := rootCoordinates(schema)
	if !ok {
		return nil, false
	}
	owner := &Owner{schema: schema, coordinates: coordinates}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, pack.Value]{
		Semantic: semantic, KeyEnd: uint64(schema.RootCount()), Lattice: schema.Lattice(), Default: schema.Bottom(),
		AdmitAt: owner.admits, Fingerprint: schema.Fingerprint,
		WidenRank: engine.Measure[coordinate, pack.Value]{Width: 4, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, pack.Value]) bool {
		var valid bool
		owner.output = factor.Output()
		owner.read, valid = engine.ExactReadForm(factor)
		if !valid {
			return false
		}
		if summarySemantic.Available() {
			normalizer, normalizerOK := engine.DeclareIdentityNormalizer(factor, summarySemantic)
			if !normalizerOK {
				return false
			}
			owner.summary, normalizerOK = engine.SummaryReadForm(normalizer)
			if !normalizerOK {
				return false
			}
			owner.hasSummary = true
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

func (owner *Owner) Schema() *pack.Schema {
	if owner == nil {
		return nil
	}
	return owner.schema
}
func (owner *Owner) Output() engine.Output[pack.Value] {
	if owner == nil {
		return engine.Output[pack.Value]{}
	}
	return owner.output
}
func (owner *Owner) ExactRead() engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]] {
	if owner == nil {
		return engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]]{}
	}
	return owner.read
}
func (owner *Owner) SummaryRead() engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]] {
	if owner == nil {
		return engine.ReadForm[pack.Value, engine.OrderedCells[pack.Value]]{}
	}
	return owner.summary
}
func (owner *Owner) ExactWrite() engine.WriteForm[pack.Value] {
	if owner == nil {
		return engine.WriteForm[pack.Value]{}
	}
	return owner.write
}
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// NewSummaryRefs starts one immutable vector for this exact Pack Factor.
func (owner *Owner) NewSummaryRefs() *SummaryRefs {
	if owner == nil || owner.factor == nil || !owner.hasSummary {
		return nil
	}
	refs := owner.factor.NewClosedRefs()
	if refs == nil {
		return nil
	}
	return &SummaryRefs{owner: owner, refs: refs}
}

// AppendSummaryRoot admits one Schema-local root through the Factor's typed
// Ref issuer. No raw coordinate or root ordinal crosses the owner boundary.
func (owner *Owner) AppendSummaryRoot(refs *SummaryRefs, root pack.Root) bool {
	if owner == nil || owner.factor == nil || refs == nil || refs.owner != owner || refs.refs == nil || !owner.factor.OwnsClosedRefs(refs.refs) {
		return false
	}
	ref, ok := owner.Locate(root)
	return ok && refs.refs.Append(ref)
}

func (owner *Owner) CloseSummaryRefs(refs *SummaryRefs) bool {
	return owner != nil && owner.factor != nil && refs != nil && refs.owner == owner && refs.refs != nil && owner.factor.OwnsClosedRefs(refs.refs) && refs.refs.Close()
}

func InstanceSummaryRead[V, O any](owner *Owner, binding *engine.RuleBinding[V, O], read engine.Read[engine.OrderedCells[pack.Value]], refs *SummaryRefs) bool {
	return owner != nil && owner.factor != nil && owner.hasSummary && refs != nil && refs.owner == owner && refs.refs != nil && owner.factor.OwnsClosedRefs(refs.refs) && engine.InstanceSummaryRead(binding, read, owner.summary, refs.refs)
}

func DerivationReadMatchesSummaryRefs[V, O any](owner *Owner, derivation engine.RuleDerivation[V, O], read engine.Read[engine.OrderedCells[pack.Value]], refs *SummaryRefs) bool {
	return owner != nil && owner.factor != nil && owner.hasSummary && refs != nil && refs.owner == owner && refs.refs != nil && owner.factor.OwnsClosedRefs(refs.refs) && engine.DerivationReadMatchesSummaryRefs(derivation, read, refs.refs)
}

// Locate issues Pack's exact Wave-E coordinate capability for one sealed
// Schema-local root. The immutable RootAt-derived map rejects duplicates at
// declaration time and foreign roots at lookup; Factor owns the
// sealed-composition fence.
func (owner *Owner) Locate(root pack.Root) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.schema == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.coordinates[root]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

// AddActivationPortRead binds one exact Pack root to one source-owned
// activation import slot. The Factor remains owner-private; the engine still
// authenticates the issued Ref against this exact issuer.
func (owner *Owner) AddActivationPortRead(port *engine.ActivationPort, slot engine.SemanticKey, ref engine.Ref[coordinate]) bool {
	return owner != nil && owner.factor != nil && engine.AddActivationPortRead(port, slot, owner.factor, ref)
}

func (owner *Owner) rootAt(index coordinate) (pack.Root, bool) {
	if owner == nil || owner.schema == nil || uint64(index) >= uint64(owner.schema.RootCount()) {
		return pack.Root{}, false
	}
	return owner.schema.RootAt(int(index))
}

// rootCoordinates freezes Schema's canonical RootAt order into Owner-local
// declaration coordinates. No subsequent code mutates this map.
func rootCoordinates(schema *pack.Schema) (map[pack.Root]coordinate, bool) {
	if schema == nil || !validCount(schema.RootCount()) {
		return nil, false
	}
	coordinates := make(map[pack.Root]coordinate, schema.RootCount())
	for index := 0; index < schema.RootCount(); index++ {
		root, ok := schema.RootAt(index)
		if !ok {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := coordinates[root]; duplicate {
			return nil, false
		}
		coordinates[root] = coordinate
	}
	return coordinates, true
}
func (owner *Owner) admits(index coordinate, fact pack.Value) bool {
	root, ok := owner.rootAt(index)
	return ok && owner.schema.Admit(root, fact)
}
func (owner *Owner) widenRank(index coordinate, fact pack.Value, component int) uint64 {
	root, ok := owner.rootAt(index)
	if !ok {
		return 0
	}
	return owner.schema.At(root, fact, component)
}
func validCount(count int) bool { return count >= 0 && uint64(count) <= uint64(^uint32(0))+1 }
