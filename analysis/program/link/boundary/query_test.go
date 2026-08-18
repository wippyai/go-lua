package boundary

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
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
	if _, ok := component.InitialOperation(project, contract, vocabulary.InitialRoot(0), projectKey); ok {
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
	emit := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"emit"}}
	contract, err := target.Seal(&target.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{emit},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		InitialRoots: []vocabulary.InitialRootSpec{{
			Identity: root,
			Shape:    vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: root}},
		}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: root, Key: boundaryInitialOperationLiteral("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: root}, Mutability: vocabulary.InitialMutable},
			{Root: root, Key: boundaryInitialOperationLiteral("emit"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: emit}, Mutability: vocabulary.InitialMutable},
			{Root: root, Key: boundaryInitialOperationLiteral("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{{Name: "_G", Root: root, Key: boundaryInitialOperationLiteral("_G")}},
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

func boundaryInitialOperationTargetKey(t testing.TB, contract *target.Contract, want string) vocabulary.ExactKey {
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
