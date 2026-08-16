package transformer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type ValueTerm uint32

// valueAccessTerm binds one canonical value-term node to the exact State
// point whose source evaluation owns that node's direct observations. It is
// access metadata on the existing term DAG, not a second expression syntax.
// hasPoint distinguishes the generic-for point-entry role from CFG point zero.
type valueAccessTerm struct {
	term     ValueTerm
	point    cfg.Point
	hasPoint bool
	// fallback is the lower-priority input used only when the preferred
	// source-point wire is unreachable. This makes the old implicit
	// read(sourcePoint)-then-read(callPoint) rule explicit in the sealed input
	// program.
	fallback bool
}

func cloneValueAccessTerms(in []valueAccessTerm) []valueAccessTerm {
	return append([]valueAccessTerm(nil), in...)
}

type PathTerm uint32
type Guard uint32

type valueOp uint8

const (
	valueInvalid valueOp = iota
	valueRoot
	valueEnvironment
	valueConstant
	valueObjectLiteral
	valueJoin
	valueSelect
	valueRefinement
	valueFalsyAbsentRefinement
	valueExpressionRefinement
	valueCellResult
	valueCallResult
	valueFrameResult
	valueDynamicRead
	valueDynamicTableRead
	valueStringConcat
	valueUnaryOperation
	valueBinaryOperation
	valueIteratorProjection
	valueGenericForResult
	valueLoopContinuation
	valuePredicateObservation
	valueStaticIndex
	valueAllocationResult
	valueLuaTypeName
)

// CellRef is a stable SCC equation reference. Generation is deliberately not
// present: generations belong to a solve transaction, not transformer identity.
type CellRef struct {
	Function uint64
	Slot     uint32
}

// relationVar is the dense deterministic identity assigned from sorted stable
// lexical body IDs for one complete forest. It has no solve generation and no
// owner-local slot component.
type relationVar uint32

// callFrameTerm is an arena-owned identity for one lexical call transaction.
// The handle is private until direct-call lowering is cut over: there must not
// be a second public composition vocabulary beside Relation.
type callFrameTerm uint32

// loopMuTerm is the owner-arena binder for one lexical WTO component. Loop
// recurrence is a graph backedge to this term, never an unrolled row sequence
// or a depth-indexed context.
type loopMuTerm uint32

// callFrameNode owns the complete correlated call boundary. Results do not
// carry independent bindings: every frame-result term points back to this one
// immutable argument/path tuple and its exact result width.
type callFrameNode struct {
	target   CellRef
	variable relationVar
	// closureProducer is the earlier frame result whose exact closure resource
	// supplies this invocation's captured environment. It is structural
	// provenance only: the resource itself remains the single State coordinate
	// owned by the boundary application graph.
	closureProducer callFrameTerm
	point           cfg.Point
	occurrence      uint32
	shape           Shape
	values          []ValueTerm
	paths           []PathTerm
	resultCount     uint32
}

type loopMuBackedge struct {
	from cfg.Point
	to   cfg.Point
}

type loopMuNode struct {
	head      cfg.Point
	parent    loopMuTerm
	members   []cfg.Point
	backedges []loopMuBackedge
}

type valueNode struct {
	op             valueOp
	owner          lexicalidentity.StableLexicalBodyID
	root           Root
	value          product.Value
	refinementMode factflow.ExpressionRefinementMode
	args           []ValueTerm
	objectPlan     luasourcevalue.ObjectLiteralPlan
	cell           CellRef
	frame          callFrameTerm
	path           PathTerm
	keyPath        PathTerm
	rangePath      PathTerm
	indexShape     indexform.IndexShape
	integerProof   ValueTerm
	iterator       iteration.Iterator
	variableIndex  int
	assertedType   typ.Type
	hasAsserted    bool
	allocation     AllocationTemplateTerm
	resultIndex    int
	point          cfg.Point
	slot           statekey.Value
	operator       string
	guard          Guard
}

func (n valueNode) expressionRefinement() factflow.ExpressionRefinement {
	switch n.refinementMode {
	case factflow.ExpressionRefinementDeclaredContract:
		return factflow.NewExpressionDeclaredContract(factflow.ValueSource{}, n.value)
	case factflow.ExpressionRefinementRuntimeValidation:
		return factflow.NewExpressionRuntimeValidation(factflow.ValueSource{}, n.value)
	default:
		return factflow.NewExpressionRefinement(factflow.ValueSource{}, n.value)
	}
}

type pathNode struct {
	root        Root
	environment symbol.ID
	segments    []segment.Segment
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
	owner          lexicalidentity.StableLexicalBodyID
	values         []valueNode
	paths          []pathNode
	guards         []guardNode
	valueKeys      map[uint64][]ValueTerm
	pathKeys       map[uint64][]PathTerm
	guardKeys      map[uint64][]Guard
	allocations    []allocationTemplateNode
	allocationKeys map[uint64][]AllocationTemplateTerm
	callFrames     []callFrameNode
	callFrameKeys  map[uint64][]callFrameTerm
	loopMus        []loopMuNode
	loopMuKeys     map[uint64][]loopMuTerm
	environment    map[statekey.Value]struct{}
	middle         relationMiddleRegisterSchema
	sealed         bool
	// fingerprintMask is all ones in production. Tests may narrow it to force
	// collisions and prove structural equality remains the sole authority.
	fingerprintMask uint64
	// typeValues is this Arena's run-scoped typevalue derivation cache. It is
	// owned for the Arena's whole lifetime so repeated guard/value evaluation
	// against the same product.Value reuses one variant-origin projection
	// instead of rebuilding it at every visit.
	typeValues *typevalue.Cache
}

func (a *Arena) bindLexicalOwner(owner lexicalidentity.StableLexicalBodyID) bool {
	if a == nil || a.sealed || owner == (lexicalidentity.StableLexicalBodyID{}) || a.owner != (lexicalidentity.StableLexicalBodyID{}) && a.owner != owner {
		return false
	}
	if a.owner == owner {
		return true
	}
	templatesByTerm := make([][]identity.AllocationTemplate, len(a.allocations))
	identitiesByTerm := make([]map[signature.AllocationTemplateID]identity.Term, len(a.allocations))
	for term := AllocationTemplateTerm(1); int(term) < len(a.allocations); term++ {
		coordinates, err := canonicalAllocationTemplates(owner, term, a.allocations[term].op)
		if err != nil {
			return false
		}
		templates := make([]identity.AllocationTemplate, 0, len(coordinates))
		identities := make(map[signature.AllocationTemplateID]identity.Term, len(coordinates))
		for name, coordinate := range coordinates {
			templates = append(templates, coordinate)
			identities[name] = identity.AllocationTerm(coordinate)
		}
		sort.Slice(templates, func(i, j int) bool {
			return templates[i].ObjectOrdinal() < templates[j].ObjectOrdinal()
		})
		templatesByTerm[term] = templates
		identitiesByTerm[term] = identities
	}
	a.owner = owner
	for term := AllocationTemplateTerm(1); int(term) < len(a.allocations); term++ {
		a.allocations[term].templates = templatesByTerm[term]
		a.allocations[term].identities = identitiesByTerm[term]
	}
	return true
}

func NewArena(reg *axis.Registry) *Arena {
	a := &Arena{reg: reg, values: []valueNode{{}}, paths: []pathNode{{}}, guards: []guardNode{{}}, allocations: []allocationTemplateNode{{}}, callFrames: []callFrameNode{{}}, loopMus: []loopMuNode{{}}, valueKeys: make(map[uint64][]ValueTerm), pathKeys: make(map[uint64][]PathTerm), guardKeys: make(map[uint64][]Guard), allocationKeys: make(map[uint64][]AllocationTemplateTerm), callFrameKeys: make(map[uint64][]callFrameTerm), loopMuKeys: make(map[uint64][]loopMuTerm), environment: make(map[statekey.Value]struct{}), fingerprintMask: ^uint64(0), typeValues: typevalue.NewCache()}
	a.internGuard(guardNode{op: guardTrue})
	a.internGuard(guardNode{op: guardFalse})
	return a
}

// Seal permanently closes this owner arena. Existing structural terms remain
// reusable; any constructor that would grow the circuit fails closed. Frozen
// Relations therefore cannot acquire post-Freeze syntax through evaluation.
func (a *Arena) Seal() {
	if a != nil {
		a.sealed = true
	}
}

func (a *Arena) Sealed() bool { return a != nil && a.sealed }

// loopMu interns one complete lexical recurrence binder. members are in the
// owner's canonical WTO order; backedges are structural CFG identities. A
// recursive traversal therefore reuses this term regardless of dynamic loop
// depth.
func (a *Arena) loopMu(head cfg.Point, parent loopMuTerm, members []cfg.Point, backedges []loopMuBackedge) loopMuTerm {
	if a == nil || len(members) == 0 || members[0] != head || len(backedges) == 0 || parent != 0 && int(parent) >= len(a.loopMus) {
		return 0
	}
	memberSet := make(map[cfg.Point]struct{}, len(members))
	for _, point := range members {
		if _, duplicate := memberSet[point]; duplicate {
			return 0
		}
		memberSet[point] = struct{}{}
	}
	for _, edge := range backedges {
		if edge.to != head {
			return 0
		}
		if _, ok := memberSet[edge.from]; !ok {
			return 0
		}
	}
	node := loopMuNode{head: head, parent: parent, members: members, backedges: backedges}
	key := a.maskFingerprint(loopMuFingerprint(node))
	for _, term := range a.loopMuKeys[key] {
		if loopMuNodeEqual(a.loopMus[term], node) {
			return term
		}
	}
	if a.sealed {
		return 0
	}
	node.members = append([]cfg.Point(nil), members...)
	node.backedges = append([]loopMuBackedge(nil), backedges...)
	term := loopMuTerm(len(a.loopMus))
	a.loopMus = append(a.loopMus, node)
	a.loopMuKeys[key] = append(a.loopMuKeys[key], term)
	return term
}

func (a *Arena) Root(root Root) ValueTerm { return a.internValue(valueNode{op: valueRoot, root: root}) }

func (a *Arena) validRoot(shape Shape, root Root) bool {
	if root.Kind == RootMiddle {
		return a != nil && a.middle.validRoot(root)
	}
	return shape.validate(root)
}

func (a *Arena) bindEnvironmentSymbol(symbol symbol.ID) ValueTerm {
	if a == nil || a.sealed || symbol == 0 {
		return 0
	}
	slot := statekey.SymbolValue(symbol)
	a.environment[slot] = struct{}{}
	value := a.internValue(valueNode{op: valueEnvironment, slot: slot})
	path := a.internPath(pathNode{environment: symbol})
	if value == 0 || path == 0 {
		return 0
	}
	return value
}

// bindCallResult seals one call-site-owned result register. Unlike a function
// ReturnSlot, its identity includes the producing CFG point, so independent
// calls with the same result ordinal cannot overwrite each other's source
// terms before a later transaction consumes them.
func (a *Arena) bindCallResult(point cfg.Point, index int) ValueTerm {
	if a == nil || a.sealed || point == 0 || index < 0 {
		return 0
	}
	slot := statekey.CallResult(uint32(point), uint32(index))
	if slot == 0 {
		return 0
	}
	a.environment[slot] = struct{}{}
	return a.internValue(valueNode{op: valueEnvironment, slot: slot})
}

func (a *Arena) bindExpressionValue(ref factflow.ExprRef) ValueTerm {
	if a == nil || a.sealed || ref == 0 {
		return 0
	}
	slot := statekey.ExpressionValue(uint32(ref))
	if slot == 0 {
		return 0
	}
	a.environment[slot] = struct{}{}
	return a.internValue(valueNode{op: valueEnvironment, slot: slot})
}

func (a *Arena) expressionValue(ref factflow.ExprRef) (ValueTerm, bool) {
	if a == nil || ref == 0 {
		return 0, false
	}
	slot := statekey.ExpressionValue(uint32(ref))
	if _, ok := a.environment[slot]; !ok {
		return 0, false
	}
	term := a.internValue(valueNode{op: valueEnvironment, slot: slot})
	return term, term != 0
}

func (a *Arena) callResultValue(point cfg.Point, index int) (ValueTerm, bool) {
	if a == nil || point == 0 || index < 0 {
		return 0, false
	}
	slot := statekey.CallResult(uint32(point), uint32(index))
	if _, ok := a.environment[slot]; !ok {
		return 0, false
	}
	term := a.internValue(valueNode{op: valueEnvironment, slot: slot})
	return term, term != 0
}

func (a *Arena) environmentValue(symbol symbol.ID) (ValueTerm, bool) {
	if a == nil || symbol == 0 {
		return 0, false
	}
	slot := statekey.SymbolValue(symbol)
	if _, ok := a.environment[slot]; !ok {
		return 0, false
	}
	return a.internValue(valueNode{op: valueEnvironment, slot: slot}), true
}

func (a *Arena) validEnvironmentSlot(slot statekey.Value) bool {
	_, ok := a.environment[slot]
	return a != nil && slot != 0 && ok
}

// EnvironmentPath is a body-owned lexical path. Unlike boundary Path roots it
// is never substituted through a call frame; it addresses the post-N4 value
// carried in this body's sealed environment slot.
func (a *Arena) EnvironmentPath(id symbol.ID, segments ...segment.Segment) PathTerm {
	if a == nil || id == 0 || !a.validEnvironmentSlot(statekey.SymbolValue(id)) {
		return 0
	}
	return a.internPath(pathNode{environment: id, segments: append([]segment.Segment(nil), segments...)})
}

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

// refinedParamRoot reports exact parameter identity through unary constraints.
// It is deliberately separate from directParamRoot: refinement preserves the
// return alias, but does not establish the legacy whole-function ReturnFlow.
func (a *Arena) refinedParamRoot(term ValueTerm) (int, bool) {
	wrapped := false
	for a != nil && term != 0 && int(term) < len(a.values) {
		node := a.values[term]
		switch node.op {
		case valueRoot:
			if wrapped && node.root.Kind == RootParam {
				return int(node.root.Index), true
			}
			return 0, false
		case valueRefinement, valueFalsyAbsentRefinement, valueExpressionRefinement:
			if len(node.args) != 1 {
				return 0, false
			}
			wrapped = true
			term = node.args[0]
		default:
			return 0, false
		}
	}
	return 0, false
}

func (a *Arena) Constant(value product.Value) ValueTerm {
	return a.internValue(valueNode{op: valueConstant, value: value})
}

// ObjectLiteralValue is the sole derived value term for one Lua constructor.
// Its immutable plan and unique raw-source arguments are evaluated by both
// concrete and guarded paths; heap materialization consumes this same term as
// its root and never re-derives a witness from old heap state.
func (a *Arena) ObjectLiteralValue(plan luasourcevalue.ObjectLiteralPlan, args ...ValueTerm) ValueTerm {
	if a == nil || !plan.Valid() || plan.ValueSourceCount() != len(args) {
		return 0
	}
	for _, arg := range args {
		if arg == 0 || int(arg) >= len(a.values) {
			return 0
		}
	}
	return a.internValue(valueNode{op: valueObjectLiteral, objectPlan: plan, args: append([]ValueTerm(nil), args...)})
}

// RefineValue retains a positive canonical factflow constraint in the term
// DAG. Specialization calls the same scalar kernel as concrete factapply.
// Context-sensitive negation and falsy-absence refinements fail closed.
func (a *Arena) RefineValue(value ValueTerm, refinement factflow.ValueRefinement) (ValueTerm, bool) {
	if value == 0 || refinement.NegatedLiteral() {
		return 0, false
	}
	constraint, ok := refinement.Constraint()
	if !ok {
		return value, true
	}
	if refinement.FalsyAbsent() {
		return a.internValue(valueNode{op: valueFalsyAbsentRefinement, value: constraint, args: []ValueTerm{value}}), true
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

// expressionRefinementValue retains one certified source-authored assertion or
// cast over an exact scalar producer. It is deliberately distinct from
// ValueRefinement: expression claims use the canonical sourcevalue operation,
// including declared-contract and runtime-validation evidence semantics.
func (a *Arena) expressionRefinementValue(value ValueTerm, refinement factflow.ExpressionRefinement) ValueTerm {
	if a == nil || value == 0 || int(value) >= len(a.values) {
		return 0
	}
	return a.internValue(valueNode{
		op: valueExpressionRefinement, value: refinement.Refinement(),
		refinementMode: refinement.Mode(), args: []ValueTerm{value},
	})
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

// SelectValue constructs a path-sensitive SSA value.  The condition remains
// part of the value's structural identity, so merging control-flow paths does
// not erase the correlation between the branch predicate and the value it
// selected.  Evaluation is lazy: a decided guard evaluates exactly one arm;
// an abstractly undecided guard joins both possible arms.
func (a *Arena) SelectValue(guard Guard, whenTrue, whenFalse ValueTerm) ValueTerm {
	if a == nil || guard == 0 || int(guard) >= len(a.guards) ||
		whenTrue == 0 || int(whenTrue) >= len(a.values) ||
		whenFalse == 0 || int(whenFalse) >= len(a.values) {
		return 0
	}
	if whenTrue == whenFalse {
		return whenTrue
	}
	switch a.guards[guard].op {
	case guardTrue:
		return whenTrue
	case guardFalse:
		return whenFalse
	}
	return a.internValue(valueNode{op: valueSelect, guard: guard, args: []ValueTerm{whenTrue, whenFalse}})
}

// CellResultValue is a scalar reference to one result slot of an SCC cell.
// It is deliberately not named Compose: resolving a product.Value does not
// compose the callee relation, its correlated rows, or any of its effects.
// Specialization fails closed until the caller supplies a CellResultResolver.
func (a *Arena) CellResultValue(cell CellRef, args ...ValueTerm) ValueTerm {
	return a.internValue(valueNode{op: valueCellResult, cell: cell, args: append([]ValueTerm(nil), args...)})
}

// CallResultValue preserves the exact value produced by one lexical call slot
// while stopping direct-boundary provenance inference at the call boundary.
// It is callback-free: evaluation unwraps its single already-composed child.
func (a *Arena) CallResultValue(point cfg.Point, slot uint32, value ValueTerm) ValueTerm {
	if a == nil || value == 0 || int(value) >= len(a.values) {
		return 0
	}
	return a.internValue(valueNode{op: valueCallResult, point: point, resultIndex: int(slot), args: []ValueTerm{value}})
}

// predicateObservationValue gives one lexical branch observation its own
// Boolean epoch while retaining the observed value as ordinary term syntax.
// Guard execution may quotient this epoch by truth already entailed by the
// incoming decision carrier; a later mutation therefore starts a new choice,
// while an unchanged repeated predicate does not grow an independent world.
func (a *Arena) predicateObservationValue(point cfg.Point, value ValueTerm) ValueTerm {
	if a == nil || value == 0 || int(value) >= len(a.values) {
		return 0
	}
	return a.internValue(valueNode{op: valuePredicateObservation, point: point, args: []ValueTerm{value}})
}

// callFrame interns one exact lexical call occurrence. The caller shape is
// checked when the owning row is validated; construction only proves that all
// handles belong to this arena and takes immutable copies of the bindings.
func (a *Arena) callFrame(target CellRef, point cfg.Point, occurrence uint32, shape Shape, values []ValueTerm, paths []PathTerm, resultCount uint32) callFrameTerm {
	if a == nil || target == (CellRef{}) || len(values) != shape.InputCount() || len(paths) != len(values) {
		return 0
	}
	for i, value := range values {
		if value == 0 || int(value) >= len(a.values) || (paths[i] != 0 && int(paths[i]) >= len(a.paths)) {
			return 0
		}
	}
	node := callFrameNode{target: target, point: point, occurrence: occurrence, shape: shape, values: values, paths: paths, resultCount: resultCount}
	key := a.maskFingerprint(callFrameFingerprint(node))
	for _, term := range a.callFrameKeys[key] {
		if callFrameNodeEqual(a.callFrames[term], node) {
			return term
		}
	}
	if a.sealed {
		return 0
	}
	node.values = append([]ValueTerm(nil), values...)
	node.paths = append([]PathTerm(nil), paths...)
	term := callFrameTerm(len(a.callFrames))
	a.callFrames = append(a.callFrames, node)
	a.callFrameKeys[key] = append(a.callFrameKeys[key], term)
	return term
}

func (a *Arena) relationFrame(variable relationVar, point cfg.Point, occurrence uint32, shape Shape, values []ValueTerm, paths []PathTerm, resultCount uint32) callFrameTerm {
	return a.relationFrameWithClosureProducer(variable, 0, point, occurrence, shape, values, paths, resultCount)
}

func (a *Arena) relationFrameWithClosureProducer(variable relationVar, producer callFrameTerm, point cfg.Point, occurrence uint32, shape Shape, values []ValueTerm, paths []PathTerm, resultCount uint32) callFrameTerm {
	if a == nil || variable == 0 || len(values) != shape.InputCount() || len(paths) != len(values) {
		return 0
	}
	if producer != 0 && int(producer) >= len(a.callFrames) {
		return 0
	}
	for i, value := range values {
		if value == 0 || int(value) >= len(a.values) || paths[i] != 0 && int(paths[i]) >= len(a.paths) {
			return 0
		}
	}
	node := callFrameNode{variable: variable, closureProducer: producer, point: point, occurrence: occurrence, shape: shape, values: append([]ValueTerm(nil), values...), paths: append([]PathTerm(nil), paths...), resultCount: resultCount}
	key := a.maskFingerprint(callFrameFingerprint(node))
	for _, term := range a.callFrameKeys[key] {
		if callFrameNodeEqual(a.callFrames[term], node) {
			return term
		}
	}
	if a.sealed {
		return 0
	}
	term := callFrameTerm(len(a.callFrames))
	a.callFrames = append(a.callFrames, node)
	a.callFrameKeys[key] = append(a.callFrameKeys[key], term)
	return term
}

// frameResultValue selects one slot from a row-owned call frame. Correlation
// is retained by the shared frame identity; row validation additionally
// requires the owning call step to be present in that same row.
func (a *Arena) frameResultValue(frame callFrameTerm, slot uint32) ValueTerm {
	if a == nil || frame == 0 || int(frame) >= len(a.callFrames) || slot >= a.callFrames[frame].resultCount {
		return 0
	}
	return a.internValue(valueNode{op: valueFrameResult, frame: frame, resultIndex: int(slot)})
}

// ValueDependsOn reports whether term's immutable DAG contains dependency.
// It is a read-only structural query for composition gates and tests; callers
// cannot inspect or mutate arena nodes through it.
func (a *Arena) ValueDependsOn(term, dependency ValueTerm) bool {
	if a == nil || term == 0 || dependency == 0 || int(term) >= len(a.values) || int(dependency) >= len(a.values) {
		return false
	}
	visited := make(map[ValueTerm]bool)
	visitedGuards := make(map[Guard]bool)
	var visit func(ValueTerm) bool
	var visitGuard func(Guard) bool
	visit = func(current ValueTerm) bool {
		if current == dependency {
			return true
		}
		if current == 0 || int(current) >= len(a.values) || visited[current] {
			return false
		}
		visited[current] = true
		node := a.values[current]
		if node.guard != 0 && visitGuard(node.guard) {
			return true
		}
		for _, arg := range node.args {
			if visit(arg) {
				return true
			}
		}
		return false
	}
	visitGuard = func(guard Guard) bool {
		if guard == 0 || int(guard) >= len(a.guards) || visitedGuards[guard] {
			return false
		}
		visitedGuards[guard] = true
		node := a.guards[guard]
		if node.value != 0 && visit(node.value) {
			return true
		}
		for _, arg := range node.args {
			if visitGuard(arg) {
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
// contract used by the canonical generic-for projection.
func (a *Arena) IteratorProjectionValueWithContract(iterator iteration.Iterator, variableIndex int, source ValueTerm, asserted typ.Type, hasAsserted bool) ValueTerm {
	return a.iteratorProjectionValueWithFallback(iterator, variableIndex, source, 0, asserted, hasAsserted)
}

// iteratorProjectionValueWithFallback models the generic-for transaction's
// exact no-write branch. When the canonical term evaluator cannot refine a
// gradual source, the optional fallback preserves the pre-transfer target; it
// does not abort the body or synthesize Top.
func (a *Arena) iteratorProjectionValueWithFallback(iterator iteration.Iterator, variableIndex int, source, fallback ValueTerm, asserted typ.Type, hasAsserted bool) ValueTerm {
	if source == 0 || variableIndex < 0 || variableIndex > 1 ||
		(iterator.Kind != iteration.IterateIndexed && iterator.Kind != iteration.IterateKeyed) || hasAsserted != (asserted != nil) {
		return 0
	}
	args := []ValueTerm{source}
	if fallback != 0 {
		args = append(args, fallback)
	}
	return a.internValue(valueNode{op: valueIteratorProjection, iterator: iterator, variableIndex: variableIndex, assertedType: asserted, hasAsserted: hasAsserted, args: args})
}

// genericForResultValue retains the complete Lua iterator protocol application
// iterator(state, control) and its exact no-write fallback.
func (a *Arena) genericForResultValue(variableIndex int, iterator, state, control, fallback ValueTerm) ValueTerm {
	if a == nil || variableIndex < 0 || iterator == 0 || state == 0 || control == 0 || fallback == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueGenericForResult, variableIndex: variableIndex, args: []ValueTerm{iterator, state, control, fallback}})
}

// loopContinuationValue is the Boolean control atom for one iterator/numeric
// loop head. Unlike an ordinary source guard, both outcomes are possible: the
// current iterator step may produce a body row or terminate. The owning CFG
// point distinguishes independent loops, while the lexical Mu lifetime closes
// and refreshes the atom at feedback.
func (a *Arena) loopContinuationValue(point cfg.Point) ValueTerm {
	return a.loopContinuationValueOwned(a.owner, point)
}

func (a *Arena) loopContinuationValueOwned(owner lexicalidentity.StableLexicalBodyID, point cfg.Point) ValueTerm {
	if a == nil || owner == (lexicalidentity.StableLexicalBodyID{}) {
		return 0
	}
	return a.internValue(valueNode{op: valueLoopContinuation, owner: owner, point: point})
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
// itself. Only the guarded factor-native resolver may execute the relation;
// scalar specialization deliberately fails closed for this nonseparable term.
func (a *Arena) DynamicReadValue(owner ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	return a.DynamicReadValueAt(0, owner, tablePath, key)
}

// DynamicReadValueAt retains the lexical CFG point whose visibility authority
// owns tablePath. Replacement-program terms use this point-aware form.
func (a *Arena) DynamicReadValueAt(point cfg.Point, owner ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	return a.DynamicReadValueAtPaths(point, owner, tablePath, key, 0)
}

// DynamicReadValueAtPaths is DynamicReadValueAt with exact optional source
// provenance for the key. The key path is semantic input to relational
// PathEvidence (membership, range and numeric facts), not debug metadata.
func (a *Arena) DynamicReadValueAtPaths(point cfg.Point, owner ValueTerm, tablePath PathTerm, key ValueTerm, keyPath PathTerm) ValueTerm {
	return a.dynamicReadValueAtPaths(point, owner, tablePath, key, keyPath, indexform.IndexShape{}, 0, 0)
}

func (a *Arena) dynamicReadValueAtPaths(point cfg.Point, owner ValueTerm, tablePath PathTerm, key ValueTerm, keyPath PathTerm, shape indexform.IndexShape, rangePath PathTerm, integerProof ValueTerm) ValueTerm {
	if owner == 0 || tablePath == 0 || key == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueDynamicRead, args: []ValueTerm{owner, key}, path: tablePath, keyPath: keyPath, rangePath: rangePath, indexShape: shape, integerProof: integerProof, point: point})
}

// DynamicReadTableValue retains a direct table value and, when one exists, its
// real caller path. Unlike DynamicReadValue, the resolver must not project the
// path from an owner again; an optional path carries exact flow-sensitive read
// evidence without excluding unnameable table expressions.
func (a *Arena) DynamicReadTableValue(table ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	return a.DynamicReadTableValueAt(0, table, tablePath, key)
}

// DynamicReadTableValueAt retains a direct table-value read at its lexical
// visibility point. tablePath is optional: an unnameable table expression has
// no path evidence, but its exact value still supports heap/type index
// semantics without manufacturing an address.
func (a *Arena) DynamicReadTableValueAt(point cfg.Point, table ValueTerm, tablePath PathTerm, key ValueTerm) ValueTerm {
	return a.DynamicReadTableValueAtPaths(point, table, tablePath, key, 0)
}

// DynamicReadTableValueAtPaths is the direct-table counterpart retaining the
// key's exact optional path provenance.
func (a *Arena) DynamicReadTableValueAtPaths(point cfg.Point, table ValueTerm, tablePath PathTerm, key ValueTerm, keyPath PathTerm) ValueTerm {
	return a.dynamicReadTableValueAtPaths(point, table, tablePath, key, keyPath, indexform.IndexShape{}, 0, 0)
}

func (a *Arena) dynamicReadTableValueAtPaths(point cfg.Point, table ValueTerm, tablePath PathTerm, key ValueTerm, keyPath PathTerm, shape indexform.IndexShape, rangePath PathTerm, integerProof ValueTerm) ValueTerm {
	if table == 0 || key == 0 {
		return 0
	}
	return a.internValue(valueNode{op: valueDynamicTableRead, args: []ValueTerm{table, key}, path: tablePath, keyPath: keyPath, rangePath: rangePath, indexShape: shape, integerProof: integerProof, point: point})
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

// AppendPath extends one already-owned boundary or lexical-environment path.
// The base namespace is preserved exactly; callers cannot convert a local
// environment path into a substitutable call-frame root or vice versa.
func (a *Arena) AppendPath(base PathTerm, suffix ...segment.Segment) PathTerm {
	if a == nil || base == 0 || int(base) >= len(a.paths) {
		return 0
	}
	node := a.paths[base]
	node.segments = append(append([]segment.Segment(nil), node.segments...), suffix...)
	return a.internPath(node)
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

// Not returns the canonical structural complement of guard.  Negation is
// eliminated at construction: constants and truth atoms are flipped directly,
// while compound guards are rebuilt by De Morgan through the same hash-consed
// And/Or vocabulary.  Consequently there is no second runtime guard operation
// and Not(Not(g)) is the original canonical guard.
func (a *Arena) Not(guard Guard) Guard {
	if a == nil || guard == 0 || int(guard) >= len(a.guards) {
		return 0
	}
	type frame struct {
		guard    Guard
		expanded bool
	}
	memo := make(map[Guard]Guard)
	stack := []frame{{guard: guard}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if memo[current.guard] != 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		node := a.guards[current.guard]
		switch node.op {
		case guardTrue:
			memo[current.guard] = a.False()
		case guardFalse:
			memo[current.guard] = a.True()
		case guardTruthy:
			memo[current.guard] = a.Falsy(node.value)
		case guardFalsy:
			memo[current.guard] = a.Truthy(node.value)
		case guardAnd, guardOr:
			if !current.expanded {
				current.expanded = true
				for index := len(node.args) - 1; index >= 0; index-- {
					if memo[node.args[index]] == 0 {
						stack = append(stack, frame{guard: node.args[index]})
					}
				}
				continue
			}
			args := make([]Guard, len(node.args))
			for index, arg := range node.args {
				args[index] = memo[arg]
				if args[index] == 0 {
					return 0
				}
			}
			if node.op == guardAnd {
				memo[current.guard] = a.Or(args...)
			} else {
				memo[current.guard] = a.And(args...)
			}
		default:
			return 0
		}
		if memo[current.guard] == 0 {
			return 0
		}
		stack = stack[:len(stack)-1]
	}
	return memo[guard]
}

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
	for index, guard := range flat {
		node := a.guards[guard]
		if node.op != guardTruthy && node.op != guardFalsy {
			continue
		}
		for _, priorGuard := range flat[:index] {
			prior := a.guards[priorGuard]
			if prior.value == node.value && (prior.op == guardTruthy || prior.op == guardFalsy) && prior.op != node.op {
				if op == guardAnd {
					return a.False()
				}
				return a.True()
			}
		}
	}
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
	if a.sealed {
		return 0
	}
	n.args = append([]ValueTerm(nil), n.args...)
	if n.objectPlan.Valid() {
		n.objectPlan = n.objectPlan.Clone()
	}
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
	if a.sealed {
		return 0
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
	if a.sealed {
		return 0
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
	for _, part := range n.owner {
		h = internalhash.MixHash(h, uint64(part))
	}
	h = hashRoot(h, n.root)
	h = internalhash.MixHash(h, n.cell.Function)
	h = internalhash.MixHash(h, uint64(n.cell.Slot))
	h = internalhash.MixHash(h, uint64(n.frame))
	h = internalhash.MixHash(h, uint64(n.path))
	h = internalhash.MixHash(h, uint64(n.keyPath))
	h = internalhash.MixHash(h, uint64(n.rangePath))
	h = internalhash.MixHash(h, uint64(n.integerProof))
	h = internalhash.MixHash(h, uint64(n.indexShape.Kind()))
	if constant, ok := n.indexShape.Constant(); ok {
		h = internalhash.MixHash(h, uint64(constant))
	}
	if coeff, offset, ok := n.indexShape.Affine(); ok {
		h = internalhash.MixHash(h, uint64(coeff))
		h = internalhash.MixHash(h, uint64(offset))
	}
	h = internalhash.MixHash(h, uint64(n.iterator.Kind))
	h = internalhash.MixHash(h, uint64(n.iterator.Source.Index))
	h = internalhash.MixHash(h, uint64(int64(n.variableIndex)))
	if n.hasAsserted {
		h = internalhash.MixHash(h, 1)
		h = internalhash.MixHash(h, typ.EqualityHash(n.assertedType))
	}
	h = internalhash.MixHash(h, uint64(n.allocation))
	h = internalhash.MixHash(h, uint64(int64(n.resultIndex)))
	h = internalhash.MixHash(h, uint64(n.point))
	h = internalhash.MixHash(h, uint64(n.slot))
	h = internalhash.MixHash(h, internalhash.FnvString(n.operator))
	h = internalhash.MixHash(h, uint64(n.guard))
	h = hashValueTerms(h, n.args)
	if n.op == valueObjectLiteral {
		h = internalhash.MixHash(h, n.objectPlan.Fingerprint())
	}
	if n.op == valueExpressionRefinement {
		h = internalhash.MixHash(h, uint64(n.refinementMode))
	}
	if (n.op == valueConstant || n.op == valueRefinement || n.op == valueFalsyAbsentRefinement || n.op == valueExpressionRefinement) && a.reg != nil {
		h = internalhash.MixHash(h, product.Hash(a.reg, n.value))
	}
	return h
}

func callFrameFingerprint(n callFrameNode) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, 0x63616c6c6672616d)
	h = internalhash.MixHash(h, n.target.Function)
	h = internalhash.MixHash(h, uint64(n.target.Slot))
	h = internalhash.MixHash(h, uint64(n.variable))
	h = internalhash.MixHash(h, uint64(n.closureProducer))
	h = internalhash.MixHash(h, uint64(n.point))
	h = internalhash.MixHash(h, uint64(n.occurrence))
	// Preserve the historical fingerprint of every body which has no ambient
	// closure-conversion inputs. Ambient is an appended, tagged schema lane;
	// inserting its zero width into the old stream would churn all call-frame
	// identities despite no semantic change.
	for _, width := range []uint32{n.shape.Params, n.shape.Captures, n.shape.Globals, n.shape.Results, n.shape.HeapTemplates, n.resultCount} {
		h = internalhash.MixHash(h, uint64(width))
	}
	if n.shape.Ambients != 0 {
		h = internalhash.MixHash(h, 0x616d6269656e7473)
		h = internalhash.MixHash(h, uint64(n.shape.Ambients))
	}
	h = hashValueTerms(h, n.values)
	h = internalhash.MixHash(h, uint64(len(n.paths)))
	for _, path := range n.paths {
		h = internalhash.MixHash(h, uint64(path))
	}
	return h
}

func callFrameNodeEqual(x, y callFrameNode) bool {
	if x.target != y.target || x.variable != y.variable || x.closureProducer != y.closureProducer || x.point != y.point || x.occurrence != y.occurrence || x.shape != y.shape || x.resultCount != y.resultCount || len(x.values) != len(y.values) || len(x.paths) != len(y.paths) {
		return false
	}
	for i := range x.values {
		if x.values[i] != y.values[i] || x.paths[i] != y.paths[i] {
			return false
		}
	}
	return true
}

func loopMuFingerprint(n loopMuNode) uint64 {
	h := internalhash.MixHash(termFingerprintSeed, 0x6c6f6f706d75)
	h = internalhash.MixHash(h, uint64(n.head))
	h = internalhash.MixHash(h, uint64(n.parent))
	h = internalhash.MixHash(h, uint64(len(n.members)))
	for _, point := range n.members {
		h = internalhash.MixHash(h, uint64(point))
	}
	h = internalhash.MixHash(h, uint64(len(n.backedges)))
	for _, edge := range n.backedges {
		h = internalhash.MixHash(h, uint64(edge.from))
		h = internalhash.MixHash(h, uint64(edge.to))
	}
	return h
}

func loopMuNodeEqual(x, y loopMuNode) bool {
	if x.head != y.head || x.parent != y.parent || len(x.members) != len(y.members) || len(x.backedges) != len(y.backedges) {
		return false
	}
	for index := range x.members {
		if x.members[index] != y.members[index] {
			return false
		}
	}
	for index := range x.backedges {
		if x.backedges[index] != y.backedges[index] {
			return false
		}
	}
	return true
}

func pathFingerprint(n pathNode) uint64 {
	h := hashRoot(internalhash.MixHash(termFingerprintSeed, 0x70617468), n.root)
	h = internalhash.MixHash(h, uint64(n.environment))
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
	if x.op != y.op || x.owner != y.owner || x.root != y.root || x.cell != y.cell || x.frame != y.frame || x.path != y.path || x.keyPath != y.keyPath || x.rangePath != y.rangePath || x.indexShape != y.indexShape || x.integerProof != y.integerProof || x.iterator != y.iterator || x.variableIndex != y.variableIndex || x.hasAsserted != y.hasAsserted || x.allocation != y.allocation || x.resultIndex != y.resultIndex || x.point != y.point || x.slot != y.slot || x.operator != y.operator || x.guard != y.guard || x.refinementMode != y.refinementMode || len(x.args) != len(y.args) {
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
	if x.op == valueObjectLiteral && !x.objectPlan.Equal(y.objectPlan) {
		return false
	}
	if x.op == valueConstant || x.op == valueRefinement || x.op == valueFalsyAbsentRefinement || x.op == valueExpressionRefinement {
		return a.reg != nil && product.Equal(a.reg, x.value, y.value)
	}
	return true
}
func pathNodeEqual(x, y pathNode) bool {
	if x.root != y.root || x.environment != y.environment || len(x.segments) != len(y.segments) {
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

type IteratorProjectionResolver func(iteration.Iterator, int, product.Value) (product.Value, bool)
type FrameResultResolver func(callFrameTerm, int) (product.Value, bool)
type MiddleValueResolver func(Root) (product.Value, bool)
type MiddlePathResolver func(Root) (pathdom.Path, bool)

// SpecializationContext owns optional concrete evaluators. A term requiring a
// missing evaluator fails the entire specialization transaction.
type SpecializationContext struct {
	CellResult         CellResultResolver
	IteratorProjection IteratorProjectionResolver
	FrameResult        FrameResultResolver
	MiddleValue        MiddleValueResolver
	MiddlePath         MiddlePathResolver
	Environment        state.State
	HasEnvironment     bool
}

func (a *Arena) evalValue(term ValueTerm, cursor BindingCursor, context SpecializationContext) (product.Value, bool) {
	return a.evalValueCanonical(term, cursor, context)
}

// evalGuardPossibilities returns the two abstractly feasible outcomes of a
// guard.  Unlike evalGuard (which asks whether a guarded row may execute), it
// preserves the false possibility needed by SelectValue.  The equations are
// the exact abstract Boolean semantics of the existing Guard algebra.
func (a *Arena) evalGuardPossibilities(guard Guard, cursor BindingCursor, context SpecializationContext) (canTrue, canFalse, ok bool) {
	return a.evalGuardPossibilitiesWithLeaves(guard, a.concreteValueLeafResolver(cursor, context))
}

func exactConcatOperand(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && subtype.IsSubtype(t, boundaryConcatOperandObligationType())
}

func possibleConcatOperand(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if subtype.IsSubtype(t, boundaryConcatOperandObligationType()) {
		return true
	}
	switch current := t.(type) {
	case *typ.Optional:
		return possibleConcatOperandType(current.Inner)
	case *typ.Union:
		for _, member := range current.Members {
			if member != nil && possibleConcatOperandType(member) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return current.Base == kind.String || current.Base == kind.Number || current.Base == kind.Integer
	}
	return possibleConcatOperandType(t)
}

func possibleConcatOperandType(t typ.Type) bool {
	if t == nil || typ.IsNever(t) {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) || subtype.IsSubtype(t, boundaryConcatOperandObligationType()) {
		return true
	}
	switch current := t.(type) {
	case *typ.Optional:
		return possibleConcatOperandType(current.Inner)
	case *typ.Union:
		for _, member := range current.Members {
			if member != nil && possibleConcatOperandType(member) {
				return true
			}
		}
		return false
	case *typ.Literal:
		return current.Base == kind.String || current.Base == kind.Number || current.Base == kind.Integer
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Tuple, kind.Function, kind.Array, kind.Map, kind.Record, kind.ReadonlyMap:
		return false
	default:
		// Nominal/deferred/intersection types can refine to a concat operand.
		// Their diagnostic obligation remains the proof boundary.
		return true
	}
}

func (a *Arena) evalPath(term PathTerm, cursor BindingCursor) (pathdom.Path, bool) {
	return a.evalPathWithContext(term, cursor, SpecializationContext{})
}

func (a *Arena) evalPathWithContext(term PathTerm, cursor BindingCursor, context SpecializationContext) (pathdom.Path, bool) {
	return a.evalPathCanonicalWithRoot(term, func(root Root) (pathdom.Path, bool) {
		if root.Kind == RootMiddle {
			if context.MiddlePath == nil {
				return pathdom.Path{}, false
			}
			return context.MiddlePath(root)
		}
		return cursor.Path(root)
	})
}

// evalPathCanonicalWithRoot is the sole path-term interpreter. Formal tuple
// leaves supply a lexical root resolver directly; concrete specialization
// supplies its BindingCursor above. Path suffix order and environment syntax
// therefore cannot drift between the two execution surfaces.
func (a *Arena) evalPathCanonicalWithRoot(term PathTerm, rootAt func(Root) (pathdom.Path, bool)) (pathdom.Path, bool) {
	if term == 0 || int(term) >= len(a.paths) {
		return pathdom.Path{}, false
	}
	n := a.paths[term]
	if n.environment != 0 {
		if !a.validEnvironmentSlot(statekey.SymbolValue(n.environment)) {
			return pathdom.Path{}, false
		}
		return pathdom.Path{Symbol: n.environment, Segments: append([]segment.Segment(nil), n.segments...)}, true
	}
	var root pathdom.Path
	var ok bool
	if rootAt == nil {
		return pathdom.Path{}, false
	}
	root, ok = rootAt(n.root)
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
	case valueEnvironment:
		return fmt.Sprintf("env:%d", n.slot)
	case valueConstant:
		if a.reg == nil {
			return "c:nil"
		}
		return "c:" + strconv.FormatUint(product.Hash(a.reg, n.value), 16)
	case valueObjectLiteral:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalValue(x)
		}
		return "obj:" + strconv.FormatUint(n.objectPlan.Fingerprint(), 16) + "(" + strings.Join(parts, ",") + ")"
	case valueJoin:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalValue(x)
		}
		sort.Strings(parts)
		return "j(" + strings.Join(parts, ",") + ")"
	case valueSelect:
		return "sel(" + a.canonicalGuard(n.guard) + "," + a.canonicalValue(n.args[0]) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueRefinement:
		return "m(" + a.canonicalValue(n.args[0]) + "," + strconv.FormatUint(product.Hash(a.reg, n.value), 16) + ")"
	case valueFalsyAbsentRefinement:
		return "mf(" + a.canonicalValue(n.args[0]) + "," + strconv.FormatUint(product.Hash(a.reg, n.value), 16) + ")"
	case valueExpressionRefinement:
		return "er" + strconv.Itoa(int(n.refinementMode)) + "(" + a.canonicalValue(n.args[0]) + "," + strconv.FormatUint(product.Hash(a.reg, n.value), 16) + ")"
	case valueCellResult:
		parts := make([]string, len(n.args))
		for i, x := range n.args {
			parts[i] = a.canonicalValue(x)
		}
		return fmt.Sprintf("c%d.%d(%s)", n.cell.Function, n.cell.Slot, strings.Join(parts, ","))
	case valueCallResult:
		return fmt.Sprintf("cr%d.%d(%s)", n.point, n.resultIndex, a.canonicalValue(n.args[0]))
	case valueFrameResult:
		return fmt.Sprintf("fr(%s).%d", a.canonicalCallFrame(n.frame), n.resultIndex)
	case valueDynamicRead:
		return fmt.Sprintf("d%d(%s,%s,%s@%s;r=%s@%s;i=%s)", n.point, a.canonicalValue(n.args[0]), a.canonicalPath(n.path), a.canonicalValue(n.args[1]), a.canonicalPath(n.keyPath), canonicalIndexShape(n.indexShape), a.canonicalPath(n.rangePath), a.canonicalValue(n.integerProof))
	case valueDynamicTableRead:
		parts := []string{a.canonicalValue(n.args[0]), a.canonicalPath(n.path), a.canonicalValue(n.args[1]) + "@" + a.canonicalPath(n.keyPath), "r=" + canonicalIndexShape(n.indexShape) + "@" + a.canonicalPath(n.rangePath), "i=" + a.canonicalValue(n.integerProof)}
		return fmt.Sprintf("dt%d(%s)", n.point, strings.Join(parts, ","))
	case valueStringConcat:
		return "s(" + a.canonicalValue(n.args[0]) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueUnaryOperation:
		return canonicalScalarUnaryValue(n.operator, a.canonicalValue(n.args[0]))
	case valueBinaryOperation:
		return canonicalScalarBinaryValue(n.operator, a.canonicalValue(n.args[0]), a.canonicalValue(n.args[1]))
	case valueIteratorProjection:
		args := a.canonicalValue(n.args[0])
		if len(n.args) == 2 {
			args += "," + a.canonicalValue(n.args[1])
		}
		if !n.hasAsserted {
			return fmt.Sprintf("i%d.%d.%d(%s)", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, args)
		}
		return fmt.Sprintf("i%d.%d.%d.%x(%s)", n.iterator.Kind, n.iterator.Source.Index, n.variableIndex, n.assertedType.Hash(), args)
	case valueGenericForResult:
		parts := make([]string, len(n.args))
		for i, arg := range n.args {
			parts[i] = a.canonicalValue(arg)
		}
		return fmt.Sprintf("gf%d(%s)", n.variableIndex, strings.Join(parts, ","))
	case valueLoopContinuation:
		return fmt.Sprintf("loop:%s:%d", n.owner.String(), n.point)
	case valuePredicateObservation:
		return fmt.Sprintf("observe:%d(%s)", n.point, a.canonicalValue(n.args[0]))
	case valueStaticIndex:
		return "si(" + a.canonicalValue(n.args[0]) + "," + a.canonicalValue(n.args[1]) + ")"
	case valueAllocationResult:
		op := a.allocations[n.allocation].op
		return fmt.Sprintf("a%d.%s.%d:r%d", op.Site().Owner, op.Site().Template, op.Site().Ordinal, n.resultIndex)
	case valueLuaTypeName:
		return "lua:type(" + a.canonicalValue(n.args[0]) + ")"
	default:
		return "_"
	}
}

func canonicalIndexShape(shape indexform.IndexShape) string {
	if !shape.Valid() {
		return "_"
	}
	if constant, ok := shape.Constant(); ok {
		return fmt.Sprintf("c%d", constant)
	}
	if coeff, offset, ok := shape.Affine(); ok {
		return fmt.Sprintf("a%d:%d", coeff, offset)
	}
	return fmt.Sprintf("k%d", shape.Kind())
}

func (a *Arena) validValue(term ValueTerm, shape Shape, seen map[ValueTerm]bool) bool {
	if term == 0 || int(term) >= len(a.values) {
		return false
	}
	stack := []ValueTerm{term}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(a.values) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		n := a.values[current]
		if n.op == valueRoot && !a.validRoot(shape, n.root) {
			return false
		}
		if n.op == valueEnvironment && (len(n.args) != 0 || !a.validEnvironmentSlot(n.slot)) {
			return false
		}
		if n.op == valueObjectLiteral && (!n.objectPlan.Valid() || n.objectPlan.ValueSourceCount() != len(n.args)) {
			return false
		}
		if n.op == valueDynamicRead && (len(n.args) != 2 || !a.validPath(n.path, shape)) {
			return false
		}
		if n.keyPath != 0 && !a.validPath(n.keyPath, shape) {
			return false
		}
		if n.rangePath != 0 && !a.validPath(n.rangePath, shape) || n.integerProof != 0 && int(n.integerProof) >= len(a.values) {
			return false
		}
		if n.indexShape.Valid() {
			switch n.indexShape.Kind() {
			case indexform.IndexFormAffine:
				if n.rangePath == 0 {
					return false
				}
			case indexform.IndexFormModuloLength:
				if n.integerProof == 0 {
					return false
				}
			}
		} else if n.rangePath != 0 || n.integerProof != 0 {
			return false
		}
		if (n.op == valueRefinement || n.op == valueFalsyAbsentRefinement || n.op == valueExpressionRefinement) && len(n.args) != 1 {
			return false
		}
		if n.op == valueSelect && (len(n.args) != 2 || !a.validGuard(n.guard, shape)) {
			return false
		}
		if n.op == valueCallResult && (len(n.args) != 1 || n.resultIndex < 0) {
			return false
		}
		if n.op == valueFrameResult && (len(n.args) != 0 || n.frame == 0 || int(n.frame) >= len(a.callFrames) || n.resultIndex < 0 || uint32(n.resultIndex) >= a.callFrames[n.frame].resultCount) {
			return false
		}
		if n.op == valueDynamicTableRead && (len(n.args) != 2 || n.path != 0 && !a.validPath(n.path, shape)) {
			return false
		}
		if n.op == valueStringConcat && len(n.args) != 2 {
			return false
		}
		if n.op == valueUnaryOperation && (len(n.args) != 1 || !isPureUnaryOperator(n.operator)) {
			return false
		}
		if n.op == valueBinaryOperation && (len(n.args) != 2 || !isPureBinaryOperator(n.operator)) {
			return false
		}
		if n.op == valueIteratorProjection && ((len(n.args) != 1 && len(n.args) != 2) || n.variableIndex < 0 || n.variableIndex > 1 ||
			(n.iterator.Kind != iteration.IterateIndexed && n.iterator.Kind != iteration.IterateKeyed) || n.hasAsserted != (n.assertedType != nil)) {
			return false
		}
		if n.op == valueGenericForResult && (len(n.args) != 4 || n.variableIndex < 0) {
			return false
		}
		if n.op == valueLoopContinuation && (n.owner == (lexicalidentity.StableLexicalBodyID{}) || len(n.args) != 0) {
			return false
		}
		if n.op == valuePredicateObservation && len(n.args) != 1 {
			return false
		}
		if n.op == valueStaticIndex && (len(n.args) != 2 || !a.validStaticIndexKey(n.args[1])) {
			return false
		}
		if n.op == valueAllocationResult && (len(n.args) != 0 || !a.validAllocation(n.allocation) || n.resultIndex < 0) {
			return false
		}
		if n.op == valueLuaTypeName && len(n.args) != 1 {
			return false
		}
		if n.op == valueInvalid {
			return false
		}
		stack = append(stack, n.args...)
		if n.integerProof != 0 {
			stack = append(stack, n.integerProof)
		}
	}
	return true
}

func (a *Arena) canonicalCallFrame(term callFrameTerm) string {
	if a == nil || term == 0 || int(term) >= len(a.callFrames) {
		return "_"
	}
	n := a.callFrames[term]
	bindings := make([]string, len(n.values))
	for i := range n.values {
		bindings[i] = a.canonicalValue(n.values[i]) + "@" + a.canonicalPath(n.paths[i])
	}
	shape := fmt.Sprintf("[%d,%d,%d,%d,%d]", n.shape.Params, n.shape.Captures, n.shape.Globals, n.shape.Results, n.shape.HeapTemplates)
	if n.shape.Ambients != 0 {
		shape += fmt.Sprintf(";a=%d", n.shape.Ambients)
	}
	return fmt.Sprintf("%d.%d/v%d^%d@%d.%d%s->%d(%s)", n.target.Function, n.target.Slot, n.variable, n.closureProducer, n.point, n.occurrence, shape, n.resultCount, strings.Join(bindings, ","))
}

func (a *Arena) validCallFrame(term callFrameTerm, caller Shape, available map[callFrameTerm]struct{}) bool {
	if a == nil || term == 0 || int(term) >= len(a.callFrames) {
		return false
	}
	n := a.callFrames[term]
	if (n.target == (CellRef{})) == (n.variable == 0) || len(n.values) != n.shape.InputCount() || len(n.paths) != len(n.values) {
		return false
	}
	for i, value := range n.values {
		if !a.validValue(value, caller, make(map[ValueTerm]bool)) || !a.valueFramesOwned(value, available, make(map[ValueTerm]bool)) || (n.paths[i] != 0 && !a.validPath(n.paths[i], caller)) {
			return false
		}
	}
	if n.closureProducer != 0 {
		if _, owned := available[n.closureProducer]; !owned {
			return false
		}
	}
	return true
}

func (a *Arena) valueFramesOwned(term ValueTerm, frames map[callFrameTerm]struct{}, seen map[ValueTerm]bool) bool {
	stack := []ValueTerm{term}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(a.values) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		n := a.values[current]
		if n.guard != 0 && !a.guardFramesOwned(n.guard, frames, make(map[Guard]bool)) {
			return false
		}
		if n.op == valueFrameResult {
			if _, ok := frames[n.frame]; !ok {
				return false
			}
		}
		stack = append(stack, n.args...)
	}
	return true
}

func (a *Arena) guardFramesOwned(guard Guard, frames map[callFrameTerm]struct{}, seen map[Guard]bool) bool {
	stack := []Guard{guard}
	valueSeen := make(map[ValueTerm]bool)
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(a.guards) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		n := a.guards[current]
		if n.value != 0 && !a.valueFramesOwned(n.value, frames, valueSeen) {
			return false
		}
		stack = append(stack, n.args...)
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
	if n.environment != 0 {
		return fmt.Sprintf("envp:%d:%v", n.environment, n.segments)
	}
	return fmt.Sprintf("p%d.%d:%v", n.root.Kind, n.root.Index, n.segments)
}

func (a *Arena) validPath(term PathTerm, shape Shape) bool {
	if term == 0 || int(term) >= len(a.paths) {
		return false
	}
	n := a.paths[term]
	if n.environment != 0 {
		return n.root == (Root{}) && a.validEnvironmentSlot(statekey.SymbolValue(n.environment))
	}
	return a.validRoot(shape, n.root)
}

func (a *Arena) validGuard(guard Guard, shape Shape) bool {
	if guard == 0 || int(guard) >= len(a.guards) {
		return false
	}
	seen := make(map[Guard]bool)
	valueSeen := make(map[ValueTerm]bool)
	stack := []Guard{guard}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(a.guards) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		n := a.guards[current]
		if n.op == guardInvalid || n.value != 0 && !a.validValue(n.value, shape, valueSeen) {
			return false
		}
		stack = append(stack, n.args...)
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
	if n.guard != 0 && a.guardContainsCellResult(n.guard, make(map[Guard]bool)) {
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
