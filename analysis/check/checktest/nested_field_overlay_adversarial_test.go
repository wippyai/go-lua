package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDirectCallArgumentPreservesDeclaredSiblingsAfterNestedFieldWrite(t *testing.T) {
	result := Check(`
type Envelope = { kind: string, id: string }
type ActorState = {
    processed: {[string]: Envelope},
    counters: {[string]: number},
    last_id: string?,
}
type Actor = {
    state: ActorState,
    handlers: {[string]: (Actor, Envelope) -> ()},
}

local function bump(state: ActorState, key: string): ()
    local current = state.counters[key]
    if current then
        state.counters[key] = current + 1
    else
        state.counters[key] = 1
    end
end

local function dispatch(actor: Actor, message: Envelope): ()
    actor.state.processed[message.id] = message
    actor.state.last_id = message.id
    bump(actor.state, message.kind)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nested leaf write preserves ActorState siblings", result.Diagnostics)
	}
}

func TestDirectCallArgumentPreservesReceiverShapeAfterNestedFieldWrite(t *testing.T) {
	result := Check(`
type Envelope = { kind: string, id: string }
type ActorState = {
    processed: {[string]: Envelope},
    counters: {[string]: number},
    last_id: string?,
}
type Actor = {
    state: ActorState,
    handlers: {[string]: (Actor, Envelope) -> ()},
}

local function dispatch(actor: Actor, message: Envelope): ()
    local handler = actor.handlers[message.kind]
    if not handler then
        return
    end
    actor.state.last_id = message.id
    handler(actor, message)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nested state write preserves Actor receiver shape", result.Diagnostics)
	}
}

func TestDirectCallArgumentReportsBadNestedFieldReplacement(t *testing.T) {
	result := Check(`
type Envelope = { kind: string, id: string }
type ActorState = {
    processed: {[string]: Envelope},
    counters: {[string]: number},
    last_id: string?,
}

local function bump(state: ActorState): () end

local state: ActorState = {
    processed = {},
    counters = {},
    last_id = nil,
}
state.counters = "bad"
bump(state)
	`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	requireEvidenceMessage(t, diag, "assignment target state.counters requires {[string]: number}")
}

func TestDirectCallArgumentStillRejectsUntrustedAnyAsRecord(t *testing.T) {
	result := Check(`
type Envelope = { kind: string, id: string }
type ActorState = {
    processed: {[string]: Envelope},
    counters: {[string]: number},
    last_id: string?,
}

local function bump(state: ActorState): () end

local raw: any = {
    last_id = "m1",
}
bump(raw)
	`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "no proof on this path shows raw satisfies the parameter type")
}

func TestMemberCallReceiverPreservesDeclaredSiblingAfterTableInsertMutation(t *testing.T) {
	result := Check(`
local methods = {}
local mt = { __index = methods }

type YieldResponse = {}
type YieldChannel = {
    receive: (self: YieldChannel) -> (YieldResponse?, boolean),
}
type NodeInstance = {
    _yield_channel: YieldChannel,
    _queued_commands: {unknown},
}

function methods.yield(self: NodeInstance): ()
    table.insert(self._queued_commands, {})
    self._queued_commands = table.create(10, 0)
    local received, ok = self._yield_channel:receive()
end

function new(): NodeInstance
    local ch: YieldChannel = {
        receive = function(_self: YieldChannel): (YieldResponse?, boolean)
            return nil, false
        end,
    }
    local instance: NodeInstance = {
        _yield_channel = ch,
        _queued_commands = {},
    }
    return setmetatable(instance, mt)
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after queued-command mutation preserves _yield_channel sibling", result.Diagnostics)
	}
}

func TestMemberCallReceiverPreservesDeclaredSiblingAfterDynamicIndexMutation(t *testing.T) {
	result := Check(`
type YieldResponse = {}
type YieldChannel = {
    receive: (self: YieldChannel) -> (YieldResponse?, boolean),
}
type NodeInstance = {
    _yield_channel: YieldChannel,
    _queued_commands: {unknown},
}

local function yield(self: NodeInstance): ()
    self._queued_commands[1] = {}
    self._queued_commands = {}
    local received, ok = self._yield_channel:receive()
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after dynamic sibling mutation preserves _yield_channel", result.Diagnostics)
	}
}

func TestMemberCallReceiverStillReportsOptionalDeclaredField(t *testing.T) {
	src := `
type YieldResponse = {}
type YieldChannel = {
    receive: (self: YieldChannel) -> (YieldResponse?, boolean),
}
type NodeInstance = {
    _yield_channel: YieldChannel?,
}

local function f(self: NodeInstance): ()
    local received, ok = self._yield_channel:receive()
end
	`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeOptionalMethodCall,
		DiagnosticCount: 1,
		Line:            11,
		Column:          26,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 26, EndLine: 11, EndCol: 44},
		MessageContains: []string{
			"cannot call method on an optional value",
			"nil check",
		},
		EvidenceMin: 2,
		EvidenceOrdered: []string{
			"receiver self._yield_channel is optional at call to self._yield_channel.receive",
			"no nil check proves receiver self._yield_channel is present before calling self._yield_channel.receive",
		},
		LabelMin:      1,
		LabelContains: []string{"method call"},
		HelpContains: []string{
			"check self._yield_channel ~= nil",
			"self._yield_channel.receive",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.call.optional_receiver]: cannot call method on an optional value without a nil check",
			"--> test.lua:11:26",
			"11 |     local received, ok = self._yield_channel:receive()",
			"↑ method call",
			"because:",
			"1. proven: receiver self._yield_channel is optional at call to self._yield_channel.receive",
			"2. missing proof: no nil check proves receiver self._yield_channel is present before calling self._yield_channel.receive",
			"help: check self._yield_channel ~= nil before calling self._yield_channel.receive.",
		},
		RenderNotContains: []string{
			"^~",
			"where:",
			"invalid call",
		},
	})
}

func TestMemberCallReceiverOptionalDeclaredFieldGuardIsClean(t *testing.T) {
	result := Check(`
type YieldResponse = {}
type YieldChannel = {
    receive: (self: YieldChannel) -> (YieldResponse?, boolean),
}
type NodeInstance = {
    _yield_channel: YieldChannel?,
}

local function f(self: NodeInstance): ()
    if self._yield_channel then
        local received, ok = self._yield_channel:receive()
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded optional receiver to be clean", result.Diagnostics)
	}
}

func TestMemberCallReceiverUsesDirectAssertedFieldPresenceProof(t *testing.T) {
	result := Check(`
type Stream = {
    read: (self: Stream) -> string,
}
type Obj = {
    stream: Stream?,
}

local function process(obj: Obj): ()
    assert(obj.stream)
    local text: string = obj.stream:read()
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want direct assertion call to prove optional receiver is present", result.Diagnostics)
	}
}

func TestMemberCallReceiverUsesInstalledIndexPresenceProof(t *testing.T) {
	result := Check(`
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

if not messages["root"] then
    messages["root"] = {
        _topic = "root",
        topic = function(self: Message): string
            return self._topic
        end,
    }
end

local installed: string = messages["root"]:topic()
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want installed map index receiver to be proven present", result.Diagnostics)
	}
}

func TestMemberCallReceiverUsesAssertedIndexPresenceProof(t *testing.T) {
	result := Check(`
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

messages["root"] = {
    _topic = "root",
    topic = function(self: Message): string
        return self._topic
    end,
}

assert(messages["root"])
local topic: string = messages["root"]:topic()
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want asserted map index receiver to be proven present", result.Diagnostics)
	}
}

func TestMemberCallReceiverAliasUsesOriginalPathDiscriminantGuard(t *testing.T) {
	result := Check(`
type SpecialData = {
    kind: "special",
    run: (self: SpecialData) -> (),
}
type OtherData = {
    kind: "other",
}
type Data = SpecialData | OtherData
type Obj = {
    data: Data,
}

local function dispatch(obj: Obj): ()
    local sub = obj.data
    if obj.data.kind == "special" then
        sub:run()
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want alias receiver narrowed by original path discriminant guard", result.Diagnostics)
	}
}
