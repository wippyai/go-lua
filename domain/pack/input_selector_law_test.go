package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func portableAnyTypes(count int) []schematype.Type {
	values := make([]schematype.Type, count)
	for index := range values {
		value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
		if !ok {
			panic("portable any type")
		}
		values[index] = value
	}
	return values
}

func selectorLawContract(t testing.TB) (*contract.Contract, vocabulary.Operation) {
	t.Helper()
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: portableAnyTypes(2), Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes:   []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:    vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil || contract == nil {
		t.Fatalf("seal selector Target: %v", err)
	}
	operation, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"send"}})
	if !ok {
		t.Fatal("selector operation")
	}
	return contract, operation
}

type selectorLawFixture struct {
	schema    *packdomain.Schema
	values    *valuedomain.Schema
	module    identity.ContentID
	callID    identity.ContentID
	receiver  identity.ContentID
	argument0 identity.ContentID
}

func selectorLawSchema(t testing.TB, contract *contract.Contract, label string) selectorLawFixture {
	t.Helper()
	return selectorLawSchemaSource(t, contract, label, "local receiver = {}\nreceiver:send(1, 2)\n")
}

// selectorLawSchemaSource seals the same Pack/Value stack over an arbitrary
// method-call source, so a fixture can author an under-applied or tail-fed
// call row instead of the fully applied default.
func selectorLawSchemaSource(t testing.TB, contract *contract.Contract, label, source string) selectorLawFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "pack_selector_" + label + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_selector_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !receiptOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	mounts := linked.Project().Mounts()
	if mounts.Count() != 1 {
		t.Fatalf("selector mounts = %d", mounts.Count())
	}
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("selector mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile selector artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()})
	if err != nil || types == nil {
		t.Fatalf("seal selector types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal selector static: %v", err)
	}
	mount, mountOK := packdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	if !mountOK {
		t.Fatal("selector Pack mount")
	}
	schema, ok := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{mount})
	if !ok || schema == nil {
		t.Fatal("seal selector Pack")
	}
	structural, structuralOK := composite.StructureVocabulary(receipt)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if !structuralOK || !heapMountOK || !valueMountOK || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("selector Value seal structural=%t heapMount=%t valueMount=%t heap=%v value=%v", structuralOK, heapMountOK, valueMountOK, heapFailure, valueFailure)
	}
	coldProgram := artifact.Program()
	catalog, catalogOK := programcatalog.CatalogID(coldProgram.SchemaID)
	if !coldProgram.Available() || !catalogOK || !catalog.Available() {
		t.Fatal("selector cold program")
	}
	callCount, callsOK := coldProgram.CallCount()
	if !callsOK {
		t.Fatal("selector call family")
	}
	for index := 0; index < callCount; index++ {
		call, callOK := coldProgram.CallAt(index)
		if !callOK || call.Form() != programschema.CallFormMethod {
			continue
		}
		receiver, receiverOK := call.ReceiverID()
		if !receiverOK {
			t.Fatal("selector method receiver")
		}
		var argument identity.ContentID
		if argumentRow, argumentOK := coldProgram.CallArgumentFor(index, 0); argumentOK {
			argument = argumentRow.ValueID()
		}
		return selectorLawFixture{schema: schema, values: values, module: module, callID: call.ID(), receiver: receiver, argument0: argument}
	}
	t.Fatal("selector method call")
	return selectorLawFixture{}
}

// TestMountedInputSummaryRefusesIncompleteFixedMembers keeps the Value/Pack
// join fail-closed. A missing fixed summary cell is not a Bottom fact and
// cannot be turned into a readable aggregate; only MountedInput.IsOpen, which
// is issued from Pack's authenticated tail row, may justify widening.
func TestMountedInputSummaryRefusesIncompleteFixedMembers(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchema(t, contract, "summary_missing_member")
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	if !inputOK || !input.Valid() || input.MemberCount() != 1 || input.IsOpen() {
		t.Fatal("fixed mounted input")
	}
	summary := valuedomain.BeginValueSummary(fixture.values)
	if _, present, readable := packtransfer.SummaryValueAtInputMember(fixture.values, summary, input, 0); present || readable {
		t.Fatalf("missing fixed member reported present/readable = %t/%t", present, readable)
	}
	if _, present, readable := packtransfer.SummaryValuesAtInput(fixture.values, summary, input); present || readable {
		t.Fatalf("incomplete fixed aggregate reported present/readable = %t/%t", present, readable)
	}
}

// TestMountedInputSummaryAcceptsCompleteFixedMembers ensures strictness does
// not reject an authenticated, complete Value summary. The Bottom value is a
// real Value fact here; it is not synthesized by the Pack bridge.
func TestMountedInputSummaryAcceptsCompleteFixedMembers(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchema(t, contract, "summary_complete_member")
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	if !inputOK || !input.Valid() {
		t.Fatal("fixed mounted input")
	}
	coordinate, coordinateOK := packtransfer.CoordinateForInputMember(fixture.values, input, 0)
	if !coordinateOK {
		t.Fatal("fixed member coordinate")
	}
	index, indexOK := fixture.values.CoordinateIndex(coordinate)
	if !indexOK || int(index) >= len(valuedomain.BeginValueSummary(fixture.values).Values) {
		t.Fatal("fixed member coordinate index")
	}
	summary := valuedomain.BeginValueSummary(fixture.values)
	summary.Values[index] = fixture.values.Bottom()
	summary.Present[index] = true
	summary.Rows = 1
	if _, present, readable := packtransfer.SummaryValueAtInputMember(fixture.values, summary, input, 0); !present || !readable {
		t.Fatalf("complete fixed member reported present/readable = %t/%t", present, readable)
	}
	if fact, present, readable := packtransfer.SummaryValuesAtInput(fixture.values, summary, input); !present || !readable || !fixture.values.Equal(fact, fixture.values.Bottom()) {
		t.Fatalf("complete fixed aggregate = %#v/%t/%t", fact, present, readable)
	}
}

func TestInputSelectorsSealTargetABIWithoutRetainingTarget(t *testing.T) {
	contract, operation := selectorLawContract(t)
	first := selectorLawSchema(t, contract, "first")
	second := selectorLawSchema(t, contract, "second")

	receiver, receiverOK := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	argument, argumentOK := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	tail, tailOK := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0})
	opaqueOperation, opaqueOK := contract.Operations.Opaque()
	whole, wholeOK := first.schema.InputSelector(opaqueOperation, vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs})
	if !receiverOK || !argumentOK || !tailOK || !opaqueOK || !wholeOK {
		t.Fatal("sealed selector inventory")
	}
	// The fixture is a method call, whose Pack call row orders receiver before
	// ordinary arguments. The complete Target ABI selectors—including its tail
	// and opaque all-inputs projection—must be issued from that seal.
	if !first.schema.OwnsInputSelector(receiver) || !first.schema.OwnsInputSelector(argument) || !first.schema.OwnsInputSelector(tail) || !first.schema.OwnsInputSelector(whole) {
		t.Fatal("Target ABI selector ownership")
	}
	receiverSource, receiverSourceOK := first.schema.MountedInputSemanticSource(first.module, first.callID, receiver)
	argumentSource, argumentSourceOK := first.schema.MountedInputSemanticSource(first.module, first.callID, argument)
	if !receiverSourceOK || !argumentSourceOK || receiverSource.ID() != first.receiver || argumentSource.ID() != first.argument0 {
		t.Fatal("method receiver-first fixed input projection")
	}
	if _, ok := first.schema.MountedInputSemanticSource(first.module, first.callID, tail); ok {
		t.Fatal("tail selector fabricated structural semantic source")
	}
	if _, ok := first.schema.MountedInputSemanticSource(first.module, first.callID, whole); ok {
		t.Fatal("whole selector fabricated structural semantic source")
	}
	if _, ok := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 2}); ok {
		t.Fatal("out-of-range fixed formal selected")
	}
	if _, ok := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 1}); ok {
		t.Fatal("foreign Values variable selected")
	}
	if _, ok := first.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}); ok {
		t.Fatal("ordinary operation selected opaque AllInputs")
	}
	if _, ok := first.schema.InputSelector(opaqueOperation, vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs, Ordinal: 1}); ok {
		t.Fatal("malformed opaque selector selected")
	}
	if _, ok := first.schema.InputSelector(0, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}); ok {
		t.Fatal("zero operation selected")
	}

	resealed, resealedOK := second.schema.InputSelector(operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !resealedOK || !second.schema.OwnsInputSelector(resealed) || first.schema.OwnsInputSelector(resealed) {
		t.Fatal("selector reseal crossed Pack owner fence")
	}
	if second.schema.OwnsInputSelector(argument) {
		t.Fatal("foreign selector crossed Pack owner fence")
	}
	if _, ok := first.schema.MountedInputSemanticSource(first.module, first.callID, resealed); ok {
		t.Fatal("foreign schema selector crossed Pack call fence")
	}
	if _, ok := first.schema.MountedInputSemanticSource(second.module, first.callID, receiver); ok {
		t.Fatal("foreign mounted module crossed Pack call fence")
	}
	if first.callID == second.callID {
		t.Fatal("fixture did not issue distinct mounted call identities")
	}
	if _, ok := first.schema.MountedInputSemanticSource(first.module, second.callID, receiver); ok {
		t.Fatal("foreign mounted call crossed Pack call fence")
	}
}

// TestMountedInputProjectsCallSpecificTailMembers proves that a ValuesVar is
// the exact fixed suffix of this mounted call, not an intrinsically open or
// fabricated tail source. The fixture has three fixed actuals and no actual
// tail, so its target tail projects only the final authored member and remains
// closed.
func TestMountedInputProjectsCallSpecificTailMembers(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchema(t, contract, "mounted_input_projection")
	actual, actualOK := fixture.schema.MountedActualProjection(fixture.module, fixture.callID)
	if !actualOK || actual.ActualCount() != 3 {
		t.Fatal("mounted actual projection")
	}
	tail, tailOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0})
	if !tailOK || !tail.Valid() || tail.IsOpen() || tail.MemberCount() != 1 {
		t.Fatalf("call-specific ValuesVar projection = valid=%t open=%t members=%d", tail.Valid(), tail.IsOpen(), tail.MemberCount())
	}
	expected, expectedOK := actual.ActualAt(2)
	member, memberOK := tail.MemberAt(0)
	if !expectedOK || !memberOK || member != expected.ID() {
		t.Fatal("ValuesVar did not retain the exact ordered fixed suffix member")
	}
	fixed, fixedOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0})
	if !fixedOK || fixed.MemberCount() != 1 || fixed.IsOpen() {
		t.Fatal("ValueFormal did not project one closed member")
	}
	if fixedMember, ok := fixed.MemberAt(0); !ok || fixedMember != fixture.receiver {
		t.Fatal("ValueFormal did not preserve receiver-first call order")
	}
}
