package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestImportedChannelSelectPayloadReportsBranchFieldMismatch(t *testing.T) {
	protocol := CheckAndExport(`
type Event = { kind: "event", id: string, attempt: number }
type Timer = { kind: "timer", elapsed: number }
type Stop = { kind: "stop", reason: string }
type Source = {
	primary: Channel<Event>,
	timers: Channel<Timer>,
	stops: Channel<Stop>,
}
local M = {}
return M
`, "protocol", WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}

	result := Check(`
local protocol = require("protocol")

function consume(source: protocol.Source)
	local result = channel.select {
		source.primary:case_receive(),
		source.timers:case_receive(),
		source.stops:case_receive(),
	}
	if result.channel == source.primary then
		local event = result.value
		local wrong: number = event.id
	end
	if result.channel == source.timers then
		local timer = result.value
		local wrong: string = timer.elapsed
	end
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"), WithModule("protocol", protocol))

	messages := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	if !hasDiagnosticMessage(messages, "cannot assign event.id because it is string, not number") ||
		!hasDiagnosticMessage(messages, "cannot assign timer.elapsed because it is number, not string") {
		t.Fatalf("diagnostics = %#v, want imported channel-select payload mismatches", messages)
	}
}

func TestChannelSelectResultRuntimeStatusFieldsAreReadable(t *testing.T) {
	result := Check(`
local function receive_once(ch: Channel<string>): ()
	local selected = channel.select {
		ch:case_receive(),
	}
	local ok: boolean = selected.ok
	local defaulted: nil = selected.default
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want select receive result to expose ok and nil default fields", result.Diagnostics)
	}
}

func TestChannelSelectDefaultResultRuntimeStatusFieldsAreReadable(t *testing.T) {
	result := Check(`
local function receive_or_default(ch: Channel<string>): ()
	local selected = channel.select {
		ch:case_receive(),
		default = true,
	}
	local ok: boolean = selected.ok
	local defaulted: boolean? = selected.default
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want default-capable select result to expose ok and default fields", result.Diagnostics)
	}
}

func TestGenericChannelListenRejectsConflictingObjectEvidence(t *testing.T) {
	protocol := CheckAndExport(`
type Event = { kind: "event", id: string }
type Timer = { kind: "timer", elapsed: number }
type Source = {
	primary: Channel<Event>,
}
local M = {}
return M
`, "protocol", WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}

	src := strings.TrimLeft(`
local protocol = require("protocol")

type ListenOptions<T> = {
	channel: Channel<T>,
	decode: (any) -> T,
}

local function listen<T>(topic: string, options: ListenOptions<T>): Channel<T>
	return options.channel
end

local source: { primary: Channel<protocol.Event> }
local function decode_timer(raw: any): protocol.Timer
	return { kind = "timer", elapsed = 1 }
end

function consume(source: protocol.Source)
	local wrong_typed_events: Channel<protocol.Event> = listen("events", {
	channel = source.primary,
	decode = decode_timer,
	})
end
`, "\n")
	result := Check(src, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"), WithModule("protocol", protocol))

	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.argument_type]: listen cannot infer one T for argument 2: argument 2.channel implies {id: string, kind: "event"}, but argument 2.decode return 1 implies {elapsed: number, kind: "timer"}
 --> test.lua:19:12
   |
19 |     channel = source.primary,
   |               ↑ argument value

because:
  1. claimed: listen parameter 2 requires one consistent T across this argument
  2. proven: listen inferred T includes {id: string, kind: "event"} from argument 2.channel
  3. proven: listen inferred T includes {elapsed: number, kind: "timer"} from argument 2.decode return 1
 --> test.lua:20:11
   |
20 |     decode = decode_timer,
   |              ^

help: Make each use of ` + "`T`" + ` in this argument agree on the same type, or split the callee signature into separate type parameters if those values are intentionally different.`
	assertRenderedEqual(t, rendered, want)
}

func TestUntypedSpawnInlineCallbackPreservesCapturedChannelPayload(t *testing.T) {
	result := Check(`
type Event = { kind: "event", id: string, attempt: number }
type Source = {
	primary: Channel<Event>,
}

local function consume(source: Source)
	coroutine.spawn(function()
		local event, ok = source.primary:receive()
		if ok then
			local id: string = event.id
			local wrong: number = event.id
			print(id, wrong)
		end
	end)
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel", "coroutine"))

	messages := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	if len(messages) != 1 || !hasDiagnosticMessage(messages, "cannot assign event.id because it is string, not number") {
		t.Fatalf("diagnostics = %#v, want only captured channel payload string mismatch", messages)
	}
}

func TestChannelSelectDiscriminantUnlocksReceiverMethods(t *testing.T) {
	result := Check(`
type Leaf = { kind: "leaf", id: string }
type Deadline = { kind: "deadline", tick: number }
type RouteA = { kind: "route_a", ch: Channel<Leaf> }
type RouteB = { kind: "route_b", ch: Channel<Deadline> }
type Box = {
	kind: "box",
	node: {
		left: Channel<RouteA | RouteB>,
		right: Channel<Leaf | Deadline>,
		next: Box | Other,
	},
}
type Other = { kind: "other", reason: string }
type Event = Box | Other

function consume(events: Channel<Event>)
	local selected = channel.select {
		events:case_receive(),
	}
	local payload = selected.value
	if payload.kind == "other" then
		return payload.reason
	end
	if payload.kind == "box" then
		local boxed = channel.select {
			payload.node.left:case_receive(),
			payload.node.right:case_receive(),
		}
		if boxed.channel == payload.node.left then
			local route = boxed.value
			if route.kind == "route_a" then
				return route.kind
			end
		end
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestChannelSelectEarlyReturnDeadlineNarrowsRemainingPayload(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }
type Deadline = { elapsed: number }

local function wait_for_topic(inbox: Channel<Message>, deadline: Channel<Deadline>): (Message?, string?)
    local result = channel.select {
        inbox:case_receive(),
        deadline:case_receive(),
    }
    if result.channel == deadline then
        return nil, "timeout waiting for message"
    end
    local msg = result.value
    if msg:topic() == "ack" then
        return msg, nil
    end
    return nil, "wrong topic"
end

local function main(inbox: Channel<Message>, deadline: Channel<Deadline>): string
    local msg, err = wait_for_topic(inbox, deadline)
    if err then
        return err
    end
    if msg == nil then
        error("missing message")
    end
    return msg:topic()
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want early-return deadline branch to narrow remaining select payload to Message", result.Diagnostics)
	}
}

func TestChannelSelectEarlyReturnUntypedDeadlineKeepsKnownPayload(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }

local function wait_for_topic(inbox: Channel<Message>, deadline: unknown): (Message?, string?)
    local result = channel.select {
        inbox:case_receive(),
        deadline:case_receive(),
    }
    if result.channel == deadline then
        return nil, "timeout waiting for message"
    end
    local msg = result.value
    if msg:topic() == "ack" then
        return msg, nil
    end
    return nil, "wrong topic"
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want known channel payload to survive an untyped deadline case", result.Diagnostics)
	}
}

func TestChannelSelectUnannotatedHelperReturnKeepsKnownPayload(t *testing.T) {
	result := Check(`
type Message = { topic: (self: Message) -> string }

local function payload_data(msg: Message): string
    return msg:topic()
end

local function wait_for_topic(inbox: Channel<Message>, deadline: unknown)
    while true do
        local result = channel.select {
            inbox:case_receive(),
            deadline:case_receive(),
        }
        if result.channel == deadline then
            return nil, "timeout waiting for message"
        end
        local msg = result.value as Message
        if msg:topic() == "ack" then
            return msg, nil
        end
    end
end

local function main(inbox: Channel<Message>, deadline: unknown): string
    local msg, err = wait_for_topic(inbox, deadline)
    if err then
        return err
    end
    if msg == nil then
        error("missing message")
    end
    return payload_data(msg as Message)
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated select helper to infer Message return", result.Diagnostics)
	}
}

func TestChannelSelectExhaustivenessReportsMissingReceiveCase(t *testing.T) {
	src := strings.TrimLeft(`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`, "\n")
	result := Check(src, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	var warnings []diagnostic.Diagnostic
	for _, d := range result.Diagnostics {
		if d.Severity == diagnostic.SeverityWarning {
			warnings = append(warnings, d)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %d, want 1: %#v", len(warnings), result.Diagnostics)
	}
	warning := warnings[0]
	if warning.Code != diagnostics.CodeChannelSelectExhaustive {
		t.Fatalf("warning code = %s, want %s", warning.Code, diagnostics.CodeChannelSelectExhaustive)
	}
	if warning.Message != "channel select is not exhaustive; missing case: `stops`" {
		t.Fatalf("warning message = %q, want missing channel display name", warning.Message)
	}
	if warning.Severity != diagnostic.SeverityWarning {
		t.Fatalf("warning severity = %s, want %s", warning.Severity, diagnostic.SeverityWarning)
	}
	requireLabelMessage(t, warning, "channel case check")
	if warning.Help != "Add an elseif branch for each missing case, or add a default branch when a fallback is valid." {
		t.Fatalf("warning help = %q", warning.Help)
	}
	evidence := warning.Explanation.Evidence()
	if len(evidence) != 4 {
		t.Fatalf("warning explanation evidence = %#v, want 4 precise items", evidence)
	}
	for i, item := range evidence {
		if i < 2 {
			if item.Kind != diagnostic.EvidenceAbstractFact || item.Trust != diagnostic.TrustProven {
				t.Fatalf("warning explanation evidence[%d] = %#v, want proven abstract fact", i, item)
			}
			continue
		}
		if item.Kind != diagnostic.EvidenceMissingProof || item.Trust != diagnostic.TrustUnknown {
			t.Fatalf("warning explanation evidence[%d] = %#v, want missing-proof evidence", i, item)
		}
	}
	explanation := warning.Explanation.String()
	for _, want := range []string{
		"branch chain checks channel `selected.channel`",
		"handled cases: `primary`, `timers`",
		"missing cases: `stops`",
		"no default case handles the remaining channel cases",
	} {
		if !strings.Contains(explanation, want) {
			t.Fatalf("warning explanation = %q, want substring %q", explanation, want)
		}
	}
	rendered := diagnostic.Render(warning, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[channel.select.exhaustiveness]: channel select is not exhaustive; missing case: ` + "`stops`" + `
 --> test.lua:10:5
   |
10 |     if selected.channel == primary then
   |        ↑ channel case check

because:
  1. proven: branch chain checks channel ` + "`selected.channel`" + `
  2. proven: handled cases: ` + "`primary`, `timers`" + `
  3. missing proof: missing cases: ` + "`stops`" + `
  4. missing proof: no default case handles the remaining channel cases

help: Add an elseif branch for each missing case, or add a default branch when a fallback is valid.`
	assertRenderedEqual(t, rendered, want)
}

func TestChannelSelectExhaustivenessAcceptsExhaustiveChain(t *testing.T) {
	result := Check(`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	elseif selected.channel == stops then
		return selected.value.reason
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if hasChannelSelectExhaustivenessWarning(result.Diagnostics) {
		t.Fatalf("diagnostics = %#v, want no channel select exhaustiveness warning", result.Diagnostics)
	}
}

func TestChannelSelectExhaustivenessUsesLatestReassignedResultPath(t *testing.T) {
	result := Check(`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
	}
	selected = channel.select {
		primary:case_receive(),
		stops:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == stops then
		return selected.value.reason
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if hasChannelSelectExhaustivenessWarning(result.Diagnostics) {
		t.Fatalf("diagnostics = %#v, want no channel select exhaustiveness warning for reassigned select result", result.Diagnostics)
	}
}

func TestChannelSelectExhaustivenessAcceptsDuplicateSameChannelReceiveCase(t *testing.T) {
	result := Check(`
type Event = { id: string }
type Timer = { elapsed: number }
function consume(primary: Channel<Event>, timers: Channel<Timer>)
	local selected = channel.select {
		primary:case_receive(),
		primary:case_receive(),
		timers:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if hasChannelSelectExhaustivenessWarning(result.Diagnostics) {
		t.Fatalf("diagnostics = %#v, want no channel select exhaustiveness warning for duplicate same-channel receive cases", result.Diagnostics)
	}
}

func TestChannelSelectExhaustivenessAcceptsDefaultCase(t *testing.T) {
	result := Check(`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
		default = true,
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

	if hasChannelSelectExhaustivenessWarning(result.Diagnostics) {
		t.Fatalf("diagnostics = %#v, want no channel select exhaustiveness warning with default case", result.Diagnostics)
	}
}

func hasDiagnosticMessage(messages []string, want string) bool {
	for _, message := range messages {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

func hasChannelSelectExhaustivenessWarning(diags []diagnostic.Diagnostic) bool {
	for _, d := range diags {
		if d.Code == diagnostics.CodeChannelSelectExhaustive && d.Severity == diagnostic.SeverityWarning {
			return true
		}
	}
	return false
}
