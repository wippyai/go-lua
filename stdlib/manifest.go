package stdlib

import (
	"sort"
	"strings"

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

	initialRoots      []moduleio.InitialRoot
	initialEntries    []moduleio.InitialEntry
	initialMetatables []moduleio.InitialMetatableAttachment
}

type declaredFunction struct {
	signature.Function
	operation *moduleio.Operation
	initial   []initialFunctionMount
}

type initialFunctionMount struct {
	root       moduleio.InitialRootReference
	key        string
	mutability moduleio.InitialMutability
}

func (function declaredFunction) operational(operation moduleio.Operation) declaredFunction {
	owned := moduleio.CloneOperation(operation)
	function.operation = &owned
	return function
}

func (function declaredFunction) mountedInitially(root moduleio.InitialRootReference, key string, mutability moduleio.InitialMutability) declaredFunction {
	function.initial = append(function.initial, initialFunctionMount{root: root, key: key, mutability: mutability})
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
		defineInitialFunctionMounts(m, name, value)
	}
	for name, value := range decl.detached {
		m.DefineDetachedFunction(name, value.signature, value.operation)
	}
	surfaceSignatures := make(map[string]declaredFunction, len(decl.signatures)+len(decl.aliases))
	for name, function := range decl.signatures {
		surfaceSignatures[name] = function
	}
	for alias, target := range decl.aliases {
		function, ok := canonicalStdlibFunction(target)
		if !ok {
			panic("stdlib: alias " + alias + " targets unknown function " + target)
		}
		surfaceSignatures[alias] = function
		m.DefineFunctionAlias(alias, target)
	}
	for _, root := range decl.initialRoots {
		m.DefineInitialRoot(root)
	}
	for _, entry := range decl.initialEntries {
		m.DefineInitialEntry(entry)
	}
	for _, attachment := range decl.initialMetatables {
		m.DefineInitialMetatable(attachment)
	}

	names := make([]string, 0, len(surfaceSignatures)+len(decl.values))
	for name := range surfaceSignatures {
		names = append(names, name)
	}
	for name := range decl.values {
		if _, callable := surfaceSignatures[name]; !callable {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	export := typetable.NewRecord()
	for _, name := range names {
		value, callable := surfaceSignatures[name]
		var valueType typ.Type
		if callable {
			valueType = value.Type
			m.DefineFunctionSignature(name, value.Function.Clone())
			_, alias := decl.aliases[name]
			if value.operation != nil && !alias {
				m.DefineFunctionOperation(name, *value.operation)
			}
			if !alias {
				defineInitialFunctionMounts(m, name, value)
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

// canonicalStdlibFunction resolves aliases from the canonical provider
// declaration itself. It projects the exact signature/effect row into the
// alias surface without authoring a second declaredFunction.
func canonicalStdlibFunction(path string) (declaredFunction, bool) {
	for _, library := range catalogue {
		declaration := library.declaration()
		for local, function := range declaration.signatures {
			canonical := local
			if library.Mount() == MountModule {
				canonical = strings.TrimSuffix(library.Name(), ".") + "." + local
			}
			if canonical == path {
				return function, true
			}
		}
	}
	return declaredFunction{}, false
}

func defineInitialFunctionMounts(manifest *moduleio.Manifest, local string, function declaredFunction) {
	for _, mount := range function.initial {
		manifest.DefineInitialEntry(moduleio.InitialEntry{
			Root: mount.root, Key: mount.key,
			Value: moduleio.InitialFunctionValue(local), Mutability: mount.mutability,
		})
	}
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
