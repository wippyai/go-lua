// Package stdlib is a legacy facade for common global type signatures.
package stdlib

import "github.com/wippyai/go-lua/types/typ"

type Config struct{}

func EngineConfig() Config { return Config{} }

func Library() map[string]typ.Type {
	return map[string]typ.Type{
		"assert":       typ.Func().Param("v", typ.Any).OptParam("message", typ.Any).Returns(typ.Any).Build(),
		"error":        typ.Func().Param("message", typ.Any).OptParam("level", typ.Integer).Returns(typ.Never).Build(),
		"require":      typ.Func().Param("modname", typ.String).Returns(typ.Any).Build(),
		"tostring":     typ.Func().Param("v", typ.Any).Returns(typ.String).Build(),
		"tonumber":     typ.Func().Param("v", typ.Any).OptParam("base", typ.Integer).Returns(typ.NewOptional(typ.Number)).Build(),
		"type":         typ.Func().Param("v", typ.Any).Returns(typ.String).Build(),
		"pairs":        typ.Func().Param("t", typ.Any).Returns(typ.Any, typ.Any, typ.Nil).Build(),
		"ipairs":       typ.Func().Param("t", typ.Any).Returns(typ.Any, typ.Any, typ.Integer).Build(),
		"pcall":        typ.Func().Param("f", typ.Any).Variadic(typ.Any).Returns(typ.Boolean, typ.Any).Build(),
		"xpcall":       typ.Func().Param("f", typ.Any).Param("msgh", typ.Any).Variadic(typ.Any).Returns(typ.Boolean, typ.Any).Build(),
		"print":        typ.Func().Variadic(typ.Any).Build(),
		"next":         typ.Func().Param("table", typ.Any).OptParam("index", typ.Any).Returns(typ.NewOptional(typ.Any), typ.NewOptional(typ.Any)).Build(),
		"select":       typ.Func().Param("index", typ.Any).Variadic(typ.Any).Returns(typ.Any).Build(),
		"rawget":       typ.Func().Param("table", typ.Any).Param("index", typ.Any).Returns(typ.Any).Build(),
		"rawset":       typ.Func().Param("table", typ.Any).Param("index", typ.Any).Param("value", typ.Any).Returns(typ.Any).Build(),
		"rawequal":     typ.Func().Param("v1", typ.Any).Param("v2", typ.Any).Returns(typ.Boolean).Build(),
		"rawlen":       typ.Func().Param("v", typ.Any).Returns(typ.Integer).Build(),
		"setmetatable": typ.Func().Param("table", typ.Any).Param("metatable", typ.NewOptional(typ.Any)).Returns(typ.Any).Build(),
		"getmetatable": typ.Func().Param("object", typ.Any).Returns(typ.NewOptional(typ.Any)).Build(),
	}
}
