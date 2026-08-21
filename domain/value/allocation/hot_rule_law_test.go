package allocation_test

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	allocation "github.com/wippyai/go-lua/domain/value/allocation"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func TestHotAllocationRuleBindsCarryReceiptAndRejectsForeignBinding(t *testing.T) {
	local := allocationFixture(t, "local allocation")
	foreign := allocationFixture(t, "foreign allocation")

	bind := func(fixture *allocationFixtureState) (*valueowner.HotOwner, *allocation.HotRule, *engine.SchemaBinding, *allocationcatalog.Catalog) {
		cold, ownerFragment, fragment, query := allocationColdSchema(t)
		binding := engine.NewSchemaBinding(cold)
		owner, ownerOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
		catalog, catalogFailure := allocationcatalog.BeginWithFailure(fixture.heaps, fixture.schema, fixture.mounts)
		rule, ruleOK := allocation.BindHot(fragment, owner, fixture.heaps, catalog)
		queryOK := valueowner.BindExactQuery(owner, query, engine.HotExactQuerySpec[valuedomain.Value, bool]{
			Project: func(engine.OrderedCells[valuedomain.Value]) bool { return true },
			Result:  allocationBoolResult(allocationKey(31_008)),
		})
		if !ownerOK || catalogFailure != allocationcatalog.SealFailureNone || !ruleOK || !queryOK || !binding.Seal() {
			return nil, nil, binding, catalog
		}
		return owner, rule, binding, catalog
	}

	owner, rule, binding, catalog := bind(local)
	if owner == nil || rule == nil || binding == nil || catalog == nil {
		t.Fatal("allocation receipt bind")
	}
	issuer, issued := rule.Implementation()
	if !issued || issuer == nil {
		t.Fatal("allocation typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementation(issuer); !ok || resolved == nil {
		t.Fatal("allocation sealed carry receipt")
	}

	foreignOwner, foreignRule, foreignBinding, foreignCatalog := bind(foreign)
	if foreignOwner == nil || foreignRule == nil || foreignBinding == nil || foreignCatalog == nil {
		t.Fatal("foreign allocation receipt bind")
	}
	foreignIssuer, issued := foreignRule.Implementation()
	if !issued || foreignIssuer == nil {
		t.Fatal("foreign allocation typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, issuer); ok || resolved != nil {
		t.Fatal("foreign equal-binding accepted the first receipt")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, foreignIssuer); !ok || resolved == nil {
		t.Fatal("foreign equal-binding rejected its own receipt")
	}
}

type allocationFixtureState struct {
	schema     *valuedomain.Schema
	heaps      heapdomain.Schema
	mounts     []heapdomain.ArtifactMount
	module     identity.ContentID
	occurrence identity.ContentID
}

func allocationFixture(t testing.TB, name string) *allocationFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local root = {}\nlocal alias = root\nreturn alias\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !ok || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	artifact, failure := artifactcompiler.CompileDetailed(programValue, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("schema seal heap=%s value=%s", heapFailure, valueFailure)
	}
	var occurrence identity.ContentID
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceAllocation {
			occurrence = row.ID()
			break
		}
	}
	if !occurrence.Available() {
		t.Fatal("allocation occurrence")
	}
	return &allocationFixtureState{schema: schema, heaps: heaps, mounts: []heapdomain.ArtifactMount{heapMount}, module: module, occurrence: occurrence}
}

func allocationColdSchema(t testing.TB) (*engine.Schema, *valueowner.SchemaFragment, *allocation.SchemaFragment, *engine.QuerySlot[bool]) {
	t.Helper()
	builder := engine.NewSchema()
	ownerFragment, ownerOK := valueowner.DeclareSchema(builder, allocationKey(31_001), allocationKey(31_002), allocationKey(31_101))
	fragment, fragmentOK := allocation.DeclareSchema(builder, allocationKey(31_003), allocationKey(31_004), allocationKey(31_005), ownerFragment)
	query, queryOK := engine.NewQuerySlot[bool](builder, engine.SchemaQuerySpec{Semantic: allocationKey(31_007), Freezer: allocationKey(31_008)})
	if !ownerOK || !fragmentOK || !queryOK || !engine.SchemaQueryRead(query, ownerFragment.ExactRead()) {
		t.Fatal("allocation cold schema")
	}
	cold, sealOK := builder.Seal()
	if !sealOK || cold == nil {
		t.Fatal("allocation cold schema seal")
	}
	return cold, ownerFragment, fragment, query
}

func allocationKey(number uint64) identity.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], number)
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("allocation semantic key")
	}
	return key
}

func allocationBoolResult(semantic identity.SemanticKey) engine.FrozenResult[bool] {
	return engine.FrozenResult[bool]{
		Semantic: semantic,
		Freeze:   func(value bool) bool { return value },
		Clone:    func(value bool) bool { return value },
		Equal:    func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
		Present: func(value bool) bool { return true },
	}
}
