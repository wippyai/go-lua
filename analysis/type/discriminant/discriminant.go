package discriminant

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Detector owns required-literal tag extraction and conflict detection for
// record variants. It is reusable so callers that join many related records can
// share the tag cache without exposing their own state.
type Detector struct {
	tags   map[typ.Type]map[string]uint64
	active map[typ.Type]bool
}

func NewDetector() *Detector {
	return &Detector{}
}

func RequiredTags(t typ.Type) map[string]uint64 {
	return NewDetector().RequiredTags(t)
}

func RecordsConflict(a, b *typ.Record) bool {
	return NewDetector().RecordsConflict(a, b)
}

func ClosedRecordSetConflicts(records []*typ.Record) bool {
	return NewDetector().ClosedRecordSetConflicts(records)
}

// ClosedRecordSetConflicts reports whether any pair in a closed record set is
// separated by a required literal discriminant.
func (d *Detector) ClosedRecordSetConflicts(records []*typ.Record) bool {
	if d == nil {
		d = NewDetector()
	}
	hasTags := false
	for _, rec := range records {
		if d.HasRequiredTag(rec) {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return false
	}

	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if d.RecordsConflict(records[i], records[j]) {
				return true
			}
		}
	}
	return false
}

// RecordsConflict reports whether two records are discriminated variants kept
// distinct rather than coalesced.
//
// A required literal field shared by both records is either a variant tag or
// incidental literal data. The structural signal of a tag is that it is the
// single literal axis on which the records disagree: exactly one shared required
// literal field has differing values, and no shared required literal field has
// an equal value acting as a constant data key. When the records disagree on
// several literal fields, or share an equal-valued literal alongside the
// difference, the literals are incidental data and the records coalesce.
// Records whose literal-erased residuals do not merge cleanly also stay
// distinct, since merging them would lose structure rather than widen a scalar.
func (d *Detector) RecordsConflict(a, b *typ.Record) bool {
	if d == nil {
		d = NewDetector()
	}
	differing, equal := d.SharedRequiredLiteralAxes(a, b)
	if differing == 1 && equal == 0 {
		return true
	}
	if differing == 0 {
		return false
	}
	return !d.literalErasedResidualsCleanlyMergeable(a, b)
}

// SharedRequiredLiteralAxes counts required literal fields both records require,
// split into differing and equal literal values.
func (d *Detector) SharedRequiredLiteralAxes(a, b *typ.Record) (differing, equal int) {
	if d == nil {
		d = NewDetector()
	}
	left := d.RequiredTags(a)
	right := d.RequiredTags(b)
	for path, leftHash := range left {
		rightHash, ok := right[path]
		if !ok {
			continue
		}
		if leftHash == rightHash {
			equal++
		} else {
			differing++
		}
	}
	return differing, equal
}

func (d *Detector) HasRequiredTag(t typ.Type) bool {
	return len(d.RequiredTags(t)) > 0
}

func (d *Detector) RequiredTags(t typ.Type) map[string]uint64 {
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return nil
	}
	if d == nil {
		d = NewDetector()
	}
	if d.tags != nil {
		if cached, ok := d.tags[t]; ok {
			return copyTags(cached)
		}
	}
	if d.active != nil && d.active[t] {
		return nil
	}
	if d.active == nil {
		d.active = make(map[typ.Type]bool)
	}
	d.active[t] = true
	defer delete(d.active, t)

	tags := d.collectRequiredTags(t)
	if d.tags == nil {
		d.tags = make(map[typ.Type]map[string]uint64)
	}
	d.tags[t] = copyTags(tags)
	return tags
}

func (d *Detector) collectRequiredTags(t typ.Type) map[string]uint64 {
	t = typ.UnwrapAnnotated(t)
	switch v := t.(type) {
	case *typ.Alias:
		return d.RequiredTags(v.Target)
	case *typ.Recursive:
		return d.RequiredTags(v.Body)
	case *typ.Record:
		tags := make(map[string]uint64)
		for _, field := range v.Fields {
			if field.Optional {
				continue
			}
			if lit, ok := literalType(field.Type); ok {
				tags[field.Name] = lit.Hash()
				continue
			}
			addPrefixedTags(tags, field.Name, d.RequiredTags(field.Type))
		}
		for _, member := range v.StaticMembers {
			if member.Optional {
				continue
			}
			path := staticMemberPath(member)
			if lit, ok := literalType(member.Type); ok {
				tags[path] = lit.Hash()
				continue
			}
			addPrefixedTags(tags, path, d.RequiredTags(member.Type))
		}
		return tags
	case *typ.Union:
		return d.commonUnionTags(v)
	}
	return nil
}

func (d *Detector) commonUnionTags(u *typ.Union) map[string]uint64 {
	if u == nil || len(u.Members) == 0 {
		return nil
	}
	var common map[string]uint64
	for i, member := range u.Members {
		memberTags := d.RequiredTags(member)
		if i == 0 {
			common = copyTags(memberTags)
			continue
		}
		for path, hash := range common {
			if memberHash, ok := memberTags[path]; !ok || memberHash != hash {
				delete(common, path)
			}
		}
	}
	return common
}

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
		if _, ok := literalType(field.Type); ok {
			continue
		}
		other := b.GetField(field.Name)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literalType(other.Type); ok {
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
		if _, ok := literalType(member.Type); ok {
			continue
		}
		other := b.GetStaticMember(member.Kind, member.Name, member.Index)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literalType(other.Type); ok {
			continue
		}
		if !d.mergeKeepsPreciseFieldType(member.Type, other.Type) {
			return false
		}
	}
	return true
}

func requiredNonLiteralPayloadMissingFrom(src, dst *typ.Record) bool {
	for _, field := range src.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literalType(field.Type); ok {
			continue
		}
		if dst.GetField(field.Name) == nil {
			return true
		}
	}
	for _, member := range src.StaticMembers {
		if member.Optional {
			continue
		}
		if _, ok := literalType(member.Type); ok {
			continue
		}
		if dst.GetStaticMember(member.Kind, member.Name, member.Index) == nil {
			return true
		}
	}
	return false
}

func (d *Detector) mergeKeepsPreciseFieldType(a, b typ.Type) bool {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if fieldMergeKind(a) != fieldMergeKind(b) {
		return false
	}
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar != nil && br != nil {
		return d.literalErasedResidualsCleanlyMergeable(ar, br) && !d.RecordsConflict(ar, br)
	}
	return true
}

func fieldMergeKind(t typ.Type) kind.Kind {
	t = typ.UnwrapAnnotated(t)
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	if lit, ok := t.(*typ.Literal); ok {
		return lit.Base
	}
	if t == nil {
		return kind.Nil
	}
	return t.Kind()
}

func unaliasRecord(t typ.Type) *typ.Record {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	rec, _ := t.(*typ.Record)
	return rec
}

func staticMemberPath(member typ.StaticMember) string {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return "[" + strconv.Quote(member.Name) + "]"
	case typ.StaticMemberIntIndex:
		return "[" + strconv.FormatInt(member.Index, 10) + "]"
	default:
		return "[]"
	}
}

func addPrefixedTags(dst map[string]uint64, prefix string, src map[string]uint64) {
	for path, hash := range src {
		dst[joinPath(prefix, path)] = hash
	}
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func copyTags(src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]uint64, len(src))
	for path, hash := range src {
		dst[path] = hash
	}
	return dst
}

func literalType(t typ.Type) (*typ.Literal, bool) {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	lit, ok := t.(*typ.Literal)
	return lit, ok
}
