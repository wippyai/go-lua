package compiler

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// countingSemantics is the Lua adapter with one census over the declaration
// admission it is asked for. Admission decodes the neutral declaration, so the
// count is also the count of canonical graph materializations Seal pays.
type countingSemantics struct {
	domaincontract.Semantics
	validations *int
}

func (s countingSemantics) Validate(value schematype.Type, formals []schematype.Type) error {
	*s.validations++
	return s.Semantics.Validate(value, formals)
}

// TestOperationAdmitsEveryDeclarationOnce pins the admission denominator: one
// operation's declaration table is the authority for which declarations that
// operation admits, so a declaration mentioned at many positions is admitted
// once, not once per mention.
func TestOperationAdmitsEveryDeclarationOnce(t *testing.T) {
	validations := 0
	semantics := countingSemantics{Semantics: domaincontract.NewSemantics(), validations: &validations}

	// One operation whose input and every outcome mention the same two
	// declarations: eight positions over two distinct declarations.
	values := vocabulary.ValuesSpec{Fixed: []schematype.Type{testNumber, testString, testNumber, testString}, Tail: vocabulary.ValuesClosed}
	spec := declaration.Spec{
		Semantics: semantics,
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"census"}}},
			Input:    values,
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	}
	if _, err := Seal(&spec); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if validations != 2 {
		t.Fatalf("Seal admitted %d declarations over 8 mentions of 2 distinct declarations, want 2", validations)
	}
}
