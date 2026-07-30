package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type relationGuardAtomSubstitution struct {
	source      ValueTerm
	substituted ValueTerm
	targetLocal bool
}

// rebaseRelationActionGuards freezes the exact per-atom boundary mapping used
// by the formal Apply guard vocabulary. Boundary-supported terms resolve to
// syntax completed by the pre-seal closure; target-local observations retain
// their target term and receive a site-local formal rank. No syntax or
// executable guard program is created here.
func rebaseRelationActionGuards(caller, target *Arena, bindings TermRootBindings, guards []Guard) ([]ValueTerm, []relationGuardAtomSubstitution, error) {
	if caller == nil || target == nil || !caller.Sealed() || !target.Sealed() {
		return nil, nil, fmt.Errorf("transformer: relation guard rebasing is unowned")
	}
	byRank, err := orderedRelationGuardAtoms(target, guards)
	if err != nil {
		return nil, nil, err
	}
	bound := make([]ValueTerm, 0)
	boundSet := make(map[ValueTerm]struct{})
	substitutions := make([]relationGuardAtomSubstitution, len(byRank))
	for index, atom := range byRank {
		rebased, err := rebaseDirectCallTermDAGs(caller, target, bindings, TermRebaseInput{Values: []ValueTerm{atom}})
		if err != nil {
			substitutions[index] = relationGuardAtomSubstitution{source: atom, substituted: atom, targetLocal: true}
			continue
		}
		if len(rebased.Values) != 1 || rebased.Values[0] == 0 {
			return nil, nil, fmt.Errorf("transformer: guard atom produced no caller term")
		}
		substituted := rebased.Values[0]
		substitutions[index] = relationGuardAtomSubstitution{source: atom, substituted: substituted}
		local, provenanceErr := relationGuardAtomHasLexicalExistential(target, atom)
		if provenanceErr != nil {
			return nil, nil, provenanceErr
		}
		if local {
			if _, duplicate := boundSet[substituted]; !duplicate {
				boundSet[substituted] = struct{}{}
				bound = append(bound, substituted)
			}
		}
	}
	return bound, substitutions, nil
}

func orderedRelationGuardAtoms(arena *Arena, guards []Guard) ([]ValueTerm, error) {
	atoms := make(map[ValueTerm]struct{})
	for _, guard := range guards {
		if err := collectRelationGuardAtoms(arena, guard, atoms, make(map[Guard]uint8)); err != nil {
			return nil, err
		}
	}
	ordered := make([]ValueTerm, 0, len(atoms))
	for atom := range atoms {
		ordered = append(ordered, atom)
	}
	names := make(map[ValueTerm]string, len(ordered))
	ranks := make(map[ValueTerm]uint32, len(ordered))
	if err := rankStructuralGuardAtoms(arena, ordered, names, ranks); err != nil {
		return nil, err
	}
	byRank := make([]ValueTerm, len(ordered))
	for atom, rank := range ranks {
		byRank[rank] = atom
	}
	return byRank, nil
}

// closeRelationGuardBoundarySyntax completes the caller-owned term closure for
// every declared frame while all arenas remain open. It attaches no plan and
// evaluates no guard: the only retained result is ordinary hash-consed Arena
// syntax. Post-seal plan derivation must find these exact terms or fail closed.
func closeRelationGuardBoundarySyntax(codes []*relationCode) error {
	if len(codes) == 0 {
		return fmt.Errorf("transformer: relation guard boundary closure has no forest")
	}
	targetAtoms := make([][]ValueTerm, len(codes))
	for index, code := range codes {
		if code == nil || code.terms == nil || code.terms.Sealed() || code.root == 0 {
			return fmt.Errorf("transformer: relation guard boundary target %d is not open", index+1)
		}
		scoped, err := reachableScopedRelationGuardsForClosure(code)
		if err != nil {
			return fmt.Errorf("transformer: relation guard boundary target %d inventory: %w", index+1, err)
		}
		guards := make([]Guard, len(scoped))
		for guardIndex := range scoped {
			guards[guardIndex] = scoped[guardIndex].guard
		}
		targetAtoms[index], err = orderedRelationGuardAtoms(code.terms, guards)
		if err != nil {
			return fmt.Errorf("transformer: relation guard boundary target %d atoms: %w", index+1, err)
		}
	}
	for callerIndex, caller := range codes {
		for frameTerm := callFrameTerm(1); int(frameTerm) < len(caller.terms.callFrames); frameTerm++ {
			frame := caller.terms.callFrames[frameTerm]
			if frame.variable == 0 || int(frame.variable) > len(codes) {
				return fmt.Errorf("transformer: relation guard boundary caller %d frame %d has a foreign target", callerIndex+1, frameTerm)
			}
			target := codes[frame.variable-1]
			if frame.shape != target.shape {
				return fmt.Errorf("transformer: relation guard boundary caller %d frame %d changed target shape", callerIndex+1, frameTerm)
			}
			bindings, err := NewTermRootBindings(target.shape, caller.shape, frame.values, frame.paths)
			if err != nil {
				return fmt.Errorf("transformer: relation guard boundary caller %d frame %d bindings: %w", callerIndex+1, frameTerm, err)
			}
			for _, atom := range targetAtoms[frame.variable-1] {
				rebased, rebaseErr := rebaseDirectCallTermDAGs(caller.terms, target.terms, bindings, TermRebaseInput{Values: []ValueTerm{atom}})
				if rebaseErr != nil {
					// Non-exportable target-local atoms remain target-owned in the
					// post-seal plan and require no caller syntax.
					continue
				}
				if len(rebased.Values) != 1 || rebased.Values[0] == 0 {
					return fmt.Errorf("transformer: relation guard boundary caller %d frame %d did not close atom %d", callerIndex+1, frameTerm, atom)
				}
			}
		}
	}
	return nil
}

// relationGuardAtomHasLexicalExistential reports whether an otherwise
// rebasable atom still depends on identity owned by the target lexical body.
// That provenance, rather than whether hash-consing happened to allocate a
// caller term during this freeze, determines whether the atom receives an
// application-local formal rank.
func relationGuardAtomHasLexicalExistential(arena *Arena, root ValueTerm) (bool, error) {
	if arena == nil || root == 0 || int(root) >= len(arena.values) {
		return false, fmt.Errorf("transformer: guard atom provenance is outside its term owner")
	}
	values := make(map[ValueTerm]uint8)
	guards := make(map[Guard]uint8)
	var visitValue func(ValueTerm) (bool, error)
	var visitGuard func(Guard) (bool, error)
	visitValue = func(term ValueTerm) (bool, error) {
		if term == 0 || int(term) >= len(arena.values) {
			return false, fmt.Errorf("transformer: guard atom provenance contains foreign value syntax")
		}
		switch values[term] {
		case 1:
			return false, fmt.Errorf("transformer: guard atom provenance contains a value cycle")
		case 2:
			return false, nil
		case 3:
			return true, nil
		}
		values[term] = 1
		node := arena.values[term]
		local := node.owner != (lexicalidentity.StableLexicalBodyID{}) || node.op == valueAllocationResult ||
			node.op == valueRoot && (node.root.Kind == RootResult || node.root.Kind == RootHeapTemplate)
		for _, child := range node.args {
			childLocal, err := visitValue(child)
			if err != nil {
				return false, err
			}
			local = local || childLocal
		}
		if node.integerProof != 0 {
			proofLocal, err := visitValue(node.integerProof)
			if err != nil {
				return false, err
			}
			local = local || proofLocal
		}
		if node.guard != 0 {
			guardLocal, err := visitGuard(node.guard)
			if err != nil {
				return false, err
			}
			local = local || guardLocal
		}
		if local {
			values[term] = 3
		} else {
			values[term] = 2
		}
		return local, nil
	}
	visitGuard = func(guard Guard) (bool, error) {
		if guard == 0 || int(guard) >= len(arena.guards) {
			return false, fmt.Errorf("transformer: guard atom provenance contains foreign guard syntax")
		}
		switch guards[guard] {
		case 1:
			return false, fmt.Errorf("transformer: guard atom provenance contains a guard cycle")
		case 2:
			return false, nil
		case 3:
			return true, nil
		}
		guards[guard] = 1
		node := arena.guards[guard]
		local := false
		if node.value != 0 {
			var err error
			local, err = visitValue(node.value)
			if err != nil {
				return false, err
			}
		}
		for _, child := range node.args {
			childLocal, err := visitGuard(child)
			if err != nil {
				return false, err
			}
			local = local || childLocal
		}
		if local {
			guards[guard] = 3
		} else {
			guards[guard] = 2
		}
		return local, nil
	}
	return visitValue(root)
}

// relationApplicationGuardPair is one target guard and the typed binding of
// each of its atoms.  Boolean syntax remains target-owned; the forest maps its
// source ranks through these atom bindings instead of manufacturing a second
// caller-owned copy of the guard expression.
type relationApplicationGuardPair struct {
	source Guard
	atoms  []relationGuardAtomSubstitution
	// targetScope is the lexical mu lifetime in which source is evaluated.
	// The same hash-consed guard may occur both outside and inside a loop; its
	// Boolean variable identity must not cross that existential boundary.
	targetScope loopMuTerm
}

// relationApplicationGuardPlan is the immutable Boolean boundary inventory for
// one caller-owned call frame. A frozen empty plan is meaningful: branch-free
// callees require no private guard vocabulary. boundAtoms contains only
// caller-arena lexical existentials introduced while rebasing closed atoms;
// target-local producers remain owned by the target invocation ranks.
type relationApplicationGuardPlan struct {
	frame        callFrameTerm
	target       relationVar
	callerScope  loopMuTerm
	scopeFrozen  bool
	targetScopes map[loopMuTerm]struct{}
	// binding is the canonical ordinary-Apply seam: guards remain target-owned
	// and their IN leaves are read lazily from this caller frame.  It contains
	// no imported syntax and no invocation equation state.
	binding    relationLazyApplyBinding
	guards     []relationApplicationGuardPair
	boundAtoms []ValueTerm
	// definition selects sparse BindIn provenance. Definition routes already
	// own their direct guard atoms; only entry-bound equalities are substituted
	// from the declaration owner. Ordinary Apply remains complete.
	definition bool
	frozen     bool
}

func (p relationApplicationGuardPlan) validFor(frame callFrameTerm, target relationVar) bool {
	if !p.frozen || !p.scopeFrozen || p.targetScopes == nil || p.frame != frame || p.target != target || !p.binding.validFor(p.binding.caller, target, frame) || p.guards == nil || p.boundAtoms == nil {
		return false
	}
	for _, guard := range p.guards {
		if guard.source == 0 || guard.atoms == nil {
			return false
		}
		for _, atom := range guard.atoms {
			if atom.source == 0 || atom.substituted == 0 {
				return false
			}
		}
	}
	for _, atom := range p.boundAtoms {
		if atom == 0 {
			return false
		}
	}
	return true
}

// freezeRelationApplicationGuardPlans performs the sole cross-arena guard
// substitution transaction for an acyclic relation forest. It runs after term
// closure has sealed every arena, so deriving metadata cannot grow syntax.
// Closure removes reconstructable SSA syntax but deliberately retains canonical
// N4 registers and frame-result selectors; the typed atom binding keeps those
// in target vocabulary. Call-SCC recursion is not unfolded here: tuple-mu reuses
// this per-edge transaction.
func freezeRelationApplicationGuardPlans(codes []*relationCode) error {
	for owner, caller := range codes {
		if caller == nil || caller.terms == nil || !caller.terms.Sealed() || caller.root == 0 {
			return fmt.Errorf("transformer: relation application guard forest has an invalid sealed caller %d", owner+1)
		}
		caller.applicationGuards = make([]relationApplicationGuardPlan, len(caller.terms.callFrames))
		_, _, topology, err := relationTopology(caller, caller.root)
		if err != nil {
			return fmt.Errorf("transformer: relation application guard caller %d topology: %w", owner+1, err)
		}
		scopes, _, err := formalGuardLexicalScopes(caller)
		if err != nil {
			return fmt.Errorf("transformer: relation application guard caller %d scopes: %w", owner+1, err)
		}
		for _, ref := range topology {
			node := caller.nodes[ref]
			for stepIndex, step := range node.steps {
				if step.kind != boundaryStepApply {
					continue
				}
				if step.apply.frame == 0 || int(step.apply.frame) >= len(caller.terms.callFrames) ||
					step.apply.variable == 0 || int(step.apply.variable) > len(codes) {
					return fmt.Errorf("transformer: relation application guard caller %d node %d step %d has a foreign frame", owner+1, ref, stepIndex)
				}
				if prior := caller.applicationGuards[step.apply.frame]; prior.frozen {
					return fmt.Errorf("transformer: relation application guard frame %d appears more than once", step.apply.frame)
				}
				target := codes[step.apply.variable-1]
				if target == nil || target.terms == nil || !target.terms.Sealed() || target.root == 0 {
					return fmt.Errorf("transformer: relation application guard frame %d has no sealed target", step.apply.frame)
				}
				frame := caller.terms.callFrames[step.apply.frame]
				if frame.variable != step.apply.variable || frame.shape != target.shape {
					return fmt.Errorf("transformer: relation application guard frame %d target shape is inconsistent", step.apply.frame)
				}
				binding, bindingErr := freezeRelationLazyApplyBinding(relationVar(owner+1), caller, target, step.apply)
				if bindingErr != nil {
					return fmt.Errorf("transformer: relation application guard frame %d lazy binding: %w", step.apply.frame, bindingErr)
				}
				bindings, bindErr := NewTermRootBindings(target.shape, caller.shape, frame.values, frame.paths)
				if bindErr != nil {
					return fmt.Errorf("transformer: relation application guard frame %d bindings: %w", step.apply.frame, bindErr)
				}
				scopedSource, sourceErr := reachableScopedRelationGuards(target)
				if sourceErr != nil {
					return fmt.Errorf("transformer: relation application guard frame %d target inventory: %w", step.apply.frame, sourceErr)
				}
				targetLexicalScopes, _, targetScopeErr := formalGuardLexicalScopes(target)
				if targetScopeErr != nil {
					return fmt.Errorf("transformer: relation application guard frame %d target scopes: %w", step.apply.frame, targetScopeErr)
				}
				source := make([]Guard, len(scopedSource))
				for index := range scopedSource {
					source[index] = scopedSource[index].guard
				}
				bound, atoms, rebaseErr := rebaseRelationActionGuards(caller.terms, target.terms, bindings, source)
				if rebaseErr != nil {
					return fmt.Errorf("transformer: relation application guard frame %d substitution: %w", step.apply.frame, rebaseErr)
				}
				pairs := make([]relationApplicationGuardPair, len(source))
				for index := range source {
					pairAtoms, atomErr := relationApplicationGuardAtoms(target.terms, source[index], atoms)
					if atomErr != nil {
						return fmt.Errorf("transformer: relation application guard frame %d atom provenance: %w", step.apply.frame, atomErr)
					}
					pairs[index] = relationApplicationGuardPair{
						source: source[index], atoms: pairAtoms, targetScope: scopedSource[index].scope,
					}
				}
				caller.applicationGuards[step.apply.frame] = relationApplicationGuardPlan{
					frame: step.apply.frame, target: step.apply.variable,
					callerScope: scopes[ref], scopeFrozen: true, targetScopes: relationApplicationScopeSet(targetLexicalScopes),
					binding: binding, guards: pairs, boundAtoms: bound, frozen: true,
				}
			}
		}
	}
	return nil
}

// freezeRelationDefinitionGuardPlans freezes the same sole cross-arena
// substitution for declaration frames, which are not Apply steps and
// therefore are absent from the ordinary relation topology scan.
func freezeRelationDefinitionGuardPlans(codes []*relationCode, definitions []relationProgramDefinition) error {
	for _, definition := range definitions {
		if definition.owner == 0 || int(definition.owner) > len(codes) || definition.target == 0 || int(definition.target) > len(codes) || definition.frame == 0 {
			return fmt.Errorf("transformer: relation definition guard plan is malformed")
		}
		caller, target := codes[definition.owner-1], codes[definition.target-1]
		if caller == nil || target == nil || caller.terms == nil || target.terms == nil || !caller.terms.Sealed() || !target.terms.Sealed() ||
			int(definition.frame) >= len(caller.applicationGuards) || int(definition.frame) >= len(caller.terms.callFrames) {
			return fmt.Errorf("transformer: relation definition guard frame %d is unowned", definition.frame)
		}
		if caller.applicationGuards[definition.frame].frozen {
			return fmt.Errorf("transformer: relation definition guard frame %d appears more than once", definition.frame)
		}
		frame := caller.terms.callFrames[definition.frame]
		if frame.variable != definition.target || frame.shape != target.shape || frame.point != definition.point {
			return fmt.Errorf("transformer: relation definition guard frame %d changed boundary", definition.frame)
		}
		binding, bindingErr := freezeRelationLazyApplyBinding(definition.owner, caller, target, relationApplyRef{variable: definition.target, frame: definition.frame})
		if bindingErr != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d lazy binding: %w", definition.frame, bindingErr)
		}
		bindings, err := NewTermRootBindings(target.shape, caller.shape, frame.values, frame.paths)
		if err != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d bindings: %w", definition.frame, err)
		}
		scoped, err := reachableScopedRelationGuards(target)
		if err != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d target inventory: %w", definition.frame, err)
		}
		targetLexicalScopes, _, targetScopeErr := formalGuardLexicalScopes(target)
		if targetScopeErr != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d target scopes: %w", definition.frame, targetScopeErr)
		}
		pairs, err := freezeRelationDefinitionGuardPairs(caller.terms, target.terms, bindings, scoped)
		if err != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d substitution: %w", definition.frame, err)
		}
		callerScopes, _, scopeErr := formalGuardLexicalScopes(caller)
		if scopeErr != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d owner scopes: %w", definition.frame, scopeErr)
		}
		callerScope, scopeErr := formalGuardDefinitionScope(caller, callerScopes, formalRelationDefinition{
			owner: definition.owner, target: definition.target, point: definition.point, frame: definition.frame,
		})
		if scopeErr != nil {
			return fmt.Errorf("transformer: relation definition guard frame %d owner scope: %w", definition.frame, scopeErr)
		}
		caller.applicationGuards[definition.frame] = relationApplicationGuardPlan{
			frame: definition.frame, target: definition.target, callerScope: callerScope, scopeFrozen: true,
			targetScopes: relationApplicationScopeSet(targetLexicalScopes),
			binding:      binding, guards: pairs, boundAtoms: []ValueTerm{}, definition: true, frozen: true,
		}
	}
	return nil
}

func relationApplicationScopeSet(scopes []loopMuTerm) map[loopMuTerm]struct{} {
	out := make(map[loopMuTerm]struct{})
	for _, scope := range scopes {
		out[scope] = struct{}{}
	}
	return out
}

// freezeRelationDefinitionGuardPairs records exactly the equalities available
// at definition BindIn. Entry-bound atoms are imported into the owner arena;
// atoms which read a post-entry target register stay in the target route's
// direct vocabulary. No raw environment term crosses RebaseTermDAGs.
func freezeRelationDefinitionGuardPairs(
	owner, target *Arena,
	bindings TermRootBindings,
	scoped []relationScopedGuard,
) ([]relationApplicationGuardPair, error) {
	if owner == nil || target == nil || !owner.Sealed() || !target.Sealed() {
		return nil, fmt.Errorf("transformer: definition guard sparse substitution is unowned")
	}
	atoms := make(map[ValueTerm]struct{})
	for _, item := range scoped {
		if err := collectRelationGuardAtoms(target, item.guard, atoms, make(map[Guard]uint8)); err != nil {
			return nil, err
		}
	}
	ordered := make([]ValueTerm, 0, len(atoms))
	for atom := range atoms {
		ordered = append(ordered, atom)
	}
	names := make(map[ValueTerm]string, len(ordered))
	ranks := make(map[ValueTerm]uint32, len(ordered))
	if err := rankStructuralGuardAtoms(target, ordered, names, ranks); err != nil {
		return nil, err
	}
	byRank := make([]ValueTerm, len(ordered))
	for atom, rank := range ranks {
		byRank[rank] = atom
	}
	substitutions := make([]relationGuardAtomSubstitution, len(byRank))
	for index, atom := range byRank {
		rebased, err := rebaseDirectCallTermDAGs(owner, target, bindings, TermRebaseInput{Values: []ValueTerm{atom}})
		if err != nil {
			substitutions[index] = relationGuardAtomSubstitution{source: atom, substituted: atom, targetLocal: true}
			continue
		}
		if len(rebased.Values) != 1 || rebased.Values[0] == 0 {
			return nil, fmt.Errorf("entry-bound atom produced no owner term")
		}
		substitutions[index] = relationGuardAtomSubstitution{source: atom, substituted: rebased.Values[0]}
	}
	pairs := make([]relationApplicationGuardPair, len(scoped))
	for index, item := range scoped {
		pairAtoms, err := relationApplicationGuardAtoms(target, item.guard, substitutions)
		if err != nil {
			return nil, err
		}
		// Definition execution compiles item.guard in the target route's direct
		// vocabulary. The substituted field is retained only as stable typed
		// inventory; unlike ordinary Apply it is never compiled in owner syntax.
		pairs[index] = relationApplicationGuardPair{
			source: item.guard, atoms: pairAtoms, targetScope: item.scope,
		}
	}
	return pairs, nil
}

func relationApplicationGuardAtoms(arena *Arena, guard Guard, substitutions []relationGuardAtomSubstitution) ([]relationGuardAtomSubstitution, error) {
	source := make(map[ValueTerm]struct{})
	if err := collectRelationGuardAtoms(arena, guard, source, make(map[Guard]uint8)); err != nil {
		return nil, err
	}
	out := make([]relationGuardAtomSubstitution, 0, len(source))
	for _, substitution := range substitutions {
		if _, present := source[substitution.source]; present {
			out = append(out, substitution)
			delete(source, substitution.source)
		}
	}
	if len(source) != 0 {
		return nil, fmt.Errorf("guard atom has no exact substitution provenance")
	}
	return out, nil
}

// reachableRelationGuards returns every distinct Choice/step guard in stable
// relation-topology and source-step order. No map iteration participates in
// the published ordering.
func reachableRelationGuards(code *relationCode) ([]Guard, error) {
	scoped, err := reachableScopedRelationGuards(code)
	if err != nil {
		return nil, err
	}
	guards := make([]Guard, len(scoped))
	for index := range scoped {
		guards[index] = scoped[index].guard
	}
	return guards, nil
}

type relationScopedGuard struct {
	guard Guard
	scope loopMuTerm
}

// reachableScopedRelationGuards inventories guards together with their exact
// lexical mu lifetime. LoopMu and LoopPortal enter the binder for their body;
// LoopMu exits retain the enclosing scope. A relation node has one lexical
// owner, so reaching it in two different scopes is malformed rather than a
// reason to merge their Boolean identities.
func reachableScopedRelationGuards(code *relationCode) ([]relationScopedGuard, error) {
	if code == nil || code.terms == nil || !code.terms.Sealed() || code.root == 0 {
		return nil, fmt.Errorf("transformer: relation guard inventory is unowned")
	}
	return reachableScopedRelationGuardsWith(code, reachableValueTermGuards)
}

func reachableScopedRelationGuardsForClosure(code *relationCode) ([]relationScopedGuard, error) {
	if code == nil || code.terms == nil || code.terms.Sealed() || code.root == 0 {
		return nil, fmt.Errorf("transformer: relation guard closure inventory is unowned")
	}
	return reachableScopedRelationGuardsWith(code, relationGuardClosureValueTermGuards)
}

func reachableScopedRelationGuardsWith(code *relationCode, inventory func(*Arena, ValueTerm) ([]Guard, error)) ([]relationScopedGuard, error) {
	if inventory == nil {
		return nil, fmt.Errorf("transformer: relation guard inventory has no value dependency law")
	}
	type guardKey struct {
		guard Guard
		scope loopMuTerm
	}
	type visit struct {
		ref   relationRootRef
		scope loopMuTerm
	}
	seenGuards := make(map[guardKey]struct{})
	guards := make([]relationScopedGuard, 0)
	appendGuard := func(guard Guard, scope loopMuTerm) error {
		if guard == 0 {
			return nil
		}
		if int(guard) >= len(code.terms.guards) {
			return fmt.Errorf("transformer: relation guard inventory contains a foreign guard")
		}
		key := guardKey{guard: guard, scope: scope}
		if _, duplicate := seenGuards[key]; duplicate {
			return nil
		}
		if err := collectRelationGuardAtoms(code.terms, guard, make(map[ValueTerm]struct{}), make(map[Guard]uint8)); err != nil {
			return err
		}
		seenGuards[key] = struct{}{}
		guards = append(guards, relationScopedGuard{guard: guard, scope: scope})
		return nil
	}

	// FIFO traversal preserves relation successor order and is independent of
	// map iteration. The scope table is also the structural well-formedness
	// proof used for shared continuations and irreducible entry portals.
	scopes := make([]loopMuTerm, len(code.nodes))
	arrived := make([]bool, len(code.nodes))
	parents := make(map[loopMuTerm]loopMuTerm)
	queue := []visit{{ref: code.root}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		ref, scope := current.ref, current.scope
		if ref == 0 || int(ref) >= len(code.nodes) {
			return nil, fmt.Errorf("transformer: relation guard inventory escaped relation code")
		}
		if arrived[ref] {
			if scopes[ref] != scope {
				return nil, fmt.Errorf("transformer: relation guard node %d is reached in incompatible loop scopes %d and %d", ref, scopes[ref], scope)
			}
			continue
		}
		arrived[ref], scopes[ref] = true, scope
		node := code.nodes[ref]
		if node.kind == relationNodeChoice {
			if err := appendGuard(node.guard, scope); err != nil {
				return nil, err
			}
		}
		for _, step := range node.steps {
			if err := appendGuard(step.guard, scope); err != nil {
				return nil, err
			}
			if step.kind == boundaryStepEnvironmentWrite {
				valueGuards, err := inventory(code.terms, step.value)
				if err != nil {
					return nil, fmt.Errorf("transformer: relation environment write guard inventory: %w", err)
				}
				for _, guard := range valueGuards {
					if err := appendGuard(guard, scope); err != nil {
						return nil, err
					}
				}
			}
			if step.kind == boundaryStepEffect {
				if code.effects == nil || step.effect == 0 || int(step.effect) >= len(code.effects.nodes) {
					return nil, fmt.Errorf("transformer: relation effect guard inventory has foreign syntax")
				}
				access, err := freezeEffectNodeCoordinateAccess(code.effects.nodes[step.effect])
				if err != nil {
					return nil, fmt.Errorf("transformer: relation effect guard inventory: %w", err)
				}
				for _, term := range access.readTerms {
					valueGuards, err := inventory(code.terms, term)
					if err != nil {
						return nil, fmt.Errorf("transformer: relation effect value guard inventory: %w", err)
					}
					for _, guard := range valueGuards {
						if err := appendGuard(guard, scope); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		switch node.kind {
		case relationNodeSequence:
			if node.next != 0 {
				queue = append(queue, visit{ref: node.next, scope: scope})
			}
		case relationNodeChoice:
			if node.whenTrue != 0 {
				queue = append(queue, visit{ref: node.whenTrue, scope: scope})
			}
			if node.whenFalse != 0 {
				queue = append(queue, visit{ref: node.whenFalse, scope: scope})
			}
		case relationNodeLoopMu, relationNodeLoopPortal:
			if node.binder == 0 || int(node.binder) >= len(code.terms.loopMus) {
				return nil, fmt.Errorf("transformer: relation guard loop node %d has no lexical binder", ref)
			}
			if prior, ok := parents[node.binder]; ok && prior != scope {
				return nil, fmt.Errorf("transformer: relation guard binder %d has incompatible enclosing scopes %d and %d", node.binder, prior, scope)
			}
			parents[node.binder] = scope
			queue = append(queue, visit{ref: node.body, scope: node.binder})
			if node.kind == relationNodeLoopMu {
				for _, exit := range node.exits {
					if exit != 0 {
						queue = append(queue, visit{ref: exit, scope: scope})
					}
				}
			}
		}
	}
	return guards, nil
}

// reachableValueTermGuards inventories every Select predicate in one sealed
// ValueTerm DAG. It is the shared freeze-time dependency primitive for any
// formal transaction which must preserve value/guard correlation.
func reachableValueTermGuards(arena *Arena, root ValueTerm) ([]Guard, error) {
	if arena == nil || root == 0 || int(root) >= len(arena.values) || !arena.Sealed() {
		return nil, fmt.Errorf("value guard inventory is unowned")
	}
	return inventoryValueTermGuards(arena, root)
}

func relationGuardClosureValueTermGuards(arena *Arena, root ValueTerm) ([]Guard, error) {
	if arena == nil || root == 0 || int(root) >= len(arena.values) || arena.Sealed() {
		return nil, fmt.Errorf("value guard closure inventory is unowned")
	}
	return inventoryValueTermGuards(arena, root)
}

func inventoryValueTermGuards(arena *Arena, root ValueTerm) ([]Guard, error) {
	seenValues := make(map[ValueTerm]struct{})
	seenGuards := make(map[Guard]struct{})
	guards := make([]Guard, 0)
	stack := []ValueTerm{root}
	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if term == 0 || int(term) >= len(arena.values) {
			return nil, fmt.Errorf("value guard inventory contains foreign syntax")
		}
		if _, duplicate := seenValues[term]; duplicate {
			continue
		}
		seenValues[term] = struct{}{}
		node := arena.values[term]
		if node.op == valueSelect {
			if node.guard == 0 || int(node.guard) >= len(arena.guards) {
				return nil, fmt.Errorf("value guard inventory contains a malformed Select")
			}
			if _, duplicate := seenGuards[node.guard]; !duplicate {
				if err := collectRelationGuardAtoms(arena, node.guard, make(map[ValueTerm]struct{}), make(map[Guard]uint8)); err != nil {
					return nil, err
				}
				seenGuards[node.guard] = struct{}{}
				guards = append(guards, node.guard)
			}
		}
		for index := len(node.args) - 1; index >= 0; index-- {
			stack = append(stack, node.args[index])
		}
	}
	return guards, nil
}
