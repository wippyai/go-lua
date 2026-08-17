package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblyTableRowsCloseOneFieldRange(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	table := c.DeclareTable(assemblyTestSpan(), body)
	key := c.Name(assemblyTestSpan(), body, "field")
	value := c.Integer(assemblyTestSpan(), body, 1)
	values := c.Values(assemblyTestSpan(), body, []keyspace.Term{value}, 0)
	field := c.TableField(assemblyTestSpan(), table, key, values, kind.FieldName)
	if table == 0 || field == 0 || !c.FillTable(table, []keyspace.Term{field}) {
		t.Fatalf("table construction failed: table=%d field=%d", table, field)
	}
}
