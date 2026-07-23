package factflow

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCallSiteOwnsDirectCalleeSource(t *testing.T) {
	shape, ok := NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("valid scalar source shape rejected")
	}
	want, ok := NewPathValueSource(pathdom.NewPath(symbol.ID(41), "callee").Key(), 0, NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("valid callee path source rejected")
	}
	point := cfg.Point(7)
	site := NewCallSite(CallSiteConfig{CalleeSource: want, HasCalleeSource: true})
	input := map[cfg.Point]CallSite{point: site}
	facts := NewFacts(FactsInput{CallSites: input})

	site.calleeSource = NewUnknownValueSource(NoValueSourceIndex)
	input[point] = site
	view, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("call site %d missing from owned facts", point)
	}
	got, ok := view.CalleeSource()
	if !ok || !ValueSourceEqual(got, want) {
		t.Fatalf("callee source = %#v/%v, want owned %#v/true", got, ok, want)
	}
	got.Kind = ValueSourceUnknown
	again, _ := facts.CallSiteView(point)
	if source, ok := again.CalleeSource(); !ok || !ValueSourceEqual(source, want) {
		t.Fatalf("callee source view exposed storage: %#v/%v", source, ok)
	}
}

func TestCallSiteMethodDoesNotClaimDirectCalleeSource(t *testing.T) {
	receiver := NewUnknownValueSource(0)
	site := NewCallSite(CallSiteConfig{
		MethodName:        "run",
		ReceiverSource:    receiver,
		HasReceiverSource: true,
	}).View()
	if source, ok := site.CalleeSource(); ok || source != (ValueSource{}) {
		t.Fatalf("method callee source = %#v/%v, want zero/false", source, ok)
	}
	if source, ok := site.ReceiverSource(); !ok || !ValueSourceEqual(source, receiver) {
		t.Fatalf("method receiver source = %#v/%v, want %#v/true", source, ok, receiver)
	}
}
