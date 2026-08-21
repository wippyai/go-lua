package wippyv1

import (
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// streamType is the byte-stream handle v1 declares once in the stream module
// and shares with http and fs. It is declared here because the http surface
// references it directly.
var streamType, streamMethods = declaredObject("stream.Stream", func(self typ.Type) []typ.Method {
	optionalError := typeexpr.Optional(errorType)
	return []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", self).OptParam("n", typ.Number).Returns(typ.String, optionalError).Build()},
		{Name: "write", Type: typ.Func().Param("self", self).Param("data", typ.String).Returns(typ.Number, optionalError).Build()},
		{Name: "seek", Type: typ.Func().Param("self", self).OptParam("whence", typ.String).OptParam("offset", typ.Number).Returns(typ.Number, optionalError).Build()},
		{Name: "flush", Type: typ.Func().Param("self", self).Returns(typ.Boolean, optionalError).Build()},
		{Name: "stat", Type: typ.Func().Param("self", self).Returns(typ.Any, optionalError).Build()},
		{Name: "close", Type: typ.Func().Param("self", self).Returns(typ.Boolean, optionalError).Build()},
	}
})

// HTTPManifest transcribes the v1 http module: the server-side request and
// response handles plus the four constant tables.
//
// This is the module that carries the production surface's optional non-error
// returns. Request.query, Request.header, Request.content_type and
// Request.param each answer string? alongside their error result, and
// MultipartFile.header answers string? with no error result at all. Those
// declarations are the whole reason a caller must test the value before using
// it, so the boundary owes them to the checker intact.
func HTTPManifest() *manifestwire.Manifest {
	declaration := newManifest("http")

	optionalError := typeexpr.Optional(errorType)
	optionalString := typeexpr.Optional(typ.String)

	multipartFileType, multipartFileMethods := declaredObject("http.MultipartFile", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "stream", Type: typ.Func().Param("self", self).Returns(streamType, optionalError).Build()},
			{Name: "size", Type: typ.Func().Param("self", self).Returns(typ.Number, optionalError).Build()},
			{Name: "name", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "header", Type: typ.Func().Param("self", self).Param("name", typ.String).Returns(optionalString).Build()},
		}
	})
	multipartFormType := typetable.NewRecord().
		OptField("values", typ.NewMap(typ.String, typ.NewArray(typ.String))).
		OptField("files", typ.NewMap(typ.String, typ.NewArray(multipartFileType))).
		Build()

	requestType, requestMethods := declaredObject("http.Request", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "method", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "path", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "query", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(optionalString, optionalError).Build()},
			{Name: "query_params", Type: typ.Func().Param("self", self).Returns(typ.NewMap(typ.String, typ.String), optionalError).Build()},
			{Name: "header", Type: typ.Func().Param("self", self).Param("name", typ.String).Returns(optionalString, optionalError).Build()},
			{Name: "content_type", Type: typ.Func().Param("self", self).Returns(optionalString, optionalError).Build()},
			{Name: "content_length", Type: typ.Func().Param("self", self).Returns(typ.Number, optionalError).Build()},
			{Name: "host", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "remote_addr", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "body", Type: typ.Func().Param("self", self).Returns(typ.String, optionalError).Build()},
			{Name: "body_json", Type: typ.Func().Param("self", self).Returns(typ.Any, optionalError).Build()},
			{Name: "has_body", Type: typ.Func().Param("self", self).Returns(typ.Boolean, optionalError).Build()},
			{Name: "accepts", Type: typ.Func().Param("self", self).Param("contentType", typ.String).Returns(typ.Boolean, optionalError).Build()},
			{Name: "is_content_type", Type: typ.Func().Param("self", self).Param("contentType", typ.String).Returns(typ.Boolean, optionalError).Build()},
			{Name: "param", Type: typ.Func().Param("self", self).Param("key", typ.String).Returns(optionalString, optionalError).Build()},
			{Name: "params", Type: typ.Func().Param("self", self).Returns(typ.NewMap(typ.String, typ.String), optionalError).Build()},
			{Name: "stream", Type: typ.Func().Param("self", self).Returns(streamType, optionalError).Build()},
			{Name: "parse_multipart", Type: typ.Func().Param("self", self).OptParam("maxMemory", typ.Number).Returns(multipartFormType, optionalError).Build()},
		}
	})

	responseType, responseMethods := declaredObject("http.Response", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "set_status", Type: typ.Func().Param("self", self).Param("status", typ.Number).Returns(optionalError).Build()},
			{Name: "set_header", Type: typ.Func().Param("self", self).Param("name", typ.String).Param("value", typ.String).Returns(optionalError).Build()},
			{Name: "write", Type: typ.Func().Param("self", self).Param("data", typ.String).Returns(optionalError).Build()},
			{Name: "flush", Type: typ.Func().Param("self", self).Returns(optionalError).Build()},
			{Name: "write_json", Type: typ.Func().Param("self", self).Param("data", typ.Any).Returns(optionalError).Build()},
			{Name: "set_content_type", Type: typ.Func().Param("self", self).Param("contentType", typ.String).Returns(optionalError).Build()},
			{Name: "write_event", Type: typ.Func().Param("self", self).Param("data", typ.Any).Returns(optionalError).Build()},
			{Name: "set_transfer", Type: typ.Func().Param("self", self).Param("encoding", typ.String).Returns(optionalError).Build()},
		}
	})

	declaration.DefineType("Request", requestType)
	declaration.DefineType("Response", responseType)
	declaration.DefineType("MultipartFile", multipartFileType)
	declaration.DefineType("MultipartForm", multipartFormType)
	declaration.DefineType("Stream", streamType)

	defineMethods(declaration, "Request", requestMethods)
	defineMethods(declaration, "Response", responseMethods)
	defineMethods(declaration, "MultipartFile", multipartFileMethods)
	defineMethods(declaration, "Stream", streamMethods)

	methodConst := typetable.NewRecord().
		Field("GET", typ.String).
		Field("POST", typ.String).
		Field("PUT", typ.String).
		Field("DELETE", typ.String).
		Field("PATCH", typ.String).
		Field("HEAD", typ.String).
		Field("OPTIONS", typ.String).
		Build()
	statusConst := typetable.NewRecord().
		Field("OK", typ.Number).
		Field("CREATED", typ.Number).
		Field("ACCEPTED", typ.Number).
		Field("NO_CONTENT", typ.Number).
		Field("PARTIAL_CONTENT", typ.Number).
		Field("MOVED_PERMANENTLY", typ.Number).
		Field("FOUND", typ.Number).
		Field("SEE_OTHER", typ.Number).
		Field("NOT_MODIFIED", typ.Number).
		Field("TEMPORARY_REDIRECT", typ.Number).
		Field("PERMANENT_REDIRECT", typ.Number).
		Field("BAD_REQUEST", typ.Number).
		Field("UNAUTHORIZED", typ.Number).
		Field("PAYMENT_REQUIRED", typ.Number).
		Field("FORBIDDEN", typ.Number).
		Field("NOT_FOUND", typ.Number).
		Field("METHOD_NOT_ALLOWED", typ.Number).
		Field("NOT_ACCEPTABLE", typ.Number).
		Field("CONFLICT", typ.Number).
		Field("GONE", typ.Number).
		Field("UNPROCESSABLE", typ.Number).
		Field("TOO_MANY_REQUESTS", typ.Number).
		Field("INTERNAL_ERROR", typ.Number).
		Field("INTERNAL_SERVER_ERROR", typ.Number).
		Field("NOT_IMPLEMENTED", typ.Number).
		Field("BAD_GATEWAY", typ.Number).
		Field("SERVICE_UNAVAILABLE", typ.Number).
		Field("GATEWAY_TIMEOUT", typ.Number).
		Field("VERSION_NOT_SUPPORTED", typ.Number).
		Build()
	contentConst := typetable.NewRecord().
		Field("JSON", typ.String).
		Field("FORM", typ.String).
		Field("MULTIPART", typ.String).
		Field("TEXT", typ.String).
		Field("STREAM", typ.String).
		Build()
	transferConst := typetable.NewRecord().
		Field("CHUNKED", typ.String).
		Field("SSE", typ.String).
		Build()

	request := typ.Func().OptParam("config", typ.Any).Returns(requestType, optionalError).Build()
	response := typ.Func().Returns(responseType, optionalError).Build()
	declaration.DefineFunctionSignature("request", signature.Function{Type: request})
	declaration.DefineFunctionSignature("response", signature.Function{Type: response})

	declaration.SetExport(typetable.NewRecord().
		Field("request", request).
		Field("response", response).
		Field("METHOD", methodConst).
		Field("STATUS", statusConst).
		Field("CONTENT", contentConst).
		Field("TRANSFER", transferConst).
		Build())
	return declaration
}

// defineMethods registers one declared object type's methods as manifest-local
// callables under the "Type.member" path the catalogue splits into a binding.
func defineMethods(declaration *manifestwire.Manifest, typeName string, methods []typ.Method) {
	for _, method := range methods {
		declaration.DefineFunctionSignature(typeName+"."+method.Name, signature.Function{Type: method.Type})
	}
}
