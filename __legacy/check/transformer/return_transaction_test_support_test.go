package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func testReturnTransactionTerm(t *testing.T, point cfg.Point, terms ...ValueTerm) returnTransactionTerm {
	t.Helper()
	sources := make([]factflow.ValueSource, len(terms))
	for index := range sources {
		sources[index] = factflow.NewUnknownValueSource(index)
	}
	transaction, exact := factapply.PlanReturnTransactionSources(factflow.Facts{}, point, sources)
	if !exact {
		t.Fatal("could not freeze test N5 transaction")
	}
	return returnTransactionTerm{transaction: transaction, sources: append([]ValueTerm(nil), terms...)}
}
