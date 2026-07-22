package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func branchRelationsEquationBinding(owner lexicalidentity.StableLexicalBodyID) BranchRelationsEquationBinding {
	entry := equation.EntryParameter{Body: equation.BodyID(owner), Name: "entry"}
	return BranchRelationsEquationBinding{
		Occurrence: formal.NewOccurrenceID(owner, 701),
		Target:     "branch-relations-result",
		Entry:      entry.Name,
		Flow:       equation.EntryTerm(entry),
		State:      equation.ClosedTerm([]byte("sealed-branch-state")),
		Guard:      equation.ClosedTerm([]byte("selected-branch-edge")),
		// The formal adapter reads predecessor/current/original factor frames,
		// the selected branch guard, and its sealed boundary transaction.  It
		// publishes only the current state image; this remains true for a
		// no-op execution, whose attempted semantic write is still audited.
		Reads: []ContractSelector{
			{Role: AccessState, Name: "branch-care-reduction"},
			{Role: AccessState, Name: "branch-current-frame"},
			{Role: AccessBoundary, Name: "branch-relation-transaction"},
			{Role: AccessFlow, Name: "predecessor-flow"},
			{Role: AccessGuard, Name: "selected-branch-edge"},
			{Role: AccessState, Name: "branch-original-frame"},
		},
		Writes:     []ContractSelector{{Role: AccessState, Name: "branch-current-frame"}},
		GuardAtoms: []string{"selected-branch-edge"},
	}
}

func genericEquationDraftForTest(occurrence RelationEquationOccurrence, ordinal uint64) (equation.Draft, error) {
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, ordinal))
	if err != nil {
		return equation.Draft{}, err
	}
	body := equation.BodyID(occurrence.Body)
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte(role))})
	}
	return equation.Draft{
		Target:     equation.Coordinate{Body: body, Name: string(occurrence.Kind) + "-result"},
		Entry:      equation.EntryParameter{Body: body, Name: "entry"},
		Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())},
		Operands:   operands,
	}, nil
}

func TestBranchRelationsEquationBindingCanonicalIdentity(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("branch-relations-equation")))
	occurrence := RelationEquationOccurrence{Body: owner, Kind: OperatorBranchRelations}
	leftBinding := branchRelationsEquationBinding(owner)
	rightBinding := leftBinding
	rightBinding.Reads = append([]ContractSelector(nil), leftBinding.Reads...)
	for left, right := 0, len(rightBinding.Reads)-1; left < right; left, right = left+1, right-1 {
		rightBinding.Reads[left], rightBinding.Reads[right] = rightBinding.Reads[right], rightBinding.Reads[left]
	}
	left, _, err := BindBranchRelationsEquationOccurrence(occurrence, leftBinding)
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := BindBranchRelationsEquationOccurrence(occurrence, rightBinding)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := equation.Skeleton().With(string(OperatorBranchRelations), BranchRelationsEquationLowerer())
	if err != nil {
		t.Fatal(err)
	}
	leftArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{left}})
	if err != nil {
		t.Fatal(err)
	}
	rightArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{right}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() != rightArtifact.ContentID() {
		t.Fatal("branch-relations artifact retained declaration order")
	}
	changedBinding := leftBinding
	changedBinding.GuardAtoms = []string{"other-selected-branch-edge"}
	changed, _, err := BindBranchRelationsEquationOccurrence(occurrence, changedBinding)
	if err != nil {
		t.Fatal(err)
	}
	changedArtifact, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{changed}})
	if err != nil {
		t.Fatal(err)
	}
	if leftArtifact.ContentID() == changedArtifact.ContentID() {
		t.Fatal("branch-relations artifact ignored semantic guard change")
	}
}

func TestBranchRelationsEquationLoweringWalksRepresentativeBody(t *testing.T) {
	program := branchRelationsEquationTestProgram(t)
	compiler, err := equation.Skeleton().With(string(OperatorOutcome), equation.BindExistingKernel("transformer/formal-outcome/v1"))
	if err != nil {
		t.Fatal(err)
	}
	var binds uint64
	binder := func(occurrence RelationEquationOccurrence) (equation.Draft, error) {
		binds++
		if occurrence.Kind == OperatorBranchRelations {
			draft, _, bindErr := BindBranchRelationsEquationOccurrence(occurrence, branchRelationsEquationBinding(occurrence.Body))
			return draft, bindErr
		}
		return genericEquationDraftForTest(occurrence, 800+binds)
	}
	if _, err := program.CompileEquationIR(compiler, binder); !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("absent branch-relations hook error = %v, want unimplemented lowering", err)
	}
	compiler, err = compiler.With(string(OperatorBranchRelations), BranchRelationsEquationLowerer())
	if err != nil {
		t.Fatal(err)
	}
	compiler, err = compiler.With(string(OperatorNonreturning), equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := program.CompileEquationIR(compiler, binder)
	if err != nil {
		t.Fatalf("CompileEquationIR: %v", err)
	}
	var branch equation.Equation
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorBranchRelations) {
			branch = lowered
		}
	}
	if branch.KernelID != FormalBranchRelationsKernelID {
		t.Fatalf("branch-relations kernel = %q", branch.KernelID)
	}
}

func TestBranchRelationsLoweringAuditsExistingKernelAccess(t *testing.T) {
	program := branchRelationsEquationTestProgram(t)
	// This executes formalTupleAlgebra.applyFormalBranchRelations, whose factor
	// application delegates to factapply.BranchRelationFactors.  The audit is
	// deliberately post-execution and cannot affect that result.
	if _, err := executeFormalRelation(context.Background(), program); err != nil {
		t.Fatalf("existing branch-relations kernel: %v", err)
	}
	owner := program.bodies[0].body
	binding := branchRelationsEquationBinding(owner)
	_, contract, err := BindBranchRelationsEquationOccurrence(
		RelationEquationOccurrence{Body: owner, Kind: OperatorBranchRelations}, binding,
	)
	if err != nil {
		t.Fatal(err)
	}
	access := OperatorAccess{
		Kind:       OperatorBranchRelations,
		Occurrence: binding.Occurrence,
		Reads:      append([]ContractSelector(nil), contract.Reads...),
		Writes:     append([]ContractSelector(nil), contract.Writes...),
	}
	execution := equation.Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
		t.Fatalf("branch-relations audit: %v", err)
	}
	access.Reads = append(access.Reads, ContractSelector{Role: AccessState, Name: "undeclared-reduction"})
	execution.Access.Payload = access
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("branch-relations audit accepted undeclared read")
	}
	execution = equation.Execution{Published: true, Access: equation.AccessRecord{Payload: OperatorAccess{Kind: OperatorBranchRelations, Occurrence: binding.Occurrence}}}
	if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
		t.Fatal("branch-relations lowering published a partial transaction")
	}
}

func branchRelationsEquationTestProgram(t *testing.T) *RelationProgram {
	t.Helper()
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	x := symbol.ID(101)
	path := pathdom.NewPath(x, "param")
	resolver := visibility.NewResolver(nil)
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	falseValue := typevalue.LiteralBool(reg, false)
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(101), Value: falseValue},
		{Slot: statekey.SymbolValue(102), Value: product.Top()},
		{Slot: statekey.SymbolValue(103), Value: product.Top()},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(point), state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(x), falseValue)))
	rows := factflow.NewBranchRefinementSet().WithNumFloorRefinements(
		factflow.NewBranchNumFloorRefinementOnEdge(path, 4, true),
	)
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows}})
	transaction := factapply.PlanBranchRelationTransaction(facts, point, true)
	return formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepBranchRelations, branch: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
}
