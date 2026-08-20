package engine

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestRuleImplementationHasOnlyCanonicalRowAddress(t *testing.T) {
	typ := reflect.TypeOf(RuleImplementation[uint64, uint64, struct{}]{})
	want := []string{"cell", "ordinal", "output"}
	if typ.NumField() != len(want) {
		t.Fatalf("RuleImplementation fields = %d, want %d", typ.NumField(), len(want))
	}
	for index, name := range want {
		if typ.Field(index).Name != name {
			t.Fatalf("RuleImplementation field[%d] = %q, want %q", index, typ.Field(index).Name, name)
		}
	}
	if typ.Field(2).Type != reflect.TypeOf((*schemaFactorBinding)(nil)).Elem() {
		t.Fatalf("RuleImplementation output type = %v, want schemaFactorBinding", typ.Field(2).Type)
	}
	if _, present := reflect.TypeOf(boundRuleMember[uint64, struct{}]{}).FieldByName("proof"); present {
		t.Fatal("boundRuleMember retains construction proof")
	}
	rowType := reflect.TypeOf(schemaRuleReadRow{})
	for _, forbidden := range []string{"state", "schema", "slot", "proof", "origin"} {
		if _, present := rowType.FieldByName(forbidden); present {
			t.Fatalf("schemaRuleReadRow retains %s", forbidden)
		}
	}
	for typ, label := range map[reflect.Type]string{
		reflect.TypeOf(RuleReadSurface{}):    "RuleReadSurface",
		reflect.TypeOf(ruleSummaryMapping{}): "ruleSummaryMapping",
		reflect.TypeOf(ruleWriteSurface{}):   "ruleWriteSurface",
	} {
		for _, forbidden := range []string{"proof", "receipt", "read", "write", "origin", "plan", "cache"} {
			if _, present := typ.FieldByName(forbidden); present {
				t.Fatalf("%s retains forbidden %s field", label, forbidden)
			}
		}
	}
}

func TestLegacyRuleWrapperAndRuntimeProofAssertionsHaveZeroShape(t *testing.T) {
	for _, name := range []string{"schema_seal_tokens.go", "runtime_rule.go", "schema_rule_read_binding.go", "schema_factor_binding.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.Contains(source, []byte("ruleRuntime"+"Binding")) {
			t.Fatalf("%s retains the deleted Rule runtime wrapper", name)
		}
		if bytes.Contains(source, []byte("runtimeRule"+"Proof()")) {
			t.Fatalf("%s retains the deleted runtime proof assertion seam", name)
		}
	}
}
