package symboliccall

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
)

// RootKind keeps caller parameters, closure captures, and vararg elements in
// separate namespaces. Numeric index equality never aliases two root kinds.
type RootKind uint8

const (
	RootParam RootKind = iota + 1
	RootCapture
	RootVararg
)

type Root struct {
	Kind  RootKind
	Index int
}

type RootGuard struct {
	Root Root
	Kind GuardKind
}

// VarargLength is an inclusive exact/ranged pack-length guard. Max < 0 means
// unbounded. It describes the pack as a unit, preserving length/value
// correlation instead of joining every positional value independently.
type VarargLength struct {
	Min int
	Max int
}

func ExactVarargLength(n int) VarargLength { return VarargLength{Min: n, Max: n} }

type BoundaryRow struct {
	Guards       []RootGuard
	VarargLength VarargLength
	Returns      []Expr
}

// BoundaryRequirement is a contravariant entry obligation: for a feasible
// guard, the concrete root must be <= Allowed. Alternative control paths join
// obligations by Meet, never Join.
type BoundaryRequirement struct {
	Guards       []RootGuard
	VarargLength VarargLength
	Root         Root
	Allowed      product.Value
}

type BoundaryPolicy struct {
	HeapMutatedCaptures bool
	Allocation          bool
	ActorState          bool
}

type BoundaryTransformer struct {
	reg          *axis.Registry
	params       int
	captures     int
	rows         []BoundaryRow
	requirements []BoundaryRequirement
	valid        bool
	contextual   string
}

func NewBoundaryTransformer(reg *axis.Registry, params, captures int, rows []BoundaryRow, requirements []BoundaryRequirement, policy BoundaryPolicy) BoundaryTransformer {
	t := BoundaryTransformer{reg: reg, params: params, captures: captures, rows: cloneBoundaryRows(rows), requirements: cloneBoundaryRequirements(requirements), valid: true}
	switch {
	case reg == nil || params < 0 || captures < 0:
		t.contextual = "invalid boundary transformer"
	case policy.ActorState:
		t.contextual = "actor state"
	case policy.HeapMutatedCaptures:
		t.contextual = "heap-mutated capture"
	case policy.Allocation:
		t.contextual = "allocation identity"
	}
	return normalizeBoundary(t)
}

func (t BoundaryTransformer) ContextualReason() string { return t.contextual }

// Instantiate binds the lexical closure environment and exact incoming vararg
// pack at the call boundary. Captures are values supplied by the closure, not
// parameters reconstructed from the caller's local namespace.
func (t BoundaryTransformer) Instantiate(params, captures, varargs []product.Value) ([][]product.Value, error) {
	if !t.valid || t.contextual != "" {
		return nil, fmt.Errorf("symboliccall: contextual boundary transformer: %s", t.contextual)
	}
	if len(params) != t.params || len(captures) != t.captures {
		return nil, fmt.Errorf("symboliccall: boundary shape got params=%d captures=%d, want %d/%d", len(params), len(captures), t.params, t.captures)
	}
	for _, requirement := range t.requirements {
		if !boundaryConditionMayHold(t.reg, requirement.Guards, requirement.VarargLength, params, captures, varargs) {
			continue
		}
		actual, ok := readRoot(t.reg, requirement.Root, params, captures, varargs)
		if !ok || !product.LessOrEq(t.reg, actual, requirement.Allowed) {
			return nil, fmt.Errorf("symboliccall: entry requirement rejected %s", rootKey(requirement.Root))
		}
	}
	width := 0
	for _, row := range t.rows {
		if len(row.Returns) > width {
			width = len(row.Returns)
		}
	}
	var out [][]product.Value
	for _, row := range t.rows {
		if !boundaryConditionMayHold(t.reg, row.Guards, row.VarargLength, params, captures, varargs) {
			continue
		}
		values := make([]product.Value, width)
		for i := range values {
			values[i] = product.Absent(t.reg)
		}
		for i, expr := range row.Returns {
			value, err := evalBoundary(t.reg, expr, params, captures, varargs)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		out = append(out, values)
	}
	return out, nil
}

func JoinBoundary(a, b BoundaryTransformer) BoundaryTransformer {
	if !a.valid {
		return b
	}
	if !b.valid {
		return a
	}
	if a.contextual != "" || b.contextual != "" {
		reason := a.contextual
		if reason == "" || b.contextual != "" && b.contextual < reason {
			reason = b.contextual
		}
		return BoundaryTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true, contextual: reason}
	}
	if a.reg != b.reg || a.params != b.params || a.captures != b.captures {
		return BoundaryTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true, contextual: "incompatible boundary shape"}
	}
	out := BoundaryTransformer{reg: a.reg, params: a.params, captures: a.captures, valid: true}
	out.rows = append(cloneBoundaryRows(a.rows), cloneBoundaryRows(b.rows)...)
	out.requirements = cloneBoundaryRequirements(a.requirements)
	for _, incoming := range b.requirements {
		matched := false
		for i := range out.requirements {
			if boundaryRequirementKey(out.requirements[i]) == boundaryRequirementKey(incoming) {
				out.requirements[i].Allowed = product.Meet(a.reg, out.requirements[i].Allowed, incoming.Allowed)
				if product.Equal(a.reg, out.requirements[i].Allowed, product.Bottom(a.reg)) {
					out.contextual = "contradictory boundary requirement"
				}
				matched = true
				break
			}
		}
		if !matched {
			out.requirements = append(out.requirements, incoming)
		}
	}
	return normalizeBoundary(out)
}

func EqualBoundary(a, b BoundaryTransformer) bool {
	a, b = normalizeBoundary(a), normalizeBoundary(b)
	if a.valid != b.valid || a.contextual != b.contextual || a.reg != b.reg || a.params != b.params || a.captures != b.captures || len(a.rows) != len(b.rows) || len(a.requirements) != len(b.requirements) {
		return false
	}
	for i := range a.rows {
		if boundaryRowKey(a.reg, a.rows[i]) != boundaryRowKey(b.reg, b.rows[i]) || !exprSliceEqual(a.rows[i].Returns, b.rows[i].Returns) {
			return false
		}
	}
	for i := range a.requirements {
		if boundaryRequirementKey(a.requirements[i]) != boundaryRequirementKey(b.requirements[i]) || !product.Equal(a.reg, a.requirements[i].Allowed, b.requirements[i].Allowed) {
			return false
		}
	}
	return true
}

func LessOrEqBoundary(a, b BoundaryTransformer) bool { return EqualBoundary(JoinBoundary(a, b), b) }

func normalizeBoundary(t BoundaryTransformer) BoundaryTransformer {
	if !t.valid || t.contextual != "" || t.reg == nil {
		return t
	}
	for _, row := range t.rows {
		if !validBoundaryCondition(row.Guards, row.VarargLength, t.params, t.captures) {
			t.contextual = "invalid boundary row"
			return t
		}
		if exprConstantCollision(t.reg, row.Returns) {
			t.contextual = "constant canonical hash collision"
			return t
		}
	}
	// Clone before canonicalization so normalization never mutates an immutable
	// transformer's caller-owned backing slices.
	t.rows = cloneBoundaryRows(t.rows)
	t.requirements = cloneBoundaryRequirements(t.requirements)
	for i := range t.rows {
		for j := range t.rows[i].Returns {
			t.rows[i].Returns[j] = canonicalizeBoundaryExpr(t.reg, t.rows[i].Returns[j])
		}
	}
	for _, req := range t.requirements {
		if !validRoot(req.Root, t.params, t.captures) || !validBoundaryCondition(req.Guards, req.VarargLength, t.params, t.captures) {
			t.contextual = "invalid boundary requirement"
			return t
		}
		if product.Equal(t.reg, req.Allowed, product.Bottom(t.reg)) {
			t.contextual = "contradictory boundary requirement"
			return t
		}
	}
	for i := range t.rows {
		sortRootGuards(t.rows[i].Guards)
	}
	for i := range t.requirements {
		sortRootGuards(t.requirements[i].Guards)
	}
	sort.Slice(t.rows, func(i, j int) bool { return boundaryRowKey(t.reg, t.rows[i]) < boundaryRowKey(t.reg, t.rows[j]) })
	rows := t.rows[:0]
	for _, row := range t.rows {
		if len(rows) == 0 || boundaryRowKey(t.reg, rows[len(rows)-1]) != boundaryRowKey(t.reg, row) || !exprSliceEqual(rows[len(rows)-1].Returns, row.Returns) {
			rows = append(rows, row)
		}
	}
	t.rows = rows
	sort.Slice(t.requirements, func(i, j int) bool {
		return boundaryRequirementKey(t.requirements[i]) < boundaryRequirementKey(t.requirements[j])
	})
	return t
}

func boundaryConditionMayHold(reg *axis.Registry, guards []RootGuard, length VarargLength, params, captures, varargs []product.Value) bool {
	if !lengthContains(length, len(varargs)) {
		return false
	}
	for _, guard := range guards {
		value, ok := readRoot(reg, guard.Root, params, captures, varargs)
		if !ok {
			return false
		}
		if guard.Kind == GuardTruthy && !valueref.CanBeTruthy(reg, value) || guard.Kind == GuardFalsy && !valueref.CanBeFalsy(reg, value) {
			return false
		}
	}
	return true
}

func readRoot(reg *axis.Registry, root Root, params, captures, varargs []product.Value) (product.Value, bool) {
	if root.Index < 0 {
		return product.Value{}, false
	}
	switch root.Kind {
	case RootParam:
		if root.Index >= len(params) {
			return product.Value{}, false
		}
		return params[root.Index], true
	case RootCapture:
		if root.Index >= len(captures) {
			return product.Value{}, false
		}
		return captures[root.Index], true
	case RootVararg:
		if root.Index >= len(varargs) {
			return product.Absent(reg), true
		}
		return varargs[root.Index], true
	default:
		return product.Value{}, false
	}
}

func validBoundaryCondition(guards []RootGuard, length VarargLength, params, captures int) bool {
	if length.Min < 0 || length.Max >= 0 && length.Max < length.Min {
		return false
	}
	seen := make(map[string]GuardKind)
	for _, guard := range guards {
		if !validRoot(guard.Root, params, captures) || guard.Kind != GuardTruthy && guard.Kind != GuardFalsy {
			return false
		}
		key := rootKey(guard.Root)
		if prior, ok := seen[key]; ok && prior != guard.Kind {
			return false
		}
		seen[key] = guard.Kind
	}
	return true
}

func validRoot(root Root, params, captures int) bool {
	if root.Index < 0 {
		return false
	}
	switch root.Kind {
	case RootParam:
		return root.Index < params
	case RootCapture:
		return root.Index < captures
	case RootVararg:
		return true
	default:
		return false
	}
}

func lengthContains(r VarargLength, n int) bool {
	return n >= r.Min && (r.Max < 0 || n <= r.Max)
}

func rootKey(r Root) string {
	return strconv.Itoa(int(r.Kind)) + ":" + strconv.Itoa(r.Index)
}

func sortRootGuards(g []RootGuard) {
	sort.Slice(g, func(i, j int) bool {
		if rootKey(g[i].Root) != rootKey(g[j].Root) {
			return rootKey(g[i].Root) < rootKey(g[j].Root)
		}
		return g[i].Kind < g[j].Kind
	})
}

func guardsKey(g []RootGuard) string {
	var b strings.Builder
	for _, x := range g {
		b.WriteString(rootKey(x.Root))
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(int(x.Kind)))
		b.WriteByte(';')
	}
	return b.String()
}

func lengthKey(r VarargLength) string {
	return strconv.Itoa(r.Min) + ":" + strconv.Itoa(r.Max)
}

func boundaryRowKey(reg *axis.Registry, r BoundaryRow) string {
	var b strings.Builder
	b.WriteString(guardsKey(r.Guards))
	b.WriteByte('|')
	b.WriteString(lengthKey(r.VarargLength))
	for _, e := range r.Returns {
		b.WriteByte('|')
		b.WriteString(exprCanonicalKey(reg, e))
	}
	return b.String()
}

func boundaryRequirementKey(r BoundaryRequirement) string {
	return guardsKey(r.Guards) + "|" + lengthKey(r.VarargLength) + "|" + rootKey(r.Root)
}

func cloneBoundaryRows(in []BoundaryRow) []BoundaryRow {
	out := make([]BoundaryRow, len(in))
	for i, r := range in {
		out[i] = r
		out[i].Guards = append([]RootGuard(nil), r.Guards...)
		out[i].Returns = cloneExprs(r.Returns)
	}
	return out
}

func cloneBoundaryRequirements(in []BoundaryRequirement) []BoundaryRequirement {
	out := make([]BoundaryRequirement, len(in))
	for i, r := range in {
		out[i] = r
		out[i].Guards = append([]RootGuard(nil), r.Guards...)
	}
	return out
}

func exprConstantCollision(reg *axis.Registry, exprs []Expr) bool {
	seen := make(map[uint64]product.Value)
	var visit func(Expr) bool
	visit = func(e Expr) bool {
		if e.n == nil {
			return false
		}
		if e.n.op == opConst {
			h := product.Hash(reg, e.n.value)
			if v, ok := seen[h]; ok && !product.Equal(reg, v, e.n.value) {
				return true
			}
			seen[h] = e.n.value
		}
		for _, arg := range e.n.args {
			if visit(arg) {
				return true
			}
		}
		return false
	}
	for _, e := range exprs {
		if visit(e) {
			return true
		}
	}
	return false
}

func canonicalizeBoundaryExpr(reg *axis.Registry, expr Expr) Expr {
	if expr.n == nil {
		return expr
	}
	switch expr.n.op {
	case opJoin:
		args := make([]Expr, len(expr.n.args))
		for i, arg := range expr.n.args {
			args[i] = canonicalizeBoundaryExpr(reg, arg)
		}
		sort.Slice(args, func(i, j int) bool {
			return exprCanonicalKey(reg, args[i]) < exprCanonicalKey(reg, args[j])
		})
		return Expr{n: &exprNode{op: opJoin, args: args}}
	case opCall:
		args := make([]Expr, len(expr.n.args))
		for i, arg := range expr.n.args {
			args[i] = canonicalizeBoundaryExpr(reg, arg)
		}
		return Expr{n: &exprNode{op: opCall, callee: expr.n.callee, slot: expr.n.slot, args: args}}
	default:
		return expr
	}
}
