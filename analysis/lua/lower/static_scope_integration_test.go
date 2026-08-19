package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
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
	if !cellOK || cellKind != authored.CellGlobal || !keyOK || value.String != "external" {
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

func assertStaticDeclarationRef(t *testing.T, p *program.Program, ref, want keyspace.Term) {
	t.Helper()
	resolution, target, root, ok := p.Static().References().Get(ref)
	if !ok || resolution != staticrefs.Declaration || target != want || root != 0 {
		t.Fatalf("Static Reference(%v) = resolution %v target %v root %v ok %v; want declaration %v", ref, resolution, target, root, ok, want)
	}
}

func TestStaticAliasesResolveNearestVisibleBareName(t *testing.T) {
	p := parseBindLower(t, "type T = number\ndo\n  type T = string\n  type Inner = T\nend\ntype Outer = T")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	outerT := controlSourceAt(t, p, entry, 0)
	block := controlSourceAt(t, p, entry, 1)
	outerUse := controlSourceAt(t, p, entry, 2)
	innerT := controlSourceAt(t, p, block, 0)
	innerUse := controlSourceAt(t, p, block, 1)
	aliases := p.Static().Declarations().Aliases()
	for _, row := range []struct {
		alias keyspace.Term
		want  keyspace.Term
	}{
		{innerUse, innerT}, {outerUse, outerT},
	} {
		_, ref, _, _, ok := aliases.Get(row.alias)
		if !ok {
			t.Fatalf("Static Alias(%v) is absent", row.alias)
		}
		assertStaticDeclarationRef(t, p, ref, row.want)
	}
}

func TestStaticTypeParametersKeepTheirOwnDeclarationRows(t *testing.T) {
	p := parseBindLower(t, "type Pair<T: U, U: T> = T")
	alias, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Static Alias")
	}
	params := p.Static().Declarations().TypeParams()
	first, firstOK := p.Static().Declarations().Aliases().ParamAt(alias, 0)
	second, secondOK := p.Static().Declarations().Aliases().ParamAt(alias, 1)
	if !firstOK || !secondOK {
		t.Fatalf("Alias parameters = %v/%v %v/%v", first, firstOK, second, secondOK)
	}
	_, _, firstConstraint, firstRowOK := params.Get(first)
	_, _, secondConstraint, secondRowOK := params.Get(second)
	if !firstRowOK || !secondRowOK {
		t.Fatal("missing Static TypeParam rows")
	}
	assertStaticDeclarationRef(t, p, firstConstraint, second)
	assertStaticDeclarationRef(t, p, secondConstraint, first)
}

func TestStaticNestedFunctionsRemainInStaticFlowContainment(t *testing.T) {
	p := parseBindLower(t, "type Box<T: typeof(function(value: T): T\n  return value\nend)> = T")
	function, ok := p.Flow().Authored().Functions().At(0)
	if !ok || !p.Flow().Containment().Static(function) {
		t.Fatalf("nested Function = %v/%v static=%v", function, ok, p.Flow().Containment().Static(function))
	}
	if count, ok := p.Source().Formals().Len(function); !ok || count != 1 {
		t.Fatalf("nested Function formal count = %d/%v, want one", count, ok)
	}
	formal, _ := p.Source().Formals().At(function, 0)
	cellKind, _, _, cellOK := p.Flow().Authored().Storage().Cells().Get(formal)
	if !cellOK || cellKind != authored.CellLocal {
		t.Fatalf("nested static Function formal cell = kind %v ok %v", cellKind, cellOK)
	}
}

func TestStaticPublicationKeepsStaticTargetAndFlowAssignSeparate(t *testing.T) {
	p := parseBindLower(t, "type T = number\nlocal M = {}\nM.Schema.T = T")
	publications := p.Static().Publications()
	publication, ok := publications.At(0)
	if !ok || publications.Count() != 1 {
		t.Fatalf("Static publication = %v/%v count=%d", publication, ok, publications.Count())
	}
	assign, pair, target, rowOK := publications.Get(publication)
	if !rowOK || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("Static publication row = assign %v pair %d target %v ok %v", assign, pair, target, rowOK)
	}
	if _, _, assignOK := p.Flow().Authored().Storage().Assigns().Get(assign); !assignOK {
		t.Fatal("publication Assign is absent from Flow")
	}
	if _, _, _, refOK := p.Static().References().Get(target); !refOK {
		t.Fatal("publication target is absent from Static References")
	}
}
