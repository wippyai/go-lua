// Package manifest seals provider declarations into one immutable catalogue.
//
// Providers own declarations. Consumers supply providers once at their
// boundary and use the resulting Catalogue for exact type and signature
// lookup. The package has no knowledge of Lua's native libraries, dependency
// resolution, analyzer stores, or runtime openers.
package manifest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/domain/type/typ"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// Mount describes whether a provider contributes initial scope. Detached
// manifests describe dependencies without implicitly admitting them.
type Mount uint8

const (
	MountDetached Mount = iota
	MountGlobals
	MountModule
)

// Provider couples stable provider identity and mount policy to one manifest
// declaration. Declaration must return provider-owned fresh data.
type Provider struct {
	Identity    string
	Mount       Mount
	Immutable   bool
	Declaration func() *moduleio.Manifest
}

// Catalogue is an immutable, exact-path view of sealed declarations.
type Catalogue struct {
	providers      []provider
	byPath         map[string]int
	functions      []Function
	byFunctionPath map[string]int
	initialGlobals []string
}

// Function is one provider-owned callable with its Lua-visible bindings kept
// separate from its canonical diagnostic path.
type Function struct {
	providerID    string
	bindings      []Binding
	canonicalPath string
	signature     signature.Function
}

func (f Function) ProviderIdentity() string { return f.providerID }
func (f Function) CanonicalPath() string    { return f.canonicalPath }
func (f Function) Signature() signature.Function {
	return f.signature.Clone()
}

// Binding is one Lua-visible spelling of a callable identity.
type Binding struct {
	mount      Mount
	modulePath string
	member     []string
}

func (b Binding) Mount() Mount       { return b.mount }
func (b Binding) ModulePath() string { return b.modulePath }
func (b Binding) Member() []string   { return append([]string(nil), b.member...) }

func (f Function) Bindings() []Binding {
	out := make([]Binding, len(f.bindings))
	for index := range f.bindings {
		out[index] = f.bindings[index].clone()
	}
	return out
}

type provider struct {
	identity  string
	mount     Mount
	immutable bool
	manifest  *moduleio.Manifest
}

// Module is one mounted module table declared by a provider.
type Module struct {
	providerID string
	path       string
	immutable  bool
}

func (m Module) ProviderIdentity() string { return m.providerID }
func (m Module) Path() string             { return m.path }
func (m Module) Immutable() bool          { return m.immutable }

// Seal validates, ownership-isolates, and indexes providers. Duplicate
// identities and duplicate manifest paths are rejected; ordering is retained
// only for deterministic enumeration, never for override semantics.
func Seal(input ...Provider) (*Catalogue, error) {
	catalogue := &Catalogue{
		providers:      make([]provider, 0, len(input)),
		byPath:         make(map[string]int, len(input)),
		byFunctionPath: make(map[string]int),
	}
	identities := make(map[string]struct{}, len(input))
	globals := make(map[string]struct{})
	for index, item := range input {
		if item.Identity == "" {
			return nil, fmt.Errorf("manifest: provider %d has no identity", index)
		}
		if _, exists := identities[item.Identity]; exists {
			return nil, fmt.Errorf("manifest: duplicate provider identity %q", item.Identity)
		}
		identities[item.Identity] = struct{}{}
		if item.Declaration == nil {
			return nil, fmt.Errorf("manifest: provider %q has no declaration", item.Identity)
		}
		declared := item.Declaration()
		if declared == nil {
			return nil, fmt.Errorf("manifest: provider %q returned no declaration", item.Identity)
		}
		owned, err := clone(declared)
		if err != nil {
			return nil, fmt.Errorf("manifest: provider %q: %w", item.Identity, err)
		}
		if _, exists := catalogue.byPath[owned.Path]; exists {
			return nil, fmt.Errorf("manifest: duplicate path %q", owned.Path)
		}
		catalogue.byPath[owned.Path] = len(catalogue.providers)
		catalogue.providers = append(catalogue.providers, provider{
			identity:  item.Identity,
			mount:     item.Mount,
			immutable: item.Immutable,
			manifest:  owned,
		})

		for local, function := range owned.FunctionSignatures {
			if _, alias := owned.FunctionAliases[local]; alias {
				continue
			}
			name := local
			if item.Mount != MountGlobals {
				name = qualify(owned.Path, local)
			}
			if _, exists := catalogue.byFunctionPath[name]; exists {
				return nil, fmt.Errorf("manifest: duplicate function path %q", name)
			}
			catalogue.byFunctionPath[name] = len(catalogue.functions)
			catalogue.functions = append(catalogue.functions, Function{
				providerID:    item.Identity,
				bindings:      []Binding{{mount: item.Mount, modulePath: owned.Path, member: strings.Split(local, ".")}},
				canonicalPath: name,
				signature:     owned.ScopeSignature(function),
			})
		}

		// Explicit ambient globals are valid for every mount. A provider can
		// install a module table and host functions side by side (Wippy's
		// restricted package provider installs both package and require).
		for _, name := range owned.Globals {
			if name != "" {
				globals[name] = struct{}{}
			}
		}
		switch item.Mount {
		case MountGlobals:
		case MountModule:
			if owned.Path == "" {
				return nil, fmt.Errorf("manifest: module provider %q has an empty path", item.Identity)
			}
			globals[owned.Path] = struct{}{}
		case MountDetached:
		default:
			return nil, fmt.Errorf("manifest: provider %q has invalid mount %d", item.Identity, item.Mount)
		}
	}
	for _, item := range catalogue.providers {
		for local, target := range item.manifest.FunctionAliases {
			aliasPath := local
			if item.mount != MountGlobals {
				aliasPath = qualify(item.manifest.Path, local)
			}
			if _, exists := catalogue.byFunctionPath[aliasPath]; exists {
				return nil, fmt.Errorf("manifest: function alias %q collides with a declaration", aliasPath)
			}
			targetIndex, exists := catalogue.byFunctionPath[target]
			if !exists {
				return nil, fmt.Errorf("manifest: function alias %q targets unknown %q", aliasPath, target)
			}
			aliasSignature, exists := item.manifest.FunctionSignatures[local]
			if !exists || !aliasSignature.Equals(catalogue.functions[targetIndex].signature) {
				return nil, fmt.Errorf("manifest: function alias %q signature differs from %q", aliasPath, target)
			}
			catalogue.functions[targetIndex].bindings = append(catalogue.functions[targetIndex].bindings, Binding{
				mount: item.mount, modulePath: item.manifest.Path, member: strings.Split(local, "."),
			})
			catalogue.byFunctionPath[aliasPath] = targetIndex
		}
	}
	for name := range globals {
		catalogue.initialGlobals = append(catalogue.initialGlobals, name)
	}
	sort.Strings(catalogue.initialGlobals)
	return catalogue, nil
}

// Modules returns mounted module tables in provider declaration order.
func (c *Catalogue) Modules() []Module {
	if c == nil {
		return nil
	}
	out := make([]Module, 0, len(c.providers))
	for _, item := range c.providers {
		if item.mount == MountModule {
			out = append(out, Module{providerID: item.identity, path: item.manifest.Path, immutable: item.immutable})
		}
	}
	return out
}

func clone(input *moduleio.Manifest) (*moduleio.Manifest, error) {
	encoded, err := moduleio.Encode(input)
	if err != nil {
		return nil, err
	}
	return moduleio.Decode(encoded)
}

func qualify(path, local string) string {
	if path == "" {
		return local
	}
	if local == "" {
		return path
	}
	return path + "." + local
}

// ProviderIdentities returns the sealed identities in declaration order.
func (c *Catalogue) ProviderIdentities() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.providers))
	for index, item := range c.providers {
		out[index] = item.identity
	}
	return out
}

// InitialGlobals returns the exact initial names implied by mount policy.
func (c *Catalogue) InitialGlobals() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.initialGlobals...)
}

// Function returns an ownership-isolated declaration by canonical diagnostic
// path, such as "assert", "table.insert", or "errors.Error.kind".
func (c *Catalogue) Function(path string) (Function, bool) {
	if c == nil || path == "" {
		return Function{}, false
	}
	index, ok := c.byFunctionPath[path]
	if !ok {
		return Function{}, false
	}
	return c.functions[index].clone(), true
}

// Functions returns every mounted declaration in canonical-path order.
func (c *Catalogue) Functions() []Function {
	if c == nil {
		return nil
	}
	out := make([]Function, len(c.functions))
	for index := range c.functions {
		out[index] = c.functions[index].clone()
	}
	sort.Slice(out, func(left, right int) bool {
		return out[left].canonicalPath < out[right].canonicalPath
	})
	return out
}

func (f Function) clone() Function {
	f.bindings = f.Bindings()
	f.signature = f.signature.Clone()
	return f
}

func (b Binding) clone() Binding {
	b.member = append([]string(nil), b.member...)
	return b
}

// Type returns one named type from an exact manifest path.
func (c *Catalogue) Type(modulePath, name string) (typ.Type, bool) {
	if c == nil || name == "" {
		return nil, false
	}
	index, ok := c.byPath[modulePath]
	if !ok {
		return nil, false
	}
	declaration := c.providers[index].manifest
	value := declaration.Types[name]
	if value == nil {
		return nil, false
	}
	return declaration.ScopeType(value), true
}

// ResolveType resolves either an unqualified unique type name or a qualified
// module path. Aliases map source import names to exact manifest paths.
func (c *Catalogue) ResolveType(path []string, aliases map[string]string) (typ.Type, bool) {
	if c == nil || len(path) == 0 {
		return nil, false
	}
	if len(path) == 1 {
		var found typ.Type
		for _, item := range c.providers {
			value := item.manifest.Types[path[0]]
			if value == nil {
				continue
			}
			if found != nil {
				return nil, false
			}
			found = item.manifest.ScopeType(value)
		}
		return found, found != nil
	}
	modulePath := strings.Join(path[:len(path)-1], ".")
	if alias := aliases[path[0]]; alias != "" {
		modulePath = alias
		if len(path) > 2 {
			modulePath += "." + strings.Join(path[1:len(path)-1], ".")
		}
	}
	return c.Type(modulePath, path[len(path)-1])
}
