package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func moduleFixture(t *testing.T, dependencyText string) (*link.Link, linkmodule.ModuleInitGeneration, Schema, Key) {
	t.Helper()
	main := moduleProgram(t, `require("dependency")`)
	dependency := moduleProgram(t, dependencyText)
	require := target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"require"}}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{require},
			Input:    target.ValuesSpec{Tail: target.ValuesClosed},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape:    target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: moduleKey("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: moduleKey("__module_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: moduleKey("require"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: require}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: moduleKey("_G")},
			{Name: "__module_absent", Root: "GlobalEnvRoot", Key: moduleKey("__module_absent")},
			{Name: "require", Root: "GlobalEnvRoot", Key: moduleKey("require")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	imported, ok := main.Module().ImportAt(0)
	if !ok {
		t.Fatal("missing exact source Import")
	}
	sealed, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module: linkmodule.Spec{
			Actors: []linkmodule.ActorSpec{{Name: "actor"}},
			ModuleCacheAliases: []linkmodule.ModuleCacheAliasClassSpec{{
				Actor: "actor", Instances: []string{"cache-main", "cache-dependency"}, Representative: "cache-main",
			}},
			AnalysisRoots: []linkmodule.AnalysisRootSpec{
				{Name: "main-root", Module: "main", Actor: "actor", Instance: "cache-main"},
				{Name: "dependency-root", Module: "dependency", Actor: "actor", Instance: "cache-dependency"},
			},
			ModuleCacheEntries: []linkmodule.ModuleCacheEntrySpec{{Module: "main", Import: imported.Term, FromRoot: "main-root", ToRoot: "dependency-root"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := NewSchema(sealed)
	if !ok {
		t.Fatal("derive Module schema from Link")
	}
	generation, ok := sealed.Module().Generations().At(0)
	if !ok {
		t.Fatal("missing Link ModuleInit generation")
	}
	_, coordinate, _, _, ok := sealed.Module().Generations().Entry(generation)
	if !ok {
		t.Fatal("missing Link ModuleInit coordinate")
	}
	key, ok := schema.KeyForCoordinate(coordinate)
	if !ok {
		t.Fatal("Module schema omitted Link ModuleInit coordinate")
	}
	return sealed, generation, schema, key
}

func moduleProgram(t *testing.T, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "module.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func moduleKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}

func readySubject(t *testing.T, source *link.Link, generation linkmodule.ModuleInitGeneration) linkmodule.ModuleReadySubject {
	t.Helper()
	for index := 0; index < source.Module().Outcomes().Count(generation); index++ {
		outcome, ok := source.Module().Outcomes().At(generation, index)
		if !ok {
			t.Fatal("missing ModuleInit outcome")
		}
		subject, ok := source.Module().Outcomes().ReadySubject(outcome)
		if ok {
			return subject
		}
	}
	t.Fatal("missing ModuleInit Ready subject")
	return linkmodule.ModuleReadySubject{}
}

func TestSchemaDerivesExactModuleInitSupport(t *testing.T) {
	source, generation, schema, key := moduleFixture(t, `return 1`)
	pending, pendingOK := schema.Pending(key, generation, materialization.Recent)
	ready, readyOK := schema.PublishReady(pending, key, generation, materialization.Recent, readySubject(t, source, generation))
	bottom, bottomOK := schema.Bottom()
	defaultValue, defaultOK := schema.Default()
	top, topOK := schema.Top()
	lattice, latticeOK := schema.Lattice()
	if !pendingOK || !readyOK || !bottomOK || !defaultOK || !topOK || !latticeOK {
		t.Fatal("derive finite Module cache relation")
	}
	if pending.PendingCount() != 1 || ready.ReadyCount() != 1 || !schema.Admits(key, pending) || !schema.Admits(key, ready) {
		t.Fatal("Module schema did not admit its exact Link-derived support")
	}
	latticelaws.LawSuite[Value]{
		Name: "module-cache", Domain: lattice,
		Sample: []Value{bottom, defaultValue, pending, ready, lattice.Join(defaultValue, pending), lattice.Join(pending, ready), top},
	}.Run(t)
}

func TestSchemaMapsEmptyAndNilModuleReturnToDefaultTrue(t *testing.T) {
	for _, text := range []string{"return", "return nil"} {
		t.Run(text, func(t *testing.T) {
			source, generation, schema, key := moduleFixture(t, text)
			subject := readySubject(t, source, generation)
			if subject.Kind() != linkmodule.ModuleReadySubjectDefaultTrue {
				t.Fatalf("%q Ready kind = %v, want DefaultTrue", text, subject.Kind())
			}
			if _, ok := source.Module().ReadySubjects().Value(subject); ok {
				t.Fatal("DefaultTrue fabricated a Program Value")
			}
			pending, pendingOK := schema.Pending(key, generation, materialization.Recent)
			if ready, ok := schema.PublishReady(pending, key, generation, materialization.Recent, subject); !pendingOK || !ok || !schema.Admits(key, ready) {
				t.Fatal("Module schema rejected its exact DefaultTrue support")
			}
		})
	}
}

func TestSchemaRejectsOtherLinkAndOtherCoordinateSupport(t *testing.T) {
	leftLink, leftGeneration, left, leftKey := moduleFixture(t, `return 1`)
	rightLink, rightGeneration, right, rightKey := moduleFixture(t, `return 1`)
	leftValue, leftOK := left.Pending(leftKey, leftGeneration, materialization.Recent)
	rightValue, rightOK := right.Pending(rightKey, rightGeneration, materialization.Recent)
	if !leftOK || !rightOK {
		t.Fatal("construct exact Module values")
	}
	if left.Equal(leftValue, rightValue) || left.LessOrEq(leftValue, rightValue) {
		t.Fatal("Module value accepted a generation from another sealed Link")
	}
	if _, ok := left.Join(leftValue, rightValue); ok {
		t.Fatal("Module family joined cross-Link values")
	}
	if _, ok := left.Pending(leftKey, rightGeneration, materialization.Recent); ok {
		t.Fatal("Module coordinate admitted a foreign Link generation")
	}
	if leftLink.ContentID() != rightLink.ContentID() {
		t.Fatal("equivalent structural fixture changed Link content")
	}
	if _, ok := NewSchema(nil); ok {
		t.Fatal("Module schema accepted no Link authority")
	}
}

func TestPendingCreatesOnlyFreshReferenceAndReplacementPreservesAlternatives(t *testing.T) {
	source, site, schema, key := moduleFixture(t, `return 1`)
	fresh, freshOK := schema.Pending(key, site, materialization.Recent)
	if !freshOK {
		t.Fatal("construct the exact fresh pending reference")
	}
	for _, role := range []materialization.Role{materialization.Invalid, materialization.Exact, materialization.Summary} {
		if _, ok := schema.Pending(key, site, role); ok {
			t.Fatalf("fresh Pending admitted non-fresh role %v", role)
		}
	}
	readySubject := readySubject(t, source, site)
	ready, readyOK := schema.PublishReady(fresh, key, site, materialization.Recent, readySubject)
	cold, coldOK := schema.Default()
	withCold, coldJoinOK := schema.Join(cold, fresh)
	current, currentOK := schema.Join(withCold, ready)
	if !readyOK || !coldOK || !coldJoinOK || !currentOK {
		t.Fatal("construct unrelated cache alternatives")
	}
	if _, ok := schema.ReplacePending(current, key, site, materialization.Summary, site, materialization.Recent); ok {
		t.Fatal("replacement accepted a mismatched expected pending reference")
	}
	replaced, replacedOK := schema.ReplacePending(current, key, site, materialization.Recent, site, materialization.Summary)
	if !replacedOK || !replaced.HasCold() || replaced.PendingCount() != 1 || replaced.ReadyCount() != 1 || !schema.Admits(key, replaced) {
		t.Fatal("exact replacement did not preserve unrelated cache alternatives")
	}
	gotSite, gotRole, gotOK := replaced.PendingAt(0)
	if !gotOK || gotSite != site || gotRole != materialization.Summary {
		t.Fatal("replacement did not use the caller-supplied pending reference")
	}
	gotReadySite, gotReadySubject, gotReadyOK := replaced.ReadyAt(0)
	if !gotReadyOK || gotReadySite != site || gotReadySubject != readySubject {
		t.Fatal("replacement changed an unrelated ready alternative")
	}
}

func TestPublishReadyRequiresItsExactPendingReference(t *testing.T) {
	source, site, schema, key := moduleFixture(t, `return 1`)
	subject := readySubject(t, source, site)
	recent, recentOK := schema.Pending(key, site, materialization.Recent)
	summary, summaryOK := schema.ReplacePending(recent, key, site, materialization.Recent, site, materialization.Summary)
	cold, coldOK := schema.Default()
	if !recentOK || !summaryOK || !coldOK {
		t.Fatal("publication setup")
	}
	if _, ok := schema.PublishReady(summary, key, site, materialization.Recent, subject); ok {
		t.Fatal("mismatched Recent reference published from Summary pending")
	}
	if _, ok := schema.PublishReady(recent, key, site, materialization.Summary, subject); ok {
		t.Fatal("mismatched Summary reference published from Recent pending")
	}
	if _, ok := schema.PublishReady(cold, key, site, materialization.Recent, subject); ok {
		t.Fatal("missing pending site published a ready subject")
	}
	ready, ok := schema.PublishReady(summary, key, site, materialization.Summary, subject)
	if !ok || ready.PendingCount() != 0 || ready.ReadyCount() != 1 || !schema.Admits(key, ready) {
		t.Fatal("exact Summary pending did not publish its exact ready subject")
	}
	gotSite, gotSubject, gotOK := ready.ReadyAt(0)
	if !gotOK || gotSite != site || gotSubject != subject {
		t.Fatal("ready state lost its structural source site")
	}
}

// TestModuleInitConsumesOnlyColdFromTheCanonicalGeneration proves the cache
// half of ModuleInit needs no sibling Suspension patch. It uses only its Cold
// predecessor and the canonical cache-entry-derived fresh generation.
func TestModuleInitConsumesColdWithoutReadingSuspensionPatch(t *testing.T) {
	_, generation, schema, key := moduleFixture(t, `return 1`)
	cold, coldOK := schema.Default()
	if !coldOK {
		t.Fatal("ModuleInit generation fixture")
	}
	pending, pendingOK := schema.BeginInit(cold, key, generation)
	if !pendingOK || pending.HasCold() || pending.PendingCount() != 1 || !schema.Admits(key, pending) {
		t.Fatal("ModuleInit did not produce only its exact pending patch")
	}
	site, role, present := pending.PendingAt(0)
	if !present || site != generation || role != materialization.Recent {
		t.Fatalf("ModuleInit pending = %v/%v/%v", site, role, present)
	}
	if _, ok := schema.BeginInit(pending, key, generation); ok {
		t.Fatal("ModuleInit accepted a non-Cold predecessor")
	}
}

// TestModuleRestoreColdRejectsAStaleGeneration proves an old completion or
// cancellation cannot mutate a newer cache alternative.
func TestModuleRestoreColdRequiresTheExactCurrentGeneration(t *testing.T) {
	_, generation, schema, key := moduleFixture(t, `return 1`)
	pending, pendingOK := schema.Pending(key, generation, materialization.Recent)
	if !pendingOK {
		t.Fatal("pending fixture")
	}
	restored, restoredOK := schema.RestoreCold(pending, key, generation, materialization.Recent)
	if !restoredOK || !restored.HasCold() || restored.PendingCount() != 0 || !schema.Admits(key, restored) {
		t.Fatal("matching pending generation did not restore Cold")
	}
	if _, ok := schema.RestoreCold(restored, key, generation, materialization.Recent); ok {
		t.Fatal("stale generation restored an already-retried cache alternative")
	}
}
