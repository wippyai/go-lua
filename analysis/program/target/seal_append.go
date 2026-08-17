package target

import (
	"errors"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func (c *Contract) appendOperation(op Operation, draft *operationDraft, keys map[keyspace.LiteralValue]ExactKey) error {
	expected, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	if op != Operation(expected) {
		return errors.New("target: noncanonical operation handle")
	}
	typeHandle, err := c.appendTypes(op, draft.types, draft.declarations)
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

func (c *Contract) appendTypes(owner Operation, input map[string][]byte, declarations map[string]schematype.Type) (map[string]Type, error) {
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
		declaration, declarationOK := declarations[key]
		if !declarationOK || !declaration.Available() {
			return nil, errors.New("target: missing neutral type declaration")
		}
		if _, err := checkedStoredLength("type bytes", len(input[key])); err != nil {
			return nil, err
		}
		handle, err := checkedStoredHandle("type table", len(c.types))
		if err != nil {
			return nil, err
		}
		c.types = append(c.types, typeRow{
			owner: owner, declaration: declaration,
			bytes: append([]byte(nil), input[key]...),
		})
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
		admission: schematype.CallableAdmissionOrdinary,
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
