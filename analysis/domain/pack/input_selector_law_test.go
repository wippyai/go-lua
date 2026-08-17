package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	typeauthority "github.com/wippyai/go-lua/analysis/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/analysis/domain/type/typecontract"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
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

func selectorLawContract(t testing.TB) (*target.Contract, target.Operation) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"send"}}},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: portableAnyTypes(2), Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:    target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil || contract == nil {
		t.Fatalf("seal selector Target: %v", err)
	}
	operation, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"send"}})
	if !ok {
		t.Fatal("selector operation")
	}
	return contract, operation
}

type selectorLawFixture struct {
	schema    *packdomain.Schema
	module    identity.ContentID
	callID    identity.ContentID
	receiver  identity.ContentID
	argument0 identity.ContentID
}

func selectorLawSchema(t testing.TB, contract *target.Contract, label string) selectorLawFixture {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "pack_selector_" + label + ".lua", Text: []byte("local receiver = {}\nreceiver:send(1, 2)\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "pack_selector_" + label, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
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
	artifact, failure := composite.CompileArtifactDetailed(program, receipt)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile selector artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), []*programartifact.Artifact{artifact})
	if err != nil || types == nil {
		t.Fatalf("seal selector types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedArtifact{{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal selector static: %v", err)
	}
	mount, mountOK := packdomain.NewArtifactMount(artifact, module, programID)
	if !mountOK {
		t.Fatal("selector Pack mount")
	}
	schema, ok := packdomain.SealMountedArtifacts(linked, statics, []packdomain.ArtifactMount{mount})
	if !ok || schema == nil {
		t.Fatal("seal selector Pack")
	}
	for index := 0; index < artifact.CallCount(); index++ {
		call, callOK := artifact.CallAt(index)
		if !callOK || call.Form() != flow.CallFormMethod {
			continue
		}
		receiver, receiverOK := call.ReceiverID()
		argumentRow, argumentOK := artifact.CallArgumentFor(index, 0)
		argument := argumentRow.ValueID()
		if !receiverOK || !argumentOK {
			t.Fatal("selector method operands")
		}
		return selectorLawFixture{schema: schema, module: module, callID: call.ID(), receiver: receiver, argument0: argument}
	}
	t.Fatal("selector method call")
	return selectorLawFixture{}
}

func TestInputSelectorsSealTargetABIWithoutRetainingTarget(t *testing.T) {
	contract, operation := selectorLawContract(t)
	first := selectorLawSchema(t, contract, "first")
	second := selectorLawSchema(t, contract, "second")

	receiver, receiverOK := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0})
	argument, argumentOK := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
	tail, tailOK := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: 0})
	opaqueOperation, opaqueOK := contract.Opaque()
	whole, wholeOK := first.schema.InputSelector(opaqueOperation, target.InputSource{Kind: target.InputSourceAllInputs})
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
	if _, ok := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2}); ok {
		t.Fatal("out-of-range fixed formal selected")
	}
	if _, ok := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: 1}); ok {
		t.Fatal("foreign Values variable selected")
	}
	if _, ok := first.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceAllInputs}); ok {
		t.Fatal("ordinary operation selected opaque AllInputs")
	}
	if _, ok := first.schema.InputSelector(opaqueOperation, target.InputSource{Kind: target.InputSourceAllInputs, Ordinal: 1}); ok {
		t.Fatal("malformed opaque selector selected")
	}
	if _, ok := first.schema.InputSelector(0, target.InputSource{Kind: target.InputSourceValueFormal}); ok {
		t.Fatal("zero operation selected")
	}

	resealed, resealedOK := second.schema.InputSelector(operation, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1})
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
