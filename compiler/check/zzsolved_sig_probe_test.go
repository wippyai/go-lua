package check

import (
	"testing"

	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestZZSolvedSigProbe(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"build_page", `
type Entry = { id: string, data: {[string]: any} }
local pages = {}
local function qualify_id(entry_id, relative_id)
    return entry_id .. ":" .. relative_id
end
function pages.build_page(entry: Entry)
    local data_func = entry.data.data_func
    if data_func and data_func ~= "" then
        data_func = qualify_id(entry.id, data_func)
    end
    local page = {}
    page.data_func = data_func
    return page
end
return pages
`},
		{"request", `
type Response = { metadata: { response_id: string } }
local M = {}
function M.request(ok: boolean): (Response?, string?)
    if ok then
        return { metadata = { response_id = "x" } }, nil
    end
    return nil, "failed"
end
return M
`},
	}

	for _, mode := range []FlowMode{FlowCanonical, FlowLegacy} {
		for _, tc := range cases {
			checker := newSessionTestChecker(nil)
			checker.flowMode = mode
			sess := checker.Check(tc.src, "mod.lua")

			root := sess.RootResultValue()
			if root == nil {
				t.Fatalf("[%v/%s] no root result", mode, tc.name)
			}
			for _, def := range root.Evidence.FunctionDefinitions {
				if def.Nested.Func == nil {
					continue
				}
				t.Logf("[mode=%v %s] def=%q IsLocal=%v sym=%d", mode, tc.name, def.Name, def.IsLocal, def.Symbol)
				res := sess.Results[def.Nested.Func]
				if res == nil {
					t.Logf("[mode=%v %s] def=%q no per-func result", mode, tc.name, def.Name)
					continue
				}
				sig := functionfact.SolvedSignatureFromResult(res, def.Nested.Func)
				fn := unwrap.Function(sig)
				t.Logf("[mode=%v %s] def=%q solved=%s errRet=%v", mode, tc.name, def.Name, sig, fn != nil && erreffect.HasErrorReturnLabel(fn))

				// Probe: per-function NarrowSynth ObservedSummary.
				if res.NarrowSynth != nil {
					obs := abstractreturns.ObservedSummary(res.Graph, res.Evidence.Returns, nil, res.NarrowSynth)
					t.Logf("[mode=%v %s] def=%q narrowSynth-observed=%v", mode, tc.name, def.Name, obs)
					// Per-return-point classification probe for the ErrorReturn proof.
					for _, ret := range res.Evidence.Returns {
						if ret.Info == nil {
							continue
						}
						vals := abstractreturns.ExpandValues(ret.Info.Exprs, 2, ret.Point, res.NarrowSynth)
						t.Logf("[mode=%v %s/%s] ret@%v exprs=%d expand2=%v", mode, tc.name, def.Name, ret.Point, len(ret.Info.Exprs), vals)
					}
					conv := erreffect.CanonicalLuaValueErrorConvention()
					proof := conv.ProveReturnPattern(res.Evidence.Returns, res.FlowSolution, res.NarrowSynth)
					t.Logf("[mode=%v %s] def=%q proof=%+v", mode, tc.name, def.Name, proof)
				}
			}
		}
	}
}
