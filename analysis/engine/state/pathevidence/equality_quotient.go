package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// EqualityQuotient is the finite ground congruence induced by the path-equality
// proofs in one Lane. It is prepared once after proof publication and shared by
// every registered axis that stores path-keyed identities.
type EqualityQuotient struct{ seal *equalityQuotientSeal }

type equalityQuotientSeal struct {
	keys       *keyspace.KeySpace
	congruence *pathCongruence
	byHash     map[uint64][]*equalityQuotientClass
}

type equalityQuotientClass struct {
	normal  pathCongruenceNormal
	members []keyspace.Key
}

// EqualityClass is an opaque, comparable identity for one exact congruence
// class. Participants can memoize their own canonical projection once per
// class instead of rescanning all observations for every stored resource.
type EqualityClass struct {
	seal  *equalityQuotientSeal
	class *equalityQuotientClass
}

// SealEqualityQuotient freezes the equality relation and indexes its finite
// observed term universe by exact normal form. Non-equality facts contribute
// observations but never equations.
func (l Lane) SealEqualityQuotient(keys *keyspace.KeySpace) (EqualityQuotient, bool) {
	if keys == nil || !keys.Valid() || l.proofsBottom {
		return EqualityQuotient{}, false
	}
	observations := make([]keyspace.Key, 0)
	seen := make(map[keyspace.KeyHandle]struct{})
	l.forEachCongruenceObservation(keys, func(candidate keyspace.Key) {
		// A lane can retain observations imported from another lexical
		// keyspace.  They are not terms in this quotient's algebra and cannot
		// affect any participant indexed by keys.  Filtering the carrier's
		// observation universe here matches EquivalentKeyspaceKeys, whose
		// normalization also ignores non-members, and prevents one foreign
		// observation from making an otherwise exact local quotient unsealable.
		if !pathCongruenceGroundTermValid(keys, candidate) {
			return
		}
		handle := candidate.Handle()
		if handle == 0 {
			return
		}
		if _, duplicate := seen[handle]; duplicate {
			return
		}
		seen[handle] = struct{}{}
		observations = append(observations, candidate)
	})
	sort.Slice(observations, func(i, j int) bool { return keys.Less(observations[i], observations[j]) })
	seal := &equalityQuotientSeal{
		keys:       keys,
		congruence: newPathCongruence(keys, l),
		byHash:     make(map[uint64][]*equalityQuotientClass),
	}
	for _, observed := range observations {
		normal, ok := seal.congruence.normal(observed)
		if !ok {
			return EqualityQuotient{}, false
		}
		hash := pathCongruenceNormalHash(normal)
		var class *equalityQuotientClass
		for _, candidate := range seal.byHash[hash] {
			if pathCongruenceNormalsEqual(candidate.normal, normal) {
				class = candidate
				break
			}
		}
		if class == nil {
			class = &equalityQuotientClass{normal: normal}
			seal.byHash[hash] = append(seal.byHash[hash], class)
		}
		class.members = append(class.members, observed)
	}
	return EqualityQuotient{seal: seal}, true
}

func pathCongruenceNormalHash(normal pathCongruenceNormal) uint64 {
	hash := internal.FnvString("path-equality-class")
	hash = internal.MixHash(hash, uint64(normal.class+1))
	if normal.class < 0 {
		hash = internal.MixHash(hash, uint64(normal.root.Kind))
		hash = internal.MixHash(hash, normal.root.Sym)
		hash = internal.MixHash(hash, uint64(normal.root.Ver))
		hash = internal.MixHash(hash, uint64(normal.root.Root))
	}
	for _, part := range normal.suffix {
		constructor := pathConstructor(normal.kind, part)
		hash = internal.MixHash(hash, uint64(constructor.Kind))
		hash = internal.MixHash(hash, internal.FnvString(constructor.Name))
		hash = internal.MixHash(hash, uint64(constructor.Index))
	}
	return hash
}

func (q EqualityQuotient) Valid() bool {
	return q.seal != nil && q.seal.keys != nil && q.seal.keys.Valid() && q.seal.congruence != nil
}

// Class returns candidate's finite observed congruence class. A structurally
// valid candidate with no observed equivalent has no class and needs no
// participant rewrite.
func (q EqualityQuotient) Class(candidate keyspace.Key) (EqualityClass, bool) {
	if !q.Valid() || candidate.Kind == keyspace.KindInvalid {
		return EqualityClass{}, false
	}
	normal, ok := q.seal.congruence.normal(candidate)
	if !ok {
		return EqualityClass{}, false
	}
	for _, class := range q.seal.byHash[pathCongruenceNormalHash(normal)] {
		if pathCongruenceNormalsEqual(class.normal, normal) {
			return EqualityClass{seal: q.seal, class: class}, true
		}
	}
	return EqualityClass{}, false
}

// RangeClass visits the finite observed members in deterministic keyspace
// order. It performs no allocation and rejects classes from another quotient.
func (q EqualityQuotient) RangeClass(class EqualityClass, visit func(keyspace.Key)) bool {
	if !q.Valid() || class.seal != q.seal || class.class == nil || visit == nil {
		return false
	}
	for _, member := range class.class.members {
		visit(member)
	}
	return true
}
