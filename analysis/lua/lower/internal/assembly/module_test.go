package assembly

import "testing"

func TestAssemblyModuleAliasRequiresReservedImport(t *testing.T) {
	c := newAssemblyCollector()
	if c.SetImportAlias(0, 0) || c.err == nil {
		t.Fatal("SetImportAlias accepted an import outside the census")
	}
}
