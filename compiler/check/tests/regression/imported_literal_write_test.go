package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
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
		{`local names = require("names"); local alias = names; local value: "first" = names.NAME`, false},
		{`local names = require("names"); names.NAME = "second"; local value: "first" = names.NAME`, true},
	} {
		result := testutil.Check(tt.source, testutil.WithStdlib(), testutil.WithManifest("names", exported.Manifest))
		if result.HasError() != tt.errorExpected {
			t.Fatalf("errorExpected=%v diagnostics=%v", tt.errorExpected, testutil.ErrorMessages(result.Diagnostics))
		}
	}
}

func TestImportedSingletonFieldWidensThroughAliasAndEscape(t *testing.T) {
	exported := testutil.CheckAndExport(`return { NAME = "first" }`, "names", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("export: %v", testutil.ErrorMessages(exported.Errors))
	}
	for _, source := range []string{
		`local names = require("names"); local alias = names; alias.NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local alias = names; local other = alias; other.NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local function mutate(x) x.NAME = "second" end; mutate(names); local value: "first" = names.NAME`,
		`local names = require("names"); local holder = {names}; holder[1].NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local function mutate() names.NAME = "second" end; mutate(); local value: "first" = names.NAME`,
	} {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("names", exported.Manifest), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if !result.HasError() {
				t.Errorf("strict=%v expected diagnostic for %s", strict, source)
			}
		}
	}
}

func TestNilValuedExtraFieldSatisfiesMap(t *testing.T) {
	for _, strict := range []bool{false, true} {
		result := testutil.Check(`local function f(x: {[string]: integer}) end; f({extra=nil})`, testutil.WithCheckOptions(check.Options{Strict: strict}))
		if result.HasError() {
			t.Errorf("strict=%v: %v", strict, testutil.ErrorMessages(result.Errors))
		}
	}
}
