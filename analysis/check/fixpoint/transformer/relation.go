package transformer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
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
	Guard Guard
	Ops   []Operation
}

// Relation is immutable. contextual is lattice Top: callers must use the
// existing per-context solver and no partial transformer result may publish.
type Relation struct {
	shape      Shape
	arena      *Arena
	rows       []Row
	contextual string
	widened    bool
}

func (r Relation) Shape() Shape             { return r.shape }
func (r Relation) ContextualReason() string { return r.contextual }
func (r Relation) Widened() bool            { return r.widened }
func (r Relation) Rows() int                { return len(r.rows) }

// Builder performs atomic validation before publishing an immutable relation.
type Builder struct {
	shape       Shape
	arena       *Arena
	caps        *OutputCapabilityRegistry
	descriptors *DescriptorRegistry
}

func NewBuilder(reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry) *Builder {
	return NewBuilderWithDescriptors(reg, shape, caps, DefaultDescriptorRegistry())
}

// NewBuilderWithDescriptors binds admission to the same descriptor handlers
// that will specialize the relation. This prevents a capability-only promise
// from publishing an output for which no Summary transaction exists.
func NewBuilderWithDescriptors(reg *axis.Registry, shape Shape, caps *OutputCapabilityRegistry, descriptors *DescriptorRegistry) *Builder {
	if caps == nil {
		caps = DefaultOutputCapabilityRegistry()
	}
	if descriptors == nil {
		descriptors = DefaultDescriptorRegistry()
	}
	return &Builder{shape: shape, arena: NewArena(reg), caps: caps, descriptors: descriptors}
}
func (b *Builder) Arena() *Arena { return b.arena }

func (b *Builder) Build(certificate SemanticCertificate, rows []Row) (Relation, error) {
	if b == nil || b.arena == nil || b.arena.reg == nil {
		return Relation{}, fmt.Errorf("transformer: nil builder registry")
	}
	if certificate.plan == nil {
		return Relation{}, fmt.Errorf("transformer: missing semantic capability certificate")
	}
	if err := b.caps.Complete(stateCatalog()); err != nil {
		return Relation{}, err
	}
	owned := make([]Row, len(rows))
	for i, row := range rows {
		if row.Guard == 0 || int(row.Guard) >= len(b.arena.guards) {
			return Relation{}, fmt.Errorf("transformer: row %d has invalid guard", i)
		}
		owned[i] = Row{Guard: row.Guard, Ops: append([]Operation(nil), row.Ops...)}
		if !b.arena.validGuard(row.Guard, b.shape) {
			return Relation{}, fmt.Errorf("transformer: row %d references an invalid boundary root", i)
		}
		if b.arena.guardContainsCellResult(row.Guard, make(map[Guard]bool)) {
			return Relation{}, fmt.Errorf("transformer: row %d uses a scalar cell result as a guard; relational composition required", i)
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
		sort.Slice(owned[i].Ops, func(x, y int) bool { return operationLess(b.arena, owned[i].Ops[x], owned[i].Ops[y]) })
	}
	sort.Slice(owned, func(i, j int) bool { return rowLess(b.arena, owned[i], owned[j]) })
	owned = dedupRows(b.arena, owned)
	return Relation{shape: b.shape, arena: b.arena, rows: owned}, nil
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
func rowKey(a *Arena, row Row) string {
	parts := make([]string, len(row.Ops))
	for i, op := range row.Ops {
		parts[i] = operationKey(a, op)
	}
	return fmt.Sprintf("%s|%s", a.canonicalGuard(row.Guard), strings.Join(parts, ";"))
}
func rowLess(a *Arena, left, right Row) bool {
	lk, rk := rowKey(a, left), rowKey(a, right)
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
func rowEqual(a, b Row) bool {
	if a.Guard != b.Guard || len(a.Ops) != len(b.Ops) {
		return false
	}
	for i := range a.Ops {
		if !operationEqual(a.Ops[i], b.Ops[i]) {
			return false
		}
	}
	return true
}
func dedupRows(a *Arena, rows []Row) []Row {
	if len(rows) == 0 {
		return nil
	}
	out := rows[:0]
	for _, row := range rows {
		duplicate := false
		key := rowKey(a, row)
		for i := len(out) - 1; i >= 0 && rowKey(a, out[i]) == key; i-- {
			if rowEqual(out[i], row) {
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
	if a.contextual != "" || b.contextual != "" || a.arena != b.arena || a.shape != b.shape {
		return Relation{shape: a.shape, arena: a.arena, contextual: "incompatible or contextual relation"}
	}
	rows := append(cloneRows(a.rows), b.rows...)
	sort.Slice(rows, func(i, j int) bool { return rowLess(a.arena, rows[i], rows[j]) })
	rows = dedupRows(a.arena, rows)
	return Relation{shape: a.shape, arena: a.arena, rows: rows, widened: a.widened || b.widened}
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
	if a.arena != b.arena || a.shape != b.shape || len(a.rows) != len(b.rows) {
		return false
	}
	used := make([]bool, len(b.rows))
	for _, left := range a.rows {
		found := false
		for j, right := range b.rows {
			if !used[j] && rowEqual(left, right) {
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
		out[i] = Row{Guard: row.Guard, Ops: append([]Operation(nil), row.Ops...)}
	}
	return out
}
