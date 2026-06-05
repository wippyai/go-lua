package canonical

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDriverDiagnosticContextsCaptureBranchNarrowedClosureCell(t *testing.T) {
	chunk, err := parse.ParseString(`
local function test(x)
    if type(x) == "number" then
        local f = function()
            local y: number = x
        end
    end
end
`, "closure-context.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("closure-context.lua")
	driver.Run(sess, chunk)

	for fn, result := range sess.results {
		if fn == nil || result == nil || result.Graph == nil || fn.ParList == nil || fn.ParList.HasVargs {
			continue
		}
		if len(result.Graph.ParamSymbols()) != 0 {
			continue
		}
		symbols := result.Graph.Bindings().SymbolsByName("x")
		if len(symbols) != 1 {
			continue
		}
		x := symbols[0]
		captured := false
		for _, sym := range result.Graph.Bindings().CapturedSymbols(result.Graph.Func()) {
			if sym == x {
				captured = true
				break
			}
		}
		if !captured {
			continue
		}
		ref, ok := testRefByGraphID(driver, result.Graph.ID())
		if !ok {
			t.Fatalf("inner closure graph %d has no canonical ref", result.Graph.ID())
		}
		contexts := driver.diagnosticContexts[ref]
		if len(contexts) == 0 {
			t.Fatalf("inner closure has no diagnostic contexts; diagnostics=%v", sess.DiagnosticsSlice())
		}
		for _, key := range contexts {
			if av, ok := key.Entry.Cells().Value(x); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
				t.Fatalf("inner closure context captured x=%v/%v, want number; contexts=%v diagnostics=%v", av.ProjectValue(), ok, contexts, sess.DiagnosticsSlice())
			}
		}
		return
	}
	t.Fatal("inner closure capturing x was not found")
}

func TestDriverDiagnosticContextsCaptureReturnedFunctionLivePrototype(t *testing.T) {
	chunk, err := parse.ParseString(`
local T = {}
local M = {}

function M.make(): string
    return T.render()
end

function T.render(): string
    return "ok"
end

return M
`, "returned-closure-context.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("returned-closure-context.lua")
	driver.Run(sess, chunk)

	for fn, result := range sess.results {
		if fn == nil || result == nil || result.Graph == nil || fn.ParList == nil || fn.ParList.HasVargs {
			continue
		}
		if len(result.Graph.ParamSymbols()) != 0 {
			continue
		}
		symbols := result.Graph.Bindings().SymbolsByName("T")
		if len(symbols) != 1 {
			continue
		}
		tSym := symbols[0]
		captured := false
		for _, sym := range result.Graph.Bindings().CapturedSymbols(result.Graph.Func()) {
			if sym == tSym {
				captured = true
				break
			}
		}
		if !captured {
			continue
		}
		ref, ok := testRefByGraphID(driver, result.Graph.ID())
		if !ok {
			t.Fatalf("returned closure graph %d has no canonical ref", result.Graph.ID())
		}
		contexts := driver.diagnosticContexts[ref]
		if got := result.NarrowedTypeAt(result.Graph.Entry(), constraint.NewPath(tSym, "T")); typeHasField(got, "render") {
			return
		}
		for _, key := range contexts {
			if av, ok := key.Entry.Cells().Value(tSym); ok && typeHasField(av.ProjectValue(), "render") {
				return
			}
		}
		t.Fatalf("returned closure contexts did not capture live T.render surface; entryT=%v free=%v contexts=%s summaries=%s diagnostics=%v", result.NarrowedTypeAt(result.Graph.Entry(), constraint.NewPath(tSym, "T")), result.Graph.IsFreeSymbol(tSym), describeContextsForSymbol(contexts, tSym), describeSummaryCaptureExports(driver), sess.DiagnosticsSlice())
	}
	t.Fatal("returned closure capturing T was not found")
}

func TestProgramCaptureEntriesUsesParentPrototypeExport(t *testing.T) {
	chunk, err := parse.ParseString(`
local T = {}
local M = {}

function M.make(): string
    return T.render()
end

function T.render(): string
    return "ok"
end

return M
`, "returned-closure-capture-entries.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, prog, _, ctx := testDriverProgram(t, chunk)
	q := summary.New(prog)
	var makeRef summary.FuncRef
	var tSym cfg.SymbolID
	for _, ref := range prog.refs {
		g := prog.Graph(ref)
		if g == nil || g.Func() == nil || g.Func().ParList == nil || len(g.ParamSymbols()) != 0 {
			continue
		}
		symbols := g.Bindings().SymbolsByName("T")
		if len(symbols) != 1 || !g.IsFreeSymbol(symbols[0]) {
			continue
		}
		makeRef = ref
		tSym = symbols[0]
		break
	}
	if makeRef == (summary.FuncRef{}) || tSym == 0 {
		t.Fatal("M.make capturing T was not found")
	}
	for _, ref := range prog.refs {
		_ = q.Summarize(ctx, ref)
	}
	cells := prog.CaptureEntries(makeRef, func(dep summary.FuncRef) flow.CaptureCells {
		return q.Summarize(ctx, dep).CaptureExports
	})
	if av, ok := cells.Value(tSym); !ok || !typeHasField(av.ProjectValue(), "render") {
		t.Fatalf("CaptureEntries T = %v/%v, want render; cells=%s chain=%v", av.ProjectValue(), ok, cells.Format(), prog.captureDependencyChain(makeRef))
	}
	fs := q.IntraWithEntryContext(
		ctx,
		makeRef,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: tSym, Value: product.FromType(typ.NewRecord().Build())}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
	)
	got, ok := flow.PointFactsOf(fs.InPoints[prog.Graph(makeRef).Entry()]).SymbolType(tSym)
	if !ok || !typeHasField(got, "render") {
		t.Fatalf("IntraWithEntryContext T = %v/%v, want render", got, ok)
	}
}

func TestDriverDiagnosticContextsPromoteHelperCallOverClosureFallback(t *testing.T) {
	chunk, err := parse.ParseString(`
type Response = {
    status_code: number,
    body: string?,
}

local json = {}

function json.decode(source: string): (any, string?)
    return {}, nil
end

local http_client = {}

function http_client.get(url: string, options: any?): (Response?, string?)
    return nil, nil
end

local client = {}

local function parse_error_response(http_response)
    local error_info = {
        status_code = http_response.status_code,
        message = "Google API error"
    }

    if http_response.body then
        local parsed, decode_err = json.decode(http_response.body)
        if not decode_err and parsed then
            error_info.metadata = parsed
        end
    end

    return error_info
end

function client.request(method, url, http_options)
    local response, err = http_client.get(url, http_options)
    if not response then
        return nil, err
    end
    if response.status_code < 200 or response.status_code >= 300 then
        return nil, parse_error_response(response)
    end
    return json.decode(response.body)
end

return client
`, "google-context.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("google-context.lua")
	driver.Run(sess, chunk)

	for fn, result := range sess.results {
		if fn == nil || result == nil || result.Graph == nil {
			continue
		}
		symbols := result.Graph.Bindings().SymbolsByName("http_response")
		if len(symbols) != 1 {
			continue
		}
		param := symbols[0]
		if !symbolIn(param, result.Graph.ParamSymbols()) {
			continue
		}
		ref, ok := testRefByGraphID(driver, result.Graph.ID())
		if !ok {
			t.Fatalf("parse_error_response graph %d has no canonical ref", result.Graph.ID())
		}
		contexts := driver.diagnosticContexts[ref]
		if len(contexts) == 0 {
			t.Fatalf("parse_error_response has no diagnostic contexts; diagnostics=%v", sess.DiagnosticsSlice())
		}
		for _, key := range contexts {
			fs, ok := driver.diagnosticStates[key]
			if !ok {
				t.Fatalf("parse_error_response context has no solved diagnostic state; contexts=%v diagnostics=%v", contexts, sess.DiagnosticsSlice())
			}
			av, ok := flow.PointFactsOf(fs.InPoints[result.Graph.Entry()]).SymbolValue(param)
			if !ok {
				av, ok = flow.PointFactsOf(fs.Points[result.Graph.Entry()]).SymbolValue(param)
			}
			if !ok {
				t.Fatalf("parse_error_response entry state missing http_response; contexts=%v diagnostics=%v", contexts, sess.DiagnosticsSlice())
			}
			got := av.ProjectValue()
			if !responseShapeMatches(got) {
				t.Fatalf("parse_error_response slot 0 = %v, want Response; keyValues=%s contexts=%v diagnostics=%v", got, formatEntryValues(key.Values.Values()), contexts, sess.DiagnosticsSlice())
			}
		}
		return
	}
	t.Fatal("parse_error_response graph was not found")
}

func testRefByGraphID(driver *Driver, graphID uint64) (summary.FuncRef, bool) {
	if driver == nil || graphID == 0 {
		return summary.FuncRef{}, false
	}
	for _, ref := range driver.refs {
		if ref.GraphID == graphID {
			return ref, true
		}
	}
	return summary.FuncRef{}, false
}

func typeHasField(t typ.Type, name string) bool {
	if t == nil || name == "" {
		return false
	}
	rec, ok := t.(*typ.Record)
	if !ok {
		return false
	}
	for _, field := range rec.Fields {
		if field.Name == name {
			return true
		}
	}
	for _, member := range rec.StaticMembers {
		if member.Name == name {
			return true
		}
	}
	return false
}

func symbolIn(sym cfg.SymbolID, symbols []cfg.SymbolID) bool {
	for _, candidate := range symbols {
		if candidate == sym {
			return true
		}
	}
	return false
}

func responseShapeMatches(t typ.Type) bool {
	status, ok := fieldType(t, "status_code")
	if !ok || !typ.TypeEquals(status, typ.Number) {
		return false
	}
	body, ok := fieldType(t, "body")
	return ok && typ.TypeEquals(body, typ.NewOptional(typ.String))
}

func fieldType(t typ.Type, name string) (typ.Type, bool) {
	rec, ok := t.(*typ.Record)
	if !ok {
		return nil, false
	}
	for _, field := range rec.Fields {
		if field.Name == name {
			return field.Type, true
		}
	}
	return nil, false
}

func formatEntryValues(values summary.EntryValues) string {
	if len(values) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{")
	first := true
	for slot, av := range values {
		if !first {
			b.WriteString(", ")
		}
		first = false
		fmt.Fprintf(&b, "%d:%v", slot, av.ProjectValue())
	}
	b.WriteString("}")
	return b.String()
}

func describeContextsForSymbol(contexts []summary.Key, sym cfg.SymbolID) string {
	var parts []string
	for _, key := range contexts {
		cellType := "<missing>"
		if av, ok := key.Entry.Cells().Value(sym); ok {
			cellType = typ.FormatShort(av.ProjectValue())
		}
		var refs []string
		for _, path := range constraint.SortedPathKeys(key.Refs.Refs()) {
			refs = append(refs, fmt.Sprintf("%s=%s", path, key.Refs.Refs()[path].Format()))
		}
		parts = append(parts, fmt.Sprintf("{cell:%s refs:[%s] closures:%s}", cellType, strings.Join(refs, ","), key.Closures.Format()))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func describeSummaryCaptureExports(driver *Driver) string {
	if driver == nil {
		return "[]"
	}
	var parts []string
	for _, ref := range driver.refs {
		sum, ok := driver.summaries[ref]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%v:%s", ref, sum.CaptureExports.Format()))
	}
	return "[" + strings.Join(parts, " ") + "]"
}
