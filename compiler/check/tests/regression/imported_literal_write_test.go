package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestImportedSingletonFieldWidensWhenWritten(t *testing.T) {
	exported := testutil.CheckAndExport(`return { NAME = "first" }`, "names", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("export: %v", testutil.ErrorMessages(exported.Errors))
	}
	for _, tt := range []struct {
		source        string
		errorExpected bool
	}{
		{`local names = require("names"); local value: "first" = names.NAME`, false},
		{`local names = require("names"); names.NAME = "second"; local value: "first" = names.NAME`, true},
	} {
		result := testutil.Check(tt.source, testutil.WithStdlib(), testutil.WithManifest("names", exported.Manifest))
		if result.HasError() != tt.errorExpected {
			t.Fatalf("errorExpected=%v diagnostics=%v", tt.errorExpected, testutil.ErrorMessages(result.Diagnostics))
		}
	}
}
