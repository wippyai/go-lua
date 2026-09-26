package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestDeclaredVsInferredAbsentField pins the first-class "declared" provenance
// bit on records: a shape produced by a type annotation, type alias, or module
// manifest is closed, so an absent field read is an error. Every other shape,
// including table literals and their joins/copies, stays inferred (open), so an
// absent field reads gradually with no diagnostic.
//
// The table covers the propagation points the bit must survive: join/union,
// return, alias, import (manifest), and Map element.
func TestDeclaredVsInferredAbsentField(t *testing.T) {
	declaredModule := io.NewManifest("mod")
	declaredModule.SetExport(typ.NewRecord().
		Field("make", typ.Func().
			Returns(typ.NewRecord().Field("name", typ.String).Build()).
			Build()).
		Build())

	inferredModule := io.NewManifest("mod")
	inferredModule.BodyBacked = true
	inferredModule.SetExport(typ.NewRecord().
		Field("make", typ.Func().
			Returns(typ.NewRecord().Field("name", typ.String).Build()).
			Build()).
		Build())

	tests := []struct {
		name      string
		code      string
		wantError bool
		manifests map[string]*io.Manifest
	}{
		{
			// Baseline: a declared local's type annotation is closed.
			name: "declared_local_absent_field_errors",
			code: `local p: {name: string} = {name = "a"}
local v = p.missing`,
			wantError: true,
		},
		{
			// Baseline: an inferred table literal is open.
			name: "inferred_local_absent_field_reads_gradually",
			code: `local p = {name = "a"}
local v = p.missing`,
		},
		{
			// Join: declared only when every contributing record is declared.
			name: "join_of_declared_records_errors",
			code: `local p: {name: string} = {name = "a"}
local q: {name: string} = {name = "b"}
local r = p
if math.random() > 0.5 then r = p else r = q end
local v = r.missing`,
			wantError: true,
		},
		{
			// Join of inferred records stays open.
			name: "join_of_inferred_records_reads_gradually",
			code: `local p = {name = "a"}
local q = {name = "b"}
local r = p
if math.random() > 0.5 then r = p else r = q end
local v = r.missing`,
		},
		{
			// A declared and an inferred contributor make the join inferred.
			name: "join_mixed_declared_and_inferred_reads_gradually",
			code: `local p: {name: string} = {name = "a"}
local q = {name = "b"}
local r = p
if math.random() > 0.5 then r = p else r = q end
local v = r.missing`,
		},
		{
			// Return: a declared return annotation is closed.
			name: "declared_return_absent_field_errors",
			code: `local function f(): {name: string} return {name = "a"} end
local r = f()
local v = r.missing`,
			wantError: true,
		},
		{
			// Return: an inferred return keeps the open shape.
			name: "inferred_return_absent_field_reads_gradually",
			code: `local function f() return {name = "a"} end
local r = f()
local v = r.missing`,
		},
		{
			// Alias: a named type alias is a declaration.
			name: "alias_absent_field_errors",
			code: `type P = {name: string}
local p: P = {name = "a"}
local v = p.missing`,
			wantError: true,
		},
		{
			// Nested alias: the flag survives through the outer record.
			name: "nested_alias_absent_field_errors",
			code: `type Inner = {name: string}
type Outer = {inner: Inner}
local o: Outer = {inner = {name = "a"}}
local v = o.inner.missing`,
			wantError: true,
		},
		{
			// Import: a runtime manifest's export record is closed.
			name: "manifest_export_record_field_errors",
			code: `local mod = require("mod")
local v = mod.missing`,
			wantError: true,
			manifests: map[string]*io.Manifest{"mod": declaredModule},
		},
		{
			// Import: a runtime manifest's returned record is closed.
			name: "manifest_returned_record_absent_field_errors",
			code: `local mod = require("mod")
local x = mod.make()
local v = x.missing`,
			wantError: true,
			manifests: map[string]*io.Manifest{"mod": declaredModule},
		},
		{
			// Import: a body-backed (Lua-inferred) export stays open.
			name: "body_backed_manifest_absent_field_reads_gradually",
			code: `local mod = require("mod")
local v = mod.missing`,
			manifests: map[string]*io.Manifest{"mod": inferredModule},
		},
		{
			// Map element: the declared element type's records are closed.
			name: "declared_map_element_absent_field_errors",
			code: `local m: {[string]: {name: string}} = {}
local v = m["k"].missing`,
			wantError: true,
		},
		{
			// Map element: an inferred table's field is open.
			name: "inferred_map_element_absent_field_reads_gradually",
			code: `local m = {k = {name = "a"}}
local v = m.k.missing`,
		},
		{
			// Copy: the flag survives a plain local copy.
			name: "copy_of_declared_absent_field_errors",
			code: `local p: {name: string} = {name = "a"}
local q = p
local v = q.missing`,
			wantError: true,
		},
		{
			// Copy: an inferred copy stays open.
			name: "copy_of_inferred_absent_field_reads_gradually",
			code: `local p = {name = "a"}
local q = p
local v = q.missing`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []testutil.Option{testutil.WithStdlib()}
			for path, manifest := range tt.manifests {
				opts = append(opts, testutil.WithManifest(path, manifest))
			}
			result := testutil.Check(tt.code, opts...)
			if result.HasError() != tt.wantError {
				t.Fatalf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
