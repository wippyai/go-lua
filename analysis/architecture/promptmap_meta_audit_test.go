package architecture

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptmapMetaAuditMatrixIsPresentAndWellFormed(t *testing.T) {
	path := filepath.Join(repoRoot(t), "analysis", "architecture", "promptmap_meta_audit.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open promptmap meta audit matrix: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read promptmap meta audit matrix: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("promptmap meta audit matrix has %d rows, want header plus at least one rule", len(rows))
	}
	header := rows[0]
	required := []string{
		"id",
		"title",
		"expected_owner",
		"allowed_surfaces",
		"banned_surfaces",
		"deterministic_probe",
		"scope_dir",
		"ext",
		"mode",
		"match",
		"exclude",
		"filter_prompt",
		"deep_prompt",
		"agent_prompt",
	}
	for _, name := range required {
		if !csvHeaderHas(header, name) {
			t.Fatalf("promptmap meta audit matrix missing required column %q", name)
		}
	}
	for rowIndex, row := range rows[1:] {
		if len(row) != len(header) {
			t.Fatalf("row %d has %d fields, want %d", rowIndex+2, len(row), len(header))
		}
		values := map[string]string{}
		for i, name := range header {
			values[name] = row[i]
		}
		for _, name := range []string{"id", "scope_dir", "mode", "filter_prompt"} {
			if values[name] == "" {
				t.Fatalf("row %d column %q is empty", rowIndex+2, name)
			}
		}
		if values["mode"] != "refine" && values["mode"] != "agentscan" {
			t.Fatalf("row %d mode = %q, want refine or agentscan", rowIndex+2, values["mode"])
		}
		if values["mode"] == "refine" && values["deep_prompt"] == "" {
			t.Fatalf("row %d refine rule has empty deep_prompt", rowIndex+2)
		}
	}
}

func csvHeaderHas(header []string, name string) bool {
	for _, got := range header {
		if got == name {
			return true
		}
	}
	return false
}
