package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestGeneratedManifestExportsMTableFunctionSignaturesToRequireConsumers(t *testing.T) {
	library := CheckAndExport(`
local M = {}

function M.singleton_component_id(id: string)
    return id
end

function M.annotated_component_id(id: string): string
    return id
end

M.assigned_component_id = function(id: string)
    return id
end

local function local_component_id(id: string)
    return id
end
M.local_component_id = local_component_id

return M
`, "component")
	if len(library.Errors) != 0 {
		t.Fatalf("library diagnostics = %#v, want none", library.Errors)
	}

	encoded, err := manifest.Encode(library.Manifest)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	generated, err := manifest.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	for _, name := range []string{
		"component.singleton_component_id",
		"component.annotated_component_id",
		"component.assigned_component_id",
		"component.local_component_id",
	} {
		sig, ok := generated.FunctionSignatures[name]
		if !ok || sig.Type == nil {
			t.Fatalf("generated signatures = %#v, want %s", generated.FunctionSignatures, name)
		}
		if len(sig.Type.Params) != 1 || !typ.TypeEquals(sig.Type.Params[0].Type, typ.String) ||
			len(sig.Type.Returns) != 1 || !typ.TypeEquals(sig.Type.Returns[0], typ.String) {
			t.Fatalf("%s signature = %v, want (string) -> string", name, sig.Type)
		}
	}

	consumer := Check(`
local lib = require("component")
local inferred: string = lib.singleton_component_id("k")
local annotated: string = lib.annotated_component_id("k")
local assigned: string = lib.assigned_component_id("k")
local local_function: string = lib.local_component_id("k")
`, WithManifest("component", generated))
	if len(consumer.Diagnostics) != 0 {
		t.Fatalf("consumer diagnostics = %#v, want M-table function returns to remain string", consumer.Diagnostics)
	}

	contrast := Check(`
local lib = require("component")
local not_a_number: number = lib.singleton_component_id("k")
`, WithManifest("component", generated))
	if len(contrast.Diagnostics) != 1 || contrast.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("contrast diagnostics = %#v, want string-to-number assignment error rather than any", contrast.Diagnostics)
	}
}

func TestManifestSignatureKeepsReferencesInOwningModuleScope(t *testing.T) {
	registry := registryLikeManifest()
	program := `
local id: string = "entry"
local e, err = registry.get(id)
if err == nil then
    local s: string = e.id
end
`

	registryOnly := Check(program, WithGlobals("registry"), WithManifest("registry", registry))
	if len(registryOnly.Diagnostics) != 0 {
		t.Fatalf("registry-only diagnostics = %#v, want none", registryOnly.Diagnostics)
	}

	withFS := Check(program,
		WithGlobals("registry"),
		WithManifest("registry", registry),
		WithManifest("fs", fsLikeManifest()),
	)
	if len(withFS.Diagnostics) != 0 {
		t.Fatalf("registry + fs diagnostics = %#v, want registry.get Entry reference to remain precise", withFS.Diagnostics)
	}
}

func TestManifestSignatureScopesNestedOwningDefinitions(t *testing.T) {
	registry := manifest.New("registry")
	entryID := typetable.NewRecord().Field("value", typ.String).Build()
	entry := typetable.NewRecord().Field("id", typ.NewRef("", "EntryID")).Build()
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	get := typ.Func().
		Param("id", typ.String).
		Returns(typ.NewRef("", "Entry"), typeexpr.Optional(typ.NewRef("", "Error"))).
		Build()
	registry.DefineType("EntryID", entryID)
	registry.DefineType("Entry", entry)
	registry.DefineType("Error", errorType)
	registry.SetExport(typetable.NewRecord().Field("get", get).Build())
	registry.DefineFunctionSignature("registry.get", signature.Function{
		Type:   get,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})

	fs := fsLikeManifest()
	fs.DefineType("EntryID", typetable.NewRecord().Field("name", typ.String).Build())

	program := `
local id: string = "entry"
local e, err = registry.get(id)
if err == nil then
    local s: string = e.id.value
end
`
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{
			name: "registry only",
			opts: []Option{
				WithGlobals("registry"),
				WithManifest("registry", registry),
			},
		},
		{
			name: "registry and fs",
			opts: []Option{
				WithGlobals("registry"),
				WithManifest("registry", registry),
				WithManifest("fs", fs),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(program, tc.opts...)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want nested registry EntryID reference to remain precise", result.Diagnostics)
			}
		})
	}
}

func TestManifestTypeNamedTypeDoesNotShadowBuiltinTypePredicate(t *testing.T) {
	entry := typetable.NewRecord().Field("id", typeexpr.Union(typ.String, typ.Number)).Build()
	registry := manifest.New("registry")
	registry.DefineGlobalType("registry_get", typ.Func().Returns(entry).Build())
	registry.SetExport(typ.Unknown)

	fs := manifest.New("fs")
	fs.DefineType("type", typetable.NewRecord().Field("shadowed", typ.String).Build())
	fs.SetExport(typ.Unknown)

	program := `
local e = registry_get()
if type(e.id) == "string" then
    local s: string = e.id
    local tag: "string" = type(e.id)
end
`

	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{
			name: "registry only",
			opts: []Option{
				WithStdlib(),
				WithManifest("registry", registry),
			},
		},
		{
			name: "registry and fs type named type",
			opts: []Option{
				WithStdlib(),
				WithManifest("registry", registry),
				WithManifest("fs", fs),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(program, tc.opts...)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want builtin type() to refine e.id to string", result.Diagnostics)
			}
		})
	}
}

func registryLikeManifest() *manifest.Manifest {
	entry := typetable.NewRecord().Field("id", typ.String).Build()
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	get := typ.Func().
		Param("id", typ.String).
		Returns(typ.NewRef("", "Entry"), typeexpr.Optional(typ.NewRef("", "Error"))).
		Build()

	m := manifest.New("registry")
	m.DefineType("Entry", entry)
	m.DefineType("Error", errorType)
	m.SetExport(typetable.NewRecord().Field("get", get).Build())
	m.DefineFunctionSignature("registry.get", signature.Function{
		Type:   get,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})
	return m
}

func fsLikeManifest() *manifest.Manifest {
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	file := typetable.NewRecord().Field("name", typ.String).Build()
	get := typ.Func().
		Param("name", typ.String).
		Returns(file, typeexpr.Optional(errorType)).
		Build()

	m := manifest.New("fs")
	m.DefineType("Error", errorType)
	m.DefineType("File", file)
	m.SetExport(typetable.NewRecord().Field("get", get).Build())
	m.DefineFunctionSignature("fs.get", signature.Function{
		Type:   get,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})
	return m
}
