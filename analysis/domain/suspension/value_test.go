package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestModuleInitGenerationIndexIsExactAndAllocationFree(t *testing.T) {
	source := suspensionGenerationFixture(t)
	schema, ok := NewSchema(source)
	if !ok || source.Module().Generations().Count() < 2 {
		t.Fatal("multiple ModuleInit generations")
	}
	generations := source.Module().Generations()
	first, firstOK := generations.At(0)
	last, lastOK := generations.At(generations.Count() - 1)
	if !firstOK || !lastOK {
		t.Fatal("first and last ModuleInit generations")
	}
	for name, generation := range map[string]linkmodule.ModuleInitGeneration{"first": first, "last": last} {
		key, keyOK := schema.KeyForModuleInitGeneration(generation)
		got, gotOK := key.ModuleInitGeneration()
		if !keyOK || !gotOK || got != generation {
			t.Fatalf("%s generation did not restore its exact Suspension key", name)
		}
		wantID, wantIDOK := generations.ID(generation)
		gotID, gotIDOK := key.OccurrenceID()
		if !wantIDOK || !gotIDOK || gotID != wantID {
			t.Fatalf("%s generation restored a different occurrence", name)
		}
	}

	foreign := suspensionGenerationFixture(t)
	foreignGeneration, foreignOK := foreign.Module().Generations().At(0)
	if !foreignOK || foreign.ContentID() != source.ContentID() {
		t.Fatal("same-content foreign generation fixture")
	}
	if key, accepted := schema.KeyForModuleInitGeneration(foreignGeneration); accepted || key.Valid() {
		t.Fatal("generation index accepted a same-content foreign Module handle")
	}

	if allocations := testing.AllocsPerRun(1_000, func() {
		key, found := schema.KeyForModuleInitGeneration(last)
		if !found || !key.Valid() {
			panic("indexed ModuleInit generation lookup")
		}
	}); allocations != 0 {
		t.Fatalf("indexed ModuleInit generation lookup allocated %g times", allocations)
	}
}

func TestModuleInitGenerationIndexRejectsContentIDCollision(t *testing.T) {
	var id keyspace.ContentID
	id[0] = 1
	owner := &schema{keys: []keySupport{{id: id}, {id: id}}}
	if owner.indexKeys() || owner.keyByID != nil {
		t.Fatal("duplicate lifecycle occurrence identity was indexed")
	}
}

func suspensionGenerationFixture(t testing.TB) *link.Link {
	t.Helper()
	main := suspensionProgram(t, `require("dependency-a")
require("dependency-b")`)
	dependencyA := suspensionProgram(t, `return 1`)
	dependencyB := suspensionProgram(t, `return 2`)
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	literal := func(text string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
	}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{require}, Input: target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}}, Effects: target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: literal("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("__module_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: literal("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: literal("_G")}, {Name: "__module_absent", Root: "GlobalEnvRoot", Key: literal("__module_absent")}, {Name: "require", Root: "GlobalEnvRoot", Key: literal("require")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, firstOK := main.Module().ImportAt(0)
	second, secondOK := main.Module().ImportAt(1)
	if !firstOK || !secondOK {
		t.Fatal("two static require imports")
	}
	source, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency-a", Program: dependencyA}, {Name: "dependency-b", Program: dependencyB}},
		Module: linkmodule.Spec{
			Actors:             []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{Actor: "actor", Instances: []string{"cache-main", "cache-a", "cache-b"}, Representative: "cache-a"}},
			AnalysisRoots: []linkmodule.AnalysisRootSpec{
				{Name: "main-root", Module: "main", Actor: "actor", Instance: "cache-main"},
				{Name: "a-root", Module: "dependency-a", Actor: "actor", Instance: "cache-a"},
				{Name: "b-root", Module: "dependency-b", Actor: "actor", Instance: "cache-b"},
			},
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{
				{Module: "main", Import: first.Term, FromRoot: "main-root", ToRoot: "a-root"},
				{Module: "main", Import: second.Term, FromRoot: "main-root", ToRoot: "b-root"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func suspensionProgram(t testing.TB, text string) *program.Program {
	t.Helper()
	value, err := lower.Lower(lower.Source{Name: "suspension.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
