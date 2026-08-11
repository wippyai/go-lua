package bootstrap

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestHostGlobalBootstrapRunsThroughSourceAssemblyAndSolver(t *testing.T) {
	schema, linked, globals := bootstrapFixture(t)
	root := bootstrapGlobal(t, linked, globals, "_G")
	result, ok := globalResultForTest(t, schema, root)
	if !ok || result.absent || schema.Equal(result.fact, schema.Default()) {
		t.Fatal("_G bootstrap result unavailable")
	}

	composition := engine.NewComposition()
	owner, ownerOK := valueowner.Declare(composition, bootstrapKey(1), bootstrapKey(2), schema)
	if !ownerOK {
		t.Fatalf("bootstrap Value owner (coordinates=%d)", schema.CoordinateCount())
	}
	rule, ruleOK := Declare(composition, bootstrapKey(3), bootstrapKey(4), bootstrapKey(5), owner)
	var read engine.QueryRead[engine.OrderedCells[value.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: bootstrapKey(6),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, ok := engine.QueryValue(row, read)
				actual, present, available := cells.At(0)
				return ok && rows == 1 && available && present && schema.Equal(actual, result.fact)
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: bootstrapKey(7),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var ok bool
		read, ok = engine.QueryReadFrom(query, owner.ExactRead())
		return ok
	})
	if !ownerOK || !ruleOK || !queryOK || !composition.Seal() {
		t.Fatal("bootstrap declaration/seal")
	}
	report, reportOK := composition.SemanticReport()
	if !reportOK || len(report.Rules) != 1 || report.Rules[0].Inputs != 0 || len(report.Rules[0].Reads) != 0 ||
		len(report.Rules[0].Writes) != 1 || report.Rules[0].Writes[0].Kind != engine.RuleWriteDispositionExact ||
		report.Rules[0].Writes[0].Factor != bootstrapKey(1) {
		t.Fatal("bootstrap rule did not retain zero reads and one exact Value write")
	}
	ref, refOK := owner.Locate(result.coordinate)
	instance, instanceOK := rule.Instance(root)
	if !refOK || !instanceOK {
		t.Fatal("bootstrap instance")
	}
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(bootstrapKey(8), scope, truth, true)
	occurrence, occurrenceOK := source.At(site)
	prepared, operandOK := source.PrepareInstance(occurrence, instance)
	if !scopeOK || !truthOK || !siteOK || !occurrenceOK || !operandOK || !source.Seal() {
		t.Fatal("bootstrap source assembly")
	}
	assembled := false
	var queryInstance *engine.QueryInstance[bool]
	solver, solverOK := source.Assemble(func(assembly *engine.Assembly) bool {
		point, pointOK := assembly.Point(site)
		member, memberOK := assembly.Member(point, prepared)
		group, groupOK := assembly.Group(point, member)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = engine.NewQueryInstance(query, func(binding *engine.QueryBinding[bool]) bool { return engine.InstanceQueryRead(binding, read, ref) })
		_, attached := assembly.Query(point, queryInstance)
		assembled = pointOK && memberOK && groupOK && group.Available() && queryInstanceOK && attached
		return assembled
	})
	if !solverOK || solver == nil || !assembled {
		t.Fatal("bootstrap solver assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	var got bool
	var readable bool
	if receiptOK {
		got, readable = engine.QueryResult(receipt, state)
	}
	if status != engine.SolveComplete || !receiptOK || !readable || !got {
		t.Fatalf("bootstrap solve status=%v readable=%t result=%t", status, readable, got)
	}
}

func TestHostGlobalBootstrapAbsentAndForeignFences(t *testing.T) {
	schema, linked, globals := bootstrapFixture(t)
	absent := bootstrapGlobal(t, linked, globals, "__link_absent")
	result, ok := globalResultForTest(t, schema, absent)
	if !ok || !result.absent || !schema.Equal(result.fact, value.Value{}) {
		t.Fatal("absent global did not yield a valid no-candidate judgment")
	}
	if _, overlap := schema.SourceSeed(mustGlobalValue(t, linked, absent)); overlap {
		t.Fatal("global bootstrap overlaps Value SourceSeed")
	}

	foreignSchema, foreignLink, foreignGlobals := bootstrapFixture(t)
	if foreignLink.ContentID() != linked.ContentID() || foreignLink.Host() == linked.Host() || foreignSchema == schema {
		t.Fatal("fixture did not produce same-content foreign Host")
	}
	foreign := bootstrapGlobal(t, foreignLink, foreignGlobals, "_G")
	if _, ok := globalResult(schemaOwnerForTest(t, schema), foreign); ok {
		t.Fatal("foreign same-content Host global crossed Value fence")
	}
}

func TestHostGlobalBootstrapDistinguishesAbsentNilAndJoinsActorCopies(t *testing.T) {
	schema, linked, globals := bootstrapFixture(t)
	absent := bootstrapGlobal(t, linked, globals, "__link_absent")
	nilGlobal := bootstrapGlobal(t, linked, globals, "nil_global")
	absentResult, absentOK := globalResultForTest(t, schema, absent)
	nilResult, nilOK := globalResultForTest(t, schema, nilGlobal)
	if !absentOK || !absentResult.absent || !nilOK || nilResult.absent || schema.Equal(nilResult.fact, schema.Default()) {
		t.Fatal("absent and nil global bootstrap collapsed")
	}

	actorSchema, actorLink, actorGlobals := bootstrapFixtureWithActors(t, 2)
	bindings := bootstrapGlobals(t, actorLink, actorGlobals, "_G")
	if len(bindings) != 2 {
		t.Fatalf("actor _G bindings=%d, want 2", len(bindings))
	}
	left, leftOK := globalResultForTest(t, actorSchema, bindings[0])
	right, rightOK := globalResultForTest(t, actorSchema, bindings[1])
	joined, joinedOK := actorSchema.Join(left.fact, right.fact)
	if !leftOK || !rightOK || left.absent || right.absent || left.coordinate != right.coordinate || !joinedOK ||
		!actorSchema.LessOrEq(left.fact, joined) || !actorSchema.LessOrEq(right.fact, joined) {
		t.Fatal("actor-local boot facts sharing one Program cell did not join at one Value coordinate")
	}
}

func bootstrapFixture(t testing.TB) (*value.Schema, *link.Link, linkhost.Globals) {
	return bootstrapFixtureWithActors(t, 1)
}

func bootstrapFixtureWithActors(t testing.TB, actors int) (*value.Schema, *link.Link, linkhost.Globals) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "value-host-bootstrap.lua", Text: []byte("local root = _G\nlocal absent = __link_absent\nlocal nilvalue = nil_global\nreturn root, absent, nilvalue")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{
		InitialRoots: []target.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: target.BootShapeSpec{Aggregate: target.BootAggregateTable, Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("nil_global"), Value: target.InitialValueSpec{Kind: target.InitialValueNil}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{{Name: "_G", Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("_G")}, {Name: "__link_absent", Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("__link_absent")}, {Name: "nil_global", Root: "GlobalEnvRoot", Key: bootstrapKeyLiteral("nil_global")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := &link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}}
	if actors > 1 {
		spec.Module.Actors = make([]linkmodule.ActorSpec, actors)
		spec.Module.ModuleCacheAliases = make([]linkmodule.ModuleCacheAliasClassSpec, actors)
		spec.Module.AnalysisRoots = make([]linkmodule.AnalysisRootSpec, actors)
		for index := 0; index < actors; index++ {
			ordinal := string(rune('a' + index))
			actor, instance := "actor-"+ordinal, "cache-"+ordinal
			spec.Module.Actors[index] = linkmodule.ActorSpec{Name: actor}
			spec.Module.ModuleCacheAliases[index] = linkmodule.ModuleCacheAliasClassSpec{Actor: actor, Instances: []string{instance}, Representative: instance}
			spec.Module.AnalysisRoots[index] = linkmodule.AnalysisRootSpec{Name: "root-" + ordinal, Module: "main", Actor: actor, Instance: instance}
		}
	}
	linked, err := link.Seal(spec)
	if err != nil {
		t.Fatal(err)
	}
	heaps, heapsOK := heap.Seal(linked)
	schema, schemaOK := value.Seal(linked, heaps)
	if !heapsOK || !schemaOK {
		t.Fatal("Value schema")
	}
	return schema, linked, linked.Host().Globals()
}

func bootstrapGlobal(t testing.TB, linked *link.Link, globals linkhost.Globals, name string) linkhost.GlobalBinding {
	t.Helper()
	bindings := bootstrapGlobals(t, linked, globals, name)
	if len(bindings) != 0 {
		return bindings[0]
	}
	t.Fatalf("global %q unavailable", name)
	return linkhost.GlobalBinding{}
}

func bootstrapGlobals(t testing.TB, linked *link.Link, globals linkhost.Globals, name string) []linkhost.GlobalBinding {
	t.Helper()
	result := make([]linkhost.GlobalBinding, 0, 1)
	for index := 0; index < globals.Count(); index++ {
		binding, ok := globals.At(index)
		if !ok {
			continue
		}
		analysis, _, _, key, _, _, mappingOK := globals.Mapping(binding)
		if !mappingOK {
			continue
		}
		// Mapping's key is canonical Program source identity. Resolve its name
		// through the exact root shard below rather than comparing a copied row.
		rootShard, _, _, rootOK := linked.Module().Roots().Mapping(analysis)
		p, programOK := linked.Project().Mounts().Program(rootShard)
		if !mappingOK || !rootOK || !programOK {
			continue
		}
		literal, literalOK := p.Source().Keys().Exact(key)
		if literalOK && literal.Kind == keyspace.LiteralString && literal.String == name {
			result = append(result, binding)
		}
	}
	return result
}

func mustGlobalValue(t testing.TB, linked *link.Link, binding linkhost.GlobalBinding) linkboundary.Value {
	t.Helper()
	analysis, _, cell, _, _, _, mappingOK := linked.Host().Globals().Mapping(binding)
	shard, _, _, rootOK := linked.Module().Roots().Mapping(analysis)
	subject, subjectOK := linked.Boundary().Values().Of(shard, cell)
	if !mappingOK || !rootOK || !subjectOK {
		t.Fatal("global Value reconstruction")
	}
	return subject
}

func globalResultForTest(t testing.TB, schema *value.Schema, binding linkhost.GlobalBinding) (result, bool) {
	t.Helper()
	owner := schemaOwnerForTest(t, schema)
	return globalResult(owner, binding)
}

func schemaOwnerForTest(t testing.TB, schema *value.Schema) *valueowner.Owner {
	t.Helper()
	composition := engine.NewComposition()
	owner, ok := valueowner.Declare(composition, bootstrapKey(90), bootstrapKey(91), schema)
	if !ok {
		t.Fatal("test owner")
	}
	return owner
}

func bootstrapKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("bootstrap semantic key")
	}
	return key
}
func bootstrapKeyLiteral(value string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
}
