package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/execution"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// routeReducer is the typed semantic half of one freeze invocation. The
// destination this route publishes at arrives beside the cell, issued by the
// same relation member the read observed, and the Heap world at that route
// arrives as the cell.
type routeReducer struct{}

func (routeReducer) Reduce(destination heapdomain.Key, cell execution.SelectedCell[heapdomain.Value]) (heapdomain.Value, structure.ReductionOutcome) {
	// The read declares the Factor's default at an unwritten route, and the
	// freeze judgment publishes the same empty normal image for a Bottom
	// predecessor as for an absent one, so there is no presence distinction
	// left for this fold to draw.
	return heapdomain.FormalFreezeFact(destination, cell.Value)
}

// Empty settles the row whose relation selected no route. A mounted call that
// justifies no exact freeze is not a refusal: it is the authenticated empty
// selection this rule's whole uncertainty vocabulary settles through.
func (routeReducer) Empty() structure.ReductionOutcome { return structure.NoSelection }

type routeRow struct {
	candidate calldomain.CallCoordinate
	callFact  execution.ExactRead[calldomain.DenseCoordinate, calldomain.Value]
	// actuals is this call's ordered mounted actuals: one sealed exact read per
	// member, at the coordinate the engine resolved for it. Value's parent row
	// and every member coordinate were settled before this row was sealed, so
	// the worker holds no call identity and enumerates nothing.
	actuals       []execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	actualsPolicy execution.ReadCellPolicy[valuedomain.Value]
	selected      execution.SelectedRead[heapdomain.DenseCoordinate, heapdomain.Value]
	write         execution.RouteWrite[heapdomain.DenseCoordinate, heapdomain.Value]
}

type routeFamily struct {
	rows        []routeRow
	heap        heapdomain.Schema
	values      *valuedomain.Schema
	calls       *calldomain.Algebra
	packs       *packdomain.Schema
	plane       execution.FormPlane[heapdomain.DenseCoordinate, heapdomain.Value]
	routeWidth  int
	actualWidth int
}

func (family *routeFamily) NewExecutor(run *execution.Run) execution.Executor {
	if family == nil || run == nil || family.routeWidth < 0 || family.actualWidth < 0 {
		return nil
	}
	// Every buffer is sized once, at the sealed widths of the two selections
	// this rule may span. An ordinary invocation therefore allocates nothing.
	return &routeWorker{
		family:      family,
		run:         run,
		members:     make([]execution.RouteMember, family.routeWidth),
		cells:       make([]execution.SelectedCell[heapdomain.Value], family.routeWidth),
		routes:      make([]heapdomain.Key, family.routeWidth),
		actualCells: make([]execution.MemberCell[valuedomain.Value], family.actualWidth),
	}
}

func (*routeFamily) InputCapacity() int  { return 3 }
func (*routeFamily) OutputCapacity() int { return 1 }

type routeWorker struct {
	family      *routeFamily
	run         *execution.Run
	call        execution.Scratch[calldomain.DenseCoordinate, calldomain.Value]
	selected    execution.SelectedScratch[heapdomain.DenseCoordinate, heapdomain.Value]
	write       execution.RouteScratch[heapdomain.DenseCoordinate, heapdomain.Value]
	members     []execution.RouteMember
	cells       []execution.SelectedCell[heapdomain.Value]
	routes      []heapdomain.Key
	actualRead  execution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	actualCells []execution.MemberCell[valuedomain.Value]
}

func (worker *routeWorker) Execute(frame execution.Frame, ticket execution.Ticket) (execution.Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || !worker.run.Owns(ticket) {
		return execution.Result{}, false
	}
	local, localOK := ticket.LocalOrdinal()
	if !localOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return execution.Result{}, false
	}
	row := worker.family.rows[local]

	var callFact calldomain.Value
	switch worker.callRead(row.callFact, ticket, &callFact) {
	case structure.NoCandidate:
		return worker.settle(ticket, structure.NoCandidate)
	case structure.Refuse:
		return worker.settle(ticket, structure.Refuse)
	}

	actuals, actualsOK := worker.observeActuals(row, ticket)
	if !actualsOK {
		return worker.settle(ticket, structure.Refuse)
	}

	plan, planOK := DeriveFreezeRoutes(worker.family.heap, worker.family.values, worker.family.calls, worker.family.packs, row.candidate, callFact, actuals)
	if !planOK {
		return worker.settle(ticket, structure.Refuse)
	}
	count := FreezeRouteCount(plan)
	if count < 0 || count > len(worker.members) || count > len(worker.cells) || count > len(worker.routes) {
		return worker.settle(ticket, structure.Refuse)
	}
	members := worker.members[:count]
	cells := worker.cells[:count]
	routes := worker.routes[:count]
	for index := 0; index < count; index++ {
		route, routeOK := FreezeRouteAt(plan, index)
		if !routeOK || route.Tag == 0 {
			return worker.settle(ticket, structure.Refuse)
		}
		dense, denseOK := worker.family.heap.DenseKeyIndex(route.Key)
		if !denseOK {
			return worker.settle(ticket, structure.Refuse)
		}
		// This declaration publishes at the coordinate it observes, so one
		// projection of the relation's member answers both halves. The
		// destination is still stated as a destination: the member carries the
		// read coordinate and the write coordinate separately.
		member, memberOK := worker.family.plane.RouteMember(dense, dense, uint64(route.Tag))
		if !memberOK {
			return worker.settle(ticket, structure.Refuse)
		}
		members[index] = member
		routes[index] = route.Key
	}

	status := row.selected.Observe(ticket, &worker.selected, members, cells)
	if count == 0 {
		if status != execution.ReadExhausted {
			return worker.settle(ticket, structure.Refuse)
		}
	} else if status != execution.ReadAvailable {
		return worker.settle(ticket, structure.Refuse)
	}

	outcome := execution.FoldSelectedRoute(ticket, row.write, &worker.write, cells, members, routes, routeReducer{})
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	written := 0
	if outcome == structure.Concrete {
		written = 1
	}
	return execution.NewResult(outcome, written)
}

// settle submits one non-publishing disposition. Every refusal path in this
// worker ends here so the ticket is settled exactly once.
func (worker *routeWorker) settle(ticket execution.Ticket, outcome structure.ReductionOutcome) (execution.Result, bool) {
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	return execution.NewResult(outcome, 0)
}

// observeActuals reads this call's ordered actual vector.
//
// The vector is Value's own published member set, addressed by (parent,
// ordinal). Which parent row this call has, and where each member's cell
// lives, are coordinates the sealed directory already holds: Value's parent
// order declares that it enumerates the same subjects as Call's mounted-call
// order, so the engine resolves the parent row at the occurrence both
// directories are addressed by, projects every member's coordinate, and seals
// one exact read at each before this row exists. This worker reads those
// cells. It holds no call identity, resolves nothing from one, and re-walks no
// owner geometry.
func (worker *routeWorker) observeActuals(row routeRow, ticket execution.Ticket) (execution.SummaryVector[valuedomain.Value], bool) {
	count := len(row.actuals)
	if count > len(worker.actualCells) {
		return execution.SummaryVector[valuedomain.Value]{}, false
	}
	cells := worker.actualCells[:count]
	for index := 0; index < count; index++ {
		if worker.actualCell(row.actuals[index], row.actualsPolicy, ticket, &cells[index]) != structure.Concrete {
			return execution.SummaryVector[valuedomain.Value]{}, false
		}
	}
	return execution.NewMemberVector(cells)
}

// actualCell reads one member's own coordinate and settles it under the read's
// sealed policy. Absence is carried, not compacted: a cell's position is the
// ordinal its owner declared it at.
func (worker *routeWorker) actualCell(read execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value], policy execution.ReadCellPolicy[valuedomain.Value], ticket execution.Ticket, destination *execution.MemberCell[valuedomain.Value]) structure.ReductionOutcome {
	if worker == nil || destination == nil || !read.Valid() {
		return structure.Refuse
	}
	if read.Read(ticket, &worker.actualRead) != execution.ReadAvailable {
		_ = worker.actualRead.Discard(ticket)
		return structure.Refuse
	}
	cell, available := worker.actualRead.Value()
	present := worker.actualRead.Present()
	if !read.Close(ticket, &worker.actualRead) || !worker.actualRead.Reuse(ticket) {
		_ = worker.actualRead.Discard(ticket)
		return structure.Refuse
	}
	if !available {
		return structure.Refuse
	}
	cell, present = policy.Cell(cell, present)
	*destination = execution.MemberCell[valuedomain.Value]{Value: cell, Present: present}
	return structure.Concrete
}

func (worker *routeWorker) callRead(read execution.ExactRead[calldomain.DenseCoordinate, calldomain.Value], ticket execution.Ticket, destination *calldomain.Value) structure.ReductionOutcome {
	if worker == nil || destination == nil || !read.Valid() {
		return structure.Refuse
	}
	switch read.Read(ticket, &worker.call) {
	case execution.ReadAvailable:
		value, available := worker.call.Value()
		present := worker.call.Present()
		if !read.Close(ticket, &worker.call) {
			_ = worker.call.Discard(ticket)
			return structure.Refuse
		}
		if !available || !present {
			// A mounted call with no published fact has no known target set to
			// read a freeze row from. That is an absent candidate, not a
			// malformed one.
			return structure.NoCandidate
		}
		*destination = value
		return structure.Concrete
	case execution.ReadExhausted:
		if !read.Close(ticket, &worker.call) {
			return structure.Refuse
		}
		return structure.NoCandidate
	default:
		_ = worker.call.Discard(ticket)
		return structure.Refuse
	}
}

type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PackSchema() *packdomain.Schema
}

type installer struct {
	heap   heapdomain.Schema
	values *valuedomain.Schema
	calls  *calldomain.Algebra
	packs  *packdomain.Schema
}

func (install installer) InstallRuleFamily(plane execution.FormPlane[heapdomain.DenseCoordinate, heapdomain.Value], _ uint32, rows []execution.FormRow) (execution.Family, []execution.FormAddress, bool) {
	if !install.heap.Valid() || install.values == nil || !install.values.Valid() || install.calls == nil || !install.calls.Valid() ||
		install.packs == nil || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	routeWidth := plane.RouteWidth()
	if routeWidth < 0 {
		return nil, nil, false
	}
	sealed := &routeFamily{
		heap: install.heap, values: install.values, calls: install.calls, packs: install.packs,
		plane: plane, routeWidth: routeWidth,
		rows: make([]routeRow, 0, len(rows)),
	}
	addresses := make([]execution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		if planRow.Form != execution.FormSelectedRoute || !planRow.Rule.Available() {
			return nil, nil, false
		}
		output, outputOK := planRow.Rule.OutputAt(0)
		first, firstOK := planRow.Rule.ReadAt(0)
		second, secondOK := planRow.Rule.ReadAt(1)
		third, thirdOK := planRow.Rule.ReadAt(2)
		if !outputOK || !firstOK || !secondOK || !thirdOK || output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent ||
			output.RouteJoin != 2 || output.Slot != 0 || first.Form != ruleprogram.Exact ||
			second.Form != ruleprogram.Summary || third.Form != ruleprogram.Selected ||
			planRow.Unit == (execution.SelectedCoordinate{}).Unit {
			return nil, nil, false
		}
		if first.Input > uint32(^uint16(0)) || second.Input > uint32(^uint16(0)) || third.Input > uint32(^uint16(0)) || third.Factor != output.Factor {
			return nil, nil, false
		}
		candidate, candidateOK := install.calls.CallCoordinateAt(int(planRow.Candidate))
		if !candidateOK || !install.calls.OwnsCallCoordinate(candidate) {
			return nil, nil, false
		}
		callForeign, callForeignOK := plane.Foreign(first.Factor)
		valueForeign, valueForeignOK := plane.Foreign(second.Factor)
		if !callForeignOK || !valueForeignOK {
			return nil, nil, false
		}
		callFact, callFactOK := calldomain.ForeignRead(callForeign, execution.SelectedCoordinate{Unit: planRow.Unit}, uint16(first.Input))
		// One exact read per actual, at the coordinate the engine resolved for
		// it. The installer supplies no member and enumerates no set: the row
		// carries this call's member coordinates because the engine resolved
		// Value's parent row for it, through the correspondence the two orders
		// declare.
		actualCount, actualCountOK := planRow.MemberCount(1)
		if !actualCountOK {
			return nil, nil, false
		}
		actuals := make([]execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value], actualCount)
		for index := 0; index < actualCount; index++ {
			memberDense, memberDenseOK := planRow.MemberAt(1, index)
			if !memberDenseOK {
				return nil, nil, false
			}
			memberRead, memberReadOK := execution.ForeignMemberExactRead[valuedomain.DenseCoordinate, valuedomain.Value](valueForeign, memberDense, uint16(second.Input))
			if !memberReadOK {
				return nil, nil, false
			}
			actuals[index] = memberRead
		}
		if actualCount > sealed.actualWidth {
			sealed.actualWidth = actualCount
		}
		actualsPolicy, actualsPolicyOK := execution.ForeignReadCellPolicy[valuedomain.DenseCoordinate, valuedomain.Value](valueForeign, second.Contract)
		// The route selection declares the Factor default at an unwritten
		// coordinate, so the substitution it delivers is sealed here from the
		// owner's own default and top rather than being decided per invocation.
		selected, selectedOK := plane.SelectedRead(uint16(third.Input), third.Contract,
			execution.NewReadCellPolicy(true, install.heap.Default(), install.heap.Top()))
		write, writeOK := plane.RouteWrite(uint16(output.Slot))
		if !callFactOK || !actualsPolicyOK || !selectedOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, execution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, routeRow{
			candidate: candidate, callFact: callFact,
			actuals: actuals, actualsPolicy: actualsPolicy, selected: selected, write: write,
		})
	}
	return sealed, addresses, true
}

// InstallFamily is the freeze rule's one generated RuleFamily claimant. The
// Heap, Value, Call and Pack authorities are captured at bind time. The solve
// loop invokes the route relation's one authored Build once per candidate; it
// reaches no owner callback and reconstructs no second route plan.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	heapAuthority := authorities.HeapAuthority()
	values := authorities.ValueAuthority()
	calls := authorities.CallAuthority()
	packs := authorities.PackSchema()
	if heapAuthority == nil || values == nil || calls == nil || packs == nil ||
		!heapAuthority.MatchesBinding(binding) || !values.MatchesBinding(binding) || !calls.MatchesBinding(binding) {
		return false
	}
	heapSchema := heapAuthority.Schema()
	valueSchema := values.Schema()
	algebra := calls.Algebra()
	if !heapSchema.Valid() || valueSchema == nil || !valueSchema.Valid() || algebra == nil || !algebra.Valid() ||
		!valueSchema.OwnsHeapSchema(heapSchema) || !valueSchema.LinkOwner().Matches(algebra.LinkOwner()) ||
		!packs.LinkOwner().Available() || !packs.LinkOwner().Matches(algebra.LinkOwner()) {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot,
		heapAuthority.FactorRef(),
		installer{heap: heapSchema, values: valueSchema, calls: algebra, packs: packs})
}
