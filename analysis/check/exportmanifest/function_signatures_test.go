package exportmanifest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromProgramResultExportsReturnedTableMemberErrorReturnEffect(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`)

	m := FromProgramResult("client", result)
	assertManifestHasNoImportOrStdlibFunctionEffectLabels(t, m)
	sig, ok := m.FunctionSignatures["client.fetch"]
	if !ok {
		t.Fatalf("missing client.fetch function signature: %#v", m.FunctionSignatures)
	}
	if len(sig.Type.Returns) != 2 {
		t.Fatalf("client.fetch returns = %d, want 2", len(sig.Type.Returns))
	}
	if !typ.TypeEquals(sig.Type.Returns[0], typeexpr.Optional(typ.Number)) {
		t.Fatalf("client.fetch return 1 = %v, want number?", sig.Type.Returns[0])
	}
	if !typ.TypeEquals(sig.Type.Returns[1], typeexpr.Optional(typ.String)) {
		t.Fatalf("client.fetch return 2 = %v, want string?", sig.Type.Returns[1])
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("client.fetch effect = %v, want ErrorReturn(0, 1)", sig.Effect)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("client.fetch operational effects = nil")
	}
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
}

func TestFromProgramResultExportsUntypedParamObligationAsFunctionType(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		local http: {get: (url: string, options: table) -> ()} = {
			get = function(url: string, options: table): () end
		}
		function client.request(endpoint_path)
			local full_url = "https://api.example.test" .. endpoint_path
			return http.get(full_url, {})
		end
		return client
	`)

	m := FromProgramResult("client", result)
	sig, ok := m.FunctionSignatures["client.request"]
	if !ok {
		t.Fatalf("missing client.request function signature: %#v", m.FunctionSignatures)
	}
	if sig.Type == nil || len(sig.Type.Params) != 1 {
		t.Fatalf("client.request type = %#v, want one inferred parameter", sig.Type)
	}
	got := sig.Type.Params[0].Type
	want := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})
	if !typ.TypeEquals(got, want) {
		t.Fatalf("endpoint_path type = %v, want %v", got, want)
	}
}

func TestFromProgramResultExportsRuntimeCastReturnForUntypedMemberFunction(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		function client.resolve(entry)
			local url = entry.url or ""
			return url :: string
		end
		return client
	`)

	root := result.RootResult()
	var raw summary.Summary
	var hasRaw bool
	exportRoots := returnedExportSourcePaths(root)
	if len(exportRoots) == 0 {
		t.Fatalf("missing returned export root")
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, exportRoots[0].path, fact.Name)
		if !ok || member.Name != "resolve" {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		raw, hasRaw = functionSummary(result, root, fact.Func, target)
		break
	}
	if !hasRaw {
		t.Fatalf("missing raw summary for client.resolve")
	}

	m := FromProgramResult("client", result)
	sig, ok := m.FunctionSignatures["client.resolve"]
	if !ok {
		var rawReturnTypes []typ.Type
		for _, value := range raw.Returns {
			if t, ok := typevalue.TypeOf(root.Registry(), value); ok {
				rawReturnTypes = append(rawReturnTypes, t)
			}
		}
		t.Fatalf("missing client.resolve function signature: %#v; raw summary returns = %#v (%#v)", m.FunctionSignatures, rawReturnTypes, raw.Returns)
	}
	if sig.Type == nil || len(sig.Type.Returns) != 1 || !typ.TypeEquals(sig.Type.Returns[0], typ.String) {
		t.Fatalf("client.resolve type = %#v, want one string return", sig.Type)
	}
}

func TestFromProgramResultExportsGenericFunctionDefinitionMemberType(t *testing.T) {
	result := checkProgram(t, `
type Collection<T> = {
    items: {T},
    count: (self: Collection<T>) -> number,
}

local M = {}

function M.new<T>(): Collection<T>
    local c: Collection<T> = {
        items = {},
        count = function(self: Collection<T>): number
            return #self.items
        end,
    }
    return c
end

return M
`)

	m := FromProgramResult("collection", result)
	record, ok := m.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T %[1]v, want record", m.Export)
	}
	field, ok := fieldByName(record, "new")
	if !ok {
		t.Fatalf("export fields = %#v, want new", record.Fields)
	}
	fn, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("new type = %T %[1]v, want function", field.Type)
	}
	if len(fn.TypeParams) != 1 || fn.TypeParams[0].Name != "T" {
		t.Fatalf("new type params = %#v, want T", fn.TypeParams)
	}
	if len(fn.Returns) != 1 || typ.IsAny(fn.Returns[0]) || typ.IsUnknown(fn.Returns[0]) {
		t.Fatalf("new returns = %#v, want concrete generic Collection<T>", fn.Returns)
	}
}

func TestFromProgramResultRefinesDeclaredAnyReturnWhenSummaryIsPortable(t *testing.T) {
	result := checkProgram(t, `
local M = {}
local Runner = {}
Runner.__index = Runner

function Runner:run(): any
    return {
        status = "error",
        error = "failed",
        migrations_failed = 1,
    }
end

function M.setup(database_id: string): any
    local self = setmetatable({}, Runner)
    return self
end

	return M
`)
	root := result.RootResult()
	var raw summary.Summary
	var hasRaw bool
	exportRoots := returnedExportSourcePaths(root)
	if len(exportRoots) == 0 {
		t.Fatalf("missing returned export root")
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, exportRoots[0].path, fact.Name)
		if !ok || member.Name != "setup" {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		raw, hasRaw = functionSummary(result, root, fact.Func, target)
		break
	}

	m := FromProgramResult("runner", result)
	sig, ok := m.FunctionSignatures["runner.setup"]
	if !ok {
		t.Fatalf("missing runner.setup function signature: %#v", m.FunctionSignatures)
	}
	if sig.Type == nil || len(sig.Type.Returns) != 1 {
		t.Fatalf("runner.setup type = %#v, want one inferred return", sig.Type)
	}
	if typ.IsAny(sig.Type.Returns[0]) || typ.IsUnknown(sig.Type.Returns[0]) {
		var rawReturns []typ.Type
		var rawReturnStrings []string
		var rawReturnIDs []identity.ID
		var heapIDs []identity.ID
		var heapMemberStrings []string
		heapObjects := 0
		if hasRaw {
			heapObjects = len(raw.HeapTableObjects)
			for id := range raw.HeapTableObjects {
				heapIDs = append(heapIDs, id)
			}
			for _, value := range raw.Returns {
				if id, ok := product.Get(root.Registry(), value, identity.Key).ID(); ok {
					rawReturnIDs = append(rawReturnIDs, id)
				}
				if t, ok := typevalue.TypeOf(root.Registry(), value); ok {
					rawReturns = append(rawReturns, t)
					rawReturnStrings = append(rawReturnStrings, t.String())
				}
			}
			for _, id := range rawReturnIDs {
				object, ok := raw.HeapTableObjects[id]
				if !ok {
					continue
				}
				for _, memberValue := range object.StaticMembers() {
					memberType := "<none>"
					if fn, ok := root.FunctionValueTypeForValue(memberValue); ok && fn != nil {
						memberType = "fn:" + fn.String()
					} else if t, ok := typevalue.TypeOf(root.Registry(), memberValue); ok {
						memberType = "type:" + t.String()
					}
					if memberID, ok := product.Get(root.Registry(), memberValue, identity.Key).ID(); ok {
						heapMemberStrings = append(heapMemberStrings, memberID.String()+":"+memberType)
					} else {
						heapMemberStrings = append(heapMemberStrings, "no-id:"+memberType)
					}
				}
			}
		}
		t.Fatalf("runner.setup returns = %#v, raw summary returns = %#v (%v), raw return ids = %#v, raw heap ids = %#v, raw heap members = %v, raw heap objects = %d, want proven portable implementation shape instead of declared any", sig.Type.Returns, rawReturns, rawReturnStrings, rawReturnIDs, heapIDs, heapMemberStrings, heapObjects)
	}
	record, ok := unwrap.Annotated(sig.Type.Returns[0]).(*typ.Record)
	if !ok {
		t.Fatalf("runner.setup return = %T %[1]v, want record with metatable/prototype surface", sig.Type.Returns[0])
	}
	member, ok := staticMemberByStringKey(record, "run")
	if !ok {
		t.Fatalf("runner.setup return = %v, want run prototype method", record)
	}
	run, ok := member.Type.(*typ.Function)
	if !ok || len(run.Returns) != 1 {
		t.Fatalf("run member type = %T %[1]v, want one-return function", member.Type)
	}
	runResult, ok := unwrap.Annotated(run.Returns[0]).(*typ.Record)
	if !ok {
		t.Fatalf("run return = %T %[1]v, want record", run.Returns[0])
	}
	errorField, ok := fieldByName(runResult, "error")
	if !ok || !typ.TypeEquals(errorField.Type, typ.LiteralString("failed")) {
		t.Fatalf("run return fields = %#v, want error literal string", runResult.Fields)
	}
}

func TestFromProgramResultExportsStaticStringMembersFromAssignedLookupTable(t *testing.T) {
	result := checkProgram(t, `
type FinishReasonMap = {[string]: string}
local M = {}

local finish_reasons: FinishReasonMap = {}
finish_reasons["end_turn"] = "stop"
finish_reasons["max_tokens"] = "length"
M.finish_reasons = finish_reasons

function M.map_finish_reason(api_reason: string): string
	return M.finish_reasons[api_reason] or "unknown"
end

return M
`)

	m := FromProgramResult("mapper", result)
	record, ok := m.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T %[1]v, want record", m.Export)
	}
	field, ok := fieldByName(record, "finish_reasons")
	if !ok {
		t.Fatalf("export fields = %#v, want finish_reasons", record.Fields)
	}
	tableRecord, ok := unwrap.Alias(field.Type).(*typ.Record)
	if !ok {
		t.Fatalf("finish_reasons type = %T %[1]v, want record", field.Type)
	}
	member, ok := staticMemberByStringKey(tableRecord, "end_turn")
	if !ok {
		t.Fatalf("finish_reasons static members = %#v, want end_turn", tableRecord.StaticMembers)
	}
	if member.Optional {
		t.Fatalf("end_turn optional = true, want proven present")
	}
	if !typ.TypeEquals(member.Type, typ.LiteralString("stop")) {
		t.Fatalf("end_turn type = %v, want literal \"stop\"", member.Type)
	}
	if tableRecord.MapValue == nil || !typ.TypeEquals(tableRecord.MapValue, typ.String) {
		t.Fatalf("finish_reasons map value = %v, want string", tableRecord.MapValue)
	}
}

func TestFromProgramResultExportsReturnedTableLiteralShapeFromFactflow(t *testing.T) {
	result := checkProgram(t, `
		local M = {
			config = {
				level = "debug",
			},
		}
		return M
	`)

	m := FromProgramResult("configmod", result)
	record, ok := m.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T %[1]v, want record", m.Export)
	}
	config, ok := fieldByName(record, "config")
	if !ok {
		t.Fatalf("export fields = %#v, want config", record.Fields)
	}
	configRecord, ok := unwrap.Alias(config.Type).(*typ.Record)
	if !ok {
		t.Fatalf("config type = %T %[1]v, want record", config.Type)
	}
	level, ok := fieldByName(configRecord, "level")
	if !ok {
		t.Fatalf("config fields = %#v, want level", configRecord.Fields)
	}
	if !typ.TypeEquals(level.Type, typ.LiteralString("debug")) {
		t.Fatalf("config.level type = %v, want literal \"debug\"", level.Type)
	}
}

func TestFromProgramResultExportsIsNilNormalReturnRefinementEffect(t *testing.T) {
	result := checkProgram(t, `
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		return test
	`)

	m := FromProgramResult("test", result)
	assertManifestHasNoImportOrStdlibFunctionEffectLabels(t, m)
	sig, ok := m.FunctionSignatures["test.is_nil"]
	if !ok {
		t.Fatalf("missing test.is_nil function signature: %#v", m.FunctionSignatures)
	}
	if !hasNormalReturnAbsentRefinement(sig.Effect, 0) {
		t.Fatalf("test.is_nil effect = %v, want normal return absent refinement for param 0", sig.Effect)
	}
	if hasNormalReturnAbsentRefinement(sig.Effect, 1) {
		t.Fatalf("test.is_nil effect = %v, did not expect absent refinement for msg param", sig.Effect)
	}
}

func TestFromProgramResultExportsRuntimeTypeGuardNormalReturnTypeRefinement(t *testing.T) {
	result := checkProgram(t, `
		local test = {}
		function test.is_string(value, msg)
			if type(value) ~= "string" then
				error(msg or "expected string", 2)
			end
			return value
		end
		return test
	`)
	root := result.RootResult()
	var raw summary.Summary
	var hasRaw bool
	exportRoots := returnedExportSourcePaths(root)
	if len(exportRoots) == 0 {
		t.Fatalf("missing returned export root")
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, exportRoots[0].path, fact.Name)
		if !ok || member.Name != "is_string" {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		raw, hasRaw = functionSummary(result, root, fact.Func, target)
		break
	}
	if !hasRaw {
		t.Fatalf("missing raw summary for test.is_string")
	}
	if len(raw.NormalReturnParams) == 0 {
		t.Fatalf("raw normal-return params = %#v, want $0:string", raw.NormalReturnParams)
	}

	m := FromProgramResult("test", result)
	sig, ok := m.FunctionSignatures["test.is_string"]
	if !ok {
		t.Fatalf("missing test.is_string function signature: %#v", m.FunctionSignatures)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("test.is_string operational effects = nil")
	}
	if len(sig.OperationalEffects.NormalReturnTypeRefinements) != 1 ||
		!sig.OperationalEffects.NormalReturnTypeRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!typ.TypeEquals(sig.OperationalEffects.NormalReturnTypeRefinements[0].Type, typ.String) {
		t.Fatalf("normal-return type refinements = %#v, raw normal-return params = %#v, raw normal-return facts = %#v, want $0:string", sig.OperationalEffects.NormalReturnTypeRefinements, raw.NormalReturnParams, raw.NormalReturnFacts)
	}
	if !sig.OperationalEffects.NormalReturnTypeRefinements[0].Assertion.Has(assertion.RuntimeClaim) {
		t.Fatalf("normal-return type refinement assertion = %s, want runtime proof", sig.OperationalEffects.NormalReturnTypeRefinements[0].Assertion.String())
	}
}

func TestFunctionSummaryEffectDoesNotSerializeParamObligationsToManifestEffects(t *testing.T) {
	reg := standard.Registry()
	got := functionSummaryEffect(reg, summary.Summary{
		ParamObligations: []product.Value{
			typevalue.FromType(reg, typ.Number),
		},
	}, typ.Func().Param("tokens", typ.Any).Returns(typ.Number).Build())
	if !got.Pure() {
		t.Fatalf("effect = %v, want no manifest effect labels for pre-call ParamObligations", got)
	}
}

func TestAnalyzedExportEffectRowDropsImportOrStdlibVocabulary(t *testing.T) {
	row := effect.Empty.With(
		ownership.Send{FromParam: 0},
		ownership.BorrowAll{},
		iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed},
		ownership.SendParam{Param: effect.ParamRef{Index: 0}},
		ownership.Borrow{Param: effect.ParamRef{Index: 1}},
		returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
	)

	got := analyzedExportEffectRow(row)

	assertNoImportOrStdlibEffectLabels(t, got)
	if !hasOwnershipSendParam(got, 0) {
		t.Fatalf("effect = %v, want SendParam retained", got)
	}
	if !hasOwnershipBorrow(got, 1) {
		t.Fatalf("effect = %v, want Borrow retained", got)
	}
	if !hasErrorReturn(got, 0, 1) {
		t.Fatalf("effect = %v, want ErrorReturn retained", got)
	}
}

func TestFunctionSummaryEffectExportsExactRootOwnershipBoundaryFacts(t *testing.T) {
	got := functionSummaryEffect(standard.Registry(), summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(1), Kind: callboundary.EscapeEventStore, Recursive: true},
				{Target: pathdom.NewPlaceholder(2), Kind: callboundary.EscapeEventRetain, Recursive: true},
				{Target: pathdom.NewPlaceholder(3), Kind: callboundary.EscapeEventExport, Recursive: true},
				{Target: pathdom.NewPlaceholder(4), Kind: callboundary.EscapeEventOpaque, Recursive: true},
				{Target: pathdom.NewPlaceholder(5).Field("child"), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(5), Kind: callboundary.EscapeEventSend},
				{Target: pathdom.NewPlaceholder(6), Kind: callboundary.EscapeEventBorrow, Recursive: true},
			},
			FrozenTables: []callboundary.FrozenTableFact{
				{Target: pathdom.NewPlaceholder(5)},
				{Target: pathdom.NewPlaceholder(1).Field("child")},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(1)},
				{Path: pathdom.NewPlaceholder(2).Field("child")},
			},
		},
	}, typ.Func().
		Param("sent", typ.Any).
		Param("stored", typ.Any).
		Param("retained", typ.Any).
		Param("exported", typ.Any).
		Param("opaque", typ.Any).
		Param("frozen", typ.Any).
		Param("borrowed", typ.Any).
		Build())

	assertNoImportOrStdlibEffectLabels(t, got)
	if !hasOwnershipSendParam(got, 0) {
		t.Fatalf("effect = %v, want exact SendParam for param 0", got)
	}
	if !hasOwnershipStoreUnknown(got, 1) {
		t.Fatalf("effect = %v, want root Store for param 1", got)
	}
	if !hasMutationTableMutator(got, 1) {
		t.Fatalf("effect = %v, want root TableMutator for param 1", got)
	}
	if !hasOwnershipRetain(got, 2) {
		t.Fatalf("effect = %v, want root Retain for param 2", got)
	}
	if !hasOwnershipExport(got, 3) {
		t.Fatalf("effect = %v, want root Export for param 3", got)
	}
	if !hasOwnershipOpaque(got, 4) {
		t.Fatalf("effect = %v, want root Opaque for param 4", got)
	}
	if !hasOwnershipFreeze(got, 5) {
		t.Fatalf("effect = %v, want root Freeze for param 5", got)
	}
	if hasOwnershipSendParam(got, 5) {
		t.Fatalf("effect = %v, did not expect descendant/non-recursive send export for param 5", got)
	}
	if hasOwnershipFreeze(got, 1) {
		t.Fatalf("effect = %v, did not expect descendant freeze export for param 1", got)
	}
	if !hasOwnershipBorrow(got, 6) {
		t.Fatalf("effect = %v, want root Borrow for param 6", got)
	}
}

func TestFunctionSummaryEffectExportsExactStoreRelationWithoutDegradedPair(t *testing.T) {
	got := functionSummaryEffect(standard.Registry(), summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventStore, Recursive: true},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(1)},
			},
			StoreRelations: []callboundary.StoreRelationFact{
				{Source: pathdom.NewPlaceholder(0), Into: pathdom.NewPlaceholder(1)},
			},
		},
	}, typ.Func().
		Param("value", typ.Any).
		Param("container", typ.Any).
		Build())

	if !hasOwnershipStoreExact(got, 0, 1) {
		t.Fatalf("effect = %v, want exact Store{Param:0, Into:1}", got)
	}
	if hasOwnershipStoreUnknown(got, 0) {
		t.Fatalf("effect = %v, did not expect degraded Store{Param:0, Into:-1}", got)
	}
	if hasMutationTableMutator(got, 1) {
		t.Fatalf("effect = %v, did not expect redundant TableMutator{Target:1, Value:-1}", got)
	}
}

func TestFunctionSummaryOperationalEffectsPreservesDescendantBoundaryFacts(t *testing.T) {
	reg := standard.Registry()
	got := functionSummaryOperationalEffects(reg, summary.Summary{
		ReturnPresenceRelations: []summary.ReturnPresenceRelation{
			{
				TriggerIndex:    1,
				TriggerPresence: presence.Present(),
				TargetIndex:     0,
				TargetPresence:  presence.Absent(),
			},
		},
		NormalReturnParams: []product.Value{
			typevalue.FromType(reg, typ.Number),
			product.Absent(reg),
		},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathStaticMembers: []callboundary.PathStaticMemberFact{
				{Path: pathdom.NewPlaceholder(0).Field("kind"), Value: typevalue.FromType(reg, typ.String)},
				{Path: pathdom.NewPlaceholder(1).Field("kind"), Value: product.Top()},
				{Path: pathdom.NewPlaceholder(2).Field("kind"), Value: typevalue.FromType(reg, typ.Boolean)},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(0).Field("items")},
			},
			FrozenTables: []callboundary.FrozenTableFact{
				{Target: pathdom.NewPlaceholder(1).Field("sealed")},
			},
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(1).Field("borrowed"), Kind: callboundary.EscapeEventBorrow},
			},
			StoreRelations: []callboundary.StoreRelationFact{
				{Source: pathdom.NewPlaceholder(0).Field("payload"), Into: pathdom.NewPlaceholder(1).Field("bucket")},
			},
		},
	}, typ.Func().
		Param("source", typ.Any).
		Param("target", typ.Any).
		Returns(typeexpr.Optional(typ.Number), typeexpr.Optional(typ.String)).
		Build(), "test.effect")

	if got == nil {
		t.Fatalf("operational effects = nil")
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v", got.ReturnPresenceRelations)
	}
	if len(got.PathStaticMembers) != 1 ||
		!got.PathStaticMembers[0].Path.Equal(pathdom.NewPlaceholder(0).Field("kind")) ||
		!typ.TypeEquals(got.PathStaticMembers[0].Type, typ.String) {
		t.Fatalf("path static members = %#v", got.PathStaticMembers)
	}
	if len(got.NormalReturnPresenceRefinements) != 2 ||
		!got.NormalReturnPresenceRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!presence.Equal(got.NormalReturnPresenceRefinements[0].Presence, presence.Present()) ||
		!got.NormalReturnPresenceRefinements[1].Path.Equal(pathdom.NewPlaceholder(1)) ||
		!presence.Equal(got.NormalReturnPresenceRefinements[1].Presence, presence.Absent()) {
		t.Fatalf("normal-return presence refinements = %#v", got.NormalReturnPresenceRefinements)
	}
	if len(got.NormalReturnTypeRefinements) != 1 ||
		!got.NormalReturnTypeRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!typ.TypeEquals(got.NormalReturnTypeRefinements[0].Type, typ.Number) {
		t.Fatalf("normal-return type refinements = %#v", got.NormalReturnTypeRefinements)
	}
	if len(got.PathInvalidations) != 1 || !got.PathInvalidations[0].Path.Equal(pathdom.NewPlaceholder(0).Field("items")) {
		t.Fatalf("path invalidations = %#v", got.PathInvalidations)
	}
	if len(got.FrozenTables) != 1 || !got.FrozenTables[0].Target.Equal(pathdom.NewPlaceholder(1).Field("sealed")) {
		t.Fatalf("frozen tables = %#v", got.FrozenTables)
	}
	if len(got.EscapeEvents) != 2 ||
		!got.EscapeEvents[0].Target.Equal(pathdom.NewPlaceholder(0).Field("payload")) ||
		got.EscapeEvents[0].Kind != signature.EscapeSend ||
		!got.EscapeEvents[0].Recursive ||
		!got.EscapeEvents[1].Target.Equal(pathdom.NewPlaceholder(1).Field("borrowed")) ||
		got.EscapeEvents[1].Kind != signature.EscapeBorrow ||
		got.EscapeEvents[1].Recursive {
		t.Fatalf("escape events = %#v", got.EscapeEvents)
	}
	if len(got.StoreRelations) != 1 ||
		!got.StoreRelations[0].Source.Equal(pathdom.NewPlaceholder(0).Field("payload")) ||
		!got.StoreRelations[0].Into.Equal(pathdom.NewPlaceholder(1).Field("bucket")) {
		t.Fatalf("store relations = %#v", got.StoreRelations)
	}
	if len(got.ParamRelations) != 2 ||
		got.ParamRelations[0].Param != 0 ||
		got.ParamRelations[0].EscapeClass != signature.EscapeSend ||
		got.ParamRelations[0].PlacementConsequence != signature.PlacementConsequenceSharedHeap ||
		got.ParamRelations[0].StoredInto != 1 ||
		!got.ParamRelations[0].HasStoredInto ||
		got.ParamRelations[1].Param != 1 ||
		got.ParamRelations[1].EscapeClass != signature.EscapeBorrow ||
		got.ParamRelations[1].PlacementConsequence != signature.PlacementConsequenceKeep {
		t.Fatalf("param relations = %#v", got.ParamRelations)
	}
}

func TestFunctionSummaryOperationalEffectsExportsReadOnlyParamRelation(t *testing.T) {
	got := functionSummaryOperationalEffects(standard.Registry(), summary.Summary{}, typ.Func().
		Param("value", typ.Any).
		Build(), "read")
	if got == nil {
		t.Fatal("operational effects = nil, want read-only param relation")
	}
	if len(got.ParamRelations) != 1 ||
		got.ParamRelations[0].Param != 0 ||
		got.ParamRelations[0].EscapeClass != signature.EscapeNone ||
		got.ParamRelations[0].PlacementConsequence != signature.PlacementConsequenceKeep ||
		got.ParamRelations[0].ThroughReturn ||
		got.ParamRelations[0].HasStoredInto {
		t.Fatalf("param relations = %#v, want $0 none/keep", got.ParamRelations)
	}
}

func TestFunctionSummaryOperationalEffectsExportsSinkAndReturnParamRelations(t *testing.T) {
	reg := standard.Registry()
	sinkSource, ok := pathaddr.RootPlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	if !ok {
		t.Fatal("RootPlaceholderKeyFromPath($0) failed")
	}
	aliasSource, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(1).Field("child"))
	if !ok {
		t.Fatal("PlaceholderKeyFromPath($1.child) failed")
	}
	got := functionSummaryOperationalEffects(reg, summary.Summary{
		ParamSinkExposures: []summary.ParamSinkExposure{{
			Source:   sinkSource,
			Contract: product.Top(),
		}},
		ReturnParamPathAliases: []summary.ReturnParamPathAlias{{
			ReturnIndex: 0,
			Source:      aliasSource,
		}},
	}, typ.Func().
		Param("stored", typ.Any).
		Param("returned", typ.Any).
		Returns(typ.Any).
		Build(), "relations")
	if got == nil {
		t.Fatal("operational effects = nil")
	}
	if len(got.ParamRelations) != 2 {
		t.Fatalf("param relations = %#v, want two rows", got.ParamRelations)
	}
	if got.ParamRelations[0].EscapeClass != signature.EscapeStore ||
		got.ParamRelations[0].PlacementConsequence != signature.PlacementConsequenceOwnedHeap {
		t.Fatalf("param 0 relation = %#v, want store/owned-heap", got.ParamRelations[0])
	}
	if !got.ParamRelations[1].ThroughReturn ||
		got.ParamRelations[1].EscapeClass != signature.EscapeNone ||
		got.ParamRelations[1].PlacementConsequence != signature.PlacementConsequenceKeep {
		t.Fatalf("param 1 relation = %#v, want throughReturn with none/keep", got.ParamRelations[1])
	}
}

func TestFromProgramResultExportsConditionalSinkStoreAsStoreRelation(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		local sink = {}
		function client.maybe_store(value: any, flag: boolean): ()
			if flag then
				sink.saved = value
			end
		end
		return client
	`)
	m := FromProgramResult("client", result)
	sig, ok := m.FunctionSignatures["client.maybe_store"]
	if !ok {
		t.Fatalf("missing client.maybe_store function signature: %#v", m.FunctionSignatures)
	}
	if sig.OperationalEffects == nil {
		t.Fatal("client.maybe_store operational effects = nil")
	}
	if len(sig.OperationalEffects.ParamRelations) < 1 {
		t.Fatalf("param relations = %#v, want relation for value", sig.OperationalEffects.ParamRelations)
	}
	relation := sig.OperationalEffects.ParamRelations[0]
	if relation.EscapeClass == signature.EscapeNone || relation.EscapeClass == signature.EscapeBorrow {
		t.Fatalf("param relation = %#v, conditional sink store must not export none/borrow", relation)
	}
	if relation.EscapeClass != signature.EscapeStore ||
		relation.PlacementConsequence != signature.PlacementConsequenceOwnedHeap {
		t.Fatalf("param relation = %#v, want store/owned-heap", relation)
	}
}

func TestFunctionSummaryOperationalEffectsLaneMatrixManifestRoundTrip(t *testing.T) {
	reg := standard.Registry()
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	rawProduct := typevalue.WithWitness(
		reg,
		typevalue.FromType(reg, typ.LiteralString("raw-product-sentinel")),
		typ.LiteralString("raw-product-sentinel"),
	)
	pathRefinementValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	dynamicKey := typevalue.WithWitness(
		reg,
		typevalue.FromType(reg, typ.LiteralString("send")),
		typ.LiteralString("send"),
	)
	dynamicValueType := typ.Func().Param("v", typ.String).Build()
	dynamicValue := typevalue.WithWitness(reg, typevalue.FromType(reg, dynamicValueType), dynamicValueType)
	heapID := identity.ID{Kind: "test", Site: "lane-matrix", Index: 1}
	ks := keyspace.New()
	heapStaticKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "static"}})
	if !ok {
		t.Fatal("heap static suffix key failed")
	}
	heapDynamicTableKey, ok := ks.FromStateKey(pathdom.PathKey("heap-only.dynamic"))
	if !ok {
		t.Fatal("heap dynamic table key failed")
	}
	fn := typ.Func().
		Param("source", typ.Any).
		Param("target", typ.Any).
		Param("optional", typ.Any).
		Returns(typeexpr.Optional(typ.Number), typeexpr.Optional(typ.String)).
		Build()
	sum := summary.Summary{
		ParamObligations: []product.Value{
			typevalue.FromType(reg, typ.LiteralString("param-obligation-sentinel")),
		},
		ParamMemberCallObligations: []summary.ParamMemberCallObligation{{
			ReceiverParam:    0,
			Member:           segment.Segment{Kind: segment.SegmentField, Name: "precallSentinel"},
			ArgParam:         1,
			MemberParamIndex: 0,
		}},
		NormalReturnParams: []product.Value{
			present,
			absent,
			product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()),
		},
		NormalReturnParamConditions: []summary.ParamCondition{
			summary.ParamConditionTruthy,
			summary.ParamConditionFalsy,
		},
		NormalReturnParamEqualities: []summary.ParamEquality{{Left: 0, Right: 1}},
		ReturnConditionParamRefinements: []summary.ReturnConditionParamRefinement{{
			ReturnIndex: 0,
			ReturnValue: true,
			Target:      pathdom.NewPlaceholder(0).Field("conditionalOnly"),
			Value:       absent,
		}},
		ReturnPresenceRelations: []summary.ReturnPresenceRelation{{
			TriggerIndex:    1,
			TriggerPresence: presence.Present(),
			TargetIndex:     0,
			TargetPresence:  presence.Absent(),
		}},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			heapID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: present,
				StaticMembers: map[keyspace.Key]product.Value{
					heapStaticKey: rawProduct,
				},
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: heapDynamicTableKey, Site: "heap.dynamic"}: {
						KeyPresence: presence.Present(),
						KeyValue:    present,
						Value:       absent,
						Admission:   dynamicindex.AdmissionAdmitted,
					},
				},
			}),
		},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{{
				Path:  pathdom.NewPlaceholder(0).Field("rawProduct"),
				Value: pathRefinementValue,
			}, {
				Path:  pathdom.NewPlaceholder(2),
				Value: present,
			}},
			PathStaticMembers: []callboundary.PathStaticMemberFact{
				{Path: pathdom.NewPlaceholder(0).Field("kind"), Value: typevalue.FromType(reg, typ.String)},
				{Path: pathdom.NewPlaceholder(0).Field("staticRaw"), Value: present},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{{
				Path: pathdom.NewPlaceholder(1).Field("items"),
			}},
			DynamicIndexFacts: []callboundary.DynamicIndexFact{{
				Table:   pathdom.NewPlaceholder(0).Field("dynamicOnly"),
				Site:    "callee.dynamic",
				KeyPath: pathdom.NewPlaceholder(1),
				Value: dynamicindex.Fact{
					KeyPresence: presence.Present(),
					KeyValue:    dynamicKey,
					Value:       dynamicValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			}, {
				Table: pathdom.Path{Root: "ret[0]"},
				Site:  "callee.returned.keys",
				Value: dynamicindex.Fact{
					KeyPresence: presence.Present(),
					KeyValue:    dynamicKey,
					Value:       dynamicValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			}},
			KeyMemberships: []callboundary.KeyMembershipFact{{
				Key:   pathdom.NewPlaceholder(1).Field("key"),
				Table: pathdom.NewPlaceholder(0).Field("table"),
			}},
			DynamicValueKeys: []callboundary.DynamicValueKeyMembershipFact{{
				Container: pathdom.Path{Root: "ret[0]"},
				Site:      "callee.returned.keys",
				Table:     pathdom.NewPlaceholder(0).Field("table"),
			}},
			BranchProofs: []callboundary.BranchProof{{
				Kind:  pathevidence.BranchProofPathEqual,
				Path:  pathdom.NewPlaceholder(0).Field("branchOnly"),
				Other: pathdom.NewPlaceholder(1).Field("branchOther"),
			}},
			ChannelSelects: []callboundary.ChannelSelectFact{{
				Select:     channelselectfact.ID("callee.select"),
				Kind:       channelselectfact.FactReceive,
				Result:     pathdom.NewPlaceholder(0).Field("selectResult"),
				Case:       pathdom.NewPlaceholder(1).Field("selectCase"),
				Index:      2,
				HasDefault: true,
			}},
			FrozenTables: []callboundary.FrozenTableFact{{
				Target: pathdom.NewPlaceholder(0).Field("sealed"),
			}},
			EffectDeltas: []callboundary.EffectDelta{{
				Target: pathdom.NewPlaceholder(0).Field("genericEffect"),
				Site:   "generic.effect",
				Kind:   effectdelta.Mutation,
				Value: effectdelta.Value{
					Before: present,
					After:  absent,
					Change: effectdelta.ChangeChanged,
				},
			}},
			EscapeEvents: []callboundary.EscapeEventFact{{
				Target:    pathdom.NewPlaceholder(0).Field("payload"),
				Kind:      callboundary.EscapeEventSend,
				Recursive: true,
			}},
			StoreRelations: []callboundary.StoreRelationFact{{
				Source: pathdom.NewPlaceholder(0).Field("payload"),
				Into:   pathdom.NewPlaceholder(1).Field("items"),
			}},
		},
	}
	want := signature.OperationalEffects{
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{{
			TriggerIndex:    1,
			TriggerPresence: presence.Present(),
			TargetIndex:     0,
			TargetPresence:  presence.Absent(),
		}},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{
			{Path: pathdom.NewPlaceholder(0), Presence: presence.Present()},
			{Path: pathdom.NewPlaceholder(0).Field("rawProduct"), Presence: presence.Present()},
			{Path: pathdom.NewPlaceholder(1), Presence: presence.Absent()},
			{Path: pathdom.NewPlaceholder(2), Presence: presence.Present()},
		},
		NormalReturnTypeRefinements: []signature.PathTypeRefinement{
			{Path: pathdom.NewPlaceholder(0).Field("rawProduct"), Type: typ.String},
		},
		PathStaticMembers: []signature.PathStaticMemberFact{{
			Path: pathdom.NewPlaceholder(0).Field("kind"),
			Type: typ.String,
		}},
		PathInvalidations: []signature.PathInvalidation{{
			Path: pathdom.NewPlaceholder(1).Field("items"),
		}},
		BranchProofs: []signature.BranchProof{{
			Kind:  signature.BranchProofPathEqual,
			Path:  pathdom.NewPlaceholder(0).Field("branchOnly"),
			Other: pathdom.NewPlaceholder(1).Field("branchOther"),
		}},
		DynamicIndexFacts: []signature.DynamicIndexFact{{
			Table:       pathdom.NewPlaceholder(0).Field("dynamicOnly"),
			Site:        "callee.dynamic",
			KeyPresence: presence.Present(),
			Key: signature.DynamicIndexOperand{
				Path: pathdom.NewPlaceholder(1),
				Type: typ.LiteralString("send"),
			},
			Value: signature.DynamicIndexOperand{
				Type: dynamicValueType,
			},
			Admission: signature.DynamicIndexAdmissionAdmitted,
		}, {
			Table:       pathdom.Path{Root: "ret[0]"},
			Site:        "callee.returned.keys",
			KeyPresence: presence.Present(),
			Key: signature.DynamicIndexOperand{
				Type: typ.LiteralString("send"),
			},
			Value: signature.DynamicIndexOperand{
				Type: dynamicValueType,
			},
			Admission: signature.DynamicIndexAdmissionAdmitted,
		}},
		KeyMemberships: []signature.KeyMembership{{
			Key:   pathdom.NewPlaceholder(1).Field("key"),
			Table: pathdom.NewPlaceholder(0).Field("table"),
		}},
		DynamicValueKeys: []signature.DynamicValueKeyMembership{{
			Container: pathdom.Path{Root: "ret[0]"},
			Site:      "callee.returned.keys",
			Table:     pathdom.NewPlaceholder(0).Field("table"),
		}},
		FrozenTables: []signature.FrozenTable{{
			Target: pathdom.NewPlaceholder(0).Field("sealed"),
		}},
		EscapeEvents: []signature.EscapeEvent{{
			Target:    pathdom.NewPlaceholder(0).Field("payload"),
			Kind:      signature.EscapeSend,
			Recursive: true,
		}},
		StoreRelations: []signature.StoreRelation{{
			Source: pathdom.NewPlaceholder(0).Field("payload"),
			Into:   pathdom.NewPlaceholder(1).Field("items"),
		}},
		ParamRelations: []signature.ParamRelation{
			{
				Param:                0,
				EscapeClass:          signature.EscapeSend,
				PlacementConsequence: signature.PlacementConsequenceSharedHeap,
				StoredInto:           1,
				HasStoredInto:        true,
			},
			{
				Param:                1,
				EscapeClass:          signature.EscapeNone,
				PlacementConsequence: signature.PlacementConsequenceKeep,
			},
			{
				Param:                2,
				EscapeClass:          signature.EscapeNone,
				PlacementConsequence: signature.PlacementConsequenceKeep,
			},
		},
	}

	got := functionSummaryOperationalEffects(reg, sum, fn, "laneMatrix")
	if got == nil {
		t.Fatalf("operational effects = nil")
	}
	if !got.Equals(want) {
		t.Fatalf("operational effects = %#v, want %#v", got, want)
	}

	m := manifest.New("example/lane-matrix")
	m.DefineFunctionSignature("laneMatrix", signature.Function{Type: fn, OperationalEffects: got})
	data, err := manifest.Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	encoded := string(data)
	for _, wantFragment := range []string{
		`"operationalEffects"`,
		`"returnPresenceRelations"`,
		`"normalReturnPresenceRefinements"`,
		`"pathStaticMembers"`,
		`"pathInvalidations"`,
		`"branchProofs"`,
		`"dynamicIndexFacts"`,
		`"keyMemberships"`,
		`"dynamicValueKeys"`,
		`"frozenTables"`,
		`"escapeEvents"`,
		`"storeRelations"`,
		`"paramRelations"`,
	} {
		if !strings.Contains(encoded, wantFragment) {
			t.Fatalf("encoded manifest missing %s:\n%s", wantFragment, encoded)
		}
	}
	for _, forbidden := range []string{
		`"pathRefinements"`,
		`"PathRefinements"`,
		`"DynamicIndexFacts"`,
		`"BranchProofs"`,
		`"channelSelects"`,
		`"ChannelSelects"`,
		`"effectDeltas"`,
		`"EffectDeltas"`,
		`"heapTableObjects"`,
		`"HeapTableObjects"`,
		`"paramObligations"`,
		`"ParamObligations"`,
		`"paramMemberCallObligations"`,
		`"ParamMemberCallObligations"`,
		`"normalReturnParamConditions"`,
		`"NormalReturnParamConditions"`,
		`"normalReturnParamEqualities"`,
		`"NormalReturnParamEqualities"`,
		`"returnConditionParamRefinements"`,
		`"ReturnConditionParamRefinements"`,
		"raw-product-sentinel",
		"param-obligation-sentinel",
		"precallSentinel",
		"conditionalOnly",
		"heap-only",
		"heap.dynamic",
		"staticRaw",
		"callee.select",
		"selectResult",
		"selectCase",
		"genericEffect",
		"generic.effect",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("encoded manifest leaked forbidden fragment %q:\n%s", forbidden, encoded)
		}
	}

	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	gotSig, ok := decoded.FunctionSignatures["laneMatrix"]
	if !ok {
		t.Fatalf("missing laneMatrix function signature")
	}
	if gotSig.OperationalEffects == nil || !gotSig.OperationalEffects.Equals(want) {
		t.Fatalf("decoded operational effects = %#v, want %#v", gotSig.OperationalEffects, want)
	}
}

func TestFunctionSummaryOperationalEffectsExportsReturnAllocationTemplate(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "summary-template", Index: 1}
	childID := identity.ID{Kind: "lua.table", Site: "summary-template", Index: 2}
	entryID := identity.ID{Kind: "lua.table", Site: "summary-template", Index: 3}
	unrelatedID := identity.ID{Kind: "lua.table", Site: "summary-template", Index: 4}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	rootType := typetable.NewRecord().Field("child", typetable.NewRecord().Build()).Build()
	childType := typetable.NewRecord().Field("name", typ.String).Build()
	entryType := typetable.NewRecord().Field("route", typ.String).Build()
	rootValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, childType), childType), identity.Key, identity.Singleton(childID))
	entryValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, entryType), entryType), identity.Key, identity.Singleton(entryID))
	unrelatedValue := product.Set(reg, present, identity.Key, identity.Singleton(unrelatedID))
	ks := keyspace.New()
	childKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	rootItemsKey, ok := ks.FromStateKey(pathdom.PathKey("root.items"))
	if !ok {
		t.Fatal("root items table key failed")
	}
	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: rootValue,
				StaticMembers: map[keyspace.Key]product.Value{
					childKey: childValue,
				},
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: rootItemsKey, Site: "write"}: {
						KeyPresence: presence.Present(),
						KeyValue:    typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
						Value:       entryValue,
						Admission:   dynamicindex.AdmissionAdmitted,
					},
				},
			}),
			childID:     heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: childValue}),
			entryID:     heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: entryValue}),
			unrelatedID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: unrelatedValue}),
		},
	}, typ.Func().Returns(rootType).Build(), "builder.build")
	if got == nil || len(got.ReturnAllocationTemplates) != 1 {
		t.Fatalf("allocation templates = %#v, want one return template", got)
	}
	template := got.ReturnAllocationTemplates[0]
	if template.ReturnIndex != 0 || template.Root != "builder.build:return:0:root" {
		t.Fatalf("return template = %#v", template)
	}
	if len(template.Objects) != 3 {
		t.Fatalf("template objects = %#v, want reachable root/child/dynamic entry only", template.Objects)
	}
	root := allocationTemplateObject(template.Objects, "builder.build:return:0:root")
	if root == nil {
		t.Fatalf("missing root object in %#v", template.Objects)
	}
	if !typ.TypeEquals(root.Type, rootType) {
		t.Fatalf("root type = %v, want %v", root.Type, rootType)
	}
	if len(root.StaticMembers) != 1 ||
		segment.FormatSegments(root.StaticMembers[0].Suffix) != ".child" ||
		root.StaticMembers[0].Value != "builder.build:return:0:root.child" {
		t.Fatalf("root static members = %#v", root.StaticMembers)
	}
	if len(root.DynamicEntries) != 1 ||
		root.DynamicEntries[0].Value != "builder.build:return:0:root:dynamic:0:value" ||
		!typ.TypeEquals(root.DynamicEntries[0].KeyType, typ.String) {
		t.Fatalf("root dynamic entries = %#v", root.DynamicEntries)
	}
	entry := allocationTemplateObject(template.Objects, "builder.build:return:0:root:dynamic:0:value")
	if entry == nil {
		t.Fatalf("missing dynamic entry object in %#v", template.Objects)
	}
	if !typ.TypeEquals(entry.Type, entryType) {
		t.Fatalf("dynamic entry type = %v, want %v", entry.Type, entryType)
	}
	child := allocationTemplateObject(template.Objects, "builder.build:return:0:root.child")
	if child == nil {
		t.Fatalf("missing child object in %#v", template.Objects)
	}
	if !typ.TypeEquals(child.Type, childType) {
		t.Fatalf("child type = %v, want %v", child.Type, childType)
	}
	if allocationTemplateObject(template.Objects, "builder.build:return:0:unrelated") != nil {
		t.Fatalf("unrelated object leaked into template: %#v", template.Objects)
	}
}

func TestFunctionSummaryOperationalEffectsDoesNotExportAllocationForDeclaredAnyReturn(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "declared-any-template", Index: 1}
	rootType := typetable.NewRecord().Field("value", typ.LiteralString("impl")).Build()
	rootValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType), identity.Key, identity.Singleton(rootID))
	ks := keyspace.New()

	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue}),
		},
	}, typ.Func().Returns(typ.Any).Build(), "builder.any")
	if got != nil && len(got.ReturnAllocationTemplates) != 0 {
		t.Fatalf("allocation templates = %#v, want none for declared any return", got.ReturnAllocationTemplates)
	}
}

func TestFunctionSummaryOperationalEffectsClampsAllocationRootWithDeclaredAnyField(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "declared-any-field-template", Index: 1}
	implType := typetable.NewRecord().Field("value", typ.LiteralString("impl")).Build()
	declaredType := typetable.NewRecord().Field("value", typ.Any).Build()
	rootValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, implType), implType), identity.Key, identity.Singleton(rootID))
	ks := keyspace.New()

	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue}),
		},
	}, typ.Func().Returns(declaredType).Build(), "builder.record")
	if got == nil || len(got.ReturnAllocationTemplates) != 1 {
		t.Fatalf("allocation templates = %#v, want one clamped template", got)
	}
	root := allocationTemplateObject(got.ReturnAllocationTemplates[0].Objects, "builder.record:return:0:root")
	if root == nil {
		t.Fatalf("missing root object in %#v", got.ReturnAllocationTemplates[0].Objects)
	}
	if !typ.TypeEquals(root.Type, declaredType) {
		t.Fatalf("root type = %v, want declared %v", root.Type, declaredType)
	}
}

func TestFunctionSummaryOperationalEffectsUsesDeclaredUnionAllocationRoot(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "declared-result-template", Index: 1}
	userType := typetable.NewRecord().
		Field("id", typ.String).
		Field("email", typ.String).
		Build()
	errorArm := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	successArm := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", userType).
		Build()
	declaredType := typeexpr.Union(successArm, errorArm)
	rootValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, errorArm), errorArm), identity.Key, identity.Singleton(rootID))
	ks := keyspace.New()

	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue}),
		},
	}, typ.Func().Returns(declaredType).Build(), "repo.find_by_id")
	if got == nil || len(got.ReturnAllocationTemplates) != 1 {
		t.Fatalf("allocation templates = %#v, want one declared-union template", got)
	}
	root := allocationTemplateObject(got.ReturnAllocationTemplates[0].Objects, "repo.find_by_id:return:0:root")
	if root == nil {
		t.Fatalf("missing root object in %#v", got.ReturnAllocationTemplates[0].Objects)
	}
	if !typ.TypeEquals(root.Type, declaredType) {
		t.Fatalf("root type = %v, want declared union %v", root.Type, declaredType)
	}
}

func TestFunctionSummaryOperationalEffectsPreservesDeclaredOptionalReturnMembers(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "declared-optional-member-template", Index: 1}
	streamType := typetable.NewRecord().Field("read", typ.Func().Returns(typ.String).Build()).Build()
	implType := typetable.NewRecord().
		Field("status_code", typ.LiteralNumber(500)).
		Field("stream", streamType).
		Build()
	declaredRecord := typetable.NewRecord().
		Field("status_code", typ.Number).
		OptField("body", typ.String).
		OptField("stream", streamType).
		Build()
	declaredType := typeexpr.Optional(declaredRecord)
	rootValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, implType), implType), identity.Key, identity.Singleton(rootID))
	ks := keyspace.New()

	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue}),
		},
	}, typ.Func().Returns(declaredType).Build(), "http_client.get")
	if got == nil || len(got.ReturnAllocationTemplates) != 1 {
		t.Fatalf("allocation templates = %#v, want one template", got)
	}
	root := allocationTemplateObject(got.ReturnAllocationTemplates[0].Objects, "http_client.get:return:0:root")
	if root == nil {
		t.Fatalf("missing root object in %#v", got.ReturnAllocationTemplates[0].Objects)
	}
	wantRecord := typetable.NewRecord().
		Field("status_code", typ.LiteralNumber(500)).
		OptField("body", typ.String).
		OptField("stream", streamType).
		Build()
	want := typeexpr.Optional(wantRecord)
	if !typ.TypeEquals(root.Type, want) {
		t.Fatalf("root type = %v, want declared envelope %v", root.Type, want)
	}
}

func TestFunctionSummaryOperationalEffectsSkipsReturnAllocationBeyondDeclaredReturns(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "summary-template", Index: 1}
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(rootID))
	ks := keyspace.New()

	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue}),
		},
	}, typ.Func().Build(), "builder.no_return")
	if got != nil {
		t.Fatalf("operational effects = %#v, want nil when return allocation is beyond declared returns", got)
	}
}

func TestFunctionSummaryOperationalEffectsSkipsDanglingReturnAllocationRefs(t *testing.T) {
	reg := standard.Registry()
	rootID := identity.ID{Kind: "lua.table", Site: "dangling-template", Index: 1}
	missingChildID := identity.ID{Kind: "lua.table", Site: "dangling-template", Index: 2}
	missingKeyID := identity.ID{Kind: "lua.table", Site: "dangling-template", Index: 3}
	missingValueID := identity.ID{Kind: "lua.table", Site: "dangling-template", Index: 4}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	rootValue := product.Set(reg, present, identity.Key, identity.Singleton(rootID))
	missingChildValue := product.Set(reg, present, identity.Key, identity.Singleton(missingChildID))
	missingKeyValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), identity.Key, identity.Singleton(missingKeyID))
	missingValue := product.Set(reg, present, identity.Key, identity.Singleton(missingValueID))
	ks := keyspace.New()
	childKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	fn := typ.Func().Returns(typetable.NewRecord().Build()).Build()
	rootItemsKey, ok := ks.FromStateKey(pathdom.PathKey("root.items"))
	if !ok {
		t.Fatal("root items table key failed")
	}
	got := functionSummaryOperationalEffects(reg, summary.Summary{
		Returns:      []product.Value{rootValue},
		HeapKeySpace: ks,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: rootValue,
				StaticMembers: map[keyspace.Key]product.Value{
					childKey: missingChildValue,
				},
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: rootItemsKey, Site: "write"}: {
						KeyPresence: presence.Present(),
						KeyValue:    missingKeyValue,
						Value:       missingValue,
						Admission:   dynamicindex.AdmissionAdmitted,
					},
				},
			}),
		},
	}, fn, "builder.dangling")
	if got == nil || len(got.ReturnAllocationTemplates) != 1 {
		t.Fatalf("allocation templates = %#v, want one return template", got)
	}
	template := got.ReturnAllocationTemplates[0]
	assertAllocationTemplateRefsPresent(t, template)
	if len(template.Objects) != 1 {
		t.Fatalf("template objects = %#v, want only reachable exported root", template.Objects)
	}
	root := allocationTemplateObject(template.Objects, "builder.dangling:return:0:root")
	if root == nil {
		t.Fatalf("missing root object in %#v", template.Objects)
	}
	if len(root.StaticMembers) != 0 {
		t.Fatalf("root static members = %#v, want missing child reference skipped", root.StaticMembers)
	}
	if len(root.DynamicEntries) != 1 ||
		root.DynamicEntries[0].Key != "" ||
		root.DynamicEntries[0].Value != "" ||
		!typ.TypeEquals(root.DynamicEntries[0].KeyType, typ.String) {
		t.Fatalf("root dynamic entries = %#v, want only safe key type evidence", root.DynamicEntries)
	}
	m := manifest.New("example/dangling-template")
	m.DefineFunctionSignature("builder.dangling", signature.Function{Type: fn, OperationalEffects: got})
	data, err := manifest.Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := manifest.Decode(data); err != nil {
		t.Fatalf("Decode: %v\n%s", err, data)
	}
}

func allocationTemplateObject(objects []signature.AllocationObjectTemplate, id signature.AllocationTemplateID) *signature.AllocationObjectTemplate {
	for i := range objects {
		if objects[i].ID == id {
			return &objects[i]
		}
	}
	return nil
}

func assertAllocationTemplateRefsPresent(t *testing.T, template signature.ReturnAllocationTemplate) {
	t.Helper()
	objects := make(map[signature.AllocationTemplateID]struct{}, len(template.Objects))
	for _, object := range template.Objects {
		objects[object.ID] = struct{}{}
	}
	for _, object := range template.Objects {
		for _, member := range object.StaticMembers {
			if _, ok := objects[member.Value]; !ok {
				t.Fatalf("object %q static member %s references missing object %q", object.ID, segment.FormatSegments(member.Suffix), member.Value)
			}
		}
		for _, entry := range object.DynamicEntries {
			if entry.Key != "" {
				if _, ok := objects[entry.Key]; !ok {
					t.Fatalf("object %q dynamic entry references missing key object %q", object.ID, entry.Key)
				}
			}
			if entry.Value != "" {
				if _, ok := objects[entry.Value]; !ok {
					t.Fatalf("object %q dynamic entry references missing value object %q", object.ID, entry.Value)
				}
			}
		}
	}
}

func TestFunctionSummaryOperationalEffectsEmptyIsAbsent(t *testing.T) {
	got := functionSummaryOperationalEffects(standard.Registry(), summary.Summary{}, typ.Func().Build(), "empty")
	if got != nil {
		t.Fatalf("operational effects = %#v, want nil for empty summary facts", got)
	}
}

func TestFromProgramResultExportsUntypedDynamicIndexOperationOnlySignature(t *testing.T) {
	result := checkProgram(t, `
		local ops = {}
		function ops.install(provider, key)
			provider[key] = function(v: string): () end
		end
		return ops
	`)
	var raw summary.Summary
	var hasRaw bool
	var exitDynamic any
	root := result.RootResult()
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		if got, ok := functionSummary(result, root, fact.Func, target); ok {
			raw, hasRaw = got, true
			break
		}
	}
	for _, fnResult := range root.FunctionResults() {
		if exit, ok := fnResult.ExitState(); ok {
			exitDynamic = exit.DynamicIndexFactsSnapshot()
			break
		}
	}
	if !hasRaw {
		t.Fatalf("missing raw summary for ops.install")
	}

	m := FromProgramResult("ops", result)
	sig, ok := m.FunctionSignatures["ops.install"]
	if !ok {
		t.Fatalf("missing ops.install function signature: %#v", m.FunctionSignatures)
	}
	if sig.Type != nil {
		t.Fatalf("ops.install type = %v, want nil operation-only signature", sig.Type)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("ops.install operational effects = nil")
	}
	found := false
	for _, invalidation := range sig.OperationalEffects.PathInvalidations {
		if invalidation.Path.Equal(pathdom.NewPlaceholder(0)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ops.install path invalidations = %#v, want $0 invalidation", sig.OperationalEffects.PathInvalidations)
	}
	foundDynamic := false
	for _, fact := range sig.OperationalEffects.DynamicIndexFacts {
		if fact.Table.Equal(pathdom.NewPlaceholder(0)) && fact.Key.Path.Equal(pathdom.NewPlaceholder(1)) {
			foundDynamic = true
			break
		}
	}
	if !foundDynamic {
		t.Fatalf("ops.install dynamic-index facts = %#v, want $0[$1] write evidence; raw summary dynamic = %#v; exit dynamic = %#v", sig.OperationalEffects.DynamicIndexFacts, raw.NormalReturnFacts.DynamicIndexFacts, exitDynamic)
	}
}

func TestFromProgramResultSkipsPureUntypedExportedFunctionSignature(t *testing.T) {
	result := checkProgram(t, `
		local ops = {}
		function ops.noop(value)
			local copy = value
		end
		return ops
	`)

	m := FromProgramResult("ops", result)
	if _, ok := m.FunctionSignatures["ops.noop"]; ok {
		t.Fatalf("unexpected pure untyped signature: %#v", m.FunctionSignatures)
	}
}

func checkProgram(t *testing.T, src string) program.Result {
	t.Helper()
	stmts, err := parse.ParseString(src, "exportmanifest_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
	return result
}

func hasErrorReturn(row effect.Row, valueIndex, errorIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		err, ok := effect.NormalizeLabel(label).(returns.ErrorReturn)
		return ok && err.ValueIndex == valueIndex && err.ErrorIndex == errorIndex
	})
}

func assertManifestHasNoImportOrStdlibFunctionEffectLabels(t *testing.T, m *manifest.Manifest) {
	t.Helper()
	if m == nil {
		t.Fatal("manifest = nil")
	}
	for name, sig := range m.FunctionSignatures {
		t.Run(name, func(t *testing.T) {
			assertNoImportOrStdlibEffectLabels(t, sig.Effect)
		})
	}
}

func TestFunctionSummaryEffectExportsParamLiteralReturnCases(t *testing.T) {
	reg := standard.Registry()
	returnType := typetable.NewRecord().
		Field("mode", typ.LiteralString("NONE")).
		Build()
	row := functionSummaryEffectForArity(reg, summary.Summary{
		ReturnParamLiteralCases: []summary.ReturnParamLiteralCase{{
			ParamIndex:  0,
			When:        typ.LiteralString("none"),
			ReturnIndex: 0,
			Value:       typevalue.WithWitness(reg, typevalue.FromType(reg, returnType), returnType),
		}},
	}, 1, 1)

	if len(row.Labels) != 1 {
		t.Fatalf("labels = %#v, want one conditional return label", row.Labels)
	}
	ret, ok := effect.NormalizeLabel(row.Labels[0]).(returns.Return)
	if !ok || ret.ReturnIndex != 0 {
		t.Fatalf("label = %#v, want return[0]", row.Labels[0])
	}
	conditional, ok := returns.AsConditionalType(ret.Transform)
	if !ok {
		t.Fatalf("transform = %#v, want ConditionalType", ret.Transform)
	}
	if conditional.Source.Index != 0 ||
		!typ.TypeEquals(conditional.When, typ.LiteralString("none")) ||
		!typ.TypeEquals(conditional.Then, returnType) {
		t.Fatalf("conditional = %#v, want param 0 none -> %s", conditional, returnType)
	}
}

func TestFromProgramResultExportsSufficientOrLiteralReturnCases(t *testing.T) {
	result := checkProgram(t, `
		local M = {}
		function M.pick(choice)
			if not choice or choice == "auto" or choice == "any" then
				return { mode = "AUTO" }, nil
			end
			return nil, "unsupported"
		end
		return M
	`)
	root := result.RootResult()
	var raw summary.Summary
	var hasRaw bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, returnedExportSourcePaths(root)[0].path, fact.Name)
		if !ok || member.Name != "pick" {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		raw, hasRaw = functionSummary(result, root, fact.Func, target)
		break
	}
	if !hasRaw {
		t.Fatalf("missing raw summary for picker.pick")
	}
	if got, ok := summaryReturnLiteralCaseValuePresence(raw, 0, typ.LiteralString("auto")); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("raw auto return case presence = %v, %v; want present", got, ok)
	}

	m := FromProgramResult("picker", result)
	sig, ok := m.FunctionSignatures["picker.pick"]
	if !ok {
		t.Fatalf("missing picker.pick function signature: %#v", m.FunctionSignatures)
	}
	if !hasConditionalReturnCase(sig.Effect, 0, typ.LiteralString("auto")) {
		t.Fatalf("effect = %v, want auto literal return case", sig.Effect)
	}
	if got, ok := conditionalReturnThen(sig.Effect, 0, typ.LiteralString("auto")); !ok || isOptionalType(got) {
		t.Fatalf("auto return case then = %v, %v; want non-optional record", got, ok)
	}
	if !hasConditionalReturnCase(sig.Effect, 0, typ.LiteralString("any")) {
		t.Fatalf("effect = %v, want any literal return case", sig.Effect)
	}
	data, err := manifest.Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := manifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v\n%s", err, data)
	}
	decodedSig, ok := decoded.FunctionSignatures["picker.pick"]
	if !ok {
		t.Fatalf("decoded signatures = %#v, want picker.pick", decoded.FunctionSignatures)
	}
	if !hasConditionalReturnCase(decodedSig.Effect, 0, typ.LiteralString("auto")) {
		t.Fatalf("decoded effect = %v, want auto literal return case", decodedSig.Effect)
	}
}

func TestFromProgramResultExportsUntypedTableMemberFunctionShapeWithAnyReturns(t *testing.T) {
	result := checkProgram(t, `
		local M = {}
		function M.classify_error(api_error)
			if not api_error then
				return "server_error", "Unknown error", nil
			end
			return "invalid_request", api_error.message or "Bad request", { status_code = api_error.status_code }
		end
		return M
	`)
	root := result.RootResult()
	var fn *ast.FunctionExpr
	var raw summary.Summary
	var hasRaw bool
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, returnedExportSourcePaths(root)[0].path, fact.Name)
		if !ok || member.Name != "classify_error" {
			continue
		}
		target := pathdom.Path{}
		if fact.HasTargetPath {
			target = fact.TargetPath
		}
		fn = fact.Func
		raw, hasRaw = functionSummary(result, root, fact.Func, target)
		break
	}
	if !hasRaw {
		t.Fatalf("missing raw summary for M.classify_error")
	}
	if len(raw.Returns) != 3 {
		t.Fatalf("raw returns = %d, want 3", len(raw.Returns))
	}
	inferred, ok := inferredFunctionTypeFromSummary(root.Registry(), root, fn, raw)
	if !ok || inferred == nil {
		var returnTypes []typ.Type
		for i, value := range raw.Returns {
			t, _ := typevalue.TypeOf(root.Registry(), enrichManifestReturnValue(root.Registry(), root, raw, i, value))
			returnTypes = append(returnTypes, t)
		}
		t.Fatalf("inferred function type = %v, %v; slots = %#v; returns = %#v; want function type from raw summary", inferred, ok, root.FunctionParamSlots(fn), returnTypes)
	}

	m := FromProgramResult("mapper", result)
	sig, ok := m.FunctionSignatures["mapper.classify_error"]
	if !ok || sig.Type == nil {
		t.Fatalf("missing mapper.classify_error function signature: %#v", m.FunctionSignatures)
	}
	if len(sig.Type.Params) != 1 || !typ.IsAny(sig.Type.Params[0].Type) {
		t.Fatalf("params = %#v, want one any param", sig.Type.Params)
	}
	if len(sig.Type.Returns) != 3 {
		t.Fatalf("returns = %#v, want 3 return slots", sig.Type.Returns)
	}
	if typ.IsAny(sig.Type.Returns[0]) || typ.IsUnknown(sig.Type.Returns[0]) ||
		!typ.IsAny(sig.Type.Returns[1]) ||
		!typ.IsAny(sig.Type.Returns[2]) {
		t.Fatalf("returns = %#v, want (concrete string-like, any, any)", sig.Type.Returns)
	}
}

func TestFromProgramResultExportsUntypedConstructorPostAssignedMethodShape(t *testing.T) {
	result := checkProgram(t, `
		local M = {}
		function M.new(messages)
			local builder = {
				messages = messages or {},
			}
			builder.get_messages = function(self: any)
				return self.messages
			end
			return builder
		end
		return M
	`)

	m := FromProgramResult("prompt", result)
	sig, ok := m.FunctionSignatures["prompt.new"]
	if !ok || sig.Type == nil || len(sig.Type.Returns) != 1 {
		t.Fatalf("missing prompt.new function signature: %#v", m.FunctionSignatures)
	}
	if typ.IsAny(sig.Type.Returns[0]) || typ.IsUnknown(sig.Type.Returns[0]) {
		t.Fatalf("prompt.new return = %v, want returned builder record shape", sig.Type.Returns[0])
	}
	rec, ok := unwrap.Alias(sig.Type.Returns[0]).(*typ.Record)
	if !ok || rec == nil || rec.GetField("get_messages") == nil {
		t.Fatalf("prompt.new return = %v, want get_messages member", sig.Type.Returns[0])
	}
}

func hasConditionalReturnCase(row effect.Row, returnIndex int, when typ.Type) bool {
	_, ok := conditionalReturnThen(row, returnIndex, when)
	return ok
}

func conditionalReturnThen(row effect.Row, returnIndex int, when typ.Type) (typ.Type, bool) {
	var out typ.Type
	found := row.Has(func(label effect.Label) bool {
		ret, ok := effect.NormalizeLabel(label).(returns.Return)
		if !ok || ret.ReturnIndex != returnIndex {
			return false
		}
		conditional, ok := returns.AsConditionalType(ret.Transform)
		if !ok || !typ.TypeEquals(conditional.When, when) {
			return false
		}
		out = conditional.Then
		return true
	})
	return out, found
}

func isOptionalType(t typ.Type) bool {
	_, ok := unwrap.Alias(t).(*typ.Optional)
	return ok
}

func summaryReturnLiteralCaseValuePresence(sum summary.Summary, returnIndex int, when typ.Type) (presence.Value, bool) {
	for _, c := range sum.ReturnParamLiteralCases {
		if c.ReturnIndex == returnIndex && typ.TypeEquals(c.When, when) {
			return product.PresenceOf(c.Value), true
		}
	}
	return presence.Bottom(), false
}

func assertNoImportOrStdlibEffectLabels(t *testing.T, row effect.Row) {
	t.Helper()
	for _, label := range row.Labels {
		desc, ok := caplabel.DescriptorFor(label)
		if !ok {
			t.Fatalf("effect row %v contains unaudited label %T", row, label)
		}
		if desc.Status == capability.StatusImportOrStdlib {
			t.Fatalf("effect row %v contains import/stdlib-only label %T (%s)", row, label, desc.ID)
		}
	}
}

func assertSignatureReturnPresenceRelation(
	t *testing.T,
	relations []signature.ReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex == triggerIndex &&
			presence.Equal(relation.TriggerPresence, triggerPresence) &&
			relation.TargetIndex == targetIndex &&
			presence.Equal(relation.TargetPresence, targetPresence) {
			return
		}
	}
	t.Fatalf("return presence relations = %#v, missing %d/%s -> %d/%s", relations, triggerIndex, triggerPresence, targetIndex, targetPresence)
}

func hasNormalReturnAbsentRefinement(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		refinement, ok := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
		if !ok || refinement.Target.Index != paramIndex {
			return false
		}
		return postcondition.Absent{}.Equals(refinement.Refinement)
	})
}

func hasOwnershipSendParam(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		send, ok := effect.NormalizeLabel(label).(ownership.SendParam)
		return ok && send.Param.Index == paramIndex
	})
}

func hasMutationTableMutator(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		mutator, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		return ok && mutator.Target.Index == paramIndex && mutator.Value.Index == -1
	})
}

func hasOwnershipStoreUnknown(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == paramIndex && store.Into.Index == -1
	})
}

func hasOwnershipStoreExact(row effect.Row, paramIndex, intoIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == paramIndex && store.Into.Index == intoIndex
	})
}

func hasOwnershipRetain(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		retain, ok := effect.NormalizeLabel(label).(ownership.Retain)
		return ok && retain.Param.Index == paramIndex
	})
}

func hasOwnershipBorrow(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		borrow, ok := effect.NormalizeLabel(label).(ownership.Borrow)
		return ok && borrow.Param.Index == paramIndex
	})
}

func hasOwnershipExport(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		export, ok := effect.NormalizeLabel(label).(ownership.Export)
		return ok && export.Param.Index == paramIndex
	})
}

func hasOwnershipOpaque(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		opaque, ok := effect.NormalizeLabel(label).(ownership.Opaque)
		return ok && opaque.Param.Index == paramIndex
	})
}

func hasOwnershipFreeze(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		freeze, ok := effect.NormalizeLabel(label).(ownership.Freeze)
		return ok && freeze.Param.Index == paramIndex
	})
}
