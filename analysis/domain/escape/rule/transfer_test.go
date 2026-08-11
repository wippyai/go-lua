package rule

import (
	"testing"

	escapedomain "github.com/wippyai/go-lua/analysis/domain/escape"
	escapeowner "github.com/wippyai/go-lua/analysis/domain/escape/owner"
	"github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestTransferOperandUsesTargetCanonicalOutcomeIdentity(t *testing.T) {
	source, contract, operation, transfer := transferOperandSource(t)
	operand, operandOK := NewTransferOperand(source, transfer, 0)
	if !operandOK {
		t.Fatal("target-static transfer operand")
	}
	want, disposition, wantOK := contract.TransferOutcomeContentID(operation, transfer, 0)
	if !wantOK || disposition == 0 || operand.ContentID() != want {
		t.Fatalf("operand ContentID = %x, want %x/%v", operand.ContentID(), want, wantOK)
	}
	if _, accepted := NewTransferOperand(source, transfer, 1); accepted {
		t.Fatal("undeclared transfer outcome entered operand family")
	}
}

// TestTransferOperandRejectsMissingOrForeignPackSelector proves the Input
// selector is authenticated into the private prototype operand once, then
// consumed without a mutable Rule lookup on the solver path. A zero selector
// and an otherwise-valid selector from a same-content foreign Pack schema are
// both rejected before admission.
func TestTransferOperandRejectsMissingOrForeignPackSelector(t *testing.T) {
	source, _, _, transfer := transferOperandSource(t)
	rule, packs := transferRuleFixture(t, source)
	operand, operandOK := NewTransferOperand(source, transfer, 0)
	if !operandOK {
		t.Fatal("transfer operand")
	}
	if _, _, accepted := rule.operandContent(operand); accepted {
		t.Fatal("missing Pack selector accepted")
	}
	bound, boundOK := rule.bindOperand(operand)
	if !boundOK {
		t.Fatal("bind canonical Pack selector")
	}
	if _, _, accepted := rule.operandContent(bound); !accepted {
		t.Fatal("canonical Pack selector rejected")
	}

	foreign, _, _, foreignTransfer := transferOperandSource(t)
	foreignPacks := transferPackSchema(t, foreign)
	contract, contractOK := foreign.Boundary().Target()
	if !contractOK || contract == nil {
		t.Fatal("foreign transfer contract")
	}
	operation, ownerOK := contract.TransferOwner(foreignTransfer)
	_, payload, _, _, _, declarationOK := contract.TransferDeclaration(foreignTransfer)
	foreignSelector, selectorOK := foreignPacks.InputSelector(operation, payload)
	if !ownerOK || !declarationOK || !selectorOK || foreignPacks == packs {
		t.Fatal("foreign Pack selector fixture")
	}
	forged := bound
	forged.selector = foreignSelector
	if _, _, accepted := rule.operandContent(forged); accepted {
		t.Fatal("foreign Pack selector accepted")
	}
}

func transferRuleFixture(t testing.TB, source *link.Link) (*Rule, *packdomain.Schema) {
	t.Helper()
	packs := transferPackSchema(t, source)
	heaps, heapsOK := heap.Seal(source)
	values, valuesOK := value.Seal(source, heaps)
	escapes, escapesOK := escapedomain.NewSchema(source, heaps)
	if !heapsOK || !valuesOK || !escapesOK {
		t.Fatal("transfer rule schemas")
	}
	composition := engine.NewComposition()
	packOwner, packOwnerOK := packowner.Declare(composition, transferRuleKey(1), packs)
	valueOwner, valueOwnerOK := valueowner.Declare(composition, transferRuleKey(2), transferRuleKey(3), values)
	escapeOwner, escapeOwnerOK := escapeowner.Declare(composition, transferRuleKey(4), escapes)
	rule, ruleOK := Declare(composition, transferRuleKey(5), transferRuleKey(6), transferRuleKey(7), escapeOwner, packOwner, valueOwner)
	if !packOwnerOK || !valueOwnerOK || !escapeOwnerOK || !ruleOK || rule == nil {
		t.Fatal("transfer rule owners")
	}
	return rule, packs
}

func transferPackSchema(t testing.TB, source *link.Link) *packdomain.Schema {
	t.Helper()
	types, typesOK := typeauthority.Seal(source)
	statics, _, staticsErr := staticdomain.Seal(source, types)
	packs, packsOK := packdomain.Seal(source, statics)
	if !typesOK || staticsErr != nil || !packsOK || packs == nil {
		t.Fatal("transfer Pack schema")
	}
	return packs
}

func transferRuleKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	digest[24] = 0x65
	digest[25] = 0x73
	digest[26] = 0x63
	digest[27] = 0x72
	digest[28] = byte(value >> 24)
	digest[29] = byte(value >> 16)
	digest[30] = byte(value >> 8)
	digest[31] = byte(value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("transfer rule key")
	}
	return key
}

func transferOperandSource(t testing.TB) (*link.Link, *target.Contract, target.Operation, target.TransferID) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"escape_operand_transfer"}}},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Transfers: []target.TransferSpec{{
			Endpoint:     target.TransferEndpoint{Kind: target.TransferEndpointExternal},
			Payload:      target.InputSource{Kind: target.InputSourceValueFormal},
			Alias:        target.InputSource{Kind: target.InputSourceValueFormal},
			Identity:     target.TransferIdentityUnspecified,
			Capabilities: target.TransferCapabilitiesUnspecified,
			Outcomes:     []target.TransferOutcomeSpec{{Outcome: 0, Possibility: target.TransferMayDeliver}},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := lower.Lower(lower.Source{Name: "escape_operand_transfer.lua", Text: []byte("return nil")})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "escape_operand_transfer", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	operation, operationOK := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"escape_operand_transfer"}})
	if !operationOK {
		t.Fatal("transfer operation")
	}
	transfer, transferOK := contract.TransferIDAt(operation, 0)
	if !transferOK {
		t.Fatal("transfer identity")
	}
	return source, contract, operation, transfer
}
