package target

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func canonicalizeBindings(drafts []operationDraft) error {
	type owner struct {
		binding BindingSpec
		draft   int
	}
	all := make([]owner, 0)
	for index := range drafts {
		for _, binding := range drafts[index].bindings {
			all = append(all, owner{binding: binding, draft: index})
		}
	}
	sort.Slice(all, func(left, right int) bool { return compareBinding(all[left].binding, all[right].binding) < 0 })
	for index := 1; index < len(all); index++ {
		if compareBinding(all[index-1].binding, all[index].binding) == 0 {
			return errors.New("target: binding belongs to multiple operations")
		}
	}
	return nil
}

type producedEdge struct {
	target int
	step   producedAnchorStep
}

// deriveProducedAnchors gives every produced-only operation one finite,
// source-order-independent anchor. It retains only parent edges and one step;
// a sorted iterative forest walk assigns lexicographic path order without
// flattening paths or recursing. The temporary adjacency is discarded before
// the Contract is returned.
func deriveProducedAnchors(drafts []operationDraft) error {
	edges := make([][]producedEdge, len(drafts))
	incoming := make([]int, len(drafts))
	for producer := range drafts {
		for outcome := range drafts[producer].outcomes {
			for _, produced := range drafts[producer].outcomes[outcome].produced {
				if produced.targetSource == 0 || uint64(produced.targetSource) > uint64(len(drafts)) {
					return errors.New("target: produced operation references unknown operation")
				}
				target := int(produced.targetSource - 1)
				if len(drafts[target].bindings) == 0 {
					incoming[target]++
				}
				step := anchorStep(drafts[producer].outcomes[outcome], produced.result)
				edges[producer] = append(edges[producer], producedEdge{target: target, step: step})
			}
		}
	}
	roots := make([]int, 0, len(drafts))
	resolved := make([]bool, len(drafts))
	for index := range drafts {
		if len(drafts[index].bindings) != 0 {
			roots = append(roots, index)
			continue
		}
		if incoming[index] != 1 {
			return errors.New("target: produced-only operation requires exactly one incoming produced anchor")
		}
	}
	sort.Slice(roots, func(left, right int) bool {
		return compareBinding(drafts[roots[left]].bindings[0], drafts[roots[right]].bindings[0]) < 0
	})
	for producer := range edges {
		children := edges[producer][:0]
		for _, edge := range edges[producer] {
			if len(drafts[edge.target].bindings) == 0 {
				children = append(children, edge)
			}
		}
		edges[producer] = children
		sort.Slice(children, func(left, right int) bool { return compareProducedEdge(children[left], children[right]) < 0 })
		for index := 1; index < len(children); index++ {
			if compareProducedEdge(children[index-1], children[index]) == 0 {
				return errors.New("target: duplicate produced anchor step")
			}
		}
	}
	// Bound operations are the stable source catalogue prefix. Produced-only
	// operations follow it in their root/path order, so adding a callable does
	// not churn an unrelated source binding handle.
	next := uint32(0)
	for _, root := range roots {
		next++
		drafts[root].canonical = next
		resolved[root] = true
	}
	for _, root := range roots {
		children := edges[root]
		stack := make([]int, 0, len(children))
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, children[index].target)
		}
		for len(stack) > 0 {
			last := len(stack) - 1
			current := stack[last]
			stack = stack[:last]
			if resolved[current] {
				return errors.New("target: produced anchor cycle")
			}
			resolved[current] = true
			next++
			drafts[current].canonical = next
			children := edges[current]
			for index := len(children) - 1; index >= 0; index-- {
				stack = append(stack, children[index].target)
			}
		}
	}
	for index := range drafts {
		if !resolved[index] {
			return errors.New("target: produced-only operation is unreachable or cyclic")
		}
	}
	return nil
}

func anchorStep(outcome outcomeDraft, result uint32) producedAnchorStep {
	return producedAnchorStep{outcome: outcome.anchor, result: result}
}

func compareOperationDraft(left, right operationDraft) int {
	if left.canonical < right.canonical {
		return -1
	}
	if left.canonical > right.canonical {
		return 1
	}
	return 0
}

func compareProducedEdge(left, right producedEdge) int {
	if left.step.outcome != right.step.outcome {
		if left.step.outcome < right.step.outcome {
			return -1
		}
		return 1
	}
	if left.step.result < right.step.result {
		return -1
	}
	if left.step.result > right.step.result {
		return 1
	}
	return 0
}

func (d *operationDraft) resolveEffects(all []operationDraft, sourceOperation []Operation) error {
	if err := d.resolveEffectList(d.effects, all, sourceOperation, "effect"); err != nil {
		return err
	}
	for index := range d.callbacks {
		if err := d.resolveEffectList(d.callbacks[index].effects.effects, all, sourceOperation, "callback effect"); err != nil {
			return fmt.Errorf("target: callback %d: %w", index, err)
		}
	}
	return nil
}

func (d *operationDraft) resolveEffectList(effects []effectDraft, all []operationDraft, sourceOperation []Operation, label string) error {
	for index := range effects {
		targetRef := effects[index].targetSource
		if targetRef == 0 || uint64(targetRef) > uint64(len(sourceOperation)) {
			return fmt.Errorf("target: %s %d references unknown operation", label, index)
		}
		targetOp := sourceOperation[uint32(targetRef)-1]
		if targetOp == 0 {
			return fmt.Errorf("target: %s %d references unresolved operation", label, index)
		}
		target := &all[uint32(targetOp)-1]
		if len(effects[index].values) != target.valueFormalCount() ||
			len(effects[index].types) != len(target.formals) ||
			uint64(len(effects[index].valuesVar)) != uint64(target.valuesVars) ||
			uint64(len(effects[index].rows)) != uint64(target.rowFormals) {
			return fmt.Errorf("target: %s %d does not match target ABI", label, index)
		}
		effects[index].target = targetOp
		if effects[index].hasPublication {
			descriptor := effects[index].publication
			if !descriptor.validConsequences() {
				return fmt.Errorf("target: %s %d has invalid publication consequences", label, index)
			}
			if uint64(descriptor.subject) >= uint64(target.valueFormalCount()) {
				return fmt.Errorf("target: %s %d publication subject outside effect target ABI", label, index)
			}
			if descriptor.destination == PublicationDestinationValueFormal && uint64(descriptor.context) >= uint64(target.valueFormalCount()) {
				return fmt.Errorf("target: %s %d publication destination outside effect target ABI", label, index)
			}
		}
	}
	sort.Slice(effects, func(left, right int) bool { return compareEffect(effects[left], effects[right]) < 0 })
	return nil
}

func (d *operationDraft) resolveCallbackReleases(all []operationDraft, sourceOperation []Operation) error {
	for index := range d.callbacks {
		release := d.callbacks[index].release
		if release == nil {
			continue
		}
		if release.operationSource == 0 || uint64(release.operationSource) > uint64(len(sourceOperation)) {
			return fmt.Errorf("target: callback %d release references unknown operation", index)
		}
		targetOp := sourceOperation[uint32(release.operationSource)-1]
		if targetOp == 0 {
			return fmt.Errorf("target: callback %d release references unresolved operation", index)
		}
		target := &all[uint32(targetOp)-1]
		if len(target.bindings) == 0 {
			return fmt.Errorf("target: callback %d release operation is not source-visible", index)
		}
		if uint64(release.input) >= uint64(target.valueFormalCount()) {
			return fmt.Errorf("target: callback %d release input outside operation scope", index)
		}
		canonical, found := canonicalOutcomeForSource(target.outcomes, release.outcome)
		if !found {
			return fmt.Errorf("target: callback %d release outcome outside operation scope", index)
		}
		release.operation, release.outcome = targetOp, canonical
		switch release.zeroBehavior {
		case CallbackReleaseZeroSuppress:
			// No outcome coordinate is retained for a suppressed zero-holder
			// release. The frozen form stores only the behavior tag.
			release.zeroOutcome = 0
		case CallbackReleaseZeroThrow, CallbackReleaseZeroIdempotent:
			zeroOutcome, zeroFound := canonicalOutcomeForSource(target.outcomes, release.zeroOutcome)
			if !zeroFound {
				return fmt.Errorf("target: callback %d zero release outcome outside operation scope", index)
			}
			kind := target.outcomes[zeroOutcome].kind
			if release.zeroBehavior == CallbackReleaseZeroThrow && kind != flowkind.OutcomeThrow {
				return fmt.Errorf("target: callback %d zero release throw outcome is not Throw", index)
			}
			if release.zeroBehavior == CallbackReleaseZeroIdempotent && kind != flowkind.OutcomeNormal {
				return fmt.Errorf("target: callback %d zero release idempotent outcome is not Normal", index)
			}
			release.zeroOutcome = zeroOutcome
		default:
			return fmt.Errorf("target: callback %d release has invalid zero behavior", index)
		}
	}
	return nil
}

func canonicalOutcomeForSource(outcomes []outcomeDraft, source uint32) (uint32, bool) {
	for index := range outcomes {
		if uint32(outcomes[index].source) == source {
			return uint32(index), true
		}
	}
	return 0, false
}

func (d *operationDraft) resolveProduced(sourceOperation []Operation) error {
	for outcomeIndex := range d.outcomes {
		for producedIndex := range d.outcomes[outcomeIndex].produced {
			produced := &d.outcomes[outcomeIndex].produced[producedIndex]
			if produced.targetSource == 0 || uint64(produced.targetSource) > uint64(len(sourceOperation)) {
				return fmt.Errorf("target: produced operation %d references unknown operation", producedIndex)
			}
			produced.target = sourceOperation[uint32(produced.targetSource)-1]
			if produced.target == 0 {
				return fmt.Errorf("target: produced operation %d references unresolved operation", producedIndex)
			}
		}
	}
	return nil
}

func (d *operationDraft) resolveSuspensions() error {
	canonical := make([]uint32, len(d.outcomes))
	for index, outcome := range d.outcomes {
		canonical[outcome.source] = uint32(index)
	}
	if len(d.suspensions) != 0 {
		for index := range d.suspensions {
			row := &d.suspensions[index]
			row.yield = canonical[row.yield]
			row.reentry = canonical[row.reentry]
		}
		sort.Slice(d.suspensions, func(left, right int) bool {
			return compareSuspension(d.suspensions[left], d.suspensions[right]) < 0
		})
		for index := 1; index < len(d.suspensions); index++ {
			if compareSuspensionKey(d.suspensions[index-1], d.suspensions[index]) == 0 {
				return errors.New("target: duplicate suspension")
			}
		}
	}
	if err := d.resolveSpawns(canonical); err != nil {
		return err
	}
	for index := range d.resumes {
		for outcome := range d.resumes[index].outcomes {
			d.resumes[index].outcomes[outcome] = canonical[d.resumes[index].outcomes[outcome]]
		}
	}
	sort.Slice(d.resumes, func(left, right int) bool {
		return compareResume(d.resumes[left], d.resumes[right]) < 0
	})
	for index := 1; index < len(d.resumes); index++ {
		if compareResume(d.resumes[index-1], d.resumes[index]) == 0 {
			return errors.New("target: duplicate resume")
		}
	}
	return nil
}

func (d *operationDraft) resolveSpawns(canonical []uint32) error {
	for index := range d.spawns {
		spawn := &d.spawns[index]
		spawn.yield = canonical[spawn.yield]
		spawn.parentResume = canonical[spawn.parentResume]
		spawn.childEntry = canonical[spawn.childEntry]
		matched := false
		for _, suspension := range d.suspensions {
			if suspension.yield == spawn.yield && suspension.reentry == spawn.parentResume && suspension.source == ReentryByProvider && suspension.multiplicity == ReentryOnce {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("target: spawn %d lacks exact one-shot provider suspension", index)
		}
	}
	return nil
}

func validateProducedResumes(drafts []operationDraft, sourceOperation []Operation) error {
	callbackCapture := make([]bool, len(drafts)+1)
	for producer := range drafts {
		for outcome := range drafts[producer].outcomes {
			for _, produced := range drafts[producer].outcomes[outcome].produced {
				if produced.target == 0 || uint64(produced.target) >= uint64(len(callbackCapture)) {
					return errors.New("target: produced resume has unresolved target")
				}
				for _, capture := range produced.captures {
					if capture.Kind == CaptureCallback {
						callbackCapture[produced.target] = true
						break
					}
				}
			}
		}
	}
	for index := range drafts {
		draft := &drafts[index]
		needsProduced := false
		for _, resume := range draft.resumes {
			needsProduced = needsProduced || resume.source == ResumeSourceProduced
		}
		if !needsProduced {
			continue
		}
		if len(draft.bindings) != 0 {
			return errors.New("target: produced resume has source binding")
		}
		op := sourceOperation[draft.source]
		if op == 0 {
			return errors.New("target: produced resume has unresolved operation")
		}
		if uint64(op) >= uint64(len(callbackCapture)) || !callbackCapture[op] {
			return errors.New("target: produced resume lacks callback capture")
		}
	}
	return nil
}

// validateSpawnAuthority keeps detached spawning an explicitly selected
// contract authority.  A profile may omit it, but a sealed contract can never
// admit two independently schedulable spawn rows.
func validateSpawnAuthority(drafts []operationDraft) error {
	count := 0
	for index := range drafts {
		count += len(drafts[index].spawns)
	}
	if count > 1 {
		return errors.New("target: duplicate spawn authority")
	}
	return nil
}

func compareSuspension(left, right suspensionDraft) int {
	if order := compareSuspensionKey(left, right); order != 0 {
		return order
	}
	if left.multiplicity < right.multiplicity {
		return -1
	}
	if left.multiplicity > right.multiplicity {
		return 1
	}
	return 0
}

func compareSuspensionKey(left, right suspensionDraft) int {
	if left.yield != right.yield {
		if left.yield < right.yield {
			return -1
		}
		return 1
	}
	if left.reentry != right.reentry {
		if left.reentry < right.reentry {
			return -1
		}
		return 1
	}
	if left.source != right.source {
		if left.source < right.source {
			return -1
		}
		return 1
	}
	return 0
}

func compareResume(left, right resumeDraft) int {
	if left.source != right.source {
		if left.source < right.source {
			return -1
		}
		return 1
	}
	if left.carrier < right.carrier {
		return -1
	}
	if left.carrier > right.carrier {
		return 1
	}
	if compared := compareValues(left.arguments, right.arguments); compared != 0 {
		return compared
	}
	return 0
}

// crossActivationOutcomeIndex maps the only outcomes that can leave an
// activation boundary to compact canonical row coordinates.
func crossActivationOutcomeIndex(kind flowkind.OutcomeKind) (int, bool) {
	switch kind {
	case flowkind.OutcomeNormal:
		return 0, true
	case flowkind.OutcomeReturn:
		return 1, true
	case flowkind.OutcomeThrow:
		return 2, true
	case flowkind.OutcomeYield:
		return 3, true
	case flowkind.OutcomeCancel:
		return 4, true
	default:
		return 0, false
	}
}

func validAuthoredInputSource(source InputSource, valueFormals int, valuesVars uint32) bool {
	switch source.Kind {
	case InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(valueFormals)
	case InputSourceValuesVar:
		return uint64(source.Ordinal) < uint64(valuesVars)
	default:
		return false
	}
}

func compareInputSource(left, right InputSource) int {
	if left.Kind < right.Kind {
		return -1
	}
	if left.Kind > right.Kind {
		return 1
	}
	if left.Ordinal < right.Ordinal {
		return -1
	}
	if left.Ordinal > right.Ordinal {
		return 1
	}
	return 0
}

func validTransferPossibility(possibility TransferPossibility) bool {
	const valid = TransferMayDeliver | TransferMayReject
	return possibility != 0 && possibility&^valid == 0
}

func validTransferEndpoint(endpoint TransferEndpoint, valueFormals int) bool {
	switch endpoint.Kind {
	case TransferEndpointInput:
		return uint64(endpoint.Input) < uint64(valueFormals)
	case TransferEndpointExternal:
		return endpoint.Input == 0
	default:
		return false
	}
}

func validTransferIdentity(identity TransferIdentity) bool {
	return identity >= TransferIdentityUnspecified && identity <= TransferIdentityDistinct
}

func validTransferCapabilities(capabilities TransferCapabilities) bool {
	return capabilities >= TransferCapabilitiesUnspecified && capabilities <= TransferCapabilitiesLoseAll
}

// validTransferInputSource admits only exact invocation inputs.  A ValuesVar
// is a transfer source only when it is the operation input tail; result,
// callback, and local Values variables have no caller-owned source Pack.
// AllInputs is reserved for the synthesized opaque operation.
func validTransferInputSource(source InputSource, d operationDraft) bool {
	switch source.Kind {
	case InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(d.valueFormalCount())
	case InputSourceValuesVar:
		return d.input.tail == ValuesVariable && source.Ordinal == uint32(d.input.varID)
	default:
		return false
	}
}

func compareTransfer(left, right transferDraft) int {
	if compared := compareTransferIdentity(left, right); compared != 0 {
		return compared
	}
	if left.identity < right.identity {
		return -1
	}
	if left.identity > right.identity {
		return 1
	}
	if left.capabilities < right.capabilities {
		return -1
	}
	if left.capabilities > right.capabilities {
		return 1
	}
	return 0
}

func compareTransferIdentity(left, right transferDraft) int {
	if left.endpoint.Kind < right.endpoint.Kind {
		return -1
	}
	if left.endpoint.Kind > right.endpoint.Kind {
		return 1
	}
	if left.endpoint.Input < right.endpoint.Input {
		return -1
	}
	if left.endpoint.Input > right.endpoint.Input {
		return 1
	}
	if compared := compareInputSource(left.payload, right.payload); compared != 0 {
		return compared
	}
	return compareInputSource(left.alias, right.alias)
}

func validCallbackLifecycle(lifecycle CallbackLifecycle) bool {
	return lifecycle >= CallbackSyncOptionalOnce && lifecycle <= CallbackRetainedRequiredMany
}

func retainedCallbackLifecycle(lifecycle CallbackLifecycle) bool {
	return lifecycle >= CallbackRetainedOptionalOnce && lifecycle <= CallbackRetainedRequiredMany
}

func onceCallbackLifecycle(lifecycle CallbackLifecycle) bool {
	switch lifecycle {
	case CallbackSyncOptionalOnce, CallbackSyncRequiredOnce,
		CallbackRetainedOptionalOnce, CallbackRetainedRequiredOnce:
		return true
	default:
		return false
	}
}

func validCallbackReleaseMode(mode CallbackReleaseMode) bool {
	return mode == CallbackReleaseOne || mode == CallbackReleaseAll
}

func validCallbackReleaseZeroBehavior(behavior CallbackReleaseZeroBehavior) bool {
	switch behavior {
	case CallbackReleaseZeroSuppress, CallbackReleaseZeroThrow, CallbackReleaseZeroIdempotent:
		return true
	default:
		return false
	}
}

func validAdmission(admission Admission) bool {
	return admission == DirectFunction || admission == OrdinaryCallable
}

func (d operationDraft) valueFormalCount() int {
	return len(d.input.types)
}

func compareCallback(left, right callbackDraft) int {
	if compared := compareCallbackIdentity(left, right); compared != 0 {
		return compared
	}
	if left.admission < right.admission {
		return -1
	}
	if left.admission > right.admission {
		return 1
	}
	if left.lifecycle < right.lifecycle {
		return -1
	}
	if left.lifecycle > right.lifecycle {
		return 1
	}
	return 0
}

func compareCallbackIdentity(left, right callbackDraft) int {
	if left.function.Kind < right.function.Kind {
		return -1
	}
	if left.function.Kind > right.function.Kind {
		return 1
	}
	if left.function.Ordinal < right.function.Ordinal {
		return -1
	}
	if left.function.Ordinal > right.function.Ordinal {
		return 1
	}
	if compared := compareValues(left.arguments, right.arguments); compared != 0 {
		return compared
	}
	for index := range left.outcomes {
		if compared := compareValues(left.outcomes[index], right.outcomes[index]); compared != 0 {
			return compared
		}
	}
	return 0
}

func compareCallbackRelease(left, right callbackReleaseRow) int {
	if left.callback < right.callback {
		return -1
	}
	if left.callback > right.callback {
		return 1
	}
	if left.input < right.input {
		return -1
	}
	if left.input > right.input {
		return 1
	}
	if left.outcome < right.outcome {
		return -1
	}
	if left.outcome > right.outcome {
		return 1
	}
	if left.mode < right.mode {
		return -1
	}
	if left.mode > right.mode {
		return 1
	}
	if left.zeroBehavior < right.zeroBehavior {
		return -1
	}
	if left.zeroBehavior > right.zeroBehavior {
		return 1
	}
	if left.zeroOutcome < right.zeroOutcome {
		return -1
	}
	if left.zeroOutcome > right.zeroOutcome {
		return 1
	}
	return 0
}

func (c *Contract) appendOperation(op Operation, draft *operationDraft, keys map[keyspace.LiteralValue]ExactKey) error {
	expected, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	if op != Operation(expected) {
		return errors.New("target: noncanonical operation handle")
	}
	typeHandle, err := c.appendTypes(op, draft.types)
	if err != nil {
		return err
	}
	bindings, err := c.appendBindings(draft.bindings, keys)
	if err != nil {
		return err
	}
	row := operationRow{
		bindings:   bindings,
		valuesVars: draft.valuesVars,
		rowFormals: draft.rowFormals,
		effectTail: draft.effectTail,
		effectVar:  draft.effectVar,
	}
	formalRange, err := checkedStoredRange("type formal pool", len(c.formals), len(draft.constraints))
	if err != nil {
		return err
	}
	for _, constraint := range draft.constraints {
		c.formals = append(c.formals, typeHandle[constraint])
	}
	row.typeFormals = formalRange
	valuesTypes, valuesTypeErr := checkedStoredRange("Values variable type pool", len(c.valuesVarTypes), len(draft.valuesTypes))
	if valuesTypeErr != nil {
		return valuesTypeErr
	}
	for _, key := range draft.valuesTypes {
		handle, found := typeHandle[key]
		if !found || handle == 0 {
			return errors.New("target: unresolved Values variable type")
		}
		c.valuesVarTypes = append(c.valuesVarTypes, handle)
	}
	row.valuesTypes = valuesTypes

	valuesHandle := make(map[string]Values)
	allValues := make(map[string]valuesDraft)
	addValues := func(values valuesDraft) error {
		key, keyErr := values.key()
		if keyErr != nil {
			return keyErr
		}
		allValues[key] = values
		return nil
	}
	inputKey, err := draft.input.key()
	if err != nil {
		return err
	}
	if err := addValues(draft.input); err != nil {
		return err
	}
	outcomeValues := make([]Values, 0, len(draft.outcomes))
	for _, outcome := range draft.outcomes {
		if err := addValues(outcome.values); err != nil {
			return err
		}
	}
	for _, callback := range draft.callbacks {
		if err := addValues(callback.arguments); err != nil {
			return err
		}
		for _, terminal := range callback.outcomes {
			if err := addValues(terminal); err != nil {
				return err
			}
		}
	}
	for _, subedge := range draft.subedges {
		if err := visitSubedgeValues(subedge, addValues); err != nil {
			return err
		}
	}
	for _, resume := range draft.resumes {
		if err := addValues(resume.arguments); err != nil {
			return err
		}
	}
	valueKeys := make([]string, 0, len(allValues))
	for key := range allValues {
		valueKeys = append(valueKeys, key)
	}
	sort.Strings(valueKeys)
	for _, key := range valueKeys {
		handle, appendErr := c.appendValues(op, allValues[key], typeHandle)
		if appendErr != nil {
			return appendErr
		}
		valuesHandle[key] = handle
	}
	row.input = valuesHandle[inputKey]
	callbackIDs, callbackRange, err := c.appendCallbacks(op, draft.callbacks, valuesHandle)
	if err != nil {
		return err
	}
	row.callbacks = callbackRange
	suspensions, suspensionErr := c.appendSuspensions(draft.suspensions)
	if suspensionErr != nil {
		return suspensionErr
	}
	row.suspensions = suspensions
	resumes, resumeErr := c.appendResumes(op, draft.resumes, valuesHandle)
	if resumeErr != nil {
		return resumeErr
	}
	row.resumes = resumes
	outcomeRange, err := checkedStoredRange("outcome table", len(c.outcomes), len(draft.outcomes))
	if err != nil {
		return err
	}
	for _, outcome := range draft.outcomes {
		key, keyErr := outcome.values.key()
		if keyErr != nil {
			return keyErr
		}
		produced, producedErr := c.appendProduced(outcome.produced, callbackIDs)
		if producedErr != nil {
			return producedErr
		}
		fresh, freshErr := c.appendFreshResults(outcome.fresh)
		if freshErr != nil {
			return freshErr
		}
		callbackResults, callbackResultErr := c.appendCallbackResults(outcome.callbackResults, callbackIDs)
		if callbackResultErr != nil {
			return callbackResultErr
		}
		resultAliases, resultAliasErr := c.appendResultAliases(outcome.resultAliases)
		if resultAliasErr != nil {
			return resultAliasErr
		}
		c.outcomes = append(c.outcomes, outcomeRow{
			kind: outcome.kind, values: valuesHandle[key], produced: produced, fresh: fresh,
			callbackResults: callbackResults, resultAliases: resultAliases,
		})
		outcomeValues = append(outcomeValues, valuesHandle[key])
	}
	row.outcomes = outcomeRange
	subedgeRange, subedgeErr := c.appendSubedges(op, draft.subedges, callbackIDs, valuesHandle, keys)
	if subedgeErr != nil {
		return subedgeErr
	}
	row.subedges = subedgeRange
	spawns, spawnErr := c.appendSpawns(op, draft.spawns, callbackIDs, outcomeValues)
	if spawnErr != nil {
		return spawnErr
	}
	row.spawns = spawns
	transfers, transferErr := c.appendTransfers(op, draft.transfers)
	if transferErr != nil {
		return transferErr
	}
	row.transfers = transfers
	effectRange, err := c.appendEffects(draft.effects)
	if err != nil {
		return err
	}
	row.effects = effectRange
	if draft.gsubTable != nil {
		branch, branchErr := c.appendGsubTableReplacement(op, *draft.gsubTable, row)
		if branchErr != nil {
			return branchErr
		}
		row.gsubTable = branch
	}
	c.operations = append(c.operations, row)
	if len(draft.bindings) != 0 {
		c.boundCount++
	}
	return nil
}

func (c *Contract) appendTypes(owner Operation, input map[string][]byte) (map[string]Type, error) {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, err := checkedStoredRange("type table", len(c.types), len(keys)); err != nil {
		return nil, err
	}
	handles := make(map[string]Type, len(keys))
	for _, key := range keys {
		if _, err := checkedStoredLength("type bytes", len(input[key])); err != nil {
			return nil, err
		}
		handle, err := checkedStoredHandle("type table", len(c.types))
		if err != nil {
			return nil, err
		}
		c.types = append(c.types, typeRow{owner: owner, bytes: append([]byte(nil), input[key]...)})
		handles[key] = Type(handle)
	}
	return handles, nil
}

func (c *Contract) appendValues(owner Operation, input valuesDraft, handles map[string]Type) (Values, error) {
	handle, err := checkedStoredHandle("Values table", len(c.values))
	if err != nil {
		return 0, err
	}
	typeRange, err := checkedStoredRange("Values type pool", len(c.valueTypes), len(input.types))
	if err != nil {
		return 0, err
	}
	suffixRange, err := checkedStoredRange("Values suffix type pool", len(c.valueTypes)+len(input.types), len(input.suffix))
	if err != nil {
		return 0, err
	}
	row := valuesRow{owner: owner, tail: input.tail, varID: input.varID}
	row.types = typeRange
	row.suffix = suffixRange
	for _, key := range input.types {
		c.valueTypes = append(c.valueTypes, handles[key])
	}
	for _, key := range input.suffix {
		c.valueTypes = append(c.valueTypes, handles[key])
	}
	c.values = append(c.values, row)
	return Values(handle), nil
}

func (c *Contract) appendBindings(input []BindingSpec, keys map[keyspace.LiteralValue]ExactKey) (indexRange, error) {
	output, err := checkedStoredRange("binding table", len(c.bindings), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, binding := range input {
		row, appendErr := c.appendBinding(binding, keys)
		if appendErr != nil {
			return indexRange{}, appendErr
		}
		c.bindings = append(c.bindings, row)
	}
	return output, nil
}

func (c *Contract) appendBinding(input BindingSpec, keys map[keyspace.LiteralValue]ExactKey) (bindingRange, error) {
	owner, err := checkedStoredRange("binding segment pool", len(c.segments), len(input.Owner))
	if err != nil {
		return bindingRange{}, err
	}
	member, err := checkedStoredRange("binding segment pool", int(owner.end), len(input.Member))
	if err != nil {
		return bindingRange{}, err
	}
	ownerKeys, err := checkedStoredRange("binding exact-key pool", len(c.bindingKeys), len(input.Owner))
	if err != nil {
		return bindingRange{}, err
	}
	memberKeys, err := checkedStoredRange("binding exact-key pool", int(ownerKeys.end), len(input.Member))
	if err != nil {
		return bindingRange{}, err
	}
	row := bindingRange{namespace: input.Namespace}
	row.owner, row.member, row.ownerKeys, row.memberKeys = owner, member, ownerKeys, memberKeys
	c.segments = append(c.segments, input.Owner...)
	c.segments = append(c.segments, input.Member...)
	for _, segment := range input.Owner {
		key, keyErr := exactKeyHandle(keys, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
		if keyErr != nil {
			return bindingRange{}, keyErr
		}
		c.bindingKeys = append(c.bindingKeys, key)
	}
	for _, segment := range input.Member {
		key, keyErr := exactKeyHandle(keys, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment})
		if keyErr != nil {
			return bindingRange{}, keyErr
		}
		c.bindingKeys = append(c.bindingKeys, key)
	}
	return row, nil
}

func (c *Contract) appendCallbacks(owner Operation, input []callbackDraft, values map[string]Values) ([]CallbackID, indexRange, error) {
	rangeOut, err := checkedStoredRange("callback table", len(c.callbacks), len(input))
	if err != nil {
		return nil, indexRange{}, err
	}
	ids := make([]CallbackID, len(input))
	for index := range input {
		callback := &input[index]
		handle, handleErr := checkedStoredHandle("callback table", len(c.callbacks))
		if handleErr != nil {
			return nil, indexRange{}, handleErr
		}
		effects, effectErr := c.appendEffects(callback.effects.effects)
		if effectErr != nil {
			return nil, indexRange{}, effectErr
		}
		id := CallbackID(handle)
		ids[callback.source] = id
		callback.sealed = id
		arguments, valuesErr := lookupDraftValues(values, callback.arguments)
		if valuesErr != nil {
			return nil, indexRange{}, valuesErr
		}
		var outcomes [5]Values
		for terminal := range callback.outcomes {
			value, terminalErr := lookupDraftValues(values, callback.outcomes[terminal])
			if terminalErr != nil {
				return nil, indexRange{}, terminalErr
			}
			outcomes[terminal] = value
		}
		c.callbacks = append(c.callbacks, callbackRow{
			owner: owner, function: callback.function, admission: callback.admission,
			arguments: arguments, outcomes: outcomes, lifecycle: callback.lifecycle,
			effects: effects, effectTail: callback.effects.tail, effectVar: callback.effects.variable,
		})
	}
	return ids, rangeOut, nil
}

func lookupDraftValues(values map[string]Values, draft valuesDraft) (Values, error) {
	key, err := draft.key()
	if err != nil {
		return 0, err
	}
	handle, ok := values[key]
	if !ok || handle == 0 {
		return 0, errors.New("target: unresolved Values endpoint")
	}
	return handle, nil
}

func (c *Contract) appendSuspensions(input []suspensionDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("suspension table", len(c.suspensions), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, suspension := range input {
		c.suspensions = append(c.suspensions, suspensionRow{
			yield: suspension.yield, reentry: suspension.reentry,
			source: suspension.source, multiplicity: suspension.multiplicity,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendSpawns(owner Operation, input []spawnDraft, callbacks []CallbackID, outcomes []Values) (indexRange, error) {
	rangeOut, err := checkedStoredRange("spawn table", len(c.spawns), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, spawn := range input {
		if spawn.child == 0 || int(spawn.child) > len(callbacks) || int(spawn.childEntry) >= len(outcomes) || int(spawn.parentResume) >= len(outcomes) {
			return indexRange{}, errors.New("target: unresolved spawn")
		}
		c.spawns = append(c.spawns, spawnRow{
			owner: owner, function: spawn.function, child: callbacks[spawn.child-1],
			yield: spawn.yield, parentResume: spawn.parentResume,
			childEntry: outcomes[spawn.childEntry], resumeValues: outcomes[spawn.parentResume],
			alternatives: spawn.alternatives,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendResumes(owner Operation, input []resumeDraft, values map[string]Values) (indexRange, error) {
	rangeOut, err := checkedStoredRange("resume table", len(c.resumes), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, resume := range input {
		arguments, valuesErr := lookupDraftValues(values, resume.arguments)
		if valuesErr != nil {
			return indexRange{}, valuesErr
		}
		c.resumes = append(c.resumes, resumeRow{owner: owner, source: resume.source, carrier: resume.carrier, arguments: arguments, outcomes: resume.outcomes})
	}
	return rangeOut, nil
}

func (c *Contract) appendTransfers(owner Operation, input []transferDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("transfer table", len(c.transfers), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, transfer := range input {
		outcomes, outcomeErr := appendStoredRange(
			&c.transferOutcomes, transfer.outcomes, "transfer outcome table",
		)
		if outcomeErr != nil {
			return indexRange{}, outcomeErr
		}
		c.transfers = append(c.transfers, transferRow{
			owner: owner, endpoint: transfer.endpoint, payload: transfer.payload, alias: transfer.alias, identity: transfer.identity,
			capabilities: transfer.capabilities, outcomes: outcomes,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendEffects(input []effectDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("effect table", len(c.effects), len(input))
	if err != nil {
		return indexRange{}, err
	}
	valueArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.values) }, "effect value argument pool")
	if err != nil {
		return indexRange{}, err
	}
	typeArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.types) }, "effect type argument pool")
	if err != nil {
		return indexRange{}, err
	}
	valuesArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.valuesVar) }, "effect Values argument pool")
	if err != nil {
		return indexRange{}, err
	}
	rowArgs, err := totalEffectArguments(input, func(effect effectDraft) int { return len(effect.rows) }, "effect row argument pool")
	if err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect value argument pool", len(c.effectVals), valueArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect type argument pool", len(c.effectType), typeArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect Values argument pool", len(c.effectVars), valuesArgs); err != nil {
		return indexRange{}, err
	}
	if _, err := checkedStoredRange("effect row argument pool", len(c.effectRows), rowArgs); err != nil {
		return indexRange{}, err
	}
	for _, effect := range input {
		row := effectRow{target: effect.target, publication: effect.publication, hasPublication: effect.hasPublication}
		if row.values, err = appendStoredRange(&c.effectVals, effect.values, "effect value argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.types, err = appendStoredRange(&c.effectType, effect.types, "effect type argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.valuesVar, err = appendStoredRange(&c.effectVars, effect.valuesVar, "effect Values argument pool"); err != nil {
			return indexRange{}, err
		}
		if row.rows, err = appendStoredRange(&c.effectRows, effect.rows, "effect row argument pool"); err != nil {
			return indexRange{}, err
		}
		c.effects = append(c.effects, row)
	}
	return rangeOut, nil
}

func (c *Contract) appendCallbackReleases(drafts []operationDraft) error {
	pending := make([][]callbackReleaseRow, len(c.operations))
	for draftIndex := range drafts {
		for callbackIndex := range drafts[draftIndex].callbacks {
			callback := drafts[draftIndex].callbacks[callbackIndex]
			if callback.release == nil {
				continue
			}
			if callback.sealed == 0 || callback.release.operation == 0 ||
				uint64(callback.release.operation) > uint64(len(pending)) {
				return errors.New("target: unresolved callback release")
			}
			pending[uint32(callback.release.operation)-1] = append(
				pending[uint32(callback.release.operation)-1],
				callbackReleaseRow{
					callback: callback.sealed, operation: callback.release.operation,
					input: callback.release.input, outcome: callback.release.outcome,
					mode: callback.release.mode, zeroBehavior: callback.release.zeroBehavior,
					zeroOutcome: callback.release.zeroOutcome,
				},
			)
		}
	}
	for operation := range pending {
		releases := pending[operation]
		if len(releases) == 0 {
			continue
		}
		sort.Slice(releases, func(left, right int) bool {
			return compareCallbackRelease(releases[left], releases[right]) < 0
		})
		for index := 1; index < len(releases); index++ {
			if releases[index-1].callback == releases[index].callback {
				return errors.New("target: callback has duplicate release")
			}
		}
		rangeOut, err := checkedStoredRange("callback release table", len(c.callbackReleases), len(releases))
		if err != nil {
			return err
		}
		c.operations[operation].releases = rangeOut
		for _, release := range releases {
			handle, handleErr := checkedStoredHandle("callback release table", len(c.callbackReleases))
			if handleErr != nil {
				return handleErr
			}
			c.callbackReleases = append(c.callbackReleases, release)
			c.callbacks[uint32(release.callback)-1].release = handle
		}
	}
	return nil
}

func (c *Contract) appendProduced(input []producedDraft, callbacks []CallbackID) (indexRange, error) {
	rangeOut, err := checkedStoredRange("produced operation table", len(c.produced), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, produced := range input {
		captures, captureErr := checkedStoredRange("produced capture table", len(c.captures), len(produced.captures))
		if captureErr != nil {
			return indexRange{}, captureErr
		}
		for _, capture := range produced.captures {
			ordinal := capture.Ordinal
			if capture.Kind == CaptureCallback {
				ordinal = uint32(callbacks[capture.Ordinal-1])
			}
			c.captures = append(c.captures, captureRow{kind: capture.Kind, ordinal: ordinal})
		}
		typeValueCapture := noTypeValueCapture
		for captureIndex, capture := range produced.captures {
			if capture.Kind == CaptureTypeValueFormal {
				typeValueCapture = uint32(captureIndex)
				break
			}
		}
		c.produced = append(c.produced, producedRow{
			result: produced.result, target: produced.target, captures: captures, typeValueCapture: typeValueCapture,
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendFreshResults(input []freshResultDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("fresh result table", len(c.fresh), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, fresh := range input {
		c.fresh = append(c.fresh, freshResultRow{result: fresh.result, ordinal: fresh.ordinal, kind: fresh.kind})
	}
	return rangeOut, nil
}

func (c *Contract) appendCallbackResults(input []callbackResultDraft, callbacks []CallbackID) (indexRange, error) {
	rangeOut, err := checkedStoredRange("callback result table", len(c.callbackResults), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, result := range input {
		c.callbackResults = append(c.callbackResults, callbackResultRow{
			result: result.result, callback: callbacks[result.callback-1],
		})
	}
	return rangeOut, nil
}

func (c *Contract) appendResultAliases(input []resultAliasDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("result alias table", len(c.resultAliases), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, alias := range input {
		c.resultAliases = append(c.resultAliases, resultAliasRow{result: alias.result, source: alias.source})
	}
	return rangeOut, nil
}

func (c *Contract) appendOpaque() error {
	opHandle, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	opaque := Operation(opHandle)
	if _, err := checkedStoredRange("outcome table", len(c.outcomes), 4); err != nil {
		return err
	}
	unknownDraft := valuesDraft{tail: ValuesUnknown}
	unknown, err := c.appendValues(opaque, unknownDraft, nil)
	if err != nil {
		return err
	}
	unknownKey, err := unknownDraft.key()
	if err != nil {
		return err
	}
	outcomes, err := checkedStoredRange("outcome table", len(c.outcomes), 4)
	if err != nil {
		return err
	}
	for _, kind := range [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel,
	} {
		c.outcomes = append(c.outcomes, outcomeRow{kind: kind, values: unknown})
	}
	transfers, err := c.appendTransfers(opaque, []transferDraft{{
		endpoint:     TransferEndpoint{Kind: TransferEndpointExternal},
		payload:      InputSource{Kind: InputSourceAllInputs},
		alias:        InputSource{Kind: InputSourceAllInputs},
		identity:     TransferIdentityUnspecified,
		capabilities: TransferCapabilitiesUnspecified,
		outcomes: []TransferPossibility{
			TransferMayDeliver | TransferMayReject,
			TransferMayDeliver | TransferMayReject,
			TransferMayDeliver | TransferMayReject,
			TransferMayDeliver | TransferMayReject,
		},
	}})
	if err != nil {
		return err
	}
	_, callbacks, err := c.appendCallbacks(opaque, []callbackDraft{{
		function:  InputSource{Kind: InputSourceAllInputs},
		admission: OrdinaryCallable,
		arguments: unknownDraft,
		outcomes: [5]valuesDraft{
			unknownDraft, unknownDraft, unknownDraft, unknownDraft, unknownDraft,
		},
		lifecycle: CallbackRetainedOptionalMany,
		effects:   rowDraft{tail: RowUnknownOpen},
	}}, map[string]Values{unknownKey: unknown})
	if err != nil {
		return err
	}
	c.operations = append(c.operations, operationRow{
		input:      unknown,
		outcomes:   outcomes,
		callbacks:  callbacks,
		transfers:  transfers,
		effectTail: RowUnknownOpen,
	})
	c.opaque = opaque
	return nil
}

func (c *Contract) buildLookup() error {
	c.lookup = make([]bindingIndexRow, 0, len(c.bindings))
	for index := 0; index < c.BoundOperationCount(); index++ {
		handle, err := checkedStoredHandle("operation handle", index)
		if err != nil {
			return err
		}
		op := Operation(handle)
		row := c.operations[index]
		for binding := row.bindings.start; binding < row.bindings.end; binding++ {
			c.lookup = append(c.lookup, bindingIndexRow{binding: binding, operation: op})
		}
	}
	sort.Slice(c.lookup, func(left, right int) bool {
		return c.compareBindingRows(c.lookup[left].binding, c.lookup[right].binding) < 0
	})
	for index := 1; index < len(c.lookup); index++ {
		if c.compareBindingRows(c.lookup[index-1].binding, c.lookup[index].binding) == 0 {
			return errors.New("target: duplicate sealed binding")
		}
	}
	return nil
}

func (c *Contract) bindingEqual(row bindingRange, input BindingSpec) bool {
	if row.namespace != input.Namespace || row.owner.len() != len(input.Owner) || row.member.len() != len(input.Member) {
		return false
	}
	for index := range input.Owner {
		if c.segments[row.owner.start+uint32(index)] != input.Owner[index] {
			return false
		}
	}
	for index := range input.Member {
		if c.segments[row.member.start+uint32(index)] != input.Member[index] {
			return false
		}
	}
	return true
}

func (c *Contract) compareBindingRows(left, right uint32) int {
	return compareBindingRanges(c.bindings[left], c.bindings[right], c.segments)
}

func compareBindingRanges(left, right bindingRange, segments []string) int {
	if left.namespace < right.namespace {
		return -1
	}
	if left.namespace > right.namespace {
		return 1
	}
	if order := compareSegments(segments[left.owner.start:left.owner.end], segments[right.owner.start:right.owner.end]); order != 0 {
		return order
	}
	return compareSegments(segments[left.member.start:left.member.end], segments[right.member.start:right.member.end])
}

func compareBindingRangeSpec(left bindingRange, right BindingSpec, segments []string) int {
	if left.namespace < right.Namespace {
		return -1
	}
	if left.namespace > right.Namespace {
		return 1
	}
	if order := compareSegments(segments[left.owner.start:left.owner.end], right.Owner); order != 0 {
		return order
	}
	return compareSegments(segments[left.member.start:left.member.end], right.Member)
}

func validOperationOutcome(kind flowkind.OutcomeKind) bool {
	switch kind {
	case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		return true
	default:
		return false
	}
}

func validFreshKind(kind FreshKind) bool {
	switch kind {
	case FreshTable, FreshFunction, FreshThread, FreshUserdata, FreshError, FreshReflection:
		return true
	default:
		return false
	}
}

func validValuesTail(tail ValuesTail, variable ValuesVar, count uint32, opaque bool) bool {
	switch tail {
	case ValuesClosed:
		return variable == 0
	case ValuesVariable:
		return uint64(variable) < uint64(count)
	case ValuesUnknown:
		return opaque && variable == 0
	default:
		return false
	}
}

func validBinding(binding BindingSpec) bool {
	if len(binding.Member) == 0 || !validSegments(binding.Member) || !bindingLengthsFit(binding) {
		return false
	}
	switch binding.Namespace {
	case BindingBuiltin:
		return len(binding.Owner) == 0
	case BindingModule, BindingProvider:
		return len(binding.Owner) != 0 && validSegments(binding.Owner)
	default:
		return false
	}
}

func validSegments(parts []string) bool {
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := checkedStoredLength("binding segment bytes", len(part)); err != nil {
			return false
		}
	}
	return true
}

func bindingLengthsFit(binding BindingSpec) bool {
	if _, err := checkedStoredLength("binding owner length", len(binding.Owner)); err != nil {
		return false
	}
	if _, err := checkedStoredLength("binding member length", len(binding.Member)); err != nil {
		return false
	}
	return true
}

func cloneBinding(input BindingSpec) BindingSpec {
	return BindingSpec{
		Namespace: input.Namespace,
		Owner:     append([]string(nil), input.Owner...), Member: append([]string(nil), input.Member...),
	}
}

func compareBinding(left, right BindingSpec) int {
	if left.Namespace != right.Namespace {
		if left.Namespace < right.Namespace {
			return -1
		}
		return 1
	}
	if order := compareSegments(left.Owner, right.Owner); order != 0 {
		return order
	}
	if order := compareSegments(left.Member, right.Member); order != 0 {
		return order
	}
	return 0
}

func compareSegments(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareOutcome(left, right outcomeDraft) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if compared := compareValues(left.values, right.values); compared != 0 {
		return compared
	}
	return compareFreshResults(left.fresh, right.fresh)
}

func compareFreshResults(left, right []freshResultDraft) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index].result < right[index].result {
			return -1
		}
		if left[index].result > right[index].result {
			return 1
		}
		if left[index].kind < right[index].kind {
			return -1
		}
		if left[index].kind > right[index].kind {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareValues(left, right valuesDraft) int {
	limit := len(left.types)
	if len(right.types) < limit {
		limit = len(right.types)
	}
	for index := 0; index < limit; index++ {
		if order := bytes.Compare([]byte(left.types[index]), []byte(right.types[index])); order != 0 {
			return order
		}
	}
	if len(left.types) < len(right.types) {
		return -1
	}
	if len(left.types) > len(right.types) {
		return 1
	}
	if left.tail < right.tail {
		return -1
	}
	if left.tail > right.tail {
		return 1
	}
	if left.varID < right.varID {
		return -1
	}
	if left.varID > right.varID {
		return 1
	}
	if order := bytes.Compare([]byte(left.tailType), []byte(right.tailType)); order != 0 {
		return order
	}
	limit = len(left.suffix)
	if len(right.suffix) < limit {
		limit = len(right.suffix)
	}
	for index := 0; index < limit; index++ {
		if order := bytes.Compare([]byte(left.suffix[index]), []byte(right.suffix[index])); order != 0 {
			return order
		}
	}
	if len(left.suffix) < len(right.suffix) {
		return -1
	}
	if len(left.suffix) > len(right.suffix) {
		return 1
	}
	return 0
}

func (values valuesDraft) key() (string, error) {
	// Framed components prevent any concatenation ambiguity; this is cold seal
	// bookkeeping only and never a public binding or effect identity.
	parts := make([]int, 0)
	for _, typ := range values.types {
		if _, err := checkedStoredLength("Values key type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	for _, typ := range values.suffix {
		if _, err := checkedStoredLength("Values key suffix type bytes", len(typ)); err != nil {
			return "", err
		}
		parts = append(parts, 4, len(typ))
	}
	if _, err := checkedStoredLength("Values key tail type bytes", len(values.tailType)); err != nil {
		return "", err
	}
	parts = append(parts, 4, 1, 4, 4, len(values.tailType))
	total, err := checkedStoredTotal("Values key", parts...)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, total)
	for _, typ := range values.types {
		length, lengthErr := checkedStoredLength("Values key type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	out = appendUint32(out, ^uint32(0))
	out = append(out, byte(values.tail))
	out = appendUint32(out, uint32(values.varID))
	out = appendUint32(out, uint32(len(values.tailType)))
	out = append(out, values.tailType...)
	for _, typ := range values.suffix {
		length, lengthErr := checkedStoredLength("Values key suffix type bytes", len(typ))
		if lengthErr != nil {
			return "", lengthErr
		}
		out = appendUint32(out, length)
		out = append(out, typ...)
	}
	return string(out), nil
}

func compareEffect(left, right effectDraft) int {
	if left.target < right.target {
		return -1
	}
	if left.target > right.target {
		return 1
	}
	if order := compareUint32Slice(left.values, right.values); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.types, right.types); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.valuesVar, right.valuesVar); order != 0 {
		return order
	}
	if order := compareUint32Slice(left.rows, right.rows); order != 0 {
		return order
	}
	if left.hasPublication != right.hasPublication {
		if !left.hasPublication {
			return -1
		}
		return 1
	}
	if !left.hasPublication {
		return 0
	}
	return comparePublicationEffectDescriptor(left.publication, right.publication)
}

func comparePublicationEffectDescriptor(left, right PublicationEffectDescriptor) int {
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	if left.subject != right.subject {
		if left.subject < right.subject {
			return -1
		}
		return 1
	}
	if left.destination != right.destination {
		if left.destination < right.destination {
			return -1
		}
		return 1
	}
	if left.context != right.context {
		if left.context < right.context {
			return -1
		}
		return 1
	}
	if left.escape != right.escape {
		if left.escape < right.escape {
			return -1
		}
		return 1
	}
	if left.mutability != right.mutability {
		if left.mutability < right.mutability {
			return -1
		}
		return 1
	}
	if left.lifetime < right.lifetime {
		return -1
	}
	if left.lifetime > right.lifetime {
		return 1
	}
	return 0
}

func compareUint32Slice[T ~uint32](left, right []T) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func totalEffectArguments(input []effectDraft, count func(effectDraft) int, what string) (int, error) {
	parts := make([]int, len(input))
	for index, effect := range input {
		parts[index] = count(effect)
	}
	return checkedStoredTotal(what, parts...)
}

func appendUint32(out []byte, value uint32) []byte {
	return append(out, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}
