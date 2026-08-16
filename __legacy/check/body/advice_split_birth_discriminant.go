package body

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// SplitBirthPayloadWriteOccurrence is one non-tag field write to the same
// split-born receiver.
type SplitBirthPayloadWriteOccurrence struct {
	Point cfg.Point
	Field string
	Span  SourceSpan
}

// SplitBirthDiscriminantOccurrence is one locally born table whose string tag
// field is assigned separately from payload fields and later used as a
// discriminant.
type SplitBirthDiscriminantOccurrence struct {
	Point                cfg.Point
	Receiver             pathdom.Path
	TagField             string
	TagValue             string
	BirthPoint           cfg.Point
	BirthSpan            SourceSpan
	TagWriteSpan         SourceSpan
	PayloadWrites        []SplitBirthPayloadWriteOccurrence
	DiscriminantUsePoint cfg.Point
	DiscriminantUseSpan  SourceSpan
}

type splitBirthTableBirth struct {
	point cfg.Point
	span  SourceSpan
}

type splitBirthFieldWrite struct {
	point            cfg.Point
	receiver         pathdom.Path
	field            string
	span             SourceSpan
	literalString    string
	hasLiteralString bool
}

type splitBirthDiscriminantUse struct {
	point cfg.Point
	path  pathdom.Path
	field string
	span  SourceSpan
}

// ForEachSplitBirthDiscriminantOccurrence visits locally born table records
// whose tag field and payload fields are assigned at separate program points
// before the tag field is used structurally as a discriminant.
func (r *Result) ForEachSplitBirthDiscriminantOccurrence(visit func(SplitBirthDiscriminantOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	births := r.splitBirthObjectLiteralBirths()
	if len(births) == 0 {
		return false
	}
	uses := r.splitBirthDiscriminantUses()
	if len(uses) == 0 {
		return false
	}
	writes := r.splitBirthStaticFieldWrites()
	if len(writes) == 0 {
		return false
	}
	writesByReceiver := splitBirthWritesByReceiver(writes)
	visited := false
	for _, tag := range writes {
		if !tag.hasLiteralString {
			continue
		}
		birth, ok := r.splitBirthForReceiverBefore(births, tag.receiver, tag.point)
		if !ok {
			continue
		}
		use, ok := r.splitBirthUseAfter(uses, tag.receiver, tag.field, tag.point)
		if !ok {
			continue
		}
		payloads := r.splitBirthPayloadWrites(tag, writesByReceiver[splitBirthPathKey(tag.receiver)], birth.point)
		if len(payloads) == 0 {
			continue
		}
		visited = true
		if !visit(SplitBirthDiscriminantOccurrence{
			Point:                tag.point,
			Receiver:             tag.receiver,
			TagField:             tag.field,
			TagValue:             tag.literalString,
			BirthPoint:           birth.point,
			BirthSpan:            birth.span,
			TagWriteSpan:         tag.span,
			PayloadWrites:        payloads,
			DiscriminantUsePoint: use.point,
			DiscriminantUseSpan:  use.span,
		}) {
			return true
		}
	}
	return visited
}

func (r *Result) splitBirthObjectLiteralBirths() map[pathdom.PathKey][]splitBirthTableBirth {
	out := make(map[pathdom.PathKey][]splitBirthTableBirth)
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.LoweredLocalAssignment(point); ok {
			root := fact.TargetPathRef()
			if root.Symbol != 0 && len(root.Segments) == 0 {
				r.addSplitBirthObjectLiteral(out, point, root, fact.Source())
			}
		}
		if write, ok := r.LoweredAssignmentWrite(point); ok &&
			write.Target.Symbol != 0 &&
			len(write.Target.Segments) == 0 &&
			r.splitBirthTargetIsLocal(write.Target) {
			r.addSplitBirthObjectLiteral(out, point, write.Target, write.Source)
		}
	}
	return out
}

func (r *Result) splitBirthTargetIsLocal(p pathdom.Path) bool {
	if p.Symbol == 0 || r == nil {
		return false
	}
	kind, ok := r.SymbolKind(p.Symbol)
	return ok && kind == symbol.Local
}

func (r *Result) addSplitBirthObjectLiteral(out map[pathdom.PathKey][]splitBirthTableBirth, point cfg.Point, root pathdom.Path, source factflow.ValueSource) {
	literal, ok := r.ObjectLiteralViewForSource(source)
	if !ok {
		return
	}
	r.addSplitBirthObjectLiteralView(out, point, root, literal)
}

func (r *Result) addSplitBirthObjectLiteralView(out map[pathdom.PathKey][]splitBirthTableBirth, point cfg.Point, root pathdom.Path, literal factflow.ObjectLiteralView) {
	key := splitBirthPathKey(root)
	if key == "" {
		return
	}
	var span SourceSpan
	if raw, ok := literal.Span(); ok {
		span = sourceSpanFromFactflow(raw)
	}
	out[key] = append(out[key], splitBirthTableBirth{
		point: point,
		span:  span,
	})
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() == 0 {
			return true
		}
		nested, ok := r.ObjectLiteralViewForSource(entry.Source())
		if !ok {
			return true
		}
		nestedRoot := root.AppendSegments(entry.SuffixSegmentsView())
		r.addSplitBirthObjectLiteralView(out, point, nestedRoot, nested)
		return true
	})
}

func (r *Result) splitBirthStaticFieldWrites() []splitBirthFieldWrite {
	var out []splitBirthFieldWrite
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		write, ok := r.LoweredAssignmentWrite(point)
		if !ok {
			continue
		}
		target, ok := write.StaticStringTarget()
		if !ok || target.Container.IsEmpty() || target.Key == "" {
			continue
		}
		lit, litOK := r.splitBirthLiteralStringWrite(point, write.Source)
		out = append(out, splitBirthFieldWrite{
			point:            point,
			receiver:         target.Container.Clone(),
			field:            target.Key,
			span:             target.Span,
			literalString:    lit,
			hasLiteralString: litOK,
		})
	}
	return out
}

func (r *Result) splitBirthLiteralStringWrite(point cfg.Point, source factflow.ValueSource) (string, bool) {
	if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralString {
		return source.String, true
	}
	return r.StaticStringValueSourceAtBoundary(point, source)
}

func (r *Result) splitBirthDiscriminantUses() map[splitBirthFieldKey][]splitBirthDiscriminantUse {
	out := make(map[splitBirthFieldKey][]splitBirthDiscriminantUse)
	seen := make(map[string]struct{})
	r.ForEachUserVisibleBranchConditionOccurrence(func(occ BranchConditionOccurrence) bool {
		for _, use := range r.splitBirthDiscriminantUsesForCheck(occ.Point, occ.Check, occ.ConditionSpan) {
			key, ok := splitBirthFieldKeyFor(use.path, use.field)
			if !ok {
				continue
			}
			if markSeen(seen, fmt.Sprintf("%d:%s:%s", use.point, key.receiver, key.field)) {
				continue
			}
			out[key] = append(out[key], use)
		}
		return true
	})
	return out
}

func (r *Result) splitBirthDiscriminantUsesForCheck(point cfg.Point, check branchcond.Check, span SourceSpan) []splitBirthDiscriminantUse {
	var out []splitBirthDiscriminantUse
	if check.Kind == branchcond.CheckLiteralEqual {
		if receiver, field, ok := splitBirthStaticStringField(check.Path); ok {
			out = append(out, splitBirthDiscriminantUse{
				point: point,
				path:  receiver,
				field: field,
				span:  span,
			})
		}
	}
	if target, ok := r.splitBirthDiscriminatedUnionTargetForCheck(point, check); ok {
		if receiver, field, ok := splitBirthStaticStringField(target); ok {
			out = append(out, splitBirthDiscriminantUse{
				point: point,
				path:  receiver,
				field: field,
				span:  span,
			})
		}
	}
	return out
}

func (r *Result) splitBirthDiscriminatedUnionTargetForCheck(point cfg.Point, check branchcond.Check) (pathdom.Path, bool) {
	lit, negate, ok := splitBirthDiscriminatedUnionCheckLiteral(check)
	if !ok {
		return pathdom.Path{}, false
	}
	for _, anchor := range r.splitBirthDiscriminatedUnionAnchors(point, check.Path) {
		family, _, ok := splitBirthDiscriminatedUnionOriginByCheck(anchor.anchorType, anchor.suffix, lit, negate)
		if !ok {
			continue
		}
		caseFamily, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || caseFamily != family || len(cases) < 2 {
			continue
		}
		return check.Path, true
	}
	return pathdom.Path{}, false
}

type splitBirthDiscriminatedUnionAnchor struct {
	anchorType typ.Type
	suffix     []segment.Segment
}

func (r *Result) splitBirthDiscriminatedUnionAnchors(point cfg.Point, target pathdom.Path) []splitBirthDiscriminatedUnionAnchor {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return nil
	}
	root := target.RootOnly()
	rootType, ok := r.DeclaredOrVariantOriginPathTypeAt(point, root)
	if !ok {
		return nil
	}
	segments := target.Segments
	out := make([]splitBirthDiscriminatedUnionAnchor, 0, len(segments))
	for prefixLen := 0; prefixLen < len(segments); prefixLen++ {
		prefix := segments[:prefixLen]
		suffix := segments[prefixLen:]
		anchorType := rootType
		if len(prefix) > 0 {
			var fieldOK bool
			anchorType, fieldOK = variant.FieldAtPath(rootType, prefix)
			if !fieldOK {
				continue
			}
		}
		out = append(out, splitBirthDiscriminatedUnionAnchor{
			anchorType: anchorType,
			suffix:     append([]segment.Segment(nil), suffix...),
		})
	}
	return out
}

func splitBirthDiscriminatedUnionCheckLiteral(check branchcond.Check) (typ.Type, bool, bool) {
	switch check.Kind {
	case branchcond.CheckLiteralEqual:
		lit, ok := check.LiteralValue()
		return lit, false, ok
	case branchcond.CheckLiteralNot:
		lit, ok := check.LiteralValue()
		return lit, true, ok
	case branchcond.CheckTruthy:
		return typ.True, false, true
	case branchcond.CheckFalsy:
		return typ.True, true, true
	default:
		return nil, false, false
	}
}

func splitBirthDiscriminatedUnionOriginByCheck(anchorType typ.Type, rest []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	if negate {
		return variant.OriginByPathLiteralNot(anchorType, rest, lit)
	}
	return variant.OriginByPathLiteral(anchorType, rest, lit)
}

type splitBirthFieldKey struct {
	receiver pathdom.PathKey
	field    string
}

func splitBirthFieldKeyFor(receiver pathdom.Path, field string) (splitBirthFieldKey, bool) {
	key := splitBirthPathKey(receiver)
	if key == "" || field == "" {
		return splitBirthFieldKey{}, false
	}
	return splitBirthFieldKey{receiver: key, field: field}, true
}

func splitBirthStaticStringField(p pathdom.Path) (pathdom.Path, string, bool) {
	if p.IsEmpty() || len(p.Segments) == 0 {
		return pathdom.Path{}, "", false
	}
	seg, ok := p.LastSegment()
	if !ok {
		return pathdom.Path{}, "", false
	}
	field, ok := splitBirthSegmentStringKey(seg)
	if !ok {
		return pathdom.Path{}, "", false
	}
	return p.Parent(), field, true
}

func splitBirthSegmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func splitBirthPathKey(p pathdom.Path) pathdom.PathKey {
	p = p.Clone()
	p.Version = 0
	return p.Key()
}

func splitBirthWritesByReceiver(writes []splitBirthFieldWrite) map[pathdom.PathKey][]splitBirthFieldWrite {
	out := make(map[pathdom.PathKey][]splitBirthFieldWrite)
	for _, write := range writes {
		key := splitBirthPathKey(write.receiver)
		if key == "" {
			continue
		}
		out[key] = append(out[key], write)
	}
	return out
}

func (r *Result) splitBirthForReceiverBefore(births map[pathdom.PathKey][]splitBirthTableBirth, receiver pathdom.Path, point cfg.Point) (splitBirthTableBirth, bool) {
	candidates := births[splitBirthPathKey(receiver)]
	for i := len(candidates) - 1; i >= 0; i-- {
		birth := candidates[i]
		if (birth.point == point || r.PointCanReach(birth.point, point)) &&
			!r.splitBirthReceiverReassignedBetween(birth.point, point, receiver) {
			return birth, true
		}
	}
	return splitBirthTableBirth{}, false
}

func (r *Result) splitBirthReceiverReassignedBetween(from, to cfg.Point, receiver pathdom.Path) bool {
	if from == 0 || to == 0 || receiver.IsEmpty() {
		return false
	}
	for _, candidate := range r.Graph().RPO() {
		if candidate == from || candidate == to {
			continue
		}
		if !r.PointCanReach(from, candidate) || !r.PointCanReach(candidate, to) {
			continue
		}
		write, ok := r.LoweredAssignmentWrite(candidate)
		if !ok || write.Target.IsEmpty() {
			continue
		}
		if len(write.Target.Segments) <= len(receiver.Segments) && receiver.HasPrefix(write.Target) {
			return true
		}
	}
	return false
}

func (r *Result) splitBirthUseAfter(uses map[splitBirthFieldKey][]splitBirthDiscriminantUse, receiver pathdom.Path, field string, point cfg.Point) (splitBirthDiscriminantUse, bool) {
	key, ok := splitBirthFieldKeyFor(receiver, field)
	if !ok {
		return splitBirthDiscriminantUse{}, false
	}
	for _, use := range uses[key] {
		if use.point != point && r.PointCanReach(point, use.point) {
			return use, true
		}
	}
	return splitBirthDiscriminantUse{}, false
}

func (r *Result) splitBirthPayloadWrites(tag splitBirthFieldWrite, writes []splitBirthFieldWrite, birthPoint cfg.Point) []SplitBirthPayloadWriteOccurrence {
	var out []SplitBirthPayloadWriteOccurrence
	for _, write := range writes {
		if write.point == tag.point || write.field == tag.field {
			continue
		}
		if birthPoint != 0 && !r.PointCanReach(birthPoint, write.point) {
			continue
		}
		out = append(out, SplitBirthPayloadWriteOccurrence{
			Point: write.point,
			Field: write.field,
			Span:  write.span,
		})
	}
	return out
}
