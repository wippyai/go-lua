package access

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Location is the normalized structural identity of a point-state access.
//
// It is syntax-free: compiler layers may lower AST lvalues into Location, while
// flow/proof layers consume its path and write-footprint projections.
type Location struct {
	Root     cfg.SymbolID
	RootName string
	Steps    []Step
}

// StepKind identifies the structural kind of a Location step.
type StepKind uint8

const (
	StepStaticMember StepKind = iota + 1
	StepDynamicIndex
)

// Step is one structural access step. Dynamic steps carry evaluated key
// evidence, not AST.
type Step struct {
	Kind   StepKind
	Member value.MemberKey
	Key    product.AbstractValue
}

// WriteFootprint is the source-facing write footprint derived from a Location.
// Flow owns the subsequent path-to-address normalization.
type WriteFootprint struct {
	WritePath                  constraint.Path
	PresentElementWrite        bool
	PresentElementArrayPath    constraint.Path
	HasPresentElementArrayPath bool
	PresentElementMember       []constraint.Segment
	Written                    product.AbstractValue
}

// FromPath converts a fully-static path into a normalized Location.
func FromPath(path constraint.Path) (Location, bool) {
	if path.Symbol == 0 {
		return Location{}, false
	}
	loc := Location{Root: path.Symbol, RootName: path.Root}
	for _, seg := range path.Segments {
		member, ok := value.MemberFromSegment(seg)
		if !ok {
			return Location{}, false
		}
		loc.Steps = append(loc.Steps, Step{Kind: StepStaticMember, Member: member})
	}
	return loc, true
}

// StaticPath projects a fully-static Location to a constraint.Path.
// Dynamic steps only count as static when their key evidence is a literal.
func (l Location) StaticPath() (constraint.Path, bool) {
	return l.staticPath(false)
}

// StaticPrefixPath projects the largest static prefix of a Location.
func (l Location) StaticPrefixPath() (constraint.Path, bool) {
	return l.staticPath(true)
}

func (l Location) staticPath(prefix bool) (constraint.Path, bool) {
	if l.Root == 0 {
		return constraint.Path{}, false
	}
	path := constraint.NewPath(l.Root, l.RootName)
	for _, step := range l.Steps {
		seg, ok := SegmentFromStep(step)
		if !ok {
			if prefix {
				break
			}
			return constraint.Path{}, false
		}
		path.Segments = append(path.Segments, seg)
	}
	return path, true
}

// WriteFootprint derives the normalized write footprint for this Location.
func (l Location) WriteFootprint(presentElementWrite bool, written product.AbstractValue) (WriteFootprint, bool) {
	path, ok := l.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return WriteFootprint{}, false
	}
	footprint := WriteFootprint{
		WritePath:           path,
		PresentElementWrite: presentElementWrite && len(l.Steps) > 0,
		Written:             written,
	}
	if footprint.PresentElementWrite {
		if arrayPath, member, ok := l.PresentElementMemberFootprint(); ok {
			footprint.PresentElementArrayPath = arrayPath
			footprint.HasPresentElementArrayPath = true
			footprint.PresentElementMember = member
		}
	}
	return footprint, true
}

// FinalDynamicIndexTargetPath returns the table path targeted by a write whose
// final access step is dynamic, such as rows[id] = value.
func (l Location) FinalDynamicIndexTargetPath() (constraint.Path, bool) {
	if l.Root == 0 || len(l.Steps) == 0 {
		return constraint.Path{}, false
	}
	if l.Steps[len(l.Steps)-1].Kind != StepDynamicIndex {
		return constraint.Path{}, false
	}
	target := Location{
		Root:     l.Root,
		RootName: l.RootName,
		Steps:    append([]Step(nil), l.Steps[:len(l.Steps)-1]...),
	}
	return target.StaticPath()
}

// PresentElementMemberFootprint reports the array path and static member suffix
// for writes like rows[k].id. The dynamic element itself is the write boundary;
// member facts below it are preserved/killed relative to the array element.
func (l Location) PresentElementMemberFootprint() (constraint.Path, []constraint.Segment, bool) {
	if l.Root == 0 {
		return constraint.Path{}, nil, false
	}
	array := constraint.NewPath(l.Root, l.RootName)
	for i, step := range l.Steps {
		if step.Kind == StepDynamicIndex {
			if i == len(l.Steps)-1 {
				return constraint.Path{}, nil, false
			}
			member, ok := staticMemberSuffix(l.Steps[i+1:])
			return array, member, ok
		}
		seg, ok := SegmentFromStep(step)
		if !ok {
			return constraint.Path{}, nil, false
		}
		array.Segments = append(array.Segments, seg)
	}
	return constraint.Path{}, nil, false
}

// SegmentFromStep converts a Location step to a static constraint segment when
// possible.
func SegmentFromStep(step Step) (constraint.Segment, bool) {
	switch step.Kind {
	case StepStaticMember:
		return SegmentFromMemberKey(step.Member)
	case StepDynamicIndex:
		if step.Key.IsZero() {
			return constraint.Segment{}, false
		}
		return staticIndexSegmentFromValue(step.Key)
	default:
		return constraint.Segment{}, false
	}
}

// SegmentFromMemberKey converts a static member key to a constraint segment.
func SegmentFromMemberKey(key value.MemberKey) (constraint.Segment, bool) {
	if !key.IsValid() {
		return constraint.Segment{}, false
	}
	switch key.Kind() {
	case value.MemberKindField:
		if key.Name() == "" {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: key.Name()}, true
	case value.MemberKindStringIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: key.Name()}, true
	case value.MemberKindIntIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: key.Index()}, true
	default:
		return constraint.Segment{}, false
	}
}

func staticMemberSuffix(steps []Step) ([]constraint.Segment, bool) {
	if len(steps) == 0 {
		return nil, false
	}
	out := make([]constraint.Segment, 0, len(steps))
	for _, step := range steps {
		seg, ok := SegmentFromStep(step)
		if !ok {
			return nil, false
		}
		out = append(out, seg)
	}
	return out, true
}

func staticIndexSegmentFromValue(av product.AbstractValue) (constraint.Segment, bool) {
	return staticIndexSegmentFromType(av.ProjectValue())
}

func staticIndexSegmentFromType(t typ.Type) (constraint.Segment, bool) {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Annotated:
		return staticIndexSegmentFromType(v.Inner)
	case *typ.Literal:
		switch v.Base {
		case kind.String:
			s, ok := v.Value.(string)
			if !ok {
				return constraint.Segment{}, false
			}
			return constraint.Segment{Kind: constraint.SegmentIndexString, Name: s}, true
		case kind.Integer:
			i, ok := v.Value.(int64)
			if !ok {
				return constraint.Segment{}, false
			}
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(i)}, true
		}
	}
	return constraint.Segment{}, false
}
