package project

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestTargetKeyInverseIsLocalExactAndAllocationFree(t *testing.T) {
	p := projectProgram(t, `local table = {}; return table.emit, table.source_only`)
	contract := projectOperationTarget(t, "GlobalEnvRoot")
	foreignTarget := projectOperationTarget(t, "GlobalEnvRoot")
	draft, err := Build(Input{Modules: []Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	keys := project.Keys()

	emitProgramKey := projectExactStringKey(t, p, "emit")
	emitProjectKey, ok := keys.ForProgram(project.MountsMustAt(0), p, emitProgramKey)
	if !ok {
		t.Fatal("Project did not issue the exact emit key")
	}
	emitTargetKey := targetExactStringKey(t, contract, "emit")
	if got, ok := keys.TargetFor(contract, emitProjectKey); !ok || got != emitTargetKey {
		t.Fatalf("Project->Target key = %d/%v, want %d", got, ok, emitTargetKey)
	}
	if got, ok := keys.ForTarget(contract, emitTargetKey); !ok || got != emitProjectKey {
		t.Fatalf("Target->Project key = %v/%v, want %v", got, ok, emitProjectKey)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := keys.TargetFor(contract, emitProjectKey); !ok {
			t.Fatal("allocation probe lost exact Target key")
		}
	}); allocations != 0 {
		t.Fatalf("TargetFor allocated %v times per call", allocations)
	}

	sourceOnlyProgramKey := projectExactStringKey(t, p, "source_only")
	sourceOnlyProjectKey, ok := keys.ForProgram(project.MountsMustAt(0), p, sourceOnlyProgramKey)
	if !ok {
		t.Fatal("Project did not issue the source-only key")
	}
	if got, ok := keys.TargetFor(contract, sourceOnlyProjectKey); ok || got != 0 {
		t.Fatalf("source-only Project key acquired Target key %d/%v", got, ok)
	}
	if _, ok := keys.TargetFor(foreignTarget, emitProjectKey); ok {
		t.Fatal("equivalent foreign Target crossed the Project owner fence")
	}
	foreignDraft, err := Build(Input{Modules: []Module{{Name: "main", Program: p}}, Target: foreignTarget})
	if err != nil {
		t.Fatal(err)
	}
	foreignProject, err := foreignDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, ok := foreignProject.Keys().ForProgram(foreignProject.MountsMustAt(0), p, emitProgramKey)
	if !ok {
		t.Fatal("foreign Project did not issue its same-ordinal key")
	}
	if _, ok := keys.TargetFor(contract, foreignKey); ok {
		t.Fatal("equivalent foreign Project key crossed the owner fence")
	}
	if _, ok := keys.TargetFor(contract, Key{authority: project.authority, ordinal: uint32(keys.Count() + 1)}); ok {
		t.Fatal("out-of-range same-owner Project key was admitted")
	}

	root, ok := contract.GlobalEnvRoot()
	if !ok {
		t.Fatal("missing global boot root")
	}
	if op, ok := contract.InitialOperation(root, emitTargetKey); !ok || op == 0 {
		t.Fatalf("operation initial did not reduce: %d/%v", op, ok)
	}
	nonOperationKey := targetExactStringKey(t, contract, "_G")
	if op, ok := contract.InitialOperation(root, nonOperationKey); ok || op != 0 {
		t.Fatalf("root initial was admitted as operation: %d/%v", op, ok)
	}
	if op, ok := contract.InitialOperation(root, vocabulary.ExactKey(0)); ok || op != 0 {
		t.Fatalf("zero Target key was admitted: %d/%v", op, ok)
	}
	if op, ok := contract.InitialOperation(vocabulary.InitialRoot(0), emitTargetKey); ok || op != 0 {
		t.Fatalf("zero Target root was admitted: %d/%v", op, ok)
	}
}

// MountsMustAt keeps this law focused on the inverse API while preserving the
// ordinary owner-fenced Mounts.At/Index surface used by production callers.
func (c *Component) MountsMustAt(index int) Shard {
	shard, ok := c.Mounts().At(index)
	if !ok {
		panic("missing test Project mount")
	}
	return shard
}

func projectOperationTarget(t testing.TB, root string) *target.Contract {
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
			{Root: root, Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: root}, Mutability: vocabulary.InitialMutable},
			{Root: root, Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "emit"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueOperation, Operation: emit}, Mutability: vocabulary.InitialMutable},
			{Root: root, Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "__link_absent"}, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{{Name: "_G", Root: root, Key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "_G"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func targetExactStringKey(t testing.TB, contract *target.Contract, want string) vocabulary.ExactKey {
	t.Helper()
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, keyOK := contract.ExactKeyAt(index)
		value, valueOK := contract.ExactKeyValue(key)
		if keyOK && valueOK && value.Kind == keyspace.LiteralString && value.String == want {
			return key
		}
	}
	t.Fatalf("missing Target exact key %q", want)
	return 0
}
