package value

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// moduleLoadRequireContract seals a contract whose single operation is the
// shape of a scoped loader. withResult selects whether its normal outcome
// actually names a bounded result: a contract without one declares the
// operation but cannot say what a call to it produces.
func moduleLoadRequireContract(t testing.TB, withResult bool) (*contract.Contract, vocabulary.Operation) {
	t.Helper()
	anyType, anyOK := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !anyOK {
		t.Fatal("any type declaration")
	}
	outcome := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}
	if withResult {
		outcome.Values = vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed}
	}
	sealed, err := compiler.Seal(&declaration.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"module-load-require"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{outcome},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	})
	if err != nil || sealed == nil {
		t.Fatalf("seal module-load contract: %v", err)
	}
	operation, operationOK := sealed.Operations.OperationAt(0)
	if !operationOK {
		t.Fatal("module-load operation handle")
	}
	return sealed, operation
}

// TestModuleLoadRefusesADeclaredRequireWithoutAResult is the soundness law for
// the module-load vertical.
//
// A Link that declares no scoped loader has no module-load vertical, and
// sealing correctly produces no rows for it. A Link that declares one whose
// contract cannot name a bounded normal result is a different situation:
// dropping the vertical there does not leave those calls unanalysed, it hands
// every require call to the generic call-result path, which holds no module
// evidence and widens to Top. The seal must refuse by name instead, so a
// malformed contract can never be read as "every value is a possible module
// root".
func TestModuleLoadRefusesADeclaredRequireWithoutAResult(t *testing.T) {
	wellFormed, wellFormedOperation := moduleLoadRequireContract(t, true)
	if !requireOutcomeResultAvailable(wellFormed, wellFormedOperation) {
		t.Fatal("a require operation with a bounded normal result was refused")
	}

	malformed, malformedOperation := moduleLoadRequireContract(t, false)
	if requireOutcomeResultAvailable(malformed, malformedOperation) {
		t.Fatal("a declared require operation with no bounded normal result was admitted")
	}

	// The refusal is by the required operation's own declaration, not by the
	// absence of a contract or of an operation handle.
	if requireOutcomeResultAvailable(nil, wellFormedOperation) {
		t.Fatal("a missing contract was admitted as a required operation")
	}
	if requireOutcomeResultAvailable(wellFormed, 0) {
		t.Fatal("an unnamed operation was admitted as a required operation")
	}
}

// TestModuleLoadClassifyRefusesAnOperandWithoutARequireOperation pins the hot
// half of the same law: no classification path turns an operand that carries
// no required operation into a staged Top. The rule refuses; it does not
// widen.
func TestModuleLoadClassifyRefusesAnOperandWithoutARequireOperation(t *testing.T) {
	// Every admitted ModuleLoadCall carries its required operation, because
	// the row is invalid without one. That is what makes the refusal total
	// rather than a hole a caller could route around.
	if _, ok := (ModuleLoadCall{}).RequireOperation(); ok {
		t.Fatal("an unsealed module-load operand reported a required operation")
	}
	if (ModuleLoadCall{}).valid() {
		t.Fatal("an unsealed module-load operand was valid")
	}
}
