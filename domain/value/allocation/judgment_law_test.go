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
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	allocation "github.com/wippyai/go-lua/domain/value/allocation"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// allocationAuthorities is the composition record this rule's family install
// arm reads: the one Value owner the declaration names.
type allocationAuthorities struct {
	owner *valueowner.HotOwner
}

func (authorities allocationAuthorities) ValueAuthority() *valueowner.HotOwner {
	return authorities.owner
}

// TestAllocationJudgmentAnswersOnlyForTheSchemaThatIssuedTheReceipt is the
// authentication law of the read-free fold. The rule declares no read, so the
// receipt it is indexed by is the whole of its evidence, and a receipt is
// evidence only for the Value schema that sealed it. Every receipt of the
// schema the judgment was derived against answers with the canonical Recent
// fact it was issued with; a receipt of an equal foreign schema is refused
// rather than answered, because the coordinate and the fact it carries belong
// to a numbering this judgment never issued.
func TestAllocationJudgmentAnswersOnlyForTheSchemaThatIssuedTheReceipt(t *testing.T) {
	local := allocationFixture(t, "local allocation")
	foreign := allocationFixture(t, "foreign allocation")

	judgment, judgmentOK := allocation.Derive(local.schema)
	if !judgmentOK || !judgment.Valid() {
		t.Fatal("allocation judgment did not seal against its own Value schema")
	}
	if unsealed, unsealedOK := allocation.Derive(nil); unsealedOK || unsealed.Valid() {
		t.Fatal("allocation judgment sealed without a Value schema")
	}

	receipts := local.schema.AllocationResultCount()
	if receipts == 0 {
		t.Fatal("fixture sealed no allocation receipt")
	}
	for index := 0; index < receipts; index++ {
		receipt, receiptOK := local.schema.AllocationResultAt(index)
		if !receiptOK {
			t.Fatalf("AllocationResultAt(%d)", index)
		}
		fresh, freshOK := receipt.Fresh()
		fact, outcome := judgment.Result(receipt)
		if !freshOK || outcome != structure.Concrete || !local.schema.Same(fact, fresh) {
			t.Fatalf("receipt %d answered %v fresh=%t same=%t", index, outcome, freshOK, local.schema.Same(fact, fresh))
		}
	}

	foreignReceipts := foreign.schema.AllocationResultCount()
	if foreignReceipts == 0 {
		t.Fatal("foreign fixture sealed no allocation receipt")
	}
	for index := 0; index < foreignReceipts; index++ {
		receipt, receiptOK := foreign.schema.AllocationResultAt(index)
		if !receiptOK {
			t.Fatalf("foreign AllocationResultAt(%d)", index)
		}
		if _, outcome := judgment.Result(receipt); outcome != structure.Refuse {
			t.Fatalf("foreign receipt %d answered %v", index, outcome)
		}
	}

	own, ownOK := local.schema.AllocationResultAt(0)
	if !ownOK {
		t.Fatal("first local allocation receipt")
	}
	var unsealed allocation.Judgment
	if _, outcome := unsealed.Result(own); outcome != structure.Refuse {
		t.Fatalf("unsealed judgment answered %v", outcome)
	}
	if _, outcome := judgment.Result(nil); outcome != structure.Refuse {
		t.Fatalf("absent candidate answered %v", outcome)
	}
}

// TestAllocationFamilyInstallsOnlyAgainstTheBindingItsOwnerWasIssuedBy is the
// install-time fence. Two structurally equal compositions issue two owners, and
// the family is sealed against the axis schema of exactly one of them: an owner
// of an equal foreign binding names the same Factor and the same semantic keys,
// so nothing but the binding identity distinguishes it, and installing against
// it would seal a family onto cells another composition owns.
func TestAllocationFamilyInstallsOnlyAgainstTheBindingItsOwnerWasIssuedBy(t *testing.T) {
	localOwner, localBinding := allocationOwner(t, allocationFixture(t, "local allocation"))
	foreignOwner, foreignBinding := allocationOwner(t, allocationFixture(t, "foreign allocation"))

	if !localOwner.MatchesBinding(localBinding) || !foreignOwner.MatchesBinding(foreignBinding) {
		t.Fatal("an owner did not answer for the binding that issued it")
	}
	if localOwner.MatchesBinding(foreignBinding) || foreignOwner.MatchesBinding(localBinding) {
		t.Fatal("an owner answered for a foreign equal binding")
	}
	if allocation.InstallFamily(localBinding, nil, allocationAuthorities{owner: foreignOwner}) {
		t.Fatal("the family installed against an owner of a foreign binding")
	}
	if allocation.InstallFamily(localBinding, nil, allocationAuthorities{}) {
		t.Fatal("the family installed without a Value owner")
	}
	if allocation.InstallFamily(nil, nil, allocationAuthorities{owner: localOwner}) {
		t.Fatal("the family installed without a binding")
	}
}

// allocationFixtureState is one sealed composition the laws below answer over:
// the Value schema that issued the allocation receipts, and the mounted
// allocation occurrence they were sealed for.
type allocationFixtureState struct {
	schema     *valuedomain.Schema
	occurrence identity.ContentID
}

func allocationFixture(t testing.TB, name string) *allocationFixtureState {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local root = {}\nlocal alias = root\nreturn alias\n")})
	if err != nil {
		t.Fatal(err)
	}
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{requireOperation}})
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
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
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
	return &allocationFixtureState{schema: schema, occurrence: occurrence}
}

// allocationOwner seals one composition's Value owner: the cold schema its
// factor and query are declared in, and the binding that issues it. Two calls
// produce two structurally equal compositions, which is what an owner fence has
// to distinguish.
func allocationOwner(t testing.TB, fixture *allocationFixtureState) (*valueowner.HotOwner, *engine.SchemaBinding) {
	t.Helper()
	builder := engine.NewSchema()
	ownerFragment, ownerOK := valueowner.DeclareSchema(builder, allocationKey(31_001), allocationKey(31_002), allocationKey(31_101))
	query, queryOK := engine.NewQuerySlot[bool](builder, engine.SchemaQuerySpec{Semantic: allocationKey(31_007), Freezer: allocationKey(31_008), Population: queryschema.PopulationKindSelectedPoint})
	readOK := engine.SchemaQueryRead(query, ownerFragment.ExactRead())
	if !ownerOK || !queryOK || !readOK {
		t.Fatalf("allocation cold schema owner=%t query=%t read=%t", ownerOK, queryOK, readOK)
	}
	cold, sealOK := builder.Seal()
	if !sealOK || cold == nil {
		t.Fatal("allocation cold schema seal")
	}
	binding := engine.NewSchemaBinding(cold)
	owner, boundOK := valueowner.BindHot(binding, ownerFragment, fixture.schema)
	queryBoundOK := valueowner.BindExactQuery(owner, query, engine.HotExactQuerySpec[valuedomain.Value, bool]{
		Project: func(engine.OrderedCells[valuedomain.Value]) bool { return true },
		Result:  allocationBoolResult(allocationKey(31_008)),
	})
	if !boundOK || owner == nil || !queryBoundOK || !binding.Seal() {
		t.Fatal("allocation owner binding")
	}
	return owner, binding
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
