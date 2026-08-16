package target

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Seal validates, freezes, and canonically orders one target contract. Spec
// is consumed on its first attempt, including a failing attempt.
func Seal(spec *Spec) (*Contract, error) {
	if spec == nil {
		return nil, errors.New("target: nil spec")
	}
	if spec.consumed {
		return nil, errors.New("target: consumed spec")
	}
	defer func() { *spec = Spec{consumed: true} }()

	operationCount, err := checkedStoredTotal("operation table", len(spec.Operations), 1)
	if err != nil {
		return nil, err
	}
	drafts := make([]operationDraft, len(spec.Operations))
	for index := range spec.Operations {
		draft, err := freezeOperation(index, spec.Operations[index])
		if err != nil {
			return nil, err
		}
		drafts[index] = draft
	}
	if err := canonicalizeBindings(drafts); err != nil {
		return nil, err
	}
	if err := deriveProducedAnchors(drafts); err != nil {
		return nil, err
	}
	sort.Slice(drafts, func(left, right int) bool {
		return compareOperationDraft(drafts[left], drafts[right]) < 0
	})
	for index := 1; index < len(drafts); index++ {
		if compareOperationDraft(drafts[index-1], drafts[index]) == 0 {
			return nil, errors.New("target: duplicate operation anchor")
		}
	}

	sourceOperation := make([]Operation, len(drafts))
	for index := range drafts {
		handle, handleErr := checkedStoredHandle("operation handle", index)
		if handleErr != nil {
			return nil, handleErr
		}
		sourceOperation[drafts[index].source] = Operation(handle)
	}
	for index := range drafts {
		if err := drafts[index].resolveEffects(drafts, sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveCallbackReleases(drafts, sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveProduced(sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveSuspensions(); err != nil {
			return nil, err
		}
	}
	if err := validateProducedResumes(drafts, sourceOperation); err != nil {
		return nil, err
	}
	if err := validateSpawnAuthority(drafts); err != nil {
		return nil, err
	}
	protocols, err := freezeProtocols(spec.Protocols)
	if err != nil {
		return nil, err
	}
	if err := resolveProtocols(protocols, drafts, sourceOperation); err != nil {
		return nil, err
	}
	boot, err := freezeBoot(spec.InitialRoots, spec.InitialEntries, spec.InitialBindings, spec.InitialMetatables, drafts, sourceOperation)
	if err != nil {
		return nil, err
	}
	if err := resolveSubedgeInitialReads(drafts, boot); err != nil {
		return nil, err
	}
	exactKeys, exactKeyHandles, err := freezeExactKeys(drafts, boot)
	if err != nil {
		return nil, err
	}

	// Contract is staging-only until the final return. Any failed append drops
	// it whole, so Seal never exposes a partially converted representation.
	contract := &Contract{operations: make([]operationRow, 0, operationCount)}
	if err := contract.appendExactKeys(exactKeys); err != nil {
		return nil, err
	}
	for index := range drafts {
		handle, handleErr := checkedStoredHandle("operation handle", index)
		if handleErr != nil {
			return nil, handleErr
		}
		op := Operation(handle)
		if err := contract.appendOperation(op, &drafts[index], exactKeyHandles); err != nil {
			return nil, err
		}
	}
	if err := resolveProtocolCallbackHolders(protocols, drafts); err != nil {
		return nil, err
	}
	if err := contract.appendCallbackReleases(drafts); err != nil {
		return nil, err
	}
	if err := contract.appendOpaque(); err != nil {
		return nil, err
	}
	if err := contract.appendProtocols(protocols); err != nil {
		return nil, err
	}
	if err := contract.appendBoot(boot, exactKeyHandles); err != nil {
		return nil, err
	}
	if err := contract.buildLookup(); err != nil {
		return nil, err
	}
	if err := contract.sealSemanticIdentities(); err != nil {
		return nil, err
	}
	// Content identity is available only after every immutable table and its
	// derived lookup authority has been completely assembled.
	contract.sealed = true
	semanticReceipt, receiptOK := buildTargetSemanticSourceReceipt(contract)
	if !receiptOK {
		return nil, errors.New("target: unavailable semantic-source receipt")
	}
	contract.semanticReceipt = semanticReceipt
	return contract, nil
}

type operationDraft struct {
	source      int
	bindings    []BindingSpec
	canonical   uint32
	formals     []*typ.TypeParam
	valuesVars  uint32
	valuesTypes []string
	rowFormals  uint32
	input       valuesDraft
	outcomes    []outcomeDraft
	callbacks   []callbackDraft
	subedges    []subedgeDraft
	suspensions []suspensionDraft
	spawns      []spawnDraft
	resumes     []resumeDraft
	transfers   []transferDraft
	gsubTable   *gsubTableReplacementDraft
	effects     []effectDraft
	effectTail  RowTail
	effectVar   RowVar
	types       map[string][]byte
	decoded     map[string]typ.Type
	assignable  map[typePair]bool
	constraints []string
}

type valuesDraft struct {
	types    []string
	tail     ValuesTail
	varID    ValuesVar
	tailType string
	suffix   []string
}

type outcomeDraft struct {
	source          int
	kind            flowkind.OutcomeKind
	values          valuesDraft
	anchor          string
	produced        []producedDraft
	fresh           []freshResultDraft
	callbackResults []callbackResultDraft
	resultAliases   []resultAliasDraft
}

type suspensionDraft struct {
	yield        uint32
	reentry      uint32
	source       ReentrySource
	multiplicity ReentryMultiplicity
}

type spawnDraft struct {
	function     InputSource
	child        CallbackRef
	yield        uint32
	parentResume uint32
	childEntry   uint32
	alternatives [2]SpawnSiblingAlternative
}

type resumeDraft struct {
	source    ResumeSource
	carrier   ValueFormal
	arguments valuesDraft
	outcomes  [5]uint32
}

type transferDraft struct {
	endpoint     TransferEndpoint
	payload      InputSource
	alias        InputSource
	identity     TransferIdentity
	capabilities TransferCapabilities
	outcomes     []TransferPossibility
}

type callbackResultDraft struct {
	result   uint32
	callback CallbackRef
}

type resultAliasDraft struct {
	result uint32
	source InputSource
}

type callbackDraft struct {
	source    int
	function  InputSource
	admission Admission
	arguments valuesDraft
	outcomes  [5]valuesDraft
	lifecycle CallbackLifecycle
	effects   rowDraft
	release   *callbackReleaseDraft
	sealed    CallbackID
}

// subedgeDraft keeps only normalized Target-local structure. Authoring refs
// are resolved after stable callback/subedge identity ordering; no Go call or
// recursive execution occurs while sealing a cyclic edge graph.
type subedgeDraft struct {
	source           int
	role             uint32
	family           SubedgeFamily
	callee           SubedgeCalleeKind
	callback         CallbackRef
	callbackRank     uint32
	readRoot         string
	readKey          keyspace.LiteralValue
	readRootID       InitialRoot
	metaKey          keyspace.LiteralValue
	admission        Admission
	arguments        valuesDraft
	ruleEntry        bool
	argumentOrigins  []subedgeArgumentOriginDraft
	outcomes         [5]valuesDraft
	admissionFailure valuesDraft
	admissionRoute   subedgeRouteDraft
	routes           [5]subedgeRouteDraft
	sealed           SubedgeID
}

type subedgeArgumentOriginDraft struct {
	segment ArgumentSegment
	index   uint32
	kind    ArgumentSource
	source  InputSource
}

type subedgeRouteDraft struct {
	route       SubedgeRoute
	adjustment  Adjustment
	result      valuesDraft
	placement   Placement
	offset      uint32
	outcome     uint32
	subedge     SubedgeRef
	subedgeRank uint32
	destination valuesDraft
}

type callbackReleaseDraft struct {
	operationSource SpecRef
	operation       Operation
	input           ValueFormal
	outcome         uint32
	mode            CallbackReleaseMode
	zeroBehavior    CallbackReleaseZeroBehavior
	zeroOutcome     uint32
}

type producedDraft struct {
	result       uint32
	targetSource SpecRef
	target       Operation
	captures     []CaptureSpec
}

const noTypeValueCapture = ^uint32(0)

type freshResultDraft struct {
	result  uint32
	ordinal uint32
	kind    FreshKind
}

type producedAnchorStep struct {
	outcome string
	result  uint32
}

type effectDraft struct {
	targetSource   SpecRef
	target         Operation
	values         []ValueFormal
	types          []TypeFormal
	valuesVar      []ValuesVar
	rows           []RowVar
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type rowDraft struct {
	effects  []effectDraft
	tail     RowTail
	variable RowVar
}

func freezeOperation(source int, input OperationSpec) (operationDraft, error) {
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
		source:      source,
		formals:     formals,
		valuesVars:  input.ValuesVars,
		rowFormals:  input.RowFormals,
		types:       make(map[string][]byte),
		decoded:     make(map[string]typ.Type),
		assignable:  make(map[typePair]bool),
		constraints: make([]string, len(formals)),
	}
	for index, formal := range formals {
		if formal.Constraint == nil {
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

func copyFormals(input []*typ.TypeParam) ([]*typ.TypeParam, error) {
	if len(input) == 0 {
		return nil, nil
	}
	out := append([]*typ.TypeParam(nil), input...)
	seen := make(map[*typ.TypeParam]struct{}, len(out))
	for index, formal := range out {
		if formal == nil {
			return nil, fmt.Errorf("target: nil type formal %d", index)
		}
		if _, duplicate := seen[formal]; duplicate {
			return nil, fmt.Errorf("target: duplicate type formal %d", index)
		}
		seen[formal] = struct{}{}
	}
	return out, nil
}

func (d *operationDraft) freezeType(value typ.Type) (string, error) {
	if err := validateAuthoringType(value); err != nil {
		return "", err
	}
	encoded, err := typ.EncodeCanonicalFormals(context.Background(), value, d.formals)
	if err != nil {
		return "", fmt.Errorf("target: nonportable type: %w", err)
	}
	if _, err := checkedStoredLength("type bytes", len(encoded)); err != nil {
		return "", err
	}
	key := string(encoded)
	if _, exists := d.types[key]; !exists {
		d.types[key] = append([]byte(nil), encoded...)
	}
	return key, nil
}

func (d *operationDraft) freezeValues(input ValuesSpec, opaque bool) (valuesDraft, error) {
	if !validValuesTail(input.Tail, input.Var, d.valuesVars, opaque) {
		return valuesDraft{}, errors.New("target: invalid Values tail")
	}
	out := valuesDraft{tail: input.Tail, varID: input.Var}
	if input.Tail != ValuesVariable {
		if input.TailType != nil {
			return valuesDraft{}, errors.New("target: Values tail type requires a ValuesVariable tail")
		}
	} else {
		tailType := input.TailType
		if tailType == nil {
			tailType = typ.Any
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
		fixed = make([]typ.Type, 0, len(input.Fixed)+len(suffix))
		fixed = append(fixed, input.Fixed...)
		fixed = append(fixed, suffix...)
		suffix = nil
	}
	if len(fixed) != 0 {
		out.types = make([]string, len(fixed))
	}
	for index, value := range fixed {
		if value == nil {
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
		if value == nil {
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
// no ValuesSpec directly constrains still carries the ABI default typ.Any.
func (d *operationDraft) sealValuesVarTypes() error {
	if d.valuesVars == 0 {
		d.valuesTypes = nil
		return nil
	}
	any, err := d.freezeType(typ.Any)
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
			classes[index] = any
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
		if !d.typeAssignable(sourceType, resultType) {
			return nil, fmt.Errorf("callback result %d is type-incompatible with its callback", index)
		}
		if !d.admitsAdmission(sourceType, callback.admission) || !d.admitsAdmission(resultType, callback.admission) {
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
		if !d.typeAssignable(d.input.types[alias.Source.Ordinal], outcome.types[alias.Result]) {
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
		if !validFreshKind(fresh.Kind) {
			return nil, fmt.Errorf("fresh result %d has invalid kind", index)
		}
		if !d.freshCompatible(outcome.types[fresh.Result], fresh.Kind) {
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

func freezeBindings(input []BindingSpec) ([]BindingSpec, error) {
	if _, err := checkedStoredLength("operation binding table", len(input)); err != nil {
		return nil, err
	}
	out := make([]BindingSpec, len(input))
	for index, binding := range input {
		if !validBinding(binding) {
			return nil, fmt.Errorf("target: invalid binding %d", index)
		}
		out[index] = cloneBinding(binding)
	}
	sort.Slice(out, func(left, right int) bool { return compareBinding(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareBinding(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate operation binding")
		}
	}
	return out, nil
}

func (d operationDraft) freezeCallbacks(input []CallbackSpec) ([]callbackDraft, error) {
	if _, err := checkedStoredLength("callback table", len(input)); err != nil {
		return nil, err
	}
	out := make([]callbackDraft, len(input))
	for index, callback := range input {
		if callback.Function.Kind != InputSourceValueFormal || uint64(callback.Function.Ordinal) >= uint64(d.valueFormalCount()) {
			return nil, fmt.Errorf("target: callback %d has invalid function source", index)
		}
		if !validAdmission(callback.Admission) {
			return nil, fmt.Errorf("target: callback %d has invalid admission", index)
		}
		arguments, argumentsErr := d.freezeValues(callback.Arguments, false)
		if argumentsErr != nil {
			return nil, fmt.Errorf("target: callback %d arguments: %w", index, argumentsErr)
		}
		if !validCallbackLifecycle(callback.Lifecycle) {
			return nil, fmt.Errorf("target: callback %d has invalid lifecycle", index)
		}
		effects, effectErr := d.freezeRow(callback.Effects, "callback expected")
		if effectErr != nil {
			return nil, fmt.Errorf("target: callback %d effects: %w", index, effectErr)
		}
		var release *callbackReleaseDraft
		if callback.Release != nil {
			if !retainedCallbackLifecycle(callback.Lifecycle) {
				return nil, fmt.Errorf("target: callback %d release requires retained lifecycle", index)
			}
			if callback.Release.Operation == 0 {
				return nil, fmt.Errorf("target: callback %d release has invalid operation", index)
			}
			if !validCallbackReleaseMode(callback.Release.Mode) {
				return nil, fmt.Errorf("target: callback %d release has invalid mode", index)
			}
			if !validCallbackReleaseZeroBehavior(callback.Release.Zero.Behavior) {
				return nil, fmt.Errorf("target: callback %d release has invalid zero behavior", index)
			}
			if callback.Release.Zero.Behavior == CallbackReleaseZeroSuppress && callback.Release.Zero.Outcome != 0 {
				return nil, fmt.Errorf("target: callback %d suppressed zero release has an outcome", index)
			}
			release = &callbackReleaseDraft{
				operationSource: callback.Release.Operation,
				input:           callback.Release.Input,
				outcome:         callback.Release.Outcome,
				mode:            callback.Release.Mode,
				zeroBehavior:    callback.Release.Zero.Behavior,
				zeroOutcome:     callback.Release.Zero.Outcome,
			}
		}
		if len(callback.Outcomes) != 5 {
			return nil, fmt.Errorf("target: callback %d has incomplete activation outcomes", index)
		}
		var outcomes [5]valuesDraft
		seen := [5]bool{}
		for outcomeIndex, outcome := range callback.Outcomes {
			kind, ok := crossActivationOutcomeIndex(outcome.Kind)
			if !ok {
				return nil, fmt.Errorf("target: callback %d outcome %d has invalid activation kind", index, outcomeIndex)
			}
			if seen[kind] {
				return nil, fmt.Errorf("target: callback %d has duplicate activation kind", index)
			}
			values, valuesErr := d.freezeValues(outcome.Values, false)
			if valuesErr != nil {
				return nil, fmt.Errorf("target: callback %d outcome %d: %w", index, outcomeIndex, valuesErr)
			}
			seen[kind] = true
			outcomes[kind] = values
		}
		out[index] = callbackDraft{
			source: index, function: callback.Function, admission: callback.Admission,
			arguments: arguments, outcomes: outcomes, lifecycle: callback.Lifecycle,
			effects: effects, release: release,
		}
	}
	sort.Slice(out, func(left, right int) bool { return compareCallback(out[left], out[right]) < 0 })
	for index := 1; index < len(out); index++ {
		if compareCallbackIdentity(out[index-1], out[index]) == 0 {
			return nil, errors.New("target: duplicate callback")
		}
	}
	return out, nil
}

func (d operationDraft) freezeProduced(input []ProducedSpec, outcome valuesDraft) ([]producedDraft, error) {
	if _, err := checkedStoredLength("produced operation table", len(input)); err != nil {
		return nil, err
	}
	out := make([]producedDraft, len(input))
	for index, produced := range input {
		if produced.Operation == 0 {
			return nil, fmt.Errorf("produced operation %d has invalid target", index)
		}
		if uint64(produced.Result) >= uint64(len(outcome.types)) {
			return nil, fmt.Errorf("produced operation %d result is not a fixed outcome slot", index)
		}
		if !d.admitsAdmission(outcome.types[produced.Result], DirectFunction) {
			return nil, fmt.Errorf("produced operation %d result is not a direct function", index)
		}
		if _, err := checkedStoredLength("produced capture table", len(produced.Captures)); err != nil {
			return nil, err
		}
		captures := append([]CaptureSpec(nil), produced.Captures...)
		typeValueCaptures := 0
		for captureIndex, capture := range captures {
			switch capture.Kind {
			case CaptureValueFormal:
				if uint64(capture.Ordinal) >= uint64(d.valueFormalCount()) {
					return nil, fmt.Errorf("produced operation %d capture %d ValueFormal outside scope", index, captureIndex)
				}
			case CaptureTypeValueFormal:
				if uint64(capture.Ordinal) >= uint64(d.valueFormalCount()) {
					return nil, fmt.Errorf("produced operation %d capture %d TypeValueFormal outside scope", index, captureIndex)
				}
				typeValueCaptures++
				if typeValueCaptures > 1 {
					return nil, fmt.Errorf("produced operation %d has more than one TypeValueFormal capture", index)
				}
			case CaptureValuesVar:
				if uint64(capture.Ordinal) >= uint64(d.valuesVars) {
					return nil, fmt.Errorf("produced operation %d capture %d ValuesVar outside scope", index, captureIndex)
				}
			case CaptureCallback:
				if capture.Ordinal == 0 || uint64(capture.Ordinal) > uint64(len(d.callbacks)) {
					return nil, fmt.Errorf("produced operation %d capture %d callback outside scope", index, captureIndex)
				}
			default:
				return nil, fmt.Errorf("produced operation %d capture %d invalid source", index, captureIndex)
			}
		}
		out[index] = producedDraft{result: produced.Result, targetSource: produced.Operation, captures: captures}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].result < out[right].result })
	for index := 1; index < len(out); index++ {
		if out[index-1].result == out[index].result {
			return nil, errors.New("target: duplicate produced outcome result")
		}
	}
	return out, nil
}

// validateProducedTypeValueFreshResults closes the structural identity law
// for retained runtime TypeValues. A TypeValue capture describes the identity
// of its Produced callable result, so that exact fixed result must also be the
// outcome's nominal FreshFunction. Both inputs are already canonical and
// unique by result; a linear merge proves the correspondence without names,
// inferred ordinals, or a Produced×FreshResult product.
func validateProducedTypeValueFreshResults(produced []producedDraft, fresh []freshResultDraft) error {
	freshIndex := 0
	for producedIndex, row := range produced {
		typeValue := false
		for _, capture := range row.captures {
			if capture.Kind == CaptureTypeValueFormal {
				typeValue = true
				break
			}
		}
		if !typeValue {
			continue
		}
		for freshIndex < len(fresh) && fresh[freshIndex].result < row.result {
			freshIndex++
		}
		if freshIndex >= len(fresh) || fresh[freshIndex].result != row.result {
			return fmt.Errorf("produced operation %d TypeValue capture result %d lacks FreshFunction", producedIndex, row.result)
		}
		if fresh[freshIndex].kind != FreshFunction {
			return fmt.Errorf("produced operation %d TypeValue capture result %d has fresh kind %d, want FreshFunction", producedIndex, row.result, fresh[freshIndex].kind)
		}
	}
	return nil
}

func (d *operationDraft) freezeEffects(input RowSpec) error {
	row, err := d.freezeRow(input, "ordinary")
	if err != nil {
		return err
	}
	d.effects, d.effectTail, d.effectVar = row.effects, row.tail, row.variable
	return nil
}

func (d operationDraft) freezeRow(input RowSpec, owner string) (rowDraft, error) {
	if input.Tail != RowClosed && input.Tail != RowVariable {
		return rowDraft{}, fmt.Errorf("target: %s row has invalid tail", owner)
	}
	if input.Tail == RowVariable {
		if uint64(input.Var) >= uint64(d.rowFormals) {
			return rowDraft{}, errors.New("target: row variable outside operation scope")
		}
	} else if input.Var != 0 {
		return rowDraft{}, errors.New("target: closed row carries variable")
	}
	out := rowDraft{tail: input.Tail, variable: input.Var}
	if _, err := checkedStoredLength("effect table", len(input.Occurrences)); err != nil {
		return rowDraft{}, err
	}
	if len(input.Occurrences) == 0 {
		return out, nil
	}
	out.effects = make([]effectDraft, len(input.Occurrences))
	valueCount := uint64(len(d.input.types))
	for index, item := range input.Occurrences {
		if item.Target == 0 {
			return rowDraft{}, fmt.Errorf("target: effect %d has invalid target", index)
		}
		draft := effectDraft{targetSource: item.Target}
		if item.Publication != nil {
			publication, publicationErr := freezePublicationEffect(*item.Publication)
			if publicationErr != nil {
				return rowDraft{}, fmt.Errorf("target: %s effect %d publication: %w", owner, index, publicationErr)
			}
			draft.publication, draft.hasPublication = publication, true
		}
		if _, err := checkedStoredLength("effect value argument pool", len(item.ValueArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := checkedStoredLength("effect type argument pool", len(item.TypeArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := checkedStoredLength("effect Values argument pool", len(item.ValuesArgs)); err != nil {
			return rowDraft{}, err
		}
		if _, err := checkedStoredLength("effect row argument pool", len(item.RowArgs)); err != nil {
			return rowDraft{}, err
		}
		draft.values = append([]ValueFormal(nil), item.ValueArgs...)
		draft.types = append([]TypeFormal(nil), item.TypeArgs...)
		draft.valuesVar = append([]ValuesVar(nil), item.ValuesArgs...)
		draft.rows = append([]RowVar(nil), item.RowArgs...)
		for _, value := range draft.values {
			if uint64(value) >= valueCount {
				return rowDraft{}, fmt.Errorf("target: effect %d has value argument outside scope", index)
			}
		}
		for _, formal := range draft.types {
			if uint64(formal) >= uint64(len(d.formals)) {
				return rowDraft{}, fmt.Errorf("target: effect %d has type argument outside scope", index)
			}
		}
		for _, variable := range draft.valuesVar {
			if uint64(variable) >= uint64(d.valuesVars) {
				return rowDraft{}, fmt.Errorf("target: effect %d has Values argument outside scope", index)
			}
		}
		for _, variable := range draft.rows {
			if uint64(variable) >= uint64(d.rowFormals) {
				return rowDraft{}, fmt.Errorf("target: effect %d has row argument outside scope", index)
			}
		}
		out.effects[index] = draft
	}
	return out, nil
}

func freezePublicationEffect(input PublicationEffectSpec) (PublicationEffectDescriptor, error) {
	descriptor := PublicationEffectDescriptor{
		kind: input.Kind, subject: input.Subject, destination: input.Destination,
		context: input.Context, escape: input.Escape, mutability: input.Mutability, lifetime: input.Lifetime,
	}
	if descriptor.destination != PublicationDestinationNone && descriptor.destination != PublicationDestinationValueFormal {
		return PublicationEffectDescriptor{}, errors.New("invalid destination role")
	}
	if descriptor.destination == PublicationDestinationNone && descriptor.context != 0 {
		return PublicationEffectDescriptor{}, errors.New("destination-free publication carries context formal")
	}
	if !descriptor.validConsequences() {
		return PublicationEffectDescriptor{}, errors.New("kind and typed consequences disagree")
	}
	return descriptor, nil
}

func (d PublicationEffectDescriptor) validConsequences() bool {
	switch d.kind {
	case PublicationEffectSendTransfer:
		return d.destination == PublicationDestinationValueFormal &&
			d.escape == PublicationEscapeSendTransfer &&
			(d.mutability == PublicationMutabilityPreserve || d.mutability == PublicationMutabilityCopyOnWrite) &&
			d.lifetime == PublicationLifetimePreserve
	case PublicationEffectReturnEscape:
		return d.destination == PublicationDestinationNone && d.escape == PublicationEscapeReturn &&
			d.mutability == PublicationMutabilityPreserve && d.lifetime == PublicationLifetimePreserve
	case PublicationEffectCallbackEscape:
		return d.destination == PublicationDestinationNone && d.escape == PublicationEscapeCallback &&
			d.mutability == PublicationMutabilityPreserve && d.lifetime == PublicationLifetimePreserve
	case PublicationEffectFreezeSeal:
		return d.destination == PublicationDestinationNone && d.escape == PublicationEscapeNone &&
			d.mutability == PublicationMutabilitySeal && d.lifetime == PublicationLifetimePreserve
	case PublicationEffectWriteMutation:
		return d.destination == PublicationDestinationNone && d.escape == PublicationEscapeNone &&
			(d.mutability == PublicationMutabilityWrite || d.mutability == PublicationMutabilityCopyOnWrite) &&
			d.lifetime == PublicationLifetimePreserve
	case PublicationEffectCloseRelease:
		return d.destination == PublicationDestinationNone && d.escape == PublicationEscapeNone &&
			d.mutability == PublicationMutabilityPreserve && d.lifetime == PublicationLifetimeRelease
	default:
		return false
	}
}

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
