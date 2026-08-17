package target

import (
	"errors"
	"fmt"
	"sort"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func freezeOperation(source int, input OperationSpec, semantics schematype.Semantics) (operationDraft, error) {
	if err := checkedCoordinateCount("Values variable count", input.ValuesVars); err != nil {
		return operationDraft{}, err
	}
	if err := checkedCoordinateCount("row formal count", input.RowFormals); err != nil {
		return operationDraft{}, err
	}
	formals, err := copyFormals(input.TypeFormals)
	if err != nil {
		return operationDraft{}, err
	}
	if _, err := checkedStoredLength("type formal pool", len(formals)); err != nil {
		return operationDraft{}, err
	}
	draft := operationDraft{
		source:            source,
		semantics:         semantics,
		formals:           formals,
		valuesVars:        input.ValuesVars,
		rowFormals:        input.RowFormals,
		types:             make(map[string][]byte),
		declarations:      make(map[string]schematype.Type),
		formalConstraints: make([]schematype.Type, len(formals)),
		constraints:       make([]string, len(formals)),
	}
	for index, formal := range formals {
		draft.formalConstraints[index] = formal.Constraint
		if !formal.Constraint.Available() {
			continue
		}
		key, freezeErr := draft.freezeType(formal.Constraint)
		if freezeErr != nil {
			return operationDraft{}, fmt.Errorf("target: type formal %d constraint: %w", index, freezeErr)
		}
		draft.constraints[index] = key
	}
	draft.input, err = draft.freezeValues(input.Input, false)
	if err != nil {
		return operationDraft{}, fmt.Errorf("target: input: %w", err)
	}
	if len(input.Input.Suffix) != 0 {
		return operationDraft{}, errors.New("target: input Values cannot have a suffix")
	}
	draft.bindings, err = freezeBindings(input.Bindings)
	if err != nil {
		return operationDraft{}, err
	}
	draft.callbacks, err = draft.freezeCallbacks(input.Callbacks)
	if err != nil {
		return operationDraft{}, err
	}
	draft.outcomes, err = draft.freezeOutcomes(input.Outcomes)
	if err != nil {
		return operationDraft{}, err
	}
	draft.suspensions, err = draft.freezeSuspensions(input.Suspensions)
	if err != nil {
		return operationDraft{}, err
	}
	draft.subedges, err = draft.freezeSubedges(input.Subedges)
	if err != nil {
		return operationDraft{}, err
	}
	draft.resumes, err = draft.freezeResumes(input.Resumes)
	if err != nil {
		return operationDraft{}, err
	}
	if err := draft.sealValuesVarTypes(); err != nil {
		return operationDraft{}, err
	}
	draft.spawns, err = draft.freezeSpawns(input.Spawns)
	if err != nil {
		return operationDraft{}, err
	}
	draft.transfers, err = draft.freezeTransfers(input.Transfers)
	if err != nil {
		return operationDraft{}, err
	}
	if err := draft.freezeEffects(input.Effects); err != nil {
		return operationDraft{}, err
	}
	if input.GsubTableReplacement != nil {
		branch, branchErr := draft.freezeGsubTableReplacement(*input.GsubTableReplacement)
		if branchErr != nil {
			return operationDraft{}, branchErr
		}
		draft.gsubTable = &branch
	}
	return draft, nil
}

func (d operationDraft) freezeSpawns(input []SpawnSpec) ([]spawnDraft, error) {
	if _, err := checkedStoredLength("spawn table", len(input)); err != nil {
		return nil, err
	}
	if len(input) > 1 {
		return nil, errors.New("target: operation has multiple spawn relations")
	}
	out := make([]spawnDraft, len(input))
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	for index, item := range input {
		if item.Function.Kind != InputSourceValueFormal || uint64(item.Function.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("target: spawn %d has invalid function source", index)
		}
		if item.Child == 0 || uint64(item.Child) > uint64(len(d.callbacks)) {
			return nil, fmt.Errorf("target: spawn %d child callback outside scope", index)
		}
		var child callbackDraft
		foundChild := false
		for _, candidate := range d.callbacks {
			if candidate.source == int(item.Child-1) {
				child, foundChild = candidate, true
				break
			}
		}
		if !foundChild || child.function != item.Function || child.lifecycle != CallbackRetainedRequiredOnce {
			return nil, fmt.Errorf("target: spawn %d child is not the exact detached callback", index)
		}
		if uint64(item.Yield) >= uint64(len(d.outcomes)) || uint64(item.ParentResume) >= uint64(len(d.outcomes)) || uint64(item.ChildEntry) >= uint64(len(d.outcomes)) {
			return nil, fmt.Errorf("target: spawn %d outcome outside scope", index)
		}
		if bySource[item.Yield].kind != flowkind.OutcomeYield || bySource[item.ParentResume].kind != flowkind.OutcomeNormal {
			return nil, fmt.Errorf("target: spawn %d has invalid parent yield/resume outcomes", index)
		}
		if !emptyClosedValues(bySource[item.ChildEntry].values) || !emptyClosedValues(bySource[item.ParentResume].values) || compareValues(bySource[item.ChildEntry].values, bySource[item.ParentResume].values) != 0 {
			return nil, fmt.Errorf("target: spawn %d child entry and parent resume must share the closed empty Pack", index)
		}
		if len(item.Alternatives) != 2 || item.Alternatives[0] == item.Alternatives[1] ||
			(item.Alternatives[0] != SpawnChildEntryThenParentResume && item.Alternatives[0] != SpawnParentResumeThenChildEntry) ||
			(item.Alternatives[1] != SpawnChildEntryThenParentResume && item.Alternatives[1] != SpawnParentResumeThenChildEntry) {
			return nil, fmt.Errorf("target: spawn %d has incomplete sibling alternatives", index)
		}
		alternatives := [2]SpawnSiblingAlternative{item.Alternatives[0], item.Alternatives[1]}
		if alternatives[1] < alternatives[0] {
			alternatives[0], alternatives[1] = alternatives[1], alternatives[0]
		}
		out[index] = spawnDraft{function: item.Function, child: item.Child, yield: item.Yield, parentResume: item.ParentResume, childEntry: item.ChildEntry, alternatives: alternatives}
	}
	return out, nil
}

func emptyClosedValues(values valuesDraft) bool {
	return len(values.types) == 0 && len(values.suffix) == 0 && values.tail == ValuesClosed && values.varID == 0
}

func copyFormals(input []TypeFormalSpec) ([]TypeFormalSpec, error) {
	if len(input) == 0 {
		return nil, nil
	}
	return append([]TypeFormalSpec(nil), input...), nil
}

func (d *operationDraft) freezeType(value schematype.Type) (string, error) {
	if !value.Available() {
		return "", errors.New("unavailable type declaration")
	}
	if err := d.semantics.Validate(value, d.formalConstraints); err != nil {
		return "", fmt.Errorf("type declaration rejected: %w", err)
	}
	encoded := value.Bytes()
	if _, err := checkedStoredLength("type bytes", len(encoded)); err != nil {
		return "", err
	}
	key := string(encoded)
	if key == "" {
		// Primitive envelopes intentionally have no domain bytes. Their digest
		// remains a stable, collision-free local key while the declaration is
		// retained for the domain semantic adapter.
		digest := value.Digest()
		key = string(digest[:])
	}
	if existing, exists := d.declarations[key]; exists && !existing.Equal(value) {
		return "", errors.New("distinct neutral type declarations share a storage key")
	}
	if _, exists := d.types[key]; !exists {
		d.types[key] = append([]byte(nil), encoded...)
		d.declarations[key] = value
	}
	return key, nil
}

func (d *operationDraft) freezeValues(input ValuesSpec, opaque bool) (valuesDraft, error) {
	if !validValuesTail(input.Tail, input.Var, d.valuesVars, opaque) {
		return valuesDraft{}, errors.New("target: invalid Values tail")
	}
	out := valuesDraft{tail: input.Tail, varID: input.Var}
	if input.Tail != ValuesVariable {
		if input.TailType.Available() {
			return valuesDraft{}, errors.New("target: Values tail type requires a ValuesVariable tail")
		}
	} else {
		tailType := input.TailType
		if !tailType.Available() {
			any, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
			if !ok {
				return valuesDraft{}, errors.New("target: unavailable default Values tail type")
			}
			tailType = any
		}
		key, err := d.freezeType(tailType)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values tail type: %w", err)
		}
		out.tailType = key
	}
	if _, err := checkedStoredLength("Values fixed list", len(input.Fixed)); err != nil {
		return valuesDraft{}, err
	}
	if _, err := checkedStoredLength("Values suffix list", len(input.Suffix)); err != nil {
		return valuesDraft{}, err
	}
	// A closed tail has no end-relative coordinate. Canonicalize its suffix
	// into the prefix so equivalent authored Values share one handle.
	fixed := input.Fixed
	suffix := input.Suffix
	if input.Tail == ValuesClosed && len(suffix) != 0 {
		fixed = make([]schematype.Type, 0, len(input.Fixed)+len(suffix))
		fixed = append(fixed, input.Fixed...)
		fixed = append(fixed, suffix...)
		suffix = nil
	}
	if len(fixed) != 0 {
		out.types = make([]string, len(fixed))
	}
	for index, value := range fixed {
		if !value.Available() {
			return valuesDraft{}, fmt.Errorf("target: nil Values element %d", index)
		}
		key, err := d.freezeType(value)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values element %d: %w", index, err)
		}
		out.types[index] = key
	}
	if len(suffix) != 0 {
		out.suffix = make([]string, len(suffix))
	}
	for index, value := range suffix {
		if !value.Available() {
			return valuesDraft{}, fmt.Errorf("target: nil Values suffix element %d", index)
		}
		key, err := d.freezeType(value)
		if err != nil {
			return valuesDraft{}, fmt.Errorf("target: Values suffix element %d: %w", index, err)
		}
		out.suffix[index] = key
	}
	return out, nil
}

func (d *operationDraft) freezeOutcomes(input []OutcomeSpec) ([]outcomeDraft, error) {
	if len(input) == 0 {
		return nil, errors.New("target: operation has no outcomes")
	}
	if _, err := checkedStoredLength("outcome table", len(input)); err != nil {
		return nil, err
	}
	out := make([]outcomeDraft, len(input))
	for index, item := range input {
		if !validOperationOutcome(item.Kind) {
			return nil, fmt.Errorf("target: invalid outcome kind %d", item.Kind)
		}
		values, err := d.freezeValues(item.Values, false)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		produced, err := d.freezeProduced(item.Produced, values)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		callbackResults, err := d.freezeCallbackResults(item.CallbackResults, values, produced)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		resultAliases, err := d.freezeResultAliases(item.ResultAliases, values)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		fresh, err := d.freezeFreshResults(item.FreshResults, values, resultAliases)
		if err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		if err := validateProducedTypeValueFreshResults(produced, fresh); err != nil {
			return nil, fmt.Errorf("target: outcome %d: %w", index, err)
		}
		anchor, anchorErr := outcomeAnchor(item.Kind, values, fresh)
		if anchorErr != nil {
			return nil, fmt.Errorf("target: outcome %d anchor: %w", index, anchorErr)
		}
		out[index] = outcomeDraft{
			source: index, kind: item.Kind, values: values,
			anchor:   anchor,
			produced: produced, fresh: fresh, callbackResults: callbackResults, resultAliases: resultAliases,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareOutcome(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareOutcome(out[index-1], out[index]) == 0 {
			// The frozen outcome discriminator is Kind, Values, and the
			// nominal FreshResult relation. Produced, callback-result, and
			// alias rows are conjunctive annotations, not alternate-case keys.
			return nil, errors.New("target: duplicate outcome case")
		}
	}
	return out, nil
}

func outcomeAnchor(kind flowkind.OutcomeKind, values valuesDraft, fresh []freshResultDraft) (string, error) {
	valuesKey, err := values.key()
	if err != nil {
		return "", err
	}
	if _, err := checkedStoredLength("outcome anchor Values key", len(valuesKey)); err != nil {
		return "", err
	}
	if _, err := checkedStoredLength("outcome anchor fresh result table", len(fresh)); err != nil {
		return "", err
	}
	// Kind plus framed Values key and fixed-width canonical Fresh rows.  The
	// frame matters because Values keys end in an uncounted suffix sequence.
	parts := []int{1, 4, len(valuesKey), 4}
	for range fresh {
		parts = append(parts, 4, 1)
	}
	total, err := checkedStoredTotal("outcome anchor", parts...)
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, total)
	out = append(out, byte(kind))
	out = appendUint32(out, uint32(len(valuesKey)))
	out = append(out, valuesKey...)
	out = appendUint32(out, uint32(len(fresh)))
	for _, row := range fresh {
		out = appendUint32(out, row.result)
		out = append(out, byte(row.kind))
	}
	return string(out), nil
}

// sealValuesVarTypes makes the ValuesVar class table total. A variable which
// no ValuesSpec directly constrains still carries the ABI default neutral Any
// declaration; the domain adapter interprets that atom during later reads.
func (d *operationDraft) sealValuesVarTypes() error {
	if d.valuesVars == 0 {
		d.valuesTypes = nil
		return nil
	}
	any, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		return errors.New("target: unavailable default Values tail type")
	}
	anyKey, err := d.freezeType(any)
	if err != nil {
		return fmt.Errorf("target: default Values tail type: %w", err)
	}
	classes := make([]string, d.valuesVars)
	seen := make([]bool, d.valuesVars)
	check := func(values valuesDraft) error {
		if values.tail != ValuesVariable {
			return nil
		}
		variable := values.varID
		if !seen[variable] {
			classes[variable], seen[variable] = values.tailType, true
			return nil
		}
		if classes[variable] != values.tailType {
			return fmt.Errorf("target: Values variable %d has conflicting tail types", variable)
		}
		return nil
	}
	if err := check(d.input); err != nil {
		return err
	}
	for _, outcome := range d.outcomes {
		if err := check(outcome.values); err != nil {
			return err
		}
	}
	for _, callback := range d.callbacks {
		if err := check(callback.arguments); err != nil {
			return err
		}
		for _, terminal := range callback.outcomes {
			if err := check(terminal); err != nil {
				return err
			}
		}
	}
	for _, subedge := range d.subedges {
		if err := visitSubedgeValues(subedge, check); err != nil {
			return err
		}
	}
	for _, resume := range d.resumes {
		if err := check(resume.arguments); err != nil {
			return err
		}
	}
	for index := range classes {
		if !seen[index] {
			classes[index] = anyKey
		}
	}
	d.valuesTypes = classes
	return nil
}

// visitSubedgeValues is the complete closure of Values endpoints owned by one
// Subedge relation. Keeping this enumeration singular makes the class-table
// and interning closures agree as the relation evolves.
func visitSubedgeValues(edge subedgeDraft, visit func(valuesDraft) error) error {
	if err := visit(edge.arguments); err != nil {
		return err
	}
	for _, terminal := range edge.outcomes {
		if err := visit(terminal); err != nil {
			return err
		}
	}
	// Admission failure is a distinct Values source, and its route owns a
	// separate projected Result. Neither is derived from a callee terminal.
	if err := visit(edge.admissionFailure); err != nil {
		return err
	}
	if err := visit(edge.admissionRoute.result); err != nil {
		return err
	}
	for _, route := range edge.routes {
		if err := visit(route.result); err != nil {
			return err
		}
	}
	return nil
}

func (d operationDraft) freezeSuspensions(input []SuspensionSpec) ([]suspensionDraft, error) {
	if _, err := checkedStoredLength("suspension table", len(input)); err != nil {
		return nil, err
	}
	if len(input) == 0 {
		return nil, nil
	}
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	out := make([]suspensionDraft, len(input))
	for index, item := range input {
		if uint64(item.Yield) >= uint64(len(bySource)) || uint64(item.Reentry) >= uint64(len(bySource)) {
			return nil, fmt.Errorf("target: suspension %d has outcome outside scope", index)
		}
		if bySource[item.Yield].kind != flowkind.OutcomeYield {
			return nil, fmt.Errorf("target: suspension %d yield is not OutcomeYield", index)
		}
		switch bySource[item.Reentry].kind {
		case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeCancel:
		default:
			return nil, fmt.Errorf("target: suspension %d reentry is not restorable", index)
		}
		if item.Source != ReentryByCall && item.Source != ReentryByProvider {
			return nil, fmt.Errorf("target: suspension %d has invalid reentry source", index)
		}
		if item.Multiplicity != ReentryOnce && item.Multiplicity != ReentryMany {
			return nil, fmt.Errorf("target: suspension %d has invalid multiplicity", index)
		}
		out[index] = suspensionDraft{yield: item.Yield, reentry: item.Reentry, source: item.Source, multiplicity: item.Multiplicity}
	}
	return out, nil
}

func (d operationDraft) freezeResumes(input []ResumeSpec) ([]resumeDraft, error) {
	if _, err := checkedStoredLength("resume table", len(input)); err != nil {
		return nil, err
	}
	bySource := make([]outcomeDraft, len(d.outcomes))
	for _, outcome := range d.outcomes {
		bySource[outcome.source] = outcome
	}
	out := make([]resumeDraft, len(input))
	for index, item := range input {
		switch item.Source {
		case ResumeSourceValueFormal:
			if uint64(item.Carrier) >= uint64(d.valueFormalCount()) {
				return nil, fmt.Errorf("target: resume %d carrier outside scope", index)
			}
		case ResumeSourceProduced:
			if item.Carrier != 0 {
				return nil, fmt.Errorf("target: produced resume %d carries ValueFormal", index)
			}
		default:
			return nil, fmt.Errorf("target: resume %d has invalid source", index)
		}
		arguments, argumentsErr := d.freezeValues(item.Arguments, false)
		if argumentsErr != nil {
			return nil, fmt.Errorf("target: resume %d arguments: %w", index, argumentsErr)
		}
		// A resume payload is precisely the open tail of this invocation after
		// its fixed carrier operands. It cannot name an unrelated local Values
		// variable: closed and unknown inputs have no exact payload relation.
		if d.input.tail != ValuesVariable ||
			arguments.tail != ValuesVariable || arguments.varID != d.input.varID ||
			len(arguments.types) != 0 || len(arguments.suffix) != 0 {
			return nil, fmt.Errorf("target: resume %d arguments are not the input Values tail", index)
		}
		if len(item.Outcomes) != 5 {
			return nil, fmt.Errorf("target: resume %d has incomplete cross-activation outcomes", index)
		}
		var outcomes [5]uint32
		seen := [5]bool{}
		for outcomeIndex, outcome := range item.Outcomes {
			resumeKind, ok := crossActivationOutcomeIndex(outcome.Kind)
			if !ok {
				return nil, fmt.Errorf("target: resume %d outcome %d has invalid cross-activation kind", index, outcomeIndex)
			}
			if seen[resumeKind] {
				return nil, fmt.Errorf("target: resume %d has duplicate cross-activation kind", index)
			}
			if uint64(outcome.Outcome) >= uint64(len(d.outcomes)) {
				return nil, fmt.Errorf("target: resume %d outcome %d outside scope", index, outcomeIndex)
			}
			// A restored outcome instantiates this existing tail, or a closed
			// outcome deliberately discards it. Unknown cannot express that exact
			// transport law and is therefore inadmissible at this boundary.
			if bySource[outcome.Outcome].values.tail == ValuesUnknown {
				return nil, fmt.Errorf("target: resume %d outcome %d has unknown Values tail", index, outcomeIndex)
			}
			seen[resumeKind] = true
			outcomes[resumeKind] = outcome.Outcome
		}
		for kind := range seen {
			if !seen[kind] {
				return nil, fmt.Errorf("target: resume %d has incomplete cross-activation outcomes", index)
			}
		}
		out[index] = resumeDraft{source: item.Source, carrier: item.Carrier, arguments: arguments, outcomes: outcomes}
	}
	return out, nil
}

func (d operationDraft) freezeTransfers(input []TransferSpec) ([]transferDraft, error) {
	if _, err := checkedStoredLength("transfer table", len(input)); err != nil {
		return nil, err
	}
	out := make([]transferDraft, len(input))
	for index, item := range input {
		if !validTransferEndpoint(item.Endpoint, d.valueFormalCount()) {
			return nil, fmt.Errorf("target: transfer %d has invalid endpoint", index)
		}
		if !validTransferInputSource(item.Payload, d) {
			return nil, fmt.Errorf("target: transfer %d payload outside scope", index)
		}
		if !validTransferInputSource(item.Alias, d) {
			return nil, fmt.Errorf("target: transfer %d alias outside scope", index)
		}
		if !validTransferIdentity(item.Identity) {
			return nil, fmt.Errorf("target: transfer %d has invalid identity relation", index)
		}
		if !validTransferCapabilities(item.Capabilities) {
			return nil, fmt.Errorf("target: transfer %d has invalid capability relation", index)
		}
		if len(item.Outcomes) != len(d.outcomes) {
			return nil, fmt.Errorf("target: transfer %d has incomplete outcome authority", index)
		}
		if _, err := checkedStoredLength("transfer outcome table", len(item.Outcomes)); err != nil {
			return nil, err
		}
		bySource := make([]TransferPossibility, len(d.outcomes))
		seen := make([]bool, len(d.outcomes))
		for outcomeIndex, outcome := range item.Outcomes {
			if uint64(outcome.Outcome) >= uint64(len(d.outcomes)) {
				return nil, fmt.Errorf("target: transfer %d outcome %d outside scope", index, outcomeIndex)
			}
			if seen[outcome.Outcome] {
				return nil, fmt.Errorf("target: transfer %d has duplicate outcome authority", index)
			}
			if !validTransferPossibility(outcome.Possibility) {
				return nil, fmt.Errorf("target: transfer %d outcome %d has invalid possibility", index, outcomeIndex)
			}
			seen[outcome.Outcome] = true
			bySource[outcome.Outcome] = outcome.Possibility
		}
		canonical := make([]TransferPossibility, len(d.outcomes))
		for canonicalOutcome, outcome := range d.outcomes {
			canonical[canonicalOutcome] = bySource[outcome.source]
		}
		out[index] = transferDraft{
			endpoint: item.Endpoint, payload: item.Payload, alias: item.Alias, identity: item.Identity,
			capabilities: item.Capabilities, outcomes: canonical,
		}
	}
	sort.Slice(out, func(left, right int) bool {
		return compareTransfer(out[left], out[right]) < 0
	})
	for index := 1; index < len(out); index++ {
		if compareTransferIdentity(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate transfer endpoint/payload/alias")
		}
	}
	return out, nil
}

func (d operationDraft) freezeCallbackResults(input []CallbackResultSpec, outcome valuesDraft, produced []producedDraft) ([]callbackResultDraft, error) {
	if _, err := checkedStoredLength("callback result table", len(input)); err != nil {
		return nil, err
	}
	out := make([]callbackResultDraft, len(input))
	for index, result := range input {
		if uint64(result.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("callback result %d is not a fixed outcome slot", index)
		}
		if result.Callback == 0 || uint64(result.Callback) > uint64(len(d.callbacks)) {
			return nil, fmt.Errorf("callback result %d callback outside scope", index)
		}
		callback, found := d.callbackBySource(int(result.Callback - 1))
		if !found || callback.function.Kind != InputSourceValueFormal || uint64(callback.function.Ordinal) >= uint64(len(d.input.types)) {
			return nil, fmt.Errorf("callback result %d callback source is malformed", index)
		}
		sourceType := d.input.types[callback.function.Ordinal]
		resultType := outcome.types[result.Result]
		sourceDeclaration, sourceOK := d.declarations[sourceType]
		resultDeclaration, resultOK := d.declarations[resultType]
		if !sourceOK || !resultOK {
			return nil, fmt.Errorf("callback result %d type relation: type declaration is not admitted", index)
		}
		assignable, relationErr := d.semantics.Assignable(sourceDeclaration, resultDeclaration, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("callback result %d type relation: %w", index, relationErr)
		}
		if !assignable {
			return nil, fmt.Errorf("callback result %d is type-incompatible with its callback", index)
		}
		sourceCallable, sourceErr := d.semantics.Callable(sourceDeclaration, callback.admission, d.formalConstraints)
		if sourceErr != nil {
			return nil, fmt.Errorf("callback result %d source callable relation: %w", index, sourceErr)
		}
		resultCallable, resultErr := d.semantics.Callable(resultDeclaration, callback.admission, d.formalConstraints)
		if resultErr != nil {
			return nil, fmt.Errorf("callback result %d result callable relation: %w", index, resultErr)
		}
		if !sourceCallable || !resultCallable {
			return nil, fmt.Errorf("callback result %d is not callable under its admission", index)
		}
		out[index] = callbackResultDraft{result: result.Result, callback: result.Callback}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate callback outcome result")
		}
	}
	producedIndex, callbackIndex := 0, 0
	for producedIndex < len(produced) && callbackIndex < len(out) {
		if produced[producedIndex].result < out[callbackIndex].result {
			producedIndex++
			continue
		}
		if produced[producedIndex].result > out[callbackIndex].result {
			callbackIndex++
			continue
		}
		return nil, errors.New("target: callback result overlaps produced result")
	}
	return out, nil
}

func (d operationDraft) callbackBySource(source int) (callbackDraft, bool) {
	for _, callback := range d.callbacks {
		if callback.source == source {
			return callback, true
		}
	}
	return callbackDraft{}, false
}

func (d operationDraft) freezeResultAliases(input []ResultAliasSpec, outcome valuesDraft) ([]resultAliasDraft, error) {
	if _, err := checkedStoredLength("result alias table", len(input)); err != nil {
		return nil, err
	}
	out := make([]resultAliasDraft, len(input))
	for index, alias := range input {
		if uint64(alias.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("result alias %d is not a fixed outcome slot", index)
		}
		if alias.Source.Kind != InputSourceValueFormal || uint64(alias.Source.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("result alias %d source is not a ValueFormal in scope", index)
		}
		sourceDeclaration, sourceOK := d.declarations[d.input.types[alias.Source.Ordinal]]
		resultDeclaration, resultOK := d.declarations[outcome.types[alias.Result]]
		if !sourceOK || !resultOK {
			return nil, fmt.Errorf("result alias %d type relation: type declaration is not admitted", index)
		}
		assignable, relationErr := d.semantics.Assignable(sourceDeclaration, resultDeclaration, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("result alias %d type relation: %w", index, relationErr)
		}
		if !assignable {
			return nil, fmt.Errorf("result alias %d is type-incompatible with its input", index)
		}
		out[index] = resultAliasDraft{result: alias.Result, source: alias.Source}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate result alias outcome result")
		}
	}
	return out, nil
}

func (d operationDraft) freezeFreshResults(input []FreshResultSpec, outcome valuesDraft, aliases []resultAliasDraft) ([]freshResultDraft, error) {
	if _, err := checkedStoredLength("fresh result table", len(input)); err != nil {
		return nil, err
	}
	out := make([]freshResultDraft, len(input))
	for index, fresh := range input {
		if uint64(fresh.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("fresh result %d is not a fixed outcome slot", index)
		}
		if !fresh.Kind.Available() {
			return nil, fmt.Errorf("fresh result %d has invalid kind", index)
		}
		declaration, declarationOK := d.declarations[outcome.types[fresh.Result]]
		if !declarationOK {
			return nil, fmt.Errorf("fresh result %d type relation: type declaration is not admitted", index)
		}
		compatible, relationErr := d.semantics.Fresh(declaration, fresh.Kind, d.formalConstraints)
		if relationErr != nil {
			return nil, fmt.Errorf("fresh result %d type relation: %w", index, relationErr)
		}
		if !compatible {
			return nil, fmt.Errorf("fresh result %d contradicts its runtime kind", index)
		}
		out[index] = freshResultDraft{result: fresh.Result, kind: fresh.Kind}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := range out {
		if index != 0 && out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate fresh outcome result")
		}
		out[index].ordinal = uint32(index)
	}
	aliasIndex, freshIndex := 0, 0
	for aliasIndex < len(aliases) && freshIndex < len(out) {
		if aliases[aliasIndex].result < out[freshIndex].result {
			aliasIndex++
			continue
		}
		if aliases[aliasIndex].result > out[freshIndex].result {
			freshIndex++
			continue
		}
		return nil, errors.New("target: fresh result overlaps result alias")
	}
	return out, nil
}
