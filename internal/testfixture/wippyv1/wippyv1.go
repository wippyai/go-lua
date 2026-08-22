// Package wippyv1 carries the host-module surface that the Wippy v1 runtime
// declares in production, transcribed into the canonical manifest vocabulary.
//
// The v1 runtime declares each host module as an io.Manifest built from the
// pre-cut type builders: the module surface is a typ.Interface (optionally
// intersected with a constant-carrying record), object types are registered by
// name through DefineType, and every fallible member returns a trailing
// Optional(LuaError). The canonical boundary states the same facts in a
// different shape: the export is a record whose fields are the mounted
// members, every callable is registered as a signature under its manifest-local
// path ("Store.get" for a member of a declared type), and the module's error
// type is named once so the value/error correlation is derived instead of
// re-tagged per member.
//
// The transcription is deliberately literal. Where v1 declares typ.Any, this
// declares typ.Any; where v1 declares an optional non-error return, this
// declares an optional non-error return. Nothing is sharpened, because the
// point of the fixture is to measure the checker against the surface Wippy
// actually ships, not against an idealized one.
package wippyv1

//go:generate go run ./cmd/gendata

import (
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// ErrorType is the structured error interface every fallible Wippy member
// returns as its trailing optional result. v1 declares it once as typ.LuaError
// and shares that single value across all modules, so it is declared once here
// and installed as each manifest's ErrorType.
func ErrorType() typ.Type { return errorType }

var errorType, _ = DeclaredObject("Error", func(self typ.Type) []typ.Method {
	return []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", self).Returns(typ.String).Build()},
		{Name: "retryable", Type: typ.Func().Param("self", self).Returns(typ.Boolean).Build()},
		{Name: "details", Type: typ.Func().Param("self", self).Returns(typ.Any).Build()},
		{Name: "message", Type: typ.Func().Param("self", self).Returns(typ.String).Build()},
		{Name: "stack", Type: typ.Func().Param("self", self).Returns(typ.String).Build()},
	}
})

// ManifestVersion identifies this transcription of the v1 host surface.
const ManifestVersion = "wippy-v1-host"

// Module is one transcribed host module: the provider identity the catalogue
// binds it under, and the declaration that produces fresh provider-owned data.
type Module struct {
	Name        string
	Declaration func() *manifestwire.Manifest
}

// Modules returns the transcribed host modules in stable name order.
func Modules() []Module {
	return []Module{
		{Name: "expr", Declaration: ExprManifest},
		{Name: "http", Declaration: HTTPManifest},
		{Name: "json", Declaration: JSONManifest},
		{Name: "process", Declaration: ProcessManifest},
		{Name: "store", Declaration: StoreManifest},
	}
}

// Providers mounts every transcribed module as a module-scoped provider.
func Providers() []manifest.Provider {
	modules := Modules()
	out := make([]manifest.Provider, 0, len(modules))
	for _, module := range modules {
		out = append(out, manifest.Provider{
			Identity:    "wippy.v1." + module.Name,
			Mount:       manifest.MountModule,
			Declaration: module.Declaration,
		})
	}
	return out
}

// Target seals the Lua standard library together with the transcribed Wippy v1
// host surface into one canonical Target contract. It is the fixture entry
// point for asking what the checker makes of the real production boundary.
func Target() (*contract.Contract, error) {
	catalogue, err := manifest.Seal(append(stdlib.Providers(), Providers()...)...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue)
}

// DeclaredObject builds one named object type and returns both the type and
// the method list it was built from. The canonical fixture Target authors its
// host object types through the same constructor, so a declared object has one
// shape wherever it is stated.
//
// v1 spells the receiver of every such method as typ.Self. Self names whichever
// receiver a call site happens to hold, so it is meaningful only inside a scope
// that already knows the receiver; a module boundary carries the declaration
// away from every such scope, and the canonical target rejects it for exactly
// that reason. The receiver of store.Store:get is store.Store, and a recursive
// node is how that is said without a scope: the builder receives the type being
// declared and uses it wherever v1 wrote Self.
func DeclaredObject(name string, build func(self typ.Type) []typ.Method) (typ.Type, []typ.Method) {
	var methods []typ.Method
	declared := typ.NewRecursive(name, func(self typ.Type) typ.Type {
		methods = build(self)
		return typ.NewInterface(name, methods)
	})
	return declared, methods
}

// DefineMethods registers one declared object type's methods as manifest-local
// callables under the "Type.member" path the catalogue splits into a binding.
func DefineMethods(declaration *manifestwire.Manifest, typeName string, methods []typ.Method) {
	for _, method := range methods {
		declaration.DefineFunctionSignature(typeName+"."+method.Name, signature.Function{Type: method.Type})
	}
}

// newManifest starts a module declaration with the shared Wippy error type
// installed, so the trailing Optional(Error) result of every fallible member is
// recognized as the error half of a value/error pair.
func newManifest(path string) *manifestwire.Manifest {
	declaration := manifestwire.New(path)
	declaration.Version = ManifestVersion
	declaration.ErrorType = errorType
	return declaration
}
