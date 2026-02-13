package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// Regression guard: inferred assert-style refinements from module source must
// survive export -> manifest encoding -> manifest decoding so downstream module
// consumers still get narrowing at call sites.
func TestRegression_ModuleExportPreservesInferredNotNilRefinement(t *testing.T) {
	moduleSource := `
local M = {}

function M.not_nil(v, msg)
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

return M
`

	exported := testutil.CheckAndExport(moduleSource, "assert_mod", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumer := `
local assert_mod = require("assert_mod")

local function maybe_string(): string?
	if _G.flag then
		return "ok"
	end
	return nil
end

local x = maybe_string()
assert_mod.not_nil(x, "x is required")
return x:sub(1, 1)
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithManifest("assert_mod", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors with imported inferred refinement, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
