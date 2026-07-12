package effectlowering

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPreparedSignatureProducerReusesCanonicalOutcomeAndIteration(t *testing.T) {
	reg := standard.Registry()
	config := SignatureOutcomeProviderConfig{
		Signatures:  signaturelookup.Source{IncludeStdlib: true},
		NameForSite: func(transfer.NodeContext, factflow.CallSiteView) (string, bool) { return "ipairs", true },
	}
	producer := PrepareSignatureProducer(config)
	if producer == nil {
		t.Fatal("producer missing")
	}
	ctx := transfer.NodeContext{Registry: reg}
	site := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
	want := SignatureOutcomeProvider(config)(ctx, site, state.State{}, nil)
	got := producer.OutcomeProvider()(ctx, site, state.State{}, nil)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("prepared signature outcome differs\nwant=%#v\n got=%#v", want, got)
	}
	iter, ok := producer.IteratorForSite(ctx, site)
	if !ok || iter.Source.Index != 0 {
		t.Fatalf("prepared iterator=%#v/%v", iter, ok)
	}
}
