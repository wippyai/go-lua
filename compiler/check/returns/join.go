package returns

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// JoinInterprocTypes centralizes the join policy used by interproc fact channels.
// Keep all interproc call sites routed through this helper to avoid scattered
// merge behavior and accidental divergence across channels.
func JoinInterprocTypes(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return typ.JoinPreferNonSoft(a, b)
}

// JoinReturnVectorsPreferNonSoft joins two return vectors element-wise, preferring non-soft types.
func JoinReturnVectorsPreferNonSoft(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		out[i] = typ.JoinReturnSlot(ai, bi)
	}
	return out
}

// ReturnTypesEqual checks if two return vectors are structurally equal.
func ReturnTypesEqual(a, b []typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typ.TypeEquals(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ReturnTypesAllNil reports whether all slots are explicit nil.
func ReturnTypesAllNil(rets []typ.Type) bool {
	if len(rets) == 0 {
		return false
	}
	for _, t := range rets {
		if t == nil || t.Kind() != kind.Nil {
			return false
		}
	}
	return true
}

// ReturnTypesRefine reports whether a refines b (element-wise subtype).
func ReturnTypesRefine(a, b []typ.Type) bool {
	if len(a) == 0 {
		return false
	}
	if len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ai := a[i]
		bi := b[i]
		if ai == nil || bi == nil {
			if ai == nil && bi == nil {
				continue
			}
			return false
		}
		if !subtype.IsSubtype(ai, bi) {
			return false
		}
	}
	return true
}

// ReturnTypesExtendRecord reports whether a extends b by adding record fields.
// This treats record field supersets as refinements for return summaries.
func ReturnTypesExtendRecord(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ar, ok := a[i].(*typ.Record)
		if !ok {
			return false
		}
		switch br := b[i].(type) {
		case *typ.Record:
			if !recordSuperset(ar, br) {
				return false
			}
		case *typ.Union:
			if !recordSupersetUnion(ar, br) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ReturnTypesElideOptional reports whether a refines b by removing nil/optional parts.
func ReturnTypesElideOptional(a, b []typ.Type) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typeElidesOptional(a[i], b[i]) {
			return false
		}
	}
	return true
}

// SelectPreferredReturnVector picks a canonical winner when one return vector
// is strictly preferable to the other without requiring a join.
//
// Preference order:
//  1. subtype refinement (with nil-only regression protection)
//  2. record extension
//  3. optional elision
//
// The nil-only guard prevents a refined-but-empty-looking update from
// regressing an already informative summary to just nil.
func SelectPreferredReturnVector(a, b []typ.Type) ([]typ.Type, bool) {
	if ReturnTypesRefine(a, b) {
		if ReturnTypesAllNil(a) && !ReturnTypesAllNil(b) {
			return b, true
		}
		return a, true
	}
	if ReturnTypesRefine(b, a) {
		if ReturnTypesAllNil(b) && !ReturnTypesAllNil(a) {
			return a, true
		}
		return b, true
	}
	if ReturnTypesExtendRecord(a, b) || ReturnTypesElideOptional(a, b) {
		return a, true
	}
	if ReturnTypesExtendRecord(b, a) || ReturnTypesElideOptional(b, a) {
		return b, true
	}
	return nil, false
}

// TypeExtendsRecord reports whether type a extends type b by adding record fields.
// This treats record field supersets as refinements when b is a record or union of records.
func TypeExtendsRecord(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	ar, ok := a.(*typ.Record)
	if !ok {
		return false
	}
	switch br := b.(type) {
	case *typ.Record:
		return recordSuperset(ar, br)
	case *typ.Union:
		return recordSupersetUnion(ar, br)
	default:
		return false
	}
}

func typeElidesOptional(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	nonNil := narrow.RemoveNil(b)
	if nonNil == nil || typ.TypeEquals(nonNil, b) {
		return false
	}
	return subtype.IsSubtype(a, nonNil)
}

func recordSuperset(newRec, oldRec *typ.Record) bool {
	if newRec == nil || oldRec == nil {
		return false
	}
	if oldRec.Metatable != nil {
		if newRec.Metatable == nil || !subtype.IsSubtype(newRec.Metatable, oldRec.Metatable) {
			return false
		}
	}
	if oldRec.HasMapComponent() {
		if !newRec.HasMapComponent() {
			return false
		}
		if !subtype.IsSubtype(newRec.MapKey, oldRec.MapKey) || !subtype.IsSubtype(newRec.MapValue, oldRec.MapValue) {
			return false
		}
	}
	oldFields := make(map[string]typ.Field, len(oldRec.Fields))
	for _, f := range oldRec.Fields {
		oldFields[f.Name] = f
	}
	for _, nf := range newRec.Fields {
		if of, ok := oldFields[nf.Name]; ok {
			if of.Optional && !nf.Optional {
				// ok: stronger requirement
			} else if !of.Optional && nf.Optional {
				return false
			}
			if of.Readonly && !nf.Readonly {
				return false
			}
			if of.Type != nil {
				if nf.Type == nil || !subtype.IsSubtype(nf.Type, of.Type) {
					return false
				}
			}
			delete(oldFields, nf.Name)
		}
	}
	return len(oldFields) == 0
}

func recordSupersetUnion(newRec *typ.Record, oldUnion *typ.Union) bool {
	if newRec == nil || oldUnion == nil {
		return false
	}
	if len(oldUnion.Members) == 0 {
		return false
	}
	for _, member := range oldUnion.Members {
		oldRec, ok := member.(*typ.Record)
		if !ok {
			return false
		}
		if !recordSuperset(newRec, oldRec) {
			return false
		}
	}
	return true
}

// NormalizeReturnVector replaces nil slots with explicit nil types.
func NormalizeReturnVector(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return nil
	}
	out := make([]typ.Type, len(rets))
	for i, t := range rets {
		if t == nil {
			out[i] = typ.Nil
		} else {
			out[i] = t
		}
	}
	return out
}
