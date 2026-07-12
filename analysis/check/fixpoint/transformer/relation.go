package transformer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
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

// Row is a may-alternative. Its outputs remain correlated under Guard and are
// never independently widened.
type Row struct {
	Guard   Guard
	Output  summary.Summary
	Ops     []Operation
	Effects []EffectTerm
}

// Relation is immutable. contextual is lattice Top: callers must use the
// existing per-context solver and no partial transformer result may publish.
type Relation struct {
	shape                   Shape
	arena                   *Arena
	effects                 *EffectArena
	descriptors             *DescriptorRegistry
	authority               *relationOutputAuthority
	rows                    []Row
	inferReturnCorrelations bool
	contextual              string
	widened                 bool
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

func (r Relation) Shape() Shape             { return r.shape }
func (r Relation) ContextualReason() string { return r.contextual }
func (r Relation) Widened() bool            { return r.widened }
func (r Relation) Rows() int                { return len(r.rows) }

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

func (b *Builder) Build(certificate SemanticCertificate, rows []Row) (Relation, error) {
	if b == nil || b.arena == nil || b.arena.reg == nil {
		return Relation{}, fmt.Errorf("transformer: nil builder registry")
	}
	if b.plan == nil {
		return Relation{}, fmt.Errorf("transformer: builder has no operation plan provenance")
	}
	if certificate.plan != b.plan {
		return Relation{}, fmt.Errorf("transformer: semantic capability certificate does not match builder operation plan")
	}
	if err := b.caps.Complete(stateCatalog()); err != nil {
		return Relation{}, err
	}
	owned := make([]Row, len(rows))
	for i, row := range rows {
		if row.Guard == 0 || int(row.Guard) >= len(b.arena.guards) {
			return Relation{}, fmt.Errorf("transformer: row %d has invalid guard", i)
		}
		owned[i] = Row{
			Guard:   row.Guard,
			Output:  summary.Normalize(b.arena.reg, row.Output),
			Ops:     append([]Operation(nil), row.Ops...),
			Effects: append([]EffectTerm(nil), row.Effects...),
		}
		if !b.arena.validGuard(row.Guard, b.shape) {
			return Relation{}, fmt.Errorf("transformer: row %d references an invalid boundary root", i)
		}
		if b.arena.guardContainsCellResult(row.Guard, make(map[Guard]bool)) {
			return Relation{}, fmt.Errorf("transformer: row %d uses a scalar cell result as a guard; relational composition required", i)
		}
		for _, kind := range summary.PresentFactKinds(owned[i].Output) {
			if unsupported := b.caps.UnsupportedSummary(kind); len(unsupported) != 0 {
				return Relation{}, fmt.Errorf("transformer: structured Summary output %q unsupported on lanes %v", kind, unsupported)
			}
		}
		if row.Guard != b.arena.True() && len(owned[i].Output.ParamObligations) != 0 {
			return Relation{}, fmt.Errorf("transformer: conditional Summary output %q requires contextual solver", DescriptorObligation)
		}
		for j, op := range owned[i].Ops {
			if op.Kind >= outputKindCount || op.Value == 0 || int(op.Value) >= len(b.arena.values) {
				return Relation{}, fmt.Errorf("transformer: row %d operation %d is invalid", i, j)
			}
			if !b.arena.validValue(op.Value, b.shape, make(map[ValueTerm]bool)) {
				return Relation{}, fmt.Errorf("transformer: row %d operation %d references an invalid boundary root", i, j)
			}
			hasCellResult := b.arena.containsCellResult(op.Value, make(map[ValueTerm]bool))
			if hasCellResult != (op.Kind == OutputCellResult) {
				return Relation{}, fmt.Errorf("transformer: row %d operation %d scalar cell result/output kind mismatch", i, j)
			}
			if unsupported := b.caps.Unsupported(op.Kind); len(unsupported) != 0 {
				return Relation{}, fmt.Errorf("transformer: %v unsupported on lanes %v", op.Kind, unsupported)
			}
			summaryKind := callboundary.BoundaryFactKind(op.Descriptor)
			if unsupported := b.caps.UnsupportedSummary(summaryKind); len(unsupported) != 0 {
				return Relation{}, fmt.Errorf("transformer: Summary output %q unsupported on lanes %v", summaryKind, unsupported)
			}
			handler := b.descriptors.handlers[op.Descriptor]
			if handler == nil {
				return Relation{}, fmt.Errorf("transformer: Summary output %q has no transaction handler", summaryKind)
			}
			if row.Guard != b.arena.True() && !handler.ConditionalAllowed() {
				return Relation{}, fmt.Errorf("transformer: conditional Summary output %q requires contextual solver", summaryKind)
			}
		}
		for j, effect := range owned[i].Effects {
			if b.effects == nil || !b.effects.Valid(effect, b.shape) {
				return Relation{}, fmt.Errorf("transformer: row %d effect %d is invalid", i, j)
			}
			kind := b.effects.Kind(effect)
			descriptor, admitted := b.effectCatalog.Descriptor(kind)
			if !admitted {
				return Relation{}, fmt.Errorf("transformer: row %d effect %d kind %d is not catalog-admitted", i, j, kind)
			}
			for _, lane := range b.effectCatalog.Lanes() {
				if descriptor.LaneUse(lane) == LaneUseUnsupported {
					return Relation{}, fmt.Errorf("transformer: row %d effect %d kind %d unsupported on lane %q", i, j, kind, lane)
				}
			}
		}
		sort.Slice(owned[i].Ops, func(x, y int) bool { return operationLess(b.arena, owned[i].Ops[x], owned[i].Ops[y]) })
	}
	sort.Slice(owned, func(i, j int) bool { return rowLess(b.arena, b.effects, owned[i], owned[j]) })
	owned = dedupRows(b.arena, b.effects, owned)
	return Relation{
		shape: b.shape, arena: b.arena, effects: b.effects, descriptors: b.descriptors,
		authority: snapshotRelationOutputAuthority(b.effectCatalog, b.caps), rows: owned,
		inferReturnCorrelations: b.inferReturnCorrelations,
	}, nil
}

func stateCatalog() state.LaneCatalog { return state.DefaultLaneCatalog() }

func operationKey(a *Arena, op Operation) string {
	return fmt.Sprintf("%d:%s:%d:%s", op.Kind, op.Descriptor, op.Slot, a.canonicalValue(op.Value))
}
func operationLess(a *Arena, left, right Operation) bool {
	lk, rk := operationKey(a, left), operationKey(a, right)
	if lk != rk {
		return lk < rk
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Descriptor != right.Descriptor {
		return left.Descriptor < right.Descriptor
	}
	if left.Slot != right.Slot {
		return left.Slot < right.Slot
	}
	return left.Value < right.Value
}
func rowKey(a *Arena, effects *EffectArena, row Row) string {
	parts := make([]string, len(row.Ops))
	for i, op := range row.Ops {
		parts[i] = operationKey(a, op)
	}
	effectKeys := make([]string, len(row.Effects))
	for i, term := range row.Effects {
		if effects == nil || term == 0 || int(term) >= len(effects.nodes) {
			effectKeys[i] = "?"
		} else {
			effectKeys[i] = effects.canonical(effects.nodes[term])
		}
	}
	return fmt.Sprintf("%s|%016x|%s|%s", a.canonicalGuard(row.Guard), uint64(summary.NormalizedPayloadDigest(a.reg, row.Output)), strings.Join(parts, ";"), strings.Join(effectKeys, ";"))
}
func rowLess(a *Arena, effects *EffectArena, left, right Row) bool {
	lk, rk := rowKey(a, effects, left), rowKey(a, effects, right)
	if lk != rk {
		return lk < rk
	}
	if left.Guard != right.Guard {
		return left.Guard < right.Guard
	}
	for i := 0; i < len(left.Ops) && i < len(right.Ops); i++ {
		if operationEqual(left.Ops[i], right.Ops[i]) {
			continue
		}
		return operationLess(a, left.Ops[i], right.Ops[i])
	}
	return len(left.Ops) < len(right.Ops)
}
func operationEqual(a, b Operation) bool { return a == b }
func rowEqual(arena *Arena, a, b Row) bool {
	if a.Guard != b.Guard || len(a.Ops) != len(b.Ops) || len(a.Effects) != len(b.Effects) || !summary.EqualNormalized(arena.reg, a.Output, b.Output) {
		return false
	}
	for i := range a.Ops {
		if !operationEqual(a.Ops[i], b.Ops[i]) {
			return false
		}
	}
	for i := range a.Effects {
		if a.Effects[i] != b.Effects[i] {
			return false
		}
	}
	return true
}
func dedupRows(a *Arena, effects *EffectArena, rows []Row) []Row {
	if len(rows) == 0 {
		return nil
	}
	out := rows[:0]
	for _, row := range rows {
		duplicate := false
		key := rowKey(a, effects, row)
		for i := len(out) - 1; i >= 0 && rowKey(a, effects, out[i]) == key; i-- {
			if rowEqual(a, out[i], row) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, row)
		}
	}
	return out
}

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

// JoinRelation is may-union. Relations from distinct arenas are deliberately
// incompatible: translating DAG ownership implicitly would break identity.
func JoinRelation(a, b Relation) Relation {
	if a.arena == nil {
		return b
	}
	if b.arena == nil {
		return a
	}
	// Relation SCC cells start at the empty relation with arena/shape ownership
	// but no row-owned effect arena yet. The first non-empty equation result
	// establishes that immutable effect identity; this is Bottom adoption, not
	// cross-arena composition.
	if a.effects == nil && a.contextual == "" && len(a.rows) == 0 {
		a.effects = b.effects
		a.authority = b.authority
		a.descriptors = b.descriptors
		a.inferReturnCorrelations = b.inferReturnCorrelations
	}
	if b.effects == nil && b.contextual == "" && len(b.rows) == 0 {
		b.effects = a.effects
		b.authority = a.authority
		b.descriptors = a.descriptors
		b.inferReturnCorrelations = a.inferReturnCorrelations
	}
	if a.contextual != "" || b.contextual != "" || a.arena != b.arena || a.effects != b.effects || a.descriptors != b.descriptors || a.shape != b.shape || a.inferReturnCorrelations != b.inferReturnCorrelations || !equalRelationOutputAuthority(a.authority, b.authority) {
		return Relation{shape: a.shape, arena: a.arena, effects: a.effects, contextual: "incompatible or contextual relation"}
	}
	rows := append(cloneRows(a.rows), b.rows...)
	sort.Slice(rows, func(i, j int) bool { return rowLess(a.arena, a.effects, rows[i], rows[j]) })
	rows = dedupRows(a.arena, a.effects, rows)
	return Relation{shape: a.shape, arena: a.arena, effects: a.effects, descriptors: a.descriptors, authority: a.authority, rows: rows, inferReturnCorrelations: a.inferReturnCorrelations, widened: a.widened || b.widened}
}

// WidenRelation preserves correlation. Budget overflow becomes contextual Top
// instead of independently collapsing guarded outputs.
func WidenRelation(prev, next Relation, maxRows int) Relation {
	out := JoinRelation(prev, next)
	if out.contextual == "" && maxRows > 0 && len(out.rows) > maxRows {
		out.rows = nil
		out.contextual = "row budget"
		out.widened = true
	}
	return out
}
func EqualRelation(a, b Relation) bool {
	if a.arena == nil || b.arena == nil {
		return a.arena == nil && b.arena == nil
	}
	if a.contextual != "" || b.contextual != "" {
		return a.contextual != "" && b.contextual != ""
	}
	if a.arena != b.arena || a.effects != b.effects || a.descriptors != b.descriptors || a.shape != b.shape || a.inferReturnCorrelations != b.inferReturnCorrelations || !equalRelationOutputAuthority(a.authority, b.authority) || len(a.rows) != len(b.rows) {
		return false
	}
	used := make([]bool, len(b.rows))
	for _, left := range a.rows {
		found := false
		for j, right := range b.rows {
			if !used[j] && rowEqual(a.arena, left, right) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func LessOrEqRelation(a, b Relation) bool {
	if b.contextual != "" {
		return true
	}
	if a.contextual != "" {
		return b.contextual != ""
	}
	return EqualRelation(JoinRelation(a, b), b)
}
func cloneRows(rows []Row) []Row {
	out := make([]Row, len(rows))
	for i, row := range rows {
		out[i] = Row{Guard: row.Guard, Output: row.Output.Clone(), Ops: append([]Operation(nil), row.Ops...), Effects: append([]EffectTerm(nil), row.Effects...)}
	}
	return out
}
