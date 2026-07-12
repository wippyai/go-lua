package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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
// IDs. Values must have exactly callee.ValueCount entries. Paths may be nil
// when no source PathTerm is imported, or have the same packed width. Missing
// value roots are rejected; individual path roots may be zero and fail closed
// only when a source PathTerm references them.
func NewTermRootBindings(callee, caller Shape, values []ValueTerm, paths []PathTerm) (TermRootBindings, error) {
	if len(values) != callee.ValueCount() {
		return TermRootBindings{}, fmt.Errorf("transformer: got %d value term bindings, want %d", len(values), callee.ValueCount())
	}
	if paths != nil && len(paths) != callee.ValueCount() {
		return TermRootBindings{}, fmt.Errorf("transformer: got %d path term bindings, want %d", len(paths), callee.ValueCount())
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
	if !b.callee.validate(root) {
		return 0
	}
	return b.values[b.callee.offset(root.Kind)+int(root.Index)]
}

func (b TermRootBindings) path(root Root) PathTerm {
	if b.paths == nil || !b.callee.validate(root) {
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
		caller:   caller,
		callee:   callee,
		bindings: bindings,
		values:   make(map[ValueTerm]ValueTerm),
		paths:    make(map[PathTerm]PathTerm),
		guards:   make(map[Guard]Guard),
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
	if len(v.bindings.values) != v.bindings.callee.ValueCount() {
		return fmt.Errorf("transformer: malformed value term binding width")
	}
	if v.bindings.paths != nil && len(v.bindings.paths) != v.bindings.callee.ValueCount() {
		return fmt.Errorf("transformer: malformed path term binding width")
	}
	// Validate all supplied caller bindings, including unused entries. A
	// malformed binding object is never a partially trusted transaction.
	for i, term := range v.bindings.values {
		if term == 0 {
			return fmt.Errorf("transformer: missing caller value binding %d", i)
		}
		if err := validateValueDAG(v.caller, term, v.bindings.caller, make(map[ValueTerm]visitState), false); err != nil {
			return fmt.Errorf("transformer: caller value binding %d: %w", i, err)
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
	case valueJoin:
		if len(n.args) < 2 {
			return fmt.Errorf("transformer: malformed join term %d", term)
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("transformer: malformed refinement term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueCellResult:
		return fmt.Errorf("transformer: scalar CellResult term %d requires relational composition", term)
	case valueDynamicRead:
		if len(n.args) != 2 || n.path == 0 {
			return fmt.Errorf("transformer: malformed DynamicRead term %d", term)
		}
		if err := v.path(n.path); err != nil {
			return err
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueDynamicTableRead:
		if (len(n.args) != 2 && len(n.args) != 3) || n.path == 0 {
			return fmt.Errorf("transformer: malformed direct DynamicRead term %d", term)
		}
		if err := v.path(n.path); err != nil {
			return err
		}
		for _, arg := range n.args {
			if err := v.value(arg); err != nil {
				return err
			}
		}
	case valueIteratorProjection:
		if len(n.args) != 1 || n.variableIndex < 0 || n.variableIndex > 1 {
			return fmt.Errorf("transformer: malformed iterator projection term %d", term)
		}
		if err := v.value(n.args[0]); err != nil {
			return err
		}
	case valueAllocationResult:
		if len(n.args) != 0 || !v.callee.validAllocation(n.allocation) || n.resultIndex < 0 {
			return fmt.Errorf("transformer: malformed allocation result term %d", term)
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
	if !v.bindings.callee.validate(n.root) {
		return fmt.Errorf("transformer: invalid source path root %#v", n.root)
	}
	if v.bindings.path(n.root) == 0 {
		return fmt.Errorf("transformer: missing path binding for source root %#v", n.root)
	}
	return nil
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
	case valueJoin:
		if len(n.args) < 2 {
			return fmt.Errorf("malformed join term %d", term)
		}
	case valueRefinement:
		if len(n.args) != 1 {
			return fmt.Errorf("malformed refinement term %d", term)
		}
	case valueCellResult:
		if !allowCell {
			return fmt.Errorf("unsupported value operation at term %d", term)
		}
	case valueDynamicRead:
		if len(n.args) != 2 {
			return fmt.Errorf("malformed DynamicRead term %d", term)
		}
		if err := validatePathTerm(arena, n.path, shape); err != nil {
			return err
		}
	case valueDynamicTableRead:
		if len(n.args) != 2 && len(n.args) != 3 {
			return fmt.Errorf("malformed direct DynamicRead term %d", term)
		}
		if err := validatePathTerm(arena, n.path, shape); err != nil {
			return err
		}
	case valueIteratorProjection:
		if len(n.args) != 1 || n.variableIndex < 0 || n.variableIndex > 1 {
			return fmt.Errorf("malformed iterator projection term %d", term)
		}
	case valueAllocationResult:
		if len(n.args) != 0 || !arena.validAllocation(n.allocation) || n.resultIndex < 0 {
			return fmt.Errorf("malformed allocation result term %d", term)
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

func validatePathTerm(arena *Arena, term PathTerm, shape Shape) error {
	if term == 0 || int(term) >= len(arena.paths) {
		return fmt.Errorf("invalid path term %d", term)
	}
	if !shape.validate(arena.paths[term].root) {
		return fmt.Errorf("invalid path boundary root %#v", arena.paths[term].root)
	}
	return nil
}

type rebaseState struct {
	caller   *Arena
	callee   *Arena
	bindings TermRootBindings
	values   map[ValueTerm]ValueTerm
	paths    map[PathTerm]PathTerm
	guards   map[Guard]Guard
	err      error
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
	case valueJoin:
		args := make([]ValueTerm, len(n.args))
		for i, arg := range n.args {
			args[i] = s.value(arg)
		}
		out = s.caller.JoinValue(args...)
	case valueRefinement:
		var ok bool
		out, ok = s.caller.RefineValue(s.value(n.args[0]), factflow.NewValueConstraint(n.value))
		if !ok {
			s.err = fmt.Errorf("transformer: validated refinement term %d failed to rebase", term)
			return 0
		}
	case valueDynamicRead:
		out = s.caller.DynamicReadValue(s.value(n.args[0]), s.path(n.path), s.value(n.args[1]))
	case valueDynamicTableRead:
		if len(n.args) == 3 {
			out = s.caller.DynamicReadTableValueOr(s.value(n.args[0]), s.path(n.path), s.value(n.args[1]), s.value(n.args[2]))
		} else {
			out = s.caller.DynamicReadTableValue(s.value(n.args[0]), s.path(n.path), s.value(n.args[1]))
		}
	case valueIteratorProjection:
		out = s.caller.IteratorProjectionValue(n.iterator, n.variableIndex, s.value(n.args[0]))
	case valueAllocationResult:
		allocation := s.caller.AllocationTemplate(s.callee.allocations[n.allocation].op)
		out = s.caller.AllocationResultValue(allocation, n.resultIndex)
	}
	s.values[term] = out
	return out
}

func (s *rebaseState) path(term PathTerm) PathTerm {
	if got := s.paths[term]; got != 0 {
		return got
	}
	n := s.callee.paths[term]
	base := s.caller.paths[s.bindings.path(n.root)]
	suffix := make([]segment.Segment, 0, len(base.segments)+len(n.segments))
	suffix = append(suffix, base.segments...)
	suffix = append(suffix, n.segments...)
	out := s.caller.Path(base.root, suffix...)
	s.paths[term] = out
	return out
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
		out = s.caller.Truthy(s.value(n.value))
	case guardFalsy:
		out = s.caller.Falsy(s.value(n.value))
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
