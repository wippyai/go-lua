package stdlib

import "github.com/wippyai/go-lua/types/typ"

// StackFrame is the type for a single stack frame in error call stacks.
var StackFrame = typ.NewRecord().
	Field("level", typ.Number).
	Field("source", typ.String).
	Field("line", typ.Number).
	Field("name", typ.String).
	Field("type", typ.String).
	Build()

// CallStack is the type for structured call stack information attached to errors.
var CallStack = typ.NewRecord().
	Field("thread", typ.String).
	Field("frames", typ.NewArray(StackFrame)).
	Build()

// ErrorSpec is the table form errors.new accepts: a required message and the
// optional kind, retryable flag and details the error carries.
var ErrorSpec = typ.NewRecord().
	Field("message", typ.String).
	OptField("kind", typ.String).
	OptField("retryable", typ.Boolean).
	OptField("details", typ.NewMap(typ.String, typ.Any)).
	Build()

var errorsMethods = typ.NewRecord().
	Field("NOT_FOUND", typ.String).
	Field("ALREADY_EXISTS", typ.String).
	Field("INVALID", typ.String).
	Field("PERMISSION_DENIED", typ.String).
	Field("UNAVAILABLE", typ.String).
	Field("INTERNAL", typ.String).
	Field("CANCELED", typ.String).
	Field("CONFLICT", typ.String).
	Field("TIMEOUT", typ.String).
	Field("RATE_LIMITED", typ.String).
	Field("UNKNOWN", typ.String).
	Field("new", typ.Func().
		Param("msg", typ.NewUnion(typ.String, ErrorSpec)).
		Returns(typ.LuaError).
		Build()).
	Field("wrap", typ.Func().
		Param("err", typ.NewUnion(typ.LuaError, typ.String)).
		Param("msg", typ.NewUnion(typ.String, typ.LuaError)).
		Returns(typ.LuaError).
		Build()).
	Field("call_stack", typ.Func().
		Param("err", typ.Any).
		Returns(typ.NewOptional(CallStack)).
		Build()).
	Field("is", typ.Func().
		Param("err", typ.Any).
		Param("kind", typ.String).
		Returns(typ.Boolean).
		Build()).
	Build()

// ErrorsLib provides types for structured error handling functions and kind constants.
var ErrorsLib typ.Type = errorsMethods
