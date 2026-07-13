package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// SignatureProducer is an immutable prepared view over canonical signature and
// effect lowering. OutcomeProvider and iterator projection share the same name
// resolution and signature lookup; neither re-encodes stdlib names or effects.
type SignatureProducer struct {
	outcome          callpayload.CallOutcomeProvider
	lookup           func(string) (signature.Function, bool)
	nameFor          SignatureNameFunc
	nameForSite      SignatureSiteNameFunc
	intrinsicForSite SignatureSiteIntrinsicFunc
}

func PrepareSignatureProducer(config SignatureOutcomeProviderConfig) *SignatureProducer {
	if config.Signatures == nil {
		return nil
	}
	lookup := config.Signatures.Lookup
	if views, ok := config.Signatures.(immutableSignatureLookup); ok {
		lookup = views.LookupView
	}
	return &SignatureProducer{
		outcome: SignatureOutcomeProvider(config), lookup: lookup,
		nameFor: config.NameFor, nameForSite: config.NameForSite,
		intrinsicForSite: config.IntrinsicForSite,
	}
}

// IntrinsicForSite returns a sealed semantic identity supplied by the lexical
// binding authority. SignatureProducer transports the identity but does not
// infer it from a resolved signature name.
func (p *SignatureProducer) IntrinsicForSite(ctx transfer.NodeContext, site factflow.CallSiteView) (signature.Intrinsic, bool) {
	if p == nil || p.intrinsicForSite == nil {
		return signature.IntrinsicNone, false
	}
	intrinsic, ok := p.intrinsicForSite(ctx, site)
	return intrinsic, ok && intrinsic.Valid()
}

func (p *SignatureProducer) OutcomeProvider() callpayload.CallOutcomeProvider {
	if p == nil {
		return nil
	}
	return p.outcome
}

// SignatureForSite returns an owned immutable producer descriptor selected by
// the same resolver as SignatureOutcomeProvider. Dynamic/unresolved sites fail
// closed. LookupView-backed sources are cloned at this public ownership edge.
func (p *SignatureProducer) SignatureForSite(ctx transfer.NodeContext, site factflow.CallSiteView) (signature.Function, bool) {
	if p == nil || p.lookup == nil {
		return signature.Function{}, false
	}
	name, ok := signatureNameForSite(ctx, site, p.nameForSite, p.nameFor)
	if !ok {
		return signature.Function{}, false
	}
	got, ok := p.lookup(name)
	if !ok {
		return signature.Function{}, false
	}
	return got.Clone(), true
}

// LookupStringMethodSignature performs canonical lookup only. It grants no
// receiver or call-shape authority; consumers must independently prove both
// and must apply their own effect/composability gate.
func (p *SignatureProducer) LookupStringMethodSignature(method string) (signature.Function, bool) {
	if p == nil || p.lookup == nil || method == "" {
		return signature.Function{}, false
	}
	got, ok := p.lookup("string." + method)
	if !ok {
		return signature.Function{}, false
	}
	return got.Clone(), true
}

func (p *SignatureProducer) IteratorForSite(ctx transfer.NodeContext, site factflow.CallSiteView) (iteration.Iterator, bool) {
	sig, ok := p.SignatureForSite(ctx, site)
	if !ok {
		return iteration.Iterator{}, false
	}
	return iteration.ActiveIterator(sig.Effect.Labels)
}
