package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalContributionNonrecursiveIsExactSparseIdentity(t *testing.T) {
	program := formalContributionTestProgram(t, semanticContribution{}, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution, contribution: 1}}, next: 2},
		{kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, _ := program.formalTemplate.equation(program.formalRegion.roots[0])
	predecessor, err := algebra.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	stepCell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	stepEquation, ok := program.formalTemplate.equation(stepCell)
	if !ok || stepEquation.Operator.stepCapability != formalRelationStepCapabilityContribution ||
		stepEquation.Operator.contribution == nil || stepEquation.Operator.contribution.active {
		t.Fatalf("nonrecursive Contribution capability = %#v/%t", stepEquation.Operator.contribution, ok)
	}
	result, err := algebra.applyFormalContribution(stepEquation.Operator, predecessor)
	if err != nil || !algebra.same(predecessor, result) {
		t.Fatalf("nonrecursive Contribution = %#v, %v", result, err)
	}
	diagnostics, reachable, err := algebra.formalDiagnosticOutput(context.Background(), result)
	want := callpayload.DiagnosticOutputLattice(program.registry).Bottom()
	if err != nil || !reachable || !diagnostics.Equal(program.registry, want) {
		t.Fatalf("nonrecursive diagnostics = %#v/%t, %v; want %#v", diagnostics, reachable, err, want)
	}
}

func TestFormalContributionRecursiveWTOJoinsCanonicalDiagnosticsDeterministically(t *testing.T) {
	contribution := semanticContribution{suspensionKnown: true, maySuspend: true}
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	const binder loopMuTerm = 1
	program := formalContributionTestProgramFromBase(t, base, contribution, []relationNode{
		{},
		{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeChoice, guard: guard, whenTrue: 4, whenFalse: 5},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeSequence, steps: []boundaryStep{
			{kind: boundaryStepContribution, contribution: 1},
			{kind: boundaryStepLoopFeedback, binder: binder},
		}},
		{kind: relationNodeSequence, steps: []boundaryStep{
			{kind: boundaryStepContribution, contribution: 1},
			{kind: boundaryStepLoopExit, binder: binder, route: 0},
		}},
	})
	// Observe the first semantic application before the recursive equation has
	// converged to its idempotent value. The final feedback cell may lawfully be
	// physical identity because publishing the same event twice changes nothing.
	probe, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, _ := program.formalTemplate.equation(program.formalRegion.roots[0])
	rootTuple, err := probe.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	contributionEquation, _ := program.formalTemplate.equation(formalRelationCell{Variable: 1, Root: 4, Step: 1, Kind: formalRelationCellStep})
	firstContribution, err := probe.applyFormalContribution(contributionEquation.Operator, rootTuple)
	if err != nil {
		t.Fatal(err)
	}
	formalContributionAssertOnlyDiagnosticsChanged(t, probe, rootTuple, firstContribution)

	first, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	want, err := materializeBoundaryPrefixDiagnostics(&program.bodies[0], contribution)
	if err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]*formalRelationExecution{"first": first, "second": second} {
		exit := run.values[formalRelationCell{Variable: 1, Root: 3, Kind: formalRelationCellNode}]
		got, reachable, projectionErr := run.algebra.formalDiagnosticOutput(context.Background(), exit)
		if projectionErr != nil || !reachable || !got.Equal(program.registry, want) {
			t.Fatalf("%s recursive diagnostics = %#v/%t, %v; want %#v", name, got, reachable, projectionErr, want)
		}
	}
	firstExit, _, _ := first.algebra.formalDiagnosticOutput(context.Background(), first.values[formalRelationCell{Variable: 1, Root: 3, Kind: formalRelationCellNode}])
	secondExit, _, _ := second.algebra.formalDiagnosticOutput(context.Background(), second.values[formalRelationCell{Variable: 1, Root: 3, Kind: formalRelationCellNode}])
	if !firstExit.RepresentationEqual(program.registry, secondExit) || len(first.algebra.components.terminals) != len(second.algebra.components.terminals) {
		t.Fatalf("recursive Contribution solve drifted: diagnostics=%#v/%#v terminals=%d/%d", firstExit, secondExit, len(first.algebra.components.terminals), len(second.algebra.components.terminals))
	}
}

func TestFormalContributionConditionalPhysicalZeroUsesDiagnosticDefault(t *testing.T) {
	contribution := semanticContribution{suspensionKnown: true, maySuspend: true}
	program := formalContributionTestProgram(t, contribution, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution, contribution: 1}}, next: 2},
		{kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, _ := program.formalTemplate.equation(program.formalRegion.roots[0])
	predecessor, err := algebra.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	span, _, authority, ok := algebra.span(predecessor.variable)
	if !ok {
		t.Fatal("formal Contribution span is missing")
	}
	var diagnostics formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role == formalFiberDiagnostics {
			diagnostics = descriptor
			break
		}
	}
	if diagnostics.role == formalFiberInvalid {
		t.Fatal("formal Contribution diagnostics descriptor is missing")
	}
	defaultValue, err := authority.defaultFor(context.Background(), diagnostics)
	if err != nil || defaultValue.kind != formalComponentDefaultTerminal {
		t.Fatalf("diagnostic default = %#v, %v", defaultValue, err)
	}
	// Physical zero is the descriptor's implicit default even when it occurs
	// beneath a decision node. It is not a globally owned terminal ID.
	conditional := algebra.decisions.branch(17, decisionFalse, algebra.decisions.terminal(defaultValue.leaf))
	predecessor, err = algebra.writeScalar(predecessor, diagnostics, conditional)
	if err != nil {
		t.Fatal(err)
	}
	stepEquation, _ := program.formalTemplate.equation(formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep})
	result, err := algebra.applyFormalContribution(stepEquation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	got, reachable, err := algebra.formalDiagnosticOutput(context.Background(), result)
	want, wantErr := materializeBoundaryPrefixDiagnostics(&program.bodies[0], contribution)
	if wantErr != nil {
		t.Fatal(wantErr)
	}
	if err != nil || !reachable || !got.Equal(program.registry, want) {
		t.Fatalf("conditional-default diagnostics = %#v/%t, %v; want %#v", got, reachable, err, want)
	}
}

func formalContributionTestProgram(t *testing.T, contribution semanticContribution, nodes []relationNode) *RelationProgram {
	t.Helper()
	return formalContributionTestProgramFromBase(t, formalRootInputTestProgram(t, standard.Registry()), contribution, nodes)
}

func formalContributionTestProgramFromBase(t *testing.T, base *RelationProgram, contribution semanticContribution, nodes []relationNode) *RelationProgram {
	t.Helper()
	base.bodies[0].relation.code.contributions = []semanticContribution{{}, contribution.clone()}
	return formalRelationExecutorTestProgramFromBase(t, base, nodes)
}

func formalContributionAssertOnlyDiagnosticsChanged(t *testing.T, algebra *formalTupleAlgebra, before, after formalRelationTuple) {
	t.Helper()
	span, directory, _, ok := algebra.span(before.variable)
	if !ok || before.root.owner != directory || after.root.owner != directory {
		t.Fatal("Contribution sparse comparison has foreign tuple")
	}
	changed := 0
	for ordinal, descriptor := range span.descriptors() {
		left, leftErr := directory.valueAt(before.root, formalFiberOrdinal(ordinal))
		right, rightErr := directory.valueAt(after.root, formalFiberOrdinal(ordinal))
		if leftErr != nil || rightErr != nil {
			t.Fatalf("Contribution descriptor %d read = %v/%v", ordinal, leftErr, rightErr)
		}
		if left == right {
			continue
		}
		changed++
		if descriptor.role != formalFiberDiagnostics {
			t.Fatalf("Contribution changed non-diagnostic descriptor %d role %d", ordinal, descriptor.role)
		}
	}
	if changed != 1 {
		t.Fatalf("Contribution changed %d physical descriptors, want exactly Diagnostics", changed)
	}
}
