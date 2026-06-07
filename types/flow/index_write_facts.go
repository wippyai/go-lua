package flow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// IndexWriteAdmissionFact is a point-local must-fact proving that the transfer
// admitted one dynamic-index replacement write at this point.
//
// Target is the structural container path before the dynamic index. KeyPath and
// ValuePath are optional identity refinements when the source syntax lowers to a
// stable path. Key and Value carry the abstract values used by the value-domain
// admission law, so consumers do not re-evaluate write legality.
type IndexWriteAdmissionFact struct {
	Target    constraint.PathKey
	KeyPath   constraint.PathKey
	Key       product.AbstractValue
	ValuePath constraint.PathKey
	Value     product.AbstractValue
}

// AddressFact projects a stored fact into the public address-domain form.
// Stored keys are private deterministic identities; consumers that reason about
// source locations should use this view rather than parsing keys themselves.
func (fact IndexWriteAdmissionFact) AddressFact() (IndexWriteAdmissionAddressFact, bool) {
	target, ok := StableAddressFromCanonicalKey(fact.Target)
	if !ok || fact.Key.IsZero() || fact.Value.IsZero() {
		return IndexWriteAdmissionAddressFact{}, false
	}
	addressFact := IndexWriteAdmissionAddressFact{
		Target: target,
		Key:    fact.Key,
		Value:  fact.Value,
	}
	if fact.KeyPath != "" {
		keyPath, ok := StableAddressFromCanonicalKey(fact.KeyPath)
		if !ok {
			return IndexWriteAdmissionAddressFact{}, false
		}
		addressFact.KeyPath = keyPath
		addressFact.HasKeyPath = true
	}
	if fact.ValuePath != "" {
		valuePath, ok := StableAddressFromCanonicalKey(fact.ValuePath)
		if !ok {
			return IndexWriteAdmissionAddressFact{}, false
		}
		addressFact.ValuePath = valuePath
		addressFact.HasValuePath = true
	}
	return addressFact, true
}

// IndexWriteAdmissionAddressFact is the normalized publication form for
// dynamic-index write readback proofs. The finite domain stores deterministic
// private keys internally, but callers should publish structural identities as
// StableAddress values.
type IndexWriteAdmissionAddressFact struct {
	Target       StableAddress
	KeyPath      StableAddress
	HasKeyPath   bool
	Key          product.AbstractValue
	ValuePath    StableAddress
	HasValuePath bool
	Value        product.AbstractValue
}

// IndexWriteAdmissionFacts is the finite must-fact domain for admitted dynamic
// index writes. Bottom is unreachable, Top is the empty proof set, and Join keeps
// only proofs present on every incoming path.
type IndexWriteAdmissionFacts struct {
	bottom  bool
	entries []IndexWriteAdmissionFact
}

// IndexWriteAddressQuery is the normalized address-domain query for admitted
// dynamic-index write readback.
type IndexWriteAddressQuery struct {
	Target       StableAddress
	KeyPath      StableAddress
	HasKeyPath   bool
	KeyValue     product.AbstractValue
	ValuePath    StableAddress
	HasValuePath bool
}

// IndexWriteAdmissionFactsOf constructs a canonical finite proof set.
func IndexWriteAdmissionFactsOf(entries []IndexWriteAdmissionFact) IndexWriteAdmissionFacts {
	return canonicalIndexWriteAdmissionFacts(entries, product.Domain.Join)
}

func (f IndexWriteAdmissionFacts) IsBottom() bool { return f.bottom }

// Entries returns a defensive copy of the finite entries. Bottom has no
// consumable finite entries.
func (f IndexWriteAdmissionFacts) Entries() []IndexWriteAdmissionFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]IndexWriteAdmissionFact(nil), f.entries...)
}

// With returns f with fact added or merged into the canonical proof set.
func (f IndexWriteAdmissionFacts) With(fact IndexWriteAdmissionFact) IndexWriteAdmissionFacts {
	if !validIndexWriteAdmissionFact(fact) {
		return f
	}
	if f.bottom {
		f = IndexWriteAdmissionFacts{}
	}
	next := f.Entries()
	next = append(next, fact)
	return canonicalIndexWriteAdmissionFacts(next, product.Domain.Join)
}

// WithAddress returns f with fact added or merged into the canonical proof set.
func (f IndexWriteAdmissionFacts) WithAddress(fact IndexWriteAdmissionAddressFact) IndexWriteAdmissionFacts {
	keyed, ok := indexWriteAdmissionFactFromAddress(fact)
	if !ok {
		return f
	}
	return f.With(keyed)
}

// ForEachAddress visits each admission fact through its normalized address view.
func (f IndexWriteAdmissionFacts) ForEachAddress(fn func(IndexWriteAdmissionAddressFact) bool) {
	if f.bottom || len(f.entries) == 0 || fn == nil {
		return
	}
	for _, entry := range f.entries {
		fact, ok := entry.AddressFact()
		if !ok {
			continue
		}
		if !fn(fact) {
			return
		}
	}
}

// WithAliasedKeyPathAddress copies all admissions whose key identity is source
// under target. This keeps key-path replay inside the admission domain instead
// of exposing raw storage keys to transfer proofs.
func (f IndexWriteAdmissionFacts) WithAliasedKeyPathAddress(
	source StableAddress,
	target StableAddress,
) IndexWriteAdmissionFacts {
	sourceKey := source.Key()
	targetKey := target.Key()
	if f.bottom || sourceKey == "" || targetKey == "" || len(f.entries) == 0 {
		return f
	}
	out := f
	for _, entry := range f.entries {
		if entry.KeyPath != sourceKey {
			continue
		}
		next := entry
		next.KeyPath = targetKey
		out = out.With(next)
	}
	return out
}

// AdmissionAtAddress returns the admitted value proof matching q.
func (f IndexWriteAdmissionFacts) AdmissionAtAddress(q IndexWriteAddressQuery) (product.AbstractValue, bool) {
	if f.bottom || len(f.entries) == 0 {
		return product.AbstractValue{}, false
	}
	target := q.Target.Key()
	if target == "" {
		return product.AbstractValue{}, false
	}
	for _, entry := range f.entries {
		if entry.Target != target {
			continue
		}
		matchedKeyPath := false
		if q.HasKeyPath && entry.KeyPath != "" {
			keyPath := q.KeyPath.Key()
			if keyPath == "" || entry.KeyPath != keyPath {
				continue
			}
			matchedKeyPath = true
		}
		if q.HasValuePath && entry.ValuePath != "" {
			valuePath := q.ValuePath.Key()
			if valuePath == "" || entry.ValuePath != valuePath {
				continue
			}
		}
		if !matchedKeyPath && !indexWriteKeyValueExactlyMatches(entry.Key, q.KeyValue) {
			continue
		}
		return entry.Value, true
	}
	return product.AbstractValue{}, false
}

// KillAffectedByWriteAddress removes admission facts that are no longer valid
// after a write to write.
func (f IndexWriteAdmissionFacts) KillAffectedByWriteAddress(write StableAddress) IndexWriteAdmissionFacts {
	if f.bottom || write.Key() == "" || len(f.entries) == 0 {
		return f
	}
	entries := make([]IndexWriteAdmissionFact, 0, len(f.entries))
	for _, entry := range f.entries {
		if indexWritePathOverlapsAddress(entry.Target, write) ||
			indexWritePathOverlapsAddress(entry.KeyPath, write) ||
			indexWritePathOverlapsAddress(entry.ValuePath, write) {
			continue
		}
		entries = append(entries, entry)
	}
	return canonicalIndexWriteAdmissionFacts(entries, product.Domain.Join)
}

// PreservePresentElementWriteAddress applies the invalidation law for a
// definitely-present dynamic element write to write.
func (f IndexWriteAdmissionFacts) PreservePresentElementWriteAddress(
	write StableAddress,
	written product.AbstractValue,
) IndexWriteAdmissionFacts {
	if !written.DefinitelyPresent() {
		return f.KillAffectedByWriteAddress(write)
	}
	writePath := write.Key()
	if f.bottom || writePath == "" || len(f.entries) == 0 {
		return f
	}
	entries := make([]IndexWriteAdmissionFact, 0, len(f.entries))
	for _, entry := range f.entries {
		if indexWritePathOverlapsAddress(entry.KeyPath, write) ||
			indexWritePathOverlapsAddress(entry.ValuePath, write) {
			continue
		}
		if !indexWritePathOverlapsAddress(entry.Target, write) {
			entries = append(entries, entry)
			continue
		}
		if entry.Target != writePath {
			continue
		}
		entry.Value = product.Domain.Join(entry.Value, written)
		if validIndexWriteAdmissionFact(entry) {
			entries = append(entries, entry)
		}
	}
	return canonicalIndexWriteAdmissionFacts(entries, product.Domain.Join)
}

func indexWriteKeyValueExactlyMatches(fact, query product.AbstractValue) bool {
	if fact.IsZero() || query.IsZero() {
		return false
	}
	return product.Domain.LessOrEq(fact, query) && product.Domain.LessOrEq(query, fact)
}

func indexWriteAdmissionFactFromAddress(fact IndexWriteAdmissionAddressFact) (IndexWriteAdmissionFact, bool) {
	target := fact.Target.Key()
	if target == "" || fact.Key.IsZero() || fact.Value.IsZero() {
		return IndexWriteAdmissionFact{}, false
	}
	keyed := IndexWriteAdmissionFact{
		Target: target,
		Key:    fact.Key,
		Value:  fact.Value,
	}
	if fact.HasKeyPath {
		keyPath := fact.KeyPath.Key()
		if keyPath == "" {
			return IndexWriteAdmissionFact{}, false
		}
		keyed.KeyPath = keyPath
	}
	if fact.HasValuePath {
		valuePath := fact.ValuePath.Key()
		if valuePath == "" {
			return IndexWriteAdmissionFact{}, false
		}
		keyed.ValuePath = valuePath
	}
	return keyed, true
}

func indexWritePathOverlapsAddress(path constraint.PathKey, addr StableAddress) bool {
	pathAddr, ok := StableAddressFromCanonicalKey(path)
	return ok && pathAddr.Overlaps(addr)
}

func validIndexWriteAdmissionFact(fact IndexWriteAdmissionFact) bool {
	return fact.Target != "" && !fact.Key.IsZero() && !fact.Value.IsZero()
}

func (f IndexWriteAdmissionFacts) coversWithAbsentKeyPaths(
	want IndexWriteAdmissionFacts,
	pred func(product.AbstractValue, product.AbstractValue) bool,
	absent func(constraint.PathKey) bool,
) bool {
	if f.bottom {
		return true
	}
	if want.bottom {
		return false
	}
	for _, entry := range want.entries {
		if idx, ok := findIndexWriteAdmissionFact(f.entries, entry); ok {
			have := f.entries[idx]
			if pred(have.Key, entry.Key) && pred(have.Value, entry.Value) {
				continue
			}
		}
		if entry.KeyPath != "" && indexWriteAdmissionAbsent(absent, entry.KeyPath) {
			continue
		}
		return false
	}
	return true
}

func (f IndexWriteAdmissionFacts) withFactsProvedByAbsentKeyPaths(
	facts IndexWriteAdmissionFacts,
	absent func(constraint.PathKey) bool,
) IndexWriteAdmissionFacts {
	if facts.bottom {
		return f
	}
	out := f
	for _, entry := range facts.entries {
		if entry.KeyPath == "" || out.hasIdentity(entry) {
			continue
		}
		if indexWriteAdmissionAbsent(absent, entry.KeyPath) {
			out = out.With(entry)
		}
	}
	return out
}

func (f IndexWriteAdmissionFacts) hasIdentity(fact IndexWriteAdmissionFact) bool {
	if f.bottom {
		return false
	}
	_, ok := findIndexWriteAdmissionFact(f.entries, fact)
	return ok
}

func indexWriteAdmissionAbsent(absent func(constraint.PathKey) bool, key constraint.PathKey) bool {
	return absent != nil && absent(key)
}

// Format renders f deterministically for tests and diagnostics.
func (f IndexWriteAdmissionFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 {
		return "⊤"
	}
	parts := make([]string, 0, len(f.entries))
	for _, entry := range f.entries {
		parts = append(parts, fmt.Sprintf("%s[%s:%s]=%s:%s",
			entry.Target,
			entry.KeyPath,
			productFormat(entry.Key),
			entry.ValuePath,
			productFormat(entry.Value),
		))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// IndexWriteAdmissionFactsDomain is the finite must-set lattice over admitted
// dynamic-index writes. Join/Widen intersect proof identities and join/widen
// their product values when the same proof survives both paths.
var IndexWriteAdmissionFactsDomain = lattice.Lattice[IndexWriteAdmissionFacts]{
	Bottom: func() IndexWriteAdmissionFacts {
		return IndexWriteAdmissionFacts{bottom: true}
	},
	Top: func() IndexWriteAdmissionFacts {
		return IndexWriteAdmissionFacts{}
	},
	Equal: func(a, b IndexWriteAdmissionFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return indexWriteAdmissionRowIdentity.EqualBy(a.entries, b.entries, indexWriteAdmissionFactEqual)
	},
	LessOrEq: func(a, b IndexWriteAdmissionFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return indexWriteAdmissionFactsContainAll(a.entries, b.entries, product.Domain.LessOrEq)
	},
	Join: func(a, b IndexWriteAdmissionFacts) IndexWriteAdmissionFacts {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		return intersectIndexWriteAdmissionFacts(a, b, product.Domain.Join)
	},
	Meet: nil,
	Widen: func(prev, next IndexWriteAdmissionFacts) IndexWriteAdmissionFacts {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		return intersectIndexWriteAdmissionFacts(prev, next, product.Domain.Widen)
	},
}

func canonicalIndexWriteAdmissionFacts(
	entries []IndexWriteAdmissionFact,
	merge func(product.AbstractValue, product.AbstractValue) product.AbstractValue,
) IndexWriteAdmissionFacts {
	if len(entries) == 0 {
		return IndexWriteAdmissionFacts{}
	}
	out := append([]IndexWriteAdmissionFact(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return indexWriteAdmissionFactLess(out[i], out[j]) })
	dst := out[:0]
	for _, entry := range out {
		if !validIndexWriteAdmissionFact(entry) {
			continue
		}
		if len(dst) > 0 && indexWriteAdmissionSameIdentity(dst[len(dst)-1], entry) {
			dst[len(dst)-1].Key = merge(dst[len(dst)-1].Key, entry.Key)
			dst[len(dst)-1].Value = merge(dst[len(dst)-1].Value, entry.Value)
			if !validIndexWriteAdmissionFact(dst[len(dst)-1]) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, entry)
	}
	return IndexWriteAdmissionFacts{entries: append([]IndexWriteAdmissionFact(nil), dst...)}
}

func indexWriteAdmissionFactEqual(a, b IndexWriteAdmissionFact) bool {
	return a.Target == b.Target &&
		a.KeyPath == b.KeyPath &&
		a.ValuePath == b.ValuePath &&
		product.Domain.Equal(a.Key, b.Key) &&
		product.Domain.Equal(a.Value, b.Value)
}

func indexWriteAdmissionSameIdentity(a, b IndexWriteAdmissionFact) bool {
	return a.Target == b.Target && a.KeyPath == b.KeyPath && a.ValuePath == b.ValuePath
}

func indexWriteAdmissionFactLess(a, b IndexWriteAdmissionFact) bool {
	if a.Target != b.Target {
		return a.Target < b.Target
	}
	if a.KeyPath != b.KeyPath {
		return a.KeyPath < b.KeyPath
	}
	return a.ValuePath < b.ValuePath
}

func indexWriteAdmissionFactsContainAll(
	have, want []IndexWriteAdmissionFact,
	pred func(product.AbstractValue, product.AbstractValue) bool,
) bool {
	return indexWriteAdmissionRowIdentity.ContainsAllBy(have, want, func(have, want IndexWriteAdmissionFact) bool {
		return indexWriteAdmissionSameIdentity(have, want) &&
			pred(have.Key, want.Key) &&
			pred(have.Value, want.Value)
	})
}

func intersectIndexWriteAdmissionFacts(
	a, b IndexWriteAdmissionFacts,
	op func(product.AbstractValue, product.AbstractValue) product.AbstractValue,
) IndexWriteAdmissionFacts {
	out := indexWriteAdmissionRowIdentity.MergeIntersect(a.entries, b.entries, func(left, right IndexWriteAdmissionFact) (IndexWriteAdmissionFact, bool) {
		return IndexWriteAdmissionFact{
			Target:    left.Target,
			KeyPath:   left.KeyPath,
			Key:       op(left.Key, right.Key),
			ValuePath: left.ValuePath,
			Value:     op(left.Value, right.Value),
		}, true
	})
	return canonicalIndexWriteAdmissionFacts(out, op)
}

func findIndexWriteAdmissionFact(entries []IndexWriteAdmissionFact, fact IndexWriteAdmissionFact) (int, bool) {
	return indexWriteAdmissionRowIdentity.Find(entries, fact)
}

var indexWriteAdmissionRowIdentity = orderedRowIdentity[IndexWriteAdmissionFact]{
	less: indexWriteAdmissionFactLess,
	same: indexWriteAdmissionSameIdentity,
}

func productFormat(av product.AbstractValue) string {
	if av.IsZero() {
		return "⊥"
	}
	return fmt.Sprint(product.ProjectValueOrUnknown(av))
}
