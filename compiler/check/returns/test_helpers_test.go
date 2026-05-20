package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

func callEvidenceForGraph(graph *cfg.Graph) []api.CallEvidence {
	if graph == nil {
		return nil
	}
	var calls []api.CallEvidence
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		calls = append(calls, api.CallEvidence{Point: p, Info: info})
	})
	return calls
}
