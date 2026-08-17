package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblyReturnLinksValuesToOwningBody(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	value := c.Integer(assemblyTestSpan(), body, 1)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{value}, 0)
	if term := c.Return(assemblyTestSpan(), body, values); term == 0 {
		t.Fatal("Return rejected an authored Values range")
	}
}
