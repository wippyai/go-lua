package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type formalApplyBoundaryRoot struct {
	schema       state.BoundaryFactorRoot
	slot         FormalSlot
	boundarySlot statekey.Value
	raw          product.Value
	value        product.Value
	destination  int
}

type formalApplyBoundaryDestination struct {
	target   state.BoundaryFactorRootTarget[FormalSlot]
	path     keyspace.Key
	optional bool
	required bool
}

// formalApplyBoundaryRootMap seals the functional destination schema relation.
// A source owns only its destination ordinal; every preimage of one destination
// therefore receives the same structural path by construction.
func formalApplyBoundaryRootMap(source []formalApplyBoundaryRoot, destinations []formalApplyBoundaryDestination) (state.BoundaryRootMap, error) {
	out := make(state.BoundaryRootMap, len(source))
	for index, root := range source {
		if root.destination < 0 || root.destination >= len(destinations) {
			return nil, fmt.Errorf("transformer: formal Apply source %d has no destination schema", index)
		}
		out[index] = state.BoundaryRootBinding{FromRoot: index, ToRoot: root.destination, To: destinations[root.destination].path}
	}
	return out, nil
}

func compactFormalApplyBoundary(source []formalApplyBoundaryRoot, destinations []formalApplyBoundaryDestination) ([]formalApplyBoundaryRoot, []formalApplyBoundaryDestination, error) {
	remap := make(map[int]int, len(destinations))
	active := make([]bool, len(destinations))
	for index := range source {
		old := source[index].destination
		if old < 0 || old >= len(destinations) {
			return nil, nil, fmt.Errorf("transformer: formal Apply source %d has no frozen destination", index)
		}
		active[old] = true
	}
	compact := make([]formalApplyBoundaryDestination, 0, len(destinations))
	for old, destination := range destinations {
		if !active[old] {
			if destination.required {
				return nil, nil, fmt.Errorf("transformer: mandatory frozen Apply destination %d is inactive", old)
			}
			continue
		}
		remap[old] = len(compact)
		compact = append(compact, destination)
	}
	for index := range source {
		source[index].destination = remap[source[index].destination]
	}
	return source, compact, nil
}

// formalApplyResultDestinationRelation is the frozen one-to-many image of one
// callee result: the point-owned canonical CallResult carrier and every
// linked state target. Destination schemas live only in the destination table.
type formalApplyResultDestinationRelation struct {
	destinations []int
}

type formalApplyTerminalMode uint8

const (
	formalApplyTerminalNormal formalApplyTerminalMode = iota + 1
	formalApplyTerminalNonreturning
	formalApplyTerminalDefinition
)

func formalApplyValueAt(values state.ValueFactor[FormalSlot], slot FormalSlot, regValue product.Value) product.Value {
	if values.Top {
		return product.Top()
	}
	if value, ok := values.Values[slot]; ok {
		return value
	}
	return regValue
}

func (a *formalTupleAlgebra) formalizeApplyPath(body *relationProgramBody, path keyspace.Key) (keyspace.Key, error) {
	if path.Kind == keyspace.KindInvalid {
		return keyspace.Key{}, nil
	}
	span, ok := a.program.formalFibers.span(body.variable)
	if !ok {
		return keyspace.Key{}, errFormalComponentForeignOwner
	}
	return body.productDomain.RekeyStructuralKeyFormal(span.rekey, path)
}

func (a *formalTupleAlgebra) formalApplyResultValues(
	step *formalApplyStep,
	region formalApplyCorrelatedRegion,
	actuals formalApplyActualTuple,
	callerValues, targetValues state.ValueFactor[FormalSlot],
) ([]product.Value, [][]formalQualifiedBinding, []bool, error) {
	frame := step.linked
	count := int(frame.shape.Results)
	caller := &a.program.bodies[step.owner-1]
	target := &a.program.bodies[step.target-1]
	bottom := product.Bottom(a.program.registry)
	paramValue := func(body *relationProgramBody, values state.ValueFactor[FormalSlot], index int) product.Value {
		slot, ok := a.program.formalSlots.Slot(body.body, Root{Kind: RootParam, Index: uint32(index)})
		if !ok {
			return bottom
		}
		return formalApplyValueAt(values, slot, bottom)
	}
	contracts := prepareBoundaryReturnContractPlanFromValues(a.program.registry, caller, target, frame,
		func(index int) product.Value { return paramValue(caller, callerValues, index) },
		func(index int) product.Value { return paramValue(target, targetValues, index) })
	capability, err := region.target.materializeValueFactorAccess(step.resultValueAccess, step.resultValueFactorGroups)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("transformer: formal Apply result value factors: %w", err)
	}
	values := make([]product.Value, count)
	bindings := make([][]formalQualifiedBinding, count)
	exactResults := make([]bool, count)
	for index := range exactResults {
		exactResults[index] = true
	}
	for result := 0; result < count; result++ {
		resultSlot, ok := a.program.formalSlots.Slot(target.body, Root{Kind: RootResult, Index: uint32(result)})
		if !ok {
			return nil, nil, nil, fmt.Errorf("transformer: formal Apply result %d has no canonical N5 ReturnSlot", result)
		}
		joined := product.Bottom(a.program.registry)
		for occurrenceIndex, occurrence := range region.occurrences {
			transaction := occurrence.code.outcomes[occurrence.ref].returnTransaction
			for bindingIndex := 0; bindingIndex < transaction.transaction.ResultBindingCount(); bindingIndex++ {
				sourceIndex, resultIndex, exact := transaction.transaction.ResultBinding(bindingIndex)
				if !exact || sourceIndex < 0 || sourceIndex >= len(transaction.sources) || resultIndex < 0 || resultIndex >= count {
					return nil, nil, nil, fmt.Errorf("transformer: formal Apply result %d occurrence %d has malformed return binding %d", result, occurrenceIndex, bindingIndex)
				}
				if resultIndex != result {
					continue
				}
				source := transaction.sources[sourceIndex]
				binding, err := formalApplyInputBinding(step.binding, source, 0, occurrence.scope)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("transformer: formal Apply result %d occurrence %d ref=%d scope=%d input binding: %w",
						result, occurrenceIndex, occurrence.ref, occurrence.scope, err)
				}
				bindings[result] = append(bindings[result], binding)
				specialized, exact := a.evaluateFormalApplyResultBinding(region, actuals, binding, capability)
				if !exact {
					// A valid borrowed term over still-symbolic caller inputs is the
					// canonical residual relation, not a failed specialization. Its
					// ground Values coordinate remains Bottom until a concrete Apply
					// environment instantiates the same retained view.
					if !binding.validForAuthority(region.caller.authority) {
						return nil, nil, nil, fmt.Errorf("transformer: formal Apply result %d source is malformed", result)
					}
					exactResults[result] = false
					continue
				}
				joined = product.Join(a.program.registry, joined, specialized)
			}
		}
		if len(bindings[result]) == 0 {
			joined = formalApplyValueAt(targetValues, resultSlot, product.Absent(a.program.registry))
		}
		values[result] = contracts.NormalizeResult(result, joined)
	}
	return values, bindings, exactResults, nil
}

// evaluateFormalApplyResultBinding specializes one callee return source while
// the correlated region still owns both tuple leaves. Target input roots read
// through the frozen caller binding; target MID/Result roots read from the
// stabilized target leaf. No borrowed target syntax survives publication.
func (a *formalTupleAlgebra) evaluateFormalApplyResultBinding(
	region formalApplyCorrelatedRegion,
	actuals formalApplyActualTuple,
	binding formalQualifiedBinding,
	capability *formalValueFactorAccess,
) (product.Value, bool) {
	if !binding.apply.present() {
		return region.caller.evalQualifiedValue(binding)
	}
	view := binding.apply
	arena, term := binding.value.arena, binding.value.term
	if arena == nil || arena != view.binding.targetArena || term == 0 || int(term) >= len(arena.values) || !arena.Sealed() {
		return product.Value{}, false
	}
	var evaluateTargetBinding func(formalQualifiedBinding) (product.Value, bool)
	var resolveRoot func(Root, loopMuTerm) (product.Value, bool)
	resolveRoot = func(root Root, scope loopMuTerm) (product.Value, bool) {
		if view.binding.targetShape.validateInput(root) {
			var actual product.Value
			found := false
			for _, candidate := range actuals.values {
				if candidate.targetRoot == root {
					actual, found = candidate.value, true
					break
				}
			}
			if !found {
				return product.Value{}, false
			}
			constraint, exact := a.formalApplyInputConstraint(region, view.binding.target, root)
			if !exact {
				return product.Value{}, false
			}
			if product.Equal(a.program.registry, constraint, product.Bottom(a.program.registry)) {
				return actual, true
			}
			return factapply.RefineProductValueConstraint(a.program.registry, actual, constraint), true
		}
		if root.Kind == RootMiddle {
			alternatives, present, exact := a.formalApplyTargetMiddleBindings(region, root)
			if !exact {
				return product.Value{}, false
			}
			if present {
				joined := product.Bottom(a.program.registry)
				for _, alternative := range alternatives {
					value, valueExact := evaluateTargetBinding(alternative)
					if !valueExact {
						return product.Value{}, false
					}
					joined = product.Join(a.program.registry, joined, value)
				}
				constraint, constraintExact := region.target.valueAtRoot(root)
				if !constraintExact {
					return product.Value{}, false
				}
				if !product.Equal(a.program.registry, constraint, product.Bottom(a.program.registry)) {
					joined = factapply.RefineProductValueConstraint(a.program.registry, joined, constraint)
				}
				return joined, true
			}
		}
		return region.target.evalArenaRootValue(binding.value.owner, arena, root, scope, formalApplyTermView{})
	}
	var evaluateTargetTerm func(ValueTerm, loopMuTerm) (product.Value, bool)
	evaluateTargetTerm = func(candidateTerm ValueTerm, scope loopMuTerm) (product.Value, bool) {
		var resolver valueNodeLeafResolver
		resolver = valueNodeLeafResolver{
			guard: func(guard Guard) (bool, bool, bool) {
				return region.target.exactGuardPossibilities(binding.value.owner, arena, scope, guard)
			},
			root: func(root Root) (product.Value, bool) { return resolveRoot(root, scope) },
			slot: func(slot statekey.Value) (product.Value, bool) {
				root, ok := region.target.rootForValueSlot(binding.value.owner, slot)
				if !ok {
					return product.Value{}, false
				}
				return resolveRoot(root, scope)
			},
			dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
				if capability == nil {
					return product.Value{}, false
				}
				return resolveFormalDynamicValue(region.target.body, region.target.span, node, args, capability.factors, func(child ValueTerm) (product.Value, bool) {
					return arena.evalValueCanonicalWithLeaves(child, resolver)
				})
			},
			allocationResult: func(candidate valueNode) (product.Value, bool) {
				return arena.allocationResult(candidate.allocation, candidate.resultIndex)
			},
		}
		return arena.evalValueCanonicalWithLeaves(candidateTerm, resolver)
	}
	evaluateTargetBinding = func(candidate formalQualifiedBinding) (product.Value, bool) {
		if candidate.apply.present() || candidate.value.owner != binding.value.owner || candidate.value.arena != arena ||
			candidate.value.term == 0 || int(candidate.value.term) >= len(arena.values) {
			return product.Value{}, false
		}
		return evaluateTargetTerm(candidate.value.term, candidate.scope)
	}
	return evaluateTargetTerm(term, binding.scope)
}

func (a *formalTupleAlgebra) formalApplyTargetMiddleBindings(region formalApplyCorrelatedRegion, root Root) ([]formalQualifiedBinding, bool, bool) {
	if a == nil || a.program == nil || a.program.formalSlots == nil || !region.target.valid() || root.Kind != RootMiddle {
		return nil, false, false
	}
	slot, ok := a.program.formalSlots.Slot(region.target.body.body, root)
	if !ok {
		return nil, false, false
	}
	for ordinal, descriptor := range region.target.span.descriptors() {
		if descriptor.role != formalFiberMiddleValue || descriptor.slot != slot {
			continue
		}
		leaf, present := region.target.leaves.leaf(formalFiberOrdinal(ordinal))
		if !present {
			return nil, false, false
		}
		if leaf == 0 {
			return nil, false, true
		}
		terminal, err := region.target.authority.terminal(leaf)
		if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) == 0 {
			return nil, false, false
		}
		return append([]formalQualifiedBinding(nil), terminal.bindings...), true, true
	}
	return nil, false, false
}

// formalApplyCallerInputAlias reads the caller's current symbolic Middle
// carrier and proves that it is still exactly one immutable boundary input.
// This is the SSA-aware inverse of root-input instantiation: reassignment or a
// join changes the binding terminal and therefore cannot refine the original
// parameter on normal return.
func (a *formalTupleAlgebra) formalApplyCallerInputAlias(evaluator formalTupleLeafEvaluator, root Root) (Root, bool) {
	if a == nil || a.program == nil || a.program.formalSlots == nil || !evaluator.valid() || root.Kind != RootMiddle {
		return Root{}, false
	}
	slot, ok := a.program.formalSlots.Slot(evaluator.body.body, root)
	if !ok {
		return Root{}, false
	}
	for ordinal, descriptor := range evaluator.span.descriptors() {
		if descriptor.role != formalFiberMiddleValue || descriptor.slot != slot {
			continue
		}
		leaf, present := evaluator.leaves.leaf(formalFiberOrdinal(ordinal))
		if !present {
			return Root{}, false
		}
		if leaf == 0 {
			return Root{}, false
		}
		terminal, err := evaluator.authority.terminal(leaf)
		if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) != 1 {
			return Root{}, false
		}
		binding := terminal.bindings[0]
		if binding.apply.present() || binding.value.owner != evaluator.variable || binding.value.arena != evaluator.authority.terms ||
			binding.value.term == 0 || int(binding.value.term) >= len(binding.value.arena.values) {
			return Root{}, false
		}
		node := binding.value.arena.values[binding.value.term]
		return node.root, node.op == valueRoot && evaluator.body.relation.shape.validateInput(node.root)
	}
	return Root{}, false
}

// formalApplyInputConstraint reads the one target Middle value paired with a
// boundary input. Bottom is the symbolic-entry identity (no added
// constraint); a branch refinement replaces it with the exact product
// constraint to instantiate against the caller actual.
func (a *formalTupleAlgebra) formalApplyInputConstraint(region formalApplyCorrelatedRegion, target relationVar, input Root) (product.Value, bool) {
	if a == nil || a.program == nil || a.program.formalTemplate == nil || target == 0 ||
		int(target) > len(a.program.formalTemplate.rootInputs) || region.target.variable != target {
		return product.Value{}, false
	}
	for _, binding := range a.program.formalTemplate.rootInputs[target-1].bindings {
		if binding.input == input {
			return region.target.valueAtRoot(binding.middle)
		}
	}
	return product.Value{}, false
}

func formalApplyDirectInputAlias(binding formalQualifiedBinding) (formalQualifiedBinding, bool) {
	view := binding.apply
	if !view.present() || binding.value.arena != view.binding.targetArena || binding.value.term == 0 ||
		int(binding.value.term) >= len(binding.value.arena.values) {
		return formalQualifiedBinding{}, false
	}
	node := binding.value.arena.values[binding.value.term]
	if node.op != valueRoot || !view.binding.targetShape.validateInput(node.root) {
		return formalQualifiedBinding{}, false
	}
	value, ok := view.binding.inputValue(node.root)
	if !ok {
		return formalQualifiedBinding{}, false
	}
	path, present, ok := view.binding.inputPath(node.root)
	if !ok {
		return formalQualifiedBinding{}, false
	}
	return formalQualifiedBinding{value: value, path: path, pathPresent: present, scope: view.callerScope}, true
}

// formalApplyBoundaryParametricValue proves that a retained target value is a
// pure function of immutable boundary inputs.  Product evaluation can be
// exact while still losing caller-input correlation (for example
// select(truthy(p1), p0, constant)); that value must retain its Apply view.
// Conversely, target-local environment/cell/frame/allocation observations are
// not a boundary transformer and must never be published as a caller binding.
// The sealed Arena remains the only expression syntax and every ValueOp keeps
// its executable semantics in the canonical value-node algebra.
func formalApplyBoundaryParametricValue(binding formalQualifiedBinding) bool {
	view := binding.apply
	arena := binding.value.arena
	if !view.present() || arena == nil || arena != view.binding.targetArena ||
		binding.value.term == 0 || int(binding.value.term) >= len(arena.values) || !arena.Sealed() {
		return false
	}
	type dependence struct {
		state   uint8
		safe    bool
		depends bool
	}
	values := make(map[ValueTerm]dependence)
	guards := make(map[Guard]dependence)
	var visitValue func(ValueTerm) (bool, bool)
	var visitGuard func(Guard) (bool, bool)
	visitValue = func(term ValueTerm) (bool, bool) {
		if term == 0 || int(term) >= len(arena.values) {
			return false, false
		}
		if known := values[term]; known.state != 0 {
			if known.state == 1 { // A sealed value circuit must be acyclic.
				return false, false
			}
			return known.safe, known.depends
		}
		values[term] = dependence{state: 1}
		node := arena.values[term]
		if node.owner != (lexicalidentity.StableLexicalBodyID{}) {
			values[term] = dependence{state: 2}
			return false, false
		}
		safe, depends := true, false
		switch node.op {
		case valueRoot:
			safe = view.binding.targetShape.validateInput(node.root)
			depends = safe
		case valueConstant:
			// A constant is a valid part of a boundary-parametric expression,
			// but does not make an otherwise constant term parametric.
		case valueEnvironment, valueCellResult, valueFrameResult,
			valueDynamicRead, valueDynamicTableRead, valueLoopContinuation,
			valueAllocationResult, valueInvalid:
			safe = false
		default:
			for _, child := range node.args {
				childSafe, childDepends := visitValue(child)
				safe = safe && childSafe
				depends = depends || childDepends
			}
			if safe && node.integerProof != 0 {
				childSafe, childDepends := visitValue(node.integerProof)
				safe = childSafe
				depends = depends || childDepends
			}
			if safe && node.guard != 0 {
				guardSafe, guardDepends := visitGuard(node.guard)
				safe = guardSafe
				depends = depends || guardDepends
			}
		}
		values[term] = dependence{state: 2, safe: safe, depends: depends}
		return safe, depends
	}
	visitGuard = func(guard Guard) (bool, bool) {
		if guard == 0 || int(guard) >= len(arena.guards) {
			return false, false
		}
		if known := guards[guard]; known.state != 0 {
			if known.state == 1 {
				return false, false
			}
			return known.safe, known.depends
		}
		guards[guard] = dependence{state: 1}
		node := arena.guards[guard]
		safe, depends := node.op == guardTrue || node.op == guardFalse, false
		switch node.op {
		case guardTruthy, guardFalsy:
			safe, depends = visitValue(node.value)
		case guardAnd, guardOr:
			safe = true
			for _, child := range node.args {
				childSafe, childDepends := visitGuard(child)
				safe = safe && childSafe
				depends = depends || childDepends
			}
		}
		guards[guard] = dependence{state: 2, safe: safe, depends: depends}
		return safe, depends
	}
	safe, depends := visitValue(binding.value.term)
	return safe && depends
}

func (a *formalTupleAlgebra) formalApplyRegionPublication(
	step *formalApplyStep,
	footprint formalOperatorCoordinateFootprint,
	region formalApplyCorrelatedRegion,
	mode formalApplyTerminalMode,
) (publication formalApplyLeafPublication, reachable bool, err error) {
	phase := "product factors"
	defer func() {
		if err != nil {
			err = fmt.Errorf("transformer: formal Apply region phase %q: %w", phase, err)
		}
	}()
	if mode != formalApplyTerminalNormal && mode != formalApplyTerminalNonreturning && mode != formalApplyTerminalDefinition {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply terminal mode is unowned")
	}
	callerValues, callerFactors, err := region.caller.productFactors()
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply caller factors: %w", err)
	}
	targetValues, targetFactors, err := region.target.productFactors()
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply target factors: %w", err)
	}
	targetFormalValues, err := region.target.formalValuesFactor()
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply target formal Values: %w", err)
	}
	for index, factor := range targetFactors {
		families, familyErr := region.target.authority.product.CoordinateFamilies(factor.Lane())
		if familyErr != nil {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply target factor %d families: %w", index, familyErr)
		}
		if len(families) == 0 {
			continue
		}
		targetFactors[index], familyErr = region.target.authority.product.SelectCoordinateLaneFactor(
			factor, footprint.source,
		)
		if familyErr != nil {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply target factor %d selection: %w", index, familyErr)
		}
	}
	phase = "actual inputs"
	actuals, err := a.evaluateFormalApplyActuals(step, region.caller)
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply actuals: %w", err)
	}
	phase = "identity authority"
	identityAuthority, err := a.formalApplyIdentityAuthority(step.binding, actuals)
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply identity authority: %w", err)
	}
	phase = "execution capability"
	capability, err := a.formalApplySelectedFactorExecutionCapabilityRef(region.target, footprint.sourceSelector)
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply execution capability: %w", err)
	}
	phase = "identity substitution"
	identitySupport, err := capability.specializedIdentitySupport(a.ctx, region.target.authority, targetFormalValues, targetFactors)
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply specialized execution capability: %w", err)
	}
	identityPlan, unreachable, err := state.SealIdentitySubstitutionPlanWithSupport(
		a.ctx, region.target.authority.product, region.target.authority.coordinateKeys,
		identityAuthority, identitySupport,
	)
	if err != nil || unreachable {
		if err != nil {
			err = fmt.Errorf("transformer: formal Apply identity plan: %w", err)
		}
		return formalApplyLeafPublication{}, !unreachable, err
	}
	imageValues, unreachable, err := state.ApplyValueFactorIdentitySubstitution(
		a.ctx, region.target.authority.product, identityPlan, targetValues,
	)
	if err != nil || unreachable {
		if err != nil {
			err = fmt.Errorf("transformer: formal Apply Values substitution: %w", err)
		}
		return formalApplyLeafPublication{}, !unreachable, err
	}
	imageFactors := make([]state.LaneFactor, len(targetFactors))
	for index := range targetFactors {
		imageFactors[index], unreachable, err = region.target.authority.product.ApplyIdentitySubstitutionFactor(a.ctx, identityPlan, targetFactors[index])
		if err != nil || unreachable {
			if err != nil {
				err = fmt.Errorf("transformer: formal Apply factor %d substitution: %w", index, err)
			}
			return formalApplyLeafPublication{}, !unreachable, err
		}
	}

	var resultValues, rawResultValues []product.Value
	var resultBindings [][]formalQualifiedBinding
	var resultExact []bool
	if mode == formalApplyTerminalNormal {
		resultValues, resultBindings, resultExact, err = a.formalApplyResultValues(step, region, actuals, callerValues, targetValues)
		if err != nil {
			return formalApplyLeafPublication{}, false, err
		}
		rawResultValues = append([]product.Value(nil), resultValues...)
		for index := range resultValues {
			resultValues[index], unreachable, err = state.ApplyIdentitySubstitutionValue(region.target.authority.product, identityPlan, resultValues[index])
			if err != nil || unreachable {
				if err != nil {
					err = fmt.Errorf("transformer: formal Apply result %d identity substitution: %w", index, err)
				}
				return formalApplyLeafPublication{}, !unreachable, err
			}
		}
	}

	phase = "boundary roots"
	frame := step.linked
	caller := &a.program.bodies[step.owner-1]
	target := &a.program.bodies[step.target-1]
	bottom := product.Bottom(a.program.registry)
	sourceRoots := make([]formalApplyBoundaryRoot, 0,
		len(frame.rootCircuit)+frame.mutableAmbientCount()+len(frame.exitBridges)+len(resultValues)+len(frame.resultSources))
	destinations := make([]formalApplyBoundaryDestination, len(frame.boundary.destinations))
	for index, frozen := range frame.boundary.destinations {
		destinations[index].optional = frozen.optional
		destinations[index].required = !frozen.optional && (mode == formalApplyTerminalNormal || (frozen.kind != linkedFrameBoundaryDestinationCanonicalResult && frozen.kind != linkedFrameBoundaryDestinationStateResult))
	}
	destinationSet := make([]bool, len(destinations))
	setDestinationRoot := func(ordinal int, root state.BoundaryFactorRootTarget[FormalSlot], path keyspace.Key) error {
		if ordinal < 0 || ordinal >= len(destinations) || destinationSet[ordinal] {
			return fmt.Errorf("transformer: formal Apply boundary destination ordinal %d is invalid or repeated", ordinal)
		}
		prior := destinations[ordinal]
		destinations[ordinal] = formalApplyBoundaryDestination{target: root, path: path, optional: prior.optional, required: prior.required}
		destinationSet[ordinal] = true
		return nil
	}
	appendSource := func(edgeOrdinal int, slot FormalSlot, raw, value product.Value) error {
		if edgeOrdinal < 0 || edgeOrdinal >= len(frame.boundary.edges) {
			return fmt.Errorf("transformer: formal Apply source has no frozen topology edge")
		}
		edge := frame.boundary.edges[edgeOrdinal]
		path := keyspace.Key{}
		if edge.source.Path.Kind != keyspace.KindInvalid {
			var pathErr error
			path, pathErr = a.formalizeApplyPath(target, edge.source.Path)
			if pathErr != nil {
				return pathErr
			}
		} else if edge.root.Kind != 0 {
			sourceSlot, exact := a.program.formalSlots.Slot(target.body, edge.root)
			if !exact {
				return fmt.Errorf("transformer: formal Apply topology edge %d has no source slot", edgeOrdinal)
			}
			root, exact := sourceSlot.Root()
			if !exact {
				return fmt.Errorf("transformer: formal Apply topology edge %d has no source root", edgeOrdinal)
			}
			path, exact = region.target.authority.coordinateKeys.InternFormalRoot(root)
			if !exact {
				return fmt.Errorf("transformer: formal Apply topology edge %d source root is foreign", edgeOrdinal)
			}
		}
		sourceRoots = append(sourceRoots, formalApplyBoundaryRoot{
			schema: state.BoundaryFactorRoot{Slot: edge.source.Slot, Path: path}, slot: slot, boundarySlot: edge.source.Slot,
			raw: raw, value: value, destination: edge.destination,
		})
		return nil
	}
	for index, wire := range frame.rootCircuit {
		actual := actuals.values[index]
		destination := frame.boundary.inputs[index]
		destinationPath := keyspace.Key{}
		if concrete := frame.boundary.destinations[destination].path; concrete.Kind != keyspace.KindInvalid {
			var pathErr error
			destinationPath, pathErr = a.formalizeApplyPath(caller, concrete)
			if pathErr != nil {
				return formalApplyLeafPublication{}, false, pathErr
			}
		}
		if err := setDestinationRoot(destination, state.BoundaryFactorRootTarget[FormalSlot]{
			Slot: actual.slot,
			// A normal-return target parameter is the callee's registered
			// postcondition on the caller actual. Publish its complete product
			// through the same root transport as every coordinate family; omitting
			// this scalar loses Presence (and any future value axis) while leaving
			// the structural fibers apparently transported. Definition BindIn and
			// nonreturning terminals are not normal-return postconditions.
			WriteScalar: actual.slotSet && (wire.root.Kind != RootParam || mode == formalApplyTerminalNormal),
		}, destinationPath); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
		rawValue := formalApplyValueAt(targetValues, actual.targetSlot, bottom)
		imageValue := formalApplyValueAt(imageValues, actual.targetSlot, bottom)
		if wire.root.Kind == RootParam && mode == formalApplyTerminalNormal {
			// Parameters are pass-by-value, so their postcondition is represented
			// by the root-input Middle constraint rather than a mutable Values
			// coordinate. Instantiate that registered product constraint against
			// the caller actual before the ordinary boundary root transport. This
			// is the same canonical constraint law used by result expressions and
			// automatically covers Presence and every future product axis.
			constraint, exact := a.formalApplyInputConstraint(region, step.target, wire.root)
			if !exact {
				return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply parameter %d has no exact normal-return constraint", index)
			}
			rawValue, err = instantiateFormalInputProduct(a.program.registry, actual.value, constraint)
			if err != nil {
				return formalApplyLeafPublication{}, false, err
			}
			imageValue, unreachable, err = state.ApplyIdentitySubstitutionValue(region.target.authority.product, identityPlan, rawValue)
			if err != nil || unreachable {
				return formalApplyLeafPublication{}, !unreachable, err
			}
		}
		if err := appendSource(frame.boundary.inputEdges[index], actual.targetSlot, rawValue, imageValue); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
		if wire.root.Kind == RootParam && mode == formalApplyTerminalNormal && actual.slotSet {
			// A caller Middle is the current SSA carrier; the frozen outbound path
			// is its persistent boundary identity.  A callee postcondition applies
			// to both only when the call-site path lens proves they are the same
			// root.  Publishing just the Middle loses wrapper-to-wrapper parameter
			// facts, while publishing every lexical parameter would be unsound after
			// reassignment.  The presealed path role makes the distinction exact.
			actualRoot, rootExact := actual.slot.relationRoot()
			persistentRoot, aliasExact := a.formalApplyCallerInputAlias(region.caller, actualRoot)
			if rootExact && aliasExact && wire.destination.hasPersistent {
				persistentSlot, slotExact := a.program.formalSlots.Slot(caller.body, persistentRoot)
				persistentConcrete, pathExact := caller.rootPathKey(persistentRoot)
				if !slotExact || !pathExact || persistentConcrete != wire.destination.persistentPath {
					return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply parameter %d persistent destination is outside caller boundary", index)
				}
				persistentPath, pathErr := a.formalizeApplyPath(caller, wire.destination.persistentPath)
				if pathErr != nil {
					return formalApplyLeafPublication{}, false, pathErr
				}
				if persistentSlot != actual.slot {
					edgeOrdinal := frame.boundary.persistentEdges[index]
					if edgeOrdinal < 0 {
						return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply persistent input has no frozen topology edge")
					}
					edge := frame.boundary.edges[edgeOrdinal]
					if err := setDestinationRoot(edge.destination, state.BoundaryFactorRootTarget[FormalSlot]{Slot: persistentSlot, WriteScalar: true}, persistentPath); err != nil {
						return formalApplyLeafPublication{}, false, err
					}
					if err := appendSource(edgeOrdinal, actual.targetSlot, rawValue, imageValue); err != nil {
						return formalApplyLeafPublication{}, false, err
					}
				}
			}
		}
	}
	for ambientIndex, wire := range frame.ambientCircuit {
		if !wire.target.mutable {
			continue
		}
		actual := actuals.values[len(frame.rootCircuit)+ambientIndex]
		edgeOrdinal := frame.boundary.ambientEdges[ambientIndex]
		if edgeOrdinal < 0 || edgeOrdinal >= len(frame.boundary.edges) {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply ambient has no frozen topology edge")
		}
		destination := frame.boundary.edges[edgeOrdinal].destination
		destinationPath := keyspace.Key{}
		if destination >= 0 && frame.boundary.destinations[destination].path.Kind != keyspace.KindInvalid {
			var pathErr error
			destinationPath, pathErr = a.formalizeApplyPath(caller, frame.boundary.destinations[destination].path)
			if pathErr != nil {
				return formalApplyLeafPublication{}, false, pathErr
			}
		}
		if err := setDestinationRoot(destination, state.BoundaryFactorRootTarget[FormalSlot]{Slot: actual.slot, WriteScalar: actual.slotSet}, destinationPath); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
		if err := appendSource(frame.boundary.ambientEdges[ambientIndex], actual.targetSlot, formalApplyValueAt(targetValues, actual.targetSlot, bottom), formalApplyValueAt(imageValues, actual.targetSlot, bottom)); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
	}
	for bridgeIndex, bridge := range frame.exitBridges {
		root, ok := region.target.rootForValueSlot(step.target, bridge.root.Slot)
		if !ok {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply exit bridge has no formal source root")
		}
		slot, ok := a.program.formalSlots.Slot(a.program.bodies[step.target-1].body, root)
		if !ok || bridge.input < 0 || bridge.input >= len(frame.rootCircuit) {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply exit bridge is malformed")
		}
		if err := appendSource(frame.boundary.exitEdges[bridgeIndex], slot, formalApplyValueAt(targetValues, slot, bottom), formalApplyValueAt(imageValues, slot, bottom)); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
	}
	resultRelations := make([]formalApplyResultDestinationRelation, len(resultValues))
	for result := range resultValues {
		descriptor, descriptorErr := a.applyResultDescriptor(step.owner, frame.point, uint32(result))
		if descriptorErr != nil {
			return formalApplyLeafPublication{}, false, descriptorErr
		}
		resultSlot, slotOK := a.program.formalSlots.Slot(target.body, Root{Kind: RootResult, Index: uint32(result)})
		if !slotOK {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result %d has no normal-return slot", result)
		}
		_, concreteCallResult, carrierErr := frameCallResultCarrier(caller.keys, caller.body, frame.point, uint32(result))
		if carrierErr != nil {
			return formalApplyLeafPublication{}, false, carrierErr
		}
		formalCallResult, pathErr := a.formalizeApplyPath(caller, concreteCallResult)
		if pathErr != nil {
			return formalApplyLeafPublication{}, false, pathErr
		}
		appendDestination := func(edgeOrdinal, destination int, slot FormalSlot, write bool, path keyspace.Key) error {
			if err := setDestinationRoot(destination, state.BoundaryFactorRootTarget[FormalSlot]{Slot: slot, WriteScalar: write}, path); err != nil {
				return err
			}
			resultRelations[result].destinations = append(resultRelations[result].destinations, destination)
			return appendSource(edgeOrdinal, resultSlot, rawResultValues[result], resultValues[result])
		}
		resultDestinations := frame.boundary.results[uint32(result)]
		if len(resultDestinations) == 0 {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result %d has no frozen destinations", result)
		}
		resultEdges := frame.boundary.resultEdges[uint32(result)]
		if len(resultEdges) != len(resultDestinations) {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result edge topology differs from destinations")
		}
		if err := appendDestination(resultEdges[0], resultDestinations[0], descriptor.slot, true, formalCallResult); err != nil {
			return formalApplyLeafPublication{}, false, err
		}
		for destinationIndex := 1; destinationIndex < len(resultDestinations); destinationIndex++ {
			destination := frame.boundary.destinations[resultDestinations[destinationIndex]]
			var slot FormalSlot
			write := destination.slot != 0
			if write {
				var exact bool
				slot, exact = formalMiddleSlotForStateKey(a.program, caller, destination.slot)
				if !exact {
					return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result %d target has no Values coordinate", result)
				}
			}
			path := keyspace.Key{}
			if destination.path.Kind != keyspace.KindInvalid {
				var pathErr error
				path, pathErr = a.formalizeApplyPath(caller, destination.path)
				if pathErr != nil {
					return formalApplyLeafPublication{}, false, pathErr
				}
			}
			if err := appendDestination(resultEdges[destinationIndex], resultDestinations[destinationIndex], slot, write, path); err != nil {
				return formalApplyLeafPublication{}, false, err
			}
		}
	}
	for sourceIndex, source := range frame.resultSources {
		if int(source.result) >= len(resultRelations) {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result-source index is malformed")
		}
		resultSlot, slotOK := a.program.formalSlots.Slot(target.body, Root{Kind: RootResult, Index: source.result})
		if !slotOK {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply result source %d has no normal-return slot", source.result)
		}
		for _, edgeOrdinal := range frame.boundary.resultAliasEdges[sourceIndex] {
			if err := appendSource(edgeOrdinal, resultSlot, rawResultValues[source.result], resultValues[source.result]); err != nil {
				return formalApplyLeafPublication{}, false, err
			}
		}
	}

	schemas := make([]state.BoundaryFactorRoot, len(sourceRoots))
	rawValues := make([]product.Value, len(sourceRoots))
	imageRootValues := make([]product.Value, len(sourceRoots))
	for index, root := range sourceRoots {
		schemas[index], rawValues[index], imageRootValues[index] = root.schema, root.raw, root.value
	}
	sourceRoots, destinations, err = compactFormalApplyBoundary(sourceRoots, destinations)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	rootMap, err := formalApplyBoundaryRootMap(sourceRoots, destinations)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	phase = "boundary selection"
	selection, err := state.SealBoundaryFactorSelection(region.target.authority.coordinateKeys, schemas, nil, false)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	selection, err = capability.reachability.Close(selection, rawValues)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	selection, unreachable, err = state.ApplyBoundaryFactorSelectionIdentitySubstitution(region.target.authority.product, identityPlan, selection)
	if err != nil || unreachable {
		return formalApplyLeafPublication{}, !unreachable, err
	}
	var normalReturn *formalApplyNormalReturnSource
	if mode == formalApplyTerminalNormal {
		roots := make([]formalApplyNormalReturnRoot, len(sourceRoots))
		for index, root := range sourceRoots {
			roots[index] = formalApplyNormalReturnRoot{slot: root.slot, boundarySlot: root.boundarySlot, value: root.value}
		}
		normalReturn = &formalApplyNormalReturnSource{
			selection: selection,
			factors:   imageFactors,
			roots:     roots,
		}
	}
	phase = "boundary transport"
	transport, err := frame.allocations.BindTransport(region.caller.authority.coordinateKeys, rootMap, frame.existentials)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	var companion *state.LaneFactor
	if lane, present := region.target.authority.product.BoundaryClosureCompanion(); present {
		for index, group := range region.target.layout.nonValues {
			if group.lane == lane {
				companion = &imageFactors[index]
				break
			}
		}
		if companion == nil {
			return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: formal Apply closure companion is absent")
		}
	}
	projection, err := region.target.authority.product.ProjectBoundaryClosureCompanion(selection, companion)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	boundaryPlan, err := region.caller.authority.product.PrepareBoundaryFactorTransportPlan(transport, selection, projection)
	if err != nil {
		return formalApplyLeafPublication{}, false, fmt.Errorf("transformer: Apply boundary transport caller=%s target=%s roots=%d: %w", frame.callerBody.String(), frame.targetBody.String(), len(rootMap), err)
	}
	relation, err := state.SealBoundaryValueSlotRelation([]FormalSlot{}, []FormalSlot{}, nil)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	valueTransport, err := state.PrepareBoundaryValueFactorTransport(boundaryPlan, relation)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	destinationRoots := make([]state.BoundaryFactorRootTarget[FormalSlot], len(destinations))
	for index := range destinations {
		destinationRoots[index] = destinations[index].target
	}
	next, err := state.ApplyBoundaryFactorTuple(
		a.ctx, boundaryPlan, valueTransport,
		state.BoundaryFactorTuple[FormalSlot]{Values: callerValues, Factors: callerFactors},
		state.BoundaryFactorTuple[FormalSlot]{Values: imageValues, Factors: imageFactors},
		imageRootValues, destinationRoots,
	)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	traceFormalApplyCoordinates(a.program, step, region, callerFactors, imageFactors, next.Factors, rootMap)
	phase = "caller factorization"
	leaves, err := a.factorFormalApplyProductLeaf(region.caller, next.Values, next.Factors)
	if err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	diagnosticRole := boundaryCallDiagnosticCompose
	switch mode {
	case formalApplyTerminalNonreturning:
		diagnosticRole = boundaryCallDiagnosticCalleeCarry
	case formalApplyTerminalDefinition:
		diagnosticRole = boundaryCallDiagnosticKnown
	}
	phase = "diagnostics"
	if err := a.applyFormalApplicationDiagnostics(step, region, leaves, diagnosticRole); err != nil {
		return formalApplyLeafPublication{}, false, err
	}
	phase = "result bindings"
	callerAuthority := region.caller.authority
	for result := range resultBindings {
		// A direct target-input expression retains its Apply view until value
		// specialization so caller actual and callee constraint can be met. Its
		// frozen wire still supplies exact caller alias/path provenance. Every
		// other expression is already represented canonically by the computed
		// CallResult Values coordinate factored above; manufacturing a symbolic
		// self-read here would overwrite that result with an uninstantiated cycle.
		alternatives := resultBindings[result]
		direct := len(alternatives) != 0
		boundaryParametric := len(alternatives) != 0
		directBindings := make([]formalQualifiedBinding, 0, len(alternatives))
		for _, binding := range alternatives {
			candidate, exact := formalApplyDirectInputAlias(binding)
			direct = direct && exact
			boundaryParametric = boundaryParametric && formalApplyBoundaryParametricValue(binding)
			if exact {
				directBindings = append(directBindings, candidate)
			}
		}
		if !direct && !boundaryParametric && (result >= len(resultExact) || resultExact[result]) {
			continue
		}
		descriptor, descriptorErr := a.applyResultDescriptor(step.owner, frame.point, uint32(result))
		if descriptorErr != nil {
			return formalApplyLeafPublication{}, false, descriptorErr
		}
		ordinal, ok := region.caller.span.ordinal(descriptor)
		if !ok {
			return formalApplyLeafPublication{}, false, errFormalComponentMalformed
		}
		leaf := decisionLeaf(0)
		publishBindings := directBindings
		if !direct {
			publishBindings = alternatives
		}
		for _, binding := range publishBindings {
			candidate, internErr := callerAuthority.internBinding(binding)
			if internErr != nil {
				return formalApplyLeafPublication{}, false, internErr
			}
			if leaf == 0 {
				leaf = candidate
			} else {
				leaf, internErr = callerAuthority.combine(a.ctx, formalComponentJoin, leaf, candidate)
				if internErr != nil {
					return formalApplyLeafPublication{}, false, internErr
				}
			}
		}
		leaves[ordinal] = leaf
	}
	return formalApplyLeafPublication{guard: region.guard, leaves: leaves, normalReturn: normalReturn}, true, nil
}
