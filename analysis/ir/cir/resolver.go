package cir

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// AccessMode selects how an operand's source path is projected onto a state-cell
// key at a CFG point. The same source operand names different state cells under
// different modes: a read observes the value visible on entry to the point,
// while a write defines a fresh point-local cell. Modes are the closed set the
// transfer interpreter needs; lowering never chooses a mode, it only records the
// operand.
type AccessMode uint8

const (
	// AccessReadBefore resolves the operand at the input to point: the value
	// visible before the point's own writes. This is the mode for operand reads.
	AccessReadBefore AccessMode = iota
	// AccessWriteLocal resolves the operand to the point-local write cell: the
	// fresh SSA version a write at point defines. This is the mode for a Dst.
	AccessWriteLocal
	// AccessRootOrVisible resolves a symbol root structurally but member paths
	// under the point-visible SSA version. Facts stored root-relative use it.
	AccessRootOrVisible
	// AccessEvidence resolves the operand to its path-evidence key lane, the cell
	// carrying branch/relation evidence rather than a value.
	AccessEvidence
)

// String returns the mode mnemonic, for diagnostics and cache introspection.
func (m AccessMode) String() string {
	switch m {
	case AccessReadBefore:
		return "read-before"
	case AccessWriteLocal:
		return "write-local"
	case AccessRootOrVisible:
		return "root-or-visible"
	case AccessEvidence:
		return "evidence"
	default:
		return "mode?"
	}
}

// AddressResolver maps a cir operand at a CFG point to an opaque state-cell key.
//
// Operand identity is not state-cell identity by design (decision D2): a cir
// operand is a source path ref, stable across the whole Body, whereas the state
// cell it addresses depends on the point (SSA version), the access mode
// (before/after, root/visible, evidence), and visibility. The resolver owns that
// projection; lowering stays free of it.
//
// The production implementation binds to the engine visibility resolver inside
// factapply and is constructed per Body (it closes over the Body to decode an
// operand's interned path). cir defines only the contract and a test fake.
//
// Caching contract: Resolve is a pure function of (point, op, mode) for a fixed
// Body, so a resolver MAY memoize its result and callers MAY assume repeated
// calls with equal arguments return the same key without side effects. A
// non-path operand (temp, const, vararg, none) has no addressable cell and
// resolves to the invalid zero key with ok == false. CachingResolver enforces
// the contract over any inner resolver.
type AddressResolver interface {
	Resolve(point cfg.Point, op Operand, mode AccessMode) (keyspace.Key, bool)
}

// resolveKey is the (point, operand, mode) tuple that keys the resolution cache.
// It is comparable so it maps directly.
type resolveKey struct {
	point cfg.Point
	op    Operand
	mode  AccessMode
}

// CachingResolver memoizes an inner AddressResolver by (point, op, mode),
// realizing the caching contract for any resolver that does not memoize itself.
// It is not safe for concurrent use.
type CachingResolver struct {
	inner AddressResolver
	cache map[resolveKey]cachedKey
}

type cachedKey struct {
	key keyspace.Key
	ok  bool
}

// NewCachingResolver wraps inner with a per-(point, op, mode) memo.
func NewCachingResolver(inner AddressResolver) *CachingResolver {
	return &CachingResolver{inner: inner, cache: make(map[resolveKey]cachedKey)}
}

// Resolve returns the cached key for (point, op, mode), computing and storing it
// through the inner resolver on the first request.
func (r *CachingResolver) Resolve(point cfg.Point, op Operand, mode AccessMode) (keyspace.Key, bool) {
	k := resolveKey{point: point, op: op, mode: mode}
	if hit, ok := r.cache[k]; ok {
		return hit.key, hit.ok
	}
	key, ok := r.inner.Resolve(point, op, mode)
	r.cache[k] = cachedKey{key: key, ok: ok}
	return key, ok
}
