package callsite

// This file contains the interprocedural Call-body Effect transfer.  It is
// intentionally a sibling of the existing selected/opaque external-call
// rules: those rules continue to own Target operation rows, while this rule
// only transports an already-computed Effect summary for a callable Program
// body.  The engine's ordinary WTO/SCC solver supplies recursion; this rule
// does not recurse in Go or maintain a second summary table.

import (
	"crypto/sha256"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// bodyCallOperand is one exact caller Application/body-root witness.  The
// body targets selected by the input Call value are deliberately not copied
// into this operand; they are the runtime Call fact and are projected through
// the Call-owned Body capability during Transfer.  Consequently there is no
// caller×callee root product in the operand or in cold Rule identity.
type bodyCallOperand struct {
	root effectfactor.Root
	key  calldomain.Key
	app  linkproject.Application
	id   keyspace.ContentID
}

func newBodyCallOperand(effects *effectfactor.Algebra, calls *calldomain.Algebra, root effectfactor.Root, app linkproject.Application) (bodyCallOperand, bool) {
	if effects == nil || !effects.Valid() || calls == nil || !calls.Valid() || effects.Link() != calls.Link() || !effects.ContainsCall(root, app) {
		return bodyCallOperand{}, false
	}
	key, ok := calls.KeyForApplication(app)
	if !ok {
		return bodyCallOperand{}, false
	}
	callID, ok := key.ContentID()
	if !ok || !callID.Available() {
		return bodyCallOperand{}, false
	}
	rootID, ok := effects.RootID(root)
	if !ok || !rootID.Available() {
		return bodyCallOperand{}, false
	}
	const prefix = "wippy.analysis.effect.body-call.v1\x00"
	var payload [len(prefix) + 2*sha256.Size]byte
	copy(payload[:], prefix)
	copy(payload[len(prefix):], callID[:])
	copy(payload[len(prefix)+sha256.Size:], rootID[:])
	id := keyspace.ContentID(sha256.Sum256(payload[:]))
	return bodyCallOperand{root: root, key: key, app: app, id: id}, id.Available()
}

// BodyCallRule transfers the summaries of exactly those known Call targets
// that are Program bodies to the caller's Effect root.  External Target
// operation/seed alternatives are intentionally ignored here and remain the
// input to the existing selected/opaque rules.
type BodyCallRule struct {
	semantic    engine.SemanticKey
	composition *engine.Composition
	rule        *engine.Rule[effectfactor.Value, bodyCallOperand]
	calls       *callowner.Owner
	effects     *effectowner.Owner
	bodies      calldomain.Bodies
	callRead    engine.Read[engine.OrderedCells[calldomain.Value]]
	summary     engine.Read[engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]]]
	write       engine.Write[effectfactor.Value]
}

// DeclareBody declares the one typed body-summary transfer. The exact Call
// read is the predecessor authority. The staged Effect read projects only the
// body roots named by that Call value; it never retains a global summary
// vector or caller-by-body relation.
func DeclareBody(c *engine.Composition, semantic, family, evidence engine.SemanticKey, calls *callowner.Owner, effects *effectowner.Owner) (*BodyCallRule, bool) {
	if c == nil || calls == nil || effects == nil || calls.Algebra() == nil || effects.Algebra() == nil ||
		calls.Link() == nil || calls.Link() != effects.Link() || !semantic.Available() || !family.Available() || !evidence.Available() ||
		semantic == family || semantic == evidence || family == evidence {
		return nil, false
	}
	declaration := &BodyCallRule{semantic: semantic, composition: c, calls: calls, effects: effects, bodies: calls.Algebra().Bodies()}
	declared, ok := engine.DeclareRule(c, engine.RuleSpec[effectfactor.Value, bodyCallOperand]{
		Semantic:       semantic,
		OperandFamily:  family,
		OperandContent: bodyCallOperandContent,
		Output:         effects.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[effectfactor.Value, bodyCallOperand]) bool {
		input, inputOK := rule.InputAt(0)
		if !inputOK {
			return false
		}
		callRead, callReadOK := engine.ReadFrom(rule, input, calls.ExactRead())
		summary, summaryOK := engine.SelectRead[effectfactor.Value, bodyCallOperand, effectfactor.Value, engine.OrderedCells[effectfactor.Value], uint64](
			rule, input, effects.ExactRead(), []engine.Dependency{engine.ReadDependency(callRead)}, declaration.locateBodies,
		)
		write, writeOK := engine.WriteTo(rule, effects.ExactWrite())
		if !callReadOK || !summaryOK || !writeOK {
			return false
		}
		declaration.rule, declaration.callRead, declaration.summary, declaration.write = rule, callRead, summary, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

// Instance binds one existing Project base Application. The caller instance
// owns only its exact Call key and output root; the staged selector chooses
// the exact callee Effect roots at runtime.
func (rule *BodyCallRule) Instance(app linkproject.Application) (*engine.RuleInstance[effectfactor.Value, bodyCallOperand], bool) {
	if rule == nil || rule.rule == nil || rule.calls == nil || rule.effects == nil || rule.calls.Algebra() == nil || rule.effects.Algebra() == nil || rule.bodies.Count() == 0 {
		return nil, false
	}
	root, ok := rule.effects.Algebra().RootForCall(app)
	if !ok {
		return nil, false
	}
	operand, ok := newBodyCallOperand(rule.effects.Algebra(), rule.calls.Algebra(), root, app)
	if !ok {
		return nil, false
	}
	callRef, callOK := rule.calls.Locate(operand.key)
	outputRef, outputOK := rule.effects.Locate(operand.root)
	if !callOK || !outputOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[effectfactor.Value, bodyCallOperand]) bool {
		return engine.InstanceRead(binding, rule.callRead, callRef) &&
			engine.InstanceSelectorRead(binding, rule.summary, rule.effects.ExactRead()) &&
			engine.InstanceWrite(binding, rule.write, outputRef)
	})
}

func bodyCallOperandContent(value bodyCallOperand) (bodyCallOperand, [32]byte, bool) {
	if !value.id.Available() {
		return bodyCallOperand{}, [32]byte{}, false
	}
	return value, [32]byte(value.id), true
}

func (rule *BodyCallRule) validOperand(value bodyCallOperand) bool {
	if rule == nil || rule.calls == nil || rule.effects == nil || rule.calls.Algebra() == nil || rule.effects.Algebra() == nil ||
		rule.calls.Link() == nil || rule.calls.Link() != rule.effects.Link() || !value.id.Available() || !value.key.Valid() {
		return false
	}
	return rule.effects.Algebra().Admit(value.root, rule.effects.Algebra().Bottom())
}

// bodyRoute is a transient selector witness. The tag is the Effect owner's
// root coordinate, while root remains the owner-issued semantic capability.
// It is never retained on BodyCallRule or in an operand.
type bodyRoute struct {
	tag  uint64
	root effectfactor.Root
}

func (rule *BodyCallRule) routeForBody(body calldomain.Body) (bodyRoute, bool) {
	if rule == nil || rule.calls == nil || rule.effects == nil || !body.Valid() {
		return bodyRoute{}, false
	}
	if _, indexed := rule.bodies.Index(body); !indexed {
		return bodyRoute{}, false
	}
	shard, term, resolved := rule.calls.Algebra().ResolveBody(body)
	root, rooted := rule.effects.Algebra().RootForBody(shard, term)
	index, indexed := rule.effects.Algebra().RootIndex(root)
	if !resolved || !rooted || !indexed || index < 0 {
		return bodyRoute{}, false
	}
	return bodyRoute{tag: uint64(index), root: root}, true
}

func (rule *BodyCallRule) routeAt(index int) (bodyRoute, bool) {
	if rule == nil || index < 0 || index >= rule.bodies.Count() {
		return bodyRoute{}, false
	}
	body, ok := rule.bodies.At(index)
	if !ok {
		return bodyRoute{}, false
	}
	return rule.routeForBody(body)
}

// locateBodies is the only Call-to-body projection. It runs after the exact
// Call predecessor has completed and emits one exact Effect Ref per selected
// body. External operation/seed targets are intentionally ignored here;
// selected/opaque owns those alternatives.
func (rule *BodyCallRule) locateBodies(context engine.SelectorContext, operand bodyCallOperand) bool {
	if rule == nil || !rule.validOperand(operand) {
		return false
	}
	cells, readable := engine.SelectorRead(context, rule.callRead)
	if !readable || cells.Count() != 1 {
		return false
	}
	callValue, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	emit := func(route bodyRoute) bool {
		ref, ok := rule.effects.Locate(route.root)
		return ok && engine.SelectRoute(context, ref, route.tag)
	}
	if callValue.IsTop() {
		for index := 0; index < rule.bodies.Count(); index++ {
			route, ok := rule.routeAt(index)
			if !ok || !emit(route) {
				return false
			}
		}
		return true
	}
	for index := 0; index < callValue.KnownTargetCount(); index++ {
		target, ok := callValue.KnownTargetAt(index)
		if !ok {
			return false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		route, routed := rule.routeForBody(body)
		if !routed || !emit(route) {
			return false
		}
	}
	return true
}

// transfer is intentionally non-recursive.  A recursive body call simply
// reads the current WTO iteration's callee summary and transports its atoms
// into the caller root; the engine schedules the strongly connected component.
func (rule *BodyCallRule) transfer(access engine.Access[effectfactor.Value, bodyCallOperand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !rule.validOperand(operand) {
		return false
	}
	effects := rule.effects.Algebra()
	return engine.Product(access, func(row engine.Row) bool {
		callCells, callReadable := engine.ReadValue(access, row, rule.callRead)
		selection, summaryReadable := engine.ReadValue(access, row, rule.summary)
		if !callReadable || !summaryReadable || callCells.Count() != 1 {
			return false
		}
		callValue, present, available := callCells.At(0)
		if !available {
			return false
		}
		if !present {
			count, counted := engine.SelectionCount(access, row, selection)
			if !counted || count != 0 {
				return false
			}
			return engine.NoCandidate(access, row)
		}
		next, reduced := rule.reduceSelection(access, row, operand, callValue, selection)
		if !reduced {
			return false
		}
		if effects.Equal(next, effects.Bottom()) {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

func (rule *BodyCallRule) reduceSelection(
	access engine.Access[effectfactor.Value, bodyCallOperand],
	row engine.Row,
	operand bodyCallOperand,
	callValue calldomain.Value,
	selection engine.Selection[uint64, engine.OrderedCells[effectfactor.Value]],
) (effectfactor.Value, bool) {
	if rule == nil || !rule.validOperand(operand) || !rule.calls.Algebra().Admits(operand.key, callValue) {
		return effectfactor.Value{}, false
	}
	count, counted := engine.SelectionCount(access, row, selection)
	if !counted {
		return effectfactor.Value{}, false
	}
	routes, routesOK := rule.expectedRoutes(callValue)
	if !routesOK || count != len(routes) {
		return effectfactor.Value{}, false
	}
	effects := rule.effects.Algebra()
	atoms := make([]effectfactor.Atom, 0)
	seen := make(map[uint64]struct{}, count)
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.SelectionAt(access, row, selection, index)
		if !selected || cells.Count() != 1 {
			return effectfactor.Value{}, false
		}
		if _, duplicate := seen[tag]; duplicate {
			return effectfactor.Value{}, false
		}
		seen[tag] = struct{}{}
		if _, routed := routeByTag(routes, tag); !routed {
			return effectfactor.Value{}, false
		}
		part, present, available := cells.At(0)
		if !available {
			return effectfactor.Value{}, false
		}
		if !present {
			continue
		}
		if effects.Equal(part, effects.Top()) {
			return effects.Top(), true
		}
		transported, ok := effects.Transport(part, operand.root)
		if !ok {
			return effectfactor.Value{}, false
		}
		for atomIndex := 0; ; atomIndex++ {
			atom, exists := effects.AtomAt(transported, atomIndex)
			if !exists {
				break
			}
			atoms = append(atoms, atom)
		}
	}
	return effects.FromAtoms(atoms)
}

func (rule *BodyCallRule) expectedRoutes(callValue calldomain.Value) ([]bodyRoute, bool) {
	if rule == nil {
		return nil, false
	}
	routes := make([]bodyRoute, 0)
	seen := make(map[uint64]struct{})
	appendRoute := func(route bodyRoute) bool {
		if _, duplicate := seen[route.tag]; duplicate {
			return false
		}
		seen[route.tag] = struct{}{}
		routes = append(routes, route)
		return true
	}
	if callValue.IsTop() {
		for index := 0; index < rule.bodies.Count(); index++ {
			route, ok := rule.routeAt(index)
			if !ok || !appendRoute(route) {
				return nil, false
			}
		}
		return routes, true
	}
	for index := 0; index < callValue.KnownTargetCount(); index++ {
		target, ok := callValue.KnownTargetAt(index)
		if !ok {
			return nil, false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		route, routed := rule.routeForBody(body)
		if !routed || !appendRoute(route) {
			return nil, false
		}
	}
	return routes, true
}

func routeByTag(routes []bodyRoute, tag uint64) (bodyRoute, bool) {
	for _, route := range routes {
		if route.tag == tag {
			return route, true
		}
	}
	return bodyRoute{}, false
}

func (rule *BodyCallRule) reduceDispositionSelection(
	derivation engine.RuleDerivation[effectfactor.Value, bodyCallOperand],
	disposition engine.RuleDisposition[effectfactor.Value],
	operand bodyCallOperand,
	callValue calldomain.Value,
	routes []bodyRoute,
) (effectfactor.Value, bool) {
	if rule == nil || !rule.validOperand(operand) || !rule.calls.Algebra().Admits(operand.key, callValue) {
		return effectfactor.Value{}, false
	}
	count, counted := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.summary)
	if !counted || count != len(routes) {
		return effectfactor.Value{}, false
	}
	effects := rule.effects.Algebra()
	atoms := make([]effectfactor.Atom, 0)
	seen := make(map[uint64]struct{}, count)
	for index := 0; index < count; index++ {
		tag, cells, selected := engine.DerivationDispositionSelectionAt(derivation, disposition, rule.summary, index)
		if !selected || cells.Count() != 1 {
			return effectfactor.Value{}, false
		}
		route, routed := routeByTag(routes, tag)
		if !routed {
			return effectfactor.Value{}, false
		}
		if _, duplicate := seen[tag]; duplicate {
			return effectfactor.Value{}, false
		}
		seen[tag] = struct{}{}
		ref, refOK := rule.effects.Locate(route.root)
		if !refOK || !engine.DerivationDispositionSelectionMatchesRef(derivation, disposition, rule.summary, index, ref) {
			return effectfactor.Value{}, false
		}
		part, present, available := cells.At(0)
		if !available {
			return effectfactor.Value{}, false
		}
		if !present {
			continue
		}
		if effects.Equal(part, effects.Top()) {
			return effects.Top(), true
		}
		transported, transportOK := effects.Transport(part, operand.root)
		if !transportOK {
			return effectfactor.Value{}, false
		}
		for atomIndex := 0; ; atomIndex++ {
			atom, exists := effects.AtomAt(transported, atomIndex)
			if !exists {
				break
			}
			atoms = append(atoms, atom)
		}
	}
	return effects.FromAtoms(atoms)
}

func (rule *BodyCallRule) check(derivation engine.RuleDerivation[effectfactor.Value, bodyCallOperand]) (engine.RuleEvidence, bool) {
	// A selector with no emitted route has no second checker-visible
	// observation; once a body Ref is selected, the staged read contributes the
	// second read alongside the exact Call predecessor.
	readCount := derivation.ReadCount()
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || (readCount != 1 && readCount != 2) || derivation.DispositionCount() == 0 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !rule.validOperand(operand) || !derivation.OperandContentMatches([32]byte(operand.id)) {
		return engine.RuleEvidence{}, false
	}
	rebuilt, rebuiltOK := newBodyCallOperand(rule.effects.Algebra(), rule.calls.Algebra(), operand.root, operand.app)
	if !rebuiltOK || rebuilt.id != operand.id || rebuilt.key != operand.key || rebuilt.root != operand.root {
		return engine.RuleEvidence{}, false
	}
	callRef, callOK := rule.calls.Locate(operand.key)
	outputRef, outputOK := rule.effects.Locate(operand.root)
	if !callOK || !outputOK || !engine.DerivationReadMatchesRef(derivation, rule.callRead, callRef) {
		return engine.RuleEvidence{}, false
	}
	if input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < derivation.DispositionCount(); index++ {
		disposition, dispositionOK := derivation.DispositionAt(index)
		if !dispositionOK || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
			return engine.RuleEvidence{}, false
		}
		callCells, callReadOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.callRead)
		if !callReadOK || callCells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		callValue, present, available := callCells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			selectionCount, selectionOK := engine.DerivationDispositionSelectionCount(derivation, disposition, rule.summary)
			if !selectionOK || selectionCount != 0 {
				return engine.RuleEvidence{}, false
			}
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		if !rule.calls.Algebra().Admits(operand.key, callValue) {
			return engine.RuleEvidence{}, false
		}
		routes, routesOK := rule.expectedRoutes(callValue)
		if !routesOK {
			return engine.RuleEvidence{}, false
		}
		expected, reduced := rule.reduceDispositionSelection(derivation, disposition, operand, callValue, routes)
		if !reduced {
			return engine.RuleEvidence{}, false
		}
		if rule.effects.Algebra().Equal(expected, rule.effects.Algebra().Bottom()) {
			if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		actual, staged := disposition.Value()
		target, targetOK := disposition.TargetAt(0)
		if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK || !rule.effects.Algebra().Equal(actual, expected) || !engine.TargetMatchesRef(target, outputRef) {
			return engine.RuleEvidence{}, false
		}
	}
	return derivation.Accept()
}
