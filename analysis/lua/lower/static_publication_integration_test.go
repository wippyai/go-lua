package lower_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/static"
)

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

func TestStaticTypePublicationIsAdditiveToRuntimeAssignment(t *testing.T) {
	p := parseBindLower(t, `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	publications := p.Static().Publications()
	if got := publications.Count(); got != 1 {
		t.Fatalf("TypePublicationCount = %d, want 1", got)
	}
	publication, ok := publications.At(0)
	if !ok {
		t.Fatal("missing TypePublication")
	}
	assign, pair, target, ok := publications.Get(publication)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf(
			"TypePublication = assign %v pair %d target %v ok %v",
			assign, pair, target, ok,
		)
	}
	if state, declaration, _, ok := p.Static().References().Get(target); !ok ||
		state != static.TypeRefDeclaration || declaration == 0 {
		t.Fatalf("publication TypeRef = state %v declaration %v ok %v", state, declaration, ok)
	}
	assigns := p.Flow().Authored().Storage().Assigns()
	if assigns.Count() != 1 {
		t.Fatalf("AssignCount = %d, want executable assignment retained", assigns.Count())
	}
	if _, _, ok := assigns.Get(assign); !ok {
		t.Fatalf("publication Assign %v is missing", assign)
	}
	write, writeOK := assigns.WriteAt(assign, int(pair))
	_, targetTerm, targetOK := p.Flow().Authored().Storage().Writes().Get(write)
	if !writeOK || !targetOK || targetTerm == 0 {
		t.Fatalf("publication Assign pair %d = Write %v/%v target %v/%v", pair, write, writeOK, targetTerm, targetOK)
	}
	if reads := p.Flow().Authored().Storage().Reads().Count(); reads != 4 {
		t.Fatalf("ReadCount = %d, want ordinary nested-LHS, RHS, and return reads only", reads)
	}
}

func TestStaticTypePublicationUsesPerPairEvidenceWithoutExtraRuntimeWork(t *testing.T) {
	p := parseBindLower(t, `
type User = { id: string }
local M = {}
local value = 1
M.User, M.value = User, value
`)
	assigns := p.Flow().Authored().Storage().Assigns()
	publications := p.Static().Publications()
	if assigns.Count() != 1 || publications.Count() != 1 {
		t.Fatalf("Assigns/Publications = %d/%d, want 1/1", assigns.Count(), publications.Count())
	}
	assign, ok := assigns.At(0)
	if !ok {
		t.Fatal("missing runtime Assign")
	}
	if count, ok := assigns.WriteCount(assign); !ok || count != 2 {
		t.Fatalf("Assign target count = %d/%v, want 2", count, ok)
	}
	_, values, ok := assigns.Get(assign)
	if !ok {
		t.Fatalf("root %v is not Assign", assign)
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 2 || valuesTail(t, p, values) != 0 {
		t.Fatalf("Assign Values = fixed %d/%v tail %v, want two authored RHS values", fixed, ok, valuesTail(t, p, values))
	}
	publication, _ := publications.At(0)
	gotAssign, pair, _, ok := publications.Get(publication)
	if !ok || gotAssign != assign || pair != 0 {
		t.Fatalf("TypePublication Assign/pair = %v/%d/%v, want %v/0/true", gotAssign, pair, ok, assign)
	}
	write, writeOK := assigns.WriteAt(assign, int(pair))
	_, target, targetOK := p.Flow().Authored().Storage().Writes().Get(write)
	if !writeOK || !targetOK || target == 0 {
		t.Fatalf("TypePublication pair Write = %v/%v target %v/%v", write, writeOK, target, targetOK)
	}
	if p.Flow().Authored().Storage().Reads().Count() != 4 || p.Flow().Authored().Access().Exact().Count() != 2 || p.Flow().Authored().Calls().Count() != 0 {
		t.Fatalf("runtime topology Reads/Lenses/Calls = %d/%d/%d, want 4/2/0", p.Flow().Authored().Storage().Reads().Count(), p.Flow().Authored().Access().Exact().Count(), p.Flow().Authored().Calls().Count())
	}
}

func TestQualifiedAssignmentsRemainRuntime(t *testing.T) {
	p := parseBindLower(t, `
local M = {}
local Runtime = {}
local value = 1
M.User, M.value = Runtime, value
M.new = Factory.new
`)
	if got := p.Static().Publications().Count(); got != 0 {
		t.Fatalf("TypePublicationCount = %d, want no inferred Factory.new metadata", got)
	}
	if got := p.Flow().Authored().Storage().Assigns().Count(); got != 2 {
		t.Fatalf("AssignCount = %d, want 2", got)
	}
}

func TestStaticTypePublicationDeepPathRetainsCompactPublication(t *testing.T) {
	const depth = 2048
	var source strings.Builder
	source.WriteString("type Published = number\nlocal M = {}\nM")
	for index := 0; index < depth; index++ {
		fmt.Fprintf(&source, ".k%04d", index)
	}
	source.WriteString(" = Published\n")

	p := parseBindLower(t, source.String())
	publications := p.Static().Publications()
	if got := publications.Count(); got != 1 {
		t.Fatalf("TypePublicationCount = %d, want one", got)
	}
	publication, ok := publications.At(0)
	if !ok {
		t.Fatal("missing TypePublication")
	}

	assign, pair, target, ok := publications.Get(publication)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("deep TypePublication = assign %v pair %d target %v ok %v", assign, pair, target, ok)
	}
	if lenses := p.Flow().Authored().Access().Exact().Count(); lenses != depth {
		t.Fatalf("deep TypePublication exact lenses = %d, want %d", lenses, depth)
	}
}

func TestBracketAssignmentRemainsRuntimeOnly(t *testing.T) {
	p := parseBindLower(t, `
type Published = number
local M = {}
M["Published"] = Published
`)
	if got := p.Static().Publications().Count(); got != 0 {
		t.Fatalf("TypePublicationCount = %d, want no static publication for a bracket key", got)
	}
	if got := p.Flow().Authored().Storage().Assigns().Count(); got != 1 {
		t.Fatalf("AssignCount = %d, want retained runtime assignment", got)
	}
}
