package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

type unusedLocals struct{}

func (unusedLocals) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	reachable := collectDiagnosticReachability(result, graph)
	readsByPoint := collectReachableSymbolReads(result, graph, reachable)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !diagnosticPointReachable(reachable, point) {
			continue
		}
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || ignoredUnusedLocalName(fact.Name) {
			continue
		}
		if symbolHasReachableRead(readsByPoint, fact.Symbol) {
			continue
		}
		out = append(out, unusedLocalDiagnostic(fact))
	}
	return out
}

func ignoredUnusedLocalName(name string) bool {
	return name == "" || strings.HasPrefix(name, "_")
}

func unusedLocalDiagnostic(fact semantics.LocalAssignmentFact) diagnostic.Diagnostic {
	span := localNameSpan(fact.Stmt, fact.Index, fact.Name)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeUnusedLocal,
		Severity:    diagnostic.SeverityWarning,
		Message:     unusedLocalMessage(fact.Name),
		Explanation: unusedLocalExplanation(span, fact.Name),
		Help:        unusedLocalHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnusedLocal)},
	})
}

func unusedLocalExplanation(span diagnostic.Span, name string) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Message: unusedLocalEvidence(name),
		},
	)
}
