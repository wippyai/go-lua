package stdlib

import (
	"sort"

	"github.com/wippyai/go-lua/domain/effect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	declarations "github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// ManifestVersion identifies the provider-owned declaration set. It is not a
// module resolver version: dependency selection and admission remain the
// responsibility of the embedding runtime.
const ManifestVersion = "go-lua-stdlib-v1"

type declaration struct {
	signatures map[string]signature.Function
	// methods are callable declarations below the direct module export level.
	// Their keys are manifest-local paths such as "Error.kind".
	methods   map[string]signature.Function
	aliases   map[string]string
	values    map[string]typ.Type
	types     map[string]typ.Type
	errorType typ.Type
	readonly  bool
}

// Manifest returns a fresh declaration manifest for id. The manifest describes
// values and callable behavior only. It does not mount the library, resolve a
// dependency, authorize an import, or create an analysis scope.
func Manifest(id ID) (*moduleio.Manifest, bool) {
	library, ok := Lookup(id)
	if !ok || library.declaration == nil {
		return nil, false
	}
	return buildManifest(library, library.declaration()), true
}

// Providers returns every native declaration in runtime catalogue order.
// Runtime opener binding and declaration binding therefore share one exact
// identity set without a second registry.
func Providers() []declarations.Provider {
	out := make([]declarations.Provider, 0, len(catalogue))
	for _, library := range catalogue {
		library := library
		declared := library.declaration()
		mount := declarations.MountModule
		if library.Mount() == MountGlobals {
			mount = declarations.MountGlobals
		}
		out = append(out, declarations.Provider{
			Identity:  string(library.ID()),
			Mount:     mount,
			Immutable: declared.readonly,
			Declaration: func() *moduleio.Manifest {
				return buildManifest(library, declared)
			},
		})
	}
	return out
}

// Catalogue seals the native provider declarations through the same exact
// catalogue used by LState.OpenLibs.
func Catalogue() (*declarations.Catalogue, error) {
	return declarations.Seal(Providers()...)
}

func buildManifest(library Library, decl declaration) *moduleio.Manifest {
	m := moduleio.New(library.Name())
	m.Version = ManifestVersion
	m.ErrorType = decl.errorType

	for name, value := range decl.types {
		m.DefineType(name, value)
	}
	for name, value := range decl.methods {
		m.DefineFunctionSignature(name, value.Clone())
	}
	for alias, target := range decl.aliases {
		m.DefineFunctionAlias(alias, target)
	}

	names := make([]string, 0, len(decl.signatures)+len(decl.values))
	for name := range decl.signatures {
		names = append(names, name)
	}
	for name := range decl.values {
		if _, callable := decl.signatures[name]; !callable {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	export := typetable.NewRecord()
	for _, name := range names {
		value, callable := decl.signatures[name]
		var valueType typ.Type
		if callable {
			valueType = value.Type
			m.DefineFunctionSignature(name, value.Clone())
		} else {
			valueType = decl.values[name]
		}
		if decl.readonly {
			export.ReadonlyField(name, valueType)
		} else {
			export.Field(name, valueType)
		}
		if library.Mount() == MountGlobals {
			m.DefineGlobalType(name, valueType)
		}
	}
	m.SetExport(export.Build())
	return m
}

func authored(fn *typ.Function, labels ...effect.Label) signature.Function {
	return signature.Function{Type: fn, Effect: effect.Empty.With(labels...)}
}

func openAuthored(tail string, fn *typ.Function, labels ...effect.Label) signature.Function {
	return signature.Function{Type: fn, Effect: effect.Open(tail, labels...)}
}

func withResultTail(function signature.Function, tail typ.Type) signature.Function {
	function.ResultTail = tail
	return function
}

func withResults(function signature.Function, tail typ.Type, suffix ...typ.Type) signature.Function {
	function.ResultTail = tail
	function.ResultSuffix = append([]typ.Type(nil), suffix...)
	return function
}
