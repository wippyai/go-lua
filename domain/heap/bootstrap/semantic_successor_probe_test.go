package bootstrap

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// ResultForSchemaTest is an internal-test-only bridge to Bootstrap's complete
// preissued value constructor, including its header and raw-cell ledger.
func ResultForSchemaTest(schema heapdomain.Schema, root Root) (heapdomain.Key, heapdomain.Value, bool) {
	return resultForSchema(schema, root)
}
