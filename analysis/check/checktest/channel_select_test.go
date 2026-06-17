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
	if !hasDiagnosticMessage(messages, "cannot assign string to number") ||
		!hasDiagnosticMessage(messages, "cannot assign number to string") {
		t.Fatalf("diagnostics = %#v, want imported channel-select payload mismatches", messages)
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

	result := Check(`
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
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"), WithModule("protocol", protocol))

	messages := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	if !hasDiagnosticMessage(messages, "argument 2 is") {
		t.Fatalf("diagnostics = %#v, want generic object evidence conflict", messages)
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

func TestChannelSelectExhaustivenessReportsMissingReceiveCase(t *testing.T) {
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
	end
	return ""
end
`, WithStdlib(), WithManifest("channel", ChannelManifest()), WithGlobals("channel"))

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
	if !strings.Contains(warning.Message, "stops") {
		t.Fatalf("warning message = %q, want missing channel display name", warning.Message)
	}
	if warning.Severity != diagnostic.SeverityWarning {
		t.Fatalf("warning severity = %s, want %s", warning.Severity, diagnostic.SeverityWarning)
	}
	if len(warning.Labels) != 1 || warning.Labels[0].Message != "channel select case chain" {
		t.Fatalf("warning labels = %#v, want channel select case chain label", warning.Labels)
	}
	if warning.Help != "Handle each channel select case explicitly in the if/elseif chain." {
		t.Fatalf("warning help = %q", warning.Help)
	}
	evidence := warning.Explanation.Evidence()
	if len(evidence) != 4 {
		t.Fatalf("warning explanation evidence = %#v, want 4 precise items", evidence)
	}
	explanation := warning.Explanation.String()
	for _, want := range []string{
		"selected.channel",
		"handled channel select cases: primary, timers",
		"missing channel select cases: stops",
		"no default case",
	} {
		if !strings.Contains(explanation, want) {
			t.Fatalf("warning explanation = %q, want substring %q", explanation, want)
		}
	}
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
