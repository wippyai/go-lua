package returnescape

// This file is the generated ReturnEscape execution family.  The engine owns
// the neutral descriptor and the execution cursors; this package owns the
// heterogeneous Value-to-Placement bridge and the one route derivation that
// turns an authenticated ReturnBoundary into Placement RouteMembers.

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// returnEscapeValueMember is one member of Value's owner-issued fixed vector.
// ReturnBoundaryMembers is a self-provided nested member set, not a routed
// publication: the coordinate is already the owner-authenticated dense
// position catalog.boundary resolved through MemberAt/Project, and the exact
// read is sealed directly at that coordinate through
// execution.ForeignMemberExactRead - no RouteTable, RouteMember, or selection
// tag is minted for it.
type returnEscapeValueMember struct {
	coordinate valuedomain.Coordinate
	read       execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
}

type returnEscapeRow struct {
	boundary valuedomain.ReturnBoundary
	root     valuedomain.Coordinate
	rootRead execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	values   []returnEscapeValueMember
	selected execution.SelectedRead[placementdomain.DenseCoordinate, placementdomain.Fact]
	write    execution.RouteWrite[placementdomain.DenseCoordinate, placementdomain.Fact]
}

type returnEscapeFamily struct {
	rows      []returnEscapeRow
	values    *valuedomain.Schema
	placement placementdomain.Schema
	plane     execution.FormPlane[placementdomain.DenseCoordinate, placementdomain.Fact]
	width     int
}

func (family *returnEscapeFamily) NewExecutor(run *execution.Run) execution.Executor {
	if family == nil || run == nil || family.width < 0 {
		return nil
	}
	return &returnEscapeWorker{
		family:  family,
		run:     run,
		members: make([]execution.RouteMember, family.width),
		cells:   make([]execution.SelectedCell[placementdomain.Fact], family.width),
	}
}

func (*returnEscapeFamily) InputCapacity() int  { return 1 }
func (*returnEscapeFamily) OutputCapacity() int { return 1 }

type returnEscapeWorker struct {
	family        *returnEscapeFamily
	run           *execution.Run
	value         execution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	selectScratch execution.SelectedScratch[placementdomain.DenseCoordinate, placementdomain.Fact]
	write         execution.RouteScratch[placementdomain.DenseCoordinate, placementdomain.Fact]
	members       []execution.RouteMember
	cells         []execution.SelectedCell[placementdomain.Fact]
}

type returnEscapeReducer struct{}

// Reduce authenticates the destination's predecessor at this cell's own
// presence, exactly as Store's own routeReducer does: a fresh allocation's
// prior Placement fact is legitimately sparse-absent, and AuthenticateFactCell
// admits that as the Factor's Bottom default rather than this reducer
// fabricating one or refusing a candidate that is not malformed.
func (returnEscapeReducer) Reduce(cell execution.SelectedCell[placementdomain.Fact]) (placementdomain.Fact, structure.ReductionOutcome) {
	current, currentOK := placementdomain.AuthenticateFactCell(cell.Value, cell.Present, true)
	if !currentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return ReturnEscapeFold(cell.Tag, current)
}

func (returnEscapeReducer) Empty() structure.ReductionOutcome { return structure.NoSelection }

func (worker *returnEscapeWorker) Execute(frame execution.Frame, ticket execution.Ticket) (execution.Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || !worker.run.Owns(ticket) {
		return execution.Result{}, false
	}
	local, localOK := ticket.LocalOrdinal()
	if !localOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return execution.Result{}, false
	}
	row := worker.family.rows[local]

	root, rootOutcome := worker.readValue(row.rootRead, row.root, ticket)
	switch rootOutcome {
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

	facts, factsOK := newReturnFacts(len(row.values))
	if !factsOK {
		return worker.settle(ticket, structure.Refuse)
	}
	for index, member := range row.values {
		if !member.read.Valid() {
			return worker.settle(ticket, structure.Refuse)
		}
		item, itemOutcome := worker.readValue(member.read, member.coordinate, ticket)
		// A fixed member vector is a prerequisite, not an optional scan. An
		// exhausted or malformed member therefore refuses the whole candidate.
		if itemOutcome != structure.Concrete || !facts.set(index, item) {
			return worker.settle(ticket, structure.Refuse)
		}
	}
	for index := 0; index < facts.len(); index++ {
		item, itemOK := facts.at(index)
		if !itemOK || !item.available {
			return worker.settle(ticket, structure.Refuse)
		}
	}
	_ = root // The exact root is an authenticated candidate prerequisite.

	// Build is deliberately one invocation-local call. It consumes only the
	// owner-issued fixed member facts and the boundary's sealed tail bit.
	plan, planOK := routePlanForFacts(worker.family.placement, worker.family.values, facts, row.boundary.HasTail())
	if !planOK || plan.class == routeBottom {
		return worker.settle(ticket, structure.Refuse)
	}
	count := plan.routeCount()
	if count < 0 || count > len(worker.members) || count > len(worker.cells) {
		return worker.settle(ticket, structure.Refuse)
	}
	members := worker.members[:count]
	cells := worker.cells[:count]
	for index := 0; index < count; index++ {
		candidate, candidateOK := plan.routeAt(index)
		if !candidateOK || candidate.tag == 0 {
			return worker.settle(ticket, structure.Refuse)
		}
		dense, denseOK := worker.family.placement.Heap().KeyIndex(candidate.key)
		if !denseOK || dense < 0 || uint64(dense) > uint64(^uint32(0)) || uint64(candidate.tag) != uint64(dense)+1 {
			return worker.settle(ticket, structure.Refuse)
		}
		member, memberOK := worker.family.plane.RouteMember(uint32(dense), uint64(candidate.tag))
		if !memberOK || !member.Routed() || member.Tag() != uint64(candidate.tag) {
			return worker.settle(ticket, structure.Refuse)
		}
		members[index] = member
	}

	status := row.selected.Observe(ticket, &worker.selectScratch, members, cells)
	if count == 0 {
		if status != execution.ReadExhausted {
			return worker.settle(ticket, structure.Refuse)
		}
	} else if status != execution.ReadAvailable {
		return worker.settle(ticket, structure.Refuse)
	}
	outcome := execution.FoldSelectedRoute(ticket, row.write, &worker.write, cells, members, returnEscapeReducer{})
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	countOut := 0
	if outcome == structure.Concrete {
		countOut = 1
	}
	return execution.NewResult(outcome, countOut)
}

func (worker *returnEscapeWorker) settle(ticket execution.Ticket, outcome structure.ReductionOutcome) (execution.Result, bool) {
	if worker == nil || !outcome.Available() || !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	return execution.NewResult(outcome, 0)
}

// readValue closes one exact Value cursor before authenticating its returned
// cell. Sparse Bottom is accepted only when Value supplied that exact default;
// a present value must also be admitted at the member's owner coordinate.
func (worker *returnEscapeWorker) readValue(
	read execution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value],
	coordinate valuedomain.Coordinate,
	ticket execution.Ticket,
) (returnFact, structure.ReductionOutcome) {
	if worker == nil || worker.family == nil || !read.Valid() {
		return returnFact{}, structure.Refuse
	}
	readStatus := read.Read(ticket, &worker.value)
	switch readStatus {
	case execution.ReadAvailable:
		fact, available := worker.value.Value()
		present := worker.value.Present()
		// One invocation reads the exact root and then every fixed member off
		// the same shared Scratch lane. Closing a read leaves that lane bound to
		// this still-open Ticket, so the next Read on it must go through Reuse
		// first, exactly as product.go's own linear refinement cut does; without
		// it every read past the first on this lane refuses.
		if !read.Close(ticket, &worker.value) || !worker.value.Reuse(ticket) {
			_ = worker.value.Discard(ticket)
			return returnFact{}, structure.Refuse
		}
		if !authenticatedReturnFact(worker.family.values, fact, present, available) {
			return returnFact{}, structure.Refuse
		}
		if present && !worker.family.values.AdmitsCoordinate(coordinate, fact) {
			return returnFact{}, structure.Refuse
		}
		return returnFact{fact: fact, present: present, available: available}, structure.Concrete
	case execution.ReadExhausted:
		if !read.Close(ticket, &worker.value) || !worker.value.Reuse(ticket) {
			return returnFact{}, structure.Refuse
		}
		return returnFact{}, structure.NoCandidate
	default:
		_ = worker.value.Discard(ticket)
		return returnFact{}, structure.Refuse
	}
}

// returnEscapeCatalog is the owner-issued Value relation/projection census
// captured while the family is sealed. No runtime path reopens the catalog or
// scans a member directory.
type returnEscapeCatalog struct {
	candidates uint32
	roots      uint32
	members    uint32
	rootKey    uint32
	memberKey  uint32
}

func newReturnEscapeCatalog(values *valuedomain.Schema) (returnEscapeCatalog, *valuedomain.RelationOwner, bool) {
	if values == nil || !values.Valid() {
		return returnEscapeCatalog{}, nil, false
	}
	catalog := valuedomain.AxisMemberCatalog()
	candidates, candidatesOK := catalog.RelationOrdinal(valuedomain.ReturnBoundaryCandidates)
	roots, rootsOK := catalog.RelationOrdinal(valuedomain.ReturnBoundaryRoots)
	members, membersOK := catalog.RelationOrdinal(valuedomain.ReturnBoundaryMembers)
	rootKey, rootKeyOK := catalog.ProjectionOrdinal(valuedomain.ReturnBoundaryRootKey)
	memberKey, memberKeyOK := catalog.ProjectionOrdinal(valuedomain.ReturnBoundaryMemberKey)
	owner := valuedomain.NewRelationOwner(values)
	if !candidatesOK || !rootsOK || !membersOK || !rootKeyOK || !memberKeyOK || owner == nil {
		return returnEscapeCatalog{}, nil, false
	}
	return returnEscapeCatalog{
		candidates: candidates,
		roots:      roots,
		members:    members,
		rootKey:    rootKey,
		memberKey:  memberKey,
	}, owner, true
}

func (catalog returnEscapeCatalog) boundary(
	values *valuedomain.Schema,
	owner *valuedomain.RelationOwner,
	candidate uint32,
) (valuedomain.ReturnBoundary, valuedomain.Coordinate, []returnEscapeValueMember, bool) {
	if values == nil || owner == nil {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	boundary, boundaryOK := values.ReturnBoundaryAt(int(candidate))
	if !boundaryOK || !values.OwnsReturnBoundary(boundary) {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	ordinal, ordinalOK := values.ReturnBoundaryOrdinal(boundary)
	if !ordinalOK || ordinal != candidate {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	root, rootOK := boundary.Root()
	rootIndex, rootIndexOK := values.CoordinateIndex(root)
	projectedRoot, projectedRootOK := owner.Project(catalog.roots, catalog.rootKey, candidate)
	if !rootOK || !rootIndexOK || !projectedRootOK || projectedRoot != rootIndex {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	count := boundary.MemberCount()
	if count < 0 {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	census, censusOK := owner.MemberCount(catalog.members, candidate)
	if !censusOK || census != count {
		return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
	}
	members := make([]returnEscapeValueMember, count)
	for index := 0; index < count; index++ {
		memberDense, memberDenseOK := owner.MemberAt(catalog.members, candidate, index)
		member, memberOK := boundary.MemberAt(index)
		coordinate, coordinateOK := member.Coordinate()
		row, rowOK := values.ReturnBoundaryMemberAt(int(memberDense))
		rowCoordinate, rowCoordinateOK := row.Coordinate()
		memberOrdinal, memberOrdinalOK := values.ReturnBoundaryMemberOrdinal(row)
		coordinateIndex, coordinateIndexOK := values.CoordinateIndex(coordinate)
		projected, projectedOK := owner.Project(catalog.members, catalog.memberKey, memberDense)
		if !memberDenseOK || !memberOK || !coordinateOK || !rowOK || !rowCoordinateOK || !memberOrdinalOK || memberOrdinal != memberDense || !coordinateIndexOK || !projectedOK || projected != coordinateIndex || rowCoordinate != coordinate {
			return valuedomain.ReturnBoundary{}, valuedomain.Coordinate{}, nil, false
		}
		members[index].coordinate = coordinate
	}
	return boundary, root, members, true
}

func returnEscapeContract(read generated.ReadPlan, form ruleprogram.ReadForm, sparse ruleprogram.Sparse, opaque ruleprogram.OnOpaque) bool {
	return read.Form == form && read.Contract.Order == ruleprogram.OrderCanonical &&
		read.Contract.Sparse == sparse && read.Contract.OnOpaque == opaque &&
		read.Contract.Multiplicity == ruleprogram.MultiplicityOne && read.Denominator.Present
}

type returnEscapeInstaller struct {
	values    *valuedomain.Schema
	placement placementdomain.Schema
	rule      uint32
}

func (install returnEscapeInstaller) InstallRuleFamily(
	plane execution.FormPlane[placementdomain.DenseCoordinate, placementdomain.Fact],
	ruleOrdinal uint32,
	rows []execution.FormRow,
) (execution.Family, []execution.FormAddress, bool) {
	if install.values == nil || !install.values.Valid() || !install.placement.Valid() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	width := plane.RouteWidth()
	if width < 0 {
		return nil, nil, false
	}
	catalog, owner, catalogOK := newReturnEscapeCatalog(install.values)
	if !catalogOK {
		return nil, nil, false
	}
	sealed := &returnEscapeFamily{
		values:    install.values,
		placement: install.placement,
		plane:     plane,
		width:     width,
		rows:      make([]returnEscapeRow, 0, len(rows)),
	}
	addresses := make([]execution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		if !returnEscapeRuleShape(planRow.Rule) || planRow.Form != execution.FormSelectedRoute {
			return nil, nil, false
		}
		first, firstOK := planRow.Rule.ReadAt(0)
		second, secondOK := planRow.Rule.ReadAt(1)
		third, thirdOK := planRow.Rule.ReadAt(2)
		output, outputOK := planRow.Rule.OutputAt(0)
		if !firstOK || !secondOK || !thirdOK || !outputOK || first.Factor != second.Factor || third.Factor != output.Factor {
			return nil, nil, false
		}

		boundary, root, members, boundaryOK := catalog.boundary(install.values, owner, planRow.Candidate)
		if !boundaryOK {
			return nil, nil, false
		}
		// first and second both name Value's Factor (checked above): the root
		// and every fixed member read the same foreign axis, so one resolved
		// handle serves both. Each member's coordinate is already the
		// owner-authenticated dense position catalog.boundary resolved through
		// Value's own MemberAt/Project directory - there is no selection to
		// resolve here, only an exact read to seal at a coordinate the caller
		// already holds.
		foreign, foreignOK := plane.Foreign(first.Factor)
		if !foreignOK {
			return nil, nil, false
		}
		rootRead, rootReadOK := execution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, planRow, 0)
		if !rootReadOK {
			return nil, nil, false
		}
		for index := range members {
			coordinateIndex, coordinateIndexOK := install.values.CoordinateIndex(members[index].coordinate)
			if !coordinateIndexOK {
				return nil, nil, false
			}
			read, readOK := execution.ForeignMemberExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, coordinateIndex, 0)
			if !readOK {
				return nil, nil, false
			}
			members[index].read = read
		}
		selected, selectedOK := plane.SelectedRead(0, third.Contract, execution.ReadCellPolicy[placementdomain.Fact]{})
		write, writeOK := plane.RouteWrite(uint16(output.Slot))
		if !selectedOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, execution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, returnEscapeRow{
			boundary: boundary,
			root:     root,
			rootRead: rootRead,
			values:   members,
			selected: selected,
			write:    write,
		})
	}
	return sealed, addresses, true
}

func returnEscapeRuleShape(rule generated.CompiledRule) bool {
	if !rule.Available() || rule.ReadCount() != 3 || rule.OutputCount() != 1 {
		return false
	}
	if _, carryPresent := rule.CarryMode(); carryPresent {
		return false
	}
	output, outputOK := rule.OutputAt(0)
	first, firstOK := rule.ReadAt(0)
	second, secondOK := rule.ReadAt(1)
	third, thirdOK := rule.ReadAt(2)
	_, firstPredicate, firstPredicateOK := rule.ReadPredicateAt(0)
	_, secondPredicate, secondPredicateOK := rule.ReadPredicateAt(1)
	_, thirdPredicate, thirdPredicateOK := rule.ReadPredicateAt(2)
	if !outputOK || !firstOK || !secondOK || !thirdOK || !firstPredicateOK || !secondPredicateOK || !thirdPredicateOK ||
		firstPredicate || secondPredicate || thirdPredicate || output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Slot != 0 ||
		first.Form != ruleprogram.Exact || second.Form != ruleprogram.Selected || third.Form != ruleprogram.Selected ||
		first.Input != 0 || second.Input != 0 || third.Input != 0 || first.Factor != second.Factor {
		return false
	}
	return returnEscapeContract(first, ruleprogram.Exact, ruleprogram.SparseDefault, ruleprogram.OnOpaquePropagateAuthenticated) &&
		returnEscapeContract(second, ruleprogram.Selected, ruleprogram.SparseDefault, ruleprogram.OnOpaquePropagateAuthenticated) &&
		returnEscapeContract(third, ruleprogram.Selected, ruleprogram.SparseDefault, ruleprogram.OnOpaqueRefuse)
}

type returnEscapeAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

// InstallFamily is ReturnEscape's generated RuleFamily claimant. The Value
// owner issues the candidate/root/member catalog; the Placement plane issues
// the paired RouteMember geometry. No legacy RuleSlot, local key directory,
// or runtime route fallback participates in this path.
func InstallFamily[A returnEscapeAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
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
	return engine.BindRuleFamily[placementdomain.DenseCoordinate](binding, slot, placement.FactorRef(), returnEscapeInstaller{values: valueSchema, placement: placementSchema, rule: uint32(ordinal)})
}
