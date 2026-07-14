package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
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

func TestExternalCallSurfaceTargetMatchesOnlyExactDescriptor(t *testing.T) {
	want, ok := NewSignatureCallOperation(signature.Function{Type: typ.Func().Param("value", typ.String).Returns(typ.String).Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}
	target, ok := NewExternalCallSurfaceTarget(want)
	if !ok || !target.MatchesExternalOperation(want) {
		t.Fatal("external target did not match owned descriptor")
	}
	drifted, _ := NewSignatureCallOperation(signature.Function{Type: typ.Func().Param("value", typ.String).Returns(typ.Number).Build()})
	if target.MatchesExternalOperation(drifted) {
		t.Fatal("external target admitted descriptor drift")
	}
	if RejectedCallSurfaceTarget().MatchesExternalOperation(want) {
		t.Fatal("rejected target matched external descriptor")
	}
}

func TestPlanOwnsOnlyMatchingCompleteCallSurface(t *testing.T) {
	owner := callSurfaceBody("owner", 1)
	callee := callSurfaceBody("owner", 2)
	target, ok := NewLexicalCallSurfaceTarget(callee)
	if !ok {
		t.Fatal("lexical target rejected")
	}
	surface := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: target})
	plan := New(testCallSurfaceGraph(3), factflow.FactsInput{}).
		WithObservationIdentity(owner, testCallSurfaceWIR(3), testCallSurfaceGraph(3)).
		WithCallSurface(surface)
	got, ok := plan.CallSurface()
	if !ok || !got.Complete() || got.Owner() != owner || got.Digest() != surface.Digest() {
		t.Fatalf("owned call surface = %#v/%v", got, ok)
	}
	sites := got.Sites()
	sites[0] = CallSurfaceSite{}
	again, ok := plan.CallSurface()
	if !ok || len(again.Sites()) != 1 || again.Sites()[0].Target.Kind() != CallSurfaceTargetLexical {
		t.Fatal("plan call surface exposed mutable site storage")
	}
	rebound := plan.WithObservationIdentity(owner, testCallSurfaceWIR(3), testCallSurfaceGraph(3))
	if got, ok := rebound.CallSurface(); ok || got.Complete() {
		t.Fatal("observation identity rebind retained a stale call surface")
	}

	for name, rejected := range map[string]CallSurface{
		"wrong owner": mustCallSurface(t, callSurfaceBody("other", 1), 3, CallSurfaceSite{Point: 1, Target: target}),
		"wrong width": mustCallSurface(t, owner, 4, CallSurfaceSite{Point: 1, Target: target}),
		"zero":        {},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := New(testCallSurfaceGraph(3), factflow.FactsInput{}).
				WithObservationIdentity(owner, testCallSurfaceWIR(3), testCallSurfaceGraph(3)).
				WithCallSurface(rejected)
			if got, ok := candidate.CallSurface(); ok || got.Complete() || got.Digest().Available() {
				t.Fatalf("rejected surface remained available: %#v", got)
			}
		})
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

func TestCallSurfaceKeepsKnownLexicalTargetsSeparateFromUnresolvedSites(t *testing.T) {
	owner := callSurfaceBody("graph", 1)
	left := callSurfaceBody("graph", 2)
	right := callSurfaceBody("graph", 3)
	leftTarget, _ := NewLexicalCallSurfaceTarget(left)
	rightTarget, _ := NewLexicalCallSurfaceTarget(right)
	external := callSurfaceExternal(t, false)
	surface, err := SealCallSurface(owner, 10, []cfg.Point{8, 2, 6, 4, 1}, []CallSurfaceSite{
		{Point: 6, Target: RejectedTemporaryCallSurfaceTarget(9)},
		{Point: 1, Target: rightTarget},
		{Point: 8, Target: external},
		{Point: 4, Target: RejectedPathCallSurfaceTarget("sym41.member")},
		{Point: 2, Target: leftTarget},
	})
	if err != nil {
		t.Fatal(err)
	}
	sites := surface.Sites()
	if len(sites) != 5 || sites[0].Point != 1 || sites[1].Point != 2 || sites[2].Point != 4 || sites[3].Point != 6 || sites[4].Point != 8 {
		t.Fatalf("canonical sites = %#v", sites)
	}
	if got, _ := sites[0].Target.LexicalBody(); got != right {
		t.Fatalf("first target = %x, want right %x", got, right)
	}
	if got, _ := sites[1].Target.LexicalBody(); got != left {
		t.Fatalf("second target = %x, want left %x", got, left)
	}
	if sites[2].Target.kind != CallSurfaceTargetRejected || sites[2].Target.residue.kind != callSurfaceResidueUnresolved ||
		sites[2].Target.residue.hint.kind != callSurfaceHintPath || sites[2].Target.residue.hint.path != "sym41.member" {
		t.Fatalf("path residue = %#v", sites[2].Target)
	}
	if sites[3].Target.kind != CallSurfaceTargetRejected || sites[3].Target.residue.hint.kind != callSurfaceHintTemporary || sites[3].Target.residue.hint.temporary != 9 {
		t.Fatalf("temporary residue = %#v", sites[3].Target)
	}
	if sites[4].Target.kind != CallSurfaceTargetExternal || sites[4].Target.residue.kind != callSurfaceResidueExternal ||
		sites[4].Target.residue.hint.kind != callSurfaceHintExternalContent || !sites[4].Target.residue.hint.externalID.Available() {
		t.Fatalf("external residue = %#v", sites[4].Target)
	}
}

func TestCallSurfaceResidueDigestIsCanonicalAndSeparatesGuardHints(t *testing.T) {
	owner := callSurfaceBody("residue-digest", 1)
	method := RejectedMethodCallSurfaceTarget("sym7.field", "run")
	path := RejectedPathCallSurfaceTarget("sym7.field.run")
	temporary := RejectedTemporaryCallSurfaceTarget(7)
	base := mustCallSurface(t, owner, 4, CallSurfaceSite{Point: 2, Target: method})
	for name, candidate := range map[string]CallSurface{
		"hint namespace": mustCallSurface(t, owner, 4, CallSurfaceSite{Point: 2, Target: path}),
		"hint content":   mustCallSurface(t, owner, 4, CallSurfaceSite{Point: 2, Target: temporary}),
	} {
		if candidate.Digest() == base.Digest() {
			t.Fatalf("%s did not change digest", name)
		}
	}
	reordered, err := SealCallSurface(owner, 5, []cfg.Point{3, 1}, []CallSurfaceSite{{Point: 3, Target: method}, {Point: 1, Target: temporary}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := SealCallSurface(owner, 5, []cfg.Point{1, 3}, []CallSurfaceSite{{Point: 1, Target: temporary}, {Point: 3, Target: method}})
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Digest() != canonical.Digest() {
		t.Fatal("source/map input ordering changed residue digest")
	}
}

func TestCallSurfaceResiduePreservesZeroBasedTemporaryIdentity(t *testing.T) {
	target := RejectedTemporaryCallSurfaceTarget(0)
	if target.kind != CallSurfaceTargetRejected || target.residue.kind != callSurfaceResidueUnresolved ||
		target.residue.hint.kind != callSurfaceHintTemporary || target.residue.hint.temporary != 0 {
		t.Fatalf("temporary zero collapsed to absent hint: %#v", target)
	}
	owner := callSurfaceBody("zero-temp", 1)
	zero := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: target})
	absent := mustCallSurface(t, owner, 3, CallSurfaceSite{Point: 1, Target: RejectedCallSurfaceTarget()})
	if zero.Digest() == absent.Digest() {
		t.Fatal("temporary zero and absent hint have the same digest")
	}
}

func TestCallSurfaceSitesRepresentSelfAndRecursiveSCCShapes(t *testing.T) {
	left := callSurfaceBody("recursive", 1)
	right := callSurfaceBody("recursive", 2)
	leftTarget, _ := NewLexicalCallSurfaceTarget(left)
	rightTarget, _ := NewLexicalCallSurfaceTarget(right)
	self := mustCallSurface(t, left, 3, CallSurfaceSite{Point: 1, Target: leftTarget})
	leftSurface := mustCallSurface(t, left, 3, CallSurfaceSite{Point: 1, Target: rightTarget})
	rightSurface := mustCallSurface(t, right, 3, CallSurfaceSite{Point: 1, Target: leftTarget})
	if target, ok := self.Sites()[0].Target.LexicalBody(); !ok || target != self.Owner() {
		t.Fatalf("self edge = %x/%v owner=%x", target, ok, self.Owner())
	}
	leftEdge, leftOK := leftSurface.Sites()[0].Target.LexicalBody()
	rightEdge, rightOK := rightSurface.Sites()[0].Target.LexicalBody()
	if !leftOK || !rightOK || leftEdge != right || rightEdge != left {
		t.Fatalf("SCC edges = %x/%v %x/%v", leftEdge, leftOK, rightEdge, rightOK)
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

func testCallSurfaceGraph(points int) cfg.Graph {
	graph := cfg.New()
	previous := graph.Entry()
	for graph.Size() < points {
		next := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(previous, next, false)
		previous = next
	}
	graph.AddEdge(previous, graph.Exit(), false)
	return graph
}

func testCallSurfaceWIR(points int) *wir.Body {
	body := wir.NewBody("call-surface")
	body.AssignDebugPointOrdinals(testCallSurfaceGraph(points))
	return body
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
