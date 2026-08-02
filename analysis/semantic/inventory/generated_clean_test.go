package inventory_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/inventory"
	"github.com/wippyai/go-lua/analysis/semantic/inventory/internal/gen"
)

func TestGeneratedInventoryIsClean(t *testing.T) {
	dir := inventoryDir(t)
	in, err := inventory.Base()
	if err != nil {
		t.Fatal(err)
	}
	bindingFile, err := os.Open(filepath.Join(dir, "bindings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := gen.DecodeBindings(bindingFile)
	if closeErr := bindingFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	want, err := gen.RenderRegistry(in, bindings)
	if err != nil {
		t.Fatal(err)
	}
	assertGeneratedFile(t, filepath.Join(dir, "..", "..", "domain", "value", "registry", "zz_generated_registry.go"), want)
}

func inventoryDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func assertGeneratedFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("generated file %s is stale; run go generate ./analysis/semantic/inventory", path)
	}
}
