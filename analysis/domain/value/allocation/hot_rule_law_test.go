package allocation_test

import (
	"encoding/binary"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	allocation "github.com/wippyai/go-lua/analysis/domain/value/allocation"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestHotAllocationRuleBindsCarryReceiptAndRejectsForeignBinding(t *testing.T) {
	local := allocationFixture(t, "local allocation")
	foreign := allocationFixture(t, "foreign allocation")

	bind := func(fixture *allocationFixtureState) (*valueowner.HotOwner, *allocation.HotRule, *engine.SchemaBinding, *allocationcatalog.Catalog) {
		cold, ownerFragment, fragment, query := allocationColdSchema(t)
		binding := engine.NewSchemaBinding(cold)
		owner, ownerOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
		catalog, catalogFailure := allocationcatalog.BeginWithFailure(fixture.heaps, fixture.schema, owner, fixture.mounts)
		rule, ruleOK := allocation.BindHot(fragment, owner, fixture.heaps, catalog)
		queryOK := valueowner.BindExactQuery(owner, query, engine.HotExactQuerySpec[valuedomain.Value, bool]{
			Project: func(engine.OrderedCells[valuedomain.Value]) bool { return true },
			Result:  allocationBoolResult(allocationKey(31_008)),
		})
		if !ownerOK || catalogFailure != allocationcatalog.SealFailureNone || !ruleOK || !queryOK || !binding.Seal() || catalog.SealSummaryReceiptsWithFailure() != allocationcatalog.SealFailureNone {
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

func TestHotAllocationReceiptRejectsForeignMountAndOccurrence(t *testing.T) {
	local := allocationFixture(t, "local mount")
	foreign := allocationFixture(t, "foreign mount")
	bind := func(fixture *allocationFixtureState) (*valueowner.HotOwner, *allocation.HotRule, *allocationcatalog.Catalog) {
		cold, ownerFragment, fragment, query := allocationColdSchema(t)
		binding := engine.NewSchemaBinding(cold)
		owner, ownerOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
		catalog, catalogFailure := allocationcatalog.BeginWithFailure(fixture.heaps, fixture.schema, owner, fixture.mounts)
		rule, ruleOK := allocation.BindHot(fragment, owner, fixture.heaps, catalog)
		queryOK := valueowner.BindExactQuery(owner, query, engine.HotExactQuerySpec[valuedomain.Value, bool]{
			Project: func(engine.OrderedCells[valuedomain.Value]) bool { return true },
			Result:  allocationBoolResult(allocationKey(31_008)),
		})
		if !ownerOK || catalogFailure != allocationcatalog.SealFailureNone || !ruleOK || !queryOK || !binding.Seal() || catalog.SealSummaryReceiptsWithFailure() != allocationcatalog.SealFailureNone {
			return nil, nil, nil
		}
		return owner, rule, catalog
	}
	_, localRule, localCatalog := bind(local)
	_, foreignRule, foreignCatalog := bind(foreign)
	if localRule == nil || localCatalog == nil || foreignRule == nil || foreignCatalog == nil {
		t.Fatal("allocation mounted receipt bind")
	}
	localMount, localMountOK := localCatalog.ForMount(local.module)
	foreignMount, foreignMountOK := foreignCatalog.ForMount(foreign.module)
	if !localMountOK || !foreignMountOK {
		t.Fatal("allocation mounted catalog")
	}
	localKey, localKeyOK := localMount.KeyForOccurrence(local.occurrence)
	foreignKey, foreignKeyOK := foreignMount.KeyForOccurrence(foreign.occurrence)
	if !localKeyOK || !foreignKeyOK || localKey == foreignKey {
		t.Fatal("allocation mounted keys")
	}
	localIssuer, localIssuerOK := localRule.ForMount(local.module)
	if !localIssuerOK {
		t.Fatal("local allocation mount issuer")
	}
	if _, ok := localIssuer.ReceiptForOccurrence(local.occurrence); !ok {
		t.Fatal("local allocation occurrence receipt rejected")
	}
	_, foreignIssuerOK := localRule.ForMount(foreign.module)
	if foreignIssuerOK {
		t.Fatal("foreign allocation occurrence crossed local mount")
	}
}

type allocationFixtureState struct {
	schema     *valuedomain.Schema
	heaps      heapdomain.Schema
	mounts     []heapdomain.ArtifactMount
	module     keyspace.ContentID
	occurrence keyspace.ContentID
}

func allocationFixture(t testing.TB, name string) *allocationFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local root = {}\nlocal alias = root\nreturn alias\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := programschema.Global()
	if !ok {
		t.Fatal("program schema receipt")
	}
	artifact, failure := schemaadapter.CompileDetailed(programValue.TransformerInput(), receipt)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(artifact, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(artifact, module, programID)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount})
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("schema seal heap=%s value=%s", heapFailure, valueFailure)
	}
	var occurrence keyspace.ContentID
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, rowOK := artifact.OccurrenceAt(index)
		if rowOK && row.Kind() == programartifact.OccurrenceAllocation {
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
	ownerFragment, ownerOK := valueowner.DeclareSchema(builder, allocationKey(31_001), allocationKey(31_002))
	fragment, fragmentOK := allocation.DeclareSchema(builder, allocationKey(31_003), allocationKey(31_004), allocationKey(31_005), allocationKey(31_006), ownerFragment)
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

func allocationKey(number uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], number)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("allocation semantic key")
	}
	return key
}

func allocationBoolResult(semantic engine.SemanticKey) engine.FrozenResult[bool] {
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
	}
}
