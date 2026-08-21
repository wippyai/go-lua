package wippyv1

import (
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// StoreManifest transcribes the v1 store module: a key/value handle obtained by
// name, with fully described option and result records. v1 declares the backend
// and consistency constants as plain string fields rather than literal unions,
// so the constants carry no discriminating value here either.
func StoreManifest() *manifestwire.Manifest {
	declaration := newManifest("store")

	backendConst := typetable.NewRecord().
		Field("KV_RAFT", typ.String).
		Field("KV_CRDT", typ.String).
		Field("MEMORY", typ.String).
		Field("SQL", typ.String).
		Field("UNKNOWN", typ.String).
		Build()
	consistencyConst := typetable.NewRecord().
		Field("LINEARIZABLE", typ.String).
		Field("EVENTUAL", typ.String).
		Field("LOCAL", typ.String).
		Field("UNKNOWN", typ.String).
		Build()
	infoType := typetable.NewRecord().
		Field("id", typ.String).
		Field("backend", typ.String).
		Field("consistency", typ.String).
		Field("durable", typ.Boolean).
		Field("list", typ.Boolean).
		Field("versioned", typ.Boolean).
		Field("conditional_put", typ.Boolean).
		Field("ttl", typ.Boolean).
		Build()
	entryType := typetable.NewRecord().
		Field("key", typ.String).
		Field("value", typ.Any).
		Field("version", typ.String).
		Build()
	pageType := typetable.NewRecord().
		Field("items", typ.NewArray(entryType)).
		Field("cursor", typ.String).
		Field("has_more", typ.Boolean).
		Build()
	listOptionsType := typetable.NewRecord().
		OptField("prefix", typ.String).
		OptField("after", typ.String).
		OptField("limit", typ.Integer).
		Build()
	putOptionsType := typetable.NewRecord().
		OptField("ttl", typ.Number).
		OptField("only_if_absent", typ.Boolean).
		OptField("if_version", typ.String).
		Build()

	optionalError := typeexpr.Optional(errorType)
	storeType, storeMethods := declaredObject("store.Store", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "info", Type: typ.Func().Param("self", self).Returns(infoType, optionalError).Build()},
			{Name: "get", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(typ.Any, optionalError).Build()},
			{Name: "entry", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(entryType, optionalError).Build()},
			{Name: "list", Type: typ.Func().Param("self", self).OptParam("opts", listOptionsType).Returns(pageType, optionalError).Build()},
			{Name: "put", Type: typ.Func().Param("self", self).Param("key", typ.String).Param("value", typ.Any).OptParam("opts", putOptionsType).Returns(entryType, optionalError).Build()},
			{Name: "set", Type: typ.Func().Param("self", self).Param("key", typ.String).Param("value", typ.Any).OptParam("ttl", typ.Number).Returns(typ.Boolean, optionalError).Build()},
			{Name: "delete", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(typ.Boolean, optionalError).Build()},
			{Name: "has", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(typ.Boolean, optionalError).Build()},
			{Name: "release", Type: typ.Func().Param("self", self).Returns(typ.Boolean).Build()},
		}
	})

	declaration.DefineType("Store", storeType)
	declaration.DefineType("Info", infoType)
	declaration.DefineType("Entry", entryType)
	declaration.DefineType("Page", pageType)
	declaration.DefineType("ListOptions", listOptionsType)
	declaration.DefineType("PutOptions", putOptionsType)
	defineMethods(declaration, "Store", storeMethods)

	moduleGet := typ.Func().Param("name", typ.String).Returns(storeType, optionalError).Build()
	declaration.DefineFunctionSignature("get", signature.Function{Type: moduleGet})

	declaration.SetExport(typetable.NewRecord().
		Field("get", moduleGet).
		Field("backend", backendConst).
		Field("consistency", consistencyConst).
		Build())
	return declaration
}
