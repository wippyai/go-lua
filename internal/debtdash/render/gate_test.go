package render

import "testing"

func belowCeilingReport() Report {
	return Report{
		DomainTotal:    LOC{NonTest: domainNonTestCeiling - 1, Test: domainTestCeiling - 1},
		ExportedEngine: engineExportedCeiling - 1,
		ExportedSchema: schemaExportedCeiling - 1,
	}
}

func TestGatePassesAtCeiling(t *testing.T) {
	g := Gate(belowCeilingReport())
	if g.Overall != GatePass {
		t.Fatalf("Overall = %s, want %s; criteria: %+v", g.Overall, GatePass, g.Criteria)
	}
	for _, c := range g.Criteria {
		if c.Status != GatePass {
			t.Errorf("criterion %q = %s, want PASS", c.Name, c.Status)
		}
	}
}

func TestGateFailsOnLOCCeiling(t *testing.T) {
	r := belowCeilingReport()
	r.DomainTotal.NonTest = domainNonTestCeiling
	g := Gate(r)
	if g.Overall != GateFail {
		t.Fatalf("Overall = %s, want %s", g.Overall, GateFail)
	}
	found := false
	for _, c := range g.Criteria {
		if c.Name == "domain non-test LOC" {
			found = true
			if c.Status != GateFail {
				t.Errorf("domain non-test LOC status = %s, want FAIL", c.Status)
			}
		}
	}
	if !found {
		t.Fatal("domain non-test LOC criterion not present")
	}
}

func TestGateFailsOnResidualLedgerRow(t *testing.T) {
	r := belowCeilingReport()
	r.ScheduledDeathRows = 1
	g := Gate(r)
	if g.Overall != GateFail {
		t.Fatalf("Overall = %s, want %s", g.Overall, GateFail)
	}
}

func TestGateFailsOnAnyResidueFile(t *testing.T) {
	r := belowCeilingReport()
	r.ResidueFiles = 3
	g := Gate(r)
	if g.Overall != GateFail {
		t.Fatalf("Overall = %s, want %s", g.Overall, GateFail)
	}
}

func TestFormatGateIncludesOverallLine(t *testing.T) {
	g := Gate(belowCeilingReport())
	out := FormatGate(g)
	if !containsLine(out, "GATE: PASS") {
		t.Errorf("FormatGate output missing overall line, got:\n%s", out)
	}
}

func containsLine(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
