package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// TermRootBindings maps every callee boundary namespace into one caller arena.
// Value and path term IDs are caller-arena IDs. Every value is required; a zero
// path means that namespace root has no caller path. The binding is a
// term-rebasing foundation only: it does not compose Relation rows, SCC cells,
// effects, or Summary fields.
type TermRootBindings struct {
	callee Shape
	caller Shape
	values []ValueTerm
	paths  []PathTerm
}

// NewTermRootBindings takes an immutable snapshot of dense caller-arena term
// IDs. Values must have exactly callee.InputCount entries. Callee Result and
// HeapTemplate roots are lexical existentials and are never caller bindings.
// Paths may be nil
// when no source PathTerm is imported, or have the same packed width. Missing
// value roots are rejected; individual path roots may be zero and fail closed
// only when a source PathTerm references them.
func NewTermRootBindings(callee, caller Shape, values []ValueTerm, paths []PathTerm) (TermRootBindings, error) {
	if len(values) != callee.InputCount() {
		return TermRootBindings{}, fmt.Errorf("transformer: got %d value term bindings, want %d", len(values), callee.InputCount())
	}
	if paths != nil && len(paths) != callee.InputCount() {
		return TermRootBindings{}, fmt.Errorf("transformer: got %d path term bindings, want %d", len(paths), callee.InputCount())
	}
	for i, term := range values {
		if term == 0 {
			return TermRootBindings{}, fmt.Errorf("transformer: missing value term binding %d", i)
		}
	}
	return TermRootBindings{
		callee: callee,
		caller: caller,
		values: append([]ValueTerm(nil), values...),
		paths:  append([]PathTerm(nil), paths...),
	}, nil
}

func (b TermRootBindings) value(root Root) ValueTerm {
	if !b.callee.validateInput(root) {
		return 0
	}
	return b.values[b.callee.offset(root.Kind)+int(root.Index)]
}

func (b TermRootBindings) path(root Root) PathTerm {
	if b.paths == nil || !b.callee.validateInput(root) {
		return 0
	}
	return b.paths[b.callee.offset(root.Kind)+int(root.Index)]
}

// TermRebaseInput is a set of source-arena roots imported as one transaction.
// Output positions correspond exactly to input positions.
type TermRebaseInput struct {
	Values []ValueTerm
	Paths  []PathTerm
	Guards []Guard
}

// TermRebaseOutput contains caller-arena IDs. A failed transaction always
// returns the zero value and leaves the caller arena unchanged.
type TermRebaseOutput struct {
	Values []ValueTerm
	Paths  []PathTerm
	Guards []Guard
}

// RebaseTermDAGs transactionally imports value, path, and guard DAGs from a
// callee arena into a caller arena. Callee Roots are substituted by bindings;
// constants, joins, guards, and path suffixes are rebuilt through the caller's
// hash-consing tables. Scalar CellResult terms require relational composition
// and therefore fail closed here.
func RebaseTermDAGs(caller, callee *Arena, bindings TermRootBindings, input TermRebaseInput) (TermRebaseOutput, error) {
	return rebaseTermDAGs(caller, callee, bindings, input, false)
}

// rebaseDirectCallTermDAGs imports callee terms across one lexical call
// boundary. A callee-local CallResult identity is an observation of how the
// callee obtained a value, not part of that value's caller-visible identity.
// Strip those wrappers during import; direct-call composition installs exactly
// one identity for the caller's own source point and result slot afterwards.
// This also makes recursive call SCCs finite: each round cannot accumulate a
// fresh nesting of already-crossed lexical call identities.
func rebaseDirectCallTermDAGs(caller, callee *Arena, bindings TermRootBindings, input TermRebaseInput) (TermRebaseOutput, error) {
	return rebaseTermDAGs(caller, callee, bindings, input, true)
}

func rebaseTermDAGs(caller, callee *Arena, bindings TermRootBindings, input TermRebaseInput, dropCallResults bool) (TermRebaseOutput, error) {
	if caller == nil || callee == nil || caller.reg == nil || callee.reg == nil {
		return TermRebaseOutput{}, fmt.Errorf("transformer: term rebasing requires two registered arenas")
	}
	// product.Value equality and hashing are registry-defined. Importing a
	// constant across registry ownership would make hash-consing ambiguous.
	if caller.reg != callee.reg {
		return TermRebaseOutput{}, fmt.Errorf("transformer: term rebasing requires identical axis registry ownership")
	}
	check := rebaseValidator{caller: caller, callee: callee, bindings: bindings}
	if err := check.validate(input); err != nil {
		return TermRebaseOutput{}, err
	}

	// Validation above proves that these imports cannot fail. Consequently no
	// destination node is interned until the complete transaction is admissible.
	state := rebaseState{
		caller:          caller,
		callee:          callee,
		bindings:        bindings,
		values:          make(map[ValueTerm]ValueTerm),
		paths:           make(map[PathTerm]PathTerm),
		guards:          make(map[Guard]Guard),
		dropCallResults: dropCallResults,
	}
	out := TermRebaseOutput{
		Values: make([]ValueTerm, len(input.Values)),
		Paths:  make([]PathTerm, len(input.Paths)),
		Guards: make([]Guard, len(input.Guards)),
	}
	for i, term := range input.Values {
		out.Values[i] = state.value(term)
		if state.err != nil {
			return TermRebaseOutput{}, state.err
		}
	}
	for i, term := range input.Paths {
		out.Paths[i] = state.path(term)
		if state.err != nil {
			return TermRebaseOutput{}, state.err
		}
	}
	for i, guard := range input.Guards {
		out.Guards[i] = state.guard(guard)
		if state.err != nil {
			return TermRebaseOutput{}, state.err
		}
	}
	return out, nil
}

type visitState uint8

const (
	visitActive visitState = iota + 1
	visitDone
)

type rebaseValidator struct {
	caller   *Arena
	callee   *Arena
	bindings TermRootBindings
	values   map[ValueTerm]visitState
	guards   map[Guard]visitState
}

func (v *rebaseValidator) validate(input TermRebaseInput) error {
	v.values = make(map[ValueTerm]visitState)
	v.guards = make(map[Guard]visitState)
	if len(v.bindings.values) != v.bindings.callee.InputCount() {
		return fmt.Errorf("transformer: malformed value term binding width")
	}
	if v.bindings.paths != nil && len(v.bindings.paths) != v.bindings.callee.InputCount() {
		return fmt.Errorf("transformer: malformed path term binding width")
	}
	// Validate all supplied caller bindings, including unused entries. A
	// malformed binding object is never a partially trusted transaction.
	for i, term := range v.bindings.values {
		if term == 0 {
			return fmt.Errorf("transformer: missing caller value binding %d", i)
		}
		// A caller binding is an opaque, already-owned caller expression. It
		// may legitimately be an Environment, FrameResult, or CellResult term;
		// only source-callee terms are forbidden from exporting those nodes.
		// Reusing the boundary-export validator here conflated the two sides of
		// substitution and rejected valid composed arguments.
		if !v.caller.validValue(term, v.bindings.caller, make(map[ValueTerm]bool)) {
			return fmt.Errorf("transformer: caller value binding %d is not a valid caller expression", i)
		}
	}
	for i, term := range v.bindings.paths {
		if term != 0 {
			if err := validatePathTerm(v.caller, term, v.bindings.caller); err != nil {
				return fmt.Errorf("transformer: caller path binding %d: %w", i, err)
			}
		}
	}
	for _, term := range input.Values {
		if err := v.value(term); err != nil {
			return err
		}
	}
	for _, term := range input.Paths {
		if err := v.path(term); err != nil {
			return err
		}
	}
	for _, guard := range input.Guards {
		if err := v.guard(guard); err != nil {
			return err
		}
	}
	return nil
}

func (v *rebaseValidator) value(term ValueTerm) error {
	if term == 0 || int(term) >= len(v.callee.values) {
		return fmt.Errorf("transformer: invalid source value term %d", term)
	}
	switch v.values[term] {
	case visitActive:
		return fmt.Errorf("transformer: cyclic source value DAG at term %d", term)
	case visitDone:
		return nil
	}
	v.values[term] = visitActive
	n := v.callee.values[term]
	switch n.op {
	case valueRoot:
		if len(n.args) != 0 || !v.bindings.callee.validate(n.root) {
			return fmt.Errorf("transformer: invalid source boundary root %#v", n.root)
		}
		bound := v.bindings.value(n.root)
		if bound == 0 {
			return fmt.Errorf("transformer: missing value binding for source root %#v", n.root)
		}
	case valueConstant:
		if len(n.args) != 0 {
			return fmt.Errorf("transformer: malformed constant term %d", term)
		}
	case valueObjectLiteral:
		if !n.objectPlan.Valid() || n.objectPlan.ValueSourceCount() != len(n.args) {
			return fmt.Errorf("transformer: malformed object term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueJoin:
		if len(n.args) < 2 {
			return fmt.Errorf("transformer: malformed join term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueSelect:
		if len(n.args) != 2 || n.guard == 0 {
			return fmt.Errorf("transformer: malformed select term %d", term)
		}
		if err := v.guard(n.guard); err != nil {
			return err
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueRefinement, valueFalsyAbsentRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("transformer: malformed refinement term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueExpressionRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("transformer: malformed runtime validation term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueCellResult:
		return fmt.Errorf("transformer: scalar CellResult term %d requires relational composition", term)
	case valueCallResult:
		if len(n.args) != 1 || n.resultIndex < 0 {
			return fmt.Errorf("transformer: malformed call-result term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valuePredicateObservation:
		if len(n.args) != 1 {
			return fmt.Errorf("transformer: malformed predicate-observation term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueDynamicRead:
		if len(n.args) != 2 || n.path == 0 {
			return fmt.Errorf("transformer: malformed DynamicRead term %d", term)
		}
		if err := v.path(n.path); err != nil {
			return err
		}
		if n.keyPath != 0 {
			if err := v.optionalPath(n.keyPath); err != nil {
				return err
			}
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueDynamicTableRead:
		if len(n.args) != 2 {
			return fmt.Errorf("transformer: malformed direct DynamicRead term %d", term)
		}
		if n.path != 0 {
			if err := v.optionalPath(n.path); err != nil {
				return err
			}
		}
		if n.keyPath != 0 {
			if err := v.optionalPath(n.keyPath); err != nil {
				return err
			}
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueStringConcat:
		if len(n.args) != 2 {
			return fmt.Errorf("transformer: malformed string concat term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueUnaryOperation:
		if len(n.args) != 1 || !isPureUnaryOperator(n.operator) {
			return fmt.Errorf("transformer: malformed scalar unary term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueBinaryOperation:
		if len(n.args) != 2 || !isPureBinaryOperator(n.operator) {
			return fmt.Errorf("transformer: malformed scalar binary term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueIteratorProjection:
		if (len(n.args) != 1 && len(n.args) != 2) || n.variableIndex < 0 || n.variableIndex > 1 || n.hasAsserted != (n.assertedType != nil) {
			return fmt.Errorf("transformer: malformed iterator projection term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueGenericForResult:
		if len(n.args) != 4 || n.variableIndex < 0 {
			return fmt.Errorf("transformer: malformed generic-for result term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueLoopContinuation:
		if n.owner == (lexicalidentity.StableLexicalBodyID{}) || len(n.args) != 0 {
			return fmt.Errorf("transformer: malformed loop continuation term %d", term)
		}
	case valueStaticIndex:
		if len(n.args) != 2 || !v.callee.validStaticIndexKey(n.args[1]) {
			return fmt.Errorf("transformer: malformed static index term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueAllocationResult:
		if len(n.args) != 0 || !v.callee.validAllocation(n.allocation) || n.resultIndex < 0 {
			return fmt.Errorf("transformer: malformed allocation result term %d", term)
		}
	case valueLuaTypeName:
		if len(n.args) != 1 {
			return fmt.Errorf("transformer: malformed Lua type-name term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	default:
		return fmt.Errorf("transformer: invalid source value operation at term %d", term)
	}
	v.values[term] = visitDone
	return nil
}

func (v *rebaseValidator) path(term PathTerm) error {
	if term == 0 || int(term) >= len(v.callee.paths) {
		return fmt.Errorf("transformer: invalid source path term %d", term)
	}
	n := v.callee.paths[term]
	if n.environment != 0 {
		return fmt.Errorf("transformer: body-owned environment path %d cannot cross a call frame", n.environment)
	}
	if !v.bindings.callee.validate(n.root) {
		return fmt.Errorf("transformer: invalid source path root %#v", n.root)
	}
	if v.bindings.path(n.root) == 0 {
		return fmt.Errorf("transformer: missing path binding for source root %#v", n.root)
	}
	return nil
}

// optionalPath validates an embedded evidence lens without requiring the
// caller to own an address for a value-only argument. Direct-table reads are
// exact from their scalar table/key terms; these paths only add relational
// evidence. Explicitly projected paths and owner-path reads continue through
// path and therefore still require complete bindings.
func (v *rebaseValidator) optionalPath(term PathTerm) error {
	if term == 0 || int(term) >= len(v.callee.paths) {
		return fmt.Errorf("transformer: invalid optional source path term %d", term)
	}
	n := v.callee.paths[term]
	if n.environment != 0 {
		return nil
	}
	if !v.bindings.callee.validate(n.root) {
		return fmt.Errorf("transformer: invalid optional source path root %#v", n.root)
	}
	if v.bindings.path(n.root) == 0 {
		return nil
	}
	return v.path(term)
}

func (v *rebaseValidator) guard(guard Guard) error {
	if guard == 0 || int(guard) >= len(v.callee.guards) {
		return fmt.Errorf("transformer: invalid source guard %d", guard)
	}
	switch v.guards[guard] {
	case visitActive:
		return fmt.Errorf("transformer: cyclic source guard DAG at guard %d", guard)
	case visitDone:
		return nil
	}
	v.guards[guard] = visitActive
	n := v.callee.guards[guard]
	switch n.op {
	case guardTrue, guardFalse:
		if n.value != 0 || len(n.args) != 0 {
			return fmt.Errorf("transformer: malformed constant guard %d", guard)
		}
	case guardTruthy, guardFalsy:
		if n.value == 0 || len(n.args) != 0 {
			return fmt.Errorf("transformer: malformed predicate guard %d", guard)
		}
		if err := v.value(n.value); err != nil {
			return err
		}
	case guardAnd, guardOr:
		if n.value != 0 || len(n.args) < 2 {
			return fmt.Errorf("transformer: malformed logical guard %d", guard)
		}
		for _, arg := range n.args {
			if err := v.guard(arg); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("transformer: invalid source guard operation at guard %d", guard)
	}
	v.guards[guard] = visitDone
	return nil
}

func validateValueDAG(arena *Arena, term ValueTerm, shape Shape, seen map[ValueTerm]visitState, allowCell bool) error {
	if term == 0 || int(term) >= len(arena.values) {
		return fmt.Errorf("invalid value term %d", term)
	}
	switch seen[term] {
	case visitActive:
		return fmt.Errorf("cyclic value DAG at term %d", term)
	case visitDone:
		return nil
	}
	seen[term] = visitActive
	n := arena.values[term]
	switch n.op {
	case valueRoot:
		if !shape.validate(n.root) || len(n.args) != 0 {
			return fmt.Errorf("invalid boundary root %#v", n.root)
		}
	case valueConstant:
		if len(n.args) != 0 {
			return fmt.Errorf("malformed constant term %d", term)
		}
	case valueObjectLiteral:
		if !n.objectPlan.Valid() || n.objectPlan.ValueSourceCount() != len(n.args) {
			return fmt.Errorf("malformed object term %d", term)
		}
	case valueJoin:
		if len(n.args) < 2 {
			return fmt.Errorf("malformed join term %d", term)
		}
	case valueSelect:
		if len(n.args) != 2 || n.guard == 0 {
			return fmt.Errorf("malformed select term %d", term)
		}
		if err := validateGuardDAG(arena, n.guard, shape, seen, make(map[Guard]visitState), allowCell); err != nil {
			return err
		}
	case valueRefinement, valueFalsyAbsentRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("malformed refinement term %d", term)
		}
	case valueExpressionRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("malformed runtime validation term %d", term)
		}
	case valueCellResult:
		if !allowCell {
			return fmt.Errorf("unsupported value operation at term %d", term)
		}
	case valueCallResult:
		if len(n.args) != 1 || n.resultIndex < 0 {
			return fmt.Errorf("malformed call-result term %d", term)
		}
	case valuePredicateObservation:
		if len(n.args) != 1 {
			return fmt.Errorf("malformed predicate-observation term %d", term)
		}
	case valueDynamicRead:
		if len(n.args) != 2 {
			return fmt.Errorf("malformed DynamicRead term %d", term)
		}
		if err := validatePathTerm(arena, n.path, shape); err != nil {
			return err
		}
	case valueDynamicTableRead:
		if len(n.args) != 2 {
			return fmt.Errorf("malformed direct DynamicRead term %d", term)
		}
		if n.path != 0 {
			if err := validatePathTerm(arena, n.path, shape); err != nil {
				return err
			}
		}
	case valueStringConcat:
		if len(n.args) != 2 {
			return fmt.Errorf("malformed string concat term %d", term)
		}
	case valueUnaryOperation:
		if len(n.args) != 1 || !isPureUnaryOperator(n.operator) {
			return fmt.Errorf("malformed scalar unary term %d", term)
		}
	case valueBinaryOperation:
		if len(n.args) != 2 || !isPureBinaryOperator(n.operator) {
			return fmt.Errorf("malformed scalar binary term %d", term)
		}
	case valueIteratorProjection:
		if (len(n.args) != 1 && len(n.args) != 2) || n.variableIndex < 0 || n.variableIndex > 1 || n.hasAsserted != (n.assertedType != nil) {
			return fmt.Errorf("malformed iterator projection term %d", term)
		}
	case valueGenericForResult:
		if len(n.args) != 4 || n.variableIndex < 0 {
			return fmt.Errorf("malformed generic-for result term %d", term)
		}
	case valueLoopContinuation:
		if n.owner == (lexicalidentity.StableLexicalBodyID{}) || len(n.args) != 0 {
			return fmt.Errorf("malformed loop continuation term %d", term)
		}
	case valueStaticIndex:
		if len(n.args) != 2 || !arena.validStaticIndexKey(n.args[1]) {
			return fmt.Errorf("malformed static index term %d", term)
		}
	case valueAllocationResult:
		if len(n.args) != 0 || !arena.validAllocation(n.allocation) || n.resultIndex < 0 {
			return fmt.Errorf("malformed allocation result term %d", term)
		}
	case valueLuaTypeName:
		if len(n.args) != 1 {
			return fmt.Errorf("malformed Lua type-name term %d", term)
		}
	default:
		return fmt.Errorf("invalid value operation at term %d", term)
	}
	for _, arg := range n.args {
		if err := validateValueDAG(arena, arg, shape, seen, allowCell); err != nil {
			return err
		}
	}
	seen[term] = visitDone
	return nil
}

func validateGuardDAG(arena *Arena, guard Guard, shape Shape, values map[ValueTerm]visitState, guards map[Guard]visitState, allowCell bool) error {
	if guard == 0 || int(guard) >= len(arena.guards) {
		return fmt.Errorf("invalid guard %d", guard)
	}
	switch guards[guard] {
	case visitActive:
		return fmt.Errorf("cyclic guard DAG at guard %d", guard)
	case visitDone:
		return nil
	}
	guards[guard] = visitActive
	n := arena.guards[guard]
	switch n.op {
	case guardTrue, guardFalse:
		if n.value != 0 || len(n.args) != 0 {
			return fmt.Errorf("malformed constant guard %d", guard)
		}
	case guardTruthy, guardFalsy:
		if n.value == 0 || len(n.args) != 0 {
			return fmt.Errorf("malformed predicate guard %d", guard)
		}
		if err := validateValueDAG(arena, n.value, shape, values, allowCell); err != nil {
			return err
		}
	case guardAnd, guardOr:
		if n.value != 0 || len(n.args) < 2 {
			return fmt.Errorf("malformed logical guard %d", guard)
		}
		for _, arg := range n.args {
			if err := validateGuardDAG(arena, arg, shape, values, guards, allowCell); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid guard operation at guard %d", guard)
	}
	guards[guard] = visitDone
	return nil
}

func validatePathTerm(arena *Arena, term PathTerm, shape Shape) error {
	if term == 0 || int(term) >= len(arena.paths) {
		return fmt.Errorf("invalid path term %d", term)
	}
	n := arena.paths[term]
	if n.environment != 0 {
		if n.root != (Root{}) || !arena.validEnvironmentSlot(statekey.SymbolValue(n.environment)) {
			return fmt.Errorf("invalid environment path root %d", n.environment)
		}
		return nil
	}
	if !shape.validate(n.root) {
		return fmt.Errorf("invalid path boundary root %#v", arena.paths[term].root)
	}
	return nil
}

type rebaseState struct {
	caller          *Arena
	callee          *Arena
	bindings        TermRootBindings
	values          map[ValueTerm]ValueTerm
	paths           map[PathTerm]PathTerm
	guards          map[Guard]Guard
	dropCallResults bool
	err             error
}

func (s *rebaseState) value(term ValueTerm) ValueTerm {
	if got := s.values[term]; got != 0 {
		return got
	}
	n := s.callee.values[term]
	var out ValueTerm
	switch n.op {
	case valueRoot:
		out = s.bindings.value(n.root)
	case valueConstant:
		out = s.caller.Constant(n.value)
	case valueObjectLiteral:
		args := make([]ValueTerm, len(n.args))
		for index, arg := range n.args {
			args[index] = s.value(arg)
		}
		out = s.caller.ObjectLiteralValue(n.objectPlan, args...)
	case valueJoin:
		args := make([]ValueTerm, len(n.args))
		for i, arg := range n.args {
			args[i] = s.value(arg)
		}
		out = s.caller.JoinValue(args...)
	case valueSelect:
		out = s.caller.SelectValue(s.guard(n.guard), s.value(n.args[0]), s.value(n.args[1]))
		if out == 0 {
			s.err = fmt.Errorf("transformer: validated select term %d failed to rebase", term)
			return 0
		}
	case valueRefinement:
		var ok bool
		out, ok = s.caller.RefineValue(s.value(n.args[0]), factflow.NewValueConstraint(n.value))
		if !ok {
			s.err = fmt.Errorf("transformer: validated refinement term %d failed to rebase", term)
			return 0
		}
	case valueFalsyAbsentRefinement:
		var ok bool
		out, ok = s.caller.RefineValue(s.value(n.args[0]), factflow.NewFalsyAbsentConstraint(n.value))
		if !ok {
			s.err = fmt.Errorf("transformer: validated falsy-absent refinement term %d failed to rebase", term)
			return 0
		}
	case valueExpressionRefinement:
		out = s.caller.expressionRefinementValue(s.value(n.args[0]), n.expressionRefinement())
		if out == 0 {
			s.err = fmt.Errorf("transformer: validated expression refinement term %d failed to rebase", term)
			return 0
		}
	case valueCallResult:
		produced := s.value(n.args[0])
		if s.dropCallResults {
			out = produced
		} else {
			out = s.caller.CallResultValue(n.point, uint32(n.resultIndex), produced)
		}
		if out == 0 {
			s.err = fmt.Errorf("transformer: validated call-result term %d failed to rebase", term)
			return 0
		}
	case valuePredicateObservation:
		out = s.caller.predicateObservationValue(n.point, s.value(n.args[0]))
		if out == 0 {
			s.err = fmt.Errorf("transformer: predicate-observation term %d failed to rebase", term)
			return 0
		}
	case valueDynamicRead:
		keyPath := PathTerm(0)
		if n.keyPath != 0 {
			keyPath = s.optionalPath(n.keyPath)
		}
		shape, rangePath, integerProof := s.dynamicIndexEvidence(n)
		out = s.caller.dynamicReadValueAtPaths(n.point, s.value(n.args[0]), s.path(n.path), s.value(n.args[1]), keyPath, shape, rangePath, integerProof)
	case valueDynamicTableRead:
		path := PathTerm(0)
		if n.path != 0 {
			path = s.optionalPath(n.path)
		}
		keyPath := PathTerm(0)
		if n.keyPath != 0 {
			keyPath = s.optionalPath(n.keyPath)
		}
		shape, rangePath, integerProof := s.dynamicIndexEvidence(n)
		out = s.caller.dynamicReadTableValueAtPaths(n.point, s.value(n.args[0]), path, s.value(n.args[1]), keyPath, shape, rangePath, integerProof)
	case valueStringConcat:
		out = s.caller.StringConcatValue(s.value(n.args[0]), s.value(n.args[1]))
	case valueUnaryOperation:
		out, _ = s.caller.ScalarUnaryValue(n.operator, s.value(n.args[0]))
	case valueBinaryOperation:
		out = s.caller.scalarBinaryValue(n.operator, s.value(n.args[0]), s.value(n.args[1]))
	case valueIteratorProjection:
		fallback := ValueTerm(0)
		if len(n.args) == 2 {
			fallback = s.value(n.args[1])
		}
		out = s.caller.iteratorProjectionValueWithFallback(n.iterator, n.variableIndex, s.value(n.args[0]), fallback, n.assertedType, n.hasAsserted)
	case valueGenericForResult:
		out = s.caller.genericForResultValue(n.variableIndex, s.value(n.args[0]), s.value(n.args[1]), s.value(n.args[2]), s.value(n.args[3]))
	case valueLoopContinuation:
		out = s.caller.loopContinuationValueOwned(n.owner, n.point)
	case valueStaticIndex:
		owner := s.value(n.args[0])
		keyNode := s.callee.values[n.args[1]]
		if keyNode.op != valueConstant {
			s.err = fmt.Errorf("transformer: static index key %d is not canonical constant", n.args[1])
			return 0
		}
		member, ok := typevalue.ExactScalarKeySegment(s.callee.reg, nil, keyNode.value)
		if !ok {
			s.err = fmt.Errorf("transformer: static index key %d is not exact", n.args[1])
			return 0
		}
		out = s.caller.StaticIndexValue(owner, member)
	case valueAllocationResult:
		allocation := s.caller.AllocationTemplate(s.callee.allocations[n.allocation].op)
		out = s.caller.AllocationResultValue(allocation, n.resultIndex)
	case valueLuaTypeName:
		out = s.caller.LuaTypeNameValue(s.value(n.args[0]))
	}
	s.values[term] = out
	return out
}

// dynamicIndexEvidence rebases the normalized range tuple atomically. A lost
// optional path/certificate removes only the range optimization; retaining an
// operator whose required witness disappeared would create malformed symbolic
// syntax and a parallel interpretation of IndexShape validity.
func (s *rebaseState) dynamicIndexEvidence(n valueNode) (indexform.IndexShape, PathTerm, ValueTerm) {
	shape := n.indexShape
	rangePath := PathTerm(0)
	if n.rangePath != 0 {
		rangePath = s.optionalPath(n.rangePath)
	}
	integerProof := ValueTerm(0)
	if n.integerProof != 0 {
		integerProof = s.value(n.integerProof)
	}
	if !shape.Valid() {
		return indexform.IndexShape{}, 0, 0
	}
	switch shape.Kind() {
	case indexform.IndexFormAffine:
		if rangePath == 0 {
			return indexform.IndexShape{}, 0, 0
		}
	case indexform.IndexFormModuloLength:
		if integerProof == 0 {
			return indexform.IndexShape{}, 0, 0
		}
	}
	return shape, rangePath, integerProof
}

func (s *rebaseState) path(term PathTerm) PathTerm {
	if got := s.paths[term]; got != 0 {
		return got
	}
	n := s.callee.paths[term]
	if n.environment != 0 {
		s.err = fmt.Errorf("transformer: body-owned environment path %d cannot be rebased", n.environment)
		return 0
	}
	base := s.caller.paths[s.bindings.path(n.root)]
	suffix := make([]segment.Segment, 0, len(base.segments)+len(n.segments))
	suffix = append(suffix, base.segments...)
	suffix = append(suffix, n.segments...)
	out := s.caller.Path(base.root, suffix...)
	s.paths[term] = out
	return out
}

func (s *rebaseState) optionalPath(term PathTerm) PathTerm {
	if term == 0 {
		return 0
	}
	n := s.callee.paths[term]
	if n.environment != 0 {
		// Same-body relation composition keeps the lexical environment slot in
		// the destination arena. Preserve that exact address instead of erasing
		// optional flow evidence. A true cross-body application has no such slot
		// and therefore still drops the unexportable callee-local path.
		if !s.caller.validEnvironmentSlot(statekey.SymbolValue(n.environment)) {
			return 0
		}
		out := s.caller.EnvironmentPath(n.environment, n.segments...)
		s.paths[term] = out
		return out
	}
	if s.bindings.path(n.root) == 0 {
		return 0
	}
	return s.path(term)
}

func (s *rebaseState) guard(guard Guard) Guard {
	if got := s.guards[guard]; got != 0 {
		return got
	}
	n := s.callee.guards[guard]
	var out Guard
	switch n.op {
	case guardTrue:
		out = s.caller.True()
	case guardFalse:
		out = s.caller.False()
	case guardTruthy:
		out = s.rebasedTruthGuard(n.value, true)
	case guardFalsy:
		out = s.rebasedTruthGuard(n.value, false)
	case guardAnd, guardOr:
		args := make([]Guard, len(n.args))
		for i, arg := range n.args {
			args[i] = s.guard(arg)
		}
		if n.op == guardAnd {
			out = s.caller.And(args...)
		} else {
			out = s.caller.Or(args...)
		}
	}
	s.guards[guard] = out
	return out
}

func (s *rebaseState) rebasedTruthGuard(source ValueTerm, truthy bool) Guard {
	value := s.value(source)
	if value != 0 && int(value) < len(s.caller.values) {
		node := s.caller.values[value]
		if node.op == valueConstant {
			canTruthy := valuerefine.CanBeTruthy(s.caller.reg, node.value)
			canFalsy := valuerefine.CanBeFalsy(s.caller.reg, node.value)
			if truthy && !canTruthy || !truthy && !canFalsy {
				return s.caller.False()
			}
			if truthy && !canFalsy || !truthy && !canTruthy {
				return s.caller.True()
			}
		}
	}
	if truthy {
		return s.caller.Truthy(value)
	}
	return s.caller.Falsy(value)
}
