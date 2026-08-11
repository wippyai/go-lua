package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestNestedTypeOfCastComposition(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
type Snapshot = typeof(x as typeof(x))
`)
	staticView := p.Static()
	if got := staticView.Operators().TypeOfs().Count(); got != 2 {
		t.Fatalf("TypeOfCount = %d, want outer and cast-target typeof", got)
	}
	alias, ok := staticView.Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Snapshot alias")
	}
	_, target, _, _, ok := staticView.Declarations().Aliases().Get(alias)
	if !ok {
		t.Fatal("missing Snapshot target")
	}
	scope, operand, ok := staticView.Operators().TypeOfs().Get(target)
	if !ok || scope != alias {
		t.Fatalf("outer TypeOf = scope %v operand %v ok %v, want Snapshot host", scope, operand, ok)
	}
	_, _, claimKind, ok := p.Flow().Authored().Claims().Get(operand)
	inner, innerOK := staticView.Operands().Claims().Target(operand)
	if !ok || claimKind != kind.ValueClaimTypeAs || !innerOK || inner == 0 {
		t.Fatalf("outer operand = ValueClaim target %v/%v kind %v ok %v, want as-claim", inner, innerOK, claimKind, ok)
	}
	if !p.Flow().Containment().Static(operand) {
		t.Fatalf("nested ValueClaim %v escaped static classification", operand)
	}
	innerScope, innerOperand, ok := staticView.Operators().TypeOfs().Get(inner)
	if !ok || innerScope != operand || innerOperand == 0 {
		t.Fatalf("nested TypeOf = scope %v operand %v ok %v, want ValueClaim host", innerScope, innerOperand, ok)
	}
}

func TestStaticValueClaimNestingKeepsTargetlessNonNilStructural(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof((false as typeof(false))!)
`)
	flowView := p.Flow()
	claims := flowView.Authored().Claims()
	staticView := p.Static()
	if claims.Count() != 2 || staticView.Operators().TypeOfs().Count() != 2 {
		t.Fatalf("ValueClaims/TypeOfs = %d/%d, want 2/2", claims.Count(), staticView.Operators().TypeOfs().Count())
	}
	typed, _ := claims.At(0)
	nonNil, _ := claims.At(1)
	_, typedOperand, typedKind, typedOK := claims.Get(typed)
	_, nonNilOperand, nonNilKind, nonNilOK := claims.Get(nonNil)
	typedTarget, typedTargetOK := staticView.Operands().Claims().Target(typed)
	nonNilTarget, nonNilTargetOK := staticView.Operands().Claims().Target(nonNil)
	if !typedOK || typedKind != kind.ValueClaimTypeAs || !typedTargetOK || typedTarget == 0 || typedOperand == 0 {
		t.Fatalf("typed ValueClaim = operand %v target %v/%v kind %v ok %v", typedOperand, typedTarget, typedTargetOK, typedKind, typedOK)
	}
	if !nonNilOK || nonNilKind != kind.ValueClaimNonNil || nonNilTargetOK || nonNilTarget != 0 || nonNilOperand != typed {
		t.Fatalf("NonNil ValueClaim = operand %v target %v/%v kind %v ok %v", nonNilOperand, nonNilTarget, nonNilTargetOK, nonNilKind, nonNilOK)
	}
	for _, term := range []keyspace.Term{typed, nonNil, typedOperand} {
		if !flowView.Containment().Static(term) {
			t.Fatalf("static value-claim descendant %v escaped static classification", term)
		}
	}
}

func TestNestedTypeOfInReachableNestedClosureRestoresGlobalEvidence(t *testing.T) {
	p := parseBindLower(t, `
local outer = function()
	return function(x: typeof(x as typeof(x)))
		return external
	end
end
return outer
`)
	flowView := p.Flow()
	if got := flowView.Authored().Functions().Count(); got != 2 {
		t.Fatalf("FunctionCount = %d, want outer and reachable nested closure", got)
	}
	if got := p.Static().Operators().TypeOfs().Count(); got != 2 {
		t.Fatalf("TypeOfCount = %d, want nested function-header composition", got)
	}
	if got := flowView.Authored().Storage().Reads().ImplicitCount(); got != 1 {
		t.Fatalf("implicit reads = %d, want reachable closure body global after typeof closes", got)
	}
	implicit, ok := flowView.Authored().Storage().Reads().ImplicitAt(0)
	if !ok {
		t.Fatal("missing nested closure implicit read")
	}
	_, sourceTerm, _, ok := flowView.Authored().Storage().Reads().Get(implicit)
	if !ok {
		t.Fatal("nested closure implicit evidence is not a Read")
	}
	cellKind, _, key, cellOK := flowView.Authored().Storage().Cells().Get(sourceTerm)
	value, keyOK := p.Source().Keys().Exact(key)
	if !cellOK || cellKind != flow.CellGlobal || !keyOK || value.String != "external" {
		t.Fatalf("nested closure implicit source = cell %v key %#v/%v, want global external", cellKind, value, keyOK)
	}
}

func TestStaticFunctionLabelMetadataStaysOutOfExecutableControl(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof(function()
::again::
goto again
end)
`)
	flowView := p.Flow()
	function, ok := flowView.Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing static Function")
	}
	_, body, _, ok := flowView.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("missing static Function Body")
	}
	label, labelOK := flowView.Authored().Control().Labels().At(0)
	jump, jumpOK := flowView.Authored().Control().Gotos().At(0)
	if !labelOK || !jumpOK {
		t.Fatal("missing static Label/Goto")
	}
	labelOwner, labelOK := flowView.Authored().Control().Labels().Get(label)
	jumpOwner, target, jumpOK := flowView.Authored().Control().Gotos().Get(jump)
	if !labelOK || !jumpOK || labelOwner != body || jumpOwner != body || target != label {
		t.Fatalf(
			"static control = label owner %v, goto owner %v target %v, ok %v/%v",
			labelOwner, jumpOwner, target, labelOK, jumpOK,
		)
	}
	for _, term := range []keyspace.Term{function, body, label, jump} {
		if !flowView.Containment().Static(term) {
			t.Fatalf("static control descendant %v was classified executable", term)
		}
	}
}
