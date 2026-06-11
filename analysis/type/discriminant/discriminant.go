package discriminant

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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

// NarrowByPathLiteral keeps the variants of t whose static member path admits
// lit. The returned bool reports whether a strict narrowing was possible.
func NarrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return nil, false
	}
	narrowed, ok := narrowByPathLiteral(t, suffix, lit, 0)
	if !ok || narrowed == nil || typ.SameNodeOrAcyclicEqual(narrowed, t) {
		return narrowed, false
	}
	return narrowed, true
}

func narrowByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return narrowByPathLiteral(v.UnaliasedTarget(), suffix, lit, depth+1)
	case *typ.Optional:
		return narrowByPathLiteral(v.Inner, suffix, lit, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if pathAdmitsLiteral(member, suffix, lit, depth+1) {
				out = append(out, member)
			}
		}
		if len(out) == 0 || len(out) == len(v.Members) {
			return t, false
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathAdmitsLiteral(t, suffix, lit, depth+1) {
			return t, false
		}
		return typ.Never, true
	}
}

func pathAdmitsLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, depth int) bool {
	field, ok := fieldAtPath(t, suffix, depth+1)
	return ok && subtype.IsSubtype(lit, field)
}

func fieldAtPath(t typ.Type, suffix []segment.Segment, depth int) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return fieldAtPath(v.UnaliasedTarget(), suffix, depth+1)
	case *typ.Optional:
		return fieldAtPath(v.Inner, suffix, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			field, ok := fieldAtPath(member, suffix, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, field)
		}
		return normalize.UnionForEvidence(out...), true
	case *typ.Record:
		field, ok := directRecordMember(v, suffix[0])
		if !ok {
			return nil, false
		}
		if len(suffix) == 1 {
			return field, true
		}
		return fieldAtPath(field, suffix[1:], depth+1)
	default:
		return nil, false
	}
}

func directRecordMember(r *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := r.GetField(seg.Name); field != nil {
			return field.Type, true
		}
		if member := r.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, true
		}
	case segment.SegmentIndexString:
		if member := r.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, true
		}
		if field := r.GetField(seg.Name); field != nil {
			return field.Type, true
		}
	case segment.SegmentIndexInt:
		if member := r.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return member.Type, true
		}
	}
	return nil, false
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
	differing, equal := d.sharedRequiredLiteralAxes(a, b)
	if differing == 1 && equal == 0 {
		return true
	}
	if differing == 0 {
		return false
	}
	return !d.literalErasedResidualsCleanlyMergeable(a, b)
}

func (d *Detector) sharedRequiredLiteralAxes(a, b *typ.Record) (differing, equal int) {
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
	t = unwrap.Annotated(t)
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
	t = unwrap.Annotated(t)
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
			if lit, ok := literal.ExtractAliasOnly(field.Type); ok {
				tags[field.Name] = typ.EqualityHash(lit)
				continue
			}
			addPrefixedTags(tags, field.Name, d.RequiredTags(field.Type))
		}
		for _, member := range v.StaticMembers {
			if member.Optional {
				continue
			}
			path := staticMemberPath(member)
			if lit, ok := literal.ExtractAliasOnly(member.Type); ok {
				tags[path] = typ.EqualityHash(lit)
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

func requiredNonLiteralPayloadMissingFrom(src, dst *typ.Record) bool {
	for _, field := range src.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literal.ExtractAliasOnly(field.Type); ok {
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
		if _, ok := literal.ExtractAliasOnly(member.Type); ok {
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
