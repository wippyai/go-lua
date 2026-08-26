package contribution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type lawFixture struct {
	fence        binding.Fence
	foreignFence binding.Fence
	state        State
	directory    Directory
	issuer       binding.Issuer
	typeID       model.TypeID
	port         output.OutputPort
	spec         output.ContributionSpec
	lineages     [2]model.LineageRef
	destination  [2]model.RowID
	cells        [2]binding.CellToken
	witness      binding.DenominatorWitness
	scope        binding.ScopeToken
	handles      [2]Handle
}

func contributionFixture(t *testing.T) lawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(identity.ContentID{1})
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, identity.ContentID{2})
	if !ok {
		t.Fatal("schema")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{3}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	foreignFence, ok := binding.NewFence(schema, identity.MountID{4}, identity.Generation(1))
	if !ok {
		t.Fatal("foreign fence")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{5})
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, identity.ContentID{6})
	if !ok {
		t.Fatal("column")
	}
	op, ok := model.IssueOperationID(owner, identity.ContentID{7})
	if !ok {
		t.Fatal("operation")
	}
	typeID, ok := model.IssueTypeID(owner, identity.ContentID{11})
	if !ok {
		t.Fatal("type")
	}
	port := output.OutputPort{Operation: signature.Identity{Operation: op, Version: 1}, Column: column}
	keyID, ok := model.IssueKeyID(relation, identity.ContentID{14})
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, keyID)
	if !ok {
		t.Fatal("denominator")
	}
	var cells [2]binding.CellToken
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	sealedSignature, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: op, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schema},
		Outputs:     []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}},
		Cardinality: cardinality,
	})
	if !ok {
		t.Fatal("signature")
	}
	algebra, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("algebra")
	}
	spec, ok := output.Seal(output.Spec{Signature: sealedSignature, Port: port, ValueType: typeID, Algebra: algebra, Reducer: output.Contributions})
	if !ok {
		t.Fatal("spec")
	}
	lineages := [2]model.LineageRef{}
	for index, content := range []byte{12, 13} {
		lineages[index], ok = model.IssueLineageRef(owner, identity.ContentID{content})
		if !ok {
			t.Fatalf("lineage %d", index)
		}
	}
	rows := [2]model.RowID{}
	for index, content := range []byte{8, 9} {
		rows[index], ok = model.IssueRowID(relation, identity.ContentID{content})
		if !ok {
			t.Fatalf("row %d", index)
		}
	}
	destination, ok := model.IssueRowID(relation, identity.ContentID{10})
	if !ok {
		t.Fatal("destination")
	}
	directory, ok := NewDirectory(fence)
	if !ok {
		t.Fatal("directory")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	var handles [2]Handle
	for index, row := range rows {
		scope, scopeOK := issuer.IssueScope(identity.ContentID{byte(20 + index)})
		if !scopeOK {
			t.Fatalf("scope %d", index)
		}
		tuple, tupleOK := invocation.NewTupleSources([]model.RowID{row})
		if !tupleOK {
			t.Fatalf("tuple %d", index)
		}
		vector, vectorOK := invocation.NewSourceVector([]invocation.TupleSources{tuple})
		if !vectorOK {
			t.Fatalf("vector %d", index)
		}
		address, addressOK := invocation.New(scope, []invocation.SourceVector{vector})
		if !addressOK {
			t.Fatalf("address %d", index)
		}
		directory, handles[index], ok = directory.Intern(address)
		if !ok {
			t.Fatalf("retain handle %d", index)
		}
	}
	membership, ok := binding.NewMembershipView(relation, []model.RowID{destination, rows[0]})
	if !ok {
		t.Fatal("membership")
	}
	witness, ok := issuer.IssueDenominator(denominator, membership, identity.ContentID{15})
	if !ok {
		t.Fatal("witness")
	}
	scope, ok := issuer.IssueScope(identity.ContentID{16})
	if !ok {
		t.Fatal("cell scope")
	}
	for index, row := range []model.RowID{destination, rows[0]} {
		fixtureCell, cellOK := issuer.IssueCell(witness, scope, column, row)
		if !cellOK {
			t.Fatalf("cell %d", index)
		}
		cells[index] = fixtureCell
	}
	state, ok := New(fence)
	if !ok {
		t.Fatal("state")
	}
	return lawFixture{fence: fence, foreignFence: foreignFence, state: state, directory: directory, issuer: issuer, typeID: typeID, port: port, spec: spec, lineages: lineages, destination: [2]model.RowID{destination, rows[0]}, cells: cells, witness: witness, scope: scope, handles: handles}
}

func rowFor(t *testing.T, fixture lawFixture, producer int, destination model.RowID, value byte) Row {
	t.Helper()
	return rowForPresence(t, fixture, producer, destination, value, model.Present)
}

func rowForPresence(t *testing.T, fixture lawFixture, producer int, destination model.RowID, value byte, kind model.PresenceKind) Row {
	t.Helper()
	key, ok := NewKey(fixture.handles[producer], fixture.port, destination)
	if !ok {
		t.Fatal("key")
	}
	presence, ok := model.NewPresence(kind)
	if !ok {
		t.Fatal("presence")
	}
	payload := binding.ValueToken{}
	if kind == model.Present || kind == model.AuthenticatedOpaque {
		payload, ok = fixture.issuer.IssueValue(fixture.typeID, identity.ContentID{value})
		if !ok {
			t.Fatal("payload")
		}
	}
	cell := binding.CellToken{}
	for index, candidate := range fixture.destination {
		if candidate == destination {
			cell = fixture.cells[index]
			break
		}
	}
	row, ok := NewRow(key, cell, payload, presence, fixture.lineages[producer])
	if !ok {
		t.Fatal("row")
	}
	return row
}

func TestContributionDirectoryIsImmutableAndFenceScoped(t *testing.T) {
	fixture := contributionFixture(t)
	address, ok := fixture.directory.Resolve(fixture.handles[0])
	if !ok {
		t.Fatal("first address")
	}
	changed, ok := fixture.directory.Resolve(fixture.handles[1])
	if !ok {
		t.Fatal("second address")
	}
	root, ok := NewDirectory(fixture.fence)
	if !ok {
		t.Fatal("directory")
	}
	first, firstHandle, ok := root.Intern(address)
	if !ok || !first.SuccessorOf(root) || root.Len() != 0 || first.Len() != 1 {
		t.Fatal("first immutable intern")
	}
	repeated, repeatedHandle, ok := first.Intern(address)
	if !ok || !repeated.Same(first) || !repeatedHandle.Same(firstHandle) {
		t.Fatal("repeated address did not reuse root and handle")
	}
	second, secondHandle, ok := first.Intern(changed)
	if !ok || !second.SuccessorOf(first) || !secondHandle.Available() {
		t.Fatal("changed address successor")
	}
	if _, ok := first.Resolve(secondHandle); ok {
		t.Fatal("predecessor resolved a successor handle")
	}
	resolved, ok := second.Resolve(firstHandle)
	if !ok || !resolved.Same(address) {
		t.Fatal("successor lost first structural address")
	}
	if _, comparable := CompareHandles(firstHandle, secondHandle); !comparable {
		t.Fatal("same-directory handles were not comparable")
	}
	other, ok := NewDirectory(fixture.fence)
	if !ok {
		t.Fatal("other directory")
	}
	_, otherHandle, ok := other.Intern(address)
	if !ok {
		t.Fatal("other handle")
	}
	if _, comparable := CompareHandles(firstHandle, otherHandle); comparable {
		t.Fatal("cross-directory handles compared")
	}
	foreignIssuer, ok := binding.NewIssuer(fixture.foreignFence)
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreignScope, ok := foreignIssuer.IssueScope(identity.ContentID{61})
	if !ok {
		t.Fatal("foreign scope")
	}
	foreignTuple, ok := invocation.NewTupleSources([]model.RowID{fixture.destination[0]})
	if !ok {
		t.Fatal("foreign tuple")
	}
	foreignChild, ok := invocation.NewSourceVector([]invocation.TupleSources{foreignTuple})
	if !ok {
		t.Fatal("foreign child")
	}
	foreignAddress, ok := invocation.New(foreignScope, []invocation.SourceVector{foreignChild})
	if !ok {
		t.Fatal("foreign address")
	}
	if _, _, accepted := root.Intern(foreignAddress); accepted {
		t.Fatal("foreign-fence address was interned")
	}
}

func TestTwoProducersSameDestinationAndRemoveOnePreservesOther(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[0], 31)
	second := rowFor(t, fixture, 1, fixture.destination[0], 32)
	state, firstDelta, ok := fixture.state.Upsert(first)
	if !ok || !firstDelta.Changed() {
		t.Fatal("first upsert")
	}
	state, secondDelta, ok := state.Upsert(second)
	if !ok || !secondDelta.Changed() || state.Len() != 2 {
		t.Fatal("second producer was not retained")
	}
	target, targetOK := NewTarget(fixture.port, fixture.destination[0])
	if !targetOK {
		t.Fatal("target")
	}
	rows := state.RowsFor(target)
	if len(rows) != 2 {
		t.Fatalf("same destination rows=%d, want 2", len(rows))
	}
	if compareKey(rows[0].Key, rows[1].Key) >= 0 {
		t.Fatal("producer rows were not canonically ordered")
	}
	state, removedDelta, ok := state.Remove(first.Key)
	if !ok || !removedDelta.Changed() || state.Len() != 1 {
		t.Fatal("remove first producer")
	}
	rows = state.RowsFor(target)
	if len(rows) != 1 || !rows[0].Value.Same(second.Value) || rows[0].Presence != second.Presence || rows[0].Lineage != second.Lineage {
		t.Fatal("removing one producer removed its sibling")
	}
}

func TestReplacementIsOneAtomicProducerRepresentation(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[0], 41)
	replacement := rowFor(t, fixture, 0, fixture.destination[0], 42)
	state, _, ok := fixture.state.Upsert(first)
	if !ok {
		t.Fatal("first upsert")
	}
	next, delta, ok := state.Upsert(replacement)
	if !ok || !delta.Changed() || next.Len() != 1 {
		t.Fatal("replacement was not atomic")
	}
	got, ok := next.Row(replacement.Key)
	if !ok || !got.Value.Same(replacement.Value) || got.Presence != replacement.Presence || got.Lineage != replacement.Lineage {
		t.Fatal("replacement value not represented")
	}
	target, targetOK := NewTarget(fixture.port, fixture.destination[0])
	if !targetOK || len(next.RowsFor(target)) != 1 {
		t.Fatal("replacement left multiple producer rows")
	}
}

func TestAffectedReductionAndRowsAreDeterministic(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[1], 51)
	second := rowFor(t, fixture, 1, fixture.destination[0], 52)
	state, _, ok := fixture.state.Upsert(first)
	if !ok {
		t.Fatal("first upsert")
	}
	state, delta, ok := state.Upsert(second)
	if !ok {
		t.Fatal("second upsert")
	}
	var seen []Target
	if !state.ReduceAffectedTargets(delta, func(target Target, rows []Row) bool {
		if len(rows) != 1 {
			t.Fatalf("destination rows=%d", len(rows))
		}
		seen = append(seen, target)
		return true
	}) {
		t.Fatal("affected reduction refused")
	}
	wantTarget, targetOK := NewTarget(fixture.port, fixture.destination[0])
	if !targetOK || len(seen) != 1 || !seen[0].Same(wantTarget) {
		t.Fatal("reduction did not expose only the changed destination")
	}
	all := state.Targets()
	if len(all) != 2 || compareTarget(all[0], all[1]) >= 0 {
		t.Fatal("targets are not deterministic")
	}
}

func TestProvenAbsentContributionRetainsStatusAndLineage(t *testing.T) {
	fixture := contributionFixture(t)
	absent := rowForPresence(t, fixture, 0, fixture.destination[0], 0, model.ProvenAbsent)
	if absent.Value.Available() || !absent.Presence.Is(model.ProvenAbsent) || !absent.Lineage.Available() {
		t.Fatal("ProvenAbsent payload was not canonical")
	}
	state, delta, ok := fixture.state.Upsert(absent)
	if !ok || !delta.Changed() {
		t.Fatal("ProvenAbsent contribution was rejected")
	}
	got, ok := state.Row(absent.Key)
	if !ok || got.Value.Available() || !got.Presence.Is(model.ProvenAbsent) || got.Lineage != absent.Lineage {
		t.Fatal("ProvenAbsent contribution did not retain status and lineage")
	}
}

func TestForeignFenceIsRejected(t *testing.T) {
	fixture := contributionFixture(t)
	foreignIssuer, ok := binding.NewIssuer(fixture.foreignFence)
	if !ok {
		t.Fatal("foreign issuer")
	}
	scope, ok := foreignIssuer.IssueScope(identity.ContentID{61})
	if !ok {
		t.Fatal("foreign scope")
	}
	tuples, ok := invocation.NewTupleSources([]model.RowID{fixture.destination[0]})
	if !ok {
		t.Fatal("foreign tuple")
	}
	vector, ok := invocation.NewSourceVector([]invocation.TupleSources{tuples})
	if !ok {
		t.Fatal("foreign vector")
	}
	address, ok := invocation.New(scope, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("foreign address")
	}
	foreignDirectory, ok := NewDirectory(fixture.foreignFence)
	if !ok {
		t.Fatal("foreign directory")
	}
	_, handle, ok := foreignDirectory.Intern(address)
	if !ok {
		t.Fatal("foreign handle")
	}
	key, ok := NewKey(handle, fixture.port, fixture.destination[0])
	if !ok {
		t.Fatal("foreign key")
	}
	payload, ok := foreignIssuer.IssueValue(fixture.typeID, identity.ContentID{62})
	if !ok {
		t.Fatal("foreign payload")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("foreign presence")
	}
	foreignKeyID, keyOK := model.IssueKeyID(fixture.port.Column.Relation(), identity.ContentID{64})
	if !keyOK {
		t.Fatal("foreign key")
	}
	foreignDenominator, denominatorOK := model.NewDenominatorRef(fixture.port.Column.Relation(), foreignKeyID)
	if !denominatorOK {
		t.Fatal("foreign denominator")
	}
	foreignMembership, membershipOK := binding.NewMembershipView(fixture.port.Column.Relation(), []model.RowID{fixture.destination[0]})
	if !membershipOK {
		t.Fatal("foreign membership")
	}
	foreignWitness, witnessOK := foreignIssuer.IssueDenominator(foreignDenominator, foreignMembership, identity.ContentID{65})
	if !witnessOK {
		t.Fatal("foreign witness")
	}
	foreignCellScope, scopeOK := foreignIssuer.IssueScope(identity.ContentID{66})
	if !scopeOK {
		t.Fatal("foreign cell scope")
	}
	cell, cellOK := foreignIssuer.IssueCell(foreignWitness, foreignCellScope, fixture.port.Column, fixture.destination[0])
	if !cellOK {
		t.Fatal("foreign cell")
	}
	row, ok := NewRow(key, cell, payload, presence, fixture.lineages[0])
	if !ok {
		t.Fatal("foreign row")
	}
	if _, _, accepted := fixture.state.Upsert(row); accepted {
		t.Fatal("foreign-fence contribution accepted")
	}
}
