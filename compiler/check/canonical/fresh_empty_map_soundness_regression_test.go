package canonical_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFreshEmptyForeignMapWriteDoesNotProvePairsValueTail(t *testing.T) {
	src := `
local time = require("time")

type ActiveSession = {
	created_at: time.Time,
	last_activity: time.Time?,
}

local state = {
	active_sessions = {},
}

local now = time.now()

state.active_sessions["s1"] = {
	created_at = now,
	last_activity = now,
}

for _, session_info in pairs(state.active_sessions) do
	local last_activity = session_info.last_activity or session_info.created_at
	local elapsed = now:sub(last_activity)
	return elapsed:seconds()
end

return 0
`
	res := testutil.Check(src,
		testutil.WithStdlib(),
		testutil.WithManifest("time", canonicalTimeManifest()),
	)
	root := res.Session.RootResult
	if root == nil || root.Graph == nil {
		t.Fatal("missing canonical root result")
	}

	obs := observation.FromFuncResult(root, nil).WithProofValues()
	var sawSub bool
	root.Graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.Call == nil {
			return
		}
		if ident, ok := info.Call.Func.(*ast.IdentExpr); ok && ident.Value == "pairs" && len(info.Call.Args) > 0 {
			source := obs.TypeOf(info.Call.Args[0], p)
			iter := querycore.EntryValueType(source)
			if !typ.TypeEquals(iter, typ.Any) {
				t.Fatalf("pairs(state.active_sessions) entry value = %v, want any; source=%v", iter, source)
			}
			return
		}
		if info.Call.Method == "sub" && len(info.Call.Args) > 0 {
			sawSub = true
			arg := obs.TypeOf(info.Call.Args[0], p)
			ident, ok := info.Call.Args[0].(*ast.IdentExpr)
			if !ok {
				t.Fatalf("sub argument = %T, want ident", info.Call.Args[0])
			}
			sym, ok := root.Graph.Bindings().SymbolOf(ident)
			if !ok || sym == 0 {
				t.Fatalf("sub argument %q has no symbol", ident.Value)
			}
			if facts, ok := root.Facts.(flow.ProductFacts); ok {
				if pathFacts, ok := root.Facts.(flow.ProductPathFacts); ok {
					stateSyms := root.Graph.Bindings().SymbolsByName("state")
					if len(stateSyms) != 1 {
						t.Fatalf("state symbols = %v, want one", stateSyms)
					}
					activePath := constraint.NewPath(stateSyms[0], "state").Field("active_sessions")
					activePV := pathFacts.RefinedPathValueAt(p, activePath)
					activeType := product.ProjectValueOrUnknown(activePV.Value)
					if activePV.State != flow.StateResolved || querycore.EntryValueType(activeType) == nil {
						t.Fatalf("product path fact for state.active_sessions = state %v value %v, want iterable", activePV.State, activeType)
					}
					if iter := querycore.EntryValueType(activeType); !typ.TypeEquals(iter, typ.Any) {
						t.Fatalf("state.active_sessions iterator value in product path = %v, want any; source=%v", iter, activeType)
					}
				} else {
					t.Fatal("root facts do not expose product path facts")
				}
				sessionSyms := root.Graph.Bindings().SymbolsByName("session_info")
				if len(sessionSyms) != 1 {
					t.Fatalf("session_info symbols = %v, want one", sessionSyms)
				}
				sessionPV := facts.RefinedValueAt(p, sessionSyms[0])
				sessionType := product.ProjectValueOrUnknown(sessionPV.Value)
				if sessionPV.State != flow.StateResolved || !typ.TypeEquals(sessionType, typ.Any) {
					t.Fatalf("product fact for session_info = state %v value %v, want resolved any", sessionPV.State, sessionType)
				}
				pv := facts.RefinedValueAt(p, sym)
				pt := product.ProjectValueOrUnknown(pv.Value)
				if pv.State != flow.StateResolved || !typ.TypeEquals(pt, typ.Any) {
					t.Fatalf("product fact for last_activity = state %v value %v, want resolved any", pv.State, pt)
				}
				if pv.Value.IsGradualTop() {
					t.Fatalf("product fact for last_activity is gradual-top any; want strict any from untyped map tail")
				}
			} else {
				t.Fatal("root facts do not expose product facts")
			}
			if facts, ok := root.Facts.(flow.PathObservationFacts); ok {
				path := constraint.NewPath(sym, ident.Value)
				pathObs := facts.ObservePath(flow.PathObservationQuery{
					Point:         p,
					Path:          path,
					View:          flow.PathReadCurrent,
					PreserveProof: true,
				})
				if !pathObs.Resolved() || !typ.TypeEquals(pathObs.Type, typ.Any) {
					t.Fatalf("path observation for last_activity = %#v, want resolved strict any", pathObs)
				}
			} else {
				t.Fatal("root facts do not expose path observation facts")
			}
			if !typ.TypeEquals(arg, typ.Any) {
				t.Fatalf("sub(last_activity) argument type = %v, want any", arg)
			}
		}
	})
	if !sawSub {
		t.Fatal("missing time:sub call site")
	}

	for _, msg := range testutil.ErrorMessages(res.Diagnostics) {
		if strings.Contains(msg, "argument 1: expected time.Time, got any") {
			return
		}
	}
	t.Fatalf("expected time.Time vs any diagnostic, got %v", testutil.ErrorMessages(res.Diagnostics))
}
