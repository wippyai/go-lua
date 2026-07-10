package body

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ShapeConditionalFieldOccurrence is a static field whose write does not
// dominate a shape-relevant use of its locally born receiver.
type ShapeConditionalFieldOccurrence struct {
	Point cfg.Point
	Name  string
	Span  SourceSpan
}

// ShapePolymorphicOccurrence records a proof-positive failure to construct a
// fixed table shape. It deliberately does not classify maps or dynamic-key
// tables: those are not record-shaped construction intent.
type ShapePolymorphicOccurrence struct {
	Point             cfg.Point
	Receiver          pathdom.Path
	BirthPoint        cfg.Point
	BirthSpan         SourceSpan
	UsePoint          cfg.Point
	UseSpan           SourceSpan
	ConditionalFields []ShapeConditionalFieldOccurrence
	UnionFields       []string
}

type shapePolymorphicBirth struct {
	point  cfg.Point
	root   pathdom.Path
	span   SourceSpan
	fields []string
}

type shapePolymorphicUse struct {
	point cfg.Point
	span  SourceSpan
}

// ForEachShapePolymorphicOccurrence finds locally born record-like tables that
// have static writes on non-dominating paths and are returned or field-read.
// StableShape absence alone is not enough: every emitted item also has a
// concrete field write that can reach the use but does not dominate it.
func (r *Result) ForEachShapePolymorphicOccurrence(visit func(ShapePolymorphicOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	births := r.shapePolymorphicBirths()
	if len(births) == 0 {
		return false
	}
	writes := r.splitBirthStaticFieldWrites()
	visited := false
	for _, birth := range births {
		if r.shapePolymorphicHasDynamicKeyWrite(birth) {
			continue
		}
		for _, use := range r.shapePolymorphicUses(birth.root) {
			if _, stable := r.StableShapeForPathAtBoundary(use.point, birth.root); stable {
				continue
			}
			fields := r.shapePolymorphicConditionalFields(birth, use, writes)
			if len(fields) == 0 {
				continue
			}
			visited = true
			if !visit(ShapePolymorphicOccurrence{Point: use.point, Receiver: birth.root, BirthPoint: birth.point, BirthSpan: birth.span, UsePoint: use.point, UseSpan: use.span, ConditionalFields: fields, UnionFields: shapePolymorphicUnionFields(birth.fields, fields)}) {
				return true
			}
		}
	}
	return visited
}

func (r *Result) shapePolymorphicBirths() []shapePolymorphicBirth {
	var out []shapePolymorphicBirth
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.LoweredLocalAssignment(point)
		if !ok {
			continue
		}
		root := fact.TargetPathRef()
		if root.Symbol == 0 || len(root.Segments) != 0 {
			continue
		}
		kind, local := r.SymbolKind(root.Symbol)
		if !local || kind != symbol.Local {
			continue
		}
		literal, ok := r.ObjectLiteralViewForSource(fact.Source())
		if !ok {
			continue
		}
		var span SourceSpan
		if raw, ok := literal.Span(); ok {
			span = sourceSpanFromFactflow(raw)
		}
		out = append(out, shapePolymorphicBirth{point: point, root: root, span: span, fields: shapePolymorphicLiteralFields(literal)})
	}
	return out
}

func shapePolymorphicLiteralFields(literal factflow.ObjectLiteralView) []string {
	seen := map[string]struct{}{}
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() != 1 {
			return true
		}
		seg, ok := entry.SuffixSegmentAt(0)
		if !ok || (seg.Kind != segment.SegmentField && seg.Kind != segment.SegmentIndexString) || seg.Name == "" {
			return true
		}
		seen[seg.Name] = struct{}{}
		return true
	})
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func shapePolymorphicUnionFields(initial []string, conditional []ShapeConditionalFieldOccurrence) []string {
	seen := map[string]struct{}{}
	for _, name := range initial {
		seen[name] = struct{}{}
	}
	for _, field := range conditional {
		seen[field.Name] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Result) shapePolymorphicUses(root pathdom.Path) []shapePolymorphicUse {
	var out []shapePolymorphicUse
	r.ForEachReturnValueOccurrence(func(occ ReturnValueOccurrence) bool {
		if occ.HasPath && occ.SourcePath.Equal(root) {
			out = append(out, shapePolymorphicUse{point: occ.Point, span: occ.SourceSpan})
		}
		return true
	})
	r.ForEachStaticMemberReadOccurrence(func(occ StaticMemberReadOccurrence) bool {
		if occ.HasReceiverPath && occ.ReceiverPath.Equal(root) {
			out = append(out, shapePolymorphicUse{point: occ.Point, span: occ.Span})
		}
		return true
	})
	return out
}

func (r *Result) shapePolymorphicHasDynamicKeyWrite(birth shapePolymorphicBirth) bool {
	for _, point := range r.Graph().RPO() {
		write, ok := r.LoweredAssignmentWrite(point)
		if !ok || len(write.Target.Segments) <= len(birth.root.Segments) || !write.Target.HasPrefix(birth.root) {
			continue
		}
		target, static := write.StaticStringTarget()
		if !static || !target.Container.Equal(birth.root) {
			return true
		}
	}
	return false
}

func (r *Result) shapePolymorphicConditionalFields(birth shapePolymorphicBirth, use shapePolymorphicUse, writes []splitBirthFieldWrite) []ShapeConditionalFieldOccurrence {
	byName := map[string]ShapeConditionalFieldOccurrence{}
	blocked := map[string]map[cfg.Point]struct{}{}
	for _, write := range writes {
		if !write.receiver.Equal(birth.root) || !r.PointCanReach(birth.point, write.point) || !r.PointCanReach(write.point, use.point) {
			continue
		}
		if _, exists := byName[write.field]; !exists {
			byName[write.field] = ShapeConditionalFieldOccurrence{Point: write.point, Name: write.field, Span: write.span}
		}
		if blocked[write.field] == nil {
			blocked[write.field] = map[cfg.Point]struct{}{}
		}
		blocked[write.field][write.point] = struct{}{}
	}
	out := make([]ShapeConditionalFieldOccurrence, 0, len(byName))
	for name, field := range byName {
		if !r.shapePolymorphicCanBypassWrites(birth.point, use.point, blocked[name]) {
			continue
		}
		out = append(out, field)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Point < out[j].Point
	})
	return out
}

// shapePolymorphicCanBypassWrites proves that a path reaches use without any
// write of one field. This is stronger than non-dominance: if both branches
// assign the same field, every path crosses one of the blocked writes and the
// field is uniform, so no advice is emitted.
func (r *Result) shapePolymorphicCanBypassWrites(from, to cfg.Point, blocked map[cfg.Point]struct{}) bool {
	if r == nil || r.Graph() == nil {
		return false
	}
	queue := []cfg.Point{from}
	seen := map[cfg.Point]struct{}{from: struct{}{}}
	for len(queue) > 0 {
		point := queue[0]
		queue = queue[1:]
		if point == to {
			return true
		}
		for _, next := range r.Graph().Successors(point) {
			if _, skip := blocked[next]; skip {
				continue
			}
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return false
}
