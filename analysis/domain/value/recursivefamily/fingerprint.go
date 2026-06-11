package recursivefamily

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// RecursiveFamilyFingerprint folds structural recursive names reachable from t
// into a stable, order-independent hash. Interner-owned family keys are not
// global typ metadata; callers that need keyed family identity must use
// RecursiveFamilyInterner.RecursiveFamilyFingerprint.
//
// The fingerprint is the discriminator a structural product-family relation
// lacks when recursive-containing terminals collapse to one family placeholder.
// Combining a relation-level same-family check with an equal fingerprint keeps
// equivalent unfoldings shared while keeping distinct recursive names or
// interner-owned families apart.
func RecursiveFamilyFingerprint(t typ.Type) uint64 {
	fp, _ := RecursiveFamilyFingerprintWithin(t, 0)
	return fp
}

// RecursiveFamilyFingerprint folds this interner's recursive-family identities
// reachable from t into a stable, order-independent hash.
func (i *RecursiveFamilyInterner) RecursiveFamilyFingerprint(t typ.Type) uint64 {
	fp, _ := i.RecursiveFamilyFingerprintWithin(t, 0)
	return fp
}

// RecursiveFamilyFingerprintWithin is the bounded form of
// RecursiveFamilyFingerprint. When maxNodes is positive and the scan would exceed
// it, the bool is false. Callers that use the fingerprint only to share
// optimization caches can then decline sharing instead of walking enormous
// recursive product surfaces.
func RecursiveFamilyFingerprintWithin(t typ.Type, maxNodes int) (uint64, bool) {
	if t == nil {
		return 0, true
	}
	scan := recursiveFamilyFingerprintScan{
		seenFamilies: make(map[uint64]bool),
		seenNodes:    make(map[uintptr]bool),
		maxNodes:     maxNodes,
	}
	scan.scan(t)
	return scan.fp, !scan.exceeded
}

// RecursiveFamilyFingerprintWithin is the bounded form of
// RecursiveFamilyFingerprint for this interner's family identities.
func (i *RecursiveFamilyInterner) RecursiveFamilyFingerprintWithin(t typ.Type, maxNodes int) (uint64, bool) {
	if t == nil {
		return 0, true
	}
	scan := recursiveFamilyFingerprintScan{
		interner:     i,
		seenFamilies: make(map[uint64]bool),
		seenNodes:    make(map[uintptr]bool),
		maxNodes:     maxNodes,
	}
	scan.scan(t)
	return scan.fp, !scan.exceeded
}

const (
	recursiveFamilyKeyedSalt uint64 = 0x9e3779b97f4a7c15
	recursiveFamilyNamedSalt uint64 = 0xc2b2ae3d27d4eb4f
)

type recursiveFamilyFingerprintScan struct {
	interner     *RecursiveFamilyInterner
	fp           uint64
	seenFamilies map[uint64]bool
	seenNodes    map[uintptr]bool
	maxNodes     int
	nodes        int
	exceeded     bool
}

func (s *recursiveFamilyFingerprintScan) scan(t typ.Type) {
	if t == nil || s == nil || s.exceeded {
		return
	}
	if s.maxNodes > 0 {
		s.nodes++
		if s.nodes > s.maxNodes {
			s.exceeded = true
			return
		}
	}
	if rec, ok := t.(*typ.Recursive); ok {
		s.add(rec)
		return
	}
	t = unwrap.Annotations(t)
	if rec, ok := t.(*typ.Recursive); ok {
		s.add(rec)
		return
	}
	if ptr := nodeid.StructuralPointer(t); ptr != 0 {
		if s.seenNodes[ptr] {
			return
		}
		s.seenNodes[ptr] = true
	}
	switch v := t.(type) {
	case *typ.Optional:
		s.scan(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			s.scan(member)
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			s.scan(member)
		}
	case *typ.Array:
		s.scan(v.Element)
	case *typ.Map:
		s.scan(v.Key)
		s.scan(v.Value)
	case *typ.ReadonlyMap:
		s.scan(v.Key)
		s.scan(v.Value)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			s.scan(elem)
		}
	case *typ.Function:
		for _, param := range v.Params {
			s.scan(param.Type)
		}
		s.scan(v.Variadic)
		for _, ret := range v.Returns {
			s.scan(ret)
		}
	case *typ.Record:
		for _, field := range v.Fields {
			s.scan(field.Type)
		}
		for _, member := range v.StaticMembers {
			s.scan(member.Type)
		}
		s.scan(v.Metatable)
		if v.HasMapComponent() {
			s.scan(v.MapKey)
			s.scan(v.MapValue)
		}
	case *typ.Alias:
		s.scan(v.Target)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			s.scan(arg)
		}
	case *typ.Interface:
		for _, method := range v.Methods {
			s.scan(method.Type)
		}
	}
}

func (s *recursiveFamilyFingerprintScan) add(rec *typ.Recursive) {
	if rec == nil || s == nil {
		return
	}
	var id uint64
	if s.interner != nil {
		if identityHash, ok := s.interner.FamilyIdentityHash(rec); ok {
			id = identityHash
		}
	}
	if id == 0 {
		id = hash.MixHash(recursiveFamilyNamedSalt, hash.FnvString(rec.Name))
	}
	if s.seenFamilies[id] {
		return
	}
	s.seenFamilies[id] = true
	// XOR folds per-family identities order-independently so the traversal order
	// does not perturb the fingerprint.
	s.fp ^= id
}
