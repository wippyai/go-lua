package wire

import (
	"strings"

	"github.com/wippyai/go-lua/types/signature"
)

// Rebound returns a shallow copy of m addressed under newPath: its Path becomes
// newPath and every function signature keyed under the old path prefix is
// re-keyed under newPath.
//
// A module manifest is exported keyed by the defining module's own path
// (mod.f -> "<export path>.f"). When a consumer imports it under a different
// alias, callers resolve signatures by "<alias>.f". Without re-keying, that
// lookup misses and the signature's effects - the value/error correlation, the
// parameter narrowing postconditions a guard helper proves - are silently lost.
// Rebinding keeps the keys consistent with the Path the manifest is consumed
// under. When newPath equals the current Path the original manifest is returned
// unchanged.
func (m *Manifest) Rebound(newPath string) *Manifest {
	if m == nil || newPath == "" || newPath == m.Path {
		return m
	}
	clone := *m
	clone.Path = newPath
	clone.FunctionSignatures = rekeyedSignatures(m.FunctionSignatures, m.Path, newPath)
	return &clone
}

// rekeyedSignatures returns a copy of sigs with every key under oldPath's prefix
// moved to newPath. Keys outside that prefix are preserved verbatim.
func rekeyedSignatures(sigs map[string]signature.Function, oldPath, newPath string) map[string]signature.Function {
	if len(sigs) == 0 || oldPath == "" {
		return sigs
	}
	out := make(map[string]signature.Function, len(sigs))
	for key, sig := range sigs {
		out[rekeyName(key, oldPath, newPath)] = sig
	}
	return out
}

// rekeyName rewrites a single signature name from the oldPath prefix to newPath,
// covering the exact module name and its field ("oldPath.member") and index
// ("oldPath[...]") member forms.
func rekeyName(key, oldPath, newPath string) string {
	switch {
	case key == oldPath:
		return newPath
	case strings.HasPrefix(key, oldPath+"."):
		return newPath + strings.TrimPrefix(key, oldPath)
	case strings.HasPrefix(key, oldPath+"["):
		return newPath + strings.TrimPrefix(key, oldPath)
	default:
		return key
	}
}
