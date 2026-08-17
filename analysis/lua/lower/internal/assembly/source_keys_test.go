package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblySourceKeysKeepOwnerAndSpelling(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	name := c.Name(assemblyTestSpan(), body, "field")
	list := c.List(assemblyTestSpan(), body, 1)
	if keyspace.TermFamily(name) != keyspace.FamilyKey || keyspace.TermFamily(list) != keyspace.FamilyKey || name == list {
		t.Fatalf("source key terms = %d/%d, want distinct Key terms", name, list)
	}
}
