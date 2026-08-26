package contribution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
)

func addressForHandle(t *testing.T, fixture lawFixture, handle Handle) invocation.InvocationAddress {
	t.Helper()
	address, ok := fixture.directory.Resolve(handle)
	if !ok {
		t.Fatal("address")
	}
	return address
}

func sideForRow(t *testing.T, row Row) binding.ContributionSide {
	t.Helper()
	side, ok := binding.NewContributionSide(row.Value, row.Presence, row.Lineage)
	if !ok {
		t.Fatal("side")
	}
	return side
}

func transitionFor(t *testing.T, fixture lawFixture, address invocation.InvocationAddress, cell binding.CellToken, before, after binding.ContributionSide) invocation.ContributionTransition {
	t.Helper()
	transition, ok := invocation.NewContributionTransition(fixture.spec, address, cell, fixture.fence, before, after)
	if !ok {
		t.Fatal("transition")
	}
	return transition
}

func newAddress(t *testing.T, fixture lawFixture, scopeByte byte, source model.RowID) invocation.InvocationAddress {
	t.Helper()
	scope, ok := fixture.issuer.IssueScope(identity.ContentID{scopeByte})
	if !ok {
		t.Fatal("scope")
	}
	tuple, ok := invocation.NewTupleSources([]model.RowID{source})
	if !ok {
		t.Fatal("tuple")
	}
	vector, ok := invocation.NewSourceVector([]invocation.TupleSources{tuple})
	if !ok {
		t.Fatal("vector")
	}
	address, ok := invocation.New(scope, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("address")
	}
	return address
}

func TestApplyTransitionsInsertsOneContributionAtomically(t *testing.T) {
	fixture := contributionFixture(t)
	row := rowFor(t, fixture, 0, fixture.destination[0], 71)
	transition := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), row.Cell(), binding.NoContributionSide(), sideForRow(t, row))
	nextDirectory, nextState, delta, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{transition})
	if !ok || !nextDirectory.Same(fixture.directory) || !nextState.SuccessorOf(fixture.state) || !delta.Available() || !delta.Changed() {
		t.Fatal("atomic insertion")
	}
	if nextState.Len() != 1 || len(delta.AffectedTargets()) != 1 {
		t.Fatal("insertion result")
	}
	got, present := nextState.Row(row.Key)
	if !present || !got.Cell().Same(row.Cell()) {
		t.Fatal("inserted row missing")
	}
}

func TestApplyTransitionsDeletesExactProducerAndPreservesSibling(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[0], 72)
	second := rowFor(t, fixture, 1, fixture.destination[0], 73)
	insertFirst := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), first.Cell(), binding.NoContributionSide(), sideForRow(t, first))
	insertSecond := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[1]), second.Cell(), binding.NoContributionSide(), sideForRow(t, second))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{insertFirst, insertSecond})
	if !ok {
		t.Fatal("seed contributions")
	}
	remove := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), first.Cell(), sideForRow(t, first), binding.NoContributionSide())
	finalDirectory, finalState, delta, ok := ApplyTransitions(directory, state, []invocation.ContributionTransition{remove})
	if !ok || !finalDirectory.Same(directory) || !finalState.SuccessorOf(state) || !delta.Changed() {
		t.Fatal("exact removal")
	}
	target, ok := NewTarget(fixture.port, fixture.destination[0])
	if !ok || len(finalState.RowsFor(target)) != 1 || !finalState.RowsFor(target)[0].Value.Same(second.Value) {
		t.Fatal("sibling contribution was not preserved")
	}
}

func TestApplyTransitionsDeletesFinalProducerWithValidEmptySuccessor(t *testing.T) {
	fixture := contributionFixture(t)
	row := rowFor(t, fixture, 0, fixture.destination[0], 73)
	seed := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), row.Cell(), binding.NoContributionSide(), sideForRow(t, row))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{seed})
	if !ok || state.Len() != 1 {
		t.Fatal("seed final producer")
	}
	remove := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), row.Cell(), sideForRow(t, row), binding.NoContributionSide())
	nextDirectory, nextState, delta, ok := ApplyTransitions(directory, state, []invocation.ContributionTransition{remove})
	if !ok || !nextDirectory.Same(directory) || !nextState.SuccessorOf(state) || nextState.Len() != 0 || !delta.Available() || !delta.Changed() {
		t.Fatal("final producer removal")
	}
}

func TestApplyTransitionsReplacesExactProducerAtomically(t *testing.T) {
	fixture := contributionFixture(t)
	old := rowFor(t, fixture, 0, fixture.destination[0], 74)
	newValue := rowFor(t, fixture, 0, fixture.destination[0], 75)
	insert := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), binding.NoContributionSide(), sideForRow(t, old))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{insert})
	if !ok {
		t.Fatal("seed replacement")
	}
	replace := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), sideForRow(t, old), sideForRow(t, newValue))
	finalDirectory, finalState, delta, ok := ApplyTransitions(directory, state, []invocation.ContributionTransition{replace})
	if !ok || !finalDirectory.Same(directory) || !finalState.SuccessorOf(state) || !delta.Changed() {
		t.Fatal("replacement")
	}
	got, ok := finalState.Row(newValue.Key)
	if !ok || !got.Value.Same(newValue.Value) || !got.Destination.Same(newValue.Destination) {
		t.Fatal("replacement payload")
	}
}

func TestApplyTransitionsAfterOnlyReplacesExistingProducer(t *testing.T) {
	fixture := contributionFixture(t)
	old := rowFor(t, fixture, 0, fixture.destination[0], 108)
	updated := rowFor(t, fixture, 0, fixture.destination[0], 109)
	seed := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), binding.NoContributionSide(), sideForRow(t, old))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{seed})
	if !ok {
		t.Fatal("seed after-only replacement")
	}
	afterOnly := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), updated.Cell(), binding.NoContributionSide(), sideForRow(t, updated))
	_, finalState, delta, ok := ApplyTransitions(directory, state, []invocation.ContributionTransition{afterOnly})
	if !ok || !delta.Changed() {
		t.Fatal("after-only replacement refused")
	}
	got, ok := finalState.Row(updated.Key)
	if !ok || !got.Value.Same(updated.Value) {
		t.Fatal("after-only replacement did not replace")
	}
}

func TestApplyTransitionsPreservesTwoProducersAtOneTarget(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[0], 76)
	second := rowFor(t, fixture, 1, fixture.destination[0], 77)
	firstTransition := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), first.Cell(), binding.NoContributionSide(), sideForRow(t, first))
	secondTransition := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[1]), second.Cell(), binding.NoContributionSide(), sideForRow(t, second))
	_, state, delta, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{firstTransition, secondTransition})
	if !ok || !delta.Changed() {
		t.Fatal("two-producer batch")
	}
	target, targetOK := NewTarget(fixture.port, fixture.destination[0])
	if !targetOK || len(state.RowsFor(target)) != 2 || len(delta.AffectedTargets()) != 1 || !delta.AffectedTargets()[0].Same(target) {
		t.Fatal("target grouping collapsed producers")
	}
}

func TestContributionTargetsRetainOutputPortIdentityForOneRow(t *testing.T) {
	fixture := contributionFixture(t)
	secondColumn, ok := model.IssueColumnID(fixture.port.Column.Relation(), identity.ContentID{105})
	if !ok {
		t.Fatal("second column")
	}
	secondPort := fixture.port
	secondPort.Column = secondColumn
	secondCell, ok := fixture.issuer.IssueCell(fixture.witness, fixture.scope, secondColumn, fixture.destination[0])
	if !ok {
		t.Fatal("second cell")
	}
	key, ok := NewKey(fixture.handles[0], secondPort, fixture.destination[0])
	if !ok {
		t.Fatal("second key")
	}
	value, ok := fixture.issuer.IssueValue(fixture.typeID, identity.ContentID{106})
	if !ok {
		t.Fatal("second value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("second presence")
	}
	secondRow, ok := NewRow(key, secondCell, value, presence, fixture.lineages[0])
	if !ok {
		t.Fatal("second row")
	}
	firstRow := rowFor(t, fixture, 0, fixture.destination[0], 107)
	state, _, ok := fixture.state.Upsert(firstRow)
	if !ok {
		t.Fatal("first target row")
	}
	state, _, ok = state.Upsert(secondRow)
	if !ok {
		t.Fatal("second target row")
	}
	firstTarget, firstOK := NewTarget(fixture.port, fixture.destination[0])
	secondTarget, secondOK := NewTarget(secondPort, fixture.destination[0])
	if !firstOK || !secondOK || firstTarget.Same(secondTarget) || len(state.Targets()) != 2 || len(state.RowsFor(firstTarget)) != 1 || len(state.RowsFor(secondTarget)) != 1 {
		t.Fatal("output ports were collapsed into one target")
	}
}

func TestApplyTransitionsInternsMultipleNewAddressesInOneDirectorySuccessor(t *testing.T) {
	fixture := contributionFixture(t)
	rootDirectory, ok := NewDirectory(fixture.fence)
	if !ok {
		t.Fatal("root directory")
	}
	firstAddress := newAddress(t, fixture, 81, fixture.destination[0])
	secondAddress := newAddress(t, fixture, 82, fixture.destination[1])
	first := rowFor(t, fixture, 0, fixture.destination[0], 78)
	second := rowFor(t, fixture, 1, fixture.destination[1], 79)
	firstTransition := transitionFor(t, fixture, firstAddress, first.Cell(), binding.NoContributionSide(), sideForRow(t, first))
	secondTransition := transitionFor(t, fixture, secondAddress, second.Cell(), binding.NoContributionSide(), sideForRow(t, second))
	nextDirectory, nextState, delta, ok := ApplyTransitions(rootDirectory, fixture.state, []invocation.ContributionTransition{firstTransition, secondTransition})
	if !ok || !nextDirectory.SuccessorOf(rootDirectory) || nextDirectory.Len() != 2 || !nextState.SuccessorOf(fixture.state) || nextState.Len() != 2 || !delta.Changed() {
		t.Fatal("multi-address batch was not one direct successor")
	}
	if _, ok := rootDirectory.Resolve(nextState.Rows()[0].Key.Invocation); ok {
		t.Fatal("predecessor directory resolved new handle")
	}
}

func TestApplyTransitionsRejectsStaleBeforeWithoutMutation(t *testing.T) {
	fixture := contributionFixture(t)
	old := rowFor(t, fixture, 0, fixture.destination[0], 80)
	seed := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), binding.NoContributionSide(), sideForRow(t, old))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{seed})
	if !ok {
		t.Fatal("seed stale test")
	}
	stale := rowFor(t, fixture, 0, fixture.destination[0], 81)
	transition := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), sideForRow(t, stale), binding.NoContributionSide())
	if _, _, _, accepted := ApplyTransitions(directory, state, []invocation.ContributionTransition{transition}); accepted {
		t.Fatal("stale predecessor accepted")
	}
}

func TestApplyTransitionsRejectsForeignAndMismatchedBefore(t *testing.T) {
	fixture := contributionFixture(t)
	old := rowFor(t, fixture, 0, fixture.destination[0], 82)
	seed := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), binding.NoContributionSide(), sideForRow(t, old))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{seed})
	if !ok {
		t.Fatal("seed foreign test")
	}
	mismatch := rowFor(t, fixture, 0, fixture.destination[0], 83)
	transition := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), old.Cell(), sideForRow(t, mismatch), sideForRow(t, old))
	if _, _, _, accepted := ApplyTransitions(directory, state, []invocation.ContributionTransition{transition}); accepted {
		t.Fatal("mismatched predecessor accepted")
	}
	foreignFence, ok := binding.NewFence(fixture.fence.Schema(), identity.MountID{99}, fixture.fence.Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	foreignIssuer, ok := binding.NewIssuer(foreignFence)
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreignScope, ok := foreignIssuer.IssueScope(identity.ContentID{100})
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
	foreignAddress, ok := invocation.New(foreignScope, []invocation.SourceVector{vector})
	if !ok {
		t.Fatal("foreign address")
	}
	foreignKey, keyOK := model.IssueKeyID(fixture.port.Column.Relation(), identity.ContentID{102})
	if !keyOK {
		t.Fatal("foreign key")
	}
	foreignDenominator, denominatorOK := model.NewDenominatorRef(fixture.port.Column.Relation(), foreignKey)
	if !denominatorOK {
		t.Fatal("foreign denominator")
	}
	foreignMembership, membershipOK := binding.NewMembershipView(fixture.port.Column.Relation(), []model.RowID{fixture.destination[0]})
	if !membershipOK {
		t.Fatal("foreign membership")
	}
	foreignWitness, witnessOK := foreignIssuer.IssueDenominator(foreignDenominator, foreignMembership, identity.ContentID{103})
	if !witnessOK {
		t.Fatal("foreign witness")
	}
	foreignCellScope, scopeOK := foreignIssuer.IssueScope(identity.ContentID{104})
	if !scopeOK {
		t.Fatal("foreign cell scope")
	}
	foreignCell, cellOK := foreignIssuer.IssueCell(foreignWitness, foreignCellScope, fixture.port.Column, fixture.destination[0])
	if !cellOK {
		t.Fatal("foreign cell")
	}
	foreignTransition, ok := invocation.NewContributionTransition(fixture.spec, foreignAddress, foreignCell, foreignFence, binding.NoContributionSide(), sideForRow(t, old))
	if ok {
		t.Fatal("foreign transition unexpectedly accepted base payload")
	}
	foreignValue, valueOK := foreignIssuer.IssueValue(fixture.typeID, identity.ContentID{101})
	if !valueOK {
		t.Fatal("foreign value")
	}
	foreignSide, sideOK := binding.NewContributionSide(foreignValue, old.Presence, old.Lineage)
	if !sideOK {
		t.Fatal("foreign side")
	}
	foreignTransition, ok = invocation.NewContributionTransition(fixture.spec, foreignAddress, foreignCell, foreignFence, binding.NoContributionSide(), foreignSide)
	if !ok {
		t.Fatal("foreign transition")
	}
	if _, _, _, accepted := ApplyTransitions(directory, state, []invocation.ContributionTransition{foreignTransition}); accepted {
		t.Fatal("foreign transition accepted")
	}
}

func TestApplyTransitionsRejectsContradictoryOrderedChain(t *testing.T) {
	fixture := contributionFixture(t)
	first := rowFor(t, fixture, 0, fixture.destination[0], 84)
	second := rowFor(t, fixture, 0, fixture.destination[0], 85)
	wrongBefore := rowFor(t, fixture, 0, fixture.destination[0], 86)
	address := addressForHandle(t, fixture, fixture.handles[0])
	firstTransition := transitionFor(t, fixture, address, first.Cell(), binding.NoContributionSide(), sideForRow(t, first))
	contradictory := transitionFor(t, fixture, address, second.Cell(), sideForRow(t, wrongBefore), sideForRow(t, second))
	if _, _, _, accepted := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{firstTransition, contradictory}); accepted {
		t.Fatal("contradictory ordered chain accepted")
	}
}

func TestApplyTransitionsAcceptsExactNoOpWithoutAuthorityChurn(t *testing.T) {
	fixture := contributionFixture(t)
	row := rowFor(t, fixture, 0, fixture.destination[0], 87)
	seed := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), row.Cell(), binding.NoContributionSide(), sideForRow(t, row))
	directory, state, _, ok := ApplyTransitions(fixture.directory, fixture.state, []invocation.ContributionTransition{seed})
	if !ok {
		t.Fatal("seed no-op test")
	}
	noop := transitionFor(t, fixture, addressForHandle(t, fixture, fixture.handles[0]), row.Cell(), binding.NoContributionSide(), sideForRow(t, row))
	nextDirectory, nextState, delta, ok := ApplyTransitions(directory, state, []invocation.ContributionTransition{noop})
	if !ok || !nextDirectory.Same(directory) || !nextState.Same(state) || delta.Changed() || len(delta.AffectedTargets()) != 0 {
		t.Fatal("exact no-op churned or refused")
	}
}
