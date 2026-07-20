package callpayload

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type callOutcomeEvaluator func(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	input CallOutcomeInput,
) (CallOutcome, error)

type callOutcomeShape func(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSiteShape, error)

type callOutcomeReadShape func(ctx transfer.NodeContext, site factflow.CallSiteView, point cfg.Point) (state.LaneSet, error)

type callOutcomeSitePreparer func(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSiteProgram, error)

// CallOutcomeSiteEvaluator is the execution half of one immutable lexical
// call-site specialization. All site-only lookup and planning belongs in the
// preparation that produces this evaluator, never in fixed-point execution.
type CallOutcomeSiteEvaluator func(ctx transfer.NodeContext, input CallOutcomeInput) (CallOutcome, error)

// CallOutcomeSitePreparation binds one provider to a lexical call site in one
// operation. Shape and execution are deliberately prepared together so they
// cannot repeat signature lookup, structural planning, or capability
// selection through independent callbacks.
type CallOutcomeSitePreparation struct {
	Shape    CallOutcomeSiteShape
	Evaluate CallOutcomeSiteEvaluator
}

// CallOutcomeSitePrepareFunc constructs one immutable lexical specialization.
type CallOutcomeSitePrepareFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSitePreparation, error)

// CallOutcomeProgram is the canonical external-call semantic program. It owns
// both execution and the exhaustive upper bound of CallOutcome fields that
// execution can produce. The zero value is the absent program.
type CallOutcomeProgram struct {
	evaluate callOutcomeEvaluator
	maximum  []CallOutcomeFieldRole
	primary  state.LaneSet
	history  state.LaneSet
	shape    callOutcomeShape
	read     callOutcomeReadShape
	prepare  callOutcomeSitePreparer
}

// CallOutcomeSiteProgram is one fully validated, immutable site specialization.
// Site-shape work is paid once while freezing the caller; Evaluate is the sole
// runtime provider entry and never rebuilds the capability.
type CallOutcomeSiteProgram struct {
	capability CallOutcomeCapability
	evaluate   func(transfer.NodeContext, CallOutcomeInput) (CallOutcome, error)
}

// CallOutcomeCapability is one program's immutable, site-specialized output
// shape. ResultIndices contains exactly the result slots observed by the
// canonical call transaction at this site; provider results outside that
// inventory are semantically unobservable.
type CallOutcomeCapability struct {
	roles                    []CallOutcomeFieldRole
	correlations             []CallOutcomeCorrelationShape
	resultIndices            []int
	primaryInputLanes        state.LaneSet
	typestateResourceQueries []state.TypestateResourceQuery
	readInputLanes           func(cfg.Point) (state.LaneSet, error)
}

// SealCallOutcomeProgram binds an evaluator to its exhaustive possible field
// roles. Unknown and duplicate fields are programmer errors: a provider may
// neither invent a lane nor ambiguously declare one.
func SealCallOutcomeProgram(
	owner string,
	fieldNames []string,
	primaryInputLanes state.LaneSet,
	historicalInputLanes state.LaneSet,
	shape callOutcomeShape,
	readShape callOutcomeReadShape,
	evaluate callOutcomeEvaluator,
) CallOutcomeProgram {
	if evaluate == nil {
		panic(fmt.Sprintf("callpayload: %s has nil call-outcome evaluator", programOwner(owner)))
	}
	if err := state.DefaultLaneCatalog().ValidateLaneSet(primaryInputLanes); err != nil {
		panic(fmt.Sprintf("callpayload: %s has invalid primary input lanes: %v", programOwner(owner), err))
	}
	if err := state.DefaultLaneCatalog().ValidateLaneSet(historicalInputLanes); err != nil {
		panic(fmt.Sprintf("callpayload: %s has invalid historical input lanes: %v", programOwner(owner), err))
	}
	roles := CallOutcomeFieldRoles()
	byName := make(map[string]CallOutcomeFieldRole, len(roles))
	for _, role := range roles {
		byName[role.FieldName] = role
	}
	seen := make(map[string]struct{}, len(fieldNames))
	selected := make([]CallOutcomeFieldRole, 0, len(fieldNames))
	for _, name := range fieldNames {
		role, ok := byName[name]
		if !ok {
			panic(fmt.Sprintf("callpayload: %s declares unknown CallOutcome field %q", programOwner(owner), name))
		}
		if _, duplicate := seen[name]; duplicate {
			panic(fmt.Sprintf("callpayload: %s declares duplicate CallOutcome field %q", programOwner(owner), name))
		}
		seen[name] = struct{}{}
		selected = append(selected, role)
	}
	// Store roles in the canonical CallOutcome catalog order, independent of
	// constructor spelling, so composition and fingerprints are deterministic.
	sort.Slice(selected, func(i, j int) bool {
		return callOutcomeRoleOrdinal(roles, selected[i].FieldName) < callOutcomeRoleOrdinal(roles, selected[j].FieldName)
	})
	return CallOutcomeProgram{
		evaluate: evaluate,
		maximum:  selected,
		primary:  state.NewLaneSet(primaryInputLanes.IDs()...),
		history:  state.NewLaneSet(historicalInputLanes.IDs()...),
		shape:    shape,
		read:     readShape,
	}
}

// SealPreparedCallOutcomeProgram seals a provider whose immutable lexical
// work must be performed together with site-shape selection. The returned
// program has the same validation and composition laws as
// SealCallOutcomeProgram; only its lifecycle is stricter: Prepare is invoked
// once per frozen site and Evaluate contains dynamic semantic work only.
func SealPreparedCallOutcomeProgram(
	owner string,
	fieldNames []string,
	primaryInputLanes state.LaneSet,
	historicalInputLanes state.LaneSet,
	prepare CallOutcomeSitePrepareFunc,
) CallOutcomeProgram {
	if prepare == nil {
		panic(fmt.Sprintf("callpayload: %s has nil call-outcome site preparer", programOwner(owner)))
	}
	program := SealCallOutcomeProgram(
		owner, fieldNames, primaryInputLanes, historicalInputLanes,
		nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			panic("callpayload: prepared call-outcome evaluator was not bound")
		},
	)
	program.evaluate = nil
	program.prepare = func(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSiteProgram, error) {
		prepared, err := prepare(ctx, site)
		if err != nil {
			return CallOutcomeSiteProgram{}, err
		}
		shaped := program
		shaped.shape = func(transfer.NodeContext, factflow.CallSiteView) (CallOutcomeSiteShape, error) {
			return prepared.Shape, nil
		}
		capability, err := shaped.prepareCapability(ctx, site)
		if err != nil {
			return CallOutcomeSiteProgram{}, err
		}
		return CallOutcomeSiteProgram{capability: capability, evaluate: prepared.Evaluate}, nil
	}
	return program
}

// ComposeCallOutcomePrograms composes programs in order and derives the
// possible-output shape solely as their set union. Callers cannot redeclare or
// widen the composed capability.
func ComposeCallOutcomePrograms(programs []CallOutcomeProgram, merge func(transfer.NodeContext, CallOutcome, CallOutcome) CallOutcome) CallOutcomeProgram {
	compact := make([]CallOutcomeProgram, 0, len(programs))
	for _, program := range programs {
		if !program.Empty() {
			compact = append(compact, program)
		}
	}
	switch len(compact) {
	case 0:
		return CallOutcomeProgram{}
	case 1:
		return compact[0]
	}
	if merge == nil {
		panic("callpayload: composed call-outcome program has nil merge law")
	}
	allRoles := CallOutcomeFieldRoles()
	present := make(map[string]struct{}, len(allRoles))
	for _, program := range compact {
		for _, role := range program.maximum {
			present[role.FieldName] = struct{}{}
		}
	}
	maximumPrimary := state.LaneSet{}
	maximumHistory := state.LaneSet{}
	for _, program := range compact {
		maximumPrimary = maximumPrimary.With(program.primary.IDs()...)
		maximumHistory = maximumHistory.With(program.history.IDs()...)
	}
	prepare := func(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSiteProgram, error) {
		children := make([]CallOutcomeSiteProgram, len(compact))
		for index, program := range compact {
			child, err := program.PrepareSite(ctx, site)
			if err != nil {
				return CallOutcomeSiteProgram{}, err
			}
			children[index] = child
		}
		capability, err := composeCallOutcomeCapabilities(children)
		if err != nil {
			return CallOutcomeSiteProgram{}, err
		}
		return CallOutcomeSiteProgram{
			capability: capability,
			evaluate: func(evalCtx transfer.NodeContext, input CallOutcomeInput) (CallOutcome, error) {
				out, err := children[0].Evaluate(evalCtx, input)
				if err != nil {
					return CallOutcome{}, err
				}
				for _, child := range children[1:] {
					next, err := child.Evaluate(evalCtx, input)
					if err != nil {
						return CallOutcome{}, err
					}
					out = merge(evalCtx, out, next)
				}
				return out, nil
			},
		}, nil
	}
	return CallOutcomeProgram{
		maximum: selectedCallOutcomeRoles(allRoles, present),
		primary: maximumPrimary,
		history: maximumHistory,
		prepare: prepare,
	}
}

// Empty reports whether no semantic program is present.
func (p CallOutcomeProgram) Empty() bool { return p.evaluate == nil && p.prepare == nil }

// PrepareSite validates and binds the exact capability once for one lexical
// call site. The absent program produces an inert site program.
func (p CallOutcomeProgram) PrepareSite(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeSiteProgram, error) {
	if p.prepare != nil {
		return p.prepare(ctx, site)
	}
	if p.evaluate == nil {
		return CallOutcomeSiteProgram{}, nil
	}
	capability, err := p.prepareCapability(ctx, site)
	if err != nil {
		return CallOutcomeSiteProgram{}, err
	}
	return CallOutcomeSiteProgram{
		capability: capability,
		evaluate: func(evalCtx transfer.NodeContext, input CallOutcomeInput) (CallOutcome, error) {
			return p.evaluate(evalCtx, site, input)
		},
	}, nil
}

// Capability returns the already-validated immutable site capability.
func (p CallOutcomeSiteProgram) Capability() CallOutcomeCapability { return p.capability }

// Owner returns the immutable provider identity retained through site preparation.
// Evaluate executes the bound provider and validates its runtime output.
func (p CallOutcomeSiteProgram) Evaluate(ctx transfer.NodeContext, input CallOutcomeInput) (CallOutcome, error) {
	if p.evaluate == nil {
		return CallOutcome{}, nil
	}
	out, err := p.evaluate(ctx, input)
	if err != nil {
		return CallOutcome{}, err
	}
	for _, lane := range callOutcomeLanes {
		if lane.has(out) && !p.capability.HasField(lane.fieldName) {
			return CallOutcome{}, fmt.Errorf("callpayload: evaluator emitted undeclared CallOutcome field %q", lane.fieldName)
		}
	}
	if err := validateOutcomeCorrelations(p.capability, out); err != nil {
		return CallOutcome{}, err
	}
	return out, nil
}

func (p CallOutcomeProgram) prepareCapability(ctx transfer.NodeContext, site factflow.CallSiteView) (CallOutcomeCapability, error) {
	roles := append([]CallOutcomeFieldRole(nil), p.maximum...)
	primaryInputLanes := state.NewLaneSet(p.primary.IDs()...)
	var correlations []CallOutcomeCorrelationShape
	var typestateResourceQueries []state.TypestateResourceQuery
	if p.shape != nil {
		shape, err := p.shape(ctx, site)
		if err != nil {
			return CallOutcomeCapability{}, err
		}
		names, selectedInputs := shape.FieldNames, shape.InputLanes
		present := make(map[string]struct{}, len(names))
		for _, name := range names {
			if _, duplicate := present[name]; duplicate {
				return CallOutcomeCapability{}, fmt.Errorf("callpayload: call-outcome shape declares duplicate field %q", name)
			}
			present[name] = struct{}{}
			if !hasCallOutcomeRole(p.maximum, name) {
				return CallOutcomeCapability{}, fmt.Errorf("callpayload: call-outcome shape field %q exceeds sealed maximum", name)
			}
		}
		selected := roles[:0]
		for _, role := range roles {
			if _, ok := present[role.FieldName]; ok {
				selected = append(selected, role)
			}
		}
		roles = selected
		for _, lane := range selectedInputs.IDs() {
			if !p.primary.Has(lane) {
				return CallOutcomeCapability{}, fmt.Errorf("callpayload: call-outcome shape input lane %q exceeds sealed maximum", lane)
			}
		}
		primaryInputLanes = state.NewLaneSet(selectedInputs.IDs()...)
		typestateResourceQueries, err = canonicalTypestateResourceQueries(shape.TypestateResourceQueries)
		if err != nil {
			return CallOutcomeCapability{}, err
		}
		for _, query := range typestateResourceQueries {
			for _, lane := range query.SourceLanes() {
				if !p.primary.Has(lane.ID()) {
					return CallOutcomeCapability{}, fmt.Errorf("callpayload: typestate resource query source lane %q exceeds sealed maximum", lane.ID())
				}
			}
		}
		correlations, err = canonicalCorrelationShapes(roles, shape.Correlations)
		if err != nil {
			return CallOutcomeCapability{}, err
		}
	}
	readInputLanes := func(point cfg.Point) (state.LaneSet, error) {
		if p.read == nil {
			return state.NewLaneSet(p.history.IDs()...), nil
		}
		selected, err := p.read(ctx, site, point)
		if err != nil {
			return state.LaneSet{}, err
		}
		for _, lane := range selected.IDs() {
			if !p.history.Has(lane) {
				return state.LaneSet{}, fmt.Errorf("callpayload: call-outcome historical input lane %q exceeds sealed maximum", lane)
			}
		}
		return state.NewLaneSet(selected.IDs()...), nil
	}
	if !hasCallOutcomeRole(roles, "Results") && len(correlations) == 0 {
		return CallOutcomeCapability{roles: roles, correlations: correlations, primaryInputLanes: primaryInputLanes, typestateResourceQueries: typestateResourceQueries, readInputLanes: readInputLanes}, nil
	}
	indices := make([]int, 0, site.ResultTargetCount())
	seen := make(map[int]struct{}, site.ResultTargetCount())
	if site.Context() == factflow.CallSiteContextCondition {
		seen[0] = struct{}{}
		indices = append(indices, 0)
	}
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		index := target.ResultIndex()
		if index < 0 {
			return true
		}
		if _, ok := seen[index]; !ok {
			seen[index] = struct{}{}
			indices = append(indices, index)
		}
		return true
	})
	sort.Ints(indices)
	containsResult := func(index int) bool {
		position := sort.SearchInts(indices, index)
		return position < len(indices) && indices[position] == index
	}
	for _, correlation := range correlations {
		if !containsResult(correlation.ReturnIndex) {
			return CallOutcomeCapability{}, fmt.Errorf("callpayload: correlation trigger result %d is outside site inventory", correlation.ReturnIndex)
		}
		switch correlation.Kind {
		case CallOutcomeReturnConditionPath:
			if !correlation.Target.IsPlaceholder() || correlation.Target.PlaceholderIndex() >= site.ArgumentSourceCount() {
				return CallOutcomeCapability{}, fmt.Errorf("callpayload: condition-refinement target is outside site parameter inventory")
			}
		case CallOutcomeReturnConditionSlot, CallOutcomeReturnPresence:
			if !containsResult(correlation.TargetIndex) {
				return CallOutcomeCapability{}, fmt.Errorf("callpayload: correlation target result %d is outside site inventory", correlation.TargetIndex)
			}
		}
	}
	return CallOutcomeCapability{roles: roles, correlations: correlations, resultIndices: indices, primaryInputLanes: primaryInputLanes, typestateResourceQueries: typestateResourceQueries, readInputLanes: readInputLanes}, nil
}

// FieldRoles returns a detached copy in canonical CallOutcome field order.
func (c CallOutcomeCapability) FieldRoles() []CallOutcomeFieldRole {
	return append([]CallOutcomeFieldRole(nil), c.roles...)
}

// OperandValueWrites reports whether any selected outcome field can rewrite
// caller operand slots. The answer is derived solely from the canonical field
// descriptors retained by this site capability.
func (c CallOutcomeCapability) OperandValueWrites() bool {
	for _, role := range c.roles {
		if role.transaction != nil && role.transaction.operandValueWrites {
			return true
		}
	}
	return false
}

// TransactionLanes returns the exact residual State lanes written by the
// selected outcome fields. The returned set is detached and preserves the
// canonical descriptor/lane order.
func (c CallOutcomeCapability) TransactionLanes() state.LaneSet {
	lanes := state.NewLaneSet()
	for _, role := range c.roles {
		if role.transaction != nil {
			lanes = lanes.With(role.transaction.lanes.IDs()...)
		}
	}
	return lanes
}

func (c CallOutcomeCapability) CorrelationShapes() []CallOutcomeCorrelationShape {
	out := append([]CallOutcomeCorrelationShape(nil), c.correlations...)
	for index := range out {
		out[index].Target = out[index].Target.Clone()
	}
	return out
}

// ResultIndices returns a detached, sorted copy of observable result slots.
func (c CallOutcomeCapability) ResultIndices() []int {
	return append([]int(nil), c.resultIndices...)
}

// PrimaryInputLanes returns the exact residual State lanes observed from the
// call transfer's primary input. Value slots are represented separately by the
// frozen term footprint.
func (c CallOutcomeCapability) PrimaryInputLanes() state.LaneSet {
	return state.NewLaneSet(c.primaryInputLanes.IDs()...)
}

// TypestateResourceQueries returns the exact site-prepared keyed lifecycle
// observations. Their source lanes are compilation authority, not raw provider
// inputs.
func (c CallOutcomeCapability) TypestateResourceQueries() []state.TypestateResourceQuery {
	return append([]state.TypestateResourceQuery(nil), c.typestateResourceQueries...)
}

// ReadInputLanes returns the exact residual lanes observed through read(point).
func (c CallOutcomeCapability) ReadInputLanes(point cfg.Point) (state.LaneSet, error) {
	if c.readInputLanes == nil {
		return state.LaneSet{}, nil
	}
	return c.readInputLanes(point)
}

func composeCallOutcomeCapabilities(programs []CallOutcomeSiteProgram) (CallOutcomeCapability, error) {
	allRoles := CallOutcomeFieldRoles()
	present := make(map[string]struct{}, len(allRoles))
	primary := state.LaneSet{}
	var correlations []CallOutcomeCorrelationShape
	var indices []int
	var typestateResourceQueries []state.TypestateResourceQuery
	for _, program := range programs {
		capability := program.capability
		for _, role := range capability.roles {
			present[role.FieldName] = struct{}{}
		}
		primary = primary.With(capability.primaryInputLanes.IDs()...)
		correlations = append(correlations, capability.correlations...)
		indices = append(indices, capability.resultIndices...)
		typestateResourceQueries = append(typestateResourceQueries, capability.typestateResourceQueries...)
	}
	roles := selectedCallOutcomeRoles(allRoles, present)
	var err error
	correlations, err = canonicalCorrelationShapes(roles, correlations)
	if err != nil {
		return CallOutcomeCapability{}, err
	}
	sort.Ints(indices)
	write := 0
	for _, index := range indices {
		if write == 0 || indices[write-1] != index {
			indices[write], write = index, write+1
		}
	}
	indices = indices[:write]
	typestateResourceQueries, err = canonicalTypestateResourceQueries(typestateResourceQueries)
	if err != nil {
		return CallOutcomeCapability{}, err
	}
	read := func(point cfg.Point) (state.LaneSet, error) {
		lanes := state.LaneSet{}
		for _, program := range programs {
			selected, err := program.capability.ReadInputLanes(point)
			if err != nil {
				return state.LaneSet{}, err
			}
			lanes = lanes.With(selected.IDs()...)
		}
		return lanes, nil
	}
	return CallOutcomeCapability{
		roles: roles, correlations: correlations, resultIndices: indices,
		primaryInputLanes: primary, typestateResourceQueries: typestateResourceQueries, readInputLanes: read,
	}, nil
}

func selectedCallOutcomeRoles(all []CallOutcomeFieldRole, present map[string]struct{}) []CallOutcomeFieldRole {
	selected := make([]CallOutcomeFieldRole, 0, len(present))
	for _, role := range all {
		if _, ok := present[role.FieldName]; ok {
			selected = append(selected, role)
		}
	}
	return selected
}

// HasField reports whether the program may produce the named field.
func (c CallOutcomeCapability) HasField(fieldName string) bool {
	return hasCallOutcomeRole(c.roles, fieldName)
}

func hasCallOutcomeRole(roles []CallOutcomeFieldRole, fieldName string) bool {
	for _, role := range roles {
		if role.FieldName == fieldName {
			return true
		}
	}
	return false
}

func callOutcomeRoleOrdinal(roles []CallOutcomeFieldRole, fieldName string) int {
	for index, role := range roles {
		if role.FieldName == fieldName {
			return index
		}
	}
	return len(roles)
}

func programOwner(owner string) string {
	if owner == "" {
		return "call-outcome program"
	}
	return owner
}
