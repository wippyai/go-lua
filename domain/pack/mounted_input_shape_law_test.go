package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
)

// TestMountedInputShapeIsTotalOverItsFourReadings pins the one accessor a
// consumer switches on. The four readings partition every valid row, so no rule
// re-derives the trichotomy from MemberCount, IsOpen and IsProvenNil and gets
// one combination wrong; an unissued row is the only Invalid reading.
func TestMountedInputShapeIsTotalOverItsFourReadings(t *testing.T) {
	contract, operation := selectorLawContract(t)

	provenNil := selectorLawSchemaSource(t, contract, "shape_proven_nil", "local receiver = {}\nreceiver:send()\n")
	tailFed := selectorLawSchemaSource(t, contract, "shape_open", "local receiver = {}\nlocal function outer(...)\n  receiver:send(...)\nend\nouter(1)\n")
	applied := selectorLawSchemaSource(t, contract, "shape_members", "local receiver = {}\nreceiver:send(1, 2)\n")
	closedEmpty := selectorLawSchemaSource(t, contract, "shape_empty", "local receiver = {}\nreceiver:send(1)\n")

	formal := vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}
	variable := vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0}

	for _, item := range []struct {
		name   string
		schema *selectorLawFixture
		source vocabulary.InputSource
		want   packtransfer.MountedShape
	}{
		{"proven nil", &provenNil, formal, packtransfer.MountedShapeProvenNil},
		{"open tail", &tailFed, formal, packtransfer.MountedShapeOpen},
		{"members", &applied, formal, packtransfer.MountedShapeMembers},
		{"closed empty", &closedEmpty, variable, packtransfer.MountedShapeEmpty},
	} {
		input, inputOK := packtransfer.NewMountedInput(item.schema.schema, item.schema.module, item.schema.callID, operation, item.source)
		if !inputOK || !input.Valid() {
			t.Fatalf("%s: mounted input refused", item.name)
		}
		if shape := input.Shape(); shape != item.want {
			t.Fatalf("%s: shape = %d members=%d open=%t provenNil=%t, want %d", item.name, shape, input.MemberCount(), input.IsOpen(), input.IsProvenNil(), item.want)
		}
		if input.Shape() == packtransfer.MountedShapeInvalid {
			t.Fatalf("%s: a valid row read as invalid", item.name)
		}
	}

	if (packtransfer.MountedInput{}).Shape() != packtransfer.MountedShapeInvalid {
		t.Fatal("an unissued row did not read as invalid")
	}
}

// TestMountedInputShapeAgreesWithTheReadingsItReplaces keeps the enum honest
// against the three predicates the rules used to combine by hand.
func TestMountedInputShapeAgreesWithTheReadingsItReplaces(t *testing.T) {
	contract, operation := selectorLawContract(t)
	for _, source := range []string{
		"local receiver = {}\nreceiver:send()\n",
		"local receiver = {}\nreceiver:send(1)\n",
		"local receiver = {}\nreceiver:send(1, 2)\n",
		"local receiver = {}\nlocal function outer(...)\n  receiver:send(...)\nend\nouter(1)\n",
	} {
		fixture := selectorLawSchemaSource(t, contract, "shape_agreement", source)
		for _, selected := range []vocabulary.InputSource{
			{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
			{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1},
			{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0},
		} {
			input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, selected)
			if !inputOK {
				continue
			}
			switch input.Shape() {
			case packtransfer.MountedShapeMembers:
				if input.MemberCount() == 0 || input.IsProvenNil() {
					t.Fatalf("Members shape with %d members provenNil=%t", input.MemberCount(), input.IsProvenNil())
				}
			case packtransfer.MountedShapeOpen:
				if input.MemberCount() != 0 || !input.IsOpen() || input.IsProvenNil() {
					t.Fatalf("Open shape members=%d open=%t provenNil=%t", input.MemberCount(), input.IsOpen(), input.IsProvenNil())
				}
			case packtransfer.MountedShapeProvenNil:
				if input.MemberCount() != 0 || input.IsOpen() || !input.IsProvenNil() {
					t.Fatalf("ProvenNil shape members=%d open=%t provenNil=%t", input.MemberCount(), input.IsOpen(), input.IsProvenNil())
				}
			case packtransfer.MountedShapeEmpty:
				if input.MemberCount() != 0 || input.IsOpen() || input.IsProvenNil() {
					t.Fatalf("Empty shape members=%d open=%t provenNil=%t", input.MemberCount(), input.IsOpen(), input.IsProvenNil())
				}
			default:
				t.Fatalf("a valid mounted row read as shape %d", input.Shape())
			}
		}
	}
}
