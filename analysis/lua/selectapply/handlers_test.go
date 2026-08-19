package selectapply

import (
	"testing"
)

func TestHandlersNamesHandledChannelEqualsOnSelectResult(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number}

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
    return "timeout"
end
`)
	apps := Apply(p)
	if len(apps) != 1 {
		t.Fatalf("apps=%d", len(apps))
	}
	handlers := Handlers(p, apps)
	if len(handlers) != 1 {
		t.Fatalf("handlers=%d", len(handlers))
	}
	handler := handlers[0]
	if handler.Site != apps[0].Site || handler.Result != "result" || handler.SelectDefault || handler.ElseDefault {
		t.Fatalf("handler site/default = %+v", handler)
	}
	if handler.Names[2] != "timeout_ch" {
		t.Fatalf("timeout name = %q", handler.Names[2])
	}
	if len(handler.Handled) != 2 || handler.Handled[0] != 0 || handler.Handled[1] != 1 {
		t.Fatalf("handled = %v, want [0 1]", handler.Handled)
	}
	if handler.Names[0] != "events_ch" || handler.Names[1] != "stop_ch" {
		t.Fatalf("names = %v", handler.Names)
	}
	if handler.Location.StartLine != 12 || handler.Location.StartCol != 8 {
		t.Fatalf("location = %d:%d, want 12:8", handler.Location.StartLine, handler.Location.StartCol)
	}
	missing := apps[0].Facts.MissingArms(handler.Site, handler.Handled, handler.SelectDefault || handler.ElseDefault)
	if len(missing) != 1 || missing[0].Ordinal != 2 {
		t.Fatalf("missing = %+v, want timeout ordinal 2", missing)
	}
}

func TestHandlersTreatsSelectDefaultAsCovering(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>): string
    local result = channel.select {
        events_ch:case_receive(),
        stop_ch:case_receive(),
        default = true,
    }
    if result.channel == events_ch then
        return "e"
    end
    return "d"
end
`)
	apps := Apply(p)
	handlers := Handlers(p, apps)
	if len(handlers) != 1 || !handlers[0].SelectDefault {
		t.Fatalf("handlers = %+v", handlers)
	}
}

func TestHandlersTreatsElseAsCoveringRemaining(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>): string
    local result = channel.select {
        events_ch:case_receive(),
        stop_ch:case_receive(),
    }
    if result.channel == events_ch then
        return "e"
    else
        return "s"
    end
end
`)
	apps := Apply(p)
	handlers := Handlers(p, apps)
	if len(handlers) != 1 || !handlers[0].ElseDefault || handlers[0].SelectDefault {
		t.Fatalf("handlers = %+v", handlers)
	}
	if len(handlers[0].Handled) != 1 || handlers[0].Handled[0] != 0 {
		t.Fatalf("handled = %v, want [0]", handlers[0].Handled)
	}
	if missing := apps[0].Facts.MissingArms(handlers[0].Site, handlers[0].Handled, handlers[0].SelectDefault || handlers[0].ElseDefault); len(missing) != 0 {
		t.Fatalf("else-covered select still missing %v", missing)
	}
}
