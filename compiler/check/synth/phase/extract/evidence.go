package extract

import (
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
)

func (s *Synthesizer) graphEvidence(graph *compcfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	if s != nil && s.deps != nil && !s.deps.Evidence.IsZero() {
		if s.deps.CheckCtx != nil {
			if current, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok && current == graph {
				return s.deps.Evidence
			}
		}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}
