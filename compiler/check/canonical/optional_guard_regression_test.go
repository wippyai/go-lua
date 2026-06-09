package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestOptionalGuardNarrowingRegression(t *testing.T) {
	cases := map[string]struct {
		src          string
		wantClean    bool
		wantContains string
	}{
		"if-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
local function run(f: Cb?)
    if f then
        f()
    end
end
return run
`,
		},
		"if-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc) }
local function run(s: Svc?)
    if s then
        s:go()
    end
end
return run
`,
		},
		"and-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc): boolean }
local function run(s: Svc?)
    local ok = s and s:go()
end
return run
`,
		},
		"if-and-index": {
			wantClean: true,
			src: `
type Row = { exists: boolean }
type QR = { [number]: Row }
local function run(result: QR?)
    if result and result[1] then
        local r = result[1].exists
    end
end
return run
`,
		},
		"if-and-index-array": {
			wantClean: true,
			src: `
type QueryResult = {[string]: any}
local function run(result: {QueryResult}?)
    if result and result[1] then
        local r = result[1].exists
    end
end
return run
`,
		},
		"index-guard-does-not-leak-sibling": {
			wantContains: "cannot index optional value",
			src: `
type QueryResult = {[string]: any}
local function run(result: {QueryResult})
    if result[1] then
        local a = result[1]["k"]
        local b = result[3]["k"]
    end
end
return run
`,
		},
		"if-field-then-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
type Obj = { cb: Cb? }
local function run(o: Obj)
    if o.cb then
        o.cb()
    end
end
return run
`,
		},
		"local-from-field-if-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc) }
type Holder = { store: Svc? }
local function run(h: Holder)
    local store = h.store
    if store then
        store:go()
    end
end
return run
`,
		},
		"upvalue-if-method": {
			wantClean: true,
			src: `
type Svc = { lookup: fun(self: Svc, k: string): boolean }
type Holder = { store: Svc? }
local function build(h: Holder)
    local store = h.store
    return function(token: string)
        if store then
            local snap = store:lookup(token)
        end
    end
end
return build
`,
		},
		"upvalue-if-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
type Holder = { cb: Cb? }
local function build(h: Holder)
    local cb = h.cb
    return function()
        if cb then
            cb()
        end
    end
end
return build
`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.wantClean {
				requireCanonicalClean(t, tc.src)
				return
			}
			requireCanonicalDiagnosticContains(t, tc.src, tc.wantContains)
		})
	}
}

func TestOptionalGuardCopiedLocalMethodReceiverStateAtCall(t *testing.T) {
	svc := typ.NewRecord().
		Field("go", typ.Func().Param("self", typ.Self).Build()).
		Build()
	holder := typ.NewRecord().
		Field("store", typ.NewOptional(svc)).
		Build()

	fs, g := solveFn(t,
		[]string{"h"},
		[]ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "Holder"}},
		func(expr ast.TypeExpr, _ *scope.State) typ.Type {
			if prim, ok := expr.(*ast.PrimitiveTypeExpr); ok && prim.Name == "Holder" {
				return holder
			}
			return nil
		},
		`
local store = h.store
if store then
    store:go()
end
`)
	var checked bool
	g.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if checked || info == nil || info.Call == nil || info.Call.Method != "go" {
			return
		}
		receiver, ok := info.Call.Receiver.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("method receiver = %T, want identifier", info.Call.Receiver)
		}
		sym, ok := g.Bindings().SymbolOf(receiver)
		if !ok {
			t.Fatalf("method receiver %q has no symbol", receiver.Value)
		}
		av, ok := flow.PointFactsOf(fs.InPoints[p]).SymbolValue(sym)
		if !ok || !typ.TypeEquals(av.ProjectValue(), svc) {
			t.Fatalf("call-point receiver value = %v/%v, want non-optional Svc", av.ProjectValue(), ok)
		}
		checked = true
	})
	if !checked {
		t.Fatal("test did not find store:go call site")
	}
}

func TestOptionalGuardCopiedLocalMethodReceiverObservationAtCall(t *testing.T) {
	res := testutil.Check(`
type Svc = { go: fun(self: Svc) }
type Holder = { store: Svc? }
local function run(h: Holder)
    local store = h.store
    if store then
        store:go()
    end
end
return run
`)
	fn := findFunctionWithParamNames(t, res.Session.Results, "h")
	storeSym := singleSymbolNamed(t, fn.Graph, "store")
	var checked bool
	fn.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if checked || info == nil || info.Call == nil || info.Call.Method != "go" {
			return
		}
		receiver, ok := info.Call.Receiver.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("method receiver = %T, want identifier", info.Call.Receiver)
		}
		flowType := fn.NarrowedTypeAt(p, constraint.NewPath(storeSym, "store"))
		synthType := fn.SolvedSynth().Narrow().TypeOf(receiver, p)
		if _, optional := typ.SplitNilableFieldType(flowType); optional {
			t.Fatalf("flow receiver type at call = %v, want non-optional", flowType)
		}
		if _, optional := typ.SplitNilableFieldType(synthType); optional {
			t.Fatalf("synth receiver type at call = %v, flow=%v diagnostics=%v", synthType, flowType, testutil.ErrorMessages(res.Diagnostics))
		}
		checked = true
	})
	if !checked {
		t.Fatal("test did not find store:go call site")
	}
}
