// Package signaturelookup merges bounded function signature sources for module
// analysis.
package signaturelookup

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup/internal/stdlib"
)

// Source is a narrow read view over effect-bearing function signatures.
//
// Stdlib is the fallback. Manifests override it, and later manifests override
// earlier manifests.
type Source struct {
	Manifests     []*manifest.Manifest
	IncludeStdlib bool
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

// Lookup returns a cloned signature for name.
func (s Source) Lookup(name string) (signature.Function, bool) {
	if name == "" {
		return signature.Function{}, false
	}
	if s.IncludeStdlib && isBareStdlibName(name) {
		if sig, ok := lookupGlobalManifestSignature(s.Manifests, name); ok {
			return sig.Clone(), true
		}
		return stdlib.Lookup(name)
	}
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil {
			continue
		}
		if sig, ok := lookupManifestSignature(m, name); ok {
			return sig.Clone(), true
		}
	}
	if s.IncludeStdlib {
		return stdlib.Lookup(name)
	}
	return signature.Function{}, false
}

func isBareStdlibName(name string) bool {
	if name == "" || strings.ContainsAny(name, ".[") {
		return false
	}
	_, ok := stdlib.Lookup(name)
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

// Signatures returns a cloned merged map, with manifest entries overriding
// stdlib entries for the same stable function name.
func (s Source) Signatures() map[string]signature.Function {
	out := make(map[string]signature.Function)
	if s.IncludeStdlib {
		for name, sig := range stdlib.Signatures() {
			out[name] = sig
		}
	}
	for _, m := range s.Manifests {
		if m == nil {
			continue
		}
		for name, sig := range m.FunctionSignatures {
			if s.IncludeStdlib && m.Path != "" && isBareStdlibName(name) {
				continue
			}
			out[name] = sig.Clone()
		}
	}
	return out
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
