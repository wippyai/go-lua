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
	moduleio "github.com/wippyai/go-lua/types/io"
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
	Declaration func() *moduleio.Manifest
}

// Catalogue is an immutable, exact-path view of sealed declarations.
type Catalogue struct {
	providers      []provider
	byPath         map[string]int
	signatures     map[string]signature.Function
	initialGlobals []string
}

type provider struct {
	identity string
	mount    Mount
	manifest *moduleio.Manifest
}

// Seal validates, ownership-isolates, and indexes providers. Duplicate
// identities and duplicate manifest paths are rejected; ordering is retained
// only for deterministic enumeration, never for override semantics.
func Seal(input ...Provider) (*Catalogue, error) {
	catalogue := &Catalogue{
		providers:  make([]provider, 0, len(input)),
		byPath:     make(map[string]int, len(input)),
		signatures: make(map[string]signature.Function),
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
			identity: item.Identity,
			mount:    item.Mount,
			manifest: owned,
		})

		for local, function := range owned.FunctionSignatures {
			name := qualify(owned.Path, local)
			if _, exists := catalogue.signatures[name]; exists {
				return nil, fmt.Errorf("manifest: duplicate function path %q", name)
			}
			catalogue.signatures[name] = owned.ScopeSignature(function)
		}

		switch item.Mount {
		case MountGlobals:
			for _, name := range owned.Globals {
				if name != "" {
					globals[name] = struct{}{}
				}
			}
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
	for name := range globals {
		catalogue.initialGlobals = append(catalogue.initialGlobals, name)
	}
	sort.Strings(catalogue.initialGlobals)
	return catalogue, nil
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

// Signature returns an ownership-isolated function declaration by canonical
// path, such as "assert", "table.insert", or "errors.Error.kind".
func (c *Catalogue) Signature(path string) (signature.Function, bool) {
	if c == nil || path == "" {
		return signature.Function{}, false
	}
	function, ok := c.signatures[path]
	if !ok {
		return signature.Function{}, false
	}
	return function.Clone(), true
}

// SignatureNames returns every canonical callable path in lexical order.
func (c *Catalogue) SignatureNames() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.signatures))
	for name := range c.signatures {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
