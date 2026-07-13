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
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	valueStringConcat
	valueScalarEqual
	valueScalarNotEqual
	valueScalarAnd
	valueScalarOr
	valueIteratorProjection
	valueStaticIndex
	valueAllocationResult
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
	assertedType  typ.Type
	hasAsserted   bool
	allocation    AllocationTemplateTerm
	resultIndex   int
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
	reg            *axis.Registry
	values         []valueNode
	paths          []pathNode
	guards         []guardNode
	valueKeys      map[uint64][]ValueTerm
	pathKeys       map[uint64][]PathTerm
	guardKeys      map[uint64][]Guard
	allocations    []allocationTemplateNode
	allocationKeys map[uint64][]AllocationTemplateTerm
	// fingerprintMask is all ones in production. Tests may narrow it to force
	// collisions and prove structural equality remains the sole authority.
	fingerprintMask uint64
}

func NewArena(reg *axis.Registry) *Arena {
	a := &Arena{reg: reg, values: []valueNode{{}}, paths: []pathNode{{}}, guards: []guardNode{{}}, allocations: []allocationTemplateNode{{}}, valueKeys: make(map[uint64][]ValueTerm), pathKeys: make(map[uint64][]PathTerm), guardKeys: make(map[uint64][]Guard), allocationKeys: make(map[uint64][]AllocationTemplateTerm), fingerprintMask: ^uint64(0)}
	a.internGuard(guardNode{op: guardTrue})
	a.internGuard(guardNode{op: guardFalse})
	return a
}

func (a *Arena) Root(root Root) ValueTerm { return a.internValue(valueNode{op: valueRoot, root: root}) }

func (a *Arena) directParamRoot(term ValueTerm) (int, bool) {
	if a == nil || term == 0 || int(term) >= len(a.values) {
		return 0, false
	}
	node := a.values[term]
	if node.op != valueRoot || node.root.Kind != RootParam {
		return 0, false
	}
	return int(node.root.Index), true
}
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
	if a != nil && value != 0 && int(value) < len(a.values) {
		prior := a.values[value]
		if prior.op == valueRefinement && product.Equal(a.reg, prior.value, constraint) {
			return value
		}
	}
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

// ValueDependsOn reports whether term's immutable DAG contains dependency.
// It is a read-only structural query for composition gates and tests; callers
// cannot inspect or mutate arena nodes through it.
func (a *Arena) ValueDependsOn(term, dependency ValueTerm) bool {
	if a == nil || term == 0 || dependency == 0 || int(term) >= len(a.values) || int(dependency) >= len(a.values) {
		return false
	}
	visited := make(map[ValueTerm]bool)
	var visit func(ValueTerm) bool
	visit = func(current ValueTerm) bool {
		if current == dependency {
			return true
		}
		if current == 0 || int(current) >= len(a.values) || visited[current] {
			return false
		}
		visited[current] = true
		for _, arg := range a.values[current].args {
			if visit(arg) {
				return true
			}
		}
		return false
	}
	return visit(term)
}

// IteratorProjectionValue retains one key/value projection from a canonical
// signature iterator effect. The source container remains symbolic; loop
// cardinality and SCC convergence stay owned by the CFG solver.
func (a *Arena) IteratorProjectionValue(iterator iteration.Iterator, variableIndex int, source ValueTerm) ValueTerm {
	return a.IteratorProjectionValueWithContract(iterator, variableIndex, source, nil, false)
}

// IteratorProjectionValueWithContract retains the immutable iterator source
// contract used by the concrete generic-for transfer.
func (a *Arena) IteratorProjectionValueWithContract(iterator iteration.Iterator, variableIndex int, source ValueTerm, asserted typ.Type, hasAsserted bool) ValueTerm {
	if source == 0 || variableIndex < 0 || variableIndex > 1 ||
		(iterator.Kind != iteration.IterateIndexed && iterator.Kind != iteration.IterateKeyed) || hasAsserted != (asserted != nil) {
		return 0
	}
	return a.internValue(valueNode{op: valueIteratorProjection, iterator: iterator, variableIndex: variableIndex, assertedType: asserted, hasAsserted: hasAsserted, args: []ValueTerm{source}})
}

// StaticIndexValue retains one pure, statically named index projection. It is
// intentionally value-only: identity-backed heap reads require caller state
// and remain on DynamicRead rather than leaking through this term.
func (a *Arena) StaticIndexValue(owner ValueTerm, member segment.Segment) ValueTerm {
	if owner == 0 {
		return 0
	}
	key, ok := enginesourcevalue.StaticPathSegmentValue(a.reg, member)
	if !ok {
		return 0
	}
	return a.internValue(valueNode{op: valueStaticIndex, args: []ValueTerm{owner, a.Constant(key)}})
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

// StringConcatValue retains the pure Lua string concatenation of two symbolic
// operands. The narrow constructor deliberately does not model Lua's numeric
// coercion: specialization accepts only operands proven to contain strings and
// otherwise fails the entire relation transaction.
func (a *Arena) StringConcatValue(left, right ValueTerm) ValueTerm {
	if left == 0 || right == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueStringConcat, args: []ValueTerm{left, right}})
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
	key := a.maskFingerprint(a.valueFingerprint(n))
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
	key := a.maskFingerprint(pathFingerprint(n))
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
	key := a.maskFingerprint(guardFingerprint(n))
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

func (a *Arena) maskFingerprint(fingerprint uint64) uint64 {
	if a == nil {
		return fingerprint
	}
	return fingerprint & a.fingerprintMask
}

func (a *Arena) valueFingerprint(n valueNode) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, uint64(n.op))
	h = hashRoot(h, n.root)
	h = internalhash.MixHash(h, n.cell.Function)
	h = internalhash.MixHash(h, uint64(n.cell.Slot))
	h = internalhash.MixHash(h, uint64(n.path))
	h = internalhash.MixHash(h, uint64(n.iterator.Kind))
	h = internalhash.MixHash(h, uint64(n.iterator.Source.Index))
	h = internalhash.MixHash(h, uint64(int64(n.variableIndex)))
	if n.hasAsserted {
		h = internalhash.MixHash(h, 1)
		h = internalhash.MixHash(h, typ.EqualityHash(n.assertedType))
	}
	h = internalhash.MixHash(h, uint64(n.allocation))
	h = internalhash.MixHash(h, uint64(int64(n.resultIndex)))
	h = hashValueTerms(h, n.args)
	if (n.op == valueConstant || n.op == valueRefinement) && a.reg != nil {
		h = internalhash.MixHash(h, product.Hash(a.reg, n.value))
	}
	return h
}

func pathFingerprint(n pathNode) uint64 {
	h := hashRoot(internalhash.MixHash(termFingerprintSeed, 0x70617468), n.root)
	h = internalhash.MixHash(h, uint64(len(n.segments)))
	for _, suffix := range n.segments {
		h = hashSegment(h, suffix)
	}
	return h
}

func guardFingerprint(n guardNode) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, 0x6775617264)
	h = internalhash.MixHash(h, uint64(n.op))
	h = internalhash.MixHash(h, uint64(n.value))
	h = internalhash.MixHash(h, uint64(len(n.args)))
	for _, arg := range n.args {
		h = internalhash.MixHash(h, uint64(arg))
	}
	return h
}
func (a *Arena) valueEqual(x, y valueNode) bool {
	if x.op != y.op || x.root != y.root || x.cell != y.cell || x.path != y.path || x.iterator != y.iterator || x.variableIndex != y.variableIndex || x.hasAsserted != y.hasAsserted || x.allocation != y.allocation || x.resultIndex != y.resultIndex || len(x.args) != len(y.args) {
		return false
	}
	if x.hasAsserted && !typ.TypeEquals(x.assertedType, y.assertedType) {
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
	case valueStringConcat:
		if len(n.args) != 2 {
			return product.Value{}, false
		}
		left, ok := a.evalValue(n.args[0], cursor, context)
		if !ok || !exactStringOperand(a.reg, left) {
			return product.Value{}, false
		}
		right, ok := a.evalValue(n.args[1], cursor, context)
		if !ok || !exactStringOperand(a.reg, right) {
			return product.Value{}, false
		}
		return luasourcevalue.BinaryOperationValue(a.reg, nil, "..", left, right)
	case valueScalarEqual, valueScalarNotEqual, valueScalarAnd, valueScalarOr:
		return a.evalScalarBinaryValue(n.op, n.args, cursor, context)
	case valueIteratorProjection:
		if len(n.args) != 1 {
			return product.Value{}, false
		}
		source, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		if context.IteratorProjection != nil {
			if value, resolved := context.IteratorProjection(n.iterator, n.variableIndex, source); resolved {
				return value, true
			}
		}
		// The canonical Lua iterator projection is pure for type/value-shaped
		// containers and requires no caller state. Context resolvers remain the
		// precision extension for heap/member-backed containers.
		return luasourcevalue.IteratorVariableValue(a.reg, nil, n.iterator, n.variableIndex, source, n.assertedType, n.hasAsserted)
	case valueStaticIndex:
		if len(n.args) != 2 {
			return product.Value{}, false
		}
		owner, ok := a.evalValue(n.args[0], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		key, ok := a.evalValue(n.args[1], cursor, context)
		if !ok {
			return product.Value{}, false
		}
		return enginesourcevalue.StaticIndexValue(a.reg, nil, owner, key)
	case valueAllocationResult:
		return a.allocationResult(n.allocation, n.resultIndex)
	default:
		return product.Value{}, false
	}
}

func exactStringOperand(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && subtype.IsSubtype(t, typ.String)
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
		if a.reg == nil {
			return "c:nil"
		}
		return "c:" + strconv.FormatUint(product.Hash(a.reg, n.value), 16)
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
	case valueStringConcat:
		return "s(" + a.canonicalValue(n.args[0]) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueScalarEqual, valueScalarNotEqual, valueScalarAnd, valueScalarOr:
		return canonicalScalarBinaryValue(n.op, a.canonicalValue(n.args[0]), a.canonicalValue(n.args[1]))
	case valueIteratorProjection:
		if !n.hasAsserted {
			return fmt.Sprintf("i%d.%d.%d(%s)", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, a.canonicalValue(n.args[0]))
		}
		return fmt.Sprintf("i%d.%d.%d.%x(%s)", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, n.assertedType.Hash(), a.canonicalValue(n.args[0]))
	case valueStaticIndex:
		return "si(" + a.canonicalValue(n.args[0]) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueAllocationResult:
		op := a.allocations[n.allocation].op
		return fmt.Sprintf("a%d.%s.%d:r%d", op.Site().Owner, op.Site().Template, op.Site().Ordinal, n.resultIndex)
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
	if n.op == valueStringConcat && len(n.args) != 2 {
		return false
	}
	if isScalarBinaryValueOp(n.op) && len(n.args) != 2 {
		return false
	}
	if n.op == valueIteratorProjection && (len(n.args) != 1 || n.variableIndex < 0 || n.variableIndex > 1 ||
		(n.iterator.Kind != iteration.IterateIndexed && n.iterator.Kind != iteration.IterateKeyed) || n.hasAsserted != (n.assertedType != nil)) {
		return false
	}
	if n.op == valueStaticIndex && (len(n.args) != 2 || !a.validStaticIndexKey(n.args[1])) {
		return false
	}
	if n.op == valueAllocationResult && (len(n.args) != 0 || !a.validAllocation(n.allocation) || n.resultIndex < 0) {
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

func (a *Arena) validStaticIndexKey(term ValueTerm) bool {
	if a == nil || term == 0 || int(term) >= len(a.values) {
		return false
	}
	node := a.values[term]
	if node.op != valueConstant || len(node.args) != 0 {
		return false
	}
	_, ok := typevalue.ExactScalarKeySegment(a.reg, nil, node.value)
	return ok
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
