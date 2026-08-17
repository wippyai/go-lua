package stdlib

import (
	"sort"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/domain/effect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	manifest "github.com/wippyai/go-lua/types/io"
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
	values    map[string]typ.Type
	types     map[string]typ.Type
	errorType typ.Type
	readonly  bool
}

// Manifest returns a fresh declaration manifest for id. The manifest describes
// values and callable behavior only. It does not mount the library, resolve a
// dependency, authorize an import, or create an analysis scope.
func Manifest(id ID) (*manifest.Manifest, bool) {
	library, ok := Lookup(id)
	if !ok || library.declaration == nil {
		return nil, false
	}
	return buildManifest(library, library.declaration()), true
}

// ManifestByName returns a fresh manifest for the catalogue entry whose Lua
// mount name is name. An empty name selects the base/global declaration.
// Presence here is descriptive and must not be used as an admission decision.
func ManifestByName(name string) (*manifest.Manifest, bool) {
	for _, library := range catalogue {
		if library.Name() == name {
			return Manifest(library.ID())
		}
	}
	return nil, false
}

// ManifestProviders binds every declaration provider through the exact same
// catalogue coverage law used by the native openers.
func ManifestProviders() []ManifestBinding[*manifest.Manifest] {
	providers := make(map[ID]ManifestProvider[*manifest.Manifest], len(catalogue))
	for _, library := range catalogue {
		id := library.ID()
		providers[id] = func() *manifest.Manifest {
			m, ok := Manifest(id)
			if !ok {
				panic("stdlib: missing manifest for " + string(id))
			}
			return m
		}
	}
	bound, err := BindManifests(providers)
	if err != nil {
		panic(err)
	}
	return bound
}

// Signatures returns ownership-isolated local callable signatures for id.
// Named libraries use member names ("abs"), while Base uses bare globals
// ("assert").
func Signatures(id ID) (map[string]signature.Function, bool) {
	library, ok := Lookup(id)
	if !ok || library.declaration == nil {
		return nil, false
	}
	decl := library.declaration()
	out := make(map[string]signature.Function, len(decl.signatures))
	for name, sig := range decl.signatures {
		out[name] = sig.Clone()
	}
	return out, true
}

var (
	signatureIndexOnce sync.Once
	signatureIndex     map[string]signature.Function
)

// Signature returns the provider-owned signature for a bare base global or a
// dotted named-library member. Dependency aliases are deliberately not
// interpreted here; the embedding runtime resolves an alias to a manifest
// before asking analysis to read it.
func Signature(name string) (signature.Function, bool) {
	signatureIndexOnce.Do(buildSignatureIndex)
	sig, ok := signatureIndex[name]
	if !ok {
		return signature.Function{}, false
	}
	return sig.Clone(), true
}

// SignatureNames returns every native stdlib callable path in sorted order.
func SignatureNames() []string {
	signatureIndexOnce.Do(buildSignatureIndex)
	names := make([]string, 0, len(signatureIndex))
	for name := range signatureIndex {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// InitialGlobalNames returns the names installed by LState.OpenLibs. Base
// exports are mounted directly; named libraries contribute only their module
// root. Embedders that open a subset must project the selected catalogue IDs
// themselves rather than treating this inventory as an admission policy.
func InitialGlobalNames() []string {
	names := make([]string, 0, len(catalogue)*4)
	for _, library := range catalogue {
		if library.Mount() == MountGlobals {
			decl := library.declaration()
			for name := range decl.signatures {
				names = append(names, name)
			}
			for name := range decl.values {
				names = append(names, name)
			}
			continue
		}
		names = append(names, library.Name())
	}
	sort.Strings(names)
	return names
}

func buildSignatureIndex() {
	index := make(map[string]signature.Function)
	for _, library := range catalogue {
		if library.declaration == nil {
			continue
		}
		decl := library.declaration()
		prefix := library.Name()
		for name, sig := range decl.signatures {
			qualified := name
			if prefix != "" {
				qualified = prefix + "." + name
			}
			index[qualified] = sig
		}
		for name, sig := range decl.methods {
			index[strings.TrimSuffix(prefix+".", ".")+"."+name] = sig
		}
	}
	signatureIndex = index
}

func buildManifest(library Library, decl declaration) *manifest.Manifest {
	m := manifest.New(library.Name())
	m.Version = ManifestVersion
	m.ErrorType = decl.errorType

	for name, value := range decl.types {
		m.DefineType(name, value)
	}
	for name, value := range decl.methods {
		m.DefineFunctionSignature(name, value.Clone())
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
