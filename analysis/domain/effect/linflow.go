package effect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

type PassThrough struct {
	ParamIndex  int
	ReturnIndex int
}

func (PassThrough) label() {}

func (p PassThrough) String() string {
	return fmt.Sprintf("passthrough(param[%d]->ret[%d])", p.ParamIndex, p.ReturnIndex)
}

func (p PassThrough) Equals(other Label) bool {
	if o, ok := other.(PassThrough); ok {
		return p.ParamIndex == o.ParamIndex && p.ReturnIndex == o.ReturnIndex
	}
	return false
}

type FlowInto struct {
	ParamIndex  int
	SourcePath  PathSuffix
	ReturnIndex int
	TargetPath  PathSuffix
	Remainder   typ.Type
}

func (FlowInto) label() {}

func (f FlowInto) String() string {
	source := fmt.Sprintf("param[%d]", f.ParamIndex)
	if !f.SourcePath.Empty() {
		source += f.SourcePath.String()
	}
	target := fmt.Sprintf("ret[%d]", f.ReturnIndex)
	if !f.TargetPath.Empty() {
		target += f.TargetPath.String()
	}
	if f.Remainder != nil {
		return fmt.Sprintf("FlowInto(%s->%s | %s)", source, target, f.Remainder)
	}
	return fmt.Sprintf("FlowInto(%s->%s)", source, target)
}

func (f FlowInto) Equals(other Label) bool {
	if o, ok := other.(FlowInto); ok {
		return f.ParamIndex == o.ParamIndex &&
			f.ReturnIndex == o.ReturnIndex &&
			f.SourcePath.Equal(o.SourcePath) &&
			f.TargetPath.Equal(o.TargetPath) &&
			typ.TypeEquals(f.Remainder, o.Remainder)
	}
	return false
}

type PathSegmentKind uint8

const (
	PathSegmentField PathSegmentKind = iota
	PathSegmentIndexString
	PathSegmentIndexInt
)

type PathSegment struct {
	Kind  PathSegmentKind
	Name  string
	Index int
}

type PathSuffix []PathSegment

func FieldPath(names ...string) PathSuffix {
	if len(names) == 0 {
		return nil
	}
	out := make(PathSuffix, 0, len(names))
	for _, name := range names {
		out = append(out, PathSegment{Kind: PathSegmentField, Name: name})
	}
	return out
}

func PathSuffixFromSegments(segments []PathSegment) PathSuffix {
	if len(segments) == 0 {
		return nil
	}
	out := make(PathSuffix, len(segments))
	copy(out, segments)
	return out
}

func (p PathSuffix) Empty() bool {
	return len(p) == 0
}

func (p PathSuffix) Segments() []PathSegment {
	if len(p) == 0 {
		return nil
	}
	out := make([]PathSegment, len(p))
	copy(out, p)
	return out
}

func (p PathSuffix) Append(seg PathSegment) PathSuffix {
	out := make(PathSuffix, 0, len(p)+1)
	out = append(out, p...)
	out = append(out, seg)
	return out
}

func (p PathSuffix) Join(suffix PathSuffix) PathSuffix {
	if len(p) == 0 {
		return PathSuffixFromSegments(suffix)
	}
	if len(suffix) == 0 {
		return PathSuffixFromSegments(p)
	}
	out := make(PathSuffix, 0, len(p)+len(suffix))
	out = append(out, p...)
	out = append(out, suffix...)
	return out
}

func (p PathSuffix) Equal(other PathSuffix) bool {
	if len(p) != len(other) {
		return false
	}
	for i := range p {
		if p[i] != other[i] {
			return false
		}
	}
	return true
}

func (p PathSuffix) String() string {
	if len(p) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range p {
		switch seg.Kind {
		case PathSegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case PathSegmentIndexString:
			writeQuotedPathSegment(&b, seg.Name)
		case PathSegmentIndexInt:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		}
	}
	return b.String()
}

func writeQuotedPathSegment(b *strings.Builder, key string) {
	b.WriteString("[\"")
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(key[i])
		default:
			b.WriteByte(key[i])
		}
	}
	b.WriteString("\"]")
}
