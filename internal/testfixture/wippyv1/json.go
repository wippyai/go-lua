package wippyv1

import (
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// JSONManifest transcribes the v1 json module. Its schema parameter is declared
// as a union of any table structure and a string reference; because one member
// of that union is Any the union carries no discrimination, which is exactly
// what v1 declares.
func JSONManifest() *manifestwire.Manifest {
	declaration := newManifest("json")

	schemaParam := typeexpr.Union(typ.Any, typ.String)

	encode := typ.Func().
		Param("value", typ.Any).
		Returns(typ.String, typeexpr.Optional(errorType)).
		Build()
	decode := typ.Func().
		Param("str", typ.String).
		Returns(typ.Any, typeexpr.Optional(errorType)).
		Build()
	validate := typ.Func().
		Param("schema", schemaParam).
		Param("data", typ.Any).
		Returns(typ.Boolean, typeexpr.Optional(errorType)).
		Build()
	validateString := typ.Func().
		Param("schema", schemaParam).
		Param("str", typ.String).
		Returns(typ.Boolean, typeexpr.Optional(errorType)).
		Build()

	declaration.DefineFunctionSignature("encode", signature.Function{Type: encode})
	declaration.DefineFunctionSignature("decode", signature.Function{Type: decode})
	declaration.DefineFunctionSignature("validate", signature.Function{Type: validate})
	declaration.DefineFunctionSignature("validate_string", signature.Function{Type: validateString})

	declaration.SetExport(typetable.NewRecord().
		Field("encode", encode).
		Field("decode", decode).
		Field("validate", validate).
		Field("validate_string", validateString).
		Build())
	return declaration
}
