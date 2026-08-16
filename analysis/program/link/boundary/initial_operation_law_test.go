package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestInitialOperationIsBoundaryOwnedLocalAndAllocationFree(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "boundary-initial-operation.lua", Text: []byte(`local table = {}; return table.emit`)})
	if err != nil {
		t.Fatal(err)
	}
	contract := boundaryInitialOperationTarget(t, "GlobalEnvRoot")
	project := boundaryInitialOperationProject(t, p, contract)
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := project.Mounts().At(0)
	if !ok {
		t.Fatal("missing Project mount")
	}
	programKey := boundaryInitialOperationProgramKey(t, p, "emit")
	projectKey, ok := project.Keys().ForProgram(shard, p, programKey)
	if !ok {
		t.Fatal("missing Project emit key")
	}
	root, ok := contract.GlobalEnvRoot()
	if !ok {
		t.Fatal("missing Target global root")
	}
	if op, ok := component.InitialOperation(project, contract, root, projectKey); !ok || op == 0 {
		t.Fatalf("initial operation = %d/%v", op, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := component.InitialOperation(project, contract, root, projectKey); !ok {
			t.Fatal("allocation probe lost initial operation")
		}
	}); allocations != 0 {
		t.Fatalf("InitialOperation allocated %v times per call", allocations)
	}

	foreignProject := boundaryInitialOperationProject(t, p, contract)
	if _, ok := component.InitialOperation(foreignProject, contract, root, projectKey); ok {
		t.Fatal("foreign equivalent Project crossed Boundary owner fence")
	}
	foreignTarget := boundaryInitialOperationTarget(t, "GlobalEnvRoot")
	if _, ok := component.InitialOperation(project, foreignTarget, root, projectKey); ok {
		t.Fatal("foreign equivalent Target crossed Boundary owner fence")
	}
	nonOperationKey := boundaryInitialOperationTargetKey(t, contract, "_G")
	nonOperationProjectKey, ok := project.Keys().ForTarget(contract, nonOperationKey)
	if !ok {
		t.Fatal("missing Project root key")
	}
	if _, ok := component.InitialOperation(project, contract, root, nonOperationProjectKey); ok {
		t.Fatal("non-operation initial value was admitted")
	}
	if _, ok := component.InitialOperation(project, contract, root, linkproject.Key{}); ok {
		t.Fatal("zero Project key was admitted")
	}
	if _, ok := component.InitialOperation(project, contract, target.InitialRoot(0), projectKey); ok {
		t.Fatal("zero initial root was admitted")
	}
}

func boundaryInitialOperationProject(t testing.TB, p *program.Program, contract *target.Contract) *linkproject.Component {
	t.Helper()
	draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func boundaryInitialOperationTarget(t testing.TB, root string) *target.Contract {
	t.Helper()
	emit := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"emit"}}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{emit},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: root,
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: root}},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: root, Key: boundaryInitialOperationLiteral("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: root}, Mutability: target.InitialMutable},
			{Root: root, Key: boundaryInitialOperationLiteral("emit"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: emit}, Mutability: target.InitialMutable},
			{Root: root, Key: boundaryInitialOperationLiteral("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: root, Key: boundaryInitialOperationLiteral("_G")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func boundaryInitialOperationLiteral(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}

func boundaryInitialOperationProgramKey(t testing.TB, p *program.Program, want string) keyspace.Key {
	t.Helper()
	keys := p.Source().Keys()
	for index := 0; index < keys.ExactCount(); index++ {
		key, value, ok := keys.ExactAt(index)
		if ok && value.Kind == keyspace.LiteralString && value.String == want {
			return key
		}
	}
	t.Fatalf("missing Program key %q", want)
	return 0
}

func boundaryInitialOperationTargetKey(t testing.TB, contract *target.Contract, want string) target.ExactKey {
	t.Helper()
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, keyOK := contract.ExactKeyAt(index)
		value, valueOK := contract.ExactKeyValue(key)
		if keyOK && valueOK && value.Kind == keyspace.LiteralString && value.String == want {
			return key
		}
	}
	t.Fatalf("missing Target key %q", want)
	return 0
}
