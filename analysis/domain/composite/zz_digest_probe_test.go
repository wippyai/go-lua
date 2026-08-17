package composite

import "testing"

func TestZZDigestProbe(t *testing.T) {
	table, failure := Table()
	if table == nil {
		t.Fatalf("no table: %v", failure)
	}
	t.Logf("DIGEST=%x", table.Digest())
	for position := 0; position < AxisCount(); position++ {
		principal, _ := AxisPrincipalAt(position)
		declared, _ := AxisMountDeclared(principal)
		t.Logf("AXIS %d principal=%v mount=%v", position, principal, declared)
	}
}
