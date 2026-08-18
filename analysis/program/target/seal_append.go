package target

import (
	"errors"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"sort"
)

func (c *Contract) appendOperation(op vocabulary.Operation, draft *operationDraft, keys map[keyspace.LiteralValue]vocabulary.ExactKey) error {
	expected, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	if op != vocabulary.Operation(expected) {
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

	valuesHandle := make(map[string]vocabulary.Values)
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
	outcomeValues := make([]vocabulary.Values, 0, len(draft.outcomes))
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
	effectRange, err := c.appendEffects(effectOwnerOperation, draft.effects)
	if err != nil {
		return err
	}
	row.effects = effectRange
	if draft.subedgeRelation != nil {
		branch, branchErr := c.appendSubedgeRelation(op, *draft.subedgeRelation, row)
		if branchErr != nil {
			return branchErr
		}
		row.subedgeRelation = branch
	}
	c.operations = append(c.operations, row)
	if len(draft.bindings) != 0 {
		c.boundCount++
	}
	return nil
}

func (c *Contract) appendTypes(owner vocabulary.Operation, input map[string][]byte, declarations map[string]schematype.Type) (map[string]vocabulary.Type, error) {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, err := checkedStoredRange("type table", len(c.types), len(keys)); err != nil {
		return nil, err
	}
	handles := make(map[string]vocabulary.Type, len(keys))
	for _, key := range keys {
		declaration, declarationOK := declarations[key]
		if !declarationOK || !declaration.Available() {
			return nil, errors.New("target: missing neutral type declaration")
		}
		if _, err := vocabulary.CheckedStoredLength("type bytes", len(input[key])); err != nil {
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
		handles[key] = vocabulary.Type(handle)
	}
	return handles, nil
}

func (c *Contract) appendValues(owner vocabulary.Operation, input valuesDraft, handles map[string]vocabulary.Type) (vocabulary.Values, error) {
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
	return vocabulary.Values(handle), nil
}

func (c *Contract) appendBindings(input []vocabulary.BindingSpec, keys map[keyspace.LiteralValue]vocabulary.ExactKey) (indexRange, error) {
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

func (c *Contract) appendBinding(input vocabulary.BindingSpec, keys map[keyspace.LiteralValue]vocabulary.ExactKey) (bindingRange, error) {
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

func lookupDraftValues(values map[string]vocabulary.Values, draft valuesDraft) (vocabulary.Values, error) {
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

func (c *Contract) appendOpaque() error {
	opHandle, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	opaque := vocabulary.Operation(opHandle)
	if _, err := checkedStoredRange("outcome table", len(c.outcomes), 4); err != nil {
		return err
	}
	unknownDraft := valuesDraft{tail: vocabulary.ValuesUnknown}
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
		endpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
		payload:      vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		alias:        vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		identity:     vocabulary.TransferIdentityUnspecified,
		capabilities: vocabulary.TransferCapabilitiesUnspecified,
		outcomes: []vocabulary.TransferPossibility{
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
		},
	}})
	if err != nil {
		return err
	}
	_, callbacks, err := c.appendCallbacks(opaque, []callbackDraft{{
		function:  vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		admission: schematype.CallableAdmissionOrdinary,
		arguments: unknownDraft,
		outcomes: [5]valuesDraft{
			unknownDraft, unknownDraft, unknownDraft, unknownDraft, unknownDraft,
		},
		lifecycle: vocabulary.CallbackRetainedOptionalMany,
		effects:   rowDraft{tail: vocabulary.RowUnknownOpen},
	}}, map[string]vocabulary.Values{unknownKey: unknown})
	if err != nil {
		return err
	}
	c.operations = append(c.operations, operationRow{
		input:      unknown,
		outcomes:   outcomes,
		callbacks:  callbacks,
		transfers:  transfers,
		effectTail: vocabulary.RowUnknownOpen,
	})
	c.opaque = opaque
	return nil
}

func (c *Contract) buildLookup() error {
	c.lookup = make([]bindingIndexRow, 0, len(c.bindings))
	for index := 0; index < c.boundOperationCount(); index++ {
		handle, err := checkedStoredHandle("operation handle", index)
		if err != nil {
			return err
		}
		op := vocabulary.Operation(handle)
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

func (c *Contract) appendProtocolTransitions(input []transitionDraft) (indexRange, error) {
	rangeOut, err := checkedStoredRange("protocol transition table", len(c.transitions), len(input))
	if err != nil {
		return indexRange{}, err
	}
	for _, item := range input {
		outcomes := make([]transitionOutcomeRow, len(item.outcomes))
		for i, outcome := range item.outcomes {
			outcomes[i] = transitionOutcomeRow{outcome: outcome.outcome, to: outcome.to}
		}
		rangeItems, appendErr := appendStoredRange(&c.transitionOutcomes, outcomes, "protocol transition outcome table")
		if appendErr != nil {
			return indexRange{}, appendErr
		}
		c.transitions = append(c.transitions, transitionRow{operation: item.operation, input: item.input, from: item.from, outcomes: rangeItems})
	}
	return rangeOut, nil
}

func (c *Contract) appendInitialValueBinding(input vocabulary.BindingSpec, keys map[keyspace.LiteralValue]vocabulary.ExactKey) (uint32, error) {
	if _, err := checkedStoredRange("initial value binding table", len(c.initialValueBinds), 1); err != nil {
		return 0, err
	}
	binding, err := c.appendBinding(input, keys)
	if err != nil {
		return 0, err
	}
	c.initialValueBinds = append(c.initialValueBinds, binding)
	return uint32(len(c.initialValueBinds)), nil
}
