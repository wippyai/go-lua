package ingress

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
)

// IngressResultForTest is an internal-test-only bridge to the production
// zero-input WorldZero seed constructor.
func IngressResultForTest(schema heapdomain.Schema, operand source.Root) (heapdomain.Key, heapdomain.Value, bool) {
	return ingressResult(schema, operand)
}
