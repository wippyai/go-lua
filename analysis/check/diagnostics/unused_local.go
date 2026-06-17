package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type unusedLocals producerContext

func (p unusedLocals) Produce(result *body.Result) []diagnostic.Diagnostic {
	_ = p
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || ignoredUnusedLocalName(fact.Name) {
			continue
		}
		if result.SymbolHasRead(fact.Symbol) {
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
	span := ast.SpanOf(fact.Stmt)
	message := fmt.Sprintf("local %q is never read", fact.Name)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:        span,
		Code:        CodeUnusedLocal,
		Severity:    diagnostic.SeverityWarning,
		Message:     message,
		Explanation: unusedLocalExplanation(span, fact.Name),
		Labels:      []diagnostic.Label{{Span: span, Message: "unused local declaration"}},
		Help:        "Remove the local, use it, or prefix its name with _ to mark it intentionally unused.",
	}
}

func unusedLocalExplanation(span diagnostic.Span, name string) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: fmt.Sprintf("local %q is declared here", name),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: fmt.Sprintf("no identifier read is bound to local %q", name),
		},
	)
}
