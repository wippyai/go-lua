package readmodel

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
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
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/projection"
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
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
		Globals:  []string{"value"},
	}})
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
	want := typetable.NewRecord().
		Field("id", typ.LiteralString("r1")).
		Field("retries", typ.LiteralInt(2)).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("SourceType = %v/%v, want %v", got, ok, want)
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

func TestForEachAssignmentReportsRuntimeTableAppendToOptionalFieldRecordArray(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type BindingSpec = {
	id: string?,
	priority: number?,
}

local function normalize(binding: any): {BindingSpec}
	local normalized: {BindingSpec} = {}
	if type(binding) == "table" then
		normalized[#normalized + 1] = binding
	end
	return normalized
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	child := root.FunctionResults()[0]
	reader := New(child)
	var got []Assignment
	reader.ForEachAssignment(func(assignment Assignment) bool {
		if assignment.SourceLabel == "binding" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("dynamic append assignments = %#v, want one binding append assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want runtime table rejected for BindingSpec element contract")
	}
	if got[0].Expected == nil || !strings.Contains(got[0].Expected.String(), "id") || !strings.Contains(got[0].Expected.String(), "priority") {
		t.Fatalf("expected = %v, want BindingSpec element contract", got[0].Expected)
	}
}

func TestForEachAssignmentReportsRuntimeTableAppendFromDefaultedIterator(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type BindingSpec = {
	id: string?,
	priority: number?,
}

local function normalize(bindings: any): {BindingSpec}
	local normalized: {BindingSpec} = {}
	for _, binding in ipairs(bindings or {}) do
		if type(binding) == "table" then
			normalized[#normalized + 1] = binding
		end
	end
	return normalized
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one child", root)
	}
	child := root.FunctionResults()[0]
	reader := New(child)
	var got []Assignment
	reader.ForEachAssignment(func(assignment Assignment) bool {
		if assignment.SourceLabel == "binding" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("dynamic append assignments = %#v, want one binding append assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want runtime table rejected for BindingSpec element contract")
	}
	if got[0].Expected == nil || !strings.Contains(got[0].Expected.String(), "id") || !strings.Contains(got[0].Expected.String(), "priority") {
		t.Fatalf("expected = %v, want BindingSpec element contract", got[0].Expected)
	}
}

func TestForEachAssignmentAcceptsInferredRecordFieldTableReplacement(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local caller = {}
caller.__index = caller

function caller.new(): any
	local self = setmetatable({}, caller)
	self.wrapper_context = {}
	return self
end

function caller:set_wrapper_context(context: table?): any
	self.wrapper_context = context or {}
	return self
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
		Globals:  []string{"setmetatable"},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var assignments []Assignment
	for _, child := range root.FunctionResults() {
		New(child).ForEachAssignment(func(assignment Assignment) bool {
			if assignment.TargetLabel == "self.wrapper_context" {
				assignments = append(assignments, assignment)
			}
			return true
		})
	}
	if len(assignments) != 0 {
		t.Fatalf("self.wrapper_context assignments = %#v, want inferred table-like field replacement accepted", assignments)
	}
}

func TestForEachAssignmentAcceptsLocalExclusiveInferredRecordReplacement(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local root = { api = {} }
function root.api.send(v: number): () end

root.api = {
	send = function(v: string): () end,
}
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var assignments []Assignment
	New(root).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "root.api" {
			assignments = append(assignments, assignment)
		}
		return true
	})
	if len(assignments) != 0 {
		t.Fatalf("root.api assignments = %#v, want local exclusive inferred record replacement accepted", assignments)
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

func TestForEachAssignmentReportsExplicitAnyFieldThroughIPairs(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local raw: any = nil
local pages = {
	{ id = raw, route = "/ok" },
}
local accessible: {[string]: string} = {}
for _, page in ipairs(pages) do
	accessible[page.route] = page.id
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var got []Assignment
	New(root).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "accessible" && assignment.SourceLabel == "page.id" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("assignments = %#v, want explicit-any map write rejected", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible, want explicit any field rejected for map value contract")
	}
	if !got[0].UntrustedTopOrigin {
		t.Fatalf("assignment did not preserve explicit any origin through ipairs")
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
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
		Globals:  []string{"value"},
	}})
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

func TestForEachAssignmentKeepsNonDominatingWrapperReturnMemberNilable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function run(flag: boolean)
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

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.run()
	local answer: string = res.answer
	return answer
end

return run
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult missing")
	}
	var got []Assignment
	for _, fn := range root.FunctionResults() {
		New(fn).ForEachAssignment(func(assignment Assignment) bool {
			if assignment.TargetLabel == "answer" {
				got = append(got, assignment)
			}
			return true
		})
	}
	if len(got) != 1 {
		t.Fatalf("answer assignments = %#v, want one wrapper return member assignment", got)
	}
	if got[0].Check.Admissible {
		t.Fatalf("assignment check is admissible with source=%v expected=%v, want non-dominating wrapper return member to require nil proof", got[0].TypeWithPresence, got[0].Expected)
	}
	if !typevalue.TypeIncludesNil(got[0].TypeWithPresence) {
		t.Fatalf("source type = %v, want nilable wrapper return member", got[0].TypeWithPresence)
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
	if !req.CallResult.Present || req.CallResult.ResultIndex != 0 || req.CallResult.ReturnSpan.StartLine != 1 || req.CallResult.ReturnSpan.StartCol != 32 {
		t.Fatalf("req call result source = %#v, want present result 0 with declared return span", req.CallResult)
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

func TestForEachReturnAcceptsRuntimeCastOnIndexedRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Item = {description: string}
type Context = {items: {Item}}

local function f(context: Context): Item
	return context.items[#context.items] :: Item
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
		t.Fatalf("returns = %#v, want one return projection", returns)
	}
	if ret := returns[0]; !ret.Check.Admissible {
		t.Fatalf("return check = %#v type=%v, want runtime-validated indexed read accepted", ret.Check, ret.TypeWithPresence)
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

func TestForEachReturnAcceptsNestedClosureLogicalBooleanWithCapturedLocals(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local output = {}
output.TYPE = { ERROR = "error" }

type ErrorInfo = { type: string, message: string, code: any? }
type OutputChunk = { type: string, error: ErrorInfo? }
type Streamer = {
	send_error: (self: Streamer, type: string, message: string, code: any?) -> boolean,
}

function output.error(err_type: string, message: string, code: any?): OutputChunk
	return {
		type = output.TYPE.ERROR,
		error = { type = err_type, message = message, code = code },
	}
end

function output.streamer(pid: string, topic: string): Streamer
	local streamer = {}
	local target_pid = tostring(pid)
	local target_topic = tostring(topic)
	streamer.send_error = function(self: Streamer, err_type: string, message: string, code: any?): boolean
		local chunk: OutputChunk = output.error(err_type, message, code)
		return chunk.type == output.TYPE.ERROR and target_pid ~= "" and target_topic ~= ""
	end
	return streamer :: Streamer
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	var returns []Return
	var visit func(*body.Result)
	visit = func(result *body.Result) {
		for _, child := range result.FunctionResults() {
			New(child).ForEachReturn(func(ret Return) bool {
				returns = append(returns, ret)
				return true
			})
			visit(child)
		}
	}
	visit(root)
	if len(returns) != 1 {
		var booleanReturns []Return
		for _, ret := range returns {
			if typ.TypeEquals(ret.Expected, typ.Boolean) {
				booleanReturns = append(booleanReturns, ret)
			}
		}
		returns = booleanReturns
	}
	if len(returns) != 1 {
		t.Fatalf("returns = %d, want nested send_error boolean return: %#v", len(returns), returns)
	}
	ret := returns[0]
	if !ret.Check.Admissible {
		t.Fatalf("return check = %#v type=%v untrusted=%v explicit=%v, want admissible boolean proof",
			ret.Check, ret.TypeWithPresence, ret.UntrustedTopOrigin, ret.ExplicitTopOrigin)
	}
}

func TestForEachReturnProjectsDeclaredObjectLiteralFieldThroughRootCast(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Spec = { id: string, prompt: string }
local function make(raw: any): Spec
	local spec = { id = "agent", prompt = raw.prompt }
	return spec :: Spec
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
	if got, want := ret.ExpectedLabel, "returned value 1.prompt"; got != want {
		t.Fatalf("expected label = %q, want %q", got, want)
	}
	if got, want := ret.SourceLabel, "raw.prompt"; got != want {
		t.Fatalf("source label = %q, want %q", got, want)
	}
	if !ret.UntrustedTopOrigin {
		t.Fatalf("untrusted origin = false, want any-derived field proof")
	}
	if ret.Check.Admissible {
		t.Fatalf("check = %#v, want missing field proof despite root cast", ret.Check)
	}
}

func TestForEachReturnRootCastSkipsMissingFieldsOnCopiedTable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Payload = { host: table, agent: table? }
local function copy(payload: Payload?): Payload
	local out = {}
	for k, v in pairs(payload or {}) do
		out[k] = v
	end
	return out :: Payload
end
`)
	checked, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{Registry: reg, Globals: []string{"pairs"}},
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
	for _, ret := range returns {
		if !ret.Check.Admissible {
			t.Fatalf("returns = %#v, want root cast to validate copied table shape", returns)
		}
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

func TestForEachAssignmentMarksMemberReadThroughIndexedParent(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Tool = { id: string }
type Spec = { tools: {Tool} }
local spec: Spec = { tools = {} }
local first_tool_id: string = spec.tools[1].id
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
		if assignment.TargetLabel == "first_tool_id" {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("first_tool_id assignments = %#v, want one", got)
	}
	if !got[0].SourceIndexedRead {
		t.Fatalf("SourceIndexedRead = false for %q; member reads through indexed parents must carry indexed-read proof reason", got[0].SourceLabel)
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

func TestOrdinaryAssignmentTargetTypeUsesWriteMeetForStaticUnionRecordFields(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Box = {value: number} | {value: string}

local function dot(box: Box): ()
	box.value = "bad"
end

local function bracket(box: Box): ()
	box["value"] = "bad"
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
	if root == nil || len(root.FunctionResults()) != 2 {
		t.Fatalf("function results = %#v, want two children", root)
	}
	var checkedTargets int
	for _, child := range root.FunctionResults() {
		reader := New(child)
		for _, point := range child.Graph().RPO() {
			fact, ok := child.OrdinaryAssignment(point)
			if !ok {
				continue
			}
			label := body.AssignmentSourceLabel(fact.Target)
			if label != "box.value" && label != `box["value"]` {
				continue
			}
			checkedTargets++
			got, ok := reader.ordinaryAssignmentTargetType(point, fact)
			if !ok {
				t.Fatalf("ordinaryAssignmentTargetType returned false for %s", label)
			}
			if subtype.IsSubtype(typ.LiteralString("bad"), got) {
				t.Fatalf("%s target type = %v, want write meet rejecting literal string", label, got)
			}
		}
	}
	if checkedTargets != 2 {
		t.Fatalf("checked static union write targets = %d, want 2", checkedTargets)
	}
}

func TestOrdinaryAssignmentTargetTypeUsesAliasDeclaredContractForDiscriminantWrite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function use(result: Result<string>): string
	local alias = result
	if result.ok then
		alias.ok = false
		return result.value
	end
	return ""
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
		if !ok || body.AssignmentSourceLabel(fact.Target) != "alias.ok" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for alias.ok")
		}
		if !subtype.IsSubtype(typ.False, got) {
			t.Fatalf("target type = %v, want declared discriminant contract accepting false", got)
		}
		if strings.Contains(got.String(), "false & true") {
			t.Fatalf("target type = %v, leaked narrowed branch proof into write contract", got)
		}
	}
	if !found {
		t.Fatal("missing alias.ok ordinary assignment")
	}
}

func TestOrdinaryAssignmentTargetTypeUsesDeclaredRecordForMutableLiteralField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Projection = {
	status: "queued" | "started" | "completed" | "failed",
}

local projection: Projection = {
	status = "queued",
}
projection.status = "started"
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
	if root == nil {
		t.Fatalf("root result is nil")
	}
	reader := New(root)
	var found bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || body.AssignmentSourceLabel(fact.Target) != "projection.status" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for projection.status")
		}
		if !subtype.IsSubtype(typ.LiteralString("started"), got) {
			t.Fatalf("target type = %v, want declared status union accepting \"started\"", got)
		}
	}
	if !found {
		t.Fatal("missing projection.status ordinary assignment")
	}
}

func TestOrdinaryAssignmentTargetTypeWidensMapRecoveredMutableLiteralField(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Projection = {
	status: "queued" | "started" | "completed" | "failed",
}
type State = {
	projections: {[string]: Projection},
}

local state: State = {
	projections = {},
}
local projection = state.projections["task"]
if not projection then
	projection = {
		status = "queued",
	}
	state.projections["task"] = projection
end
projection.status = "started"
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
	if root == nil {
		t.Fatalf("root result is nil")
	}
	reader := New(root)
	var found bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || body.AssignmentSourceLabel(fact.Target) != "projection.status" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for projection.status")
		}
		if !subtype.IsSubtype(typ.LiteralString("started"), got) {
			t.Fatalf("target type = %v, want recovered status union accepting \"started\"", got)
		}
	}
	if !found {
		t.Fatal("missing projection.status ordinary assignment")
	}
}

func TestOrdinaryAssignmentTargetTypePreservesCastWidenedAliasContract(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local narrow: {x: number} = {x = 1}
local wide = narrow as {x: number | string}
wide.x = "boom"
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
	if root == nil {
		t.Fatalf("root result is nil")
	}
	reader := New(root)
	var found bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || body.AssignmentSourceLabel(fact.Target) != "wide.x" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for wide.x")
		}
		if !subtype.IsSubtype(typ.LiteralString("boom"), got) {
			t.Fatalf("target type = %v, want cast-widened union accepting string", got)
		}
	}
	if !found {
		t.Fatal("missing wide.x ordinary assignment")
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

func TestOrdinaryAssignmentTargetTypeUsesDeclaredMapForDynamicWrite(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local headers: {[string]: string} = {
	["content-type"] = "application/json",
	["accept"] = "application/json",
}

local header_name = "x-custom"
headers[tostring(header_name)] = tostring(42)
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
	var labels []string
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok {
			continue
		}
		labels = append(labels, body.AssignmentSourceLabel(fact.Target))
		if body.AssignmentSourceLabel(fact.Target) != "headers" {
			continue
		}
		found = true
		got, ok := reader.ordinaryAssignmentTargetType(point, fact)
		if !ok {
			t.Fatal("ordinaryAssignmentTargetType returned false for declared map dynamic write")
		}
		if !typ.TypeEquals(got, typ.String) {
			t.Fatalf("target type = %v, want declared map value type string", got)
		}
	}
	if !found {
		t.Fatalf("missing headers ordinary assignment; labels=%v", labels)
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

func TestCallArgumentLabelUsesPathBackedSourceWithoutSyntaxLabel(t *testing.T) {
	reg := standard.Registry()
	result, err := body.CheckFunction(parseFunction(t, `
function f(source: {primary: string}): () end
`), body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	fn := result.Function()
	slots := result.FunctionParamSlots(fn)
	if len(slots) != 1 {
		t.Fatalf("param slots = %d, want one", len(slots))
	}
	argPath := pathdom.NewPath(slots[0].Symbol, "source").Field("primary")
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewPathValueSource(argPath.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{source},
	})
	got := New(result).callArgumentLabel(site, 0, source)
	if got != "source.primary" {
		t.Fatalf("callArgumentLabel = %q, want source.primary", got)
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
	reader := New(result)
	reader.ForEachCall(func(call CallSite) bool {
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

func TestForEachCallReportsLocalFunctionUntrustedOrDefaultArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function needs_string(value: string): ()
end

local function from_untrusted(raw: any): ()
	local selected = raw or "fallback"
	needs_string(selected)
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
	for _, fn := range result.FunctionResults() {
		reader := New(fn)
		reader.ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one local-function untrusted or-default argument report", reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want missing proof for untrusted or-default", reports[0].Argument)
	}
}

func TestForEachReturnReportsRootLiteralArrayReadWithoutLengthProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function bad(): number
	local xs: {number} = {}
	return xs[1]
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var returns []Return
	for _, fn := range root.FunctionResults() {
		New(fn).ForEachReturn(func(ret Return) bool {
			returns = append(returns, ret)
			return true
		})
	}
	if len(returns) != 1 {
		t.Fatalf("returns = %#v, want one nilable indexed return", returns)
	}
	if returns[0].Check.Admissible || returns[0].Check.Mismatch.Kind != readapi.ReturnMismatchMayBeNil || !returns[0].SourceIndexedRead {
		t.Fatalf("return = %#v, want non-admissible indexed read may-be-nil mismatch", returns[0])
	}
}

func TestForEachCallReportsBroadNumberTupleIndexMayBeNil(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local frames = {"a", "b", "c"}

local function need_string(value: string): ()
end

local function spinner(index: number): ()
	local frame = frames[((index - 1) % #frames) + 1]
	need_string(frame)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var reports []CallArgumentReport
	for _, fn := range root.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one nilable indexed argument report", reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want broad number tuple index to remain nilable", reports[0].Argument)
	}
	if !typevalue.TypeIncludesNil(reports[0].Argument.TypeWithPresence) {
		t.Fatalf("argument type = %v, want nilable indexed read", reports[0].Argument.TypeWithPresence)
	}
}

func TestForEachCallReportsTruthyUntypedArgumentAsUnknown(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function apply_limit(limit: number): ()
end

local function list(limit)
	if limit then
		apply_limit(limit)
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var reports []CallArgumentReport
	for _, fn := range root.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one untyped truthy argument report", reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want truthy untyped value rejected for number", reports[0].Argument)
	}
	if !typ.IsAny(reports[0].Argument.TypeWithPresence) && !typ.IsUnknown(reports[0].Argument.TypeWithPresence) {
		t.Fatalf("argument type = %v, want any/unknown after truthy guard, not number", reports[0].Argument.TypeWithPresence)
	}
}

func TestForEachCallReportsOptionalTableInsertElement(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Item = {description: string}
type Context = {items: {Item}}

local function f(context: Context, maybe_item: Item?): ()
	table.insert(context.items, maybe_item)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one function", root)
	}
	var reports []CallArgumentReport
	New(root.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want optional table.insert element report", reports)
	}
	if reports[0].Argument.Label != "maybe_item" || reports[0].Check.Admissible {
		t.Fatalf("report = %#v, want non-admissible maybe_item insert", reports[0])
	}
	if !typevalue.TypeIncludesNil(reports[0].Argument.TypeWithPresence) {
		t.Fatalf("argument type = %v, want optional inserted value", reports[0].Argument.TypeWithPresence)
	}
}

func TestForEachCallReportsObjectLiteralExplicitAnyMemberAsTop(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Point = { id: string }
local function take(p: Point): () end
local raw: any = nil
take({ id = raw })
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
	New(result).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one explicit-any member report", reports)
	}
	arg := reports[0].Check.Argument
	if arg.Label != "argument 1.id (raw)" {
		t.Fatalf("argument label = %q, want object-literal member", arg.Label)
	}
	if !arg.UntrustedTopOrigin {
		t.Fatalf("argument = %#v, want untrusted top origin", arg)
	}
	if got := reports[0].Check.EffectiveActualType(); !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("effective actual type = %v, want any; argument=%#v", got, arg)
	}
}

func TestForEachCallRejectsExplicitAnyForLocalFunctionRecordParam(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Payload = { id: string, count: number }
local raw: any = { id = "cfg", count = 2 }
local function consume(payload: Payload): number
	return payload.count + 1
end
local count = consume(raw)
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
	New(result).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want explicit-any local function argument report", reports)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want explicit any to require proof", reports[0].Argument)
	}
	if !reports[0].Argument.UntrustedTopOrigin {
		t.Fatalf("argument = %#v, want explicit top origin", reports[0].Argument)
	}
}

func TestForEachCallRejectsCapturedLocalFunctionOptionalAnyParam(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need(id: string): () end
local function f(raw: any?): ()
	need(raw)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var reports []CallArgumentReport
	var calls int
	var args []CallArgument
	for _, fn := range checked.RootResult().FunctionResults() {
		reader := New(fn)
		reader.ForEachCall(func(call CallSite) bool {
			calls++
			reader.forEachCallArgument(call.Point, func(arg CallArgument) bool {
				args = append(args, arg)
				return true
			})
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("calls=%d args=%#v argument reports = %#v, want untrusted optional-any local function argument report", calls, args, reports)
	}
	arg := reports[0].Check.Argument
	if reports[0].Check.Admissible || !arg.UntrustedTopOrigin {
		t.Fatalf("argument report = %#v, want non-admissible untrusted optional-any", reports[0])
	}
}

func TestForEachCallTableRuntimeGuardDoesNotProveStringKeyMap(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_context(context: {[string]: any}?): ()
end

local function process(inputs: any): ()
	local input_context = nil
	if inputs.context then
		local context_content = inputs.context.content
		if type(context_content) ~= "table" then
			return
		end
		input_context = context_content
	end
	need_context(input_context)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var reports []CallArgumentReport
	for _, fn := range checked.RootResult().FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one table-vs-map report", reports)
	}
	check := reports[0].Check
	wantActual := typ.MaterializeOptional(typetable.BuiltinTopMarker())
	if !typ.TypeEquals(check.Argument.TypeWithPresence, wantActual) {
		t.Fatalf("argument type = %v, want %v; check=%#v", check.Argument.TypeWithPresence, wantActual, check)
	}
	if check.Admissible || !check.ProvenMismatch {
		t.Fatalf("argument check = %#v, want proven table-vs-map mismatch", check)
	}
}

func TestForEachCallRejectsDeclaredAnyRootArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_string(value: string): () end
local raw: any = 1
need_string(raw)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var reports []CallArgumentReport
	reader := New(checked.RootResult())
	reader.ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("reports = %#v, want one declared-any argument report", reports)
	}
	arg := reports[0].Check.Argument
	if !arg.UntrustedTopOrigin || !arg.ExplicitTopOrigin {
		t.Fatalf("argument origins = untrusted:%v explicit:%v type:%v, want declared any boundary",
			arg.UntrustedTopOrigin, arg.ExplicitTopOrigin, arg.TypeWithPresence)
	}
}

func TestForEachCallReportsObjectLiteralLogicalAnyFallbackMemberAsTop(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type TestEntry = {
	id: string,
	name: string,
}

local function collect(entries: any): ()
	local tests: {TestEntry} = {}
	for i, entry in ipairs(entries) do
		local meta = entry.meta or {}
		local display_name = meta.name or ("Unnamed test " .. i)
		table.insert(tests, {
			id = entry.id :: string,
			name = display_name,
		})
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 1 {
		t.Fatalf("function results = %#v, want one", result)
	}
	var reports []CallArgumentReport
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one untrusted logical fallback member report", reports)
	}
	arg := reports[0].Check.Argument
	if arg.Label != "argument 2.name (display_name)" {
		t.Fatalf("argument label = %q, want object-literal member", arg.Label)
	}
	if !arg.UntrustedTopOrigin {
		t.Fatalf("argument = %#v, want untrusted top origin", arg)
	}
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want missing proof", arg)
	}
}

func TestForEachCallKeepsTruthyAnyFieldUntrustedAfterRuntimeKindReduction(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("need_string", signature.Function{
		Type: typ.Func().Param("value", typ.String).Build(),
	})
	stmts := parseChunk(t, `
local function collect(parsed: any): ()
	if type(parsed._images) == "table" then
		for _, img in ipairs(parsed._images) do
			if type(img) == "table" and img.url then
				need_string(img.url)
			end
		end
	end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
		Globals:  []string{"type", "ipairs", "need_string"},
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
			Manifests:     []*manifest.Manifest{m},
		},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var reports []CallArgumentReport
	for _, result := range root.FunctionResults() {
		New(result).ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("argument reports = %#v, want one untrusted img.url argument report", reports)
	}
	arg := reports[0].Argument
	if reports[0].Check.Admissible {
		t.Fatalf("argument report is admissible with arg=%#v, want missing proof for untrusted img.url", arg)
	}
	if !arg.UntrustedTopOrigin {
		t.Fatalf("argument = %#v, want untrusted-top origin preserved after runtime-kind reduction", arg)
	}
	if !typ.IsAny(arg.TypeWithPresence) {
		t.Fatalf("argument type = %v, want any with untrusted proof origin after truthy field guard", arg.TypeWithPresence)
	}
}

func TestForEachCallAcceptsGuardedConcreteDynamicReadFromTypedAnyMap(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ActiveSession = {
	pid: any,
	created_at: number,
	terminating: boolean,
}

local function terminate(session_id: string, session_info: ActiveSession): ()
end

local state = {
	active_sessions = {} :: {[string]: ActiveSession},
}

local session_id = "s1"
state.active_sessions[session_id] = {
	pid = "pid",
	created_at = 1,
	terminating = false,
}
state.active_sessions[session_id] = nil

local session_info = state.active_sessions[session_id]
if session_info then
	terminate(session_id, session_info)
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
	New(result).ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if len(reports) != 0 {
		t.Fatalf("argument reports = %#v, want guarded concrete dynamic-index read accepted", reports)
	}
}

func TestForEachCallArgumentUsesBoundaryRefinedDynamicTypeNameComparison(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type IntCell  = { kind: "number",  raw: number | string | boolean }
type TextCell = { kind: "string",  raw: number | string | boolean }
type FlagCell = { kind: "boolean", raw: number | string | boolean }
type Cell = IntCell | TextCell | FlagCell

local function flip(b: boolean): boolean return not b end

local function render(cell: Cell): string
    if cell.kind == "number" and type(cell.raw) == cell.kind then
        return "n"
    elseif cell.kind == "string" and type(cell.raw) == cell.kind then
        return cell.raw
    elseif cell.kind == "boolean" and type(cell.raw) == cell.kind then
        if flip(cell.raw) then
            return "t"
        end
        return "f"
    end
    return "?"
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) < 2 {
		t.Fatalf("function results = %#v, want flip and render", root)
	}
	var checkedArg bool
	for _, fnResult := range root.FunctionResults() {
		reader := New(fnResult)
		for _, point := range reader.callPoints() {
			site, ok := fnResult.CallSite(point)
			if !ok {
				continue
			}
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "cell.raw" {
					return true
				}
				checkedArg = true
				if !typ.TypeEquals(arg.TypeWithPresence, typ.Boolean) {
					t.Fatalf("cell.raw argument type = %v, want boolean from boundary-refined type-name comparison at call %#v", arg.TypeWithPresence, site)
				}
				return true
			})
		}
	}
	if !checkedArg {
		t.Fatal("did not find flip(cell.raw) argument")
	}
}

func TestForEachCallArgumentUsesReturnSlotPresenceGuardForRootArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Event = { id: string }
local function map_receive<T>(ch: Channel<T>, fn: (T) -> string): string
	local value, ok = ch:receive()
	if ok then
		return fn(value)
	end
	return "closed"
end
local M = {}
function M.read_event(ch: Channel<Event>): string
	return map_receive(ch, function(event: Event): string
		return event.id
	end)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var sawCallback bool
	for _, fnResult := range root.FunctionResults() {
		reader := New(fnResult)
		reader.ForEachCall(func(call CallSite) bool {
			for _, point := range reader.callPoints() {
				site, ok := fnResult.CallSite(point)
				if ok && fnResult.SymbolName(site.CalleeSymbol()) == "fn" && call.Point == point {
					sawCallback = true
				}
			}
			for _, report := range call.Reports {
				if report.Kind == readapi.CallArgumentReportObligation && report.Argument.Label == "value" {
					t.Fatalf("fn(value) reports argument error after ok guard: %#v", report)
				}
			}
			return true
		})
	}
	if !sawCallback {
		t.Fatal("fn(value) callback call not found")
	}
}

func TestForEachCallArgumentUsesCompoundTypeGuardedRootFromAnyRead(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Entry = {
    id: string,
    data: {[string]: any},
}

local function qualify_id(entry_id: string, relative_id: string): string
    return entry_id .. ":" .. relative_id
end

function build_page(entry: Entry)
    local raw_data_func = entry.data.data_func
    if type(raw_data_func) == "string" and raw_data_func ~= "" then
        return qualify_id(entry.id, raw_data_func)
    end
    return ""
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) < 2 {
		t.Fatalf("function results = %#v, want qualify_id and build_page", root)
	}
	var checkedArg bool
	var reports []CallArgumentReport
	for _, fnResult := range root.FunctionResults() {
		reader := New(fnResult)
		reader.ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
		for _, point := range reader.callPoints() {
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "raw_data_func" {
					return true
				}
				checkedArg = true
				if !typ.TypeEquals(arg.TypeWithPresence, typ.String) {
					t.Fatalf("raw_data_func argument type = %v, want string from compound type guard", arg.TypeWithPresence)
				}
				return true
			})
		}
	}
	if !checkedArg {
		t.Fatal("did not find qualify_id raw_data_func argument")
	}
	if len(reports) != 0 {
		t.Fatalf("call reports = %#v, want guarded raw_data_func accepted", reports)
	}
}

func TestForEachCallArgumentUsesBoundaryRefinedRootTypeGuardElse(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_number(n: number): number
    return n
end

local function f(v: number | string): number
    if type(v) == "number" then
        return 0
    else
        return need_number(v)
    end
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil || len(root.FunctionResults()) < 2 {
		t.Fatalf("function results = %#v, want need_number and f", root)
	}
	var checkedArg bool
	var reports []CallArgumentReport
	for _, result := range root.FunctionResults() {
		reader := New(result)
		for _, point := range reader.callPoints() {
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "v" {
					return true
				}
				checkedArg = true
				if !typ.TypeEquals(arg.TypeWithPresence, typ.String) {
					site, _ := result.CallSite(point)
					source, _ := site.ArgumentSourceAt(arg.Index)
					p, pathOK := reader.callArgumentExpressionPath(source)
					declared, declaredOK := result.DeclaredPathTypeAt(point, p, pathOK)
					reduced, reducedOK := reader.RuntimeKindReducedType(arg.Value, declared)
					t.Fatalf("v argument type = %v, want string from else-edge type guard (source=%#v path=%s pathOK=%v owned=%v declared=%v declaredOK=%v reduced=%v reducedOK=%v)",
						arg.TypeWithPresence, source, p, pathOK, reader.rootPathArgumentUsesBoundary(point, p), declared, declaredOK, reduced, reducedOK)
				}
				return true
			})
		}
		reader.ForEachCall(func(call CallSite) bool {
			if call.Point != 0 && !result.PointNormallyReachable(call.Point) {
				t.Fatalf("call point %d has report candidates but is not normally reachable", call.Point)
			}
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if !checkedArg {
		t.Fatal("did not find need_number(v) argument")
	}
	if len(reports) != 1 || reports[0].Check.Admissible {
		t.Fatalf("call reports = %#v, want one non-admissible string-to-number argument report", reports)
	}
	if reports[0].Kind != readapi.CallArgumentReportObligation || reports[0].Check.Expected == nil {
		t.Fatalf("call reports = %#v, want obligation report with expected type", reports)
	}
}

func TestForEachCallArgumentUsesBoundaryRefinedRootLocalTypeGuardElse(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_number(n: number): number
    return n
end

local v: number | string = value
if type(v) == "number" then
    return 0
else
    return need_number(v)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: reg,
		Globals:  []string{"value"},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("root result is nil")
	}
	reader := New(root)
	var checkedArg bool
	var reports []CallArgumentReport
	for _, point := range reader.callPoints() {
		reader.forEachCallArgument(point, func(arg CallArgument) bool {
			if arg.Label != "v" {
				return true
			}
			checkedArg = true
			if !typ.TypeEquals(arg.TypeWithPresence, typ.String) {
				site, _ := root.CallSite(point)
				source, _ := site.ArgumentSourceAt(arg.Index)
				p, pathOK := reader.callArgumentExpressionPath(source)
				declared, declaredOK := root.DeclaredPathTypeAt(point, p, pathOK)
				reduced, reducedOK := reader.RuntimeKindReducedType(arg.Value, declared)
				pathBefore, _ := root.PathValueBeforeBoundary(point, p)
				pathBoundary, _ := root.PathValueAtBoundary(point, p)
				sourceBefore, _ := root.SourceValueBeforeBoundary(point, source)
				sourceBoundary, _ := root.SourceValueAtBoundary(point, source)
				sharp, sharpOK := reader.callArgumentSharperBoundaryValue(point, source, p)
				pathBeforeType, pathBeforeTypeOK := reader.ValueTypeWithPresence(pathBefore)
				pathBoundaryType, pathBoundaryTypeOK := reader.ValueTypeWithPresence(pathBoundary)
				sourceBeforeType, sourceBeforeTypeOK := reader.ValueTypeWithPresence(sourceBefore)
				sourceBoundaryType, sourceBoundaryTypeOK := reader.ValueTypeWithPresence(sourceBoundary)
				sharpType, _ := reader.ValueTypeWithPresence(sharp)
				t.Fatalf("v argument type = %v, want string from top-level else-edge type guard (source=%#v path=%s pathOK=%v owned=%v runtimeProof=%v declared=%v declaredOK=%v reduced=%v reducedOK=%v pathBefore=%v/%v pathBoundary=%v/%v sourceBefore=%v/%v sourceBoundary=%v/%v sharp=%v sharpOK=%v sourceCan=%v pathCan=%v sourceNilOnly=%v pathNilOnly=%v sourceTop=%v pathTop=%v)",
					arg.TypeWithPresence, source, p, pathOK, reader.rootPathArgumentUsesBoundary(point, p), reader.pathHasRuntimeProof(point, p), declared, declaredOK, reduced, reducedOK, pathBeforeType, pathBeforeTypeOK, pathBoundaryType, pathBoundaryTypeOK, sourceBeforeType, sourceBeforeTypeOK, sourceBoundaryType, sourceBoundaryTypeOK, sharpType, sharpOK,
					reader.callArgumentBoundaryCanRefine(sourceBefore, pathBoundary),
					reader.callArgumentBoundaryCanRefine(pathBefore, pathBoundary),
					root.CallArgumentBoundaryNarrowsOnlyNilability(sourceBefore, pathBoundary),
					root.CallArgumentBoundaryNarrowsOnlyNilability(pathBefore, pathBoundary),
					root.CallArgumentBoundaryConcretizesTop(sourceBefore, pathBoundary),
					root.CallArgumentBoundaryConcretizesTop(pathBefore, pathBoundary))
			}
			return true
		})
	}
	reader.ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	if !checkedArg {
		t.Fatal("did not find need_number(v) argument")
	}
	if len(reports) != 1 || reports[0].Check.Admissible {
		t.Fatalf("call reports = %#v, want one non-admissible string-to-number argument report", reports)
	}
	if !typ.TypeEquals(reports[0].Check.Argument.TypeWithPresence, typ.String) {
		t.Fatalf("report argument type = %v, want string from top-level else-edge type guard", reports[0].Check.Argument.TypeWithPresence)
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

func TestForEachCallArgumentUsesDisjointRuntimeValidationCast(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function need_record(value: {name: string}): ()
end

local function f(y: number): ()
	need_record(y as {name: string})
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
	want := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	var checkedArg bool
	var reports []CallArgumentReport
	for _, fn := range result.FunctionResults() {
		reader := New(fn)
		reader.ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
		for _, point := range reader.callPoints() {
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "y" {
					return true
				}
				checkedArg = true
				if !typ.TypeEquals(arg.TypeWithPresence, want) {
					site, _ := fn.CallSite(point)
					source, sourceOK := site.ArgumentSourceAt(arg.Index)
					exprPath, exprPathOK := fn.ExpressionPathRef(source.ExprRef)
					boundaryValue, boundaryOK := fn.SourceValueAtBoundary(point, source)
					boundaryType, boundaryTypeOK := reader.ValueTypeWithPresence(boundaryValue)
					t.Fatalf("y cast argument type = %v, want runtime-validated %v (source=%#v sourceOK=%v runtime=%v exprPath=%s exprPathOK=%v boundaryType=%v boundaryOK=%v boundaryTypeOK=%v)",
						arg.TypeWithPresence, want, source, sourceOK, fn.SourceHasRuntimeValidation(source), exprPath, exprPathOK, boundaryType, boundaryOK, boundaryTypeOK)
				}
				return true
			})
		}
	}
	if !checkedArg {
		t.Fatal("did not find need_record(y as {name: string}) argument")
	}
	if len(reports) != 0 {
		t.Fatalf("call reports = %#v, want runtime validation cast accepted", reports)
	}
}

func TestForEachCallArgumentUsesCapturedEntryLiteralWhenPreCallReadIsTop(t *testing.T) {
	reg := standard.Registry()
	contract := manifest.New("contract")
	contract.DefineFunctionSignature("contract.get", signature.Function{
		Type: typ.Func().
			Param("id", typ.String).
			Returns(typ.Any, typeexpr.Optional(typ.String)).
			Build(),
	})
	stmts := parseChunk(t, `
local contract = require("contract")
local CONTRACT_ID = "wippy.llm:usage_tracker"

local function get_usage_tracker()
    local tracker_contract, err = contract.get(CONTRACT_ID)
    return tracker_contract
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{Manifests: []*manifest.Manifest{contract}},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var checkedArg bool
	var reports []CallArgumentReport
	for _, fn := range root.ReportableFunctionResults() {
		reader := New(fn)
		reader.ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
		for _, point := range reader.callPoints() {
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "CONTRACT_ID" {
					return true
				}
				checkedArg = true
				if !subtype.IsSubtype(arg.TypeWithPresence, typ.String) {
					site, _ := fn.CallSite(point)
					source, sourceOK := site.ArgumentSourceAt(arg.Index)
					boundaryValue, boundaryOK := fn.SourceValueAtBoundary(point, source)
					boundaryType, boundaryTypeOK := reader.ValueTypeWithPresence(boundaryValue)
					beforeValue, beforeOK := fn.SourceValueBeforeBoundary(point, source)
					beforeType, beforeTypeOK := reader.ValueTypeWithPresence(beforeValue)
					exprPath, exprPathOK := fn.ExpressionPathRef(source.ExprRef)
					t.Fatalf("CONTRACT_ID argument type = %v, want string (source=%#v sourceOK=%v exprPath=%s/%v boundary=%v/%v/%v before=%v/%v/%v)",
						arg.TypeWithPresence, source, sourceOK, exprPath, exprPathOK, boundaryType, boundaryOK, boundaryTypeOK, beforeType, beforeOK, beforeTypeOK)
				}
				return true
			})
		}
	}
	if !checkedArg {
		t.Fatal("did not find contract.get(CONTRACT_ID) argument")
	}
	if len(reports) != 0 {
		t.Fatalf("call reports = %#v, want captured literal accepted as string", reports)
	}
}

func TestForEachCallArgumentUsesNilOrNotTableFallbackJoin(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function consume(value: table): ()
end

local function normalize(raw: unknown): ()
    local value = raw
    if not value or type(value) ~= "table" then
        value = { run = true }
    end
    consume(value)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var checkedArg bool
	var reports []CallArgumentReport
	for _, fn := range root.ReportableFunctionResults() {
		reader := New(fn)
		reader.ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
		for _, point := range reader.callPoints() {
			reader.forEachCallArgument(point, func(arg CallArgument) bool {
				if arg.Label != "value" {
					return true
				}
				checkedArg = true
				if typ.IsAny(arg.TypeWithPresence) || typ.IsUnknown(arg.TypeWithPresence) || !subtype.IsSubtype(arg.TypeWithPresence, typ.BuiltinTableTopMarker()) {
					site, _ := fn.CallSite(point)
					source, sourceOK := site.ArgumentSourceAt(arg.Index)
					boundaryValue, boundaryOK := fn.SourceValueAtBoundary(point, source)
					boundaryType, boundaryTypeOK := reader.ValueTypeWithPresence(boundaryValue)
					beforeValue, beforeOK := fn.SourceValueBeforeBoundary(point, source)
					beforeType, beforeTypeOK := reader.ValueTypeWithPresence(beforeValue)
					exprPath, exprPathOK := fn.ExpressionPathRef(source.ExprRef)
					t.Fatalf("value argument type = %v, want table (source=%#v sourceOK=%v exprPath=%s/%v boundary=%v/%v/%v before=%v/%v/%v)",
						arg.TypeWithPresence, source, sourceOK, exprPath, exprPathOK, boundaryType, boundaryOK, boundaryTypeOK, beforeType, beforeOK, beforeTypeOK)
				}
				return true
			})
		}
	}
	if !checkedArg {
		t.Fatal("did not find consume(value) argument")
	}
	if len(reports) != 0 {
		t.Fatalf("call reports = %#v, want nil-or-not-table fallback accepted as table", reports)
	}
}

func TestForEachCallArgumentUsesNumericForIntegerVariable(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function take_integer(n: integer): ()
end

local shuffled = {}
for i = #shuffled, 2, -1 do
    take_integer(i)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	reader := New(root)
	var checkedArg bool
	var reports []CallArgumentReport
	reader.ForEachCall(func(call CallSite) bool {
		reports = append(reports, call.Reports...)
		return true
	})
	for _, point := range reader.callPoints() {
		reader.forEachCallArgument(point, func(arg CallArgument) bool {
			if arg.Label != "i" {
				return true
			}
			checkedArg = true
			if !subtype.IsSubtype(arg.TypeWithPresence, typ.Integer) {
				site, _ := root.CallSite(point)
				source, sourceOK := site.ArgumentSourceAt(arg.Index)
				boundaryValue, boundaryOK := root.SourceValueAtBoundary(point, source)
				boundaryType, boundaryTypeOK := reader.ValueTypeWithPresence(boundaryValue)
				beforeValue, beforeOK := root.SourceValueBeforeBoundary(point, source)
				beforeType, beforeTypeOK := reader.ValueTypeWithPresence(beforeValue)
				exprPath, exprPathOK := root.ExpressionPathRef(source.ExprRef)
				t.Fatalf("i argument type = %v, want integer (source=%#v sourceOK=%v exprPath=%s/%v boundary=%v/%v/%v before=%v/%v/%v)",
					arg.TypeWithPresence, source, sourceOK, exprPath, exprPathOK, boundaryType, boundaryOK, boundaryTypeOK, beforeType, beforeOK, beforeTypeOK)
			}
			return true
		})
	}
	if !checkedArg {
		t.Fatal("did not find take_integer(i) argument")
	}
	if len(reports) != 0 {
		t.Fatalf("call reports = %#v, want numeric-for integer variable accepted", reports)
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

func TestForEachCallReportsOptionalNonCallableMemberType(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Client = {invoke: number}
function f(c: Client?)
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

func TestForEachCallReportsOptionalReceiverAfterReverseMapPrimaryDelete(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	handler: (any) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any) -> any): ()
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { handler = handler }
	channel_to_id[chan] = channel_id
end

local function dispatch(chan: any): any
	register_channel(chan, function(value) return value end)
	local channel_id = channel_to_id[chan]
	if channel_id then
		registered_channels[channel_id] = nil
		local stale_id = channel_to_id[chan]
		if stale_id then
			local channel_info = registered_channels[stale_id]
			return channel_info.handler(chan)
		end
	end
	return nil
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatalf("root result is nil")
	}
	var reports []CallCalleeReport
	for _, fn := range result.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			if call.Callee.Kind != readapi.CallCalleeReportNone {
				reports = append(reports, call.Callee)
			}
			return true
		})
	}
	if len(reports) != 1 {
		t.Fatalf("callee reports = %#v, want optional receiver report", reports)
	}
	if reports[0].Kind != readapi.CallCalleeReportMayBeNil || !reports[0].MemberAccess || reports[0].Callable {
		t.Fatalf("callee report = %#v, want optional receiver member report", reports[0])
	}
}

func TestForEachCallAcceptsReverseMapValueAsTrustedArgumentProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type ChannelInfo = {
	chan: any,
	handler: (any, any, boolean, string) -> any,
}

local registered_channels: {[string]: ChannelInfo} = {}
local channel_to_id: {[any]: string} = {}

local function register_channel(chan: any, handler: (any, any, boolean, string) -> any): ()
	local channel_id = tostring(chan)
	registered_channels[channel_id] = { chan = chan, handler = handler }
	channel_to_id[chan] = channel_id
end

local function dispatch(result: { channel: any, value: any, ok: boolean }, state: any): any
	register_channel(result.channel, function(inner_state, value, ok, id) return value end)
	local channel_id = channel_to_id[result.channel]
	if channel_id then
		local channel_info = registered_channels[channel_id]
		return channel_info.handler(state, result.value, result.ok, channel_id)
	end
	return nil
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatalf("root result is nil")
	}
	var reports []CallArgumentReport
	for _, fn := range result.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			reports = append(reports, call.Reports...)
			return true
		})
	}
	if len(reports) != 0 {
		t.Fatalf("argument reports = %#v, want reverse-map value to prove handler arguments", reports)
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

func TestForEachCallUsesAliasDiscriminantProofForMemberCallee(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type SpecialData = {
	kind: "special",
	run: (self: SpecialData) -> (),
}
type OtherData = {
	kind: "other",
}
type Data = SpecialData | OtherData
type Obj = {
	data: Data,
}

local function dispatch(obj: Obj): ()
	local sub = obj.data
	if obj.data.kind == "special" then
		sub:run()
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
	var calls []CallSite
	New(result.FunctionResults()[0]).ForEachCall(func(call CallSite) bool {
		calls = append(calls, call)
		return true
	})
	if len(calls) != 1 {
		t.Fatalf("calls = %#v, want one sub:run call", calls)
	}
	if calls[0].Callee.Kind != readapi.CallCalleeReportNone || len(calls[0].Reports) != 0 {
		t.Fatalf("call = %#v, want alias discriminant proof to satisfy member callee", calls[0])
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

func TestForEachCallUsesPreCallReceiverStateForOptionalMethodReport(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local value = {}
function value:clear()
	value = false
end
if value then
	value:clear()
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil {
		t.Fatalf("root result is nil")
	}
	var found bool
	New(result).ForEachCall(func(call CallSite) bool {
		found = true
		if call.Callee.Kind != readapi.CallCalleeReportNone {
			t.Fatalf("callee report = %#v, want guarded receiver before method effects", call.Callee)
		}
		return true
	})
	if !found {
		t.Fatal("missing value.clear call")
	}
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

func TestForEachCallParamObligationExpectedTypeUsesContractProjection(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function id<T>(x: T): T
	return x
end

local function f(): ()
	local raw = ({ id = "ok" } :: any)
	id(raw)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	result := checked.RootResult()
	if result == nil || len(result.FunctionResults()) != 2 {
		t.Fatalf("function results = %#v, want id and f bodies", result)
	}
	var found bool
	New(result.FunctionResults()[1]).ForEachCall(func(call CallSite) bool {
		for _, report := range call.Reports {
			if report.Kind != readapi.CallArgumentReportObligation {
				continue
			}
			found = true
			if typevalue.TypeIncludesNil(report.Check.Expected) {
				t.Fatalf("expected obligation type = %v, actual=%v, value presence=%s, want pure contract without value-presence nilability", report.Check.Expected, report.Check.Argument.TypeWithPresence, product.PresenceOf(report.Check.Argument.Value))
			}
		}
		return true
	})
	if !found {
		t.Fatal("missing call argument obligation for id(raw)")
	}
}

func TestForEachCallParamObligationOriginUsesOwningFunctionName(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function invoke(provider: any, payload)
	provider[1](payload)
end

local p: { [1]: (number) -> () } = {
	[1] = function(v: number): () end,
}

invoke(p, "bad")
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var found bool
	results := append([]*body.Result{root}, root.FunctionResults()...)
	for _, fn := range results {
		New(fn).ForEachCall(func(call CallSite) bool {
			for _, report := range call.Reports {
				if report.Kind != readapi.CallArgumentReportObligation {
					continue
				}
				origin := report.Check.ExpectedOrigin
				if !origin.HasOrigin {
					continue
				}
				found = true
				if origin.FunctionName != "invoke" ||
					origin.SubjectLabel != "argument 1 (payload)" ||
					origin.ProviderLabel != "provider[1]" ||
					origin.MemberParamNumber != 1 {
					t.Fatalf("origin = %#v, want invoke payload -> provider[1] parameter 1", origin)
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("missing member call parameter obligation origin")
	}
}

func TestForEachCallReportsProjectedParamObligationForExplicitAnyCaller(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local function accept(req: { id: string }): ()
end

local function forward(payload)
	accept(payload)
end

local function entry(raw: any): ()
	forward(raw)
end
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	root := checked.RootResult()
	if root == nil {
		t.Fatal("RootResult nil")
	}
	var found bool
	for _, fn := range root.FunctionResults() {
		New(fn).ForEachCall(func(call CallSite) bool {
			for _, report := range call.Reports {
				if report.Kind != readapi.CallArgumentReportObligation {
					continue
				}
				if report.Argument.Label != "raw" {
					continue
				}
				found = true
				if report.Check.Admissible {
					t.Fatalf("report = %#v, want explicit-any raw rejected by projected obligation", report)
				}
				if !report.Argument.UntrustedTopOrigin || !report.Argument.ExplicitTopOrigin {
					t.Fatalf("argument origin = untrusted:%v explicit:%v type:%v, want explicit any",
						report.Argument.UntrustedTopOrigin, report.Argument.ExplicitTopOrigin, report.Argument.TypeWithPresence)
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("missing projected obligation report for forward(raw)")
	}
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
	if reports[0].Kind != readapi.CallCalleeReportNotCallable || reports[0].CallableName != "x" || !typ.TypeEquals(reports[0].Type, typ.LiteralInt(42)) {
		t.Fatalf("first callee report = %#v, want x not-callable literal 42", reports[0])
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

func TestCallArgumentUsesTrustedNarrowedAssignmentAlias(t *testing.T) {
	reg := standard.Registry()
	appError := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	success := typetable.NewRecord().
		Field("ok", typ.True).
		Field("value", typ.String).
		Build()
	failure := typetable.NewRecord().
		Field("ok", typ.False).
		Field("error", appError).
		Build()
	resultType := typeexpr.Union(success, failure)
	validatorManifest := manifest.New("validator")
	validatorManifest.SetExport(typetable.NewRecord().
		Field("validate_name", typ.Func().Param("input", typ.String).Returns(resultType).Build()).
		Build())
	validatorManifest.DefineFunctionSignature("validator.validate_name", signature.Function{
		Type: typ.Func().Param("input", typ.String).Returns(resultType).Build(),
		Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
			Source:     effect.ParamRef{Index: 0},
			Projection: projection.Projection{Steps: []projection.Step(nil)},
			When:       typ.LiteralString(""),
			Then:       failure,
		}}),
	})
	errorsManifest := manifest.New("errors")
	errorsManifest.SetExport(typetable.NewRecord().
		Field("wrap", typ.Func().Param("err", appError).Param("context", typ.String).Returns(appError).Build()).
		Build())
	stmts := parseChunk(t, `
local errors = require("errors")
local validator = require("validator")
local result = validator.validate_name("Alice")
if result.ok then
    local name = result.value
else
    local err = result.error
    local wrapped = errors.wrap(err, "registration")
end
`)
	result, err := body.CheckChunk(stmts, body.Config{
		Registry: reg,
		ModuleExports: importlookup.Source{
			Manifests: []*manifest.Manifest{errorsManifest, validatorManifest},
		},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{validatorManifest},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	reader := New(result)
	var seen bool
	for _, point := range result.Graph().RPO() {
		reader.forEachCallArgument(point, func(arg CallArgument) bool {
			if arg.Label != "err" {
				return true
			}
			seen = true
			if !typ.TypeEquals(arg.TypeWithPresence, appError) {
				t.Fatalf("err call argument type = %v, want AppError", arg.TypeWithPresence)
			}
			return true
		})
	}
	if !seen {
		t.Fatal("did not find err call argument")
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

func requireLocalAssignment(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) (cfg.Point, body.LocalAssignmentFact) {
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
	return 0, body.LocalAssignmentFact{}
}

func requireLocalAssignmentByName(t *testing.T, result *body.Result, name string) (cfg.Point, body.LocalAssignmentFact) {
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
	return 0, body.LocalAssignmentFact{}
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
