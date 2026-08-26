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
// sealed placement lands in: the one its rule's declaration names. A committed
// activation trigger exists because its rule declared an activation, and an
// ordinary mounted placement commits none because its rule declared none.
// Nothing about a rule's spelling participates.
func TestActivationAdmissionIsRoutedByTheDeclaredLane(t *testing.T) {
	record := mountedRecord(t, "activation-lane-routing", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	compilation := bound.Compilation()
	declaring := activationDeclaringRuleKeys(t, compilation)
	committed, _ := queryCanonicalProgram(t, record, bound)
	if committed.ActivationCount() == 0 {
		t.Fatal("the fixture committed no activation trigger")
	}
	// The routing law is a count agreement in both directions: every placement
	// of an activation-declaring rule commits one trigger, and no other
	// placement commits one. A trigger routed off the declared lane would
	// break one side or the other.
	placed := activationPlacementCount(t, record, declaring)
	if placed == 0 {
		t.Fatal("the fixture placed no occurrence for an activation-declaring rule")
	}
	if committed.ActivationCount() != placed {
		t.Fatalf("activation-declaring placements=%d, committed triggers=%d", placed, committed.ActivationCount())
	}
	// A trigger addresses the coordinates its placement was made at, and
	// states the application it activates for, whether or not it reaches a
	// body. Both are what routing on the declared lane delivers.
	for index := 0; index < committed.ActivationCount(); index++ {
		activation, activationOK := committed.ActivationAt(index)
		if !activationOK {
			t.Fatalf("committed activation %d is not enumerable", index)
		}
		application, applicationOK := activation.Application()
		_, memberOK := activation.Member()
		if !activation.Mount().Available() || !activation.Point().Available() || !activation.Occurrence().Available() ||
			!applicationOK || !application.Available() || !memberOK {
			t.Fatalf("activation %d was routed without its mounted coordinates, application, or graph member: mount=%t point=%t occurrence=%t application=%t member=%t",
				index, activation.Mount().Available(), activation.Point().Available(), activation.Occurrence().Available(),
				applicationOK && application.Available(), memberOK)
		}
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
