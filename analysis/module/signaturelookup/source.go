// Package signaturelookup merges bounded function signature sources for module
// analysis.
package signaturelookup

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/module/manifest"
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

// Lookup returns a cloned signature for name.
func (s Source) Lookup(name string) (signature.Function, bool) {
	if name == "" {
		return signature.Function{}, false
	}
	for i := len(s.Manifests) - 1; i >= 0; i-- {
		m := s.Manifests[i]
		if m == nil {
			continue
		}
		sig, ok := m.FunctionSignatures[name]
		if ok {
			return sig.Clone(), true
		}
	}
	if s.IncludeStdlib {
		return stdlib.Lookup(name)
	}
	return signature.Function{}, false
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
			out[name] = sig.Clone()
		}
	}
	return out
}

// StdlibSignatures returns cloned standard-library signatures.
func StdlibSignatures() map[string]signature.Function {
	return stdlib.Signatures()
}
