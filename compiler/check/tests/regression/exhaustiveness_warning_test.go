package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestExhaustivenessWarning_DiscriminatedUnionMissingVariant(t *testing.T) {
	source := `
type Message = {kind: "message", text: string}
type Tool = {kind: "tool", name: string}
type Timeout = {kind: "timeout", at: number}
type Event = Message | Tool | Timeout

local function render(event: Event): string
	if event.kind == "message" then
		return event.text
	elseif event.kind == "tool" then
		return event.name
	end
	return "unknown"
end

return render({kind = "message", text = "hi"})
`

	result := testutil.Check(source)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNonExhaustiveWarning(t, result.Diagnostics, "event.kind", `"timeout"`)
}

func TestExhaustivenessWarning_ChannelSelectMissingCase(t *testing.T) {
	source := `
type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number, nsec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>): string
	local result = channel.select {
		events_ch:case_receive(),
		stop_ch:case_receive(),
		timeout_ch:case_receive(),
	}

	if result.channel == events_ch then
		return result.value.kind
	elseif result.channel == stop_ch then
		return result.value.reason
	end
	return "fallback"
end
`

	result := testutil.Check(source, testutil.WithManifest("channel", testutil.ChannelManifest()))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNonExhaustiveWarning(t, result.Diagnostics, "result.channel", "timeout_ch")
}

func TestExhaustivenessWarning_ChannelSelectAllCasesHandled(t *testing.T) {
	source := `
type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number, nsec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>): string
	local result = channel.select {
		events_ch:case_receive(),
		stop_ch:case_receive(),
		timeout_ch:case_receive(),
	}

	if result.channel == events_ch then
		return result.value.kind
	elseif result.channel == stop_ch then
		return result.value.reason
	elseif result.channel == timeout_ch then
		return tostring(result.value.sec)
	end
	return "fallback"
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", testutil.ChannelManifest()))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoNonExhaustiveWarning(t, result.Diagnostics)
}

func TestExhaustivenessWarning_ChannelSelectSingleGuardNoWarning(t *testing.T) {
	source := `
type Event = {kind: string}
type Time = {sec: number, nsec: number}

local function handle(events_ch: Channel<Event>, timeout_ch: Channel<Time>): string
	local result = channel.select {
		events_ch:case_receive(),
		timeout_ch:case_receive(),
	}

	if result.channel == timeout_ch then
		return "timeout"
	end
	return result.value.kind
end
`

	result := testutil.Check(source, testutil.WithManifest("channel", testutil.ChannelManifest()))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoNonExhaustiveWarning(t, result.Diagnostics)
}

func TestExhaustivenessWarning_NoWarningWithElse(t *testing.T) {
	source := `
type Message = {kind: "message", text: string}
type Tool = {kind: "tool", name: string}
type Timeout = {kind: "timeout", at: number}
type Event = Message | Tool | Timeout

local event: Event = {kind = "message", text = "hi"}
if event.kind == "message" then
	return event.text
elseif event.kind == "tool" then
	return event.name
else
	return "timeout"
end
`

	result := testutil.Check(source)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoNonExhaustiveWarning(t, result.Diagnostics)
}

func TestExhaustivenessWarning_NoWarningWhenAllVariantsHandled(t *testing.T) {
	source := `
type Message = {kind: "message", text: string}
type Tool = {kind: "tool", name: string}
type Timeout = {kind: "timeout", at: number}
type Event = Message | Tool | Timeout

local event: Event = {kind = "message", text = "hi"}
if event.kind == "message" then
	return event.text
elseif event.kind == "tool" then
	return event.name
elseif event.kind == "timeout" then
	return tostring(event.at)
end
return "unreachable"
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoNonExhaustiveWarning(t, result.Diagnostics)
}

func TestExhaustivenessWarning_NoWarningForOpenDiscriminant(t *testing.T) {
	source := `
type Message = {kind: string, text: string}
type Tool = {kind: "tool", name: string}
type Timeout = {kind: "timeout", at: number}
type Event = Message | Tool | Timeout

local event: Event = {kind = "message", text = "hi"}
if event.kind == "message" then
	return event.text
elseif event.kind == "tool" then
	return event.name
end
return "open"
`

	result := testutil.Check(source)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoNonExhaustiveWarning(t, result.Diagnostics)
}

func TestExhaustivenessWarning_NumberDiscriminantMissingVariant(t *testing.T) {
	source := `
type One = {case: 1, value: string}
type Two = {case: 2, value: string}
type Three = {case: 3, value: string}
type Token = One | Two | Three

local token: Token = {case = 1, value = "a"}
if token.case == 1 then
	return token.value
elseif token.case == 2 then
	return token.value
end
return "fallback"
`

	result := testutil.Check(source)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNonExhaustiveWarning(t, result.Diagnostics, "token.case", "3")
}

func assertNonExhaustiveWarning(t *testing.T, diags []diag.Diagnostic, parts ...string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity != diag.SeverityWarning || d.Code != diag.ErrNonExhaustive {
			continue
		}
		matches := true
		for _, part := range parts {
			if !strings.Contains(d.Message, part) {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("expected non-exhaustive warning containing %v, got diagnostics: %v", parts, diags)
}

func assertNoNonExhaustiveWarning(t *testing.T, diags []diag.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == diag.SeverityWarning && d.Code == diag.ErrNonExhaustive {
			t.Fatalf("unexpected non-exhaustive warning: %s", d.Message)
		}
	}
}
