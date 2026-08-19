package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func sourceKeyText(t testing.TB, p *program.Program, key keyspace.Key) string {
	t.Helper()
	value, ok := p.Source().Keys().Exact(key)
	if !ok || value.Kind != keyspace.LiteralString {
		t.Fatalf("Source exact key = %#v/%v", value, ok)
	}
	return value.String
}

func TestFlowSelectorsKeepDirectSelectorCalls(t *testing.T) {
	p := parseBindLower(t, "\nlocal key = \"abs\"\nmath.abs(1)\nmath[key](2)\nmath:abs(3)\n")
	calls := p.Flow().Authored().Calls()
	bindings := p.Flow().AccessGeometry()
	var plain, method keyspace.Term
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		read, form, ok := bindings.DirectCall(call)
		if !ok {
			continue
		}
		root, depth, selected := bindings.ExactRead(read)
		if !selected || root == 0 || depth != 1 {
			t.Fatalf("direct Call[%d] selection = root %v depth %d ok %v", index, root, depth, selected)
		}
		_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(root)
		if !cellOK || sourceKeyText(t, p, key) != "math" {
			t.Fatalf("direct Call[%d] root cell = %v/%v", index, root, cellOK)
		}
		switch form {
		case accessgeometry.CallFormPlain:
			plain = call
		case accessgeometry.CallFormMethod:
			method = call
		default:
			t.Fatalf("direct Call[%d] form = %v", index, form)
		}
	}
	if plain == 0 || method == 0 {
		t.Fatalf("plain/method direct calls = %v/%v, want both", plain, method)
	}
}

func TestModuleImportAndStaticPublicationUseTheirFinalOwners(t *testing.T) {
	p := parseBindLower(t, "\nlocal M = require(\"pkg.core\")\ntype User = M.Schema.User\nM.Schema.User = User\nreturn M\n")
	module := p.Module()
	if module.Count() != 1 {
		t.Fatalf("Module Import count = %d, want one", module.Count())
	}
	imported, ok := module.ImportAt(0)
	if !ok || imported.Call == 0 || imported.Alias == 0 || imported.Request == 0 || imported.Key == 0 {
		t.Fatalf("Module Import = %#v/%v", imported, ok)
	}
	request, _, text, requestOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
	if !requestOK || request != imported.Request || text != "pkg.core" {
		t.Fatalf("Module Import request = %v/%q/%v", request, text, requestOK)
	}
	value, keyOK := p.Source().Keys().Exact(imported.Key)
	if !keyOK || value.Kind != keyspace.LiteralString || value.String != "pkg.core" {
		t.Fatalf("Module Import key = %#v/%v", value, keyOK)
	}
	publication, publicationOK := p.Static().Publications().At(0)
	if !publicationOK {
		t.Fatal("missing Static publication")
	}
	assign, pair, target, publicationRowOK := p.Static().Publications().Get(publication)
	if !publicationRowOK || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("Static publication = assign %v pair %d target %v ok %v", assign, pair, target, publicationRowOK)
	}
	root, owner, depth, bindingOK := p.Flow().AccessGeometry().TypePublication(publication)
	if !bindingOK || root != imported.Alias || owner == 0 || depth != 2 {
		t.Fatalf("Flow publication binding = root %v owner %v depth %d ok %v", root, owner, depth, bindingOK)
	}
	path, pathOK := p.Flow().AccessGeometry().TypePublicationPath(publication)
	if !pathOK {
		t.Fatal("missing Flow publication path cursor")
	}
	first, next, firstOK := path.Segment()
	second, _, secondOK := next.Segment()
	if !firstOK || !secondOK || sourceKeyText(t, p, first) != "User" || sourceKeyText(t, p, second) != "Schema" {
		t.Fatalf("Flow publication path = %v/%v %v/%v", first, firstOK, second, secondOK)
	}
}

func TestModuleEntryKeepsReturnedRootAndMembers(t *testing.T) {
	p := parseBindLower(t, "return { api = { f = function() end }, value = 1 }")
	entry := p.Module().Entry()
	returned, ok := entry.ReturnAt(0)
	if !ok || returned == 0 {
		t.Fatalf("Module Entry return = %v/%v", returned, ok)
	}
	if count, ok := entry.MemberCount(returned); !ok || count != 2 {
		t.Fatalf("Module Entry member count = %d/%v, want 2", count, ok)
	}
	for index := 0; index < 2; index++ {
		member, memberOK := entry.MemberAt(returned, index)
		if !memberOK || member == 0 {
			t.Fatalf("Module Entry member[%d] = %v/%v", index, member, memberOK)
		}
	}
}

func TestFlowExactSelectorDeepPathIsAllocationFree(t *testing.T) {
	const depth = 256
	var input strings.Builder
	input.WriteString("api")
	for index := 0; index < depth; index++ {
		input.WriteString(".x")
		input.WriteString(strconv.Itoa(index))
	}
	input.WriteString("()")
	p := parseBindLower(t, input.String())
	call, ok := p.Flow().Authored().Calls().At(0)
	if !ok {
		t.Fatal("missing Call")
	}
	read, form, bindingOK := p.Flow().AccessGeometry().DirectCall(call)
	if !bindingOK || form != accessgeometry.CallFormPlain {
		t.Fatalf("direct Call binding = read %v form %v ok %v", read, form, bindingOK)
	}
	if _, gotDepth, ok := p.Flow().AccessGeometry().ExactRead(read); !ok || gotDepth != depth {
		t.Fatalf("deep selector depth = %d/%v, want %d", gotDepth, ok, depth)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		path, _ := p.Flow().AccessGeometry().ExactReadPath(read)
		for {
			_, next, ok := path.Segment()
			if !ok {
				break
			}
			path = next
		}
	}); allocations != 0 {
		t.Fatalf("deep selector cursor allocations = %v, want 0", allocations)
	}
}
