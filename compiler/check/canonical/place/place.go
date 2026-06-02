package place

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Place is a semantic lvalue/read location inside a PointState.
//
// A Place is not a CFG point. A point is where Kildall stores a PointState;
// a Place is the location inside that state touched by a local transfer, such as
// users, users.byName, or subscribers[cid][pid].
//
// Place values are canonical IR with no AST/cfg-lowering dependencies.
type Place struct {
	Root     cfg.SymbolID
	RootName string
	Steps    []Step
}

// StepKind identifies the structural kind of a location segment.
type StepKind uint8

const (
	StepStaticMember StepKind = iota + 1
	StepDynamicIndex
)

// Step identifies a structural path segment under a Place root.
//
// Static steps store a resolved MemberKey. Dynamic steps keep an already-evaluated
// abstract key value for the eventual lvalue semantics consumed by product.
type Step struct {
	Kind   StepKind
	Member value.MemberKey
	Key    product.AbstractValue
}

// FromStaticPath converts a fully-static constraint.Path into a Place.
// It is lossless for paths with dynamic-free segments, returning false for
// placeholder/static-member-incompatible segments.
func FromStaticPath(path constraint.Path) (Place, bool) {
	if path.Symbol == 0 {
		return Place{}, false
	}
	p := Place{Root: path.Symbol, RootName: path.Root}
	for _, seg := range path.Segments {
		member, ok := value.MemberFromSegment(seg)
		if !ok {
			return Place{}, false
		}
		p.Steps = append(p.Steps, Step{Kind: StepStaticMember, Member: member})
	}
	return p, true
}

// FromSymbolPath builds a symbol-rooted Place from static segments.
func FromSymbolPath(sym cfg.SymbolID, segments []constraint.Segment) (Place, bool) {
	if sym == 0 {
		return Place{}, false
	}
	path := constraint.NewPath(sym, "")
	path.Segments = append(path.Segments, segments...)
	return FromStaticPath(path)
}

// staticPath projects the Place into a constraint.Path.
//
// If prefix is true, dynamic steps truncate the path. If prefix is false,
// any non-static step fails path conversion.
func (p Place) staticPath(prefix bool) (constraint.Path, bool) {
	if p.Root == 0 {
		return constraint.Path{}, false
	}
	path := constraint.NewPath(p.Root, p.RootName)
	for _, step := range p.Steps {
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

// StaticPath projects a fully-static Place to a constraint.Path.
// Dynamic steps fail instead of inventing an exact path.
func (p Place) StaticPath() (constraint.Path, bool) {
	return p.staticPath(false)
}

// StaticPrefixPath projects the largest static prefix of Place.
//
// It is the write invalidation footprint for dynamic places like x[k].field:
// stale facts under x can be killed without pretending k is a concrete segment.
func (p Place) StaticPrefixPath() (constraint.Path, bool) {
	return p.staticPath(true)
}

// StaticPathKey returns the constraint.Path key used by condition/function-ref
// style domains (`sym<ID>...`).
func (p Place) StaticPathKey() (constraint.PathKey, bool) {
	path, ok := p.StaticPath()
	if !ok || path.IsEmpty() {
		return "", false
	}
	return path.Key(), true
}

// String returns a deterministic location string representation.
func (p Place) String() string {
	root := p.RootName
	if root == "" && p.Root != 0 {
		root = "$sym" + strconv.FormatUint(uint64(p.Root), 10)
	}
	var b strings.Builder
	b.WriteString(root)
	for _, step := range p.Steps {
		switch step.Kind {
		case StepStaticMember:
			seg, ok := SegmentFromMemberKey(step.Member)
			if !ok {
				b.WriteString("[?]")
				continue
			}
			b.WriteString(constraint.FormatSegments([]constraint.Segment{seg}))
		case StepDynamicIndex:
			if seg, ok := SegmentFromStep(step); ok {
				b.WriteString(constraint.FormatSegments([]constraint.Segment{seg}))
			} else {
				b.WriteString("[?]")
			}
		default:
			b.WriteString("[?]")
		}
	}
	return b.String()
}

// SegmentFromStep converts a Place Step to a static constraint segment when
// possible. Dynamic indexes with concrete literal values are converted to their
// static segment form.
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

func staticIndexSegmentFromValue(av product.AbstractValue) (constraint.Segment, bool) {
	t := av.ProjectValue()
	return staticIndexSegmentFromType(t)
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
