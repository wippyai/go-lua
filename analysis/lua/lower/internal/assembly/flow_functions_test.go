package assembly

import "testing"

func TestAssemblyFunctionRowsCloseAgainstNestedBody(t *testing.T) {
	c := newAssemblyCollector()
	owner := c.Body(assemblyTestSpan())
	function := c.DeclareFunction(assemblyTestSpan(), owner)
	body := c.Body(assemblyTestSpan())
	if function == 0 || body == 0 || !c.FillFunction(function, body, nil, 0, nil) {
		t.Fatalf("function construction failed: function=%d body=%d", function, body)
	}
}
