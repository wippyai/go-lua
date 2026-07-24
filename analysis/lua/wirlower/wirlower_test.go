package wirlower_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// lowerSource parses, binds, lowers, and prints src to the golden textual wir.
func lowerSource(t *testing.T, src string) string {
	t.Helper()
	return lowerSourceG(t, src, "type", "print", "pairs", "ipairs", "f", "g", "h", "obj", "t")
}

// lowerSourceG lowers src with an explicit set of recognized globals.
func lowerSourceG(t *testing.T, src string, globals ...string) string {
	t.Helper()
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: true})
	body := wirlower.LowerWithResolverAndOptions("main", stmts, bindings, built, typeresolve.New(bindings), wirlower.Options{SealedLuaTypeChecks: true})
	return wir.Print(body, built.Graph)
}

func lowerBody(t *testing.T, src string, globals ...string) *wir.Body {
	t.Helper()
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: true})
	return wirlower.LowerWithResolverAndOptions("main", stmts, bindings, built, typeresolve.New(bindings), wirlower.Options{SealedLuaTypeChecks: true})
}

func TestBranchCarriesImpliedChecks(t *testing.T) {
	stmts, err := parse.ParseString(`
local x, y, a, b
if x == y and a ~= b then
    local hit = true
end
`, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	body := wirlower.Lower("main", stmts, bindings, built)

	var got []wir.ImpliedCheck
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpBranch {
			continue
		}
		checks := body.ImpliedChecks(inst.ImpliedChecks)
		if containsImpliedCheckKind(checks, wir.CheckPathNot) {
			got = checks
			break
		}
	}
	if len(got) != 2 {
		t.Fatalf("compound branch implied checks = %d, want 2: %#v", len(got), got)
	}
	if got[0].Check.Kind != wir.CheckPathEqual || !got[0].Edge || !got[0].Polarity {
		t.Fatalf("first implied check = %#v, want true-edge path equality", got[0])
	}
	if got[1].Check.Kind != wir.CheckPathNot || !got[1].Edge || !got[1].Polarity {
		t.Fatalf("second implied check = %#v, want true-edge path inequality", got[1])
	}
	if got[0].Check.Path.IsEmpty() || got[0].Check.OtherPath.IsEmpty() || got[1].Check.Path.IsEmpty() || got[1].Check.OtherPath.IsEmpty() {
		t.Fatalf("implied checks lost paths: %#v", got)
	}
}

func TestLowerIfRetainsElseIfChainDescriptor(t *testing.T) {
	body := lowerBody(t, `
if first then
elseif second then
elseif third then
else
end
`, "first", "second", "third")
	descriptor, ok := body.IfChainDescriptor(1)
	if !ok {
		t.Fatal("missing if/elseif chain descriptor")
	}
	if !descriptor.HasElse || len(descriptor.Branches) != 3 {
		t.Fatalf("descriptor = %#v, want three branches and else", descriptor)
	}
	for index, branch := range descriptor.Branches {
		if branch.Point == 0 || !branch.Span.Valid() {
			t.Fatalf("branch %d = %#v, want point and span", index, branch)
		}
		if index > 0 && descriptor.Branches[index-1].Point == branch.Point {
			t.Fatalf("branch %d repeats previous point", index)
		}
	}
}

func containsImpliedCheckKind(checks []wir.ImpliedCheck, kind wir.CheckKind) bool {
	for _, check := range checks {
		if check.Check.Kind == kind {
			return true
		}
	}
	return false
}

func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "assignment_binop",
			src:  "local c = a + b",
			want: `body main
b0: entry
b1: c = add a b
b2: exit
`,
		},
		{
			name: "if_type_number",
			src:  "local y\nif type(x) == \"number\" then y = x else y = 0 end",
			want: `body main
b0: entry
b1: y = nil
b2: branch type_eq x "number"  then b4 else b3
b3: y = 0
b4: y = x
b5: noop
b6: exit
`,
		},
		{
			name: "numeric_for",
			src:  "local s = 0\nfor i = 1, 10 do s = s + i end",
			want: `body main
b0: entry
b1: s = 0
b2: i = _
b3: i = iterate.numeric [1, 10, 1]
b4: s = add s i
b5: noop
b6: exit
`,
		},
		{
			name: "direct_call",
			src:  "print(a, b)",
			want: `body main
b0: entry
b1: call print(a, b)
b2: exit
`,
		},
		{
			name: "cast",
			src:  "local y = x as string",
			want: `body main
b0: entry
b1: y = claim.cast x : string
b2: exit
`,
		},
		{
			name: "method_call_sugar",
			src:  "local n = obj:len()",
			want: `body main
b0: entry
b1: %0 = call obj:len()
b2: n = %0
b3: exit
`,
		},
		{
			name: "annotation_claim",
			src:  "local x: number = 1",
			want: `body main
b0: entry
b1: x = 1
    x = claim.annotation x : number
b2: exit
`,
		},
		{
			name: "member_and_index_write",
			src:  "obj.field = 5\nt[k] = v",
			want: `body main
b0: entry
b1: store.field obj.field = 5
b2: store.index t[k] = v
b3: exit
`,
		},
		{
			name: "multret_call_and_return",
			src:  "local a, b = f()\nreturn g(h())",
			want: `body main
b0: entry
b1: %0, %1 = call f()
b2: a = %0
b3: b = %1
b4: %2 = call h() multret
b5: %3 = call g(%2...) multret
b6: return %3...
b7: exit
`,
		},
		{
			name: "nonnil_assert",
			src:  "local y = x!",
			want: `body main
b0: entry
b1: y = claim.assert x
b2: exit
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lowerSource(t, tc.src)
			if got != tc.want {
				t.Fatalf("wir mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, tc.want)
			}
		})
	}
}

func TestTableConstructorCarriesStaticEntryMetadata(t *testing.T) {
	body := lowerBody(t, `
local t = {
    name = payload.id,
    ["key"] = 2,
    [7] = 3,
    4,
    [dynamic] = 5,
    6,
}
`, "dynamic", "payload")
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpMakeTable {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpMakeTable {
		t.Fatal("missing OpMakeTable")
	}
	entries := body.TableEntries(inst.TableEntries)
	if len(entries) != 5 {
		t.Fatalf("table entries = %#v, want 5 static entries", entries)
	}
	assertEntry := func(i int, want path.Path) {
		t.Helper()
		if !entries[i].Suffix.Equal(want) {
			t.Fatalf("entry %d suffix = %v, want %v", i, entries[i].Suffix, want)
		}
		if entries[i].Value.Kind == wir.OperandNone {
			t.Fatalf("entry %d missing value operand", i)
		}
	}
	assertEntry(0, suffixPath(segment.Segment{Kind: segment.SegmentField, Name: "name"}))
	assertEntry(1, suffixPath(segment.Segment{Kind: segment.SegmentIndexString, Name: "key"}))
	assertEntry(2, suffixPath(segment.Segment{Kind: segment.SegmentIndexInt, Index: 7}))
	assertEntry(3, suffixPath(segment.Segment{Kind: segment.SegmentIndexInt, Index: 1}))
	assertEntry(4, suffixPath(segment.Segment{Kind: segment.SegmentIndexInt, Index: 2}))
	if got := entries[0].ValueLabel; got != "payload.id" {
		t.Fatalf("entry 0 value label = %q, want payload.id", got)
	}
	if !entries[0].ValueSpan.Valid() || entries[0].ValueSpan.StartLine != 3 {
		t.Fatalf("entry 0 value span = %#v, want valid line 3 span", entries[0].ValueSpan)
	}
	if inst.StaticStringKeysComplete {
		t.Fatal("table with dynamic key reported complete static string key set")
	}
}

func TestTableConstructorMarksCompleteStaticStringKeySet(t *testing.T) {
	body := lowerBody(t, `
local t = {
    name = payload.id,
    ["key"] = 2,
    [7] = 3,
    4,
}
`, "payload")
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpMakeTable {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpMakeTable {
		t.Fatal("missing OpMakeTable")
	}
	if !inst.StaticStringKeysComplete {
		t.Fatal("static table reported incomplete string key set")
	}
}

func TestTableConstructorCarriesDeclaredType(t *testing.T) {
	body := lowerBody(t, `
type Context = {[string]: any}
local ctx: Context = {}
`)
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpMakeTable {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpMakeTable {
		t.Fatal("missing OpMakeTable")
	}
	if inst.Type == 0 {
		t.Fatal("OpMakeTable missing declared type")
	}
	got := body.Type(inst.Type)
	want := typ.NewMap(typ.String, typ.Any)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("OpMakeTable declared type = %v, want %v", got, want)
	}
}

func TestCallCarriesSourceMetadata(t *testing.T) {
	body := lowerBody(t, `
local value = "x"
send(value, payload.id)
`, "send", "payload")
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpCall {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpCall {
		t.Fatal("missing OpCall")
	}
	if !inst.CallSpan.Valid() || inst.CallSpan.StartLine != 3 {
		t.Fatalf("call span = %#v, want valid line 3 span", inst.CallSpan)
	}
	if !inst.CalleeSpan.Valid() || inst.CalleeSpan.StartLine != 3 {
		t.Fatalf("callee span = %#v, want valid line 3 span", inst.CalleeSpan)
	}
	args := body.CallArgumentMeta(inst.CallArgs)
	if len(args) != 2 {
		t.Fatalf("call argument metadata = %#v, want 2 entries", args)
	}
	if got := args[0].Label; got != "value" {
		t.Fatalf("arg 0 label = %q, want value", got)
	}
	if got := args[1].Label; got != "payload.id" {
		t.Fatalf("arg 1 label = %q, want payload.id", got)
	}
	if !args[0].Span.Valid() || args[0].Span.StartLine != 3 {
		t.Fatalf("arg 0 span = %#v, want valid line 3 span", args[0].Span)
	}
}

func TestReturnCarriesSourceMetadata(t *testing.T) {
	body := lowerBody(t, `
local value = "x"
return value, payload.id
`, "payload")
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpReturn {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpReturn {
		t.Fatal("missing OpReturn")
	}
	meta := body.ReturnValueMeta(inst.ReturnValues)
	if len(meta) != 2 {
		t.Fatalf("return metadata = %#v, want 2 entries", meta)
	}
	if got := meta[0].Label; got != "value" {
		t.Fatalf("return 0 label = %q, want value", got)
	}
	if got := meta[1].Label; got != "payload.id" {
		t.Fatalf("return 1 label = %q, want payload.id", got)
	}
	if !meta[0].Span.Valid() || meta[0].Span.StartLine != 3 {
		t.Fatalf("return 0 span = %#v, want valid line 3 span", meta[0].Span)
	}
}

func TestNonFinalAssignmentCallCarriesResultTarget(t *testing.T) {
	body := lowerBody(t, `
local a, b, c = make(), pack()
`, "make", "pack")
	var calls []wir.Instruction
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpCall {
			calls = append(calls, inst)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want make and pack", calls)
	}
	makeTargets := body.CallResultTargets(calls[0].Point)
	if len(makeTargets) != 1 ||
		makeTargets[0].Kind != wir.CallResultTargetLocalAssignment ||
		makeTargets[0].Index != 0 ||
		makeTargets[0].ResultIndex != 0 {
		t.Fatalf("make targets = %#v, want local assignment target 0/result 0", makeTargets)
	}
	packTargets := body.CallResultTargets(calls[1].Point)
	if len(packTargets) != 2 ||
		packTargets[0].Kind != wir.CallResultTargetLocalAssignment ||
		packTargets[0].Index != 1 ||
		packTargets[0].ResultIndex != 0 ||
		packTargets[1].Kind != wir.CallResultTargetLocalAssignment ||
		packTargets[1].Index != 2 ||
		packTargets[1].ResultIndex != 1 {
		t.Fatalf("pack targets = %#v, want local assignment targets 1/result 0 and 2/result 1", packTargets)
	}
}

func TestFunctionBodyCarriesDeclaredReturns(t *testing.T) {
	stmts, err := parse.ParseString(`
function f(): (string, number)
    return "x", 1
end
`, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	bindings := bind.BindFunction(def.Func, bind.Options{})
	built := cfgbuild.BuildFunction(def.Func, bindings)
	body := wirlower.LowerFunction("f", def.Func, bindings, built)
	if got := body.DeclaredReturnArity(); got != 2 {
		t.Fatalf("declared return arity = %d, want 2", got)
	}
	returns := body.DeclaredReturnTypes()
	if len(returns) != 2 || !typ.TypeEquals(returns[0], typ.String) || !typ.TypeEquals(returns[1], typ.Number) {
		t.Fatalf("declared return types = %#v, want string, number", returns)
	}
}

func TestDirectTypeValueCallCarriesResolvedType(t *testing.T) {
	body := lowerBody(t, `
type Point = { x: number }
local raw: any = {}
local point = Point(raw)
`)
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpCall {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpCall {
		t.Fatal("missing OpCall")
	}
	got := body.Type(inst.Type)
	want := typetable.NewRecord().Field("x", typ.Number).Build()
	if got == nil || !typ.TypeEquals(got, want) {
		t.Fatalf("OpCall.Type = %v, want %v", got, want)
	}
}

func TestBuiltinTypeCallIgnoresSameNamedManifestType(t *testing.T) {
	registry := manifest.New("registry")
	registry.DefineType("Entry", typetable.NewRecord().Field("id", typ.String).Build())
	fs := manifest.New("fs")
	fs.DefineType("type", typetable.NewRecord().Field("shadowed", typ.String).Build())

	stmts, err := parse.ParseString(`
local e = registry_get()
if type(e.id) == "string" then
    local s: string = e.id
    local tag: "string" = type(e.id)
end
`, "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"registry_get", "type"}})
	built := cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: true})

	for _, tc := range []struct {
		name      string
		manifests []*manifest.Manifest
	}{
		{name: "registry only", manifests: []*manifest.Manifest{registry}},
		{name: "registry and fs type named type", manifests: []*manifest.Manifest{registry, fs}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver := typeresolve.NewWithExternal(bindings, typelookup.Source{Manifests: tc.manifests})
			body := wirlower.LowerWithResolverAndOptions("main", stmts, bindings, built, resolver, wirlower.Options{SealedLuaTypeChecks: true})
			typeCalls := 0
			typePredicateChecks := 0

			for i := 0; i < body.Len(); i++ {
				inst := body.Instr(i)
				if inst.Op == wir.OpBranch {
					check := body.Check(inst.Check)
					if check.Kind == wir.CheckTypeEqual && check.Path.Root == "e" && check.TypeName == "string" {
						typePredicateChecks++
					}
				}
				if inst.Op != wir.OpCall || inst.Call.Callee.Kind != wir.OperandPath {
					continue
				}
				callee := body.Path(wir.PathRef(inst.Call.Callee.Ref))
				if callee.Root != "type" || len(callee.Segments) != 0 {
					continue
				}
				typeCalls++
				if inst.Type != 0 {
					t.Fatalf("builtin type() OpCall.Type = %d/%v, want no declared-type target", inst.Type, body.Type(inst.Type))
				}
			}
			if typeCalls != 1 {
				t.Fatalf("builtin type() calls = %d, want 1:\n%s", typeCalls, wir.Print(body, built.Graph))
			}
			if typePredicateChecks != 1 {
				t.Fatalf("builtin type() predicate checks = %d, want 1:\n%s", typePredicateChecks, wir.Print(body, built.Graph))
			}
		})
	}
}

func TestNestedFunctionRetainsSealedTypeGuardBody(t *testing.T) {
	body := lowerBody(t, `
local function validate(value: any)
    if type(value.item) == "table" then
        local labels: {string} = value.item
        return labels
    end
    return nil
end
`, "type")
	protos := body.Protos()
	if len(protos) != 1 || protos[0].Body == nil {
		t.Fatalf("nested prototypes = %#v", protos)
	}
	child := protos[0].Body
	var branch, claim bool
	for index := 0; index < child.Len(); index++ {
		switch child.Instr(index).Op {
		case wir.OpBranch:
			branch = true
		case wir.OpClaim:
			claim = true
		}
	}
	if !branch || !claim {
		t.Fatalf("sealed nested type guard lost its body: branch=%t claim=%t", branch, claim)
	}
}

func TestCallCarriesExplicitTypeArguments(t *testing.T) {
	call := &ast.FuncCallExpr{
		Func: ident("send"),
		Args: []ast.Expr{ident("value")},
		TypeArgs: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	stmts := []ast.Stmt{&ast.FuncCallStmt{Expr: call}}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"send", "value"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	body := wirlower.Lower("main", stmts, bindings, built)
	var inst wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpCall {
			inst = candidate
			break
		}
	}
	if inst.Op != wir.OpCall {
		t.Fatal("missing OpCall")
	}
	refs := body.TypeRefs(inst.CallTypeArgs)
	if len(refs) != 2 {
		t.Fatalf("call type args = %#v, want two refs", refs)
	}
	if got := body.Type(refs[0]); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("first call type arg = %v, want string", got)
	}
	if got := body.Type(refs[1]); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("second call type arg = %v, want number", got)
	}
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func TestDirectPrimitiveNameValueShadowDoesNotCarryType(t *testing.T) {
	body := lowerBody(t, `
local number = function(value) return value end
local raw: any = {}
local value = number(raw)
`)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpCall {
			continue
		}
		if inst.Type != 0 {
			t.Fatalf("shadowed primitive OpCall.Type = %d/%v, want none", inst.Type, body.Type(inst.Type))
		}
		return
	}
	t.Fatal("missing OpCall")
}

func TestTableConstructorCarriesFlattenedNestedEntryMetadata(t *testing.T) {
	body := lowerBody(t, `
local t = {
    x = 1,
    a = {
        b = payload.child,
        [dynamic] = 3,
    },
}
`, "dynamic", "payload")
	var root wir.Instruction
	for i := 0; i < body.Len(); i++ {
		candidate := body.Instr(i)
		if candidate.Op == wir.OpMakeTable && candidate.Dst.Kind == wir.OperandPath {
			root = candidate
			break
		}
	}
	if root.Op != wir.OpMakeTable {
		t.Fatal("missing root OpMakeTable")
	}
	entries := body.TableEntries(root.TableEntries)
	if len(entries) != 3 {
		t.Fatalf("root table entries = %#v, want x, a, and a.b", entries)
	}
	want := []path.Path{
		suffixPath(segment.Segment{Kind: segment.SegmentField, Name: "x"}),
		suffixPath(segment.Segment{Kind: segment.SegmentField, Name: "a"}),
		suffixPath(
			segment.Segment{Kind: segment.SegmentField, Name: "a"},
			segment.Segment{Kind: segment.SegmentField, Name: "b"},
		),
	}
	for i := range want {
		if !entries[i].Suffix.Equal(want[i]) {
			t.Fatalf("entry %d suffix = %v, want %v", i, entries[i].Suffix, want[i])
		}
		if entries[i].Value.Kind == wir.OperandNone {
			t.Fatalf("entry %d missing value operand", i)
		}
	}
	if got := entries[2].ValueLabel; got != "payload.child" {
		t.Fatalf("flattened nested entry label = %q, want payload.child", got)
	}
	if !entries[2].ValueSpan.Valid() || entries[2].ValueSpan.StartLine != 5 {
		t.Fatalf("flattened nested entry span = %#v, want valid line 5 span", entries[2].ValueSpan)
	}
}

func TestTableConstructorCarriesStableExpressionIdentity(t *testing.T) {
	body := lowerBody(t, `
local first = { a = 1 }
local second = { child = { b = 2 } }
`)
	var ids []wir.ExpressionID
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpMakeTable {
			continue
		}
		if inst.ExprID == 0 {
			t.Fatalf("OpMakeTable at instruction %d missing expression identity", i)
		}
		ids = append(ids, inst.ExprID)
	}
	if len(ids) != 3 {
		t.Fatalf("table expression ids = %v, want first, nested child, and second", ids)
	}
	seen := map[wir.ExpressionID]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate table expression id %d in %v", id, ids)
		}
		seen[id] = true
	}
}

func TestCallCarriesStableExpressionIdentity(t *testing.T) {
	body := lowerBody(t, `
local first = f()
local second = g(h())
`)
	var ids []wir.ExpressionID
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpCall {
			continue
		}
		if inst.ExprID == 0 {
			t.Fatalf("OpCall at instruction %d missing expression identity", i)
		}
		ids = append(ids, inst.ExprID)
	}
	if len(ids) != 3 {
		t.Fatalf("call expression ids = %v, want f, h, and g", ids)
	}
	seen := map[wir.ExpressionID]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate call expression id %d in %v", id, ids)
		}
		seen[id] = true
	}
}

func TestClaimWrappedCallBindingEmitsClaimAtAssignmentPoint(t *testing.T) {
	body := lowerBody(t, `
type Message = {topic: string}
local inbox = make() as Message
local ready = check()!
`, "make", "check")
	var claims []wir.Instruction
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpClaim {
			claims = append(claims, inst)
		}
	}
	if len(claims) != 2 {
		t.Fatalf("claims = %#v, want cast and non-nil claims", claims)
	}
	if claims[0].Claim != wir.ClaimCast || claims[0].Type == 0 || claims[0].Dst.Kind != wir.OperandPath || claims[0].A.Kind != wir.OperandTemp {
		t.Fatalf("cast claim = %#v, want path target claiming call result temp with type", claims[0])
	}
	if claims[1].Claim != wir.ClaimAssert || claims[1].Type != 0 || claims[1].Dst.Kind != wir.OperandPath || claims[1].A.Kind != wir.OperandTemp {
		t.Fatalf("non-nil claim = %#v, want path target claiming call result temp without type", claims[1])
	}
}

// TestClaimSourcedLocalAssignmentResolvesAliasSource proves a claim-sourced
// local assignment (`local y = obj.child :: number`) exposes its aliased
// source path through Instruction.AssignmentSourceOperand. OpClaim writes a
// value the same way OpAssign/OpStaticMemberWrite do; consumers that resolve
// assignment sources must see through the claim to the path it wraps.
func TestClaimSourcedLocalAssignmentResolvesAliasSource(t *testing.T) {
	body := lowerBody(t, `
local obj = {}
local y = obj.child :: number
`)
	var claim wir.Instruction
	found := false
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpClaim {
			claim = inst
			found = true
		}
	}
	if !found {
		t.Fatalf("no OpClaim instruction lowered for claim-sourced assignment")
	}
	source, ok := claim.AssignmentSourceOperand()
	if !ok {
		t.Fatalf("AssignmentSourceOperand() ok = false, want true for claim-sourced assignment")
	}
	if source.Kind != wir.OperandPath {
		t.Fatalf("AssignmentSourceOperand() = %#v, want path operand", source)
	}
	got := body.Path(wir.PathRef(source.Ref))
	want := []segment.Segment{{Kind: segment.SegmentField, Name: "child"}}
	if len(got.Segments) != 1 || got.Segments[0] != want[0] {
		t.Fatalf("resolved alias path = %#v, want obj.child", got)
	}
}

func TestDirectGlobalAssertCallCarriesNormalizedCheck(t *testing.T) {
	src := `
local x = nil
assert(x ~= nil)
`
	body := lowerBody(t, src, "assert")
	var checks []wir.Check
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpCall || inst.Check == 0 {
			continue
		}
		checks = append(checks, body.Check(inst.Check))
	}
	if len(checks) != 1 {
		t.Fatalf("assert checks = %#v, want one normalized check", checks)
	}
	if checks[0].Kind != wir.CheckNotNil || checks[0].Path.Root != "x" {
		t.Fatalf("assert check = %#v, want x ~= nil", checks[0])
	}
	printed := lowerSourceG(t, src, "assert")
	if !strings.Contains(printed, "check[notnil x]") {
		t.Fatalf("printed WIR missing assert check metadata:\n%s", printed)
	}
}

func TestShadowedAssertCallDoesNotCarryNormalizedCheck(t *testing.T) {
	body := lowerBody(t, `
local x = nil
local assert = other
assert(x)
`, "assert", "other")
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op == wir.OpCall && inst.Check != 0 {
			t.Fatalf("shadowed assert call carried check: %#v", inst)
		}
	}
}

func suffixPath(segs ...segment.Segment) path.Path {
	return path.Path{Segments: append([]segment.Segment(nil), segs...)}
}

// TestGoldenExtended covers the constructs the prototype skipped: control-flow
// (repeat, break, goto/label), short-circuit and/or, closures and function
// definitions, table array+hash+spread constructors, channel-select, and the
// adversarial multret interactions (call in middle vs tail, multi-assign tail
// expansion).
func TestGoldenExtended(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		globals []string
		want    string
	}{
		{
			name: "repeat_until",
			src:  "local s = 0\nrepeat s = s + 1 until s > 10",
			want: `body main
b0: entry
b1: s = 0
b2: s = add s 1
b3: branch num_ge s 11  then b4 else b2
b4: noop
b5: exit
`,
		},
		{
			name:    "while_break",
			src:     "while cond do if x then break end end",
			globals: []string{"cond", "x"},
			want: `body main
b0: entry
b1: branch truthy cond  then b2 else b6
b2: branch truthy x  then b5 else b3
b3: noop
b4: noop
b5: noop
b6: noop
b7: exit
`,
		},
		{
			name:    "logical_and_or",
			src:     "local y = a and b or c",
			globals: []string{"a", "b", "c"},
			want: `body main
b0: entry
b1: branch truthy a  then b2 else b3
b2: noop
b3: noop
b4: %0 = and a b
    branch cond %0  then b6 else b5
b5: noop
b6: noop
b7: y = or %0 c
b8: exit
`,
		},
		{
			name:    "logical_and_effectful",
			src:     "local y = a and f()",
			globals: []string{"a", "f"},
			want: `body main
b0: entry
b1: %1 = a
    branch truthy a  then b2 else b3
b2: %0 = call f()
    %1 = %0
b3: noop
b4: y = %1
b5: exit
`,
		},
		{
			name:    "logical_or_effectful",
			src:     "local y = a or f()",
			globals: []string{"a", "f"},
			want: `body main
b0: entry
b1: %1 = a
    branch truthy a  then b3 else b2
b2: %0 = call f()
    %1 = %0
b3: noop
b4: y = %1
b5: exit
`,
		},
		{
			name:    "logical_and_member_effectful",
			src:     "local y = a and t.f",
			globals: []string{"a", "t"},
			want: `body main
b0: entry
b1: %0 = a
    branch truthy a  then b2 else b3
b2: %0 = t.f
b3: noop
b4: y = %0
b5: exit
`,
		},
		{
			name: "local_function_closure",
			src:  "local function inc(n) return n + 1 end\nlocal r = inc(2)",
			want: `body main
b0: entry
b1: inc = closure main.fn0
b2: %0 = call inc(2)
b3: r = %0
b4: exit

body main.fn0
b0: entry
b1: noop
b2: %0 = add n 1
    return %0
b3: exit
`,
		},
		{
			name: "funcdef_member",
			src:  "function obj.m(v) return v end",
			want: `body main
b0: entry
b1: %0 = closure main.fn0
    store.field obj.m = %0
b2: exit

body main.fn0
b0: entry
b1: noop
b2: return v
b3: exit
`,
		},
		{
			name: "method_def",
			src:  "function obj:m(v) return v end",
			want: `body main
b0: entry
b1: %0 = closure main.fn0
    store.field obj.m = %0
b2: exit

body main.fn0
b0: entry
b1: noop
b2: noop
b3: return v
b4: exit
`,
		},
		{
			name: "goto_label_dead_code",
			src:  "goto done\nprint(1)\n::done::",
			want: `body main
b0: entry
b1: noop
b2: noop
b3: exit
`,
		},
		{
			name: "table_array_hash",
			src:  "local tbl = {10, x = 1, [\"k\"] = 2}",
			want: `body main
b0: entry
b1: tbl = table [10, 1, 2]
b2: exit
`,
		},
		{
			name: "table_spread_tail",
			src:  "local tbl = {1, 2, f()}",
			want: `body main
b0: entry
b1: %0 = call f() multret
b2: tbl = table [1, 2, %0]
b3: exit
`,
		},
		{
			name:    "nested_closure",
			src:     "local mk = function() return function() return x end end",
			globals: []string{"x"},
			want: `body main
b0: entry
b1: mk = closure main.fn0
b2: exit

body main.fn0
b0: entry
b1: %0 = closure main.fn0.fn0
    return %0
b2: exit

body main.fn0.fn0
b0: entry
b1: return x
b2: exit
`,
		},
		{
			name:    "channel_select",
			src:     "type Message = {kind: string}\nlocal ch: Channel<Message>\nlocal r = channel.select { ch:case_receive(), default = true }",
			globals: []string{"channel"},
			want: `body main
b0: entry
b1: noop
b2: ch = nil
    ch = claim.annotation ch : Channel<{kind: string}>
b3: %0 = select [ch] default
b4: r = %0
b5: exit
`,
		},
		{
			name: "multret_call_in_middle_vs_tail",
			src:  "print(f(), g())",
			want: `body main
b0: entry
b1: %0 = call f()
b2: %1 = call g() multret
b3: call print(%0, %1...)
b4: exit
`,
		},
		{
			name: "multret_multi_assign_tail_expansion",
			src:  "local a, b, c = f(), g()",
			want: `body main
b0: entry
b1: %0 = call f()
b2: %1, %2 = call g()
b3: a = %0
b4: b = %1
b5: c = %2
b6: exit
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			globals := tc.globals
			if globals == nil {
				globals = []string{"type", "print", "pairs", "ipairs", "f", "g", "h", "obj", "t"}
			}
			got := lowerSourceG(t, tc.src, globals...)
			if got != tc.want {
				t.Fatalf("wir mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, tc.want)
			}
		})
	}
}
