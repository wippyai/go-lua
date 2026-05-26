package effect

import (
	"fmt"

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
//	-- FlowInto{ParamIndex: 0, ReturnIndex: 0, TargetPath: "inner"}
//
//	function map(info) return {message = info.message or "fallback"} end
//	-- FlowInto{ParamIndex: 0, SourcePath: "message", ReturnIndex: 0, TargetPath: "message", Remainder: string}
//
// At the call site:
//
//	local wrapped = wrap(42)  -- wrapped has type {inner: integer}
//	local wrapped2 = wrap("hello")  -- wrapped2 has type {inner: string}
//
// SourcePath and TargetPath support dotted paths for nested fields. SourcePath
// is empty when the whole parameter flows. Remainder is an optional static type
// for non-parameter alternatives in the expression, e.g. the fallback side of
// a Lua "or" expression.
type FlowInto struct {
	ParamIndex  int      // Which parameter (0-based)
	SourcePath  string   // Field path under the parameter, empty means the whole parameter
	ReturnIndex int      // Which return position (0-based)
	TargetPath  string   // Field path under the return, e.g. "inner" or "data.value"
	Remainder   typ.Type // Static non-parameter alternatives that may also reach the target
}

func (FlowInto) label() {}

func (f FlowInto) String() string {
	source := fmt.Sprintf("param[%d]", f.ParamIndex)
	if f.SourcePath != "" {
		source += "." + f.SourcePath
	}
	target := fmt.Sprintf("ret[%d]", f.ReturnIndex)
	if f.TargetPath != "" {
		target += "." + f.TargetPath
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
			f.SourcePath == o.SourcePath &&
			f.TargetPath == o.TargetPath &&
			typ.TypeEquals(f.Remainder, o.Remainder)
	}

	return false
}
