package publicationescape

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Source is one published publication-source row: the Value coordinate the
// batch's publication reads its subject member at, and the tag the selection
// stamps that row with so a reading rule correlates the returned cell with the
// member it was selected for.
type Source struct {
	spec sourceSpec
}

// Coordinate is the Value coordinate this source row is read at.
func (source Source) Coordinate() (valuedomain.Coordinate, bool) {
	return source.spec.coordinate, source.spec.coordinate.Valid() && source.spec.tag != 0
}

// Predicate is the tag this row carries.
func (source Source) Predicate() (uint64, bool) {
	return uint64(source.spec.tag), source.spec.coordinate.Valid() && source.spec.tag != 0
}

// SourcePlan is the source set one admitted batch publishes, together with the
// sealed reading and the operation gate it was selected under. A route
// derivation consumes the whole plan because the routes of a batch depend on
// the same rows and the same gate, not only on the cells this vector returned.
type SourcePlan struct {
	prepared *PreparedBatch
	gate     operationGate
	view     sourceView
}

// Available reports whether the plan carries an authenticated reading.
func (plan SourcePlan) Available() bool { return plan.prepared != nil }

// Count is the census of published source rows.
func (plan SourcePlan) Count() int { return plan.view.len() }

// At returns one source row at its publication position.
func (plan SourcePlan) At(index int) (Source, bool) {
	spec, ok := plan.view.at(index)
	if !ok {
		return Source{}, false
	}
	return Source{spec: spec}, true
}

// PublicationSourceCount and PublicationSourceAt are the direct composition
// accessors of one derived source plan.
func PublicationSourceCount(plan SourcePlan) int { return plan.Count() }

// PublicationSourceAt returns one source row of a derived plan.
func PublicationSourceAt(plan SourcePlan, index int) (Source, bool) { return plan.At(index) }

// DerivePublicationSources is the operation that publishes the publication
// source rows of one mounted call.
//
// Which rows exist depends on the Call fact the read before it delivered: only
// the operations that fact authorizes publish a receipt, so the coordinates
// are not a directory anything could enumerate before that value is known.
//
// It takes the sealed inputs alone - the Value schema the coordinates belong
// to, the Effect algebra that issued the call row, the Call algebra that
// admitted the fact, and the row and fact themselves. The publication batch is
// data reached from that row rather than a second candidate directory, which
// is what this resolves it as.
func DerivePublicationSources(values *valuedomain.Schema, effects *effectfactor.Algebra, calls *calldomain.Algebra, mounted effectfactor.MountedCall, callFact calldomain.Value) (SourcePlan, bool) {
	if effects == nil || !effects.Valid() || !mounted.Valid() {
		return SourcePlan{}, false
	}
	_, module, occurrence, identityOK := effects.MountedCallIdentity(mounted)
	if !identityOK || !module.Available() || !occurrence.Available() {
		return SourcePlan{}, false
	}
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(effects)
	if !indexOK || index == nil || !index.Valid() {
		return SourcePlan{}, false
	}
	batch, batchOK := index.BatchForCall(module, occurrence)
	if !batchOK {
		return SourcePlan{}, false
	}
	prepared, preparedOK := PrepareBatch(values, batch)
	if !preparedOK {
		return SourcePlan{}, false
	}
	return SourcePlanFor(prepared, calls, callFact)
}

// SourcePlanFor is the same statement over a batch whose sealed reading is
// already held. The hot rule prepares every batch once at bind and reaches the
// plan through this door; DerivePublicationSources prepares and then calls it,
// so both arms publish rows through one reading and one gate.
func SourcePlanFor(prepared *PreparedBatch, calls *calldomain.Algebra, callFact calldomain.Value) (SourcePlan, bool) {
	if prepared == nil {
		return SourcePlan{}, false
	}
	gate, gateOK := operationGateFor(calls, prepared, callFact)
	if !gateOK {
		return SourcePlan{}, false
	}
	return SourcePlan{prepared: prepared, gate: gate, view: prepared.sourcesForGate(gate)}, true
}

// SourceCell is the Value cell delivered for one published source row, in the
// plan's own order.
type SourceCell struct {
	Value   valuedomain.Value
	Present bool
}

// SourceFacts is the authenticated join of a source plan with the cells its
// read delivered: one Value fact per publication row, merged under the Value
// authority that owns the coordinates.
type SourceFacts struct {
	buffer factBuffer
}

// NewSourceFacts authenticates the delivered cells against the plan that
// selected them. Each cell answers for the row at the same position, its value
// must be admissible at that row's coordinate, and an absent cell is the Value
// owner's exact Bottom or nothing at all.
func NewSourceFacts(values *valuedomain.Schema, plan SourcePlan, cells []SourceCell) (SourceFacts, bool) {
	if values == nil || !values.Valid() || !plan.Available() || len(cells) != plan.Count() {
		return SourceFacts{}, false
	}
	var facts factBuffer
	for index, cell := range cells {
		source, sourceOK := plan.view.at(index)
		if !sourceOK {
			return SourceFacts{}, false
		}
		if !values.AdmitsCoordinate(source.coordinate, values.Bottom()) || source.tag == 0 ||
			!source.rowID.Available() || source.operation == 0 || source.member < 0 {
			return SourceFacts{}, false
		}
		if cell.Present && !values.AdmitsCoordinate(source.coordinate, cell.Value) ||
			!cell.Present && !values.Equal(cell.Value, values.Bottom()) {
			return SourceFacts{}, false
		}
		if !facts.merge(values, factEntry{rowID: source.rowID, value: cell.Value, present: cell.Present}) {
			return SourceFacts{}, false
		}
	}
	return SourceFacts{buffer: facts}, true
}

// Route is one published publication-escape route: the allocation cell the
// rule reads and publishes into, and the tag the selection stamps it with. A
// publication escape moves nothing between cells - the requirement is joined
// into the reached allocation's own cell - so both endpoints are that key.
type Route struct {
	planned plannedRoute
}

// Coordinates are the cell this route reads and the cell it publishes into.
func (route Route) Coordinates() (heapdomain.Key, heapdomain.Key) {
	return route.planned.key, route.planned.key
}

// Predicate is the tag this row carries.
func (route Route) Predicate() uint64 { return uint64(route.planned.tag) }

// RoutePlan is the route set one admitted batch publishes.
type RoutePlan struct {
	buffer routeBuffer
}

// RouteCount is the census of published routes.
func (plan RoutePlan) RouteCount() int { return plan.buffer.len() }

// RouteAt returns one route at its publication position.
func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	planned, ok := plan.buffer.at(index)
	if !ok {
		return Route{}, false
	}
	return Route{planned: planned}, true
}

// PublicationRouteCount and PublicationRouteAt are the direct composition
// accessors of one derived route plan.
func PublicationRouteCount(plan RoutePlan) int { return plan.RouteCount() }

// PublicationRouteAt returns one route of a derived plan.
func PublicationRouteAt(plan RoutePlan, index int) (Route, bool) { return plan.RouteAt(index) }

// DerivePublicationRoutes is the operation that publishes the route rows of
// one mounted batch: for every publication its Call fact authorized, the
// allocation roots that publication's subject reaches, widened to every root
// where the subject is open.
//
// The rows do not exist until the source cells are known, which is why the
// plan and the facts joined to it are its inputs rather than a directory.
func DerivePublicationRoutes(schema placementdomain.Schema, values *valuedomain.Schema, plan SourcePlan, facts SourceFacts) (RoutePlan, bool) {
	if !plan.Available() {
		return RoutePlan{}, false
	}
	buffer, ok := routeSetFor(schema, values, plan.prepared, plan.gate, facts.buffer)
	if !ok {
		return RoutePlan{}, false
	}
	return RoutePlan{buffer: buffer}, true
}

// position recovers the publication position of one selected row from the tag
// it carries. The engine exposes a selection in physical order, so the tag is
// the only thing that says which source row a returned cell answers for.
func (plan SourcePlan) position(tag sourceTag) (int, bool) {
	for index := 0; index < plan.view.len(); index++ {
		source, ok := plan.view.at(index)
		if !ok {
			return 0, false
		}
		if source.tag == tag {
			return index, true
		}
	}
	return 0, false
}
