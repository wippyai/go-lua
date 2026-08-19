package target

import (
	"errors"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"sort"
)

func (c *Contract) appendOperation(op vocabulary.Operation, draft *operationDraft, keys exactkey.Table, callbackIDs []vocabulary.CallbackID) error {
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
	rowFormals := draft.rowFormals
	effectTail, effectVar := draft.effectTail, draft.effectVar
	formalRange, err := checkedStoredRange("type formal pool", len(c.formals), len(draft.constraints))
	if err != nil {
		return err
	}
	for _, constraint := range draft.constraints {
		c.formals = append(c.formals, typeHandle[constraint])
	}
	operationTypeFormals := formalRange
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
	operationValuesTypes := valuesTypes

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
	operationInput := valuesHandle[inputKey]
	callbackIDs, _, err = c.appendCallbacks(op, draft.callbacks, valuesHandle, callbackIDs)
	if err != nil {
		return err
	}
	suspensions, suspensionErr := c.appendSuspensions(draft.suspensions)
	if suspensionErr != nil {
		return suspensionErr
	}
	operationSuspensions := suspensions
	resumes, resumeErr := c.appendResumes(op, draft.resumes, valuesHandle)
	if resumeErr != nil {
		return resumeErr
	}
	operationResumes := resumes
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
	operationOutcomes := outcomeRange
	behaviorRange, behaviorPredicateRange, behaviorErr := c.appendBehavior(draft.behavior)
	if behaviorErr != nil {
		return behaviorErr
	}
	operationBehavior, operationPredicates := behaviorRange, behaviorPredicateRange
	subedgeRange, subedgeErr := c.appendSubedges(op, draft.subedges, callbackIDs, valuesHandle, keys)
	if subedgeErr != nil {
		return subedgeErr
	}
	operationSubedges := subedgeRange
	spawns, spawnErr := c.appendSpawns(op, draft.spawns, callbackIDs, outcomeValues)
	if spawnErr != nil {
		return spawnErr
	}
	operationSpawns := spawns
	transfers, transferErr := c.appendTransfers(op, draft.transfers)
	if transferErr != nil {
		return transferErr
	}
	operationTransfers := transfers
	effectRange, err := c.appendEffects(effectOwnerOperation, draft.effects)
	if err != nil {
		return err
	}
	operationEffects := effectRange
	operationRelation := uint32(0)
	if draft.subedgeRelation != nil {
		branch, branchErr := c.appendSubedgeRelation(op, *draft.subedgeRelation, operationSubedges, operationOutcomes, operationEffects)
		if branchErr != nil {
			return branchErr
		}
		operationRelation = branch
	}
	c.operations = append(c.operations, operationRow{
		input: operationInput, outcomes: operationOutcomes,
		behavior: operationBehavior, behaviorPredicates: operationPredicates,
		valuesTypes: operationValuesTypes, subedges: operationSubedges,
		suspensions: operationSuspensions, spawns: operationSpawns,
		resumes: operationResumes, transfers: operationTransfers,
		subedgeRelation: operationRelation, effects: operationEffects,
		typeFormals: operationTypeFormals, rowFormals: rowFormals,
		effectTail: effectTail, effectVar: effectVar,
	})
	return nil
}

func (c *Contract) appendBehavior(input behaviorDraft) (indexRange, indexRange, error) {
	resultRange, err := checkedStoredRange("behavior result table", len(c.behaviorResults), len(input.results))
	if err != nil {
		return indexRange{}, indexRange{}, err
	}
	predicateRange, err := checkedStoredRange("behavior predicate table", len(c.behaviorPredicates), len(input.predicates))
	if err != nil {
		return indexRange{}, indexRange{}, err
	}
	for _, result := range input.results {
		c.behaviorResults = append(c.behaviorResults, behaviorResultRow{
			outcome: result.outcome, result: result.result, source: result.source, relation: result.relation,
		})
	}
	for _, predicate := range input.predicates {
		c.behaviorPredicates = append(c.behaviorPredicates, behaviorPredicateRow{
			outcome: predicate.outcome, result: predicate.result, subject: predicate.subject, relation: predicate.relation,
		})
	}
	if len(input.results) == 0 && len(input.predicates) == 0 {
		return indexRange{}, indexRange{}, nil
	}
	return resultRange, predicateRange, nil
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

func (c *Contract) appendOpaque(opaque vocabulary.Operation) error {
	if opaque == 0 || uint64(opaque) != uint64(len(c.operations)+1) {
		return errors.New("target: noncanonical opaque operation handle")
	}
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
	issuedOpaque := callbackIDForOpaque(c.operationCore, opaque)
	_, _, err = c.appendCallbacks(opaque, []callbackDraft{{
		function:  vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		admission: schematype.CallableAdmissionOrdinary,
		arguments: unknownDraft,
		outcomes: [5]valuesDraft{
			unknownDraft, unknownDraft, unknownDraft, unknownDraft, unknownDraft,
		},
		lifecycle: vocabulary.CallbackRetainedOptionalMany,
		effects:   rowDraft{tail: vocabulary.RowUnknownOpen},
	}}, map[string]vocabulary.Values{unknownKey: unknown}, []vocabulary.CallbackID{issuedOpaque})
	if err != nil {
		return err
	}
	c.operations = append(c.operations, operationRow{
		input: unknown, outcomes: outcomes, transfers: transfers,
		effectTail: vocabulary.RowUnknownOpen,
	})
	return nil
}

func callbackIDForOpaque(core operationvalue.Core, opaque vocabulary.Operation) vocabulary.CallbackID {
	callback, ok := core.CallbackAt(opaque, 0)
	if !ok {
		return 0
	}
	return callback
}
