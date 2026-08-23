package composite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestActivationAdmissionIsRoutedByTheDeclaredLane states which inventory a
// sealed placement lands in: the one its capability's declared lane names. An
// activation row is admitted because its rule declared the activation lane,
// and an ordinary mounted row is admitted because its rule did not. Nothing
// about a rule's spelling participates.
func TestActivationAdmissionIsRoutedByTheDeclaredLane(t *testing.T) {
	record := mountedRecord(t, "activation-lane-routing", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	mounted, activations, failed := rules.MountedAdmissions(record.Artifacts, record.Source.ContextDirectory())
	if failed.Available() {
		t.Fatalf("mounted admissions refused: %s", failed)
	}
	if len(activations) == 0 {
		t.Fatal("the fixture placed no activation trigger")
	}
	for index, row := range activations {
		if !row.Capability.Activation() {
			t.Fatalf("activation admission %d was routed without the activation lane", index)
		}
	}
	for index, row := range mounted {
		if row.Capability.Activation() || !row.Capability.Mounted() {
			t.Fatalf("mounted admission %d carries the wrong lane", index)
		}
	}
	// The lane is a property of the declaration, so the inventory census must
	// agree with what the catalog declares for each admitted key.
	declared := 0
	for _, row := range activations {
		key := capabilityKey(t, bound.Compilation(), rules, row.Capability)
		template, templateOK := templateForKey(rules.catalog, key)
		if !templateOK || !template.Lane().Mounted() {
			t.Fatalf("activation admission names key %q, which the catalog does not declare on a mounted lane", key)
		}
		declared++
	}
	if declared != len(activations) {
		t.Fatalf("activation admissions=%d, catalog-declared=%d", len(activations), declared)
	}
}

// TestAdmissionNamesNoRuleKey is the no-magic-names half of the same law. A
// rule key literal in the admission walk is one rule's spelling deciding a
// lane for every composition: renaming the rule, or adding a second activation
// rule, silently changes which inventory a placement reaches. The lane belongs
// to the declaration, so this file may not spell a rule key at all.
func TestAdmissionNamesNoRuleKey(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "rule_admission.go", nil, 0)
	if err != nil {
		t.Fatalf("parse the admission walk: %v", err)
	}
	record := mountedRecord(t, "activation-lane-naming", "return 1")
	bound := materializerBinding(t, record)
	compilation := bound.Compilation()
	keys := make(map[schema.Key]struct{}, RuleCount(compilation))
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK || !key.Available() {
			t.Fatalf("rule key at position %d", position)
		}
		keys[key] = struct{}{}
	}
	if len(keys) == 0 {
		t.Fatal("the sealed rule inventory names no rule")
	}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, isLiteral := node.(*ast.BasicLit)
		if !isLiteral || literal.Kind != token.STRING {
			return true
		}
		spelled := schema.Key(strings.Trim(literal.Value, `"`))
		if _, named := keys[spelled]; named {
			t.Fatalf("the admission walk names rule key %q: a rule's spelling decides no lane", spelled)
		}
		return true
	})
}
