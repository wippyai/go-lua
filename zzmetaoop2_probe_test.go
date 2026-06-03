package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestZZMetaOop2Probe reproduces metatable-oop cross-module identity: a handler
// typed (self: class.EventEmitter, ...) passed where the imported on() expects
// EventHandler = (self: EventEmitter, ...) defined inside the class module.
func TestZZMetaOop2Probe(t *testing.T) {
	classSrc := `
type EventHandler = (self: EventEmitter, event: string, data: any) -> ()
type EventEmitter = {
    _handlers: {[string]: {EventHandler}},
    on: (self: EventEmitter, event: string, handler: EventHandler) -> EventEmitter,
    emit: (self: EventEmitter, event: string, data: any) -> (),
}
local EventEmitter = {}
EventEmitter.__index = EventEmitter
function EventEmitter.new(): EventEmitter
    local self: EventEmitter = { _handlers = {}, on = EventEmitter.on, emit = EventEmitter.emit }
    setmetatable(self, EventEmitter)
    return self
end
function EventEmitter:on(event: string, handler: EventHandler): EventEmitter
    return self
end
function EventEmitter:emit(event: string, data: any) end
local M = {}
M.EventEmitter = EventEmitter
M.new = EventEmitter.new
return M
`
	counterSrc := `
local class = require("class")
local function on_change(emitter: class.EventEmitter, handler: (self: class.EventEmitter, event: string, data: any) -> ())
    emitter:on("change", handler)
end
return on_change
`
	classMod := testutil.CheckAndExport(classSrc, "class")
	t.Logf("class module: %d errors", len(classMod.Errors))
	for _, d := range classMod.Errors {
		t.Logf("   class %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
	}
	counterMod := testutil.CheckAndExport(counterSrc, "counter", testutil.WithModule("class", classMod))
	t.Logf("counter module: %d errors", len(counterMod.Errors))
	for _, d := range counterMod.Errors {
		t.Logf("   counter %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
	}

	// Pull the class export to inspect EventEmitter and EventHandler identities.
	cm := classMod.Manifest
	names := make([]string, 0, len(cm.Types))
	for n := range cm.Types {
		names = append(names, n)
	}
	t.Logf("class manifest type names: %v", names)
	ee := cm.Types["EventEmitter"]
	eh := cm.Types["EventHandler"]
	if ee != nil {
		t.Logf("EventEmitter type: %s (%T)", ee.String(), ee)
	}
	if eh != nil {
		t.Logf("EventHandler type: %s (%T)", eh.String(), eh)
	}

	// Reconstruct the comparison the arg check does: caller's handler param type
	// (self: class.EventEmitter, ...) vs the imported EventHandler (self: EventEmitter, ...).
	eeTarget := unwrap.Alias(ee)
	t.Logf("EventEmitter unaliased: %s (%T)", eeTarget.String(), eeTarget)
	if rec, ok := eeTarget.(*typ.Recursive); ok {
		t.Logf("EventEmitter Recursive: ID=%d Name=%q", rec.ID, rec.Name)
	}
	ehTarget := unwrap.Alias(eh)
	t.Logf("EventHandler unaliased: %s (%T)", ehTarget.String(), ehTarget)

	// callerHandler self uses the SAME imported EventEmitter alias (class.EventEmitter).
	callerHandler := typ.Func().
		Param("self", ee).
		Param("event", typ.String).
		Param("data", typ.Any).
		Build()
	t.Logf("callerHandler: %s", callerHandler.String())
	t.Logf("IsSubtype(callerHandler, EventHandler) = %v", subtype.IsSubtype(callerHandler, eh))
	t.Logf("Consistent(callerHandler, EventHandler) = %v", subtype.Consistent(callerHandler, eh))

	// The self contravariant sub-check: EventHandler.self <: callerHandler.self.
	if ehFn := unwrap.Function(ehTarget); ehFn != nil && len(ehFn.Params) > 0 {
		ehSelf := ehFn.Params[0].Type
		t.Logf("EventHandler.self: %s (%T)", ehSelf.String(), ehSelf)
		t.Logf("IsSubtype(EventHandler.self=%s, caller.self=class.EventEmitter) = %v", ehSelf.String(), subtype.IsSubtype(ehSelf, ee))
		t.Logf("IsSubtype(class.EventEmitter, EventHandler.self) = %v", subtype.IsSubtype(ee, ehSelf))

		// The real engine's failing comparison: a BARE unresolved Ref("EventEmitter")
		// (module="") vs the resolved Recursive family #2. checkFunction self contravariance.
		bareRef := typ.NewRef("", "EventEmitter")
		t.Logf("bareRef vs Recursive#2: IsSubtype(Ref, Rec)=%v IsSubtype(Rec, Ref)=%v",
			subtype.IsSubtype(bareRef, eeTarget), subtype.IsSubtype(eeTarget, bareRef))
		// And a module-qualified Ref("class","EventEmitter").
		qref := typ.NewRef("class", "EventEmitter")
		t.Logf("qualifiedRef(class.EventEmitter) vs Recursive#2: IsSubtype(Ref,Rec)=%v IsSubtype(Rec,Ref)=%v",
			subtype.IsSubtype(qref, eeTarget), subtype.IsSubtype(eeTarget, qref))

		// Function whose self is the bare Ref (the imported EventHandler shape) vs
		// function whose self is the resolved family (the caller handler shape).
		ehShape := typ.Func().Param("self", bareRef).Param("event", typ.String).Param("data", typ.Any).Build()
		callerShape := typ.Func().Param("self", eeTarget).Param("event", typ.String).Param("data", typ.Any).Build()
		t.Logf("IsSubtype(callerShape{self:Rec#2}, ehShape{self:Ref}) = %v", subtype.IsSubtype(callerShape, ehShape))
		t.Logf("Consistent(callerShape{self:Rec#2}, ehShape{self:Ref}) = %v", subtype.Consistent(callerShape, ehShape))
	}
}
