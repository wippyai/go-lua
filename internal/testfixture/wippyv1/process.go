package wippyv1

import (
	"sort"

	"github.com/wippyai/go-lua/domain/type/ambient"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// ProcessManifest transcribes the v1 process module: the actor surface that
// spawns, links, monitors and messages other processes.
//
// It is the module the send-safety question is really about, and the
// transcription answers that question by what it does not contain. v1 declares
// send as pid, topic and a variadic payload answering a boolean and an error,
// and it declares nothing else: no ownership label on the payload, no transfer
// endpoint, no publication, no freeze. The same holds for every spawn variant
// that forwards a variadic payload into a new actor. Whatever admission rule
// the checker applies to a cross-actor payload, the v1 manifest supplies no
// evidence for it.
//
// The one behavioral declaration v1 does make on this surface is on listen: a
// contract spec whose return type is refined to a Message channel when the
// options argument carries message = true. The canonical manifest boundary has
// no field-conditional return declaration, so listen is transcribed with the
// unrefined channel result it declares outside that case.
func ProcessManifest() *manifestwire.Manifest {
	declaration := newManifest("process")

	optionalError := typeexpr.Optional(errorType)

	messageType, messageMethods := declaredObject("process.Message", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "from", Type: typ.Func().Param("self", self).Returns(typ.String).Build()},
			{Name: "topic", Type: typ.Func().Param("self", self).Returns(typ.String).Build()},
			{Name: "payload", Type: typ.Func().Param("self", self).Returns(typ.Any).Build()},
		}
	})

	eventRecord := typetable.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		OptField("result", typ.Any).
		OptField("error", typ.Any).
		OptField("reason", typ.String).
		Build()
	eventMethodsType, eventMethods := declaredObject("process.EventMethods", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "payload", Type: typ.Func().Param("self", self).Returns(typeexpr.Optional(typ.Any)).Build()},
		}
	})
	eventType := typ.NewAlias("process.Event", typeexpr.Intersection(eventRecord, eventMethodsType))

	channelGeneric := ambient.ChannelGeneric()
	messageChannel := typ.Instantiate(channelGeneric, messageType)
	eventChannel := typ.Instantiate(channelGeneric, eventType)
	rawChannel := typ.Instantiate(channelGeneric, typ.Any)

	optionsType := typetable.NewRecord().
		Field("trap_links", typ.Boolean).
		Field("upgradable", typ.Boolean).
		Build()
	// set_options applies a partial update; get_options always answers both.
	optionsUpdateType := typetable.NewRecord().
		OptField("trap_links", typ.Boolean).
		OptField("upgradable", typ.Boolean).
		Build()

	eventConst := typetable.NewRecord().
		Field("CANCEL", typ.String).
		Field("EXIT", typ.String).
		Field("LINK_DOWN", typ.String).
		Field("OUTDATED", typ.String).
		Build()

	registryRegister := typ.Func().Param("name", typ.String).OptParam("pid", typ.String).OptParam("scope", typ.Number).Returns(typ.Boolean, optionalError).Build()
	registryLookup := typ.Func().Param("name", typ.String).Returns(typ.String, optionalError).Build()
	registryUnregister := typ.Func().Param("name", typ.String).OptParam("scope", typ.Number).Returns(typ.Boolean, optionalError).Build()
	registryType := typetable.NewRecord().
		Field("register", registryRegister).
		Field("lookup", registryLookup).
		Field("unregister", registryUnregister).
		Field("LOCAL", typ.Number).
		Field("EVENTUAL", typ.Number).
		Field("CONSISTENT", typ.Number).
		Field("STRONG", typ.Number).
		Build()
	declaration.DefineFunctionSignature("registry.register", signature.Function{Type: registryRegister})
	declaration.DefineFunctionSignature("registry.lookup", signature.Function{Type: registryLookup})
	declaration.DefineFunctionSignature("registry.unregister", signature.Function{Type: registryUnregister})

	// v1 spells the builder chain with typ.Self in return position. Self names
	// the receiver, so it is meaningful only while a call site still knows the
	// receiver; the module boundary carries the type on its own and a portable
	// declaration must therefore name the builder itself. The recursive node is
	// that name, and it says the same thing v1 means: each with_* answers the
	// builder it was called on.
	var builderMethods []typ.Method
	spawnBuilderType := typ.NewRecursive("process.SpawnBuilder", func(self typ.Type) typ.Type {
		spawnResult := func() *typ.Function {
			return typ.Func().
				Param("self", self).
				Param("module", typ.String).
				Param("func", typ.String).
				Variadic(typ.Any).
				Returns(typ.String, optionalError).
				Build()
		}
		builderMethods = []typ.Method{
			{Name: "with_context", Type: typ.Func().Param("self", self).Param("context", typ.NewMap(typ.String, typ.Any)).Returns(self).Build()},
			{Name: "with_options", Type: typ.Func().Param("self", self).Param("options", typ.NewMap(typ.String, typ.Any)).Returns(self).Build()},
			{Name: "with_actor", Type: typ.Func().Param("self", self).Param("actor", typ.Any).Returns(self).Build()},
			{Name: "with_scope", Type: typ.Func().Param("self", self).Param("scope", typ.Any).Returns(self).Build()},
			{Name: "with_name", Type: typ.Func().Param("self", self).Param("name", typ.String).Returns(self).Build()},
			{Name: "with_message", Type: typ.Func().Param("self", self).Param("msg", typ.String).Variadic(typ.Any).Returns(self).Build()},
			{Name: "spawn", Type: spawnResult()},
			{Name: "spawn_monitored", Type: spawnResult()},
			{Name: "spawn_linked", Type: spawnResult()},
			{Name: "spawn_linked_monitored", Type: spawnResult()},
			{Name: "exec", Type: typ.Func().Param("self", self).Param("module", typ.String).Param("func", typ.String).Variadic(typ.Any).Returns(typ.Any, optionalError).Build()},
		}
		return typ.NewInterface("process.SpawnBuilder", builderMethods)
	})

	declaration.DefineType("Message", messageType)
	declaration.DefineType("Event", eventType)
	declaration.DefineType("Options", optionsType)
	declaration.DefineType("SpawnBuilder", spawnBuilderType)
	defineMethods(declaration, "Message", messageMethods)
	defineMethods(declaration, "SpawnBuilder", builderMethods)
	defineMethods(declaration, "Event", eventMethods)

	moduleSpawn := func() *typ.Function {
		return typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, optionalError).
			Build()
	}
	pidPredicate := func() *typ.Function {
		return typ.Func().Param("pid", typ.String).Returns(typ.Boolean, optionalError).Build()
	}

	members := map[string]*typ.Function{
		"id":                     typ.Func().Returns(typ.String, optionalError).Build(),
		"pid":                    typ.Func().Returns(typ.String, optionalError).Build(),
		"send":                   typ.Func().Param("pid", typ.String).Param("topic", typ.String).Variadic(typ.Any).Returns(typ.Boolean, optionalError).Build(),
		"spawn":                  moduleSpawn(),
		"spawn_monitored":        moduleSpawn(),
		"spawn_linked":           moduleSpawn(),
		"spawn_linked_monitored": moduleSpawn(),
		"terminate":              pidPredicate(),
		"cancel":                 typ.Func().Param("pid", typ.String).OptParam("reason", typ.Any).Returns(typ.Boolean, optionalError).Build(),
		"get_options":            typ.Func().Returns(optionsType).Build(),
		"set_options":            typ.Func().Param("opts", optionsUpdateType).Returns(typ.Boolean, optionalError).Build(),
		"monitor":                pidPredicate(),
		"unmonitor":              pidPredicate(),
		"link":                   pidPredicate(),
		"unlink":                 pidPredicate(),
		"with_context":           typ.Func().Param("context", typ.NewMap(typ.String, typ.Any)).Returns(spawnBuilderType).Build(),
		"with_options":           typ.Func().Param("options", typ.NewMap(typ.String, typ.Any)).Returns(spawnBuilderType).Build(),
		"inbox":                  typ.Func().Returns(messageChannel).Build(),
		"events":                 typ.Func().Returns(eventChannel).Build(),
		"listen":                 typ.Func().Param("topic", typ.String).OptParam("options", typ.Any).Returns(rawChannel, optionalError).Build(),
		"unlisten":               typ.Func().Param("listener", typ.Any).Returns(typ.Boolean, optionalError).Build(),
		"upgrade":                typ.Func().OptParam("path", typ.String).Variadic(typ.Any).Returns(typ.Boolean, optionalError).Build(),
		"exec":                   typ.Func().Param("module", typ.String).Param("func", typ.String).Variadic(typ.Any).Returns(typ.Any, optionalError).Build(),
	}

	export := typetable.NewRecord().
		Field("event", eventConst).
		Field("registry", registryType)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		declaration.DefineFunctionSignature(name, signature.Function{Type: members[name]})
		export = export.Field(name, members[name])
	}
	declaration.SetExport(export.Build())
	return declaration
}
