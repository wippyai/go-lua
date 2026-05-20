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
	if s != nil && s.deps != nil && !flowEvidenceEmpty(s.deps.Evidence) {
		if s.deps.CheckCtx != nil {
			if current, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok && current == graph {
				return s.deps.Evidence
			}
		}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}

func flowEvidenceEmpty(e api.FlowEvidence) bool {
	return len(e.Calls) == 0 &&
		len(e.Returns) == 0 &&
		len(e.Assignments) == 0 &&
		len(e.Branches) == 0 &&
		!e.NormalExit.Valid &&
		len(e.IdentifierUses) == 0 &&
		len(e.FieldDefaults) == 0 &&
		len(e.FreshTableLiterals) == 0 &&
		len(e.FunctionDefinitions) == 0 &&
		len(e.EscapedFunctions) == 0 &&
		len(e.LocalTypePredicates) == 0 &&
		len(e.CapturedFields) == 0 &&
		len(e.CapturedContainers) == 0
}
