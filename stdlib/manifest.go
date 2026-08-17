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
	signatures map[string]declaredFunction
	detached   map[string]detachedFunction
	// methods are callable declarations below the direct module export level.
	// Their keys are manifest-local paths such as "Error.kind".
	methods   map[string]declaredFunction
	aliases   map[string]string
	values    map[string]typ.Type
	types     map[string]typ.Type
	errorType typ.Type
	readonly  bool
}

type declaredFunction struct {
	signature.Function
	operation *moduleio.Operation
}

func (function declaredFunction) operational(operation moduleio.Operation) declaredFunction {
	owned := moduleio.CloneOperation(operation)
	function.operation = &owned
	return function
}

type detachedFunction struct {
	signature signature.Function
	operation moduleio.Operation
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

func buildManifest(library Library, decl declaration) *moduleio.Manifest {
	m := moduleio.New(library.Name())
	m.Version = ManifestVersion
	m.ErrorType = decl.errorType

	for name, value := range decl.types {
		m.DefineType(name, value)
	}
	for name, value := range decl.methods {
		m.DefineFunctionSignature(name, value.Function.Clone())
		if value.operation != nil {
			m.DefineFunctionOperation(name, *value.operation)
		}
	}
	for name, value := range decl.detached {
		m.DefineDetachedFunction(name, value.signature, value.operation)
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
			m.DefineFunctionSignature(name, value.Function.Clone())
			if value.operation != nil {
				m.DefineFunctionOperation(name, *value.operation)
			}
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

func authored(fn *typ.Function, labels ...effect.Label) declaredFunction {
	return declaredFunction{Function: signature.Function{Type: fn, Effect: effect.Empty.With(labels...)}}
}

func openAuthored(tail string, fn *typ.Function, labels ...effect.Label) declaredFunction {
	return declaredFunction{Function: signature.Function{Type: fn, Effect: effect.Open(tail, labels...)}}
}

func withResultTail(function declaredFunction, tail typ.Type) declaredFunction {
	function.ResultTail = tail
	return function
}

func withResults(function declaredFunction, tail typ.Type, suffix ...typ.Type) declaredFunction {
	function.ResultTail = tail
	function.ResultSuffix = append([]typ.Type(nil), suffix...)
	return function
}
