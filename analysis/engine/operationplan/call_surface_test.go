package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallSurfaceSealsCanonicalImmutableCensus(t *testing.T) {
	owner := callSurfaceBody("owner", 1)
	callee := callSurfaceBody("owner", 2)
	lexical, ok := NewLexicalCallSurfaceTarget(callee)
	if !ok {
		t.Fatal("lexical target rejected")
	}
	external := callSurfaceExternal(t, false)
	input := []CallSurfaceSite{
		{Point: 7, Target: RejectedCallSurfaceTarget()},
		{Point: 1, Target: lexical},
		{Point: 4, Target: external},
	}
	extracted := []cfg.Point{4, 7, 1}
	surface, err := SealCallSurface(owner, 9, extracted, input)
	if err != nil {
		t.Fatalf("SealCallSurface: %v", err)
	}
	input[0] = CallSurfaceSite{}
	extracted[0] = 8
	if !surface.Complete() || !surface.Digest().Available() || surface.Owner() != owner || surface.PointCount() != 9 {
		t.Fatalf("invalid sealed surface: %#v", surface)
	}
	sites := surface.Sites()
	if len(sites) != 3 || sites[0].Point != 1 || sites[1].Point != 4 || sites[2].Point != 7 {
		t.Fatalf("sites are not canonical: %#v", sites)
	}
	sites[0] = CallSurfaceSite{}
	if got, ok := surface.Site(1); !ok || got.Point != 1 || got.Target.Kind() != CallSurfaceTargetLexical {
		t.Fatalf("surface storage escaped: %#v/%v", got, ok)
	}
	if _, ok := surface.Site(2); ok {
		t.Fatal("published absent point")
	}

	reordered, err := SealCallSurface(owner, 9, []cfg.Point{7, 1, 4}, []CallSurfaceSite{inputSite(4, external), inputSite(7, RejectedCallSurfaceTarget()), inputSite(1, lexical)})
	if err != nil {
		t.Fatalf("SealCallSurface reordered: %v", err)
	}
	if surface.Digest() != reordered.Digest() {
		t.Fatal("input order changed canonical digest")
	}
}

func TestCallSurfaceClassifiesSealedLuaTypeAsExternal(t *testing.T) {
	target := callSurfaceExternal(t, true)
	if target.Kind() != CallSurfaceTargetExternal {
		t.Fatalf("target kind = %d", target.Kind())
	}
	if _, ok := target.LexicalBody(); ok {
		t.Fatal("external target published a lexical body")
	}
	if content, ok := target.ExternalContentID(); !ok || !content.Available() {
		t.Fatal("external target lost canonical signature identity")
	}
	operation, ok := target.ExternalOperation()
	if !ok {
		t.Fatal("external operation unavailable")
	}
	intrinsic, ok := operation.Intrinsic()
	if !ok || intrinsic != signature.IntrinsicLuaType {
		t.Fatalf("intrinsic = %d/%v", intrinsic, ok)
	}
}

func TestCallSurfaceFailsClosedOnIncompleteOrMalformedCensus(t *testing.T) {
	owner := callSurfaceBody("owner", 1)
	callee := callSurfaceBody("owner", 2)
	lexical, _ := NewLexicalCallSurfaceTarget(callee)
	valid := []CallSurfaceSite{{Point: 1, Target: lexical}}
	tests := []struct {
		name      string
		owner     lexicalidentity.StableLexicalBodyID
		points    int
		extracted []cfg.Point
		sites     []CallSurfaceSite
	}{
		{name: "missing owner", points: 2, extracted: []cfg.Point{1}, sites: valid},
		{name: "below minimum points", owner: owner, points: 1},
		{name: "missing call", owner: owner, points: 3, extracted: []cfg.Point{1, 2}, sites: valid},
		{name: "extra call", owner: owner, points: 2, sites: valid},
		{name: "classified out of range", owner: owner, points: 2, extracted: []cfg.Point{1}, sites: []CallSurfaceSite{{Point: 2, Target: lexical}}},
		{name: "extracted out of range", owner: owner, points: 2, extracted: []cfg.Point{2}, sites: valid},
		{name: "duplicate extracted point", owner: owner, points: 3, extracted: []cfg.Point{1, 1}, sites: []CallSurfaceSite{{Point: 1, Target: lexical}, {Point: 2, Target: RejectedCallSurfaceTarget()}}},
		{name: "duplicate classified point", owner: owner, points: 3, extracted: []cfg.Point{1, 2}, sites: []CallSurfaceSite{{Point: 1, Target: lexical}, {Point: 1, Target: RejectedCallSurfaceTarget()}}},
		{name: "substituted non-call point", owner: owner, points: 4, extracted: []cfg.Point{1, 2}, sites: []CallSurfaceSite{{Point: 1, Target: lexical}, {Point: 3, Target: RejectedCallSurfaceTarget()}}},
		{name: "unclassified", owner: owner, points: 2, extracted: []cfg.Point{1}, sites: []CallSurfaceSite{{Point: 1}}},
		{name: "mixed target", owner: owner, points: 2, extracted: []cfg.Point{1}, sites: []CallSurfaceSite{{Point: 1, Target: CallSurfaceTarget{kind: CallSurfaceTargetRejected, lexical: callee}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if surface, err := SealCallSurface(test.owner, test.points, test.extracted, test.sites); err == nil || surface.Complete() || surface.Digest().Available() {
				t.Fatalf("malformed census published: %#v, err=%v", surface, err)
			}
		})
	}
}

func TestCallSurfaceDigestSeparatesFullWidthIdentityAndTargetNamespaces(t *testing.T) {
	owner := callSurfaceBody("owner", 1)
	lexicalID := callSurfaceBody("callee", 1)
	lexical, _ := NewLexicalCallSurfaceTarget(lexicalID)

	operation, ok := NewSignatureCallOperation(signature.Function{Type: typ.Func().Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}
	// Give the external identity the lexical target's exact bytes. The target
	// namespace tag must still keep the surfaces distinct.
	operation.contentID = signature.ContentID(lexicalID)
	external, ok := NewExternalCallSurfaceTarget(operation)
	if !ok {
		t.Fatal("external target rejected")
	}
	lexicalSurface := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: lexical})
	externalSurface := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: external})
	if lexicalSurface.Digest() == externalSurface.Digest() {
		t.Fatal("lexical and external target namespaces collided")
	}

	// IDs sharing a long prefix must retain their full-width distinction.
	otherOperation := operation
	otherOperation.contentID[len(otherOperation.contentID)-1] ^= 0xff
	other, ok := NewExternalCallSurfaceTarget(otherOperation)
	if !ok {
		t.Fatal("second external target rejected")
	}
	otherSurface := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: other})
	if externalSurface.Digest() == otherSurface.Digest() {
		t.Fatal("full-width external identities collided")
	}

	otherOwner := callSurfaceBody("other-owner", 1)
	if mustCallSurface(t, otherOwner, 3, CallSurfaceSite{Point: 1, Target: external}).Digest() == externalSurface.Digest() {
		t.Fatal("owner identity missing from digest")
	}
	if mustCallSurface(t, owner, 4, CallSurfaceSite{Point: 1, Target: external}).Digest() == externalSurface.Digest() {
		t.Fatal("point count missing from digest")
	}
}

func TestCallSurfaceAcceptsExplicitlyRejectedAndEmptyCompleteCensuses(t *testing.T) {
	owner := callSurfaceBody("owner", 1)
	rejected := mustCallSurface(t, owner, 2, CallSurfaceSite{Point: 1, Target: RejectedCallSurfaceTarget()})
	if got, ok := rejected.Site(1); !ok || got.Target.Kind() != CallSurfaceTargetRejected {
		t.Fatalf("rejected site absent: %#v/%v", got, ok)
	}
	empty, err := SealCallSurface(owner, 2, nil, nil)
	if err != nil || !empty.Complete() || !empty.Digest().Available() || len(empty.Sites()) != 0 {
		t.Fatalf("empty complete census rejected: %#v, err=%v", empty, err)
	}
}

func callSurfaceBody(unit string, function uint64) lexicalidentity.StableLexicalBodyID {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(unit))
	return lexicalidentity.FunctionBody(namespace, function)
}

func callSurfaceExternal(t *testing.T, intrinsic bool) CallSurfaceTarget {
	t.Helper()
	sig := signature.Function{Type: typ.Func().Param("value", typ.Any).Returns(typ.String).Build()}
	var operation SignatureCallOperation
	var ok bool
	if intrinsic {
		operation, ok = NewSignatureIntrinsicCallOperation(sig, signature.IntrinsicLuaType)
	} else {
		operation, ok = NewSignatureCallOperation(sig)
	}
	if !ok {
		t.Fatal("signature operation rejected")
	}
	target, ok := NewExternalCallSurfaceTarget(operation)
	if !ok {
		t.Fatal("external target rejected")
	}
	return target
}

func inputSite(point cfg.Point, target CallSurfaceTarget) CallSurfaceSite {
	return CallSurfaceSite{Point: point, Target: target}
}

func mustCallSurface(t *testing.T, owner lexicalidentity.StableLexicalBodyID, pointCount int, sites ...CallSurfaceSite) CallSurface {
	t.Helper()
	extracted := make([]cfg.Point, len(sites))
	for index := range sites {
		extracted[index] = sites[index].Point
	}
	surface, err := SealCallSurface(owner, pointCount, extracted, sites)
	if err != nil {
		t.Fatalf("SealCallSurface: %v", err)
	}
	return surface
}
