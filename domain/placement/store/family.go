package store

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/execution"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// routeReducer is the typed semantic half of one Store invocation. The
// selected member and its route tag are delivered by the same RouteMember
// vector; Store only supplies its sealed candidate and exact Value source.
type routeReducer struct {
	candidate valuedomain.StorageTransfer
	source    valuedomain.Value
}

func (reducer routeReducer) Reduce(cell execution.SelectedCell[placementdomain.Fact]) (placementdomain.Fact, structure.ReductionOutcome) {
	// SelectedRead preserves sparse provenance.  An absent cell is still a
	// delivered Placement Default under this read's explicit-sparsity contract;
	// authenticate that owner-issued default before handing the value to the
	// direct reducer.  Refusing every absent cell here would reject the normal
	// first store into an unwritten allocation root.
	selected, selectedOK := placementdomain.AuthenticateFactCell(cell.Value, cell.Present, true)
	if !selectedOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return StorageFold(reducer.candidate, reducer.source, cell.Tag, selected)
}

func (routeReducer) Empty() structure.ReductionOutcome { return structure.NoSelection }

type routeRow struct {
	candidate valuedomain.StorageTransfer
	source    execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	selected  execution.SelectedRead[placementdomain.DenseCoordinate, placementdomain.Fact]
	write     execution.RouteWrite[placementdomain.DenseCoordinate, placementdomain.Fact]
}

type routeFamily struct {
	rows      []routeRow
	values    *valuedomain.Schema
	placement placementdomain.Schema
	plane     execution.FormPlane[placementdomain.DenseCoordinate, placementdomain.Fact]
	width     int
}

func (family *routeFamily) NewExecutor(run *execution.Run) execution.Executor {
	if family == nil || run == nil || family.width < 0 {
		return nil
	}
	return &routeWorker{
		family:  family,
		run:     run,
		members: make([]execution.RouteMember, family.width),
		cells:   make([]execution.SelectedCell[placementdomain.Fact], family.width),
	}
}

func (*routeFamily) InputCapacity() int  { return 1 }
func (*routeFamily) OutputCapacity() int { return 1 }

type routeWorker struct {
	family   *routeFamily
	run      *execution.Run
	source   execution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	selected execution.SelectedScratch[placementdomain.DenseCoordinate, placementdomain.Fact]
	write    execution.RouteScratch[placementdomain.DenseCoordinate, placementdomain.Fact]
	members  []execution.RouteMember
	cells    []execution.SelectedCell[placementdomain.Fact]
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

	var source valuedomain.Value
	switch worker.sourceRead(row.source, ticket, &source) {
	case structure.NoCandidate:
		if !ticket.Submit(structure.NoCandidate) {
			return execution.Result{}, false
		}
		return execution.NewResult(structure.NoCandidate, 0)
	case structure.Refuse:
		if !ticket.Submit(structure.Refuse) {
			return execution.Result{}, false
		}
		return execution.NewResult(structure.Refuse, 0)
	}

	plan, planOK := DeriveRoutes(worker.family.placement, worker.family.values, row.candidate, source)
	if !planOK {
		if !ticket.Submit(structure.Refuse) {
			return execution.Result{}, false
		}
		return execution.NewResult(structure.Refuse, 0)
	}
	count := RouteCount(plan)
	if count < 0 || count > len(worker.members) || count > len(worker.cells) {
		if !ticket.Submit(structure.Refuse) {
			return execution.Result{}, false
		}
		return execution.NewResult(structure.Refuse, 0)
	}
	members := worker.members[:count]
	cells := worker.cells[:count]
	for index := 0; index < count; index++ {
		route, routeOK := RouteAt(plan, index)
		if !routeOK || route.Tag == 0 {
			if !ticket.Submit(structure.Refuse) {
				return execution.Result{}, false
			}
			return execution.NewResult(structure.Refuse, 0)
		}
		dense, denseOK := worker.family.placement.Heap().KeyIndex(route.Key)
		if !denseOK || dense < 0 || uint64(dense) > uint64(^uint32(0)) {
			if !ticket.Submit(structure.Refuse) {
				return execution.Result{}, false
			}
			return execution.NewResult(structure.Refuse, 0)
		}
		member, memberOK := worker.family.plane.RouteMember(uint32(dense), route.Tag)
		if !memberOK {
			if !ticket.Submit(structure.Refuse) {
				return execution.Result{}, false
			}
			return execution.NewResult(structure.Refuse, 0)
		}
		members[index] = member
	}

	status := row.selected.Observe(ticket, &worker.selected, members, cells)
	if count == 0 {
		if status != execution.ReadExhausted {
			if !ticket.Submit(structure.Refuse) {
				return execution.Result{}, false
			}
			return execution.NewResult(structure.Refuse, 0)
		}
	} else if status != execution.ReadAvailable {
		if !ticket.Submit(structure.Refuse) {
			return execution.Result{}, false
		}
		return execution.NewResult(structure.Refuse, 0)
	}

	outcome := execution.FoldSelectedRoute(ticket, row.write, &worker.write, cells, members, routeReducer{candidate: row.candidate, source: source})
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	countOut := 0
	if outcome == structure.Concrete {
		countOut = 1
	}
	return execution.NewResult(outcome, countOut)
}

func (worker *routeWorker) sourceRead(read execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value], ticket execution.Ticket, destination *valuedomain.Value) structure.ReductionOutcome {
	if worker == nil || destination == nil || !read.Valid() {
		return structure.Refuse
	}
	switch read.Read(ticket, &worker.source) {
	case execution.ReadAvailable:
		value, available := worker.source.Value()
		present := worker.source.Present()
		if !read.Close(ticket, &worker.source) {
			_ = worker.source.Discard(ticket)
			return structure.Refuse
		}
		value, authenticated := authenticatedSource(worker.family.values, value, present, available)
		if !authenticated {
			return structure.Refuse
		}
		*destination = value
		return structure.Concrete
	case execution.ReadExhausted:
		if !read.Close(ticket, &worker.source) {
			return structure.Refuse
		}
		return structure.NoCandidate
	default:
		_ = worker.source.Discard(ticket)
		return structure.Refuse
	}
}

// authenticatedSource accepts only a present Value fact or the exact sparse
// Bottom issued by this Value schema. It never constructs a default from
// metadata and refuses foreign or malformed evidence before DeriveRoutes.
func authenticatedSource(schema *valuedomain.Schema, fact valuedomain.Value, present, available bool) (valuedomain.Value, bool) {
	if schema == nil || !available || !schema.Equal(fact, fact) || !present && !schema.Equal(fact, schema.Bottom()) {
		return valuedomain.Value{}, false
	}
	return fact, true
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

type installer struct {
	values    *valuedomain.Schema
	placement placementdomain.Schema
	rule      uint32
}

func (install installer) InstallRuleFamily(plane execution.FormPlane[placementdomain.DenseCoordinate, placementdomain.Fact], ruleOrdinal uint32, rows []execution.FormRow) (execution.Family, []execution.FormAddress, bool) {
	if install.values == nil || !install.values.Valid() || !install.placement.Valid() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	width := plane.RouteWidth()
	if width < 0 {
		return nil, nil, false
	}
	sealed := &routeFamily{
		values:    install.values,
		placement: install.placement,
		plane:     plane,
		width:     width,
		rows:      make([]routeRow, 0, len(rows)),
	}
	addresses := make([]execution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		if planRow.Form != execution.FormSelectedRoute || !planRow.Rule.Available() {
			return nil, nil, false
		}
		output, outputOK := planRow.Rule.OutputAt(0)
		first, firstOK := planRow.Rule.ReadAt(0)
		second, secondOK := planRow.Rule.ReadAt(1)
		if !outputOK || !firstOK || !secondOK || output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 1 || output.Slot > uint32(^uint16(0)) || first.Form != ruleprogram.Exact || second.Form != ruleprogram.Selected || planRow.Unit == (execution.SelectedCoordinate{}).Unit {
			return nil, nil, false
		}
		if first.Input > uint32(^uint16(0)) || second.Input > uint32(^uint16(0)) || output.Slot != 0 || second.Factor != output.Factor {
			return nil, nil, false
		}
		candidate, candidateOK := install.values.StorageTransferAt(int(planRow.Candidate))
		if !candidateOK || !install.values.OwnsStorageTransfer(candidate) {
			return nil, nil, false
		}
		foreign, foreignOK := plane.Foreign(first.Factor)
		if !foreignOK {
			return nil, nil, false
		}
		source, sourceOK := valuedomain.ForeignRead(foreign, execution.SelectedCoordinate{Unit: planRow.Unit}, uint16(first.Input))
		if !sourceOK {
			return nil, nil, false
		}
		selected, selectedOK := plane.SelectedRead(uint16(second.Input), second.Contract, execution.ReadCellPolicy[placementdomain.Fact]{})
		write, writeOK := plane.RouteWrite(uint16(output.Slot))
		if !selectedOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, execution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, routeRow{candidate: candidate, source: source, selected: selected, write: write})
	}
	return sealed, addresses, true
}

// InstallFamily is Store's one generated RuleFamily claimant. The placement
// and Value schemas are captured at bind time. The solve loop invokes the
// relation's one authored DeriveRoutes Build once per candidate; it reaches no
// owner callback and reconstructs no second route plan.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	placement := authorities.PlacementAuthority()
	values := authorities.ValueAuthority()
	placementSchema := authorities.PlacementSchema()
	valueSchema := authorities.ValueSchema()
	if placement == nil || values == nil || valueSchema == nil || !placementSchema.Valid() || !valueSchema.Valid() || !placement.Schema().Valid() || !values.Schema().Valid() || placementSchema.ContentID() != placement.Schema().ContentID() || values.Schema() != valueSchema {
		return false
	}
	ordinal, ordinalOK := slot.Ordinal()
	if !ordinalOK || ordinal > uint64(^uint32(0)) {
		return false
	}
	return engine.BindRuleFamily[placementdomain.DenseCoordinate](binding, slot, placement.FactorRef(), installer{values: valueSchema, placement: placementSchema, rule: uint32(ordinal)})
}
