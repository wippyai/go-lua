package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

// FactorIncidence is one canonical Factor dependency derived from the sealed
// Rule schemas. Read contributes to Write; it neither authorizes an execution
// edge nor identifies a runtime Point or schedule.
type FactorIncidence struct {
	Read  SemanticKey
	Write SemanticKey
}

// FactorComponent is one canonical strongly connected component of the
// sealed Factor-incidence graph. Successors names the canonical first Factor
// of each distinct successor component. It is build/admission metadata only,
// not an execution order or fixed-point partition.
type FactorComponent struct {
	Factors    []SemanticKey
	Successors []SemanticKey
}

// RuleOutputDisposition is the exhaustive sealed output ownership of a Rule.
// It mirrors the cold schema tag only; it does not expose an execution mode
// or admission provenance.
type RuleOutputDisposition uint8

const (
	RuleOutputDispositionInvalid RuleOutputDisposition = iota
	RuleOutputDispositionFactor
	RuleOutputDispositionStructural
)

// RuleReadDisposition mirrors one sealed cold Rule read form.
type RuleReadDisposition uint8

const (
	RuleReadDispositionInvalid RuleReadDisposition = iota
	RuleReadDispositionExact
	RuleReadDispositionSummary
	RuleReadDispositionSelect
)

// RuleWriteDisposition mirrors one sealed cold Rule write form.
type RuleWriteDisposition uint8

const (
	RuleWriteDispositionInvalid RuleWriteDisposition = iota
	RuleWriteDispositionExact
	RuleWriteDispositionSelect
	RuleWriteDispositionRoute
)

// RuleReadReport is one ordered cold predecessor observation. Dependencies
// name preceding read ordinals, so their order is part of the Rule schema.
type RuleReadReport struct {
	Kind         RuleReadDisposition
	Input        uint64
	Factor       SemanticKey
	Semantic     SemanticKey
	Normalizer   SemanticKey
	Dependencies []uint64
}

// RuleCarryReport is one whole-output predecessor relation.
type RuleCarryReport struct {
	Input     uint64
	Factor    SemanticKey
	Transform SemanticKey
}

// RuleWriteDependencyReport is one ordered read or preceding-write
// dependency of a selector write. Target distinguishes a write ordinal from
// a read ordinal; it is not a runtime target coordinate.
type RuleWriteDependencyReport struct {
	Target bool
	Index  uint64
}

// RuleWriteReport is one ordered cold output form. Candidates and
// Dependencies retain their sealed ordinal order.
type RuleWriteReport struct {
	Kind         RuleWriteDisposition
	Factor       SemanticKey
	Semantic     SemanticKey
	Route        uint64
	Candidates   []uint64
	Dependencies []RuleWriteDependencyReport
}

// RuleSchemaReport is the complete detached cold shape of one Rule. It is a
// report of the sealed authority, not a declaration API: callbacks, typed
// operands, admission provenance, runtime topology, and concrete targets are
// deliberately absent. RuleAdmissionInventory remains the sole provenance
// report.
type RuleSchemaReport struct {
	Semantic          SemanticKey
	OperandFamily     SemanticKey
	OutputDisposition RuleOutputDisposition
	OutputFactor      SemanticKey
	Inputs            uint64
	Reads             []RuleReadReport
	Carries           []RuleCarryReport
	Writes            []RuleWriteReport
	Supports          []SemanticKey
	Prunes            []SemanticKey
	Activations       []SemanticKey
}

// QueryProjectionDisposition is the exhaustive sealed observation form of a
// Query projection. It mirrors the cold schema only; it does not expose a
// query callback, a runtime point, or a resolved unit.
type QueryProjectionDisposition uint8

const (
	QueryProjectionDispositionInvalid QueryProjectionDisposition = iota
	QueryProjectionDispositionFactorExact
	QueryProjectionDispositionFactorSummary
	QueryProjectionDispositionSupport
)

// QueryProjectionReport is one ordered cold Query observation. Factor and
// Normalizer are unavailable for a structural support projection.
type QueryProjectionReport struct {
	Kind       QueryProjectionDisposition
	Factor     SemanticKey
	Normalizer SemanticKey
}

// QuerySchemaReport is the complete detached cold shape of one Query family.
// Freezer names its existing persistence law. It intentionally carries no
// operand family, query callback, equation coordinate, or result value.
type QuerySchemaReport struct {
	Semantic    SemanticKey
	Freezer     SemanticKey
	Projections []QueryProjectionReport
}

// CompositionReport is the immutable-by-value semantic inventory derived
// from one sealed Composition. It permits a composition root to check its
// reviewed declaration and derived-incidence contract without exposing cold
// schemas, typed callbacks, binding indexes, or runtime topology.
//
// Rule admission provenance remains the dedicated RuleAdmissionInventory
// report. Rules are canonical by Rule semantic identity. Factor membership is
// the union of Components' Factors.
type CompositionReport struct {
	ID                 CompositionID
	Completion         SemanticKey
	CompletionPrune    SemanticKey
	ActivationFamilies []SemanticKey
	Rules              []RuleSchemaReport
	Queries            []QuerySchemaReport
	Incidences         []FactorIncidence
	Components         []FactorComponent
}

// SemanticReport returns the complete canonical semantic inventory of a
// successfully sealed Composition. It is unavailable before Seal and every
// returned slice is detached from the cold authority.
func (composition *Composition) SemanticReport() (CompositionReport, bool) {
	if composition == nil || !composition.Sealed() || composition.sealed == nil {
		return CompositionReport{}, false
	}
	report := CompositionReport{ID: composition.id}
	if !report.ID.Available() {
		return CompositionReport{}, false
	}
	if completion, present := composition.sealed.Completion(); present {
		report.Completion = semanticKeyFromComposition(completion.Semantic)
		report.CompletionPrune = semanticKeyFromComposition(completion.Prune)
		if !report.Completion.Available() || !report.CompletionPrune.Available() {
			return CompositionReport{}, false
		}
	}

	activations := composition.sealed.ActivationFamilies()
	report.ActivationFamilies = make([]SemanticKey, len(activations))
	for index, activation := range activations {
		key := semanticKeyFromComposition(activation.Semantic)
		if !key.Available() {
			return CompositionReport{}, false
		}
		report.ActivationFamilies[index] = key
	}

	rules := composition.sealed.Rules()
	report.Rules = make([]RuleSchemaReport, len(rules))
	for index, rule := range rules {
		derived, valid := ruleSchemaReportFromCold(rule)
		if !valid {
			return CompositionReport{}, false
		}
		report.Rules[index] = derived
	}

	queries := composition.sealed.Queries()
	report.Queries = make([]QuerySchemaReport, len(queries))
	for index, query := range queries {
		derived, valid := querySchemaReportFromCold(query)
		if !valid {
			return CompositionReport{}, false
		}
		report.Queries[index] = derived
	}

	incidences := composition.sealed.Incidence()
	report.Incidences = make([]FactorIncidence, len(incidences))
	for index, incidence := range incidences {
		read := semanticKeyFromComposition(incidence.Read)
		write := semanticKeyFromComposition(incidence.Write)
		if !read.Available() || !write.Available() {
			return CompositionReport{}, false
		}
		report.Incidences[index] = FactorIncidence{Read: read, Write: write}
	}

	components := composition.sealed.Components()
	report.Components = make([]FactorComponent, len(components))
	for index, component := range components {
		factors := make([]SemanticKey, len(component.Factors))
		for factorIndex, factor := range component.Factors {
			key := semanticKeyFromComposition(factor)
			if !key.Available() {
				return CompositionReport{}, false
			}
			factors[factorIndex] = key
		}
		successors := make([]SemanticKey, len(component.Successors))
		for successorIndex, successor := range component.Successors {
			key := semanticKeyFromComposition(successor)
			if !key.Available() {
				return CompositionReport{}, false
			}
			successors[successorIndex] = key
		}
		report.Components[index] = FactorComponent{Factors: factors, Successors: successors}
	}
	return report, true
}

func querySchemaReportFromCold(query composition.QueryFamily) (QuerySchemaReport, bool) {
	semantic := semanticKeyFromComposition(query.Key)
	freezer := semanticKeyFromComposition(query.Freezer)
	if !semantic.Available() || !freezer.Available() {
		return QuerySchemaReport{}, false
	}
	report := QuerySchemaReport{Semantic: semantic, Freezer: freezer}
	if len(query.Projections) != 0 {
		report.Projections = make([]QueryProjectionReport, len(query.Projections))
	}
	for index, projection := range query.Projections {
		derived, valid := queryProjectionReportFromCold(projection)
		if !valid {
			return QuerySchemaReport{}, false
		}
		report.Projections[index] = derived
	}
	return report, true
}

func queryProjectionReportFromCold(projection composition.QueryProjection) (QueryProjectionReport, bool) {
	kind, valid := queryProjectionDispositionFromCold(projection.Kind)
	if !valid {
		return QueryProjectionReport{}, false
	}
	report := QueryProjectionReport{
		Kind:       kind,
		Factor:     semanticKeyFromComposition(projection.Factor),
		Normalizer: semanticKeyFromComposition(projection.Normalizer),
	}
	if kind == QueryProjectionDispositionSupport {
		if report.Factor.Available() || report.Normalizer.Available() {
			return QueryProjectionReport{}, false
		}
		return report, true
	}
	if !report.Factor.Available() {
		return QueryProjectionReport{}, false
	}
	if kind == QueryProjectionDispositionFactorSummary && !report.Normalizer.Available() ||
		kind == QueryProjectionDispositionFactorExact && report.Normalizer.Available() {
		return QueryProjectionReport{}, false
	}
	return report, true
}

func queryProjectionDispositionFromCold(kind composition.QueryProjectionKind) (QueryProjectionDisposition, bool) {
	switch kind {
	case composition.QueryFactorExact:
		return QueryProjectionDispositionFactorExact, true
	case composition.QueryFactorSummary:
		return QueryProjectionDispositionFactorSummary, true
	case composition.QuerySupport:
		return QueryProjectionDispositionSupport, true
	default:
		return QueryProjectionDispositionInvalid, false
	}
}

func ruleSchemaReportFromCold(rule composition.Rule) (RuleSchemaReport, bool) {
	semantic := semanticKeyFromComposition(rule.Key)
	operandFamily := semanticKeyFromComposition(rule.OperandFamily)
	if !semantic.Available() || !operandFamily.Available() {
		return RuleSchemaReport{}, false
	}
	disposition, valid := ruleOutputDispositionFromCold(rule.OutputKind)
	if !valid {
		return RuleSchemaReport{}, false
	}
	report := RuleSchemaReport{
		Semantic:          semantic,
		OperandFamily:     operandFamily,
		OutputDisposition: disposition,
		OutputFactor:      semanticKeyFromComposition(rule.Output),
		Inputs:            rule.Inputs,
	}
	if disposition == RuleOutputDispositionFactor && !report.OutputFactor.Available() ||
		disposition == RuleOutputDispositionStructural && report.OutputFactor.Available() {
		return RuleSchemaReport{}, false
	}
	if len(rule.Reads) != 0 {
		report.Reads = make([]RuleReadReport, len(rule.Reads))
	}
	for index, read := range rule.Reads {
		derived, known := ruleReadReportFromCold(read)
		if !known {
			return RuleSchemaReport{}, false
		}
		report.Reads[index] = derived
	}
	if len(rule.Carries) != 0 {
		report.Carries = make([]RuleCarryReport, len(rule.Carries))
	}
	for index, carry := range rule.Carries {
		factor := semanticKeyFromComposition(carry.Factor)
		if !factor.Available() {
			return RuleSchemaReport{}, false
		}
		report.Carries[index] = RuleCarryReport{
			Input: carry.Input, Factor: factor, Transform: semanticKeyFromComposition(carry.Transform),
		}
	}
	if len(rule.Writes) != 0 {
		report.Writes = make([]RuleWriteReport, len(rule.Writes))
	}
	for index, write := range rule.Writes {
		derived, known := ruleWriteReportFromCold(write)
		if !known {
			return RuleSchemaReport{}, false
		}
		report.Writes[index] = derived
	}
	if len(rule.Supports) != 0 {
		report.Supports = make([]SemanticKey, len(rule.Supports))
	}
	for index, support := range rule.Supports {
		key := semanticKeyFromComposition(support.Semantic)
		if !key.Available() {
			return RuleSchemaReport{}, false
		}
		report.Supports[index] = key
	}
	if len(rule.Prunes) != 0 {
		report.Prunes = make([]SemanticKey, len(rule.Prunes))
	}
	for index, prune := range rule.Prunes {
		key := semanticKeyFromComposition(prune.Semantic)
		if !key.Available() {
			return RuleSchemaReport{}, false
		}
		report.Prunes[index] = key
	}
	if len(rule.Activations) != 0 {
		report.Activations = make([]SemanticKey, len(rule.Activations))
	}
	for index, activation := range rule.Activations {
		key := semanticKeyFromComposition(activation.Family)
		if !key.Available() {
			return RuleSchemaReport{}, false
		}
		report.Activations[index] = key
	}
	return report, true
}

func ruleOutputDispositionFromCold(kind composition.OutputKind) (RuleOutputDisposition, bool) {
	switch kind {
	case composition.FactorOutput:
		return RuleOutputDispositionFactor, true
	case composition.StructuralOutput:
		return RuleOutputDispositionStructural, true
	default:
		return RuleOutputDispositionInvalid, false
	}
}

func ruleReadReportFromCold(read composition.Read) (RuleReadReport, bool) {
	kind, valid := ruleReadDispositionFromCold(read.Kind)
	factor := semanticKeyFromComposition(read.Factor)
	if !valid || !factor.Available() {
		return RuleReadReport{}, false
	}
	return RuleReadReport{
		Kind:         kind,
		Input:        read.Input,
		Factor:       factor,
		Semantic:     semanticKeyFromComposition(read.Semantic),
		Normalizer:   semanticKeyFromComposition(read.Normalizer),
		Dependencies: append([]uint64(nil), read.Dependencies...),
	}, true
}

func ruleReadDispositionFromCold(kind composition.ReadKind) (RuleReadDisposition, bool) {
	switch kind {
	case composition.ReadExact:
		return RuleReadDispositionExact, true
	case composition.ReadSummary:
		return RuleReadDispositionSummary, true
	case composition.ReadSelect:
		return RuleReadDispositionSelect, true
	default:
		return RuleReadDispositionInvalid, false
	}
}

func ruleWriteReportFromCold(write composition.Write) (RuleWriteReport, bool) {
	kind, valid := ruleWriteDispositionFromCold(write.Kind)
	factor := semanticKeyFromComposition(write.Factor)
	if !valid || !factor.Available() {
		return RuleWriteReport{}, false
	}
	var dependencies []RuleWriteDependencyReport
	if len(write.Dependencies) != 0 {
		dependencies = make([]RuleWriteDependencyReport, len(write.Dependencies))
	}
	for index, dependency := range write.Dependencies {
		dependencies[index] = RuleWriteDependencyReport{Target: dependency.Target, Index: dependency.Index}
	}
	return RuleWriteReport{
		Kind:         kind,
		Factor:       factor,
		Semantic:     semanticKeyFromComposition(write.Semantic),
		Route:        write.Route,
		Candidates:   append([]uint64(nil), write.Candidates...),
		Dependencies: dependencies,
	}, true
}

func ruleWriteDispositionFromCold(kind composition.WriteKind) (RuleWriteDisposition, bool) {
	switch kind {
	case composition.WriteExact:
		return RuleWriteDispositionExact, true
	case composition.WriteSelect:
		return RuleWriteDispositionSelect, true
	case composition.WriteRoute:
		return RuleWriteDispositionRoute, true
	default:
		return RuleWriteDispositionInvalid, false
	}
}
