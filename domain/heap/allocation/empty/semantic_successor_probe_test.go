package empty

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
)

// EmptyResultForTest is an internal-test-only bridge to Empty's production
// semantic successor. The bridge itself carries no alternate implementation.
func EmptyResultForTest(schema heapdomain.Schema, operand source.Root, predecessor heapdomain.Value) (heapdomain.Key, heapdomain.Value, bool) {
	return emptyResult(schema, operand, predecessor)
}
