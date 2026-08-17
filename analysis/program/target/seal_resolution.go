package target

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
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
