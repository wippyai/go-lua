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

func (c *Contract) appendOperation(builder *operationvalue.QueryBuilder, op vocabulary.Operation, draft *operationDraft, keys exactkey.Table, callbackIDs []vocabulary.CallbackID) error {
	expected, err := checkedStoredHandle("operation table", len(c.operations))
	if err != nil {
		return err
	}
	if op != vocabulary.Operation(expected) {
		return errors.New("target: noncanonical operation handle")
	}
	typeHandle, err := builder.AppendQueryTypes(draft.types, draft.declarations)
	if err != nil {
		return err
	}

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
		declaration := queryValuesDeclaration(op, allValues[key])
		handle, appendErr := builder.AppendQueryValues(declaration, typeHandle)
		if appendErr != nil {
			return appendErr
		}
		valuesHandle[key] = handle
	}
	operationInput := valuesHandle[inputKey]
	callbackIDs, _, err = c.appendCallbacks(builder, op, draft.callbacks, valuesHandle, callbackIDs)
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
	effectRange, err := c.appendEffects(builder, effectOwnerOperation, draft.effects)
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
		outcomes: operationOutcomes, subedges: operationSubedges,
		suspensions: operationSuspensions, spawns: operationSpawns,
		resumes:         operationResumes,
		subedgeRelation: operationRelation, effects: operationEffects,
		effectTail: draft.effectTail, effectVar: draft.effectVar,
	})
	query := operationvalue.QueryOperationInput{
		Input: operationInput, RowFormals: draft.rowFormals,
		EffectTail: draft.effectTail, EffectVar: draft.effectVar,
		EffectIndices: make([]int, operationEffects.len()),
		TypeFormals:   make([]vocabulary.Type, len(draft.constraints)),
		ValuesTypes:   make([]vocabulary.Type, len(draft.valuesTypes)),
	}
	for index, constraint := range draft.constraints {
		// A missing declaration is the canonical unconstrained formal and is
		// represented by the zero handle.
		query.TypeFormals[index] = typeHandle[constraint]
	}
	for index, key := range draft.valuesTypes {
		handle, found := typeHandle[key]
		if !found || handle == 0 {
			return errors.New("target: unresolved Values variable type")
		}
		query.ValuesTypes[index] = handle
	}
	for index := range query.EffectIndices {
		query.EffectIndices[index] = int(operationEffects.start) + index
	}
	for _, outcome := range draft.outcomes {
		key, keyErr := outcome.values.key()
		if keyErr != nil {
			return keyErr
		}
		value, found := valuesHandle[key]
		if !found {
			return errors.New("target: unresolved operation outcome Values")
		}
		query.Outcomes = append(query.Outcomes, operationvalue.QueryOutcomeInput{Kind: outcome.kind, Values: value})
	}
	for _, result := range draft.behavior.results {
		query.Behavior = append(query.Behavior, operationvalue.BehaviorResultInput{
			Outcome: result.outcome, Result: result.result, Source: result.source, Relation: result.relation,
		})
	}
	for _, predicate := range draft.behavior.predicates {
		query.BehaviorPredicates = append(query.BehaviorPredicates, operationvalue.BehaviorPredicateInput{
			Outcome: predicate.outcome, Result: predicate.result, Subject: predicate.subject, Relation: predicate.relation,
		})
	}
	for _, transfer := range draft.transfers {
		query.Transfers = append(query.Transfers, operationvalue.TransferInput{
			Endpoint: transfer.endpoint, Payload: transfer.payload, Alias: transfer.alias,
			Identity: transfer.identity, Capabilities: transfer.capabilities,
			Outcomes: append([]vocabulary.TransferPossibility(nil), transfer.outcomes...),
		})
	}
	if err := builder.AppendQueryOperation(op, query); err != nil {
		return err
	}
	return nil
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

func (c *Contract) appendOpaque(builder *operationvalue.QueryBuilder, opaque vocabulary.Operation) error {
	if opaque == 0 || uint64(opaque) != uint64(len(c.operations)+1) {
		return errors.New("target: noncanonical opaque operation handle")
	}
	if _, err := checkedStoredRange("outcome table", len(c.outcomes), 4); err != nil {
		return err
	}
	unknownDraft := valuesDraft{tail: vocabulary.ValuesUnknown}
	unknown, err := builder.AppendQueryValues(operationvalue.QueryValuesDeclaration{
		Owner: opaque, Tail: vocabulary.ValuesUnknown,
	}, nil)
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
	transfers := []operationvalue.TransferInput{{
		Endpoint:     vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal},
		Payload:      vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		Alias:        vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs},
		Identity:     vocabulary.TransferIdentityUnspecified,
		Capabilities: vocabulary.TransferCapabilitiesUnspecified,
		Outcomes: []vocabulary.TransferPossibility{
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
			vocabulary.TransferMayDeliver | vocabulary.TransferMayReject,
		},
	}}
	issuedOpaque := callbackIDForOpaque(c.Operations, opaque)
	_, _, err = c.appendCallbacks(builder, opaque, []callbackDraft{{
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
		outcomes:   outcomes,
		effectTail: vocabulary.RowUnknownOpen,
	})
	return builder.AppendQueryOperation(opaque, operationvalue.QueryOperationInput{
		Input: unknown,
		Outcomes: []operationvalue.QueryOutcomeInput{
			{Kind: flowkind.OutcomeNormal, Values: unknown},
			{Kind: flowkind.OutcomeThrow, Values: unknown},
			{Kind: flowkind.OutcomeYield, Values: unknown},
			{Kind: flowkind.OutcomeCancel, Values: unknown},
		},
		Transfers: transfers, EffectTail: vocabulary.RowUnknownOpen,
	})
}

func callbackIDForOpaque(core operationvalue.Core, opaque vocabulary.Operation) vocabulary.CallbackID {
	callback, ok := core.CallbackAt(opaque, 0)
	if !ok {
		return 0
	}
	return callback
}

func queryValuesDeclaration(owner vocabulary.Operation, draft valuesDraft) operationvalue.QueryValuesDeclaration {
	return operationvalue.QueryValuesDeclaration{
		Owner: owner, Types: append([]string(nil), draft.types...),
		Tail: draft.tail, VarID: draft.varID,
		Suffix: append([]string(nil), draft.suffix...),
	}
}
