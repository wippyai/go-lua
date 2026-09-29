package testutil

import (
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

var (
	processMessageType        typ.Type
	processEventType          typ.Type
	processMessageChannelType typ.Type
	processEventChannelType   typ.Type
	processRawChannelType     typ.Type
	processChannelGen         *typ.Generic
)

var processMessageElement = typ.NewTypeParam("T", nil)
var processMessageGeneric = typ.NewGeneric("process.Message", []*typ.TypeParam{processMessageElement},
	typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
		{Name: "data", Type: typ.Func().Param("self", typ.Self).Returns(processMessageElement).Build()},
	}))

// The type argument belongs to the receiving process. Message mode changes the
// envelope only; the same T is checked for both raw and message delivery.
func processListenType() typ.Type {
	t := typ.NewTypeParam("T", nil)
	rawOptions := typ.NewRecord().OptField("message", typ.False).Build()
	messageOptions := typ.NewRecord().Field("message", typ.True).Build()
	typedRawOptions := typ.NewRecord().OptField("message", typ.False).Field("type", typ.NewMeta(t)).Build()
	typedMessageOptions := typ.NewRecord().Field("message", typ.True).Field("type", typ.NewMeta(t)).Build()
	dynamicOptions := typ.NewRecord().OptField("message", typ.Boolean).Build()
	typedDynamicOptions := typ.NewRecord().OptField("message", typ.Boolean).Field("type", typ.NewMeta(t)).Build()
	return typ.NewUnion(
		typ.Func().TypeParam("T", nil).Param("topic", typ.String).Param("options", typedDynamicOptions).
			Returns(typ.NewUnion(typ.Instantiate(processChannelGen, t), typ.Instantiate(processChannelGen, typ.Instantiate(processMessageGeneric, t))), typ.NewOptional(typ.LuaError)).Build(),
		typ.Func().Param("topic", typ.String).Param("options", dynamicOptions).
			Returns(typ.NewUnion(processRawChannelType, processMessageChannelType), typ.NewOptional(typ.LuaError)).Build(),
		typ.Func().TypeParam("T", nil).Param("topic", typ.String).Param("options", typedMessageOptions).
			Returns(typ.Instantiate(processChannelGen, typ.Instantiate(processMessageGeneric, t)), typ.NewOptional(typ.LuaError)).Build(),
		typ.Func().TypeParam("T", nil).Param("topic", typ.String).Param("options", typedRawOptions).
			Returns(typ.Instantiate(processChannelGen, t), typ.NewOptional(typ.LuaError)).Build(),
		typ.Func().Param("topic", typ.String).Param("options", messageOptions).
			Returns(processMessageChannelType, typ.NewOptional(typ.LuaError)).Build(),
		typ.Func().Param("topic", typ.String).OptParam("options", rawOptions).
			Returns(processRawChannelType, typ.NewOptional(typ.LuaError)).Build(),
	)
}

func init() {
	processMessageType = typ.Instantiate(processMessageGeneric, typ.Any)

	eventRecord := typ.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		OptField("result", typ.Any).
		OptField("error", typ.Any).
		OptField("reason", typ.String).
		Build()
	eventMethods := typ.NewInterface("process.EventMethods", []typ.Method{
		{Name: "payload", Type: typ.Func().
			Param("self", typ.Self).
			Returns(typ.NewOptional(typ.Any)).
			Build()},
	})
	processEventType = typ.NewAlias("process.Event", typ.NewIntersection(eventRecord, eventMethods))

	if manifest := ChannelManifest(); manifest != nil {
		if t, ok := manifest.LookupType("Channel"); ok {
			if gen, ok := t.(*typ.Generic); ok {
				processChannelGen = gen
				processMessageChannelType = typ.Instantiate(processChannelGen, processMessageType)
				processEventChannelType = typ.Instantiate(processChannelGen, processEventType)
				processRawChannelType = typ.Instantiate(processChannelGen, typ.Any)
			}
		}
	}
	if processMessageChannelType == nil {
		processMessageChannelType = typ.Any
	}
	if processEventChannelType == nil {
		processEventChannelType = typ.Any
	}
	if processRawChannelType == nil {
		processRawChannelType = typ.Any
	}
}

var processOptionsType = typ.NewRecord().
	Field("trap_links", typ.Boolean).
	Field("upgradable", typ.Boolean).
	Build()

// set_options applies a partial update. get_options always returns both fields,
// but callers may set either option independently (or pass an empty table).
var processOptionsUpdateType = typ.NewRecord().
	OptField("trap_links", typ.Boolean).
	OptField("upgradable", typ.Boolean).
	Build()

var processEventKindsType = typ.NewRecord().
	Field("CANCEL", typ.String).
	Field("EXIT", typ.String).
	Field("LINK_DOWN", typ.String).
	Field("OUTDATED", typ.String).
	Build()

// process.registry surface: scoped registration with optional foreign PID.
// Scope constants live on the same table as the methods (LOCAL, EVENTUAL,
// CONSISTENT, STRONG), exposed as numeric tags.
var processRegistryFieldsType = typ.NewRecord().
	Field("LOCAL", typ.Number).
	Field("EVENTUAL", typ.Number).
	Field("CONSISTENT", typ.Number).
	Field("STRONG", typ.Number).
	Build()

var processRegistryMethodsType = typ.NewInterface("process.registry", []typ.Method{
	{Name: "register", Type: typ.Func().
		Param("name", typ.String).
		OptParam("pid", typ.String).
		OptParam("scope", typ.Number).
		Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
		Build()},
	{Name: "lookup", Type: typ.Func().
		Param("name", typ.String).
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Build()},
	{Name: "unregister", Type: typ.Func().
		Param("name", typ.String).
		OptParam("scope", typ.Number).
		Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
		Build()},
})

var processRegistrySubType = typ.NewIntersection(processRegistryMethodsType, processRegistryFieldsType)

var processSpawnBuilderType *typ.Interface

func init() {
	processSpawnBuilderType = typ.NewInterface("process.SpawnBuilder", []typ.Method{
		{Name: "with_context", Type: typ.Func().
			Param("self", typ.Self).
			Param("context", typ.NewMap(typ.String, typ.Any)).
			Returns(typ.Self).
			Build()},
		{Name: "with_options", Type: typ.Func().
			Param("self", typ.Self).
			Param("options", typ.NewMap(typ.String, typ.Any)).
			Returns(typ.Self).
			Build()},
		{Name: "with_actor", Type: typ.Func().
			Param("self", typ.Self).
			Param("actor", typ.Any).
			Returns(typ.Self).
			Build()},
		{Name: "with_scope", Type: typ.Func().
			Param("self", typ.Self).
			Param("scope", typ.Any).
			Returns(typ.Self).
			Build()},
		{Name: "with_name", Type: typ.Func().
			Param("self", typ.Self).
			Param("name", typ.String).
			Returns(typ.Self).
			Build()},
		{Name: "with_message", Type: typ.Func().
			Param("self", typ.Self).
			Param("msg", typ.String).
			Variadic(typ.Any).
			Returns(typ.Self).
			Build()},
		{Name: "spawn", Type: typ.Func().
			Param("self", typ.Self).
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_monitored", Type: typ.Func().
			Param("self", typ.Self).
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_linked", Type: typ.Func().
			Param("self", typ.Self).
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_linked_monitored", Type: typ.Func().
			Param("self", typ.Self).
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "exec", Type: typ.Func().
			Param("self", typ.Self).
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()},
	})
}

// ProcessManifest mirrors the process module manifest of the wippy runtime
// (runtime/lua/modules/process/types.go).
func ProcessManifest() *io.Manifest {
	m := io.NewManifest("process")

	m.DefineType("Message", processMessageType)
	m.DefineType("Event", processEventType)
	m.DefineType("Options", processOptionsType)
	m.DefineType("SpawnBuilder", processSpawnBuilderType)

	moduleFieldsType := typ.NewRecord().
		Field("event", processEventKindsType).
		Field("listen", processListenType()).
		Field("registry", processRegistrySubType).
		Build()

	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "pid", Type: typ.Func().Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "send", Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn", Type: typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_monitored", Type: typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_linked", Type: typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "spawn_linked_monitored", Type: typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "terminate", Type: typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "cancel", Type: typ.Func().
			Param("pid", typ.String).
			OptParam("reason", typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "get_options", Type: typ.Func().
			Returns(processOptionsType).
			Build()},
		{Name: "set_options", Type: typ.Func().
			Param("opts", processOptionsUpdateType).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "monitor", Type: typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "unmonitor", Type: typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "link", Type: typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "unlink", Type: typ.Func().
			Param("pid", typ.String).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "with_context", Type: typ.Func().
			Param("context", typ.NewMap(typ.String, typ.Any)).
			Returns(processSpawnBuilderType).
			Build()},
		{Name: "with_options", Type: typ.Func().
			Param("options", typ.NewMap(typ.String, typ.Any)).
			Returns(processSpawnBuilderType).
			Build()},
		{Name: "inbox", Type: typ.Func().
			Returns(processMessageChannelType).
			Build()},
		{Name: "events", Type: typ.Func().
			Returns(processEventChannelType).
			Build()},
		{Name: "unlisten", Type: typ.Func().
			Param("listener", typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "upgrade", Type: typ.Func().
			OptParam("path", typ.String).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
			Build()},
		{Name: "exec", Type: typ.Func().
			Param("module", typ.String).
			Param("func", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()},
	})

	m.SetExport(typ.NewIntersection(moduleMethodsType, moduleFieldsType))
	return m
}
