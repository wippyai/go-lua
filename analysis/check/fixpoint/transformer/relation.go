package transformer

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// DescriptorKind names a specialization handler, not a Summary field schema.
type DescriptorKind string

const (
	DescriptorReturn     DescriptorKind = "Returns"
	DescriptorObligation DescriptorKind = "ParamObligations"
)

// Operation is one correlated row output.
type Operation struct {
	Kind       OutputKind
	Descriptor DescriptorKind
	Slot       uint32
	Value      ValueTerm
}

// Relation is immutable reduced boundary code. It is not a lattice element;
// the evaluator's bound application cells own all semantic joins, widening,
// and narrowing.
type Relation struct {
	shape                   Shape
	arena                   *Arena
	effects                 *EffectArena
	descriptors             *DescriptorRegistry
	authority               *relationOutputAuthority
	code                    *relationCode
	root                    relationRootRef
	inferReturnCorrelations bool
	contextual              string
	widened                 bool
	observationComplete     bool
	paramContracts          []product.Value
	projection              relationProjection
	annotations             relationAnnotations
}

// relationProjection carries source-structural summary metadata whose legacy
// meaning is independent of row feasibility. It is attached after semantic
// rows are evaluated, so syntactic may-alias facts cannot invalidate the
// separate must-preservation proof used by path refinements.
type relationProjection struct {
	returnParamPathAliases []summary.ReturnParamPathAlias
}

func normalizeRelationProjection(reg *axis.Registry, aliases []summary.ReturnParamPathAlias) relationProjection {
	normalized := summary.Normalize(reg, summary.Summary{ReturnParamPathAliases: append([]summary.ReturnParamPathAlias(nil), aliases...)})
	return relationProjection{returnParamPathAliases: normalized.ReturnParamPathAliases}
}

func equalRelationProjection(reg *axis.Registry, left, right relationProjection) bool {
	return summary.EqualNormalized(reg,
		summary.Summary{ReturnParamPathAliases: left.returnParamPathAliases},
		summary.Summary{ReturnParamPathAliases: right.returnParamPathAliases},
	)
}

// relationOutputAuthority is the immutable capability snapshot carried by a
// built relation. Resolver callbacks are deliberately outside the builder, so
// specialization must re-check their fragments against each originating
// effect descriptor. Row-owned outputs use OutputCapabilityRegistry instead.
type relationOutputAuthority struct {
	effects map[EffectKind]map[callboundary.BoundaryFactKind]struct{}
	summary map[callboundary.BoundaryFactKind]struct{}
}

func snapshotRelationOutputAuthority(catalog *EffectCatalog, caps *OutputCapabilityRegistry) *relationOutputAuthority {
	if catalog == nil || caps == nil {
		return nil
	}
	out := &relationOutputAuthority{
		effects: make(map[EffectKind]map[callboundary.BoundaryFactKind]struct{}),
		summary: make(map[callboundary.BoundaryFactKind]struct{}),
	}
	for _, kind := range caps.SummaryKinds() {
		if len(caps.UnsupportedSummary(kind)) == 0 {
			out.summary[kind] = struct{}{}
		}
	}
	for kind := EffectInvalidatePath; kind < effectKindCount; kind++ {
		descriptor, ok := catalog.Descriptor(kind)
		if !ok {
			continue
		}
		allowed := make(map[callboundary.BoundaryFactKind]struct{})
		for _, boundaryKind := range descriptor.BoundaryKinds() {
			allowed[boundaryKind] = struct{}{}
		}
		out.effects[kind] = allowed
	}
	return out
}

func equalRelationOutputAuthority(a, b *relationOutputAuthority) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.effects) != len(b.effects) || len(a.summary) != len(b.summary) {
		return false
	}
	for kind := range a.summary {
		if _, ok := b.summary[kind]; !ok {
			return false
		}
	}
	for effectKind, allowed := range a.effects {
		other, ok := b.effects[effectKind]
		if !ok || len(allowed) != len(other) {
			return false
		}
		for kind := range allowed {
			if _, ok := other[kind]; !ok {
				return false
			}
		}
	}
	return true
}

func (a *relationOutputAuthority) allowsSummary(kind callboundary.BoundaryFactKind) bool {
	if a == nil {
		return false
	}
	_, ok := a.summary[kind]
	return ok
}

func (r Relation) Shape() Shape                      { return r.shape }
func (r Relation) ContextualReason() string          { return r.contextual }
func (r Relation) Widened() bool                     { return r.widened }
func (r Relation) ObservationCoverageComplete() bool { return r.observationComplete }

// IsBottom reports the owned least element of one relation cell. Ownership is
// essential: the zero Go value has neither an arena nor a boundary identity and
// must never be accepted as a resolved dependency. Row metadata is deliberately
// irrelevant because a freshly initialized SCC cell and a compiled relation
// with no feasible exits denote the same semantic Bottom.
func (r Relation) IsBottom() bool {
	return r.arena != nil && r.arena.reg != nil && r.contextual == "" && !r.widened && r.root == 0 && r.code == nil
}

// Builder performs atomic validation before publishing an immutable relation.
type Builder struct {
	shape                   Shape
	arena                   *Arena
	effects                 *EffectArena
	effectCatalog           *EffectCatalog
	caps                    *OutputCapabilityRegistry
	descriptors             *DescriptorRegistry
	plan                    *operationplan.Plan
	inferReturnCorrelations bool
}

func NewBuilder(reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry, plan *operationplan.Plan) *Builder {
	return NewBuilderWithDescriptors(reg, shape, caps, DefaultDescriptorRegistry(), plan)
}

// NewBuilderWithDescriptors binds admission to the same descriptor handlers
// that will specialize the relation. This prevents a capability-only promise
// from publishing an output for which no Summary transaction exists.
func NewBuilderWithDescriptors(reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry, descriptors *DescriptorRegistry, plan *operationplan.Plan) *Builder {
	if caps == nil {
		caps = DefaultOutputCapabilityRegistry()
	}
	if descriptors == nil {
		descriptors = DefaultDescriptorRegistry()
	}
	arena := NewArena(reg)
	return &Builder{
		shape: shape, arena: arena, effects: NewEffectArena(arena),
		effectCatalog: DefaultEffectCatalog(), caps: caps, descriptors: descriptors, plan: plan,
	}
}
func (b *Builder) Arena() *Arena             { return b.arena }
func (b *Builder) EffectArena() *EffectArena { return b.effects }

func (b *Builder) bottomRelation() Relation {
	if b == nil || b.arena == nil {
		return Relation{}
	}
	var contracts []product.Value
	if b.plan != nil {
		contracts = append([]product.Value(nil), b.plan.BoundaryParamContracts()...)
	}
	return Relation{
		shape: b.shape, arena: b.arena, effects: b.effects, descriptors: b.descriptors,
		authority:               snapshotRelationOutputAuthority(b.effectCatalog, b.caps),
		inferReturnCorrelations: b.inferReturnCorrelations,
		observationComplete:     true,
		paramContracts:          contracts,
		projection:              normalizeRelationProjection(b.arena.reg, nil),
	}
}

func stateCatalog() state.LaneCatalog { return state.DefaultLaneCatalog() }

func (a *Arena) canonicalGuard(g Guard) string {
	if g == 0 || int(g) >= len(a.guards) {
		return "?"
	}
	n := a.guards[g]
	switch n.op {
	case guardTrue:
		return "T"
	case guardFalse:
		return "F"
	case guardTruthy:
		return "t(" + a.canonicalValue(n.value) + ")"
	case guardFalsy:
		return "f(" + a.canonicalValue(n.value) + ")"
	case guardAnd, guardOr:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalGuard(x)
		}
		sort.Strings(parts)
		prefix := "&"
		if n.op == guardOr {
			prefix = "|"
		}
		return prefix + "(" + strings.Join(parts, ",") + ")"
	default:
		return "?"
	}
}

func equalParamContracts(reg *axis.Registry, left, right []product.Value) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !product.Equal(reg, left[i], right[i]) {
			return false
		}
	}
	return true
}
