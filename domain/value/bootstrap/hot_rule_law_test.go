package bootstrap_test

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	bootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func TestHotBootstrapRuleBindsReceiptAndRejectsForeignOperand(t *testing.T) {
	local := bootstrapFixture(t, "local bootstrap")
	foreign := bootstrapFixture(t, "foreign bootstrap")

	bind := func(fixture *bootstrapFixtureState) (*valueowner.HotOwner, *bootstrap.HotRule, *engine.SchemaBinding) {
		cold, ownerFragment, fragment := bootstrapColdSchema(t)
		binding := engine.NewSchemaBinding(cold)
		owner, ownerOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
		rule, ruleOK := bootstrap.BindHot(fragment, owner)
		if !ownerOK || !ruleOK || !binding.Seal() {
			return nil, nil, binding
		}
		return owner, rule, binding
	}

	owner, rule, binding := bind(local)
	if owner == nil || rule == nil || binding == nil {
		t.Fatal("bootstrap receipt bind")
	}
	issuer, issued := rule.Implementation()
	if !issued || issuer == nil {
		t.Fatal("bootstrap typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementation(issuer); !ok || resolved == nil {
		t.Fatal("bootstrap sealed receipt")
	}
	if catalog := rule.Catalog(); catalog == nil || catalog.Count() != local.schema.GlobalBootstrapResultCount() {
		t.Fatal("bootstrap catalog")
	}
	localID, localOK := local.schema.GlobalBootstrapResultIDAt(0)
	foreignID, foreignOK := foreign.schema.GlobalBootstrapResultIDAt(0)
	if !localOK || !foreignOK || localID == foreignID {
		t.Fatal("bootstrap global identities")
	}
	if receipt, ok := rule.ReceiptForID(localID); !ok || receipt != localID {
		t.Fatal("local bootstrap receipt rejected")
	}
	if receipt, ok := rule.ReceiptForID(foreignID); ok || receipt != (identity.ContentID{}) {
		t.Fatal("foreign bootstrap receipt crossed Value owner")
	}
	if receipt, ok := rule.ReceiptForID(identity.ContentID{}); ok || receipt != (identity.ContentID{}) {
		t.Fatal("zero bootstrap receipt accepted")
	}

	foreignOwner, foreignRule, foreignBinding := bind(foreign)
	if foreignOwner == nil || foreignRule == nil || foreignBinding == nil {
		t.Fatal("foreign bootstrap receipt bind")
	}
	foreignIssuer, issued := foreignRule.Implementation()
	if !issued || foreignIssuer == nil {
		t.Fatal("foreign bootstrap typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, issuer); ok || resolved != nil {
		t.Fatal("foreign equal-binding accepted the first bootstrap receipt")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, foreignIssuer); !ok || resolved == nil {
		t.Fatal("foreign equal-binding rejected its own receipt")
	}
}

type bootstrapFixtureState struct {
	schema *valuedomain.Schema
}

func bootstrapFixture(t testing.TB, name string) *bootstrapFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local root = _G\nlocal absent = __link_absent\nreturn root, absent\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics:    domaincontract.NewSemantics(),
		InitialRoots: []vocabulary.InitialRootSpec{{Identity: "GlobalEnvRoot", Shape: vocabulary.BootShapeSpec{Aggregate: vocabulary.BootAggregateTable, Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}}}},
		InitialEntries: []vocabulary.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: bootstrapLiteral("_G"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: vocabulary.InitialMutable},
			{Root: "GlobalEnvRoot", Key: bootstrapLiteral("__link_absent"), Value: vocabulary.InitialValueSpec{Kind: vocabulary.InitialValueAbsent}, Mutability: vocabulary.InitialMutable},
		},
		InitialBindings: []vocabulary.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: bootstrapLiteral("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: bootstrapLiteral("__link_absent")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Global()
	grammar, grammarOK := composite.ArtifactGrammar(receipt)
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory()
	if !ok || !grammarOK || !issuanceOK {
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
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || schema.GlobalBootstrapResultCount() == 0 {
		t.Fatalf("schema seal heap=%s value=%s globals=%d", heapFailure, valueFailure, schema.GlobalBootstrapResultCount())
	}
	return &bootstrapFixtureState{schema: schema}
}

func bootstrapColdSchema(t testing.TB) (*engine.Schema, *valueowner.SchemaFragment, *bootstrap.SchemaFragment) {
	t.Helper()
	builder := engine.NewSchema()
	ownerFragment, ownerOK := valueowner.DeclareSchema(builder, bootstrapKey(32_001), bootstrapKey(32_002), bootstrapKey(32_101))
	fragment, fragmentOK := bootstrap.DeclareSchema(builder, bootstrapKey(32_003), bootstrapKey(32_004), bootstrapKey(32_005), ownerFragment)
	if !ownerOK || !fragmentOK {
		t.Fatal("bootstrap cold schema")
	}
	cold, sealOK := builder.Seal()
	if !sealOK || cold == nil {
		t.Fatal("bootstrap cold schema seal")
	}
	return cold, ownerFragment, fragment
}

func bootstrapKey(number uint64) identity.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], number)
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("bootstrap semantic key")
	}
	return key
}

func bootstrapLiteral(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}
