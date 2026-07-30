package effectlowering

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestPreparedSignatureProducerReusesCanonicalOutcomeAndSignature(t *testing.T) {
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
	wantProgram := SignatureOutcomeProvider(config)
	gotProgram := producer.OutcomeProvider()
	if !reflect.DeepEqual(testPrepareCallOutcome(t, wantProgram, ctx, site).Capability().FieldRoles(), testPrepareCallOutcome(t, gotProgram, ctx, site).Capability().FieldRoles()) {
		t.Fatalf("prepared signature capability differs")
	}
	sig, ok := producer.SignatureForSite(ctx, site)
	if !ok || sig.Type == nil {
		t.Fatalf("prepared signature=%#v/%v", sig, ok)
	}
}
