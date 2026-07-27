// Package signaturelookup merges bounded function signature sources for module
// analysis.
package signaturelookup

import (
	"fmt"
	"strings"

	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup/internal/stdlib"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// HasCapability reports whether the immutable signature selected for name
// carries the requested canonical capability identity.
func (s Source) HasCapability(name, id string) bool {
	if id == "" {
		return false
	}
	sig, ok := s.LookupView(name)
	if !ok {
		return false
	}
	for _, label := range sig.Effect.Labels {
		if candidate, mapped := caplabel.IDFor(label); mapped && candidate == id {
			return true
		}
	}
	return false
}

func HasStdlibCapability(name, id string) bool {
	return (Source{IncludeStdlib: true}).HasCapability(name, id)
}

// Source is a narrow read view over effect-bearing function signatures.
//
// Stdlib is the fallback. Manifests override it, and later manifests override
// earlier manifests.
type Source struct {
	Manifests     []*manifest.Manifest
	IncludeStdlib bool
}

// Identity is an opaque registry selection. Consumers that recognize native
// operations pass this value instead of reconstructing identity from source
// text.
type Identity struct {
	name      string
	signature signature.Function
	modeled   bool
}

// RegistryIdentity resolves name through this source and seals the selected
// registry entry into an opaque identity.
func (s Source) RegistryIdentity(name string) (Identity, bool) {
	if sig, ok := s.LookupView(name); ok {
		return Identity{name: name, signature: sig, modeled: true}, true
	}
	if s.IncludeStdlib && !strings.Contains(name, ".") {
		for _, bare := range stdlib.BareGlobals() {
			if bare == name {
				return Identity{name: name}, true
			}
		}
	}
	return Identity{}, false
}

// Name returns the spelling sealed by the registry lookup.
func (identity Identity) Name() string { return identity.name }

// Signature returns the selected modeled signature. Declared bare globals are
// valid identities but intentionally have no modeled signature.
func (identity Identity) Signature() (signature.Function, bool) {
	return identity.signature, identity.name != "" && identity.modeled
}

// Validate checks in-memory manifest metadata before analysis consumes it.
func (s Source) Validate() error {
	for i, m := range s.Manifests {
		if m == nil {
			continue
		}
		if err := m.Validate(); err != nil {
			if m.Path != "" {
				return fmt.Errorf("signature manifest %q: %w", m.Path, err)
			}
			return fmt.Errorf("signature manifest %d: %w", i, err)
		}
	}
	return nil
}

// LookupView returns a borrowed immutable signature for analysis hot paths.
// The returned value and all reachable slices/types remain owned by the source;
// callers that may mutate them must explicitly Clone. Manifest-local type
// scoping can construct a new immutable view, but deliberately does not perform
// the otherwise redundant ownership clone.
func (s Source) LookupView(name string) (signature.Function, bool) {
	if name == "" {
		return signature.Function{}, false
	}
	if s.IncludeStdlib && isBareStdlibName(name) {
		if sig, ok := lookupGlobalManifestSignature(s.Manifests, name); ok {
			return sig, true
		}
		return stdlib.LookupView(name)
	}
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil {
			continue
		}
		if sig, ok := lookupManifestSignature(m, name); ok {
			return m.ScopeSignature(sig), true
		}
	}
	if s.IncludeStdlib {
		return stdlib.LookupView(name)
	}
	return signature.Function{}, false
}

func isBareStdlibName(name string) bool {
	if name == "" || strings.ContainsAny(name, ".[") {
		return false
	}
	_, ok := stdlib.LookupView(name)
	return ok
}

func lookupGlobalManifestSignature(manifests []*manifest.Manifest, name string) (signature.Function, bool) {
	for i := len(manifests) - 1; i >= 0; i-- {
		m := manifests[i]
		if m == nil || m.Path != "" {
			continue
		}
		if sig, ok := m.FunctionSignatures[name]; ok {
			return sig, true
		}
	}
	return signature.Function{}, false
}

func lookupManifestSignature(m *manifest.Manifest, name string) (signature.Function, bool) {
	if m == nil {
		return signature.Function{}, false
	}
	if sig, ok := m.FunctionSignatures[name]; ok {
		return sig, true
	}
	if local, ok := manifestLocalSignatureName(m.Path, name); ok {
		if sig, ok := m.FunctionSignatures[local]; ok {
			return sig, true
		}
	}
	// No explicit signature: recover one from the module's declared types.
	return deriveManifestSignature(m, name)
}

func manifestLocalSignatureName(modulePath, name string) (string, bool) {
	if modulePath == "" || name == modulePath {
		return "", false
	}
	if strings.HasPrefix(name, modulePath+".") {
		local := strings.TrimPrefix(name, modulePath+".")
		return local, local != ""
	}
	if strings.HasPrefix(name, modulePath+"[") {
		local := strings.TrimPrefix(name, modulePath)
		return local, local != ""
	}
	return "", false
}

// StdlibSignatureNames returns the standard-library function names without
// materializing cloned signatures.
func StdlibSignatureNames() []string {
	return stdlib.SignatureNames()
}

// StdlibBareGlobals returns the standard global names that are recognized by
// name but not modeled with a signature (the global table, version constants,
// and standard library tables without per-member declarations).
func StdlibBareGlobals() []string {
	return stdlib.BareGlobals()
}

// StdlibResultSlot returns one finite declared return slot from the Lua
// standard-library contract.  It is intentionally independent of manifests:
// provider-boundary projection consumes the runtime library specification, not
// a module-local override.
func StdlibResultSlot(name string, index int) (typ.Type, bool) {
	return stdlib.ResultSlot(name, index)
}

// StdlibResultSlotCondition is a literal-argument refinement declared by the
// Lua standard-library contract.
type StdlibResultSlotCondition struct {
	ResultIndex    int
	ArgumentIndex  int
	ArgumentString string
	ResultType     typ.Type
}

// StdlibConditionalResultSlots exposes the declarative conditional slots for
// one standard-library provider.
func StdlibConditionalResultSlots(name string) []StdlibResultSlotCondition {
	items := stdlib.ConditionalResultSlots(name)
	out := make([]StdlibResultSlotCondition, len(items))
	for index, item := range items {
		out[index] = StdlibResultSlotCondition{
			ResultIndex: item.ResultIndex, ArgumentIndex: item.ArgumentIndex,
			ArgumentString: item.ArgumentString, ResultType: item.ResultType,
		}
	}
	return out
}

// StdlibPositionalResultSlot is a length-bounded position condition declared by
// the Lua standard-library contract for one optional result slot.
type StdlibPositionalResultSlot struct {
	ResultIndex      int
	SubjectArgument  int
	PositionArgument int
	DefaultPosition  int64
}

// StdlibPositionalResultSlots exposes the declarative positional conditions for
// one standard-library provider.
func StdlibPositionalResultSlots(name string) []StdlibPositionalResultSlot {
	items := stdlib.PositionalResultSlots(name)
	out := make([]StdlibPositionalResultSlot, len(items))
	for index, item := range items {
		out[index] = StdlibPositionalResultSlot{
			ResultIndex: item.ResultIndex, SubjectArgument: item.SubjectArgument,
			PositionArgument: item.PositionArgument, DefaultPosition: item.DefaultPosition,
		}
	}
	return out
}

// StdlibMethodProvider returns the canonical global name of a typed standard
// library method.  The decision is owned by the standard-library contract
// table, rather than by call-site name matching.
func StdlibMethodProvider(receiver typ.Type, method string) (string, bool) {
	return stdlib.MethodProvider(receiver, method)
}
