package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblyCallCoordinatesCalleeAndActualValues(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	callee := c.String(assemblyTestSpan(), body, "callable")
	actuals := c.Values(assemblyTestSpan(), body, []keyspace.Term{callee}, 0)
	call := c.DeclareCall(assemblyTestSpan(), body, callee, 0, actuals)
	if call == 0 || !c.SetCallTypeArgs(call, nil) {
		t.Fatalf("call construction failed: call=%d", call)
	}
}
