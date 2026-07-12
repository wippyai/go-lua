package transformer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

type ValueTerm uint32
type PathTerm uint32
type Guard uint32

type valueOp uint8

const (
	valueInvalid valueOp = iota
	valueRoot
	valueConstant
	valueJoin
	valueRefinement
	valueCellResult
	valueDynamicRead
	valueDynamicTableRead
	valueIteratorProjection
)

// CellRef is a stable SCC equation reference. Generation is deliberately not
// present: generations belong to a solve transaction, not transformer identity.
type CellRef struct {
	Function uint64
	Slot     uint32
}

type valueNode struct {
	op            valueOp
	root          Root
	value         product.Value
	args          []ValueTerm
	cell          CellRef
	path          PathTerm
	iterator      iteration.Iterator
	variableIndex int
}

type pathNode struct {
	root     Root
	segments []segment.Segment
}

type guardOp uint8

const (
	guardInvalid guardOp = iota
	guardTrue
	guardFalse
	guardTruthy
	guardFalsy
	guardAnd
	guardOr
)

type guardNode struct {
	op    guardOp
	value ValueTerm
	args  []Guard
}

// Arena owns hash-consed immutable term DAGs for one build. Index zero is the
// invalid term and is never published as a semantic node.
type Arena struct {
	reg       *axis.Registry
	values    []valueNode
	paths     []pathNode
	guards    []guardNode
	valueKeys map[string][]ValueTerm
	pathKeys  map[string][]PathTerm
	guardKeys map[string][]Guard
}

func NewArena(reg *axis.Registry) *Arena {
	a := &Arena{reg: reg, values: []valueNode{{}}, paths: []pathNode{{}}, guards: []guardNode{{}}, valueKeys: make(map[string][]ValueTerm), pathKeys: make(map[string][]PathTerm), guardKeys: make(map[string][]Guard)}
	a.internGuard(guardNode{op: guardTrue})
	a.internGuard(guardNode{op: guardFalse})
	return a
}

func (a *Arena) Root(root Root) ValueTerm { return a.internValue(valueNode{op: valueRoot, root: root}) }
func (a *Arena) Constant(value product.Value) ValueTerm {
	return a.internValue(valueNode{op: valueConstant, value: value})
}

// RefineValue retains a positive canonical factflow constraint in the term
// DAG. Specialization calls the same scalar kernel as concrete factapply.
// Context-sensitive negation and falsy-absence refinements fail closed.
func (a *Arena) RefineValue(value ValueTerm, refinement factflow.ValueRefinement) (ValueTerm, bool) {
	if value == 0 || refinement.NegatedLiteral() || refinement.FalsyAbsent() {
		return 0, false
	}
	constraint, ok := refinement.Constraint()
	if !ok {
		return value, true
	}
	return a.refineConstraintValue(value, constraint), true
}

// refineConstraintValue is the infallible internal constructor used after a
// transaction has already validated positive-refinement shape.
func (a *Arena) refineConstraintValue(value ValueTerm, constraint product.Value) ValueTerm {
	return a.internValue(valueNode{op: valueRefinement, value: constraint, args: []ValueTerm{value}})
}

// JoinValue constructs a flattened, commutative and idempotent value join.
func (a *Arena) JoinValue(terms ...ValueTerm) ValueTerm {
	flat := make([]ValueTerm, 0, len(terms))
	for _, term := range terms {
		if term == 0 || int(term) >= len(a.values) {
			continue
		}
		if a.values[term].op == valueJoin {
			flat = append(flat, a.values[term].args...)
		} else {
			flat = append(flat, term)
		}
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i] < flat[j] })
	flat = compactValues(flat)
	if len(flat) == 0 {
		return 0
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return a.internValue(valueNode{op: valueJoin, args: flat})
}

// CellResultValue is a scalar reference to one result slot of an SCC cell.
// It is deliberately not named Compose: resolving a product.Value does not
// compose the callee relation, its correlated rows, or any of its effects.
// Specialization fails closed until the caller supplies a CellResultResolver.
func (a *Arena) CellResultValue(cell CellRef, args ...ValueTerm) ValueTerm {
	return a.internValue(valueNode{op: valueCellResult, cell: cell, args: append([]ValueTerm(nil), args...)})
}

// IteratorProjectionValue retains one key/value projection from a canonical
// signature iterator effect. The source container remains symbolic; loop
// cardinality and SCC convergence stay owned by the CFG solver.
func (a *Arena) IteratorProjectionValue(iterator iteration.Iterator, variableIndex int, source ValueTerm) ValueTerm {
	if source == 0 || variableIndex < 0 || variableIndex > 1 ||
		(iterator.Kind != iteration.IterateIndexed && iterator.Kind != iteration.IterateKeyed) {
		return 0
	}
	return a.internValue(valueNode{op: valueIteratorProjection, iterator: iterator, variableIndex: variableIndex, args: []ValueTerm{source}})
}

// DynamicReadValue retains the functional relation tablePath[key] without
// encoding a marker into product.Value. owner is the value at tablePath's root
// when tablePath has a suffix; for a root-only path it is the table value
// itself. The resolver projects the path before indexing. Specialization fails
// closed unless a canonical DynamicReadResolver and all bindings are present.
func (a *Arena) DynamicReadValue(owner ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	if owner == 0 || tablePath == 0 || key == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueDynamicRead, args: []ValueTerm{owner, key}, path: tablePath})
}

// DynamicReadTableValue retains a direct table value together with its real
// caller path. Unlike DynamicReadValue, the resolver must not project the path
// from an owner again; the path exists for exact flow-sensitive read evidence.
func (a *Arena) DynamicReadTableValue(table ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	if table == 0 || tablePath == 0 || key == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueDynamicTableRead, args: []ValueTerm{table, key}, path: tablePath})
}

// DynamicReadTableValueOr retains a compile-time proven static fallback for a
// direct table read. The fallback is used only when concrete path/heap/type
// projection cannot answer; it is part of the term identity and rebases with
// the rest of the DAG.
func (a *Arena) DynamicReadTableValueOr(table ValueTerm, tablePath PathTerm, key, fallback ValueTerm) ValueTerm {
	if table == 0 || tablePath == 0 || key == 0 || fallback == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueDynamicTableRead, args: []ValueTerm{table, key, fallback}, path: tablePath})
}

func (a *Arena) Path(root Root, suffix ...segment.Segment) PathTerm {
	return a.internPath(pathNode{root: root, segments: append([]segment.Segment(nil), suffix...)})
}

func (a *Arena) True() Guard  { return 1 }
func (a *Arena) False() Guard { return 2 }
func (a *Arena) Truthy(value ValueTerm) Guard {
	return a.internGuard(guardNode{op: guardTruthy, value: value})
}
func (a *Arena) Falsy(value ValueTerm) Guard {
	return a.internGuard(guardNode{op: guardFalsy, value: value})
}
func (a *Arena) And(guards ...Guard) Guard { return a.logical(guardAnd, guards) }
func (a *Arena) Or(guards ...Guard) Guard  { return a.logical(guardOr, guards) }

func (a *Arena) logical(op guardOp, guards []Guard) Guard {
	flat := make([]Guard, 0, len(guards))
	for _, guard := range guards {
		if guard == 0 || int(guard) >= len(a.guards) {
			continue
		}
		n := a.guards[guard]
		if op == guardAnd && n.op == guardFalse || op == guardOr && n.op == guardTrue {
			return guard
		}
		if op == guardAnd && n.op == guardTrue || op == guardOr && n.op == guardFalse {
			continue
		}
		if n.op == op {
			flat = append(flat, n.args...)
		} else {
			flat = append(flat, guard)
		}
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i] < flat[j] })
	flat = compactGuards(flat)
	if len(flat) == 0 {
		if op == guardAnd {
			return a.True()
		}
		return a.False()
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return a.internGuard(guardNode{op: op, args: flat})
}

func compactValues(in []ValueTerm) []ValueTerm {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
func compactGuards(in []Guard) []Guard {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func (a *Arena) internValue(n valueNode) ValueTerm {
	key := a.valueKey(n)
	for _, id := range a.valueKeys[key] {
		if a.valueEqual(a.values[id], n) {
			return id
		}
	}
	n.args = append([]ValueTerm(nil), n.args...)
	id := ValueTerm(len(a.values))
	a.values = append(a.values, n)
	a.valueKeys[key] = append(a.valueKeys[key], id)
	return id
}
func (a *Arena) internPath(n pathNode) PathTerm {
	key := fmt.Sprintf("%d:%d:%v", n.root.Kind, n.root.Index, n.segments)
	for _, id := range a.pathKeys[key] {
		if pathNodeEqual(a.paths[id], n) {
			return id
		}
	}
	n.segments = append([]segment.Segment(nil), n.segments...)
	id := PathTerm(len(a.paths))
	a.paths = append(a.paths, n)
	a.pathKeys[key] = append(a.pathKeys[key], id)
	return id
}
func (a *Arena) internGuard(n guardNode) Guard {
	key := fmt.Sprintf("%d:%d:%v", n.op, n.value, n.args)
	for _, id := range a.guardKeys[key] {
		if guardNodeEqual(a.guards[id], n) {
			return id
		}
	}
	n.args = append([]Guard(nil), n.args...)
	id := Guard(len(a.guards))
	a.guards = append(a.guards, n)
	a.guardKeys[key] = append(a.guardKeys[key], id)
	return id
}

func (a *Arena) valueKey(n valueNode) string {
	switch n.op {
	case valueRoot:
		return fmt.Sprintf("r:%d:%d", n.root.Kind, n.root.Index)
	case valueConstant:
		if a.reg == nil {
			return "c:nil"
		}
		return "c:" + strconv.FormatUint(product.Hash(a.reg, n.value), 16)
	case valueJoin:
		return fmt.Sprintf("j:%v", n.args)
	case valueRefinement:
		if a.reg == nil {
			return "m:nil"
		}
		return fmt.Sprintf("m:%d:%x", n.args[0], product.Hash(a.reg, n.value))
	case valueCellResult:
		return fmt.Sprintf("x:%d:%d:%v", n.cell.Function, n.cell.Slot, n.args)
	case valueDynamicRead:
		return fmt.Sprintf("d:%d:%d:%d", n.args[0], n.path, n.args[1])
	case valueDynamicTableRead:
		return fmt.Sprintf("dt:%d:%v", n.path, n.args)
	case valueIteratorProjection:
		return fmt.Sprintf("i:%d:%d:%d:%v", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, n.args)
	default:
		return "invalid"
	}
}
func (a *Arena) valueEqual(x, y valueNode) bool {
	if x.op != y.op || x.root != y.root || x.cell != y.cell || x.path != y.path || x.iterator != y.iterator || x.variableIndex != y.variableIndex || len(x.args) != len(y.args) {
		return false
	}
	for i := range x.args {
		if x.args[i] != y.args[i] {
			return false
		}
	}
	if x.op == valueConstant || x.op == valueRefinement {
		return a.reg != nil && product.Equal(a.reg, x.value, y.value)
	}
	return true
}
func pathNodeEqual(x, y pathNode) bool {
	if x.root != y.root || len(x.segments) != len(y.segments) {
		return false
	}
	for i := range x.segments {
		if x.segments[i] != y.segments[i] {
			return false
		}
	}
	return true
}
func guardNodeEqual(x, y guardNode) bool {
	if x.op != y.op || x.value != y.value || len(x.args) != len(y.args) {
		return false
	}
	for i := range x.args {
		if x.args[i] != y.args[i] {
			return false
		}
	}
	return true
}

// CellResultResolver resolves one scalar result of an SCC cell. It cannot
// represent relational call composition or callee effects; those must be
// composed before specialization by the lexical SCC relation solver.
type CellResultResolver func(CellRef, []product.Value) (product.Value, bool)

// DynamicReadResolver binds a syntax-free table read to the caller's concrete
// state/visibility context. Its first product value is the tablePath root owner
// (or the table itself for a root-only path). Implementations must project the
// path using the engine's canonical path, heap, and dynamic-index semantics.
type DynamicReadResolver func(pathdom.Path, product.Value, product.Value) (product.Value, bool)

type IteratorProjectionResolver func(iteration.Iterator, int, product.Value) (product.Value, bool)

// SpecializationContext owns optional concrete evaluators. A term requiring a
// missing evaluator fails the entire specialization transaction.
type SpecializationContext struct {
	CellResult         CellResultResolver
	DynamicRead        DynamicReadResolver
	DynamicTableRead   DynamicReadResolver
	IteratorProjection IteratorProjectionResolver
}

func (a *Arena) evalValue(term ValueTerm, cursor BindingCursor, context SpecializationContext) (product.Value, bool) {
	if term == 0 || int(term) >= len(a.values) || a.reg == nil {
		return product.Value{}, false
	}
	n := a.values[term]
	switch n.op {
	case valueRoot:
		return cursor.Value(n.root)
	case valueConstant:
		return n.value, true
	case valueJoin:
		out := product.Bottom(a.reg)
		for _, arg := range n.args {
			v, ok := a.evalValue(arg, cursor, context)
			if !ok {
				return product.Value{}, false
			}
			out = product.Join(a.reg, out, v)
		}
		return out, true
	case valueRefinement:
		if len(n.args) != 1 {
			return product.Value{}, false
		}
		value, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		return factapply.RefineProductValueConstraint(a.reg, value, n.value), true
	case valueCellResult:
		if context.CellResult == nil {
			return product.Value{}, false
		}
		args := make([]product.Value, len(n.args))
		for i, arg := range n.args {
			v, ok := a.evalValue(arg, cursor, context)
			if !ok {
				return product.Value{}, false
			}
			args[i] = v
		}
		return context.CellResult(n.cell, args)
	case valueDynamicRead:
		if context.DynamicRead == nil || len(n.args) != 2 {
			return product.Value{}, false
		}
		table, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		key, ok := a.evalValue(n.args[1], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		tablePath, ok := a.evalPath(n.path, cursor)
		if !ok {
			return product.Value{}, false
		}
		return context.DynamicRead(tablePath, table, key)
	case valueDynamicTableRead:
		if context.DynamicTableRead == nil || (len(n.args) != 2 && len(n.args) != 3) {
			return product.Value{}, false
		}
		table, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		key, ok := a.evalValue(n.args[1], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		tablePath, ok := a.evalPath(n.path, cursor)
		if !ok {
			return product.Value{}, false
		}
		if value, ok := context.DynamicTableRead(tablePath, table, key); ok {
			return value, true
		}
		if len(n.args) == 3 {
			return a.evalValue(n.args[2], cursor, context)
		}
		return product.Value{}, false
	case valueIteratorProjection:
		if context.IteratorProjection == nil || len(n.args) != 1 {
			return product.Value{}, false
		}
		source, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		return context.IteratorProjection(n.iterator, n.variableIndex, source)
	default:
		return product.Value{}, false
	}
}

func (a *Arena) evalPath(term PathTerm, cursor BindingCursor) (pathdom.Path, bool) {
	if term == 0 || int(term) >= len(a.paths) {
		return pathdom.Path{}, false
	}
	n := a.paths[term]
	root, ok := cursor.Path(n.root)
	if !ok || root.IsEmpty() {
		return pathdom.Path{}, false
	}
	out := root.Clone()
	out.Segments = append(out.Segments, n.segments...)
	return out, true
}

func (a *Arena) canonicalValue(term ValueTerm) string {
	if term == 0 || int(term) >= len(a.values) {
		return "_"
	}
	n := a.values[term]
	switch n.op {
	case valueRoot:
		return fmt.Sprintf("r%d.%d", n.root.Kind, n.root.Index)
	case valueConstant:
		return a.valueKey(n)
	case valueJoin:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalValue(x)
		}
		sort.Strings(parts)
		return "j(" + strings.Join(parts, ",") + ")"
	case valueRefinement:
		return "m(" + a.canonicalValue(n.args[0]) + "," + strconv.FormatUint(product.Hash(a.reg, n.value), 16) + ")"
	case valueCellResult:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalValue(x)
		}
		return fmt.Sprintf("c%d.%d(%s)", n.cell.Function, n.cell.Slot, strings.Join(parts, ","))
	case valueDynamicRead:
		return "d(" + a.canonicalValue(n.args[0]) + "," + a.canonicalPath(n.path) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueDynamicTableRead:
		parts := []string{a.canonicalValue(n.args[0]), a.canonicalPath(n.path), a.canonicalValue(n.args[1])}
		if len(n.args) == 3 {
			parts = append(parts, a.canonicalValue(n.args[2]))
		}
		return "dt(" + strings.Join(parts, ",") + ")"
	case valueIteratorProjection:
		return fmt.Sprintf("i%d.%d.%d(%s)", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, a.canonicalValue(n.args[0]))
	default:
		return "_"
	}
}

func (a *Arena) validValue(term ValueTerm, shape Shape, seen map[ValueTerm]bool) bool {
	if term == 0 || int(term) >= len(a.values) {
		return false
	}
	if seen[term] {
		return true
	}
	seen[term] = true
	n := a.values[term]
	if n.op == valueRoot && !shape.validate(n.root) {
		return false
	}
	if n.op == valueDynamicRead && (len(n.args) != 2 || !a.validPath(n.path, shape)) {
		return false
	}
	if n.op == valueRefinement && len(n.args) != 1 {
		return false
	}
	if n.op == valueDynamicTableRead && ((len(n.args) != 2 && len(n.args) != 3) || !a.validPath(n.path, shape)) {
		return false
	}
	if n.op == valueIteratorProjection && (len(n.args) != 1 || n.variableIndex < 0 || n.variableIndex > 1 ||
		(n.iterator.Kind != iteration.IterateIndexed && n.iterator.Kind != iteration.IterateKeyed)) {
		return false
	}
	if n.op == valueInvalid {
		return false
	}
	for _, arg := range n.args {
		if !a.validValue(arg, shape, seen) {
			return false
		}
	}
	return true
}

func (a *Arena) canonicalPath(term PathTerm) string {
	if term == 0 || int(term) >= len(a.paths) {
		return "_"
	}
	n := a.paths[term]
	return fmt.Sprintf("p%d.%d:%v", n.root.Kind, n.root.Index, n.segments)
}

func (a *Arena) validPath(term PathTerm, shape Shape) bool {
	return term != 0 && int(term) < len(a.paths) && shape.validate(a.paths[term].root)
}

func (a *Arena) validGuard(guard Guard, shape Shape) bool {
	if guard == 0 || int(guard) >= len(a.guards) {
		return false
	}
	n := a.guards[guard]
	if n.op == guardInvalid {
		return false
	}
	if n.value != 0 && !a.validValue(n.value, shape, make(map[ValueTerm]bool)) {
		return false
	}
	for _, arg := range n.args {
		if !a.validGuard(arg, shape) {
			return false
		}
	}
	return true
}

func (a *Arena) containsCellResult(term ValueTerm, seen map[ValueTerm]bool) bool {
	if term == 0 || int(term) >= len(a.values) || seen[term] {
		return false
	}
	seen[term] = true
	n := a.values[term]
	if n.op == valueCellResult {
		return true
	}
	for _, arg := range n.args {
		if a.containsCellResult(arg, seen) {
			return true
		}
	}
	return false
}

func (a *Arena) guardContainsCellResult(guard Guard, seen map[Guard]bool) bool {
	if guard == 0 || int(guard) >= len(a.guards) || seen[guard] {
		return false
	}
	seen[guard] = true
	n := a.guards[guard]
	if n.value != 0 && a.containsCellResult(n.value, make(map[ValueTerm]bool)) {
		return true
	}
	for _, arg := range n.args {
		if a.guardContainsCellResult(arg, seen) {
			return true
		}
	}
	return false
}
