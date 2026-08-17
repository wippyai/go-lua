package stdlib

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/types/signature"
)

func errorsDeclaration() declaration {
	details := typ.NewMap(typ.String, typ.Any)
	frame := typetable.NewRecord().
		ReadonlyField("level", typ.Number).
		ReadonlyField("source", typ.String).
		ReadonlyField("line", typ.Number).
		ReadonlyField("name", typ.String).
		ReadonlyField("type", typ.String).
		Build()
	stack := typetable.NewRecord().
		ReadonlyField("thread", typ.String).
		ReadonlyField("frames", typ.NewArray(frame)).
		Build()

	kind := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	retryable := typ.Func().Param("self", typ.Any).
		Returns(normalize.Optional(typ.Boolean)).Build()
	detailMethod := typ.Func().Param("self", typ.Any).
		Returns(normalize.Optional(details)).Build()
	message := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	stackMethod := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: kind},
		{Name: "retryable", Type: retryable},
		{Name: "details", Type: detailMethod},
		{Name: "message", Type: message},
		{Name: "stack", Type: stackMethod},
	})

	newInput := typeexpr.Union(
		typ.String,
		typetable.NewRecord().
			Field("message", typ.String).
			OptField("kind", typ.String).
			OptField("retryable", typ.Boolean).
			OptField("details", details).
			Build(),
	)

	return declaration{
		signatures: map[string]signature.Function{
			"call_stack": openAuthored("stdlib.errors.call_stack.allocate", typ.Func().
				Param("err", typ.Any).Returns(normalize.Optional(stack)).Build(),
				ownership.BorrowAll{}),
			"is": authored(typ.Func().
				Param("err", typ.Any).Param("kind", typ.String).
				Returns(typ.Boolean).Build(), ownership.BorrowAll{}),
			"new": openAuthored("stdlib.errors.new.allocate", typ.Func().
				Param("message_or_options", newInput).Returns(errorType).Build()),
			"wrap": openAuthored("stdlib.errors.wrap.allocate", typ.Func().
				Param("parent", typeexpr.Union(typ.String, errorType)).
				Param("context", typeexpr.Union(typ.String, errorType)).
				Returns(errorType).Build(),
				ownership.Retain{Param: effect.ParamRef{Index: 0}}),
		},
		methods: map[string]signature.Function{
			"Error.__concat": authored(typ.Func().
				Param("self", errorType).Param("other", typ.Any).Returns(typ.String).Build(),
				ownership.BorrowAll{}),
			"Error.__tostring": authored(typ.Func().
				Param("self", errorType).Returns(typ.String).Build(), ownership.BorrowAll{}),
			"Error.details": openAuthored("stdlib.errors.details.allocate", detailMethod,
				ownership.BorrowAll{}),
			"Error.kind":      authored(kind, ownership.BorrowAll{}),
			"Error.message":   authored(message, ownership.BorrowAll{}),
			"Error.retryable": authored(retryable, ownership.BorrowAll{}),
			"Error.stack":     authored(stackMethod, ownership.BorrowAll{}),
		},
		values: map[string]typ.Type{
			"NOT_FOUND":         typ.LiteralString("NotFound"),
			"ALREADY_EXISTS":    typ.LiteralString("AlreadyExists"),
			"INVALID":           typ.LiteralString("Invalid"),
			"PERMISSION_DENIED": typ.LiteralString("PermissionDenied"),
			"UNAVAILABLE":       typ.LiteralString("Unavailable"),
			"INTERNAL":          typ.LiteralString("Internal"),
			"CANCELED":          typ.LiteralString("Canceled"),
			"CONFLICT":          typ.LiteralString("Conflict"),
			"TIMEOUT":           typ.LiteralString("Timeout"),
			"RATE_LIMITED":      typ.LiteralString("RateLimited"),
			// The native constant stores Kind(""); Kind.String() is "Unknown",
			// but converting the Kind directly to LString preserves the empty value.
			"UNKNOWN": typ.LiteralString(""),
		},
		types: map[string]typ.Type{
			"Error": errorType, "StackFrame": frame, "StackTrace": stack,
		},
		errorType: errorType,
		readonly:  true,
	}
}
