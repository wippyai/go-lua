package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
)

// diagnosticQuery is the diagnostic producer boundary for solved analysis
// facts. Producers should ask this facade for user-facing proof/type views
// instead of constructing lower-level read models or choosing boundary read
// mechanics directly.
type diagnosticQuery struct {
	reader readmodel.Reader
}

func newDiagnosticQuery(result *body.Result, parents ...*body.Result) diagnosticQuery {
	return diagnosticQuery{
		reader: readmodel.NewWithParents(result, parents...),
	}
}
