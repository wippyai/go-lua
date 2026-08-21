package wippyv1

import (
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// ExprManifest transcribes the v1 expr module: an expression compiler whose
// compiled Program is a declared object type with one method. Both module
// members and the Program method evaluate against an untyped context and yield
// an untyped value, so the surface is entirely any-typed apart from the source
// text and the error result.
func ExprManifest() *manifestwire.Manifest {
	declaration := newManifest("expr")

	programType, programMethods := declaredObject("expr.Program", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "run", Type: typ.Func().
				Param("self", self).
				OptParam("context", typ.Any).
				Returns(typ.Any, typeexpr.Optional(errorType)).
				Build()},
		}
	})
	declaration.DefineType("Program", programType)
	defineMethods(declaration, "Program", programMethods)

	compile := typ.Func().
		Param("text", typ.String).
		OptParam("context", typ.Any).
		Returns(programType, typeexpr.Optional(errorType)).
		Build()
	eval := typ.Func().
		Param("text", typ.String).
		OptParam("context", typ.Any).
		Returns(typ.Any, typeexpr.Optional(errorType)).
		Build()
	declaration.DefineFunctionSignature("compile", signature.Function{Type: compile})
	declaration.DefineFunctionSignature("eval", signature.Function{Type: eval})

	declaration.SetExport(typetable.NewRecord().
		Field("compile", compile).
		Field("eval", eval).
		Build())
	return declaration
}
