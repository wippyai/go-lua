package readmodel

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type splitBirthTableBirth struct {
	point cfg.Point
	span  SourceSpan
}

type splitBirthFieldWrite struct {
	point            cfg.Point
	receiver         pathdom.Path
	field            string
	label            string
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

// ForEachSplitBirthDiscriminant visits locally born table records whose tag
// field and payload fields are assigned at separate program points before the
// tag field is used structurally as a discriminant.
func (r Reader) ForEachSplitBirthDiscriminant(visit func(SplitBirthDiscriminant) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
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
		item := SplitBirthDiscriminant{
			Point:                tag.point,
			ReceiverLabel:        r.displayPathCanonical(tag.receiver),
			TagLabel:             tag.label,
			TagValue:             tag.literalString,
			BirthPoint:           birth.point,
			BirthSpan:            birth.span,
			TagWriteSpan:         tag.span,
			PayloadWrites:        payloads,
			DiscriminantUsePoint: use.point,
			DiscriminantUseSpan:  use.span,
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) splitBirthObjectLiteralBirths() map[pathdom.PathKey][]splitBirthTableBirth {
	out := make(map[pathdom.PathKey][]splitBirthTableBirth)
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LoweredLocalAssignment(point); ok {
			root := fact.TargetPathRef()
			if root.Symbol != 0 && len(root.Segments) == 0 {
				r.addSplitBirthObjectLiteral(out, point, root, fact.Source())
			}
		}
		if write, ok := r.result.LoweredAssignmentWrite(point); ok &&
			write.Target.Symbol != 0 &&
			len(write.Target.Segments) == 0 &&
			r.splitBirthTargetIsLocal(write.Target) {
			r.addSplitBirthObjectLiteral(out, point, write.Target, write.Source)
		}
	}
	return out
}

func (r Reader) splitBirthTargetIsLocal(p pathdom.Path) bool {
	if p.Symbol == 0 || r.result == nil {
		return false
	}
	kind, ok := r.result.SymbolKind(p.Symbol)
	return ok && kind == symbol.Local
}

func (r Reader) addSplitBirthObjectLiteral(out map[pathdom.PathKey][]splitBirthTableBirth, point cfg.Point, root pathdom.Path, source factflow.ValueSource) {
	literal, ok := r.result.ObjectLiteralViewForSource(source)
	if !ok {
		return
	}
	r.addSplitBirthObjectLiteralView(out, point, root, literal)
}

func (r Reader) addSplitBirthObjectLiteralView(out map[pathdom.PathKey][]splitBirthTableBirth, point cfg.Point, root pathdom.Path, literal factflow.ObjectLiteralView) {
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
		nested, ok := r.result.ObjectLiteralViewForSource(entry.Source())
		if !ok {
			return true
		}
		nestedRoot := root.AppendSegments(entry.SuffixSegmentsView())
		r.addSplitBirthObjectLiteralView(out, point, nestedRoot, nested)
		return true
	})
}

func (r Reader) splitBirthStaticFieldWrites() []splitBirthFieldWrite {
	var out []splitBirthFieldWrite
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		write, ok := r.result.LoweredAssignmentWrite(point)
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
			label:            r.splitBirthFieldLabel(target.Container, target.Key),
			span:             sourceSpanFromBody(target.Span),
			literalString:    lit,
			hasLiteralString: litOK,
		})
	}
	return out
}

func (r Reader) splitBirthLiteralStringWrite(point cfg.Point, source factflow.ValueSource) (string, bool) {
	if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralString {
		return source.String, true
	}
	return r.result.StaticStringValueSourceAtBoundary(point, source)
}

func (r Reader) splitBirthDiscriminantUses() map[splitBirthFieldKey][]splitBirthDiscriminantUse {
	out := make(map[splitBirthFieldKey][]splitBirthDiscriminantUse)
	seen := make(map[string]struct{})
	r.result.ForEachUserVisibleBranchConditionOccurrence(func(occ body.BranchConditionOccurrence) bool {
		for _, use := range r.splitBirthDiscriminantUsesForCheck(occ.Point, occ.Check, sourceSpanFromBody(occ.ConditionSpan)) {
			key, ok := splitBirthFieldKeyFor(use.path, use.field)
			if !ok {
				continue
			}
			seenKey := fmt.Sprintf("%d:%s:%s", use.point, key.receiver, key.field)
			if _, exists := seen[seenKey]; exists {
				continue
			}
			seen[seenKey] = struct{}{}
			out[key] = append(out[key], use)
		}
		return true
	})
	return out
}

func (r Reader) splitBirthDiscriminantUsesForCheck(point cfg.Point, check branchcond.Check, span SourceSpan) []splitBirthDiscriminantUse {
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
	if candidate, ok := r.discriminatedUnionCandidateForCheck(point, check); ok {
		if receiver, field, ok := splitBirthStaticStringField(candidate.target); ok {
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

func (r Reader) splitBirthForReceiverBefore(births map[pathdom.PathKey][]splitBirthTableBirth, receiver pathdom.Path, point cfg.Point) (splitBirthTableBirth, bool) {
	candidates := births[splitBirthPathKey(receiver)]
	for i := len(candidates) - 1; i >= 0; i-- {
		birth := candidates[i]
		if (birth.point == point || r.result.PointCanReach(birth.point, point)) &&
			!r.splitBirthReceiverReassignedBetween(birth.point, point, receiver) {
			return birth, true
		}
	}
	return splitBirthTableBirth{}, false
}

func (r Reader) splitBirthReceiverReassignedBetween(from, to cfg.Point, receiver pathdom.Path) bool {
	if from == 0 || to == 0 || receiver.IsEmpty() {
		return false
	}
	for _, candidate := range r.result.Graph().RPO() {
		if candidate == from || candidate == to {
			continue
		}
		if !r.result.PointCanReach(from, candidate) || !r.result.PointCanReach(candidate, to) {
			continue
		}
		write, ok := r.result.LoweredAssignmentWrite(candidate)
		if !ok || write.Target.IsEmpty() {
			continue
		}
		if len(write.Target.Segments) <= len(receiver.Segments) && receiver.HasPrefix(write.Target) {
			return true
		}
	}
	return false
}

func (r Reader) splitBirthUseAfter(uses map[splitBirthFieldKey][]splitBirthDiscriminantUse, receiver pathdom.Path, field string, point cfg.Point) (splitBirthDiscriminantUse, bool) {
	key, ok := splitBirthFieldKeyFor(receiver, field)
	if !ok {
		return splitBirthDiscriminantUse{}, false
	}
	for _, use := range uses[key] {
		if use.point != point && r.result.PointCanReach(point, use.point) {
			return use, true
		}
	}
	return splitBirthDiscriminantUse{}, false
}

func (r Reader) splitBirthPayloadWrites(tag splitBirthFieldWrite, writes []splitBirthFieldWrite, birthPoint cfg.Point) []SplitBirthPayloadWrite {
	var out []SplitBirthPayloadWrite
	for _, write := range writes {
		if write.point == tag.point || write.field == tag.field {
			continue
		}
		if birthPoint != 0 && !r.result.PointCanReach(birthPoint, write.point) {
			continue
		}
		out = append(out, SplitBirthPayloadWrite{
			Point: write.point,
			Label: write.label,
			Span:  write.span,
		})
	}
	return out
}

func (r Reader) splitBirthFieldLabel(receiver pathdom.Path, field string) string {
	base := r.displayPathCanonical(receiver)
	if base == "" {
		return field
	}
	return base + "." + field
}
