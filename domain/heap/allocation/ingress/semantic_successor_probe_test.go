package ingress

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
)

// IngressResultForTest is an internal-test-only bridge to the production
// zero-input WorldZero seed constructor.
func IngressResultForTest(schema heapdomain.Schema, operand source.Root) (heapdomain.Key, heapdomain.Value, bool) {
	if !operand.FencedTo(schema) {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	key := operand.Key()
	fact, outcome := heapdomain.IngressFact(key)
	return key, fact, outcome == structure.Concrete
}
