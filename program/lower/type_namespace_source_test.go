package lower_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/static"
)

func assertStaticDeclarationRef(t *testing.T, p *program.Program, ref, want keyspace.Term) {
	t.Helper()
	resolution, target, root, ok := p.Static().References().Get(ref)
	if !ok || resolution != static.TypeRefDeclaration || target != want || root != 0 {
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
	if !cellOK || cellKind != flow.CellLocal {
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

func TestStaticNamespaceScaleIsDeterministic(t *testing.T) {
	const declarations = 256
	var input strings.Builder
	for index := 0; index < declarations; index++ {
		fmt.Fprintf(&input, "type Shared = number -- %d\n", index)
	}
	for index := 0; index < declarations; index++ {
		fmt.Fprintf(&input, "type Use%d = Shared\n", index)
	}
	first := parseBindLower(t, input.String())
	second := parseBindLower(t, input.String())
	firstAliases := first.Static().Declarations().Aliases().Count()
	secondAliases := second.Static().Declarations().Aliases().Count()
	if firstAliases != declarations*2 || secondAliases != firstAliases {
		t.Fatalf("Static Alias counts = %d/%d, want %d", firstAliases, secondAliases, declarations*2)
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("replayed static namespace source changed Program ContentID")
	}
}
