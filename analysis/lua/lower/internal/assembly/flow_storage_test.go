package assembly

import "testing"

func TestAssemblyStorageRowsLinkReadsToLocalCells(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	cell := c.Cell(assemblyTestSpan(), body)
	read := c.Read(assemblyTestSpan(), body, cell)
	if cell == 0 || read == 0 {
		t.Fatalf("storage rows rejected a local Cell read: cell=%d read=%d", cell, read)
	}
}
