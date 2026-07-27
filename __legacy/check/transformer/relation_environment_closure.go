package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// relationTermClosureErrorKind classifies a failure to close a body-owned term
// into the callable boundary vocabulary.  Callers may use errors.As to route a
// missing producer back to the corresponding compiler family; no failure is
// recoverable by evaluating the term against a concrete State.
type relationTermClosureErrorKind uint8

const (
	relationTermClosureMalformed relationTermClosureErrorKind = iota + 1
	relationTermClosureUnresolvedEnvironment
	relationTermClosureUnresolvedCell
	relationTermClosureUnresolvedFrame
	relationTermClosureUnsupportedPath
)

type relationTermClosureError struct {
	kind relationTermClosureErrorKind
	term ValueTerm
	path PathTerm
	slot statekey.Value
	cell CellRef
	why  string
}

func (e *relationTermClosureError) Error() string {
	if e == nil {
		return "transformer: nil relation term closure error"
	}
	switch e.kind {
	case relationTermClosureUnresolvedEnvironment:
		return fmt.Sprintf("transformer: relation term %d retains unresolved environment slot %d", e.term, e.slot)
	case relationTermClosureUnresolvedCell:
		return fmt.Sprintf("transformer: relation term %d retains unresolved cell %d.%d", e.term, e.cell.Function, e.cell.Slot)
	case relationTermClosureUnresolvedFrame:
		return fmt.Sprintf("transformer: relation term %d retains an uncomposed call frame", e.term)
	case relationTermClosureUnsupportedPath:
		return fmt.Sprintf("transformer: relation path %d retains unresolved environment slot %d", e.path, e.slot)
	default:
		if e.why != "" {
			return "transformer: malformed relation term closure: " + e.why
		}
		return "transformer: malformed relation term closure"
	}
}

// relationScalarClosure is a closed scalar result of one already-built
// lexical equation. Shape names its formal input roots. FrameResult is not a
// scalar closure: it remains an exact selector over the post-Apply world.
type relationScalarClosure struct {
	arena  *Arena
	shape  Shape
	result ValueTerm
}

// relationTermClosure is the freeze-time substitution transaction for one
// lexical body. It is deliberately not retained by relationCode. Raw lexical
// selectors close into the declared IN roots or the body's sealed MID register
// roots; the formal relation cell, not a renamed root, owns each version.
type relationTermClosure struct {
	arena       *Arena
	shape       Shape
	environment map[statekey.Value]ValueTerm
	paths       map[symbol.ID]PathTerm
	cells       map[CellRef]relationScalarClosure
}

// newRelationTermClosure derives the unique formal environment-to-root map.
// There is intentionally no API for binding an arbitrary environment slot:
// adding one would make an unresolved local look caller-supplied and recreate
// the concrete State transducer at the call boundary.
func newRelationTermClosure(arena *Arena, shape Shape, plan *operationplan.Plan, ambients []AmbientRoot, cells map[CellRef]relationScalarClosure) (relationTermClosure, error) {
	if arena == nil || arena.reg == nil || arena.Sealed() || plan == nil {
		return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "closure requires an open registered arena and exact plan"}
	}
	if !arena.middle.sealed {
		if err := arena.sealMiddleRegisterSchema(); err != nil {
			return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: err.Error()}
		}
	}
	if !validAmbientRoots(ambients) || len(ambients) != int(shape.Ambients) {
		return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: fmt.Sprintf("%d ambient roots, want canonical inventory of width %d", len(ambients), shape.Ambients)}
	}
	ambientSymbols := make([]symbol.ID, len(ambients))
	for index, root := range ambients {
		ambientSymbols[index] = root.Symbol
	}
	formal := []struct {
		kind    RootKind
		symbols []symbol.ID
		width   uint32
	}{
		{RootParam, plan.BoundaryParams(), shape.Params},
		{RootCapture, plan.BoundaryCaptures(), shape.Captures},
		{RootGlobal, plan.BoundaryGlobals(), shape.Globals},
		{RootAmbient, ambientSymbols, shape.Ambients},
	}
	out := relationTermClosure{
		arena: arena, shape: shape,
		environment: make(map[statekey.Value]ValueTerm, len(arena.middle.registers)),
		paths:       make(map[symbol.ID]PathTerm, len(arena.middle.registers)),
		cells:       make(map[CellRef]relationScalarClosure, len(cells)),
	}
	// Enumerate the sealed schema, never the source map: term interning order is
	// part of deterministic relation identity.
	for _, register := range arena.middle.registers {
		value, exact := arena.middleValue(register.slot)
		if !exact || value == 0 {
			return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, slot: register.slot, why: "Middle register has no canonical value root"}
		}
		out.environment[register.slot] = value
		if register.kind == relationMiddleRegisterSymbol {
			path := arena.middleSymbolPath(register.symbol)
			if path == 0 {
				return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, slot: register.slot, why: "Middle symbol register has no canonical path root"}
			}
			out.paths[register.symbol] = path
		}
	}
	entries := make([]relationMiddleEntry, 0, shape.InputCount())
	for _, namespace := range formal {
		if len(namespace.symbols) != int(namespace.width) {
			return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: fmt.Sprintf("%d formal symbols, want %d in namespace %d", len(namespace.symbols), namespace.width, namespace.kind)}
		}
		for index, id := range namespace.symbols {
			if id == 0 {
				return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "formal environment contains zero symbol"}
			}
			slot := statekey.SymbolValue(id)
			middle, exact := arena.middleRoot(slot)
			if !exact || out.environment[slot] == 0 || out.paths[id] == 0 {
				return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureUnresolvedEnvironment, slot: slot}
			}
			input := Root{Kind: namespace.kind, Index: uint32(index)}
			if arena.Root(input) == 0 || arena.Path(input) == 0 {
				return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, slot: slot, why: fmt.Sprintf("formal symbol %d has no canonical boundary root", id)}
			}
			entries = append(entries, relationMiddleEntry{middle: middle, input: input})
		}
	}
	if err := arena.middle.bindInputs(shape, entries); err != nil {
		return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: err.Error()}
	}
	for cell, closure := range cells {
		if cell == (CellRef{}) || closure.arena == nil || closure.arena.reg != arena.reg || closure.result == 0 || int(closure.result) >= len(closure.arena.values) {
			return relationTermClosure{}, &relationTermClosureError{kind: relationTermClosureMalformed, cell: cell, why: "cell closure has invalid ownership"}
		}
		out.cells[cell] = closure
	}
	return out, nil
}

// close rewrites a set of same-arena roots as one freeze transaction.  The
// result contains only boundary roots and closed expression nodes, so the
// ordinary RebaseTermDAGs validator is the final proof that it can cross a
// call frame without a concrete State callback.
func (c relationTermClosure) close(input TermRebaseInput) (TermRebaseOutput, error) {
	if c.arena == nil || c.arena.reg == nil || c.arena.Sealed() {
		return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "term owner is nil or sealed"}
	}
	scan := relationTermClosureScan{closure: c, values: make(map[ValueTerm]uint8), guards: make(map[Guard]uint8), paths: make(map[PathTerm]struct{})}
	for _, term := range input.Values {
		if err := scan.value(term); err != nil {
			return TermRebaseOutput{}, err
		}
	}
	for _, path := range input.Paths {
		if err := scan.path(path); err != nil {
			return TermRebaseOutput{}, err
		}
	}
	for _, guard := range input.Guards {
		if err := scan.guard(guard); err != nil {
			return TermRebaseOutput{}, err
		}
	}
	if err := scan.validateCells(); err != nil {
		return TermRebaseOutput{}, err
	}

	values, paths := c.identityBindings()
	bindings, err := NewTermRootBindings(c.shape, c.shape, values, paths)
	if err != nil {
		return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: err.Error()}
	}
	state := rebaseState{
		caller: c.arena, callee: c.arena, bindings: bindings,
		values: make(map[ValueTerm]ValueTerm), paths: make(map[PathTerm]PathTerm), guards: make(map[Guard]Guard),
	}
	for term, slot := range scan.environmentValues {
		state.values[term] = c.environment[slot]
	}
	for term := range scan.retainedValues {
		// FrameResult is existing sealed call syntax, not lexical storage.
		// Apply supplies its correlated target outcome before continuation use.
		state.values[term] = term
	}
	for term, id := range scan.environmentPaths {
		node := c.arena.paths[term]
		base := c.arena.paths[c.paths[id]]
		state.paths[term] = c.arena.Path(base.root, append(append([]segment.Segment(nil), base.segments...), node.segments...)...)
	}
	for term := range scan.retainedPaths {
		state.paths[term] = term
	}
	// scan.cellOrder is postorder, so a cell argument may use an earlier closed
	// cell but can never observe a partially composed later one.
	for _, term := range scan.cellOrder {
		node := c.arena.values[term]
		closure := c.cells[node.cell]
		args := make([]ValueTerm, len(node.args))
		for index, arg := range node.args {
			args[index] = state.value(arg)
			if args[index] == 0 || state.err != nil {
				return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, term: term, cell: node.cell, why: "cell argument did not close"}
			}
		}
		cellBindings, bindErr := NewTermRootBindings(closure.shape, c.shape, args, nil)
		if bindErr != nil {
			return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, term: term, cell: node.cell, why: bindErr.Error()}
		}
		rebased, rebaseErr := RebaseTermDAGs(c.arena, closure.arena, cellBindings, TermRebaseInput{Values: []ValueTerm{closure.result}})
		if rebaseErr != nil || len(rebased.Values) != 1 || rebased.Values[0] == 0 {
			why := "cell result did not rebase"
			if rebaseErr != nil {
				why = rebaseErr.Error()
			}
			return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, term: term, cell: node.cell, why: why}
		}
		state.values[term] = rebased.Values[0]
	}

	out := TermRebaseOutput{Values: make([]ValueTerm, len(input.Values)), Paths: make([]PathTerm, len(input.Paths)), Guards: make([]Guard, len(input.Guards))}
	for index, term := range input.Values {
		out.Values[index] = state.value(term)
	}
	for index, path := range input.Paths {
		out.Paths[index] = state.path(path)
	}
	for index, guard := range input.Guards {
		out.Guards[index] = state.guard(guard)
	}
	if state.err != nil {
		return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: state.err.Error()}
	}
	for _, term := range out.Values {
		if term == 0 {
			return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "value construction returned zero"}
		}
	}
	for _, path := range out.Paths {
		if path == 0 {
			return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "path construction returned zero"}
		}
	}
	for _, guard := range out.Guards {
		if guard == 0 {
			return TermRebaseOutput{}, &relationTermClosureError{kind: relationTermClosureMalformed, why: "guard construction returned zero"}
		}
	}
	return out, nil
}

func (c relationTermClosure) identityBindings() ([]ValueTerm, []PathTerm) {
	values := make([]ValueTerm, 0, c.shape.InputCount())
	paths := make([]PathTerm, 0, c.shape.InputCount())
	for _, namespace := range []struct {
		kind  RootKind
		count uint32
	}{{RootParam, c.shape.Params}, {RootCapture, c.shape.Captures}, {RootGlobal, c.shape.Globals}, {RootAmbient, c.shape.Ambients}} {
		for index := uint32(0); index < namespace.count; index++ {
			root := Root{Kind: namespace.kind, Index: index}
			values, paths = append(values, c.arena.Root(root)), append(paths, c.arena.Path(root))
		}
	}
	return values, paths
}

type relationTermClosureScan struct {
	closure           relationTermClosure
	values            map[ValueTerm]uint8
	guards            map[Guard]uint8
	paths             map[PathTerm]struct{}
	environmentValues map[ValueTerm]statekey.Value
	environmentPaths  map[PathTerm]symbol.ID
	retainedPaths     map[PathTerm]struct{}
	retainedValues    map[ValueTerm]struct{}
	cellOrder         []ValueTerm
}

func (s *relationTermClosureScan) value(term ValueTerm) error {
	if term == 0 || int(term) >= len(s.closure.arena.values) {
		return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, why: "value term is outside owner arena"}
	}
	if s.values[term] == 1 {
		return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, why: "cyclic value DAG"}
	}
	if s.values[term] == 2 {
		return nil
	}
	s.values[term] = 1
	node := s.closure.arena.values[term]
	switch node.op {
	case valueRoot:
		if !s.closure.arena.validRoot(s.closure.shape, node.root) {
			return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, why: "value has invalid formal root"}
		}
		if node.root.Kind == RootMiddle {
			if s.retainedValues == nil {
				s.retainedValues = make(map[ValueTerm]struct{})
			}
			s.retainedValues[term] = struct{}{}
		}
	case valueEnvironment:
		if len(node.args) != 0 {
			return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, slot: node.slot, why: "environment selector has arguments"}
		}
		if s.closure.environment[node.slot] == 0 {
			return &relationTermClosureError{kind: relationTermClosureUnresolvedEnvironment, term: term, slot: node.slot}
		}
		if s.environmentValues == nil {
			s.environmentValues = make(map[ValueTerm]statekey.Value)
		}
		s.environmentValues[term] = node.slot
	case valueCellResult:
		closure, exact := s.closure.cells[node.cell]
		if !exact {
			return &relationTermClosureError{kind: relationTermClosureUnresolvedCell, term: term, cell: node.cell}
		}
		if len(node.args) != closure.shape.InputCount() {
			return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, cell: node.cell, why: "cell argument width differs from closure shape"}
		}
		for _, arg := range node.args {
			if err := s.value(arg); err != nil {
				return err
			}
		}
		s.cellOrder = append(s.cellOrder, term)
	case valueFrameResult:
		if len(node.args) != 0 || node.frame == 0 || int(node.frame) >= len(s.closure.arena.callFrames) ||
			node.resultIndex < 0 || uint32(node.resultIndex) >= s.closure.arena.callFrames[node.frame].resultCount {
			return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, why: "frame result is outside its sealed frame schema"}
		}
		if s.retainedValues == nil {
			s.retainedValues = make(map[ValueTerm]struct{})
		}
		s.retainedValues[term] = struct{}{}
	default:
		if node.guard != 0 {
			if err := s.guard(node.guard); err != nil {
				return err
			}
		}
		for _, arg := range node.args {
			if err := s.value(arg); err != nil {
				return err
			}
		}
		if node.path != 0 {
			if err := s.path(node.path); err != nil {
				return err
			}
		}
		if node.keyPath != 0 {
			if err := s.path(node.keyPath); err != nil {
				return err
			}
		}
	}
	s.values[term] = 2
	return nil
}

func (s *relationTermClosureScan) path(term PathTerm) error {
	if term == 0 || int(term) >= len(s.closure.arena.paths) {
		return &relationTermClosureError{kind: relationTermClosureMalformed, path: term, why: "path term is outside owner arena"}
	}
	if _, seen := s.paths[term]; seen {
		return nil
	}
	s.paths[term] = struct{}{}
	node := s.closure.arena.paths[term]
	if node.environment == 0 {
		if !s.closure.arena.validRoot(s.closure.shape, node.root) {
			return &relationTermClosureError{kind: relationTermClosureMalformed, path: term, why: "path has invalid formal root"}
		}
		if node.root.Kind == RootMiddle {
			if s.retainedPaths == nil {
				s.retainedPaths = make(map[PathTerm]struct{})
			}
			s.retainedPaths[term] = struct{}{}
		}
		return nil
	}
	if s.closure.paths[node.environment] == 0 {
		return &relationTermClosureError{kind: relationTermClosureUnsupportedPath, path: term, slot: statekey.SymbolValue(node.environment)}
	}
	if s.environmentPaths == nil {
		s.environmentPaths = make(map[PathTerm]symbol.ID)
	}
	s.environmentPaths[term] = node.environment
	return nil
}

func (s *relationTermClosureScan) guard(guard Guard) error {
	if guard == 0 || int(guard) >= len(s.closure.arena.guards) {
		return &relationTermClosureError{kind: relationTermClosureMalformed, why: fmt.Sprintf("guard %d is outside owner arena", guard)}
	}
	if s.guards[guard] == 1 {
		return &relationTermClosureError{kind: relationTermClosureMalformed, why: fmt.Sprintf("cyclic guard DAG at %d", guard)}
	}
	if s.guards[guard] == 2 {
		return nil
	}
	s.guards[guard] = 1
	node := s.closure.arena.guards[guard]
	if node.value != 0 {
		if err := s.value(node.value); err != nil {
			return err
		}
	}
	for _, child := range node.args {
		if err := s.guard(child); err != nil {
			return err
		}
	}
	s.guards[guard] = 2
	return nil
}

func (s *relationTermClosureScan) validateCells() error {
	validated := make(map[CellRef]struct{})
	for _, term := range s.cellOrder {
		cell := s.closure.arena.values[term].cell
		if _, done := validated[cell]; done {
			continue
		}
		validated[cell] = struct{}{}
		closure := s.closure.cells[cell]
		scratch := NewArena(s.closure.arena.reg)
		values := make([]ValueTerm, 0, closure.shape.InputCount())
		for _, namespace := range []struct {
			kind  RootKind
			count uint32
		}{{RootParam, closure.shape.Params}, {RootCapture, closure.shape.Captures}, {RootGlobal, closure.shape.Globals}, {RootAmbient, closure.shape.Ambients}} {
			for index := uint32(0); index < namespace.count; index++ {
				values = append(values, scratch.Root(Root{Kind: namespace.kind, Index: index}))
			}
		}
		bindings, err := NewTermRootBindings(closure.shape, closure.shape, values, nil)
		if err == nil {
			_, err = RebaseTermDAGs(scratch, closure.arena, bindings, TermRebaseInput{Values: []ValueTerm{closure.result}})
		}
		if err != nil {
			return &relationTermClosureError{kind: relationTermClosureMalformed, term: term, cell: cell, why: "cell closure is not a closed rebaseable expression: " + err.Error()}
		}
	}
	return nil
}

// relationEnvironmentClosure is the least finite set of non-formal lexical
// roots that must cross each function boundary because a descendant call or
// closure definition reads or mutates them. Formal parameters/captures/globals
// remain owned by Shape; this carrier only closes indirect environment effects.
type relationEnvironmentClosure struct {
	ambient [][]symbol.ID
	mutable []map[symbol.ID]struct{}
}

type relationEnvironmentDependency struct{ owner, target int }

func closeRelationEnvironments(ordered []RelationProgramUnit, surfaces []programCallSurface, byBody map[lexicalidentity.StableLexicalBodyID]relationVar) (relationEnvironmentClosure, error) {
	count := len(ordered)
	out := relationEnvironmentClosure{ambient: make([][]symbol.ID, count), mutable: make([]map[symbol.ID]struct{}, count)}
	owned := make([]map[symbol.ID]struct{}, count)
	ambient := make([]map[symbol.ID]struct{}, count)
	dependencies := make([]relationEnvironmentDependency, 0)
	for index, unit := range ordered {
		owned[index] = make(map[symbol.ID]struct{})
		ambient[index] = make(map[symbol.ID]struct{})
		out.mutable[index] = directMutableEnvironmentSymbols(unit)
		for _, id := range sealedEnvironmentSymbols(unit.Plan) {
			owned[index][id] = struct{}{}
		}
		for _, site := range surfaces[index].targets {
			for _, candidate := range site.candidates {
				if candidate.target.variable == 0 || int(candidate.target.variable) > count {
					return relationEnvironmentClosure{}, fmt.Errorf("transformer: environment closure has foreign call target")
				}
				dependencies = append(dependencies, relationEnvironmentDependency{owner: index, target: int(candidate.target.variable - 1)})
			}
		}
		for _, definition := range unit.Definitions {
			target, ok := byBody[definition.Target]
			if !ok || target == 0 || int(target) > count {
				return relationEnvironmentClosure{}, fmt.Errorf("transformer: environment closure has foreign definition target %s", definition.Target)
			}
			dependencies = append(dependencies, relationEnvironmentDependency{owner: index, target: int(target - 1)})
		}
	}
	changed := true
	for changed {
		changed = false
		for _, dependency := range dependencies {
			owner, target := dependency.owner, dependency.target
			required := append(ordered[target].Plan.BoundaryCaptures(), ordered[target].Plan.BoundaryGlobals()...)
			for id := range ambient[target] {
				required = append(required, id)
			}
			// A descendant mutation is also an environment dependency even when
			// no descendant expression reads the symbol. The owner must carry the
			// root into and back out of the linked frame; recording mutability
			// without adding this wire creates an effect with no address.
			for id := range out.mutable[target] {
				required = append(required, id)
			}
			for _, id := range required {
				if id == 0 {
					return relationEnvironmentClosure{}, fmt.Errorf("transformer: environment closure contains zero symbol")
				}
				if _, local := owned[owner][id]; local {
					continue
				}
				if _, exists := ambient[owner][id]; !exists {
					ambient[owner][id] = struct{}{}
					changed = true
				}
			}
			for id := range out.mutable[target] {
				// The declaring body's ordinary root carrier owns the mutation
				// from this boundary inward.  Its caller must not acquire a second
				// ambient spelling for that owner-local root.  Formal capture/global
				// roots are included in owned as well: their existing Shape circuit
				// already transports the updated value back to the caller.
				if _, local := owned[owner][id]; local {
					continue
				}
				if _, exists := out.mutable[owner][id]; !exists {
					out.mutable[owner][id] = struct{}{}
					changed = true
				}
			}
		}
	}
	for index := range ambient {
		out.ambient[index] = make([]symbol.ID, 0, len(ambient[index]))
		for id := range ambient[index] {
			out.ambient[index] = append(out.ambient[index], id)
		}
		sort.Slice(out.ambient[index], func(i, j int) bool { return out.ambient[index][i] < out.ambient[index][j] })
	}
	return out, nil
}

func directMutableEnvironmentSymbols(unit RelationProgramUnit) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	if unit.Plan == nil {
		return out
	}
	boundary := make(map[symbol.ID]struct{}, len(unit.Plan.BoundaryCaptures())+len(unit.Plan.BoundaryGlobals()))
	for _, symbols := range [][]symbol.ID{unit.Plan.BoundaryCaptures(), unit.Plan.BoundaryGlobals()} {
		for _, id := range symbols {
			if id != 0 {
				boundary[id] = struct{}{}
			}
		}
	}
	add := func(id symbol.ID) {
		if _, external := boundary[id]; external {
			out[id] = struct{}{}
		}
	}
	facts := unit.Plan.Facts()
	for raw := 0; raw < unit.Plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		if assignment, ok := facts.RootAssignment(point); ok && assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite {
			add(assignment.TargetSymbol())
		}
		if assignment, ok := facts.PathAssignment(point); ok {
			add(assignment.TargetPathRef().Symbol)
		}
		if assignment, ok := facts.PathStaticMemberWrite(point); ok {
			add(assignment.TargetPathRef().Symbol)
		}
		if assignment, ok := facts.DynamicIndexWrite(point); ok {
			add(assignment.TablePathRef().Symbol)
		}
	}
	return out
}
