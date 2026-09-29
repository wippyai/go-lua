package regression

import (
	"strings"
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
	exported := testutil.CheckAndExport(`return { NAME = "first", touch = function(self) end }`, "names", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("export: %v", testutil.ErrorMessages(exported.Errors))
	}
	for _, source := range []string{
		`local names = require("names"); local alias = names; alias.NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local alias = names; names.NAME = "second"; local value: "first" = alias.NAME`,
		`local names = require("names"); local alias = names; local other = alias; other.NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local function sink(x: any) end; sink(names); local value: "first" = names.NAME`,
		`local names = require("names"); names:touch(); local value: "first" = names.NAME`,
		`local names = require("names"); local holder = {names}; holder[1].NAME = "second"; local value: "first" = names.NAME`,
		`local names = require("names"); local function mutate() names.NAME = "second" end; mutate(); local value: "first" = names.NAME`,
	} {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("names", exported.Manifest), testutil.WithCheckOptions(check.Options{Strict: strict}))
			messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
			if !strings.Contains(messages, `cannot assign string to "first"`) {
				t.Errorf("strict=%v expected singleton assignment diagnostic for %s, got %s", strict, source, messages)
			}
		}
	}
}

func TestNilValuedExtraFieldSatisfiesMap(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, source := range []string{
			`local function f(x: {[string]: integer}) end; f({extra=nil})`,
			`local function f(x: {[string]: integer}) end; local extra: integer? = 1; f({extra=extra})`,
		} {
			result := testutil.Check(source, testutil.WithCheckOptions(check.Options{Strict: strict}))
			if result.HasError() {
				t.Errorf("strict=%v source=%s: %v", strict, source, testutil.ErrorMessages(result.Errors))
			}
		}
	}
}
