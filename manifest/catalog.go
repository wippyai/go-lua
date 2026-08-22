// Package manifest seals provider declarations into one immutable catalogue.
//
// Providers own declarations. Consumers supply providers once at their
// boundary and use the resulting Catalogue for exact type and signature
// lookup. The package has no knowledge of Lua's native libraries, dependency
// resolution, analyzer stores, or runtime openers.
package manifest

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
	"github.com/wippyai/go-lua/domain/typestate"
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
	// typestateProtocols is the catalogue-wide state-machine namespace. A
	// protocol name is shared vocabulary rather than provider-local, so two
	// providers may restate the identical machine and may never disagree
	// about it.
	typestateProtocols map[typestate.Protocol]typestate.Definition
	protocolOwners     map[typestate.Protocol]string
}

// Function is one provider-owned callable with its Lua-visible bindings kept
// separate from its canonical diagnostic path.
type Function struct {
	providerID    string
	bindings      []Binding
	canonicalPath string
	signature     signature.Function
	operation     moduleio.Operation
	hasOperation  bool
}

// Value is one mounted, non-callable export. Its type is the declaration of
// the initial value: singleton types preserve exact literals while aggregate
// and otherwise non-literal values remain intentionally opaque to consumers.
type Value struct {
	providerID string
	binding    Binding
	valueType  typ.Type
	immutable  bool
}

func (v Value) ProviderIdentity() string { return v.providerID }
func (v Value) Binding() Binding         { return v.binding.clone() }
func (v Value) Type() typ.Type           { return v.valueType }
func (v Value) Immutable() bool          { return v.immutable }

func (f Function) ProviderIdentity() string { return f.providerID }
func (f Function) CanonicalPath() string    { return f.canonicalPath }
func (f Function) Signature() signature.Function {
	return f.signature.Clone()
}

// Operation returns the provider-owned behavioral law. False requests the
// generic signature-derived operation and carries no provider specialization.
func (f Function) Operation() (moduleio.Operation, bool) {
	if !f.hasOperation {
		return moduleio.Operation{}, false
	}
	return moduleio.CloneOperation(f.operation), true
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

// InitialEnvironment is one provider's ownership-isolated bootstrap
// declaration. It is projected on demand from the sealed manifest; Catalogue
// does not maintain a second initial-environment registry.
type InitialEnvironment struct {
	providerID string
	modulePath string
	mount      Mount
	roots      []moduleio.InitialRoot
	entries    []moduleio.InitialEntry
	metatables []moduleio.InitialMetatableAttachment
}

func (e InitialEnvironment) ProviderIdentity() string { return e.providerID }
func (e InitialEnvironment) ModulePath() string       { return e.modulePath }
func (e InitialEnvironment) Mount() Mount             { return e.mount }

func (e InitialEnvironment) Roots() []moduleio.InitialRoot {
	return append([]moduleio.InitialRoot(nil), e.roots...)
}

func (e InitialEnvironment) Entries() []moduleio.InitialEntry {
	return append([]moduleio.InitialEntry(nil), e.entries...)
}

func (e InitialEnvironment) Metatables() []moduleio.InitialMetatableAttachment {
	return append([]moduleio.InitialMetatableAttachment(nil), e.metatables...)
}

// CanonicalFunctionPath resolves a provider-local callable path by the same
// mount law used by Catalogue.Functions.
func (e InitialEnvironment) CanonicalFunctionPath(local string) string {
	if e.mount == MountGlobals {
		return local
	}
	return qualify(e.modulePath, local)
}

// Seal validates, ownership-isolates, and indexes providers. Duplicate
// identities and duplicate manifest paths are rejected; ordering is retained
// only for deterministic enumeration, never for override semantics.
func Seal(input ...Provider) (*Catalogue, error) {
	catalogue := &Catalogue{
		providers:          make([]provider, 0, len(input)),
		byPath:             make(map[string]int, len(input)),
		byFunctionPath:     make(map[string]int),
		typestateProtocols: make(map[typestate.Protocol]typestate.Definition),
		protocolOwners:     make(map[typestate.Protocol]string),
	}
	identities := make(map[string]struct{}, len(input))
	globals := make(map[string]struct{})
	initialRootOwners := map[string]string{moduleio.GlobalEnvironmentRoot: "global environment"}
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
		claimInitialRoot := func(identity string) error {
			if owner, exists := initialRootOwners[identity]; exists {
				return fmt.Errorf("manifest: provider %q initial root %q collides with %s", item.Identity, identity, owner)
			}
			initialRootOwners[identity] = fmt.Sprintf("provider %q", item.Identity)
			return nil
		}
		if item.Mount == MountModule {
			if err := claimInitialRoot(moduleio.ModuleRootIdentity(item.Identity)); err != nil {
				return nil, err
			}
		}
		for _, root := range owned.InitialRoots {
			if err := claimInitialRoot(root.Identity); err != nil {
				return nil, err
			}
		}
		catalogue.byPath[owned.Path] = len(catalogue.providers)
		catalogue.providers = append(catalogue.providers, provider{
			identity:  item.Identity,
			mount:     item.Mount,
			immutable: item.Immutable,
			manifest:  owned,
		})
		for protocol, definition := range owned.TypestateProtocols {
			normalized := definition.Normalized()
			existing, declared := catalogue.typestateProtocols[protocol]
			if declared && !typestateDefinitionEqual(existing, normalized) {
				return nil, fmt.Errorf(
					"manifest: provider %q redeclares typestate protocol %q differently from provider %q",
					item.Identity, protocol, catalogue.protocolOwners[protocol],
				)
			}
			catalogue.typestateProtocols[protocol] = normalized
			if !declared {
				catalogue.protocolOwners[protocol] = item.Identity
			}
		}
		for local := range owned.FunctionOperations {
			_, direct := owned.FunctionSignatures[local]
			_, detached := owned.DetachedFunctions[local]
			if !direct && !detached {
				return nil, fmt.Errorf("manifest: provider %q operation %q has no function signature", item.Identity, local)
			}
		}

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
			operation, specialized := owned.FunctionOperations[local]
			catalogue.functions = append(catalogue.functions, Function{
				providerID:    item.Identity,
				bindings:      []Binding{{mount: item.Mount, modulePath: owned.Path, member: strings.Split(local, ".")}},
				canonicalPath: name,
				signature:     owned.ScopeSignature(function),
				operation:     moduleio.CloneOperation(operation),
				hasOperation:  specialized,
			})
		}
		for local, function := range owned.DetachedFunctions {
			name := qualify(owned.Path, local)
			if _, exists := catalogue.byFunctionPath[name]; exists {
				return nil, fmt.Errorf("manifest: duplicate detached function path %q", name)
			}
			operation, specialized := owned.FunctionOperations[local]
			if !specialized {
				return nil, fmt.Errorf("manifest: detached function %q has no operation", name)
			}
			catalogue.byFunctionPath[name] = len(catalogue.functions)
			catalogue.functions = append(catalogue.functions, Function{
				providerID: item.Identity, canonicalPath: name,
				signature: owned.ScopeSignature(function), operation: moduleio.CloneOperation(operation), hasOperation: true,
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

// InitialEnvironments returns only providers that declare additional
// bootstrap structure. Ordinary module/global mounts are derived elsewhere
// from the same provider rows.
func (c *Catalogue) InitialEnvironments() []InitialEnvironment {
	if c == nil {
		return nil
	}
	var out []InitialEnvironment
	for _, item := range c.providers {
		if len(item.manifest.InitialRoots) == 0 && len(item.manifest.InitialEntries) == 0 && len(item.manifest.InitialMetatables) == 0 {
			continue
		}
		out = append(out, InitialEnvironment{
			providerID: item.identity,
			modulePath: item.manifest.Path,
			mount:      item.mount,
			roots:      append([]moduleio.InitialRoot(nil), item.manifest.InitialRoots...),
			entries:    append([]moduleio.InitialEntry(nil), item.manifest.InitialEntries...),
			metatables: append([]moduleio.InitialMetatableAttachment(nil), item.manifest.InitialMetatables...),
		})
	}
	return out
}

func clone(input *moduleio.Manifest) (*moduleio.Manifest, error) {
	encoded, err := moduleio.Encode(input)
	if err != nil {
		return nil, err
	}
	owned, err := moduleio.Decode(encoded)
	return owned, err
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

// TypestateProtocols returns every declared state machine in the catalogue,
// keyed by protocol, in ownership-isolated normalized form. Seal has already
// refused a disagreeing redeclaration, so the answer is one machine per name.
func (c *Catalogue) TypestateProtocols() map[typestate.Protocol]typestate.Definition {
	if c == nil || len(c.typestateProtocols) == 0 {
		return nil
	}
	out := make(map[typestate.Protocol]typestate.Definition, len(c.typestateProtocols))
	for protocol, definition := range c.typestateProtocols {
		out[protocol] = definition.Clone()
	}
	return out
}

// typestateDefinitionEqual compares two normalized machines by value. Both
// sides come from Definition.Normalized, so equal declarations have identical
// element order.
func typestateDefinitionEqual(left, right typestate.Definition) bool {
	return left.Protocol == right.Protocol &&
		slices.Equal(left.States, right.States) &&
		slices.Equal(left.FinalStates, right.FinalStates) &&
		slices.Equal(left.Transitions, right.Transitions)
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

// Values returns every mounted non-callable export directly from provider
// manifests. It is deliberately projected on demand rather than cached as a
// second export registry.
func (c *Catalogue) Values() []Value {
	if c == nil {
		return nil
	}
	var out []Value
	for _, item := range c.providers {
		if item.mount == MountDetached {
			continue
		}
		seen := make(map[string]struct{})
		callable := func(name string) bool {
			_, ok := item.manifest.FunctionSignatures[name]
			return ok
		}
		if record, ok := unwrap.Annotated(item.manifest.Export).(*typ.Record); ok {
			for _, field := range record.Fields {
				if callable(field.Name) {
					continue
				}
				binding := Binding{mount: item.mount, modulePath: item.manifest.Path, member: []string{field.Name}}
				out = append(out, Value{providerID: item.identity, binding: binding, valueType: item.manifest.ScopeType(field.Type), immutable: item.immutable})
				seen[valueBindingKey(binding)] = struct{}{}
			}
		}
		for name, valueType := range item.manifest.GlobalTypes {
			if callable(name) {
				continue
			}
			binding := Binding{mount: MountGlobals, modulePath: item.manifest.Path, member: []string{name}}
			if _, exists := seen[valueBindingKey(binding)]; exists {
				continue
			}
			out = append(out, Value{providerID: item.identity, binding: binding, valueType: item.manifest.ScopeType(valueType), immutable: false})
		}
	}
	sort.Slice(out, func(left, right int) bool {
		return valueBindingKey(out[left].binding) < valueBindingKey(out[right].binding)
	})
	return out
}

func valueBindingKey(binding Binding) string {
	return fmt.Sprintf("%d/%s/%s", binding.mount, binding.modulePath, strings.Join(binding.member, "\x00"))
}

func (f Function) clone() Function {
	f.bindings = f.Bindings()
	f.signature = f.signature.Clone()
	f.operation = moduleio.CloneOperation(f.operation)
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
