package render

import (
	"strings"
	"testing"
)

func TestFormatTableSingleSnapshotHasNoDeltaColumn(t *testing.T) {
	labels := []Labeled{
		{Commit: "A", Report: Report{
			DomainAreas: []AreaLOC{{Name: "value", LOC: LOC{NonTest: 10, Test: 5}}},
			DomainTotal: LOC{NonTest: 10, Test: 5},
		}},
	}
	out := FormatTable(labels)
	if strings.Contains(out, "delta") {
		t.Errorf("single-snapshot table should have no delta column, got:\n%s", out)
	}
	if !strings.Contains(out, "domain/value LOC (nt/t)") {
		t.Errorf("table missing domain area row, got:\n%s", out)
	}
	if !strings.Contains(out, "10/5") {
		t.Errorf("table missing domain area value, got:\n%s", out)
	}
}

func TestFormatTableDeltaAndUnionOfAreas(t *testing.T) {
	labels := []Labeled{
		{Commit: "A", Report: Report{
			DomainAreas: []AreaLOC{{Name: "value", LOC: LOC{NonTest: 10, Test: 5}}},
			DomainTotal: LOC{NonTest: 10, Test: 5},
		}},
		{Commit: "B", Report: Report{
			DomainAreas: []AreaLOC{
				{Name: "value", LOC: LOC{NonTest: 12, Test: 5}},
				{Name: "newarea", LOC: LOC{NonTest: 3, Test: 0}},
			},
			DomainTotal: LOC{NonTest: 15, Test: 5},
		}},
	}
	out := FormatTable(labels)
	if !strings.Contains(out, "delta A->B") {
		t.Errorf("table missing delta header, got:\n%s", out)
	}
	if !strings.Contains(out, "domain/newarea LOC (nt/t)") {
		t.Errorf("table missing area only present in second snapshot, got:\n%s", out)
	}
	// newarea is absent from A, so its row must read 0/0 for A and 3/0 for B
	// with a +3/+0 delta.
	lines := strings.Split(out, "\n")
	var newareaLine string
	for _, l := range lines {
		if strings.Contains(l, "domain/newarea") {
			newareaLine = l
		}
	}
	if newareaLine == "" {
		t.Fatal("newarea row not found")
	}
	fields := strings.Fields(newareaLine)
	// fields: name "0/0" "3/0" "+3/+0"
	if fields[len(fields)-3] != "0/0" || fields[len(fields)-2] != "3/0" || fields[len(fields)-1] != "+3/+0" {
		t.Errorf("newarea row = %q", newareaLine)
	}
}
