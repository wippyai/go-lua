package analysis

import (
	"github.com/wippyai/go-lua/analysis/coverage"
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/analysis/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectcallsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapclosed "github.com/wippyai/go-lua/analysis/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/analysis/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/analysis/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/analysis/domain/pack/source"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueallocation "github.com/wippyai/go-lua/analysis/domain/value/allocation"
	valuebootstrap "github.com/wippyai/go-lua/analysis/domain/value/bootstrap"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/analysis/domain/value/transfer"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
)

// programAnalysis is the sole production composition root. This first closed
// slice owns five Factors, fifteen factor-output Rules, and one structural
// activation Rule; it remains intentionally
// incomplete without implying a second Composition or evaluator as the
// remaining typed source authorities land.
type programAnalysis struct {
	composition *engine.Composition
	semantics   semanticBundle
	coverage    coverage.Ledger
	coverageOK  bool

	heapSchema    heapdomain.Schema
	valueSchema   *valuedomain.Schema
	callAlgebra   *calldomain.Algebra
	packSchema    *packdomain.Schema
	topology      *heapindex.Topology
	effectAlgebra *effectfactor.Algebra

	values  *valueowner.Owner
	calls   *callowner.Owner
	heap    *heapowner.Owner
	packs   *packowner.Owner
	effects *effectowner.Owner

	valueSource     *valuesource.Rule
	packSource      *packsource.Rule
	heapIngress     *heapingress.Rule
	valueAllocation *valueallocation.Rule
	heapEmpty       *heapempty.Rule
	heapClosed      *heapclosed.Rule
	rawGet          *heapindex.RawGetRule
	rawSet          *heapindex.RawSetRule
	callDispatch    *calldispatch.Rule
	effectSelected  *effectcallsite.Rule
	effectOpaque    *effectcallsite.Rule
	effectBody      *effectcallsite.BodyCallRule
	valueBootstrap  *valuebootstrap.Rule
	heapBootstrap   *heapbootstrap.Rule
	valueTransfer   *valuetransfer.Rule
	callActivation  *callactivation.Source

	// bodies is the complete mounted executable Body denominator. It is used
	// only to bind detached body rows and Effect query roots.
	bodies  []mountedBody
	queries programQueries
}

// programQueries is the fixed cold query-family vocabulary for every admitted
// Program body.  A body creates one exact QueryInstance of each family during
// solve; it never creates a new cold Query schema.
type programQueries struct {
	value      *engine.Query[valueSummaryObservation]
	valueRead  engine.QueryRead[engine.OrderedCells[valuedomain.Value]]
	effect     *engine.Query[effectObservation]
	effectRead engine.QueryRead[engine.OrderedCells[effectfactor.Value]]
}

// valueSummaryObservation is the detached result of the one Value summary
// family.  The solve-local body plan interprets its canonical cells as either
// direct result/receiver coordinates or a linear storage output vector.
type valueSummaryObservation struct {
	values  []valuedomain.Value
	present []bool
	rows    uint32
	valid   bool
}

func callActivationRoutes(source *link.Link, bodies calldomain.Bodies) ([]callactivation.Route, bool) {
	if source == nil || source.Project() == nil || !source.ContentID().Available() {
		return nil, false
	}
	if bodies.Count() == 0 {
		return []callactivation.Route{}, true
	}
	projectID := source.Project().Cold().ContentID()
	if !projectID.Available() {
		return nil, false
	}
	routes := make([]callactivation.Route, bodies.Count())
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		bodyID, bodyIDOK := body.ContentID()
		if !bodyOK || !bodyIDOK {
			return nil, false
		}
		target, targetOK := analysisSemanticKeyParts(source.ContentID(), "canonical/call-body-target", projectID[:], bodyID[:])
		endpoint, endpointOK := analysisSemanticKeyParts(source.ContentID(), "canonical/call-body-endpoint", projectID[:], bodyID[:])
		if !targetOK || !endpointOK || target == endpoint {
			return nil, false
		}
		routes[index] = callactivation.Route{Body: body, Target: target, Endpoint: endpoint}
	}
	return routes, true
}

func newProgramAnalysis(source *link.Link) (*programAnalysis, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	publications, publicationsErr := source.SourcePublications()
	if publicationsErr != nil {
		return nil, false
	}
	semantics, semanticsOK := newSemanticBundle(source.ContentID())
	heapSchema, heapOK := heapdomain.Seal(source)
	valueSchema, valueOK := valuedomain.Seal(source, heapSchema)
	callAlgebra, callOK := calldomain.New(source)
	activationRoutes, activationRoutesOK := callActivationRoutes(source, callAlgebra.Bodies())
	types, typesOK := typeauthority.Seal(source)
	statics, _, staticErr := staticdomain.Seal(source, types)
	packSchema, packOK := packdomain.Seal(source, statics)
	topology, topologyOK := heapindex.Seal(heapSchema, valueSchema, callAlgebra)
	contract, contractOK := source.Boundary().Target()
	effectAlgebra, effectAlgebraOK := effectfactor.New(source, packSchema, contract)
	if !semanticsOK || !heapOK || !valueOK || !callOK || !activationRoutesOK || !typesOK || staticErr != nil || !packOK || !topologyOK || !contractOK || !effectAlgebraOK {
		return nil, false
	}

	result := &programAnalysis{
		semantics: semantics, heapSchema: heapSchema, valueSchema: valueSchema,
		callAlgebra: callAlgebra, packSchema: packSchema, topology: topology, effectAlgebra: effectAlgebra,
	}
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, semantics.valueFactor, semantics.valueSummary, valueSchema)
	calls, callsOK := callowner.Declare(composition, semantics.callFactor, callAlgebra)
	heap, heapOwnerOK := heapowner.Declare(composition, semantics.heapFactor, heapSchema)
	packs, packsOK := packowner.Declare(composition, semantics.packFactor, packSchema)
	effects, effectsOK := effectowner.Declare(composition, semantics.effectFactor, semantics.effectSummary, effectAlgebra)
	if !valuesOK || !callsOK || !heapOwnerOK || !packsOK || !effectsOK {
		return nil, false
	}
	result.values, result.calls, result.heap, result.packs, result.effects = values, calls, heap, packs, effects

	valueSource, valueSourceOK := valuesource.Declare(composition,
		semantics.valueSourceRule.rule, semantics.valueSourceRule.operand, semantics.valueSourceRule.evidence, values)
	packSource, packSourceOK := packsource.Declare(composition,
		semantics.packSourceRule.rule, semantics.packSourceRule.operand, semantics.packSourceRule.evidence, packs)
	heapIngress, heapIngressOK := heapingress.Declare(composition,
		semantics.heapIngressRule.rule, semantics.heapIngressRule.operand, semantics.heapIngressRule.evidence, heap)
	valueAllocation, valueAllocationOK := valueallocation.Declare(composition,
		semantics.valueAllocationRule.rule, semantics.valueAllocationRule.operand,
		semantics.valueAllocationRule.transform, semantics.valueAllocationRule.evidence, values)
	heapEmpty, heapEmptyOK := heapempty.Declare(composition,
		semantics.heapEmptyRule.rule, semantics.heapEmptyRule.operand,
		semantics.heapEmptyRule.transform, semantics.heapEmptyRule.evidence, heap)
	heapClosed, heapClosedOK := heapclosed.Declare(composition,
		semantics.heapClosedRule.rule, semantics.heapClosedRule.operand,
		semantics.heapClosedRule.transform, semantics.heapClosedRule.evidence, heap, values)
	rawGet, rawGetOK := heapindex.DeclareRawGet(composition,
		semantics.rawGetRule.rule, semantics.rawGetRule.operand, semantics.rawGetRule.evidence,
		topology, values, calls, heap, packs)
	rawSet, rawSetOK := heapindex.DeclareRawSet(composition,
		semantics.rawSetRule.rule, semantics.rawSetRule.operand, semantics.rawSetRule.evidence,
		topology, values, heap, packs)
	callDispatch, callDispatchOK := calldispatch.Declare(composition,
		semantics.callDispatchRule.rule, semantics.callDispatchRule.operand, semantics.callDispatchRule.evidence,
		values, heapSchema, packSchema, calls)
	effectSelected, effectSelectedOK := effectcallsite.DeclareSelected(composition,
		semantics.effectSelectedRule.rule, semantics.effectSelectedRule.operand, semantics.effectSelectedRule.evidence,
		calls, effects)
	effectOpaque, effectOpaqueOK := effectcallsite.DeclareOpaque(composition,
		semantics.effectOpaqueRule.rule, semantics.effectOpaqueRule.operand, semantics.effectOpaqueRule.evidence,
		calls, effects)
	effectBody, effectBodyOK := effectcallsite.DeclareBody(composition,
		semantics.effectBodyRule.rule, semantics.effectBodyRule.operand, semantics.effectBodyRule.evidence,
		calls, effects)
	callActivation, callActivationOK := callactivation.Declare(composition, callactivation.Spec{
		Link: source, Calls: calls,
		Semantic: semantics.callActivation, Family: semantics.callActivationFamily, Admission: semantics.callActivationAdmission,
		Routes: activationRoutes,
	})
	valueBootstrap, valueBootstrapOK := valuebootstrap.Declare(composition,
		semantics.valueBootstrapRule.rule, semantics.valueBootstrapRule.operand, semantics.valueBootstrapRule.evidence,
		values)
	heapBootstrap, heapBootstrapOK := heapbootstrap.Declare(composition,
		semantics.heapBootstrapRule.rule, semantics.heapBootstrapRule.operand, semantics.heapBootstrapRule.evidence,
		heap)
	valueTransfer, valueTransferOK := valuetransfer.Declare(composition,
		semantics.valueTransferRule.rule, semantics.valueTransferRule.operand, semantics.valueTransferRule.evidence,
		values)
	if !valueSourceOK || !packSourceOK || !heapIngressOK || !valueAllocationOK || !heapEmptyOK || !heapClosedOK || !rawGetOK || !rawSetOK ||
		!callDispatchOK || !effectSelectedOK || !effectOpaqueOK || !effectBodyOK || !callActivationOK || !valueBootstrapOK || !heapBootstrapOK || !valueTransferOK {
		return nil, false
	}
	result.valueSource, result.packSource, result.heapIngress = valueSource, packSource, heapIngress
	result.valueAllocation, result.heapEmpty, result.heapClosed, result.rawGet, result.rawSet = valueAllocation, heapEmpty, heapClosed, rawGet, rawSet
	result.callDispatch, result.effectSelected, result.effectOpaque = callDispatch, effectSelected, effectOpaque
	result.effectBody = effectBody
	result.callActivation = callActivation
	result.valueBootstrap, result.heapBootstrap = valueBootstrap, heapBootstrap
	result.valueTransfer = valueTransfer

	queries, queriesOK := declareProgramQueries(composition, values, effects, valueSchema, effectAlgebra, semantics)
	if !queriesOK {
		return nil, false
	}

	bodies, bodiesOK := mountedProgramBodies(source)
	if !bodiesOK || len(bodies) == 0 {
		return nil, false
	}
	if !composition.Seal() {
		return nil, false
	}
	productionCoverage := freezeProductionCoverage(source, publications, semantics, composition)
	// The composition has sealed and the canonical body census is still useful
	// for an explicit support decision. A failed coverage ledger is retained as
	// a cold admission bit; Analyze rejects the whole Link before execution and
	// never routes it through a fallback composition or generic source stream.
	result.coverage, result.coverageOK = productionCoverage.ledger, productionCoverage.valid
	result.composition, result.bodies, result.queries = composition, bodies, queries
	return result, true
}

func declareProgramQueries(
	composition *engine.Composition,
	values *valueowner.Owner,
	effects *effectowner.Owner,
	valueSchema *valuedomain.Schema,
	effectAlgebra *effectfactor.Algebra,
	semantics semanticBundle,
) (programQueries, bool) {
	if composition == nil || values == nil || effects == nil || valueSchema == nil || effectAlgebra == nil || !semantics.valueQuery.Available() || !semantics.valueCodec.Available() || !semantics.effectQuery.Available() || !semantics.effectCodec.Available() {
		return programQueries{}, false
	}
	var valueRead engine.QueryRead[engine.OrderedCells[valuedomain.Value]]
	valueQuery, valueQueryOK := engine.DeclareQuery(composition, engine.QuerySpec[valueSummaryObservation]{
		Semantic: semantics.valueQuery,
		Project: func(observation engine.Observation) valueSummaryObservation {
			result := valueSummaryObservation{}
			complete := engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				result.rows++
				if result.rows != 1 {
					return false
				}
				cells, cellsOK := engine.QueryValue(row, valueRead)
				if !cellsOK || cells.Count() == 0 {
					return false
				}
				result.values = make([]valuedomain.Value, cells.Count())
				result.present = make([]bool, cells.Count())
				for index := range result.values {
					var available bool
					result.values[index], result.present[index], available = cells.At(index)
					if !available {
						return false
					}
				}
				return true
			})
			// An empty Product is a valid structural observation: the queried
			// point is unreachable for this body/outcome and therefore has no
			// Value row to project.  A nonempty observation still has exactly
			// one row and a complete, nonempty cell vector; malformed one-row
			// observations remain invalid.
			result.valid = complete && (result.rows == 0 || result.rows == 1 && len(result.values) == len(result.present) && len(result.values) != 0)
			return result
		},
		Result: engine.FrozenResult[valueSummaryObservation]{
			Semantic: semantics.valueCodec,
			Freeze:   cloneValueSummaryObservation,
			Clone:    cloneValueSummaryObservation,
			Equal: func(left, right valueSummaryObservation) bool {
				return equalValueSummaryObservation(valueSchema, left, right)
			},
			Fingerprint: func(value valueSummaryObservation) uint64 {
				return fingerprintValueSummaryObservation(valueSchema, value)
			},
		},
	}, func(query *engine.Query[valueSummaryObservation]) bool {
		var ok bool
		valueRead, ok = engine.QueryReadFrom(query, values.SummaryRead())
		return ok
	})
	var effectRead engine.QueryRead[engine.OrderedCells[effectfactor.Value]]
	effectQuery, effectQueryOK := engine.DeclareQuery(composition, engine.QuerySpec[effectObservation]{
		Semantic: semantics.effectQuery,
		Project: func(observation engine.Observation) effectObservation {
			return projectEffectObservation(effectAlgebra, observation, effectRead)
		},
		Result: engine.FrozenResult[effectObservation]{
			Semantic: semantics.effectCodec, Freeze: cloneEffectObservation, Clone: cloneEffectObservation, Equal: equalEffectObservation,
			Fingerprint: fingerprintEffectObservation,
		},
	}, func(query *engine.Query[effectObservation]) bool {
		var ok bool
		effectRead, ok = engine.QueryReadFrom(query, effects.ExactRead())
		return ok
	})
	return programQueries{value: valueQuery, valueRead: valueRead, effect: effectQuery, effectRead: effectRead},
		valueQueryOK && valueQuery != nil && effectQueryOK && effectQuery != nil
}

func cloneValueSummaryObservation(value valueSummaryObservation) valueSummaryObservation {
	value.values = append([]valuedomain.Value(nil), value.values...)
	value.present = append([]bool(nil), value.present...)
	return value
}

func equalValueSummaryObservation(schema *valuedomain.Schema, left, right valueSummaryObservation) bool {
	if schema == nil || left.valid != right.valid || left.rows != right.rows || len(left.values) != len(right.values) || len(left.present) != len(right.present) {
		return false
	}
	for index := range left.values {
		if left.present[index] != right.present[index] || left.present[index] && !schema.Equal(left.values[index], right.values[index]) {
			return false
		}
	}
	return true
}

func fingerprintValueSummaryObservation(schema *valuedomain.Schema, value valueSummaryObservation) uint64 {
	if schema == nil {
		return 0
	}
	result := uint64(value.rows) << 32
	for index := range value.values {
		result ^= uint64(index+1) * 0x9e3779b97f4a7c15
		if index < len(value.present) && value.present[index] {
			result ^= schema.Fingerprint(value.values[index])
		}
	}
	if value.valid {
		result ^= 1 << 63
	}
	return result
}
