package transfer_test

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
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	transfer "github.com/wippyai/go-lua/domain/value/transfer"
)

func TestHotStorageTransferBindsExactReadCarryReceiptAndRejectsForeignProof(t *testing.T) {
	local := transferFixture(t, "local transfer")
	foreign := transferFixture(t, "foreign transfer")

	bind := func(fixture *transferFixtureState) (*valueowner.HotOwner, *transfer.HotRule, *engine.SchemaBinding) {
		cold, ownerFragment, fragment := transferColdSchema(t)
		binding := engine.NewSchemaBinding(cold)
		owner, ownerOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
		rule, ruleOK := transfer.BindHot(fragment, owner)
		if !ownerOK || !ruleOK || !binding.Seal() {
			return nil, nil, binding
		}
		return owner, rule, binding
	}

	owner, rule, binding := bind(local)
	if owner == nil || rule == nil || binding == nil {
		t.Fatal("storage transfer receipt bind")
	}
	issuer, issued := rule.Implementation()
	if !issued || issuer == nil {
		t.Fatal("storage transfer typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementation(issuer); !ok || resolved == nil {
		t.Fatal("storage transfer receipt issue")
	}
	localTransfer, localOK := local.schema.StorageTransferAt(0)
	foreignTransfer, foreignOK := foreign.schema.StorageTransferAt(0)
	if !localOK || !foreignOK || !local.schema.OwnsStorageTransfer(localTransfer) || !foreign.schema.OwnsStorageTransfer(foreignTransfer) {
		t.Fatal("storage transfer operands")
	}
	foreignOwner, foreignRule, foreignBinding := bind(foreign)
	if foreignOwner == nil || foreignRule == nil || foreignBinding == nil {
		t.Fatal("foreign storage transfer receipt bind")
	}
	foreignIssuer, issued := foreignRule.Implementation()
	if !issued || foreignIssuer == nil {
		t.Fatal("foreign storage transfer typed Rule issuer")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, issuer); ok || resolved != nil {
		t.Fatal("foreign equal binding accepted local storage transfer receipt")
	}
	if resolved, ok := valueowner.ResolveRuleImplementationFor(foreignOwner, foreignIssuer); !ok || resolved == nil {
		t.Fatal("foreign equal binding rejected its own storage transfer receipt")
	}
}

type transferFixtureState struct {
	schema     *valuedomain.Schema
	module     identity.ContentID
	occurrence identity.ContentID
}

func transferFixture(t testing.TB, name string) *transferFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local n = 1\nlocal m = n\nn = m\nreturn n\n")})
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
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || schema.StorageTransferCount() == 0 {
		t.Fatalf("schema seal heap=%s value=%s transfers=%d", heapFailure, valueFailure, schema.StorageTransferCount())
	}
	var occurrence identity.ContentID
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceStorageBindTransfer {
			occurrence = row.ID()
			break
		}
	}
	if !occurrence.Available() {
		t.Fatal("storage transfer occurrence")
	}
	return &transferFixtureState{schema: schema, module: module, occurrence: occurrence}
}

func transferColdSchema(t testing.TB) (*engine.Schema, *valueowner.SchemaFragment, *transfer.SchemaFragment) {
	t.Helper()
	builder := engine.NewSchema()
	ownerFragment, ownerOK := valueowner.DeclareSchema(builder, transferKey(71_001), transferKey(71_002), transferKey(71_101))
	fragment, fragmentOK := transfer.DeclareSchema(builder, transferKey(71_003), transferKey(71_004), ownerFragment)
	if !ownerOK || !fragmentOK {
		t.Fatal("storage transfer cold schema")
	}
	cold, sealOK := builder.Seal()
	if !sealOK || cold == nil {
		t.Fatal("storage transfer cold schema seal")
	}
	return cold, ownerFragment, fragment
}

func transferKey(number uint64) identity.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], number)
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("storage transfer semantic key")
	}
	return key
}
