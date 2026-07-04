package readmodel

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestValueTypeWitnessPresentProjectsConcreteType(t *testing.T) {
	reg := standard.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = typevalue.WithWitness(reg, value, typeexpr.Optional(typ.String))

	got, ok := New(&body.Result{}).ValueType(value)
	if ok {
		t.Fatalf("ValueType with nil registry result returned %v, want false", got)
	}

	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok = New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestChannelSelectCaseIndexPreservesDuplicateAndReversedMatches(t *testing.T) {
	selected := pathdom.Path{Root: "selected"}
	result := selected.Field("result")
	resultChannel := result.Field(channelselect.ResultChannelField)
	primary := pathdom.Path{Root: "primary"}
	timers := pathdom.Path{Root: "timers"}
	otherResult := pathdom.Path{Root: "other"}.Field("result")

	index := newReadmodelChannelSelectCaseIndex([]readmodelSelectInfo{
		{
			result: result,
			cases: []readmodelSelectCase{
				{path: primary, name: "primary receive"},
				{path: primary, name: "primary send"},
				{path: timers, name: "timers"},
			},
		},
		{
			result: otherResult,
			cases:  []readmodelSelectCase{{path: primary, name: "later primary"}},
		},
	})

	matches := index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      primary,
		OtherPath: resultChannel,
	})
	if len(matches) != 2 ||
		matches[0].selectIndex != 0 || matches[0].caseIndex != 0 ||
		matches[1].selectIndex != 0 || matches[1].caseIndex != 1 {
		t.Fatalf("reversed primary matches = %#v, want first select duplicate cases [0 1]", matches)
	}

	matches = index.matchesForCheck(branchcond.Check{
		Kind:      branchcond.CheckPathEqual,
		Path:      resultChannel,
		OtherPath: timers,
	})
	if len(matches) != 1 || matches[0].selectIndex != 0 || matches[0].caseIndex != 2 {
		t.Fatalf("direct timers matches = %#v, want select 0 case [2]", matches)
	}
}

func TestForEachUnresolvedValueReferenceReportsImplicitGlobalReads(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local x = missing + known
missing = 42
print(known)
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		Globals:  []string{"known", "print"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []UnresolvedValueReference
	New(result).ForEachUnresolvedValueReference(func(ref UnresolvedValueReference) bool {
		got = append(got, ref)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("unresolved value refs = %#v, want one missing read", got)
	}
	if got[0].Name != "missing" || got[0].Span.StartLine != 1 || got[0].Span.StartCol != 11 {
		t.Fatalf("unresolved ref = %#v, want missing at line 1 col 11", got[0])
	}
}

func TestForEachUnresolvedValueReferenceSkipsTypeSyntax(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Payload = {id: string}
local raw: any = {}
local payload = Payload(raw)
local ok = Payload:is(raw)
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []UnresolvedValueReference
	New(result).ForEachUnresolvedValueReference(func(ref UnresolvedValueReference) bool {
		got = append(got, ref)
		return true
	})
	if len(got) != 0 {
		t.Fatalf("unresolved value refs = %#v, want type syntax skipped", got)
	}
}

func TestForEachUnresolvedTypeReferenceReportsKnownOutOfScopeType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
if true then
	type LocalPoint = {x: number}
end
local p: LocalPoint = {x = 1}
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []UnresolvedTypeReference
	New(result).ForEachUnresolvedTypeReference(func(ref UnresolvedTypeReference) bool {
		got = append(got, ref)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("unresolved type refs = %#v, want one LocalPoint ref", got)
	}
	if got[0].Name != "LocalPoint" || got[0].Span.StartLine != 4 || got[0].Span.StartCol != 10 {
		t.Fatalf("unresolved type ref = %#v, want LocalPoint at line 4 col 10", got[0])
	}
}

func TestForEachUnresolvedTypeReferenceSkipsUnknownUnqualifiedNames(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local p: ExternalName = {}
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []UnresolvedTypeReference
	New(result).ForEachUnresolvedTypeReference(func(ref UnresolvedTypeReference) bool {
		got = append(got, ref)
		return true
	})
	if len(got) != 0 {
		t.Fatalf("unresolved type refs = %#v, want unknown unqualified external-style name skipped", got)
	}
}

func TestValueTypeAbsentProjectsNil(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	got, ok := New(result).ValueType(product.Absent(reg))
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.Nil)
}

func TestSourceValueReadsAnyAssertionClaimFromLocalAssignmentSource(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local request = ({id = "r1", retries = 2} :: any)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	assign := stmts[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false")
	}
	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("SourceValue did not preserve assertion.Any: %v", value)
	}
	got, ok := reader.SourceType(point, fact.Source)
	if !ok || !typ.IsAny(got) {
		t.Fatalf("SourceType = %v/%v, want any", got, ok)
	}
}

func TestValueTypeWithPresenceAddsNilForMaybeWitness(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	value = typevalue.WithWitness(reg, value, typ.String)

	got, ok := New(result).ValueTypeWithPresence(value)
	if !ok {
		t.Fatalf("ValueTypeWithPresence returned false")
	}
	assertSameType(t, got, typeexpr.Optional(typ.String))
}

func TestValueTypeMaybeWitnessStaysConcrete(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	value = typevalue.WithWitness(reg, value, typ.String)

	got, ok := New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestForEachAssignmentProjectsClosedRecordDynamicWriteContract(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Row = {
	id: string,
	count: number,
}

local key: string = "id"
local value: number = 1
local row: Row = {id = "ok", count = 0}
row[key] = value
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "row[key]" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("dynamic write assignments = %#v, want one row[key] assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want number rejected by closed-record dynamic write contract")
	}
	if got[0].Expected == nil {
		t.Fatalf("assignment expected type is nil")
	}
	if !strings.Contains(got[0].Expected.String(), "string") || !strings.Contains(got[0].Expected.String(), "number") {
		t.Fatalf("assignment expected = %v, want meet of closed-record field contracts", got[0].Expected)
	}
}

func TestForEachAssignmentKeepsLiteralIntegerWitnessForDynamicMapWrite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local counters: {[string]: integer} = {}
local key = "sent"
counters[key] = 1
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "counters[key]" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 0 {
		t.Fatalf("dynamic integer write assignments = %#v, want literal integer source accepted", got)
	}
}

func TestForEachAssignmentReportsObjectLiteralExplicitAnyMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {id: string}
local raw: any = nil
local p: Point = {id = raw}
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "p.id" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("object-literal member assignments = %#v, want one p.id assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want explicit any rejected for required member")
	}
	if !got[0].UntrustedTopOrigin {
		t.Fatalf("assignment did not preserve explicit any origin")
	}
}

func TestForEachAssignmentReportsOrdinaryObjectLiteralExplicitAnyMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {id: string}
type Box = {p: Point}
local raw: any = nil
local box: Box = {p = {id = "ok"}}
box.p = {id = raw}
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "box.p.id" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("ordinary object-literal member assignments = %#v, want one box.p.id assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want explicit any rejected for required member")
	}
	if !got[0].UntrustedTopOrigin || !got[0].ExplicitTopOrigin {
		t.Fatalf("assignment origins = untrusted:%v explicit:%v, want explicit any origin", got[0].UntrustedTopOrigin, got[0].ExplicitTopOrigin)
	}
	if got[0].ExpectedSource != readapi.AssignmentExpectedDynamicTarget {
		t.Fatalf("expected source = %v, want assignment target authority", got[0].ExpectedSource)
	}
}

func TestForEachAssignmentReportsMapReadAfterHelperMutationAsNilable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type AllowDecision = { kind: "allow", reason: string }
type DenyDecision = { kind: "deny", reason: string }
type Decision = AllowDecision | DenyDecision
type Store = {
	cached: {[string]: Decision},
}

local store: Store = { cached = {} }
local function cache_decision(s: Store, key: string, decision: Decision): ()
	s.cached[key] = decision
end
cache_decision(store, "present", { kind = "allow", reason = "ok" })
local missing: Decision = store.cached["missing"]
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "missing" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("missing assignments = %#v, want one nilable indexed read", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible with source=%v expected=%v, want map read to require nil proof", got[0].TypeWithPresence, got[0].Expected)
	}
	if !typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want nilable indexed read", got[0].TypeWithPresence)
	}
}

func TestForEachAssignmentReportsBroadDynamicKeyDoesNotProveStaticMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Box = {
	value: string?,
}

local function f(k: string): ()
	local box: Box = {}
	box[k] = "ready"
	local after: string = box.value
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	result := root.FunctionResults()[0]
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "after" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("after assignments = %#v, want one nilable member read", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible with source=%v expected=%v, want broad dynamic key not to prove box.value", got[0].TypeWithPresence, got[0].Expected)
	}
	if !typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want nilable member read", got[0].TypeWithPresence)
	}
}

func TestForEachAssignmentUsesConvergedFunctionValueType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Res = { answer: string }
local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}
function M.run()
	return M.dep.get()
end
M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}
local f: fun(): Res = M.run
local res = f()
local answer: string = res.answer
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "f" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("f assignments = %#v, want one function assignment", got)
	}
	if !got[0].Check.Admissible {
		t.Fatalf("assignment check is not admissible with source=%v expected=%v", got[0].TypeWithPresence, got[0].Expected)
	}
	source := got[0].TypeWithPresence.String()
	if !strings.Contains(source, "answer:") || !strings.Contains(source, "ok") {
		t.Fatalf("source type = %v, want converged return shape", got[0].TypeWithPresence)
	}
}

func TestForEachAssignmentKeepsBranchReassignedCallResultMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function make(): { answer: string }
	return { answer = "ok" }
end

local function run(flag: boolean)
	local res = make()
	if flag then
		res = { answer = 1 }
	end
	local answer: string? = res.answer
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 2 {
		t.Fatalf("function results = %#v, want make and run", root)
	}
	var runResult *body.Result
	for _, fn := range root.FunctionResults() {
		var sawAnswer bool
		New(fn).ForEachAssignment(func(assignment Assignment) bool {
			if assignment.TargetLabel == "answer" {
				sawAnswer = true
			}
			return true
		})
		if sawAnswer {
			runResult = fn
			break
		}
	}
	if runResult == nil {
		t.Fatal("run function result not found")
	}
	var got []Assignment
	New(runResult).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "answer" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("answer assignments = %#v, want one reassigned member read", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible with source=%v expected=%v, want branch reassignment mismatch", got[0].TypeWithPresence, got[0].Expected)
	}
	source := got[0].TypeWithPresence.String()
	if !strings.Contains(source, "string") || !strings.Contains(source, "1") {
		t.Fatalf("source type = %v, want original call arm plus reassigned literal", got[0].TypeWithPresence)
	}
}

func TestForEachAssignmentMarksCallResultSourceReturnSpan(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function add(a: number, b: number): number
	return a + b
end
local x: string = add(1, 2)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "x" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("x assignments = %#v, want one", got)
	}
	source := got[0].CallResult
	if !source.Present || source.CallableName != "add" || source.ResultIndex != 0 {
		t.Fatalf("call result source = %#v, want add result 0", source)
	}
	fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	want := sourceSpanFromAST(ast.SpanOf(fn.ReturnTypes[0]))
	if source.ReturnSpan != want {
		t.Fatalf("return span = %#v, want %#v", source.ReturnSpan, want)
	}
}

func TestForEachAssignmentMarksUnderSuppliedCallResultAsNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function one(): number return 1 end
local a: number, b: number = one()
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "b" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("b assignments = %#v, want one", got)
	}
	if !got[0].CallResult.Present || !got[0].CallResult.UnderSupplied || got[0].CallResult.ResultIndex != 1 {
		t.Fatalf("call result source = %#v, want under-supplied result 1", got[0].CallResult)
	}
	if !typ.Nil.Equals(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want nil", got[0].TypeWithPresence)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want nil-to-number mismatch")
	}
}

func TestCallResultSourceTypeUsesCanonicalSolvedContract(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function id<T>(value: T): T
	return value
end
local s: string = id("ok")
local n: number = id(1)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	reader := New(result)
	got := map[string]typ.Type{}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || (fact.Name != "s" && fact.Name != "n") {
			continue
		}
		tp, ok := reader.CallResultSourceType(fact.Source)
		if !ok {
			t.Fatalf("CallResultSourceType(%s) returned false", fact.Name)
		}
		got[fact.Name] = tp
	}
	if !typ.TypeEquals(got["s"], typ.LiteralString("ok")) {
		t.Fatalf("s result type = %v, want literal string", got["s"])
	}
	if !typ.TypeEquals(got["n"], typ.LiteralInt(1)) {
		t.Fatalf("n result type = %v, want literal integer", got["n"])
	}
}

func TestForEachAssignmentKeepsGenericCallResultIdentityWithoutReportableReturn(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function pair<T>(x: T): (T, string)
	return x, "ok"
end

local raw = ({ id = "ok" } :: any)
local req: { id: string }, label: string = pair(raw)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var req Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "req" {
			req = assignment
		}
		return true
	})
	if !req.CallResult.Present || req.CallResult.ResultIndex != 0 || req.CallResult.ReturnSpan.StartLine != 0 {
		t.Fatalf("req call result source = %#v, want present result 0 without declared return span", req.CallResult)
	}
}

func TestForEachReturnProjectsDeclaredReturnMismatch(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function f(): number
	return "bad"
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var returns []Return
	New(root.FunctionResults()[0]).ForEachReturn(func(ret Return) bool {
		returns = append(returns, ret)
		return true
	})
	if len(returns) != 1 {
		t.Fatalf("returns = %d, want 1: %#v", len(returns), returns)
	}
	ret := returns[0]
	assertSameType(t, ret.TypeWithPresence, typ.LiteralString("bad"))
	assertSameType(t, ret.Expected, typ.Number)
	if ret.SourceLabel != `"bad"` {
		t.Fatalf("source label = %q", ret.SourceLabel)
	}
	if ret.SourceSpan.StartLine != 2 || ret.DeclarationSpan.StartLine != 1 {
		t.Fatalf("spans source=%#v declaration=%#v, want return line 2 and declaration line 1", ret.SourceSpan, ret.DeclarationSpan)
	}
	if ret.Check.Admissible || !ret.Check.ProvenMismatch {
		t.Fatalf("check = %#v, want refuted mismatch", ret.Check)
	}
}

func TestForEachReturnAcceptsRuntimeCastOnCallProducer(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local Proto = {}

local function f(): {id: string}
	local self = {id = "ok"}
	return setmetatable(self, Proto) :: {id: string}
end
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{Registry: reg, Globals: []string{"setmetatable"}},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var returns []Return
	New(root.FunctionResults()[0]).ForEachReturn(func(ret Return) bool {
		returns = append(returns, ret)
		return true
	})
	if len(returns) != 1 {
		t.Fatalf("returns = %#v, want one return projection", returns)
	}
	if ret := returns[0]; !ret.Check.Admissible {
		t.Fatalf("return check = %#v, want runtime-validated call producer accepted", ret.Check)
	}
}

func TestForEachReturnProjectsObjectLiteralExplicitAnyMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = { id: string }
local function make(raw: any): Point
	return { id = raw }
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var returns []Return
	New(root.FunctionResults()[0]).ForEachReturn(func(ret Return) bool {
		returns = append(returns, ret)
		return true
	})
	if len(returns) != 1 {
		t.Fatalf("returns = %d, want 1: %#v", len(returns), returns)
	}
	ret := returns[0]
	if got, want := ret.ExpectedLabel, "returned value 1.id"; got != want {
		t.Fatalf("expected label = %q, want %q", got, want)
	}
	if got, want := ret.SourceLabel, "raw"; got != want {
		t.Fatalf("source label = %q, want %q", got, want)
	}
	assertSameType(t, ret.Expected, typ.String)
	if !ret.UntrustedTopOrigin || !ret.ExplicitTopOrigin {
		t.Fatalf("top origins = untrusted:%v explicit:%v, want both", ret.UntrustedTopOrigin, ret.ExplicitTopOrigin)
	}
	if ret.Check.Admissible {
		t.Fatalf("check = %#v, want missing proof", ret.Check)
	}
}

func TestForEachReturnProjectsDeclaredObjectLiteralExplicitAnyMember(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = { id: string }
local function make(raw: any): Point
	local result = { id = raw }
	return result
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var returns []Return
	New(root.FunctionResults()[0]).ForEachReturn(func(ret Return) bool {
		returns = append(returns, ret)
		return true
	})
	if len(returns) != 1 {
		t.Fatalf("returns = %d, want 1: %#v", len(returns), returns)
	}
	ret := returns[0]
	if got, want := ret.ExpectedLabel, "returned value 1.id"; got != want {
		t.Fatalf("expected label = %q, want %q", got, want)
	}
	if got, want := ret.SourceLabel, "raw"; got != want {
		t.Fatalf("source label = %q, want %q", got, want)
	}
	assertSameType(t, ret.Expected, typ.String)
	if !ret.UntrustedTopOrigin || !ret.ExplicitTopOrigin {
		t.Fatalf("top origins = untrusted:%v explicit:%v, want both", ret.UntrustedTopOrigin, ret.ExplicitTopOrigin)
	}
	if ret.Check.Admissible {
		t.Fatalf("check = %#v, want missing proof", ret.Check)
	}
}

func TestForEachReturnRejectsAnyReceiverMethodReturnAsStringProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function call_provider(provider_instance: any): (table?, string?)
	local raw_result, err = (provider_instance :: any):structured_output({})
	if err then
		return nil, err:message()
	end
	return raw_result :: table, nil
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var returns []Return
	New(root.FunctionResults()[0]).ForEachReturn(func(ret Return) bool {
		if ret.Index == 1 && ret.SourceLabel == "err:message(...)" {
			returns = append(returns, ret)
		}
		return true
	})
	if len(returns) != 1 {
		t.Fatalf("returns = %#v, want one err:message return", returns)
	}
	ret := returns[0]
	if !ret.UntrustedTopOrigin {
		t.Fatalf("untrusted origin = false, want err:message() from any receiver marked untrusted")
	}
	if !ret.ExplicitTopOrigin {
		t.Fatalf("explicit top origin = false, want err:message() to carry explicit any receiver origin")
	}
	if ret.Check.Admissible {
		t.Fatalf("check = %#v, want missing proof for any receiver method return", ret.Check)
	}
}

func TestForEachAssignmentProjectsOptionalNestedIndexedReadAsNilableDeclaredSlot(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Tags = {[string]: string}
type Policy = { tags: Tags }
local maybe: Policy? = nil
local source: string = maybe.tags["source"]
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "source" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("source assignments = %#v, want one nested indexed assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check admissible with source=%v, want nilable indexed read mismatch", got[0].TypeWithPresence)
	}
	if got[0].TypeWithPresence == nil {
		var pathDebug string
		var canMiss bool
		var declaredDebug string
		for _, point := range result.Graph().RPO() {
			fact, ok := result.LocalAssignment(point)
			if !ok || fact.Name != "source" {
				continue
			}
			if p, ok := result.ExpressionPath(fact.Expr); ok {
				pathDebug = p.String()
			}
			canMiss = result.MemberReadCanMiss(point, fact.Expr)
			if t, ok := result.DeclaredExpressionTypeAt(point, fact.Expr); ok && t != nil {
				declaredDebug = t.String()
			}
		}
		t.Fatalf("source type is nil, want string|nil from declared slot plus nilable receiver; expression path=%q canMiss=%v declared=%q", pathDebug, canMiss, declaredDebug)
	}
	source := got[0].TypeWithPresence.String()
	if !strings.Contains(source, "string") || !typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want string|nil from declared slot plus nilable receiver", got[0].TypeWithPresence)
	}
}

func TestForEachAssignmentProjectsInvalidatedVariantMemberReadAsNilable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type FileSlot = {
	kind: "file",
	path: string,
}
type TimerSlot = {
	kind: "timer",
	seconds: number,
}
type Slot = {
	value: FileSlot | TimerSlot,
}
type Slots = {[string]: Slot}

local slots: Slots = {
	active = {
		value = {kind = "file", path = "/tmp/active"},
	},
}
local key = "active"

if slots.active.value.kind == "file" then
	slots[key].value = {kind = "timer", seconds = 20}
	local stale_path: string = slots.active.value.path
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "stale_path" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("stale_path assignments = %#v, want one invalidated member read", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check admissible with source=%v, want invalidated member read mismatch", got[0].TypeWithPresence)
	}
	if got[0].TypeWithPresence == nil || !typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		var pathDebug string
		var canMiss bool
		var declaredDebug string
		for _, point := range result.Graph().RPO() {
			fact, ok := result.LocalAssignment(point)
			if !ok || fact.Name != "stale_path" {
				continue
			}
			if p, ok := result.ExpressionPath(fact.Expr); ok {
				pathDebug = p.String()
			}
			canMiss = result.MemberReadCanMiss(point, fact.Expr)
			if t, ok := result.DeclaredExpressionTypeAt(point, fact.Expr); ok && t != nil {
				declaredDebug = t.String()
			}
		}
		t.Fatalf("source type = %v, want nilable invalidated member read; expression path=%q canMiss=%v declared=%q", got[0].TypeWithPresence, pathDebug, canMiss, declaredDebug)
	}
}

func TestForEachMissingMemberReadReportsDiscriminantNarrowedStaticRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Dog = {kind: "dog", bark: string}
type Cat = {kind: "cat", meow: string}
type Animal = Dog | Cat

local function speak(a: Animal)
	if a.kind == "dog" then
		local bad = a["meow"]
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	var got []MissingMemberRead
	New(root.FunctionResults()[0]).ForEachMissingMemberRead(func(read MissingMemberRead) bool {
		got = append(got, read)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("missing member reads = %#v, want one", got)
	}
	if got[0].ReadLabel != `a["meow"]` || got[0].MemberName != "meow" {
		t.Fatalf("read = %#v, want a[\"meow\"] / meow", got[0])
	}
	if !strings.Contains(got[0].ReceiverType.String(), "dog") {
		t.Fatalf("receiver type = %s, want narrowed dog receiver", got[0].ReceiverType)
	}
}

func TestForEachMissingMemberReadSkipsOwnedMissingFieldDefaultToNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function build()
	local suite = {name = "alpha"}
	suite.tests = suite.tests or {}
	return suite
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	var got []MissingMemberRead
	New(root.FunctionResults()[0]).ForEachMissingMemberRead(func(read MissingMemberRead) bool {
		got = append(got, read)
		return true
	})
	if len(got) != 0 {
		t.Fatalf("missing member reads = %#v, want owned local missing-field default suppressed", got)
	}
}

func TestForEachMissingMemberReadReportsOwnedUnionArmMissingField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

local function build()
	local tree: TreeNode = { kind = "text", value = "leaf" }
	if tree.kind == "text" then
		local children = tree.children
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	var got []MissingMemberRead
	New(root.FunctionResults()[0]).ForEachMissingMemberRead(func(read MissingMemberRead) bool {
		got = append(got, read)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("missing member reads = %#v, want union-arm children miss", got)
	}
	if got[0].ReadLabel != "tree.children" || got[0].MemberName != "children" {
		t.Fatalf("missing member read = %#v, want tree.children", got[0])
	}
}

func TestForEachAssignmentUsesStdlibAssertPostcondition(t *testing.T) {
	reg := standard.Registry()
	checked, err := program.RunChunk(parseChunk(t, `
function f(x: string?)
	assert(x)
	local y: string = x
end
`), program.Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    []string{"assert"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	result := root.FunctionResults()[0]
	reader := New(result)
	var got []Assignment
	reader.ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "y" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("y assignments = %#v, want one assignment after assert", got)
	}
	if got[0].TypeWithPresence == nil || typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want assert postcondition to remove nil", got[0].TypeWithPresence)
	}
	if !got[0].Check.Admissible {
		t.Fatalf("assignment check is not admissible with source=%v expected=%v", got[0].TypeWithPresence, got[0].Expected)
	}
}

func TestForEachAssignmentUsesNarrowedMemberContainerBeforeDeclaredType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type TextNode = { kind: "text", value: string }
type GroupNode = { kind: "group", children: {TreeNode} }
type TreeNode = TextNode | GroupNode

function f(tree: TreeNode)
    if tree.kind == "group" then
        local first = tree.children[1]
        if first and first.kind == "text" then
            local value: string = first.value
            local bad_value: number = first.value
        end
    end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	result := root.FunctionResults()[0]
	reader := New(result)
	var value, badValue []Assignment
	reader.ForEachAssignment(func(assignment Assignment) bool {
		switch assignment.TargetLabel {
		case "value":
			value = append(value, assignment)
		case "bad_value":
			badValue = append(badValue, assignment)
		}
		return true
	})
	if len(value) != 1 || !value[0].Check.Admissible {
		t.Fatalf("value assignments = %#v, want narrowed first.value accepted", value)
	}
	if value[0].TypeWithPresence == nil || typevalue.TypeIncludesNil(value[0].TypeWithPresence) {
		t.Fatalf("value source type = %v, want non-nil string", value[0].TypeWithPresence)
	}
	if len(badValue) != 1 || badValue[0].Check.Admissible {
		t.Fatalf("bad_value assignments = %#v, want string-to-number mismatch", badValue)
	}
	if badValue[0].TypeWithPresence == nil || typevalue.TypeIncludesNil(badValue[0].TypeWithPresence) {
		t.Fatalf("bad_value source type = %v, want non-nil string mismatch", badValue[0].TypeWithPresence)
	}
}

func TestForEachAssignmentPreservesLoopCarriedRecordInObjectLiteralEntry(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Usage = { input_tokens: number, output_tokens: number }
type DoneEvent = { type: "done", usage: Usage? }
type OtherEvent = { type: "delta", text: string }
type Event = DoneEvent | OtherEvent
type StreamResult = { usage: Usage }

local function process(events: {Event}): StreamResult
    local usage: Usage = { input_tokens = 0, output_tokens = 0 }
    for _, event in ipairs(events) do
        if event.type == "done" then
            if event.usage then
                usage = event.usage
            end
        end
    end
    local result: StreamResult = {
        usage = usage,
    }
    return result
end
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var resultUsage []Assignment
	New(root.FunctionResults()[0]).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "result.usage" {
			resultUsage = append(resultUsage, assignment)
		}
		return true
	})
	if len(resultUsage) != 0 {
		canonical, canonicalOK := New(root.FunctionResults()[0]).ValueTypeWithPresence(resultUsage[0].Value)
		t.Fatalf("result.usage assignments = %#v, canonical source type = %v/%v, want accumulator record accepted", resultUsage, canonical, canonicalOK)
	}
}

func TestOrdinaryAssignmentTargetTypeUsesFunctionParameterAnnotations(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Key = "name" | "count"
type Bag = {name: string, count: number}

local function f(bag: Bag, key: Key): ()
	bag[key] = "bad"
end
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    []string{"pairs", "tostring"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	child := root.FunctionResults()[0]
	reader := New(child)
	var found bool
	for _, point := range child.Graph().RPO() {
		fact, ok := child.OrdinaryAssignment(point)
		if !ok || body.AssignmentSourceLabel(fact.Target) != "bag[key]" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for parameter dynamic write")
		}
		if !strings.Contains(got.String(), "number") || !strings.Contains(got.String(), "string") {
			t.Fatalf("target type = %v, want meet of Bag fields", got)
		}
		value, valueOK := reader.ordinaryAssignmentSourceValue(point, fact)
		if !valueOK {
			t.Fatal("SourceValue returned false for dynamic-write source")
		}
		sourceType, sourceOK := reader.ValueTypeWithPresence(value)
		if !sourceOK {
			t.Fatal("ValueTypeWithPresence returned false for dynamic-write source")
		}
		if sourceType == nil || !subtype.IsSubtype(sourceType, typ.String) {
			t.Fatalf("source type = %v, want string-like source", sourceType)
		}
		if reader.ValueProofAdmissible(value, got) {
			t.Fatalf("ValueProofAdmissible accepted %v for %v", sourceType, got)
		}
	}
	if !found {
		t.Fatal("missing bag[key] ordinary assignment")
	}
}

func TestOrdinaryAssignmentTargetTypeUsesPairsClosedRecordKeys(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	reader := New(root)
	var found bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || body.AssignmentSourceLabel(fact.Target) != "item[key]" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for pairs dynamic write")
		}
		if !strings.Contains(got.String(), "1") || !strings.Contains(got.String(), `"ready"`) {
			t.Fatalf("target type = %v, want meet of closed record literal fields", got)
		}
		value, valueOK := reader.ordinaryAssignmentSourceValue(point, fact)
		if !valueOK {
			t.Fatal("SourceValue returned false for pairs dynamic-write source")
		}
		sourceType, sourceOK := reader.ValueTypeWithPresence(value)
		if !sourceOK {
			t.Fatal("ValueTypeWithPresence returned false for dynamic-write source")
		}
		if sourceType == nil || !subtype.IsSubtype(sourceType, typ.String) {
			t.Fatalf("source type = %v, want string-like source", sourceType)
		}
		if reader.ValueProofAdmissible(value, got) {
			t.Fatalf("ValueProofAdmissible accepted %v for %v", sourceType, got)
		}
	}
	if !found {
		t.Fatal("missing item[key] ordinary assignment")
	}
}

func TestForEachAssignmentUsesLocalFunctionInsertedArrayReturnSlot(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function group_by_suite(entries: {Entry})
	local no_suite = {}
	for _, entry in ipairs(entries) do
		table.insert(no_suite, entry)
	end
	return no_suite
end

local entries: {Entry} = {}
local no_suite = group_by_suite(entries)
local uncategorized: {Entry} = no_suite
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry:   reg,
			Globals:    []string{"ipairs", "table"},
			Signatures: signaturelookup.Source{IncludeStdlib: true},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	var got []Assignment
	New(root).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "uncategorized" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("uncategorized assignments = %#v, want one", got)
	}
	if !got[0].Check.Admissible {
		t.Fatalf("assignment check is not admissible with source=%v expected=%v", got[0].TypeWithPresence, got[0].Expected)
	}
	if got[0].TypeWithPresence == nil || !strings.Contains(got[0].TypeWithPresence.String(), "id: string") {
		t.Fatalf("source type = %v, want returned Entry array", got[0].TypeWithPresence)
	}
}

func TestExplicitTopWitnessIsNotStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural witness")
	}
}

func TestGradualTopWitnessIsNotAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	reader := New(result)

	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted gradual-top scalar witness")
	}
	if reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible accepted gradual-top scalar witness")
	}
}

func TestAnyClaimWitnessIsNotAdmissibilityProofWithoutRuntimeValidation(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted any-origin scalar witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted any-origin scalar witness")
	}
}

func TestGradualTopRuntimeProofIsAdmissible(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if !reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible rejected gradual-top runtime proof")
	}
	if !reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible rejected gradual-top runtime proof")
	}
}

func TestExplicitTopWitnessWithoutTypeClaimIsNotStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted unclaimed explicit-top structural witness")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted unclaimed explicit-top structural witness")
	}
}

func TestExplicitTopTypeClaimWitnessIsNotStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Type())
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural TypeClaim")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural TypeClaim")
	}
}

func TestExplicitTopTypeClaimWithAnyOriginIsNotStructuralAdmissibilityProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.AnyClaim))
	reader := New(result)

	if reader.ValueAdmissible(value, record) {
		t.Fatalf("ValueAdmissible accepted explicit-top structural TypeClaim with any origin")
	}
	if reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top structural TypeClaim with any origin")
	}
}

func TestExplicitTopRuntimeProofIsAdmissibleForStructuralContract(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	value := typevalue.WithWitness(reg, presentValue(reg), record)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	reader := New(result)

	if !reader.ValueProofAdmissible(value, record) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top structural runtime proof")
	}
}

func TestExplicitTopScalarRuntimeKindIsNotAdmissibleProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	reader := New(result)

	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted explicit-top scalar runtime-kind fact")
	}
	if reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top scalar runtime-kind fact")
	}
}

func TestRuntimeTableKindDoesNotProveStringKeyMapShape(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)
	want := typetable.NewMap(typ.String, typ.Any)

	if reader.ValueAdmissible(value, want) {
		t.Fatalf("ValueAdmissible accepted runtime table kind as string-key map proof")
	}
	if reader.ValueProofAdmissible(value, want) {
		t.Fatalf("ValueProofAdmissible accepted runtime table kind as string-key map proof")
	}
}

func TestRuntimeTableKindDoesNotProveArrayShape(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)
	want := typ.NewArray(typ.String)

	if reader.ValueAdmissible(value, want) {
		t.Fatalf("ValueAdmissible accepted runtime table kind as array proof")
	}
	if reader.ValueProofAdmissible(value, want) {
		t.Fatalf("ValueProofAdmissible accepted runtime table kind as array proof")
	}
}

func TestExplicitTopScalarWitnessWithoutTypeClaimIsNotAdmissibleProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.MaterializeOptional(typ.String)), typ.MaterializeOptional(typ.String))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	reader := New(result)

	if reader.ValueProofAdmissible(value, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("ValueProofAdmissible accepted explicit-top scalar witness without runtime validation")
	}
}

func TestExplicitTopExactLiteralWitnessIsAdmissibleAsScalarProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("ready")), typ.LiteralString("ready"))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top exact literal witness for string")
	}
}

func TestGradualTopExactLiteralWitnessIsAdmissibleAsScalarProof(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("ready")), typ.LiteralString("ready"))
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected gradual-top exact literal witness for string")
	}
}

func TestExplicitTopScalarTypeClaimIsAdmissibleAsUserAssertion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Type())
	reader := New(result)

	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("ValueHasUntrustedTopOrigin rejected explicit-top scalar TypeClaim")
	}
	if reader.ValueAdmissible(value, typ.String) {
		t.Fatalf("ValueAdmissible accepted explicit-top scalar TypeClaim as trusted proof")
	}
	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top scalar TypeClaim")
	}
}

func TestExplicitTopRuntimeProofIsAdmissibleForScalarContract(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if !reader.ValueProofAdmissible(value, typ.String) {
		t.Fatalf("ValueProofAdmissible rejected explicit-top scalar runtime proof")
	}
}

func TestVariantOriginTypeProjectsStructuralUnion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	okCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("ok")).
		Field("value", typ.Number).
		Build()
	errCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("err")).
		Field("error", typ.String).
		Build()
	union := typeexpr.Union(okCase, errCase)
	value := typevalue.FromType(reg, union)

	got, ok := New(result).VariantOriginType(value)
	if !ok {
		t.Fatalf("VariantOriginType returned false")
	}
	assertSameType(t, got, union)
}

func TestRuntimeKindProjection(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want typ.Type
	}{
		{name: "nil", kind: runtimekind.Nil, want: typ.Nil},
		{name: "boolean", kind: runtimekind.Boolean, want: typ.Boolean},
		{name: "number", kind: runtimekind.Number, want: typ.Number},
		{name: "string", kind: runtimekind.String, want: typ.String},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.ValueType(value)
			if !ok {
				t.Fatalf("ValueType returned false")
			}
			assertSameType(t, got, tc.want)
		})
	}

	for _, tc := range []struct {
		name string
		kind runtimekind.Tag
		want kind.Kind
	}{
		{name: "table", kind: runtimekind.Table, want: kind.Map},
		{name: "function", kind: runtimekind.Function, want: kind.Function},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(tc.kind))
			got, ok := reader.RefineDeclaredType(typ.Unknown, value)
			if !ok {
				t.Fatalf("RefineDeclaredType returned false")
			}
			if got.Kind() != tc.want {
				t.Fatalf("RefineDeclaredType kind = %s, want %s (%v)", got.Kind(), tc.want, got)
			}
		})
	}
}

func TestRefineDeclaredTypeOptionalByPresentEvidence(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)

	got, ok := New(result).RefineDeclaredType(typeexpr.Optional(typ.String), value)
	if !ok {
		t.Fatalf("RefineDeclaredType returned false")
	}
	assertSameType(t, got, typ.String)
}

func TestValueTypeUsesOriginTypeWhenWitnessFamilyDoesNotReplay(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.String).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.String).
		Build()
	union := typeexpr.Union(dog, cat)
	dogFamily, dogCases, ok := variant.OriginOfType(dog)
	if !ok {
		t.Fatal("missing dog origin")
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(dogFamily, dogCases))

	got, ok := New(result).ValueType(value)
	if !ok {
		t.Fatalf("ValueType returned false")
	}
	assertSameType(t, got, dog)
}

func TestSourceTypeReadsCallSourceThroughBoundary(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[2].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	if fact.Source.Kind != sourceprovenance.SourceCall || !fact.Source.HasCallPoint {
		t.Fatalf("local source = %#v, want call source with call point", fact.Source)
	}
	if !fact.Source.Final || !fact.Source.Expanded || fact.Source.Adjusted || fact.Source.OpenTail {
		t.Fatalf("call source shape = final:%v expanded:%v adjusted:%v openTail:%v, want expanded final call source",
			fact.Source.Final, fact.Source.Expanded, fact.Source.Adjusted, fact.Source.OpenTail)
	}

	got, ok := New(result).SourceType(point, fact.Source)
	if !ok {
		t.Fatalf("SourceType returned false")
	}
	if got.Kind() != kind.Record {
		t.Fatalf("SourceType kind = %s, want record (%v)", got.Kind(), got)
	}
}

func TestForEachCallCarriesSyntaxFreeCallAndCalleeSpans(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(value: string): () end
consume("ok")
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []CallSite
	if !New(result).ForEachCall(func(call CallSite) bool {
		got = append(got, call)
		return true
	}) {
		t.Fatal("ForEachCall returned false, want one call")
	}
	if len(got) != 1 {
		t.Fatalf("calls = %d, want one", len(got))
	}
	call := got[0]
	if call.CallSpan.StartLine == 0 || call.CalleeSpan.StartLine == 0 {
		t.Fatalf("call spans = call:%#v callee:%#v, want syntax-free source ranges", call.CallSpan, call.CalleeSpan)
	}
	if call.CallSpan.StartLine > call.CalleeSpan.StartLine ||
		(call.CallSpan.StartLine == call.CalleeSpan.StartLine && call.CallSpan.StartCol > call.CalleeSpan.StartCol) ||
		call.CallSpan.EndLine < call.CalleeSpan.EndLine ||
		(call.CallSpan.EndLine == call.CalleeSpan.EndLine && call.CallSpan.EndCol < call.CalleeSpan.EndCol) {
		t.Fatalf("call span %#v does not cover callee span %#v", call.CallSpan, call.CalleeSpan)
	}
}

func TestForEachConcatOperandReportsDeclaredOptionalAndDynamicIndexRisk(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local maybe: string? = nil
local label = "prefix:" .. maybe
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []ConcatOperand
	New(result).ForEachConcatOperand(func(operand ConcatOperand) bool {
		got = append(got, operand)
		return true
	})
	fnResult, err := body.CheckFunction(parseFunction(t, `
function item(arr: {string}, i: number): string
	return "item:" .. arr[i]
end
`), body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	New(fnResult).ForEachConcatOperand(func(operand ConcatOperand) bool {
		got = append(got, operand)
		return true
	})
	if len(got) != 2 {
		t.Fatalf("concat operands = %d, want maybe and arr[i]: %#v", len(got), got)
	}
	labels := map[string]typ.Type{}
	for _, operand := range got {
		if !operand.NilRisk() {
			t.Fatalf("operand %#v did not carry nil risk", operand)
		}
		labels[operand.OperandLabel] = operand.TypeWithPresence
	}
	if _, ok := labels["maybe"]; !ok {
		t.Fatalf("labels = %#v, want maybe", labels)
	}
	if _, ok := labels["arr[i]"]; !ok {
		t.Fatalf("labels = %#v, want arr[i]", labels)
	}
}

func TestForEachConcatOperandDoesNotTreatFlatStringDiscriminantAsSiblingProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ContentPart = {
	type: string,
	text: string?,
}

local function merge(part: ContentPart): ()
	if part.type == "text" then
		local text = "" .. part.text
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var got []ConcatOperand
	New(root.FunctionResults()[0]).ForEachConcatOperand(func(operand ConcatOperand) bool {
		got = append(got, operand)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("concat operands = %d, want part.text nil-risk operand: %#v", len(got), got)
	}
	if got[0].OperandLabel != "part.text" || !got[0].NilRisk() {
		t.Fatalf("concat operand = %#v, want nil-risk part.text", got[0])
	}
}

func TestForEachConcatOperandDoesNotTreatConstantPathDiscriminantAsSiblingProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ContentPart = {
	type: string,
	text: string?,
}

local prompt = {
	CONTENT_TYPE = {
		TEXT = "text",
	},
}

local function merge(part: ContentPart): ()
	if part.type == prompt.CONTENT_TYPE.TEXT then
		local text = "" .. part.text
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var got []ConcatOperand
	New(root.FunctionResults()[0]).ForEachConcatOperand(func(operand ConcatOperand) bool {
		got = append(got, operand)
		return true
	})
	if len(got) != 1 {
		t.Fatalf("concat operands = %d, want part.text nil-risk operand: %#v", len(got), got)
	}
	if got[0].OperandLabel != "part.text" || !got[0].NilRisk() {
		t.Fatalf("concat operand = %#v, want nil-risk part.text", got[0])
	}
}

func TestForEachConcatOperandReportsDynamicMemberAndSiblingAfterFlatDiscriminant(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ContentPart = {
	type: string,
	text: string?,
}

local prompt = {
	CONTENT_TYPE = {
		TEXT = "text",
	},
}

local function merge(last: {content: {ContentPart}}, part: ContentPart): ()
	if part.type == prompt.CONTENT_TYPE.TEXT and
		last.content[#last.content] and
		last.content[#last.content].type == prompt.CONTENT_TYPE.TEXT then
		last.content[#last.content].text = last.content[#last.content].text .. "\n\n" .. part.text
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("root/functions = %#v, want one function result", root)
	}
	var got []ConcatOperand
	New(root.FunctionResults()[0]).ForEachConcatOperand(func(operand ConcatOperand) bool {
		got = append(got, operand)
		return true
	})
	labels := map[string]bool{}
	for _, operand := range got {
		if !operand.NilRisk() {
			t.Fatalf("concat operand = %#v, want nil risk", operand)
		}
		labels[operand.OperandLabel] = true
	}
	if !labels["last.content[#last.content].text"] || !labels["part.text"] {
		t.Fatalf("concat labels = %#v, want last.content[#last.content].text and part.text; operands=%#v", labels, got)
	}
}

func TestForEachCallReportsArityFromCanonicalContract(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("add", signature.Function{
		Type: typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build(),
	})
	result, err := body.CheckFunction(parseFunction(t, `
function f()
	add(1)
	add(1, 2, 3)
end
`), body.Config{
		Registry: reg,
		Globals:  []string{"add"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var arities []CallArityReport
	New(result).ForEachCall(func(call CallSite) bool {
		if call.Arity.Kind != readapi.CallArityReportNone {
			arities = append(arities, call.Arity)
		}
		return true
	})
	if len(arities) != 2 {
		t.Fatalf("arity reports = %d, want two: %#v", len(arities), arities)
	}
	if arities[0].Kind != readapi.CallArityReportTooFew || arities[0].ExpectedCount != 2 || arities[0].ActualCount != 1 {
		t.Fatalf("too-few report = %#v, want expected 2 actual 1", arities[0])
	}
	if arities[0].CallableName != "add" || arities[0].CallSpan.StartLine == 0 {
		t.Fatalf("too-few report anchors = %#v, want callable name and call source span", arities[0])
	}
	if arities[1].Kind != readapi.CallArityReportTooMany || arities[1].ExpectedCount != 2 || arities[1].ActualCount != 3 {
		t.Fatalf("too-many report = %#v, want expected 2 actual 3", arities[1])
	}
	if arities[1].ExtraSpan.StartLine == 0 {
		t.Fatalf("too-many extra span = %#v, want syntax-free extra-argument span", arities[1].ExtraSpan)
	}
}

func TestForEachCallReportsUntrustedOrDefaultArgument(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("need_string", signature.Function{
		Type: typ.Func().Param("value", typ.String).Build(),
	})
	result, err := body.CheckFunction(parseFunction(t, `
function f(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	need_string(url)
end
`), body.Config{
		Registry: reg,
		Globals:  []string{"need_string"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	var reports []CallArgumentReport
	New(result).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one untrusted or-default argument report", reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want missing proof for untrusted or-default", reports[0].Argument)
	}
	if !reports[0].Argument.UntrustedTopOrigin {
		t.Fatalf("argument = %#v, want untrusted-top origin preserved", reports[0].Argument)
	}
}

func TestForEachCallKeepsDifferentContractAfterInvalidDeclaration(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_integer(value: integer): ()
end

local function f(raw: any): ()
	local value: string = raw
	need_integer(value)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}
	var reports []CallArgumentReport
	var calls int
	for _, fn := range result.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			calls++
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("calls = %d argument reports = %#v, want integer contract report after invalid string declaration", calls, reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible: %#v", reports[0])
	}
	if !typ.TypeEquals(reports[0].Check.Expected, typ.Integer) {
		t.Fatalf("expected = %v, want integer", reports[0].Check.Expected)
	}
}

func TestForEachCallReportsDottedMemberFunctionContract(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: (model_id: string, payload: number) -> ()}
function f(c: Client)
	c.invoke(42)
	c.invoke("ok", 1, true)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallArgumentReport
	var arities []CallArityReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		if call.Arity.Kind != readapi.CallArityReportNone {
			arities = append(arities, call.Arity)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one dotted member argument report", reports)
	}
	if reports[0].Argument.Index != 0 || !typ.TypeEquals(reports[0].Check.Expected, typ.String) {
		t.Fatalf("argument report = %#v, want argument 1 expected string", reports[0])
	}
	if len(arities) != 2 {
		t.Fatalf("arity reports = %#v, want dotted member too-few and too-many reports", arities)
	}
	if arities[0].Kind != readapi.CallArityReportTooFew || arities[0].ExpectedCount != 2 || arities[0].ActualCount != 1 {
		t.Fatalf("first arity report = %#v, want too-few expected 2 actual 1", arities[0])
	}
	if arities[1].Kind != readapi.CallArityReportTooMany || arities[1].ExpectedCount != 2 || arities[1].ActualCount != 3 {
		t.Fatalf("second arity report = %#v, want too-many expected 2 actual 3", arities[1])
	}
}

func TestForEachCallDoesNotBindDottedReceiverAsSelf(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ClientSelf = {id: string}
type Client = {id: string, invoke: (self: ClientSelf, model_id: string) -> ()}
function f(c: Client)
	c.invoke("model")
	c:invoke("model")
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var arities []CallArityReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Arity.Kind != readapi.CallArityReportNone {
			arities = append(arities, call.Arity)
		}
		return true
	})
	if len(arities) != 1 {
		t.Fatalf("arity reports = %#v, want only dotted call to be too few", arities)
	}
	if arities[0].Kind != readapi.CallArityReportTooFew || arities[0].ExpectedCount != 2 || arities[0].ActualCount != 1 {
		t.Fatalf("arity report = %#v, want dotted call expected 2 actual 1", arities[0])
	}
}

func TestForEachCallReportsNonCallableMemberCalleeWithoutSignature(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: number}
function f(c: Client)
	c.invoke()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportNotCallable || !reports[0].MemberAccess || !typ.TypeEquals(reports[0].Type, typ.Number) {
		t.Fatalf("callee report = %#v, want non-callable member number", reports[0])
	}
}

func TestForEachCallReportsAnyMemberCalleeNeedsCallableProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
function f(config: {context_merger: any})
	config.context_merger({}, {}, {})
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportNotCallable || !reports[0].MemberAccess || !typ.IsAny(reports[0].Type) {
		t.Fatalf("callee report = %#v, want member any callable-proof report", reports[0])
	}
}

func TestForEachCallUsesReceiverContractBeforeImpreciseMemberProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type MessageChannel = Channel<string>
function f(raw: any)
	local inbox = raw as MessageChannel
	inbox:case_receive()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 0 {
		t.Fatalf("callee reports = %#v, want none because receiver contract provides case_receive", reports)
	}
}

func TestForEachCallLeavesDynamicReceiverAnyMemberCalleeGradual(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
function f(provider: any)
	provider.meta()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 0 {
		t.Fatalf("callee reports = %#v, want dynamic receiver to stay gradual", reports)
	}
}

func TestForEachCallLeavesDeclaredAnyLocalMethodReceiverGradual(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
function f()
	local router: any = {}
	router:on("commit", function() end)
	router:dispatch({})
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 0 {
		t.Fatalf("callee reports = %#v, want declared any receiver to stay gradual", reports)
	}
}

func TestForEachCallReportsOptionalCallableDotMemberCallee(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: (() -> ())?}
function f(c: Client)
	c.invoke()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want optional callable member report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMayBeNil || !reports[0].MemberAccess || !reports[0].Callable {
		t.Fatalf("callee report = %#v, want optional callable member report", reports[0])
	}
}

func TestForEachCallReportsOptionalMethodReceiver(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: (self: Client) -> ()}
function f(c: Client?)
	c:invoke()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want optional method receiver report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMayBeNil || !reports[0].MemberAccess || reports[0].Callable {
		t.Fatalf("callee report = %#v, want optional receiver member report", reports[0])
	}
}

func TestForEachCallReportsOptionalExpressionMethodReceiver(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Message = {topic: (self: Message) -> string}
local function make(): {Message}
	return {}
end
local _: string = make()[1]:topic()
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatal("missing root result")
	}
	var reports []CallCalleeReport
	New(result).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want optional expression receiver report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMayBeNil || !reports[0].MemberAccess || reports[0].Callable {
		t.Fatalf("callee report = %#v, want optional expression receiver report", reports[0])
	}
}

func TestForEachCallLeavesEmptyClosedRecordMissingMemberToMemberProducer(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {}
function f(c: Client)
	c.invoke()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 0 {
		t.Fatalf("callee reports = %#v, want empty record left to member-shape producer", reports)
	}
}

func TestForEachCallReportsMissingMemberCalleeForClosedRecordWithShape(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {id: string}
function f(c: Client)
	c.invoke()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one missing-member report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMissingMember || reports[0].MemberName != "invoke" || reports[0].CallableName != "c.invoke" {
		t.Fatalf("callee report = %#v, want missing member c.invoke", reports[0])
	}
}

func TestForEachCallReportsMissingStaticIntMemberCallee(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {[1]: () -> ()}
function f(c: Client)
	c[2]()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one missing static-int member report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMissingMember || reports[0].MemberName != "[2]" || reports[0].CallableName != "c[2]" {
		t.Fatalf("callee report = %#v, want missing member c[2]", reports[0])
	}
}

func TestForEachCallReportsUnionReceiverMissingMemberCallee(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
function f(x: string | number)
	x:upper()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one union receiver missing-member report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMissingMember || reports[0].MemberName != "upper" || reports[0].CallableName != "x.upper" {
		t.Fatalf("callee report = %#v, want missing member x.upper", reports[0])
	}
}

func TestForEachCallReportsDiscriminantNarrowedMissingMemberCallee(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Dog = {kind: "dog", bark: () -> ()}
type Cat = {kind: "cat", meow: () -> ()}
type Animal = Dog | Cat

function speak(a: Animal)
	if a.kind == "dog" then
		a.meow()
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallCalleeReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want one discriminant-narrowed missing-member report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMissingMember || reports[0].MemberName != "meow" || reports[0].CallableName != "a.meow" {
		t.Fatalf("callee report = %#v, want missing member a.meow", reports[0])
	}
}

func TestForEachCallSkipsOptionalCallableMemberAfterPresenceGuard(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: (() -> ())?}
function f(c: Client)
	if c.invoke then
		c.invoke()
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			t.Fatalf("callee report = %#v, want presence guard to remove member nilability", call.Callee)
		}
		return true
	})
}

func TestForEachCallAcceptsUnionMemberCalleeWhenAllArmsCallable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Left = {run: () -> string}
type Right = {run: () -> number}
function f(x: Left | Right)
	x:run()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			t.Fatalf("callee report = %#v, want none for all-callable union", call.Callee)
		}
		return true
	})
}

func TestForEachCallReportsDirectCalleeCallableMismatches(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local x: number = 42
x()
local maybe: (() -> string)? = nil
maybe()
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var reports []CallCalleeReport
	New(result).ForEachCall(func(call CallSite) bool {
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			reports = append(reports, call.Callee)
		}
		return true
	})
	if len(reports) != 2 {
		t.Fatalf("callee reports = %d, want two: %#v", len(reports), reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportNotCallable || reports[0].CallableName != "x" || !typ.TypeEquals(reports[0].Type, typ.Number) {
		t.Fatalf("first callee report = %#v, want x not-callable number", reports[0])
	}
	if reports[0].Span.StartLine == 0 {
		t.Fatalf("first callee span = %#v, want source anchor", reports[0].Span)
	}
	if reports[1].Kind != readapi.CallCalleeReportMayBeNil || reports[1].CallableName != "maybe" || !reports[1].Callable {
		t.Fatalf("second callee report = %#v, want maybe possibly-nil callable", reports[1])
	}
}

func TestSourceTypePrefersLocalAssignmentLoweredCallSource(t *testing.T) {
	reg := standard.Registry()
	exportType := typetable.NewRecord().Field("run", typ.Func().Build()).Build()
	m := manifest.New("pkg")
	m.SetExport(exportType)
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: typ.Func().Build()})
	stmts := parseChunk(t, `
local pkg: {run: () -> ()}? = require("pkg")
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
			Manifests:     []*manifest.Manifest{m},
		},
		ModuleExports: importlookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	assign := stmts[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	if fact.Source.Kind != sourceprovenance.SourceCall {
		t.Fatalf("local source = %#v, want call source", fact.Source)
	}
	got, ok := New(result).SourceType(point, fact.Source)
	if !ok || !typ.TypeEquals(got, exportType) {
		t.Fatalf("SourceType = %v/%v, want manifest export %v", got, ok, exportType)
	}
}

func TestUntrustedRuntimeTableWitnessIsNotProvenMismatch(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	value := typevalue.WithWitness(reg, presentValue(reg), typetable.BuiltinTopMarker())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Runtime())
	reader := New(result)

	if reader.ValueWitnessProvenMismatch(value, typ.NewArray(typ.String)) {
		t.Fatalf("ValueWitnessProvenMismatch treated untrusted runtime table as proven array mismatch")
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("ValueProofAdmissible accepted untrusted runtime table as array shape")
	}
}

func TestSourceValueReadsGuardedPathExpressionFromSolvedPath(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local block: any = {}
if type(block.items) == "table" then
    local labels: {string} = block.items
end
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		Globals:  []string{"type"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	point, fact := requireLocalAssignmentByName(t, result, "labels")
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false for guarded path expression")
	}
	got, ok := reader.ValueType(value)
	if !ok || !typetable.IsBuiltinTopMarker(got) {
		t.Fatalf("SourceValue type = %v/%v, want table runtime-kind projection", got, ok)
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("SourceValue proof accepted table runtime kind as array shape")
	}
}

func TestSourceValueReadsGuardedAnyParamPathAsUntrusted(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function rows(block: any): {string}?
    if type(block.items) == "table" then
        local labels: {string} = block.items
        return labels
    end
    return nil
end
`)
	result, err := body.CheckFunction(fn, body.Config{
		Registry:   reg,
		Globals:    []string{"type"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	assign := ifStmt.Then[0].(*ast.LocalAssignStmt)
	point, fact := requireLocalAssignment(t, result, assign, 0)
	reader := New(result)

	value, ok := reader.SourceValue(point, fact.Source)
	if !ok {
		t.Fatalf("SourceValue returned false for guarded any-param path")
	}
	got, ok := reader.ValueType(value)
	if !ok || !typetable.IsBuiltinTopMarker(got) {
		t.Fatalf("SourceValue type = %v/%v, want table runtime-kind projection", got, ok)
	}
	if !reader.ValueHasUntrustedTopOrigin(value) {
		t.Fatalf("SourceValue did not preserve untrusted any origin for guarded any-param path")
	}
	if reader.ValueProofAdmissible(value, typ.NewArray(typ.String)) {
		t.Fatalf("SourceValue proof accepted table runtime kind as array shape")
	}
}

func assertSameType(t *testing.T, got, want typ.Type) {
	t.Helper()
	if !typ.SameNodeOrAcyclicEqual(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func requireLocalAssignment(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) (cfg.Point, semantics.LocalAssignmentFact) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point, fact
		}
	}
	t.Fatalf("missing local assignment for stmt %p index %d", stmt, index)
	return 0, semantics.LocalAssignmentFact{}
}

func requireLocalAssignmentByName(t *testing.T, result *body.Result, name string) (cfg.Point, semantics.LocalAssignmentFact) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Name == name {
			return point, fact
		}
	}
	t.Fatalf("missing local assignment named %q", name)
	return 0, semantics.LocalAssignmentFact{}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(strings.TrimSpace(src), "readmodel_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func parseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := parseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want one function definition", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func TestRuntimeKindReducedTypeNarrowsByRuntimeKindExclusion(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckChunk(nil, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	declared := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})

	// A runtime kind that excludes Number narrows number | string to string.
	excluded := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	excluded = product.Set(reg, excluded, runtimekind.Key, runtimekind.Top().Without(runtimekind.Number))
	got, ok := reader.RuntimeKindReducedType(excluded, declared)
	if !ok {
		t.Fatalf("RuntimeKindReducedType returned false, want string")
	}
	assertSameType(t, got, typ.String)

	// A top runtime kind imposes no constraint, so nothing is narrowed.
	top := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if narrowed, ok := reader.RuntimeKindReducedType(top, declared); ok {
		t.Fatalf("RuntimeKindReducedType narrowed under top runtime kind: got %v", narrowed)
	}

	// A non-union declared type cannot have a single kind subtracted.
	if narrowed, ok := reader.RuntimeKindReducedType(excluded, typ.String); ok {
		t.Fatalf("RuntimeKindReducedType narrowed a non-union type: got %v", narrowed)
	}

	tableUnion := typ.MaterializeUnion([]typ.Type{typ.String, typetable.BuiltinTopMarker()})
	withoutTable := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	withoutTable = product.Set(reg, withoutTable, runtimekind.Key, runtimekind.Top().Without(runtimekind.Table))
	got, ok = reader.RuntimeKindReducedType(withoutTable, tableUnion)
	if !ok {
		t.Fatalf("RuntimeKindReducedType returned false for string | table, want string")
	}
	assertSameType(t, got, typ.String)
}
