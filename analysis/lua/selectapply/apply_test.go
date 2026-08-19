package selectapply

import (
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/domain/type/ambient"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestApplySpecializesChannelSelectAndIgnoresLookalike(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>)
    local result = channel.select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
        stop_ch:case_receive(),
        timeout_ch:case_receive(),
    }
    return result
end
`)
	apps := Apply(p)
	if len(apps) != 1 {
		t.Fatalf("Apply = %d applications, want 1", len(apps))
	}
	site, siteOK := p.CallIDAt(apps[0].Index)
	if !siteOK || site != apps[0].Site || !apps[0].Site.Available() {
		t.Fatalf("application site = %v/%v, CallIDAt(%d) = %v/%v", apps[0].Site, apps[0].Site.Available(), apps[0].Index, site, siteOK)
	}
	facts := apps[0].Facts
	if _, ok := facts.Lookup(apps[0].Site, 0); !ok {
		t.Fatal("events_ch arm was not admitted")
	}
	if _, ok := facts.Lookup(apps[0].Site, 1); ok {
		t.Fatal("lookalike table member was admitted as a case")
	}
	if _, ok := facts.Lookup(apps[0].Site, 2); !ok {
		t.Fatal("stop_ch arm was not admitted")
	}
	if _, ok := facts.Lookup(apps[0].Site, 3); !ok {
		t.Fatal("timeout_ch arm was not admitted")
	}
	events := typ.Instantiate(ambient.ChannelGeneric(), typ.NewRef("", "Event"))
	if _, ok := channelselect.ResultCaseTypeFromValue(apps[0].Result, channelselect.ResultCaseType(events, typ.NewRef("", "Event"))); !ok {
		t.Fatalf("specialized result lost the events arm: %v", apps[0].Result)
	}
	eventsFact, ok := facts.Lookup(apps[0].Site, 0)
	if !ok {
		t.Fatal("events fact missing after admit")
	}
	narrowed, ok := channelselect.ResultWithoutFact(apps[0].Result, eventsFact)
	if !ok {
		t.Fatal("ResultWithoutFact refused the events arm")
	}
	if _, still := channelselect.ResultCaseTypeFromValue(narrowed, channelselect.ResultCaseType(events, typ.NewRef("", "Event"))); still {
		t.Fatal("removing the events fact left the events member")
	}
	stop := typ.Instantiate(ambient.ChannelGeneric(), typ.NewRef("", "Stop"))
	if _, ok := channelselect.ResultCaseTypeFromValue(narrowed, channelselect.ResultCaseType(stop, typ.NewRef("", "Stop"))); !ok {
		t.Fatal("removing the events fact collapsed a live stop arm")
	}
}

func TestApplyIgnoresUserSelectAndLookalikeOnlyTable(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}

local function select(cases) end

local function handle(events_ch: Channel<Event>)
    select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
    }
    channel.select {
        { channel = events_ch, value = 1, ok = true, default = nil },
    }
end
`)
	if apps := Apply(p); len(apps) != 0 {
		t.Fatalf("Apply admitted %d applications from user select / lookalike-only table: %+v", len(apps), apps)
	}
}

func TestApplyRequiresChannelModule(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}

local other = {}
function other.select(cases) end

local function handle(events_ch: Channel<Event>)
    other.select {
        events_ch:case_receive(),
    }
end
`)
	if apps := Apply(p); len(apps) != 0 {
		t.Fatalf("Apply admitted %d applications from other.select: %+v", len(apps), apps)
	}
}

func TestApplyUsesCallIDAsSiteForEachSelect(t *testing.T) {
	p := lowerProgram(t, `
type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>)
    channel.select { events_ch:case_receive() }
    channel.select { stop_ch:case_receive() }
end
`)
	apps := Apply(p)
	if len(apps) != 2 {
		t.Fatalf("Apply = %d applications, want 2", len(apps))
	}
	if apps[0].Site == apps[1].Site {
		t.Fatal("two select calls shared a site identity")
	}
	if _, ok := apps[0].Facts.Lookup(apps[0].Site, 0); !ok {
		t.Fatal("first select admitted no arm")
	}
	if _, ok := apps[1].Facts.Lookup(apps[1].Site, 0); !ok {
		t.Fatal("second select admitted no arm")
	}
	if _, ok := apps[0].Facts.Lookup(apps[1].Site, 0); ok {
		t.Fatal("first select admitted the second site")
	}
}

func lowerProgram(t *testing.T, src string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "select.lua", Text: []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
