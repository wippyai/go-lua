package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

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
