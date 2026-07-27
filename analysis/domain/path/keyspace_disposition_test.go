package path

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZeroClientKeyspacePackageDoesNotReturn(t *testing.T) {
	entries, err := os.ReadDir("keyspace")
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" &&
			!strings.HasSuffix(entry.Name(), "_test.go") {
			t.Fatalf("analysis/domain/path/keyspace production file %q returned without a buildable consumer", entry.Name())
		}
	}
}
