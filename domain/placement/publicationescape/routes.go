package publicationescape

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Source is one owner-issued Value receipt row for a publication subject.
type source struct{ spec sourceSpec }

func (source source) Coordinate() (valuedomain.Coordinate, bool) {
	return source.spec.coordinate, source.spec.coordinate.Valid() && source.spec.tag != 0
}

func (source source) Predicate() (uint64, bool) {
	return uint64(source.spec.tag), source.spec.coordinate.Valid() && source.spec.tag != 0
}

// SourcePlan is the sealed pre-seal publication composition: Effect's batch,
// Call's operation gate, and the Value source rows selected by that gate.
type sourcePlan struct {
	prepared *PreparedBatch
	gate     operationGate
	view     sourceView
}

func (plan sourcePlan) Available() bool { return plan.prepared != nil }
func (plan sourcePlan) Count() int      { return plan.view.len() }

func (plan sourcePlan) At(index int) (source, bool) {
	spec, ok := plan.view.at(index)
	if !ok {
		return source{}, false
	}
	return source{spec: spec}, true
}

// DerivePublicationSources is the sole source-row publisher. It prepares the
// Effect-owned batch once, then applies the Call fact's authenticated gate.
func derivePublicationSources(values *valuedomain.Schema, effects *effectfactor.Algebra, calls *calldomain.Algebra, mounted effectfactor.MountedCall, callFact calldomain.Value) (sourcePlan, bool) {
	if effects == nil || !effects.Valid() || !mounted.Valid() {
		return sourcePlan{}, false
	}
	_, module, occurrence, identityOK := effects.MountedCallIdentity(mounted)
	if !identityOK || !module.Available() || !occurrence.Available() {
		return sourcePlan{}, false
	}
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(effects)
	if !indexOK || index == nil || !index.Valid() {
		return sourcePlan{}, false
	}
	batch, batchOK := index.BatchForCall(module, occurrence)
	if !batchOK {
		return sourcePlan{}, false
	}
	prepared, preparedOK := PrepareBatch(values, batch)
	if !preparedOK {
		return sourcePlan{}, false
	}
	return sourcePlanFor(prepared, calls, callFact)
}

// SourcePlanFor binds one already sealed batch to the exact Call fact.
func sourcePlanFor(prepared *PreparedBatch, calls *calldomain.Algebra, callFact calldomain.Value) (sourcePlan, bool) {
	if prepared == nil {
		return sourcePlan{}, false
	}
	gate, gateOK := operationGateFor(calls, prepared, callFact)
	if !gateOK {
		return sourcePlan{}, false
	}
	return sourcePlan{prepared: prepared, gate: gate, view: prepared.sourcesForGate(gate)}, true
}

type sourceCell struct {
	Value   valuedomain.Value
	Present bool
}

type sourceFacts struct{ buffer factBuffer }

// NewSourceFacts authenticates each delivered Value cell against the source
// row that selected it. No absent value is accepted unless it is the owner's
// exact Bottom.
func newSourceFacts(values *valuedomain.Schema, plan sourcePlan, cells []sourceCell) (sourceFacts, bool) {
	if values == nil || !values.Valid() || !plan.Available() || len(cells) != plan.Count() {
		return sourceFacts{}, false
	}
	var facts factBuffer
	for index, cell := range cells {
		source, sourceOK := plan.view.at(index)
		if !sourceOK || source.tag == 0 || !source.rowID.Available() || source.operation == 0 || source.member < 0 || !values.AdmitsCoordinate(source.coordinate, values.Bottom()) {
			return sourceFacts{}, false
		}
		if cell.Present && !values.AdmitsCoordinate(source.coordinate, cell.Value) || !cell.Present && !values.Equal(cell.Value, values.Bottom()) {
			return sourceFacts{}, false
		}
		if !facts.merge(values, factEntry{rowID: source.rowID, value: cell.Value, present: cell.Present}) {
			return sourceFacts{}, false
		}
	}
	return sourceFacts{buffer: facts}, true
}

// Route is one owner-issued Placement route. Coordinates returns the same
// allocation root for both read and destination roles.
type Route struct{ planned plannedRoute }

func (route Route) Coordinates() (heapdomain.Key, heapdomain.Key, bool) {
	return route.planned.key, route.planned.key, route.valid()
}

func (route Route) Predicate() (uint64, bool) { return uint64(route.planned.tag), route.valid() }

func (route Route) valid() bool {
	return route.planned.key.Valid() && route.planned.key.Kind() == heapdomain.RootAllocation && route.planned.tag != 0
}

type RoutePlan struct{ buffer routeBuffer }

func (plan RoutePlan) RouteCount() int { return plan.buffer.len() }

func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	planned, ok := plan.buffer.at(index)
	if !ok {
		return Route{}, false
	}
	return Route{planned: planned}, true
}

func PublicationRouteCount(plan RoutePlan) int                   { return plan.RouteCount() }
func PublicationRouteAt(plan RoutePlan, index int) (Route, bool) { return plan.RouteAt(index) }

func derivePublicationRoutes(schema placementdomain.Schema, values *valuedomain.Schema, plan sourcePlan, facts sourceFacts) (RoutePlan, bool) {
	if !plan.Available() {
		return RoutePlan{}, false
	}
	buffer, ok := routeSetFor(schema, values, plan.prepared, plan.gate, facts.buffer)
	if !ok {
		return RoutePlan{}, false
	}
	return RoutePlan{buffer: buffer}, true
}

// DerivePublicationRoutesFromComposition is the sole production constructor
// for PublicationRoutes. Its parameters are exactly the pre-seal authorities
// named by the relation: Placement, Value, Effect, and Call schemas followed
// by the mounted candidate, Call fact, and whole selected Value vector.
func DerivePublicationRoutesFromComposition(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	effects *effectfactor.Algebra,
	calls *calldomain.Algebra,
	mounted effectfactor.MountedCall,
	callFact calldomain.Value,
	actuals execution.SummaryVector[valuedomain.Value],
) (RoutePlan, bool) {
	if effects == nil || !effects.Valid() || !mounted.Valid() || !actuals.Valid() {
		return RoutePlan{}, false
	}
	batch, batchOK := effectfactor.NewMountedPublicationBatchIndex(effects)
	if !batchOK || batch == nil || !batch.Valid() {
		return RoutePlan{}, false
	}
	publication, publicationOK := batch.BatchForMountedCall(mounted)
	if !publicationOK {
		return RoutePlan{}, false
	}
	prepared, preparedOK := PrepareBatch(values, publication)
	if !preparedOK {
		return RoutePlan{}, false
	}
	plan, planOK := sourcePlanFor(prepared, calls, callFact)
	if !planOK || actuals.Count() != plan.Count() {
		return RoutePlan{}, false
	}
	cells := make([]sourceCell, plan.Count())
	for index := range cells {
		value, present, available := actuals.At(index)
		if !available {
			return RoutePlan{}, false
		}
		cells[index] = sourceCell{Value: value, Present: present}
	}
	facts, factsOK := newSourceFacts(values, plan, cells)
	if !factsOK {
		return RoutePlan{}, false
	}
	return derivePublicationRoutes(schema, values, plan, facts)
}
