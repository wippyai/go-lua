package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

const selectCompositionProtocol = `
type Event = { kind: "event", id: string, attempt: number }
type Timer = { kind: "timer", elapsed: number }
type Stop = { kind: "stop", reason: string }
type Source = {
    primary: Channel<Event>,
    retry: Channel<Event>,
    timers: Channel<Timer>,
    stops: Channel<Stop>,
}
`

// TestSelectArmConstraintsComposeAcrossSequentialGuards proves that the arm
// sets published by successive `result.channel == ch` edges compose. Every
// guard after the first sees both its own true edge and the earlier false
// edges, so the payload must be the arm those edges share.
func TestSelectArmConstraintsComposeAcrossSequentialGuards(t *testing.T) {
	result, err := engine.Check(selectCompositionProtocol + `
local function select_source(primary: Channel<Event>, retry: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>): string
    local result = channel.select {
        primary:case_receive(),
        retry:case_receive(),
        timers:case_receive(),
        stops:case_receive(),
    }

    if result.channel == primary then
        local event = result.value
        local id: string = event.id
        return id
    end

    if result.channel == retry then
        local event = result.value
        local attempt: number = event.attempt
        return tostring(attempt)
    end

    if result.channel == timers then
        local timer = result.value
        local elapsed: number = timer.elapsed
        return tostring(elapsed)
    end

    local stop = result.value
    local reason: string = stop.reason
    return reason
end
return select_source
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.member.missing" {
			t.Fatalf("composed select arms lost their payload narrowing: %s at %v: %s", diagnostic.Code, diagnostic.Span, diagnostic.Message)
		}
	}
}

// TestSelectPayloadRefutesWrongArmAssignment keeps the composed narrowing
// falsifiable: the third guard's payload is the stop arm, so assigning it to a
// number is still refuted.
func TestSelectPayloadRefutesWrongArmAssignment(t *testing.T) {
	result, err := engine.Check(selectCompositionProtocol + `
local function select_source(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>): string
    local result = channel.select {
        primary:case_receive(),
        timers:case_receive(),
        stops:case_receive(),
    }

    if result.channel == primary then
        local event = result.value
        return event.id
    end

    if result.channel == timers then
        local timer = result.value
        return tostring(timer.elapsed)
    end

    local stop = result.value
    local wrong: number = stop.reason
    return stop.reason
end
return select_source
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "stop.reason") {
		t.Fatalf("composed select arm did not refute the wrong assignment: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
}

// TestDeclaredRecordMemberChannelSelectNarrowsPayload proves the declared
// boundary publishes a channel payload witness below its root, so a select
// over `source.primary` resolves its arms instead of failing closed.
func TestDeclaredRecordMemberChannelSelectNarrowsPayload(t *testing.T) {
	result, err := engine.Check(selectCompositionProtocol + `
local function consume(source: Source): string
    local result = channel.select {
        source.primary:case_receive(),
        source.timers:case_receive(),
        source.stops:case_receive(),
    }

    if result.channel == source.primary then
        local event = result.value
        local wrong: number = event.id
        return event.id
    end

    if result.channel == source.timers then
        local timer = result.value
        local wrong: string = timer.elapsed
        return tostring(timer.elapsed)
    end

    return "x"
end
return consume
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "event.id") {
		t.Fatalf("member-path select arm lost its event payload: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "timer.elapsed") {
		t.Fatalf("member-path select arm lost its timer payload: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
}

// TestDeclaredChannelReceivePublishesPayloadResult proves the ambient channel
// contract reaches `receive` results: the payload witness carried by the
// declared boundary is the receive result's authority.
func TestDeclaredChannelReceivePublishesPayloadResult(t *testing.T) {
	result, err := engine.Check(selectCompositionProtocol + `
local function consume(source: Source): string
    local selected = channel.select { source.primary:case_receive() }
    local event, ok = source.retry:receive()
    if ok then
        local wrong: number = event.id
        return event.id
    end
    return "x"
end
return consume
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "event.id") {
		t.Fatalf("channel receive did not publish its declared payload: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
}

// TestDeclarationBoundaryAdmitsClosedStdlibCall proves a published
// standard-library call over terms the declaration entry already closes no
// longer leaves the whole body dormant.
func TestDeclarationBoundaryAdmitsClosedStdlibCall(t *testing.T) {
	result, err := engine.Check(`
type Audit = { kind: "audit", audit: { id: string } }
type Metric = { kind: "metric", metric: { value: number } }
type Leaf = Audit | Metric

local function read_leaf(leaf: Leaf): string
    if leaf.kind == "audit" then
        local wrong: number = leaf.audit.id
        return leaf.audit.id
    end
    local value: number = leaf.metric.value
    return tostring(value)
end
return read_leaf
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "leaf.audit.id") {
		t.Fatalf("declaration boundary stayed dormant behind tostring: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
}

func hasAssignmentRefutation(diagnostics []engine.PublishedDiagnostic, subject string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "type.assignment" && strings.Contains(diagnostic.Message, subject) {
			return true
		}
	}
	return false
}

func summarizeDiagnostics(diagnostics []engine.PublishedDiagnostic) []string {
	out := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, string(diagnostic.Code)+" "+diagnostic.Message)
	}
	return out
}

// TestSelectChildTransportsCapturedClosureHandle proves the select-only
// publication pass carries a captured local function as a capability. Without
// its handle the callee is opaque inside the select body and every obligation
// the callee owns is lost at that call site.
func TestSelectChildTransportsCapturedClosureHandle(t *testing.T) {
	result, err := engine.Check(selectCompositionProtocol + `
local function label(event: Event)
    return event.id
end

local function consume(primary: Channel<Event>, stops: Channel<Stop>): string
    local selected = channel.select {
        primary:case_receive(),
        stops:case_receive(),
    }
    if selected.channel == primary then
        local event = selected.value
        local wrong: number = label(event)
        return label(event)
    end
    return "x"
end
return consume
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !hasAssignmentRefutation(result.PublishedDiagnostics, "string, not number") {
		t.Fatalf("captured callee stayed opaque inside the select body: %#v", summarizeDiagnostics(result.PublishedDiagnostics))
	}
}
