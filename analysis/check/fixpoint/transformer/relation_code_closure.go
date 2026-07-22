package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// closeRelationCodeTerms is the single freeze-time closure transaction for a
// callable relation. It removes raw environment/cell selectors from every
// reachable scalar, path, guard, frame and effect reference before sealing.
// The result uses only declared IN roots and sealed body-owned MID roots; there
// is no runtime callback or Apply fallback for a term which cannot be closed.
func closeRelationProgramTerms(prepared []*PreparedPlanCompiler, units []RelationProgramUnit, definitions []relationProgramDefinition) ([]Shape, error) {
	if len(prepared) != len(units) {
		return nil, fmt.Errorf("transformer: relation closure has no complete lexical seed inventory")
	}
	codes := make([]*relationCode, len(prepared))
	for index := range prepared {
		if prepared[index] == nil || prepared[index].codeBase == nil {
			return nil, fmt.Errorf("transformer: relation closure forest contains an unprepared body")
		}
		codes[index] = prepared[index].codeBase
	}
	if err := closeRelationEntrySeedMiddleSchemas(prepared, units); err != nil {
		return nil, err
	}
	if err := closeRelationCallResultMiddleSchemas(prepared, codes, definitions); err != nil {
		return nil, err
	}
	for index := range prepared {
		for _, target := range unsealedRelationTargets(codes[index]) {
			if target == 0 || int(target) > len(codes) {
				return nil, fmt.Errorf("transformer: relation closure has foreign target %d", target)
			}
		}
		closure, err := newRelationTermClosure(
			prepared[index].builder.arena,
			prepared[index].shape,
			prepared[index].plan,
			prepared[index].ambientRoots,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("transformer: relation %d term closure: %w", index+1, err)
		}
		if err := closeRelationCodeTerms(codes[index], closure, prepared[index].plan, codes); err != nil {
			return nil, fmt.Errorf("transformer: relation %d term closure: %w", index+1, err)
		}
	}
	// Shape is a link product, not a field written back into a caller-owned
	// unit.  The arena closures above consume the draft while it is open; every
	// later phase consumes this returned sealed inventory by dense body handle.
	shapes := make([]Shape, len(prepared))
	for index := range prepared {
		shapes[index] = prepared[index].shape
	}
	return shapes, nil
}

// closeRelationEntrySeedMiddleSchemas admits the complete lexical seed
// inventory into the same MID register vocabulary used by executable terms.
// A declaration need not occur in relation syntax to own its prepared default;
// publication and later registered factors still address that declaration by
// its stable lexical slot.  middleRegisterForSlot is the single closed decoder,
// so new seed producers acquire this law without a transformer-side kind list.
//
// This does not make a local caller-supplied.  Only relationMiddleEntry binds a
// MID root to an IN root; a seed-only register remains an unbound body-local MID
// coordinate and is initialized solely by EntrySeedFactorPlan.
func closeRelationEntrySeedMiddleSchemas(prepared []*PreparedPlanCompiler, units []RelationProgramUnit) error {
	for index, compiler := range prepared {
		if compiler == nil || compiler.builder == nil || compiler.builder.arena == nil ||
			compiler.builder.arena.Sealed() || compiler.builder.arena.middle.sealed || !units[index].EntrySeedPlan.Valid() {
			return fmt.Errorf("transformer: relation %d EntrySeed Middle closure is not open", index+1)
		}
		arena := compiler.builder.arena
		if err := arena.includeMiddleRegisterInventory(units[index].EntrySeedPlan.Slots()); err != nil {
			return fmt.Errorf("transformer: relation %d EntrySeed Middle closure: %w", index+1, err)
		}
	}
	return nil
}

// closeRelationCallResultMiddleSchemas is the sole output-width authority for
// lexical call frames. It runs while every caller arena and target relationCode
// remain open, before newRelationTermClosure seals the caller's Middle schema.
// The complete point-owned result register inventory is therefore independent
// of which slots happened to mint valueFrameResult syntax during compilation.
func closeRelationCallResultMiddleSchemas(prepared []*PreparedPlanCompiler, codes []*relationCode, definitions []relationProgramDefinition) error {
	if len(prepared) == 0 || len(prepared) != len(codes) {
		return fmt.Errorf("transformer: call-result Middle closure has no complete forest")
	}
	definitionFrames := make([]map[callFrameTerm]relationVar, len(codes))
	for _, definition := range definitions {
		if definition.owner == 0 || int(definition.owner) > len(codes) || definition.target == 0 || int(definition.target) > len(codes) || definition.frame == 0 {
			return fmt.Errorf("transformer: call-result Middle closure has a malformed definition frame")
		}
		owner := definition.owner - 1
		if definitionFrames[owner] == nil {
			definitionFrames[owner] = make(map[callFrameTerm]relationVar)
		}
		if _, duplicate := definitionFrames[owner][definition.frame]; duplicate {
			return fmt.Errorf("transformer: call-result Middle closure repeats definition frame %d", definition.frame)
		}
		definitionFrames[owner][definition.frame] = definition.target
	}

	outputWidths := make([]uint32, len(prepared))
	for index := range prepared {
		if prepared[index] == nil {
			return fmt.Errorf("transformer: relation %d call-result Middle closure is not open", index+1)
		}
		outputWidths[index] = prepared[index].shape.Results
	}
	for callerIndex, caller := range prepared {
		code := codes[callerIndex]
		if caller == nil || caller.plan == nil || caller.builder == nil || caller.builder.arena == nil ||
			code == nil || code.terms != caller.builder.arena || code.sealed || code.terms.Sealed() || code.terms.middle.sealed {
			return fmt.Errorf("transformer: relation %d call-result Middle closure is not open", callerIndex+1)
		}
		arena := code.terms
		receivers := indexRelationCallReceivers(code)
		for frameTerm := callFrameTerm(1); int(frameTerm) < len(arena.callFrames); frameTerm++ {
			frame := &arena.callFrames[frameTerm]
			if frame.variable == 0 || int(frame.variable) > len(codes) || frame.point == 0 {
				return fmt.Errorf("transformer: relation %d call frame %d has no lexical target", callerIndex+1, frameTerm)
			}
			if target, definition := definitionFrames[callerIndex][frameTerm]; definition {
				if target != frame.variable || frame.resultCount != 0 {
					return fmt.Errorf("transformer: definition frame %d owns call results", frameTerm)
				}
				continue
			}

			width := uint64(frame.resultCount)
			include := func(index int, source string) error {
				if index < 0 || uint64(index) >= uint64(^uint32(0)) {
					return fmt.Errorf("transformer: call frame %d has invalid %s result %d", frameTerm, source, index)
				}
				if next := uint64(index) + 1; next > width {
					width = next
				}
				return nil
			}

			target := codes[frame.variable-1]
			if target == nil || target.terms == nil || target.sealed {
				return fmt.Errorf("transformer: call frame %d target relation is not open", frameTerm)
			}
			for outcome := boundaryOutcomeRef(1); int(outcome) < len(target.outcomes); outcome++ {
				transaction := target.outcomes[outcome].returnTransaction.transaction
				for index := 0; index < transaction.ResultTargetCount(); index++ {
					result, exact := transaction.ResultTarget(index)
					if !exact {
						return fmt.Errorf("transformer: call frame %d target outcome %d has a missing result target", frameTerm, outcome)
					}
					if err := include(result, "callee-outcome"); err != nil {
						return err
					}
				}
			}

			site, exact := caller.plan.Facts().CallSiteView(frame.point)
			if !exact {
				return fmt.Errorf("transformer: call frame %d has no frozen call-site result schema", frameTerm)
			}
			for index := 0; index < site.ResultTargetCount(); index++ {
				result, ok := site.ResultTargetAt(index)
				if !ok {
					return fmt.Errorf("transformer: call frame %d has a missing caller result target", frameTerm)
				}
				if err := include(result.ResultIndex(), "call-site"); err != nil {
					return err
				}
			}
			for _, receiver := range receivers[frame.point] {
				source, ok := receiver.transaction.Source(0)
				if !ok || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != frame.point {
					return fmt.Errorf("transformer: call frame %d has a malformed root-assignment receiver", frameTerm)
				}
				if err := include(source.TargetIndex, "root-assignment"); err != nil {
					return err
				}
			}
			for slot := range arena.environment {
				point, result, callResult := statekey.ParseCallResult(slot)
				if callResult && point == uint32(frame.point) {
					if err := include(int(result), "environment"); err != nil {
						return err
					}
				}
			}
			frame.resultCount = uint32(width)
			if frame.resultCount > outputWidths[frame.variable-1] {
				outputWidths[frame.variable-1] = frame.resultCount
			}
			for slot := 0; slot < int(frame.resultCount); slot++ {
				// FrameResult remains the sole target-OUT selector. The point-owned
				// CallResult Middle register below is its caller-local destination,
				// not a replacement output vocabulary.
				if arena.frameResultValue(frameTerm, uint32(slot)) == 0 {
					return fmt.Errorf("transformer: call frame %d result %d has no OUT selector", frameTerm, slot)
				}
				if arena.bindCallResult(frame.point, slot) == 0 {
					return fmt.Errorf("transformer: call frame %d result %d has no Middle register", frameTerm, slot)
				}
			}
		}

	}

	// A target Output root exists when either N5 writes it or some lexical frame
	// demands it. Lua supplies nil for a missing return, so caller demand is part
	// of the callable tuple shape even when the source body returns fewer values.
	// Close that forest-wide law once, while every relation and frame is open;
	// later SlotSpace/coordinate freezing must only consume this inventory.
	for targetIndex, width := range outputWidths {
		if prepared[targetIndex].shape.Results >= width {
			continue
		}
		prepared[targetIndex].shape.Results = width
		prepared[targetIndex].builder.shape.Results = width
		prepared[targetIndex].worldBase.shape.Results = width
		codes[targetIndex].shape.Results = width
	}
	for callerIndex := range prepared {
		arena := codes[callerIndex].terms
		for frameTerm := callFrameTerm(1); int(frameTerm) < len(arena.callFrames); frameTerm++ {
			frame := &arena.callFrames[frameTerm]
			frame.shape = prepared[frame.variable-1].shape
		}

		// resultCount and the now-complete target shape both participate in
		// structural frame identity. Rebuild the open arena's acceleration index
		// after the single closure transaction.
		keys := make(map[uint64][]callFrameTerm, len(arena.callFrameKeys))
		for frameTerm := callFrameTerm(1); int(frameTerm) < len(arena.callFrames); frameTerm++ {
			frame := arena.callFrames[frameTerm]
			fingerprint := arena.maskFingerprint(callFrameFingerprint(frame))
			for _, prior := range keys[fingerprint] {
				if callFrameNodeEqual(arena.callFrames[prior], frame) {
					return fmt.Errorf("transformer: call-result Middle closure collapsed frames %d and %d", prior, frameTerm)
				}
			}
			keys[fingerprint] = append(keys[fingerprint], frameTerm)
		}
		arena.callFrameKeys = keys
	}
	return nil
}

func unsealedRelationTargets(code *relationCode) []relationVar {
	set := make(map[relationVar]struct{})
	if code != nil {
		for index := 1; index < len(code.nodes); index++ {
			for _, step := range code.nodes[index].steps {
				if step.kind == boundaryStepApply && step.apply.variable != 0 {
					set[step.apply.variable] = struct{}{}
				}
			}
		}
	}
	out := make([]relationVar, 0, len(set))
	for target := range set {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func closeRelationCodeTerms(code *relationCode, closure relationTermClosure, plan *operationplan.Plan, codes []*relationCode) error {
	if code == nil || code.terms == nil || code.effects == nil || code.sealed || closure.arena != code.terms {
		return fmt.Errorf("transformer: relation closure requires one open owned code")
	}
	if plan == nil {
		return fmt.Errorf("transformer: relation closure has no semantic plan")
	}
	_, _, _, err := relationTopology(code, code.root)
	if err != nil {
		return err
	}
	topology, successors, err := relationClosureTopology(code)
	if err != nil {
		return err
	}
	incoming := make([]map[statekey.Value]ValueTerm, len(code.nodes))
	incoming[code.root] = cloneRelationEnvironment(closure.environment)
	incomingGuards := make([]Guard, len(code.nodes))
	incomingGuards[code.root] = code.terms.True()
	frames := make([]map[callFrameTerm]struct{}, len(code.nodes))
	frames[code.root] = make(map[callFrameTerm]struct{})
	for _, ref := range topology {
		env := cloneRelationEnvironment(incoming[ref])
		pathGuard := incomingGuards[ref]
		ownedFrames := cloneRelationFrames(frames[ref])
		if env == nil || ownedFrames == nil {
			return fmt.Errorf("transformer: relation node %d has no path-sensitive environment", ref)
		}
		node := &code.nodes[ref]
		local := closure
		local.environment = env
		if err := closeRelationRefs(local, relationCodeTermRefs{guards: nonzeroGuardRef(&node.guard)}); err != nil {
			return fmt.Errorf("transformer: relation node %d guard closure: %w", ref, err)
		}
		if node.kind == relationNodeSequence {
			steps := node.steps[:0]
			for index := range node.steps {
				step := node.steps[index]
				local.environment = env
				refs := relationCodeTermRefs{}
				refs.guard(&step.guard)
				refs.value(&step.value)
				for source := range step.rootAssignment.sources {
					refs.value(&step.rootAssignment.sources[source])
				}
				if step.effect != 0 {
					if int(step.effect) >= len(code.effects.nodes) {
						return fmt.Errorf("transformer: relation closure encountered foreign effect %d", step.effect)
					}
					refs.effect(&code.effects.nodes[step.effect])
				}
				if step.kind == boundaryStepContribution {
					if step.contribution == 0 || int(step.contribution) >= len(code.contributions) {
						return fmt.Errorf("transformer: relation contribution is foreign")
					}
					contribution := code.contributions[step.contribution].clone()
					code.contributions = append(code.contributions, contribution)
					step.contribution = boundaryContributionRef(len(code.contributions) - 1)
					refs.contribution(&code.contributions[step.contribution])
				}
				if step.kind == boundaryStepApply {
					if step.apply.frame == 0 || int(step.apply.frame) >= len(code.terms.callFrames) {
						return fmt.Errorf("transformer: relation Apply has a foreign frame")
					}
					frame := &code.terms.callFrames[step.apply.frame]
					for value := range frame.values {
						refs.value(&frame.values[value])
					}
					for path := range frame.paths {
						refs.path(&frame.paths[path])
					}
				}
				if (step.kind == boundaryStepExternalCall || step.kind == boundaryStepApply) && step.memberCall.site != 0 {
					refs.memberCall(&step.memberCall)
				}
				if err := closeRelationRefs(local, refs); err != nil {
					paths := make([]string, 0, len(refs.paths))
					for _, path := range refs.paths {
						paths = append(paths, code.terms.canonicalPath(*path))
					}
					values := make([]string, 0, len(refs.values))
					for _, value := range refs.values {
						values = append(values, code.terms.canonicalValue(*value))
					}
					environment := make([]string, 0, len(env))
					for slot, value := range env {
						environment = append(environment, fmt.Sprintf("%d=%s", slot, code.terms.canonicalValue(value)))
					}
					sort.Strings(environment)
					syntax := make([]string, len(node.steps))
					for stepIndex, candidate := range node.steps {
						syntax[stepIndex] = fmt.Sprintf("%d(target=%d,sources=%d)", candidate.kind, candidate.rootAssignment.transaction.TargetSymbol(), len(candidate.rootAssignment.sources))
					}
					return fmt.Errorf("transformer: relation node %d step %d kind %d closure values %v paths %v environment %v syntax %v: %w", ref, index, step.kind, values, paths, environment, syntax, err)
				}
				switch step.kind {
				case boundaryStepEnvironmentWrite:
					// The step writes this stable lexical register. The succeeding
					// formal cell is the new version; substituting the RHS here would
					// create a second SSA/phi implementation in closure.
					value, exact := code.terms.middleValue(step.slot)
					if !exact || value == 0 {
						return fmt.Errorf("transformer: relation environment write slot %d has no Middle register", step.slot)
					}
					env[step.slot] = value
				case boundaryStepRootAssignment:
					if target := step.rootAssignment.transaction.TargetSymbol(); target != 0 {
						value, exact := code.terms.middleValue(statekey.SymbolValue(target))
						if !exact || value == 0 {
							return fmt.Errorf("transformer: relation root assignment target %d has no Middle register", target)
						}
						env[statekey.SymbolValue(target)] = value
					}
				case boundaryStepExternalCall:
					if err := bindRelationExternalCallRegisters(code, plan, step.point, env); err != nil {
						return fmt.Errorf("transformer: relation node %d external call registers: %w", ref, err)
					}
				case boundaryStepGenericFor:
					if err := bindRelationGenericForRegister(code, plan, step.point, env); err != nil {
						return fmt.Errorf("transformer: relation node %d generic-for register: %w", ref, err)
					}
				case boundaryStepApply:
					if step.apply.variable == 0 || int(step.apply.variable) > len(codes) {
						return fmt.Errorf("transformer: relation Apply target is foreign")
					}
					if err := bindRelationFrameSelectors(code, plan, step.apply.frame, env); err != nil {
						return fmt.Errorf("transformer: relation node %d Apply selectors: %w", ref, err)
					}
				}
				steps = append(steps, step)
			}
			node.steps = steps
		}
		if node.kind == relationNodeOutcome {
			outcome := cloneBoundaryOutcome(code.outcomes[node.outcome])
			code.outcomes = append(code.outcomes, outcome)
			node.outcome = boundaryOutcomeRef(len(code.outcomes) - 1)
			refs := relationCodeTermRefs{}
			refs.outcome(&code.outcomes[node.outcome])
			local.environment = env
			if err := closeRelationRefs(local, refs); err != nil {
				return fmt.Errorf("transformer: relation outcome %d closure: %w", node.outcome, err)
			}
		}
		for _, successor := range successors[ref] {
			if successor == 0 {
				continue
			}
			edgeGuard := pathGuard
			if node.kind == relationNodeChoice {
				if successor == node.whenTrue {
					edgeGuard = code.terms.And(pathGuard, node.guard)
				} else {
					complement := code.terms.Not(node.guard)
					if complement == 0 {
						return fmt.Errorf("transformer: relation choice %d has no exact complement", ref)
					}
					edgeGuard = code.terms.And(pathGuard, complement)
				}
			}
			incoming[successor] = mergeRelationEnvironment(code.terms, incoming[successor], env, edgeGuard)
			if incomingGuards[successor] == 0 {
				incomingGuards[successor] = edgeGuard
			} else {
				incomingGuards[successor] = code.terms.Or(incomingGuards[successor], edgeGuard)
			}
			frames[successor] = intersectRelationFrames(frames[successor], ownedFrames)
		}
	}
	return nil
}

// relationClosureTopology is the acyclic ownership graph of the executable
// relation. A LoopMu enters only its canonical body. Its exits are a route
// inventory: the executable edges originate at the matching terminal
// LoopExit steps after the body's ordered transactions have run. Treating the
// inventory as direct LoopMu successors loses every body-owned environment
// register at an early-return or ordinary loop exit.
//
// Feedback is deliberately omitted from this freeze-time topology. The first
// entry must close every body term without relying on a value produced only by
// a later iteration; the runtime tuple-mu equation remains the sole owner of
// feedback and widening.
func relationClosureTopology(code *relationCode) ([]relationRootRef, [][]relationRootRef, error) {
	if code == nil || code.root == 0 || int(code.root) >= len(code.nodes) {
		return nil, nil, fmt.Errorf("transformer: relation closure topology is unowned")
	}
	type loopRoute struct {
		body  relationRootRef
		exits []relationRootRef
	}
	loops := make(map[loopMuTerm]loopRoute)
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		node := code.nodes[ref]
		if node.kind != relationNodeLoopMu {
			continue
		}
		if _, duplicate := loops[node.binder]; duplicate {
			return nil, nil, fmt.Errorf("transformer: relation closure loop %d has multiple owners", node.binder)
		}
		loops[node.binder] = loopRoute{body: node.body, exits: append([]relationRootRef(nil), node.exits...)}
	}

	successors := make([][]relationRootRef, len(code.nodes))
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		node := code.nodes[ref]
		switch node.kind {
		case relationNodeSequence:
			if len(node.steps) == 0 {
				successors[ref] = []relationRootRef{node.next}
				continue
			}
			terminal := node.steps[len(node.steps)-1]
			switch terminal.kind {
			case boundaryStepLoopFeedback:
				if _, exact := loops[terminal.binder]; !exact {
					return nil, nil, fmt.Errorf("transformer: relation closure feedback has no loop %d", terminal.binder)
				}
				// The runtime tuple-mu owns the feedback edge.
			case boundaryStepLoopExit:
				loop, exact := loops[terminal.binder]
				if !exact || int(terminal.route) >= len(loop.exits) {
					return nil, nil, fmt.Errorf("transformer: relation closure exit %d is outside loop %d", terminal.route, terminal.binder)
				}
				successors[ref] = []relationRootRef{loop.exits[terminal.route]}
			default:
				successors[ref] = []relationRootRef{node.next}
			}
		case relationNodeChoice:
			successors[ref] = []relationRootRef{node.whenTrue, node.whenFalse}
		case relationNodeLoopMu:
			successors[ref] = []relationRootRef{node.body}
		case relationNodeLoopPortal:
			successors[ref] = []relationRootRef{node.body}
		}
	}

	reachable := make([]bool, len(code.nodes))
	stack := []relationRootRef{code.root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref == 0 || int(ref) >= len(code.nodes) {
			if ref != 0 {
				return nil, nil, fmt.Errorf("transformer: relation closure edge is outside code")
			}
			continue
		}
		if reachable[ref] {
			continue
		}
		reachable[ref] = true
		stack = append(stack, successors[ref]...)
	}
	indegree := make([]int, len(code.nodes))
	reachableCount := 0
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		if !reachable[ref] {
			continue
		}
		reachableCount++
		for _, successor := range successors[ref] {
			if successor != 0 {
				indegree[successor]++
			}
		}
	}
	queue := make([]relationRootRef, 0, reachableCount)
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		if reachable[ref] && indegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}
	topology := make([]relationRootRef, 0, reachableCount)
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		topology = append(topology, ref)
		for _, successor := range successors[ref] {
			if successor == 0 {
				continue
			}
			indegree[successor]--
			if indegree[successor] == 0 {
				queue = append(queue, successor)
			}
		}
	}
	if len(topology) != reachableCount {
		return nil, nil, fmt.Errorf("transformer: relation closure recurrence escaped typed mu")
	}
	return topology, successors, nil
}

// bindRelationGenericForRegister records the exact invocation-local target
// written by the canonical generic-for transaction. The frozen Projection
// term is the sole value producer; closure only establishes that subsequent
// symbolic terms read the transaction-owned register.
func bindRelationGenericForRegister(code *relationCode, plan *operationplan.Plan, point cfg.Point, env map[statekey.Value]ValueTerm) error {
	if code == nil || code.terms == nil || plan == nil || point == 0 {
		return fmt.Errorf("generic-for register binding is unowned")
	}
	op, exact := plan.GenericForOperation(point)
	if !exact || op.Target() == 0 {
		return fmt.Errorf("generic-for point %d has no target operation", point)
	}
	value, present := code.terms.middleValue(statekey.SymbolValue(op.Target()))
	if !present || value == 0 {
		return fmt.Errorf("generic-for target %d has no invocation-local register", op.Target())
	}
	env[statekey.SymbolValue(op.Target())] = value
	return nil
}

// bindRelationExternalCallRegisters records the invocation-local destinations
// written atomically by the canonical external-call transaction. The closure
// does not reconstruct the provider's value from arguments or signatures;
// subsequent equations read the exact register populated by N0/N3.
func bindRelationExternalCallRegisters(code *relationCode, plan *operationplan.Plan, point cfg.Point, env map[statekey.Value]ValueTerm) error {
	if code == nil || code.terms == nil || plan == nil || point == 0 {
		return fmt.Errorf("external call register binding is unowned")
	}
	site, exact := plan.Facts().CallSiteView(point)
	if !exact {
		return fmt.Errorf("call point %d has no result target syntax", point)
	}
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, ok := site.ResultTargetAt(index)
		if !ok || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
			continue
		}
		value, present := code.terms.middleValue(statekey.SymbolValue(target.TargetSymbol()))
		if !present || value == 0 {
			return fmt.Errorf("call result target %d has no invocation-local register", target.TargetSymbol())
		}
		env[statekey.SymbolValue(target.TargetSymbol())] = value
	}
	return nil
}

func closeRelationRefs(closure relationTermClosure, refs relationCodeTermRefs) error {
	input := TermRebaseInput{Values: make([]ValueTerm, len(refs.values)), Paths: make([]PathTerm, len(refs.paths)), Guards: make([]Guard, len(refs.guards))}
	for index, ref := range refs.values {
		input.Values[index] = *ref
	}
	for index, ref := range refs.paths {
		input.Paths[index] = *ref
	}
	for index, ref := range refs.guards {
		input.Guards[index] = *ref
	}
	closed, err := closure.close(input)
	if err != nil {
		return err
	}
	for index, ref := range refs.values {
		*ref = closed.Values[index]
	}
	for index, ref := range refs.paths {
		*ref = closed.Paths[index]
	}
	for index, ref := range refs.guards {
		*ref = closed.Guards[index]
	}
	return nil
}

func nonzeroGuardRef(ref *Guard) []*Guard {
	if ref == nil || *ref == 0 {
		return nil
	}
	return []*Guard{ref}
}

func cloneRelationEnvironment(in map[statekey.Value]ValueTerm) map[statekey.Value]ValueTerm {
	if in == nil {
		return nil
	}
	out := make(map[statekey.Value]ValueTerm, len(in))
	for slot, value := range in {
		out[slot] = value
	}
	return out
}

func mergeRelationEnvironment(arena *Arena, current map[statekey.Value]ValueTerm, next map[statekey.Value]ValueTerm, nextGuard Guard) map[statekey.Value]ValueTerm {
	if current == nil {
		return cloneRelationEnvironment(next)
	}
	out := cloneRelationEnvironment(current)
	for slot, value := range out {
		other := next[slot]
		if other == 0 {
			delete(out, slot)
		} else if other != value {
			out[slot] = arena.SelectValue(nextGuard, other, value)
		}
	}
	return out
}

func cloneRelationFrames(in map[callFrameTerm]struct{}) map[callFrameTerm]struct{} {
	if in == nil {
		return nil
	}
	out := make(map[callFrameTerm]struct{}, len(in))
	for frame := range in {
		out[frame] = struct{}{}
	}
	return out
}

func intersectRelationFrames(current, next map[callFrameTerm]struct{}) map[callFrameTerm]struct{} {
	if current == nil {
		return cloneRelationFrames(next)
	}
	out := cloneRelationFrames(current)
	for frame := range out {
		if _, ok := next[frame]; !ok {
			delete(out, frame)
		}
	}
	return out
}

// bindRelationFrameSelectors records only the caller-owned post-Apply result
// selectors. It does not inspect, copy, or join any callee outcome. The Apply
// transaction writes these exact frame registers before the continuation runs.
func bindRelationFrameSelectors(code *relationCode, plan *operationplan.Plan, frameTerm callFrameTerm, env map[statekey.Value]ValueTerm) error {
	if code == nil || code.terms == nil || plan == nil || frameTerm == 0 || int(frameTerm) >= len(code.terms.callFrames) {
		return fmt.Errorf("call result selector binding is unowned")
	}
	frame := code.terms.callFrames[frameTerm]
	site, exact := plan.Facts().CallSiteView(frame.point)
	if !exact {
		return fmt.Errorf("call point %d has no result target syntax", frame.point)
	}
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, ok := site.ResultTargetAt(index)
		if !ok || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 || target.ResultIndex() < 0 {
			continue
		}
		if uint32(target.ResultIndex()) >= frame.resultCount {
			return fmt.Errorf("call result %d exceeds frame output width %d", target.ResultIndex(), frame.resultCount)
		}
		value, exact := code.terms.middleValue(statekey.SymbolValue(target.TargetSymbol()))
		if !exact || value == 0 {
			return fmt.Errorf("call result target %d has no Middle register", target.TargetSymbol())
		}
		env[statekey.SymbolValue(target.TargetSymbol())] = value
	}
	return nil
}

func closedRelationResultTerms(code *relationCode) ([]ValueTerm, error) {
	if code == nil || code.root == 0 {
		return nil, fmt.Errorf("callee relation is unclosed")
	}
	outcomes, err := closedRelationGuardedOutcomes(code)
	if err != nil {
		return nil, err
	}
	if len(outcomes) == 0 {
		return nil, fmt.Errorf("callee relation has no normal result outcome")
	}
	width := 0
	for _, outcome := range outcomes {
		if n := len(code.outcomes[outcome.outcome].returnTransaction.sources); n > width {
			width = n
		}
	}
	result := make([]ValueTerm, width)
	nilTerm := code.terms.Constant(typevalue.Nil(code.terms.reg))
	for slot := 0; slot < width; slot++ {
		for index, guarded := range outcomes {
			sources := code.outcomes[guarded.outcome].returnTransaction.sources
			value := nilTerm
			if slot < len(sources) {
				value = sources[slot]
			}
			if index == 0 {
				result[slot] = value
			} else {
				result[slot] = code.terms.SelectValue(guarded.guard, value, result[slot])
			}
		}
	}
	for _, term := range result {
		if term != 0 && int(term) < len(code.terms.values) && code.terms.values[term].op == valueEnvironment {
			return nil, fmt.Errorf("callee result retains environment term %d", term)
		}
	}
	return result, nil
}

type relationGuardedOutcome struct {
	guard   Guard
	outcome boundaryOutcomeRef
}

func closedRelationGuardedOutcomes(code *relationCode) ([]relationGuardedOutcome, error) {
	_, _, topology, err := relationTopology(code, code.root)
	if err != nil {
		return nil, err
	}
	guards := make([]Guard, len(code.nodes))
	guards[code.root] = code.terms.True()
	var out []relationGuardedOutcome
	for _, ref := range topology {
		path := guards[ref]
		if path == 0 {
			return nil, fmt.Errorf("callee relation node %d has no guard path", ref)
		}
		node := code.nodes[ref]
		if node.kind == relationNodeOutcome {
			out = append(out, relationGuardedOutcome{guard: path, outcome: node.outcome})
		}
		for _, successor := range relationSuccessors(node) {
			if successor == 0 {
				continue
			}
			edge := path
			if node.kind == relationNodeChoice {
				if successor == node.whenTrue {
					edge = code.terms.And(path, node.guard)
				} else {
					edge = code.terms.And(path, code.terms.Not(node.guard))
				}
			}
			if guards[successor] == 0 {
				guards[successor] = edge
			} else {
				guards[successor] = code.terms.Or(guards[successor], edge)
			}
		}
	}
	return out, nil
}

type relationCodeTermRefs struct {
	values []*ValueTerm
	paths  []*PathTerm
	guards []*Guard
}

func (r *relationCodeTermRefs) value(ref *ValueTerm) {
	if ref != nil && *ref != 0 {
		r.values = append(r.values, ref)
	}
}

func (r *relationCodeTermRefs) path(ref *PathTerm) {
	if ref != nil && *ref != 0 {
		r.paths = append(r.paths, ref)
	}
}

func (r *relationCodeTermRefs) guard(ref *Guard) {
	if ref != nil && *ref != 0 {
		r.guards = append(r.guards, ref)
	}
}

func (r *relationCodeTermRefs) memberCall(term *boundaryMemberCallDiagnosticTerm) {
	if term == nil {
		return
	}
	r.value(&term.receiver)
	r.value(&term.provider)
	for index := range term.pathArguments {
		r.path(&term.pathArguments[index].path)
	}
}

func (r *relationCodeTermRefs) outcome(outcome *boundaryOutcomeTuple) {
	if outcome == nil {
		return
	}
	for index := range outcome.operations {
		r.value(&outcome.operations[index].Value)
	}
	for index := range outcome.proofs {
		r.path(&outcome.proofs[index].Table)
		r.value(&outcome.proofs[index].Key)
	}
	for index := range outcome.pathObligations {
		r.path(&outcome.pathObligations[index].path)
	}
	for index := range outcome.paramExposures {
		r.path(&outcome.paramExposures[index].source)
	}
	for index := range outcome.returnTransaction.sources {
		r.value(&outcome.returnTransaction.sources[index])
	}
}

func (r *relationCodeTermRefs) contribution(contribution *semanticContribution) {
	if contribution == nil {
		return
	}
	tuple := boundaryOutcomeTuple{
		operations: contribution.operations, proofs: contribution.proofs,
		pathObligations: contribution.pathObligations, paramExposures: contribution.paramExposures,
		returnTransaction: contribution.returnTransaction,
	}
	r.outcome(&tuple)
	contribution.operations, contribution.proofs = tuple.operations, tuple.proofs
	contribution.pathObligations, contribution.paramExposures = tuple.pathObligations, tuple.paramExposures
	contribution.returnTransaction = tuple.returnTransaction
}

func (r *relationCodeTermRefs) effect(effect *effectNode) {
	if effect == nil {
		return
	}
	r.effectTarget(&effect.invalidation.Target)
	if effect.invalidation.Precise != nil {
		r.path(&effect.invalidation.Precise.Table)
		r.value(&effect.invalidation.Precise.Key)
	}
	r.effectTarget(&effect.table)
	r.value(&effect.key)
	r.value(&effect.value)
	r.path(&effect.keyPath)
	r.path(&effect.valuePath)
	r.pathStoreWrite(&effect.pathStoreAssignment)
	r.pathStoreWrite(&effect.pathStoreStatic)
	for heap := range effect.pathStoreObject.Heaps {
		object := &effect.pathStoreObject.Heaps[heap]
		r.value(&object.Root)
		for member := range object.Members {
			r.value(&object.Members[member].Value)
			r.value(&object.Members[member].Expected)
		}
	}
	for entry := range effect.pathStoreObject.Entries {
		object := &effect.pathStoreObject.Entries[entry]
		r.path(&object.Target)
		r.value(&object.Value)
		r.path(&object.SourcePath)
		r.value(&object.Expected)
	}
}

func (r *relationCodeTermRefs) effectTarget(target *EffectTargetTerm) {
	if target != nil {
		r.path(&target.path)
	}
}

func (r *relationCodeTermRefs) pathStoreWrite(write *PathStoreWriteConfig) {
	if write == nil {
		return
	}
	r.path(&write.Target)
	r.value(&write.Value)
	r.path(&write.SourcePath)
}
