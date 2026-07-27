package transformer

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLoopEquationLoweringsCompileGenericForAndLoopControl(t *testing.T) {
	compiler, err := InstallLoopEquationLowerings(equation.Skeleton())
	if err != nil {
		t.Fatal(err)
	}
	// The representative bodies retain their structural nonreturning terminal.
	// Bind it only so the walk can reach this family's occurrences.
	compiler, err = compiler.With(string(OperatorNonreturning), equation.BindExistingKernel("transformer/formal-nonreturning/v1"))
	if err != nil {
		t.Fatal(err)
	}

	generic := loopEquationGenericForProgram(t)
	artifact, err := generic.CompileEquationIR(compiler, loopEquationTestBinder)
	if err != nil {
		t.Fatalf("compile GenericFor equation: %v", err)
	}
	if len(artifact.Equations) != 2 {
		t.Fatalf("GenericFor artifact = %#v", artifact)
	}
	if !loopEquationArtifactHasKernel(artifact, formalGenericForEquationKernel) {
		t.Fatalf("GenericFor kernel missing from %#v", artifact)
	}

	loops, _, _, _, _ := formalNestedLoopControlProgram(t)
	artifact, err = loops.CompileEquationIR(compiler, loopEquationTestBinder)
	if err != nil {
		t.Fatalf("compile loop-control equation: %v", err)
	}
	if len(artifact.Equations) != 5 {
		t.Fatalf("loop-control equations = %d, want 5 including nonreturning", len(artifact.Equations))
	}
	for _, lowered := range artifact.Equations {
		if lowered.Occurrence.Kind == string(OperatorLoopControl) && lowered.KernelID != formalLoopControlEquationKernel {
			t.Fatalf("loop-control kernel = %q", lowered.KernelID)
		}
	}
}

func loopEquationTestBinder(occurrence RelationEquationOccurrence) (equation.Draft, error) {
	if occurrence.Kind == OperatorGenericFor || occurrence.Kind == OperatorLoopControl {
		return BindLoopEquationOccurrence(occurrence)
	}
	contract, err := NewOperatorContract(occurrence.Kind, formal.NewOccurrenceID(occurrence.Body, 1))
	if err != nil {
		return equation.Draft{}, err
	}
	body := equation.BodyID(occurrence.Body)
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	operands := make([]equation.Operand, 0, len(contract.Operands))
	for _, role := range contract.Operands {
		operands = append(operands, equation.Operand{Role: string(role), Term: equation.ClosedTerm([]byte(role))})
	}
	return equation.Draft{Target: equation.Coordinate{Body: body, Name: "test-aux-" + string(occurrence.Kind)}, Entry: entry, Occurrence: equation.Occurrence{Kind: string(contract.Kind), ContractID: equation.ContentID(contract.ContentID())}, Operands: operands}, nil
}

func loopEquationArtifactHasKernel(artifact equation.Artifact, kernel string) bool {
	for _, lowered := range artifact.Equations {
		if lowered.KernelID == kernel {
			return true
		}
	}
	return false
}

func TestLoopEquationSkeletonFailsClosedBeforeFamilyInstallation(t *testing.T) {
	body := equation.BodyID(lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("loop-equation-skeleton"))))
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	draft := equation.Draft{
		Target: equation.Coordinate{Body: body, Name: "generic-for/root-1/step-1"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: string(OperatorGenericFor), ContractID: equation.ContentID{1}},
		Operands:   []equation.Operand{{Role: string(AccessFlow), Term: equation.ClosedTerm([]byte("flow"))}, {Role: string(AccessNodeEntry), Term: equation.ClosedTerm([]byte("entry"))}, {Role: string(AccessPublished), Term: equation.ClosedTerm([]byte("published"))}, {Role: string(AccessState), Term: equation.ClosedTerm([]byte("values"))}},
	}
	_, err := equation.Skeleton().Compile(equation.Source{Drafts: []equation.Draft{draft}})
	if !errors.Is(err, equation.ErrUnimplementedLowering) {
		t.Fatalf("skeleton error = %v, want unimplemented lowering", err)
	}
}

func TestLoopEquationArtifactIdentityTracksClosedOperandsAndContracts(t *testing.T) {
	body := equation.BodyID(lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("loop-equation-identity"))))
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	makeDraft := func(name string, contractID byte, state, guard string) equation.Draft {
		return equation.Draft{
			Target:     equation.Coordinate{Body: body, Name: name},
			Entry:      entry,
			Guards:     []equation.Guard{{Body: body, Encoding: []byte(guard)}},
			Occurrence: equation.Occurrence{Kind: string(OperatorGenericFor), ContractID: equation.ContentID{contractID}},
			Operands: []equation.Operand{
				{Role: string(AccessFlow), Term: equation.ClosedTerm([]byte("flow"))},
				{Role: string(AccessNodeEntry), Term: equation.ClosedTerm([]byte("node-entry"))},
				{Role: string(AccessPublished), Term: equation.ClosedTerm([]byte("published"))},
				{Role: string(AccessState), Term: equation.ClosedTerm([]byte(state))},
			},
		}
	}
	compiler, err := InstallLoopEquationLowerings(equation.Skeleton())
	if err != nil {
		t.Fatal(err)
	}
	left, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft("generic-for/b", 2, "values", "execute"), makeDraft("generic-for/a", 1, "values", "execute")}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft("generic-for/a", 1, "values", "execute"), makeDraft("generic-for/b", 2, "values", "execute")}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("loop equation artifact retained declaration order")
	}
	changedOperand, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft("generic-for/a", 1, "other-values", "execute"), makeDraft("generic-for/b", 2, "values", "execute")}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == changedOperand.ContentID() {
		t.Fatal("loop equation artifact omitted a closed semantic operand")
	}
	changedGuard, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft("generic-for/a", 1, "values", "other-execute"), makeDraft("generic-for/b", 2, "values", "execute")}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == changedGuard.ContentID() {
		t.Fatal("loop equation artifact omitted a semantic guard")
	}
	changedContract, err := compiler.Compile(equation.Source{Drafts: []equation.Draft{makeDraft("generic-for/a", 3, "values", "execute"), makeDraft("generic-for/b", 2, "values", "execute")}})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() == changedContract.ContentID() {
		t.Fatal("loop equation artifact omitted its operator contract")
	}
}

func TestLoopEquationLoweringAuditFailsClosed(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("loop-equation-audit")))
	for _, test := range []struct {
		name   string
		kind   OperatorKind
		reads  []ContractSelector
		writes []ContractSelector
		guard  []string
		dep    string
	}{
		{"generic-for", OperatorGenericFor,
			[]ContractSelector{{Role: AccessFlow, Name: "predecessor"}, {Role: AccessNodeEntry, Name: "node-entry"}, {Role: AccessPublished, Name: "published-inputs"}, {Role: AccessState, Name: "values-and-factors"}, {Role: AccessGuard, Name: "execute"}, {Role: AccessAllocation, Name: "projection"}},
			[]ContractSelector{{Role: AccessState, Name: "target-values"}, {Role: AccessState, Name: "write-factors"}}, []string{"generic-for/execute"}, formalGenericForEquationKernel},
		{"loop-control", OperatorLoopControl,
			[]ContractSelector{{Role: AccessFlow, Name: "predecessor"}, {Role: AccessGuard, Name: "loop-lifetime"}},
			[]ContractSelector{{Role: AccessGuard, Name: "loop-lifetime"}}, []string{"loop-control/lifetime"}, formalLoopControlEquationKernel},
	} {
		t.Run(test.name, func(t *testing.T) {
			contract, err := NewOperatorContract(test.kind, formal.NewOccurrenceID(owner, 1))
			if err != nil {
				t.Fatal(err)
			}
			contract.Reads, contract.Writes, contract.GuardAtoms = test.reads, test.writes, test.guard
			contract.Dependencies = []ContractDependency{{Kind: test.dep, ID: contentID([]byte(test.dep))}}
			access := OperatorAccess{Kind: test.kind, Occurrence: contract.Occurrence, Reads: append([]ContractSelector(nil), test.reads...), Writes: append([]ContractSelector(nil), test.writes...), Dependencies: append([]ContractDependency(nil), contract.Dependencies...)}
			execution := Execution{Complete: true, Published: true, Access: equation.AccessRecord{Payload: access}}
			if err := VerifyLoweredOperatorAccess(contract, execution); err != nil {
				t.Fatalf("audit: %v", err)
			}
			access.Reads = append(access.Reads, ContractSelector{Role: AccessBoundary, Name: "undeclared"})
			execution.Access.Payload = access
			if err := VerifyLoweredOperatorAccess(contract, execution); err == nil {
				t.Fatal("audit accepted undeclared read")
			}
		})
	}
}

func loopEquationGenericForProgram(t *testing.T) *RelationProgram {
	t.Helper()
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Exit()
	const target = symbol.ID(101)
	op, ok := operationplan.NewGenericForOperation(0, target, target, nil, nil)
	if !ok {
		t.Fatal("generic-for operation")
	}
	body.plan = body.plan.WithExtensions([]operationplan.ExtensionInput{{Point: point, Kind: operationplan.BodyGenericFor, GenericFor: op}})
	body.nodeReads = make([][]cfg.Point, body.graph.Size())
	body.genericForMembership = formalGenericForTestAuthority{config: state.GenericForFactorConfig{Keys: body.keys, VariableIndex: 0, Target: body.keys.FromPath(pathdom.Path{Symbol: target})}}
	old := typevalue.LiteralString(reg, "old")
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{{Slot: statekey.SymbolValue(101), Value: old}})
	seed := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(101), old)
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph, state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), seed))
	arena := body.relation.code.terms
	projection, present := arena.middleValue(statekey.SymbolValue(target))
	if !present || projection == 0 {
		t.Fatal("capture GenericFor projection")
	}
	identity := frozenGenericForIdentityPublication{target: statekey.SymbolValue(target), projection: projection, finiteSources: []ValueTerm{projection}, projectionIdentity: genericForProjectionIdentityNoFinite, sealed: true}
	return formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepGenericFor, point: point, access: []valueAccessTerm{{term: projection}}, genericIdentity: identity}}, next: 2},
		{kind: relationNodeBottom},
	})
}
