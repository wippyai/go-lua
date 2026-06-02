package effect

import (
	"fmt"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// PassThrough indicates a parameter flows directly to a return position.
//
// When a function returns one of its parameters unchanged, the PassThrough
// effect enables the type checker to preserve the parameter's exact type
// (including refinements) through the call.
//
// This is essential for assertion-style functions:
//
//	function assert_not_nil(val) if val == nil then error() end return val end
//	-- PassThrough{ParamIndex: 0, ReturnIndex: 0}
//
// At the call site:
//
//	local x: string? = maybe_get_string()
//	local y = assert_not_nil(x)  -- y is string (non-nil), not string?
//
// Without PassThrough, the return type would be inferred from the declared
// return type, losing the refinement from the nil check.
type PassThrough struct {
	ParamIndex  int // Which parameter (0-based)
	ReturnIndex int // Which return position (0-based)
}

func (PassThrough) label() {}

func (p PassThrough) String() string {
	return fmt.Sprintf("passthrough(param[%d]→ret[%d])", p.ParamIndex, p.ReturnIndex)
}

func (p PassThrough) Equals(other Label) bool {
	if o, ok := other.(PassThrough); ok {
		return p.ParamIndex == o.ParamIndex && p.ReturnIndex == o.ReturnIndex
	}

	return false
}

// FlowInto indicates a parameter path flows into a field of a returned table.
//
// When a function wraps a parameter or one of its fields into a record field,
// FlowInto enables the type checker to derive the record's field type from the
// caller's argument type.
//
// Example:
//
//	function wrap(val) return {inner = val} end
//	-- FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: FieldPath("inner")}
//
//	function map(info) return {message = info.message or "fallback"} end
//	-- FlowInto{ParamIndex: 0, SourcePath: FieldPath("message"), ReturnIndex: 0, TargetPath: FieldPath("message"), Remainder: string}
//
// At the call site:
//
//	local wrapped = wrap(42)  -- wrapped has type {inner: integer}
//	local wrapped2 = wrap("hello")  -- wrapped2 has type {inner: string}
//
// SourcePath and TargetPath are structural path suffixes. They intentionally do
// not encode paths as dotted strings, because dot fields, bracket string
// indexes, integer indexes, empty string keys, and keys containing dots are
// distinct abstract facts. SourcePath is empty when the whole parameter flows.
// Remainder is an optional static type for non-parameter alternatives in the
// expression, e.g. the fallback side of a Lua "or" expression.
type FlowInto struct {
	ParamIndex  int        // Which parameter (0-based)
	SourcePath  PathSuffix // Path under the parameter, empty means the whole parameter
	ReturnIndex int        // Which return position (0-based)
	TargetPath  PathSuffix // Path under the return
	Remainder   typ.Type   // Static non-parameter alternatives that may also reach the target
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
		return fmt.Sprintf("flowinto(%s→%s | %s)", source, target, f.Remainder)
	}
	return fmt.Sprintf("flowinto(%s→%s)", source, target)
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

// PathSuffix is a rootless structural path used by effect labels.
type PathSuffix []constraint.Segment

// FieldPath constructs a dot-field suffix for compact call sites and tests.
func FieldPath(names ...string) PathSuffix {
	if len(names) == 0 {
		return nil
	}
	out := make(PathSuffix, 0, len(names))
	for _, name := range names {
		out = append(out, constraint.Segment{Kind: constraint.SegmentField, Name: name})
	}
	return out
}

// PathSuffixFromSegments copies structured segments into an effect path suffix.
func PathSuffixFromSegments(segments []constraint.Segment) PathSuffix {
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

func (p PathSuffix) Segments() []constraint.Segment {
	if len(p) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(p))
	copy(out, p)
	return out
}

func (p PathSuffix) Append(seg constraint.Segment) PathSuffix {
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
	return constraint.FormatSegments(p)
}
