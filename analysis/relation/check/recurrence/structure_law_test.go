package recurrence

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRecurrenceDoesNotReconstructSemanticContracts is a nearest-layer law:
// recurrence consumes sealed semantic bindings but never re-seals or
// reverse-engineers them.  Monotonicity is a binding/certificate obligation;
// graph recurrence owns only logical positivity and SCC policy.
func TestRecurrenceDoesNotReconstructSemanticContracts(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate recurrence package")
	}
	directory := filepath.Dir(file)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read recurrence package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		text := string(body)
		if strings.Contains(text, "signature"+"."+"Seal") {
			t.Fatalf("%s reconstructs a semantic signature", entry.Name())
		}
		if strings.Contains(text, "per"+"mutation") || strings.Contains(text, "re"+"coverStrictness") {
			t.Fatalf("%s reverse-engineers semantic contract fields", entry.Name())
		}
	}
}
