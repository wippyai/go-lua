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
