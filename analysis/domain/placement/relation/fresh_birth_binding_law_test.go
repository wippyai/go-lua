package relation_test

import (
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// realFreshResult seals the same one-call Target artifact used by Value's
// FreshResult laws.  Keeping this small cold seal local to the law means the
// binding is exercised with Value's issued operand and fact, not with a
// hand-built struct whose private owner fence was never proven.
type realFreshResult struct {
	values    *valuedomain.Schema
	candidate valuedomain.FreshResultCall
	fact      valuedomain.Value
}

func sealRealFreshResult(t *testing.T) realFreshResult {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(contract, "placement_fresh_birth_binding.lua", []byte("local co = coroutine.create(function() end)\nreturn co\n"))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mounted, mountedOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountedOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mounted})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{mounted}), []programmount.MountedArtifact{mounted}, structural)
	if heapFailure != heap.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil || values.FreshResultCallCount() == 0 {
		t.Fatalf("seal schemas heap=%s value=%s fresh=%d", heapFailure, valueFailure, values.FreshResultCallCount())
	}
	candidate, candidateOK := values.FreshResultCallAt(0)
	key, keyOK := candidate.Key()
	fact, factOK := values.FreshResultFact(key, materialization.Recent)
	if !candidateOK || !keyOK || !factOK || !values.OwnsFreshResultCall(candidate) {
		t.Fatal("sealed fresh-result operand")
	}
	return realFreshResult{values: values, candidate: candidate, fact: fact}
}

func freshBirthCardinality(t testing.TB) model.Cardinality {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("fresh-birth cardinality")
	}
	return cardinality
}

func TestPlacementFreshBirthBindsRealValueCandidateAndFact(t *testing.T) {
	real := sealRealFreshResult(t)
	place := harness.New(t, "row/fresh-birth")
	valueType := place.TypeID(t, "type/value")
	candidateType := place.TypeID(t, "type/fresh-result-candidate")
	placementType := place.TypeID(t, "type/placement")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	candidateColumn := harness.NewColumn[valuedomain.FreshResultCall](t, candidateType, "store/fresh-result-candidate", reserve)
	placementColumn := harness.NewColumn[placementdomain.Fact](t, placementType, "store/placement", reserve)
	columns, ok := placementrelation.NewPlacementFreshBirthColumns(valueColumn, placementColumn, candidateColumn)
	if !ok {
		t.Fatal("fresh-birth columns")
	}
	candidateAddress := place.Column(t, "column/fresh-result-candidate")
	factAddress := place.Column(t, "column/fresh-result-fact")
	outputAddress := place.Column(t, "column/placement")
	operation := place.Seal(t, "operation/placement-fresh-birth",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, factAddress, valueType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: placementType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		freshBirthCardinality(t), outcome.Produced, outcome.Refused)
	factory, ok := placementrelation.BindPlacementFreshBirth(operation, placementrelation.PlacementFreshBirthOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement fresh birth")
	}
	worker := place.Worker(t, factory, operation)
	candidateToken, ok := candidateColumn.Encode(place.Issuer, real.candidate)
	if !ok {
		t.Fatal("encode real fresh-result candidate")
	}
	factToken, ok := valueColumn.Encode(place.Issuer, real.fact)
	if !ok {
		t.Fatal("encode real fresh-result fact")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.ScalarSlot(t, place.Cell(t, factAddress, place.Rows[0], valueType, factToken)),
	)
	buffer := place.BufferAt(t, operation, place.Rows[0])
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("fresh birth = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, proposalOK := batch.At(0)
	if !proposalOK || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != outputAddress {
		t.Fatal("fresh birth did not publish at its declared row and column")
	}
	published, decodeOK := placementColumn.Decode(proposal.Value())
	if !decodeOK || !placementdomain.EqualFact(published, placementdomain.DefaultFact()) {
		t.Fatalf("fresh birth fact = %#v, want explicit DefaultFact", published)
	}
}

func TestPlacementFreshBirthRefusesNearestAbsentForeignAndMalformedInputs(t *testing.T) {
	real := sealRealFreshResult(t)
	place := harness.New(t, "row/fresh-birth-negatives")
	valueType := place.TypeID(t, "type/value")
	candidateType := place.TypeID(t, "type/fresh-result-candidate")
	placementType := place.TypeID(t, "type/placement")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	candidateColumn := harness.NewColumn[valuedomain.FreshResultCall](t, candidateType, "store/fresh-result-candidate", reserve)
	placementColumn := harness.NewColumn[placementdomain.Fact](t, placementType, "store/placement", reserve)
	columns, ok := placementrelation.NewPlacementFreshBirthColumns(valueColumn, placementColumn, candidateColumn)
	if !ok {
		t.Fatal("fresh-birth columns")
	}
	candidateAddress := place.Column(t, "column/fresh-result-candidate")
	factAddress := place.Column(t, "column/fresh-result-fact")
	outputAddress := place.Column(t, "column/placement")
	operation := place.Seal(t, "operation/placement-fresh-birth-negatives",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, factAddress, valueType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: placementType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		freshBirthCardinality(t), outcome.Produced, outcome.Refused)
	factory, ok := placementrelation.BindPlacementFreshBirth(operation, placementrelation.PlacementFreshBirthOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement fresh birth")
	}
	worker := place.Worker(t, factory, operation)
	encodeCandidate := func(candidate valuedomain.FreshResultCall) binding.ValueToken {
		t.Helper()
		token, encodeOK := candidateColumn.Encode(place.Issuer, candidate)
		if !encodeOK {
			t.Fatal("encode candidate")
		}
		return token
	}
	encodeFact := func(fact valuedomain.Value) binding.ValueToken {
		t.Helper()
		token, encodeOK := valueColumn.Encode(place.Issuer, fact)
		if !encodeOK {
			t.Fatal("encode Value")
		}
		return token
	}
	presentCandidate := harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, encodeCandidate(real.candidate)))
	presentFact := harness.ScalarSlot(t, place.Cell(t, factAddress, place.Rows[0], valueType, encodeFact(real.fact)))
	checkRefused := func(name string, slots ...binding.Slot) {
		t.Helper()
		buffer := place.BufferAt(t, operation, place.Rows[0])
		result := worker.Evaluate(place.Frame(t, slots...), buffer)
		batch, sealed := buffer.Seal(result)
		if !sealed || result.Code != outcome.Refused || result.RefusalID != place.Refusal || batch.Len() != 0 {
			t.Fatalf("%s = %v sealed=%t rows=%d refusal=%v, want refused/no rows", name, result.Code, sealed, batch.Len(), result.RefusalID)
		}
	}

	// A sparse Value cell is absence, not a stored Value default. The binding
	// must refuse before Placement's birth fold can run.
	checkRefused("absent Value fact", presentCandidate,
		harness.ScalarSlot(t, place.AbsentCell(t, factAddress, place.Rows[0], valueType)))
	// FreshResultCall's zero value carries no owner-issued Heap key.
	checkRefused("zero fresh-result candidate",
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, encodeCandidate(valuedomain.FreshResultCall{}))), presentFact)
	// A token issued by another candidate store is foreign even when its
	// semantic TypeID matches; the owner column must reject it at decode.
	foreignCandidateColumn := harness.NewColumn[valuedomain.FreshResultCall](t, candidateType, "store/fresh-result-candidate-foreign", reserve)
	foreignCandidate, ok := foreignCandidateColumn.Encode(place.Issuer, real.candidate)
	if !ok {
		t.Fatal("encode foreign candidate")
	}
	checkRefused("foreign candidate store",
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, foreignCandidate)), presentFact)
	// A Value token in the wrong owner column is not a typed Value input, even
	// when its Go payload happens to be a Placement fact.
	wrongTypeColumn := harness.NewColumn[placementdomain.Fact](t, valueType, "store/wrong-value-payload", reserve)
	wrongTypeToken, ok := wrongTypeColumn.Encode(place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode wrong Value payload")
	}
	checkRefused("wrong Value payload store",
		presentCandidate,
		harness.ScalarSlot(t, place.Cell(t, factAddress, place.Rows[0], valueType, wrongTypeToken)))

	// Retain the real candidate's owner proof as part of the law's fixture so
	// accidental replacement by a hand-built opaque row is visible in review.
	if !real.values.OwnsFreshResultCall(real.candidate) {
		t.Fatal("the law fixture stopped using Value's issued FreshResultCall")
	}
}
