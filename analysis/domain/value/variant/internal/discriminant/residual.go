package discriminant

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// literalErasedResidualsCleanlyMergeable reports whether two records merge into
// a single precise record once their literal-valued fields are erased.
func (d *Detector) literalErasedResidualsCleanlyMergeable(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	if requiredNonLiteralPayloadMissingFrom(a, b) && requiredNonLiteralPayloadMissingFrom(b, a) {
		return false
	}
	for _, field := range a.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(field.Type); ok {
			continue
		}
		other := b.GetField(field.Name)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(other.Type); ok {
			continue
		}
		if !d.mergeKeepsPreciseFieldType(field.Type, other.Type) {
			return false
		}
	}
	for _, member := range a.StaticMembers {
		if member.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(member.Type); ok {
			continue
		}
		other := b.GetStaticMember(member.Kind, member.Name, member.Index)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(other.Type); ok {
			continue
		}
		if !d.mergeKeepsPreciseFieldType(member.Type, other.Type) {
			return false
		}
	}
	return true
}

func (d *Detector) mergeKeepsPreciseFieldType(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if fieldMergeKind(a) != fieldMergeKind(b) {
		return false
	}
	ar := unwrap.RecordWithAliasPolicy(a, unwrap.RecordAliasTarget)
	br := unwrap.RecordWithAliasPolicy(b, unwrap.RecordAliasTarget)
	if ar != nil && br != nil {
		return d.literalErasedResidualsCleanlyMergeable(ar, br) && !d.RecordsConflict(ar, br)
	}
	return true
}

func fieldMergeKind(t typ.Type) kind.Kind {
	t = unwrap.Annotated(t)
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	if lit, ok := literal.ExtractAliasOnly(t); ok {
		return lit.Base
	}
	if t == nil {
		return kind.Nil
	}
	return t.Kind()
}
