package artifact_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
)

type artifactEdge struct {
	from, to, decision program.Term
	truthy             bool
	guarded            bool
	mu                 program.Term
	recurring          bool
	muDecisions        []program.Term
}

func activationEdgeSnapshot(t *testing.T, p *program.Program, body program.Term) []artifactEdge {
	t.Helper()
	count, ok := p.ActivationEdgeCount(body)
	if !ok {
		t.Fatalf("ActivationEdgeCount(%v) is absent", body)
	}
	result := make([]artifactEdge, count)
	for index := range result {
		edge, ok := p.ActivationEdgeAt(body, index)
		if !ok {
			t.Fatalf("ActivationEdgeAt(%v, %d) is absent", body, index)
		}
		decision, truthy, guarded := edge.Decision()
		mu, recurring := edge.Mu()
		result[index] = artifactEdge{
			from: edge.From(), to: edge.To(), decision: decision,
			truthy: truthy, guarded: guarded, mu: mu, recurring: recurring,
		}
		count, decisions := edgeMuDecisions(t, edge, recurring)
		if count != 0 {
			result[index].muDecisions = decisions
		}
	}
	return result
}

func edgeMuDecisions(t *testing.T, edge program.Edge, recurring bool) (int, []program.Term) {
	t.Helper()
	count, ok := edge.MuDecisionCount()
	if !recurring {
		if ok || count != 0 {
			t.Fatalf("ordinary Edge MuDecisionCount = %d/%v, want 0/false", count, ok)
		}
		return 0, nil
	}
	if !ok {
		t.Fatal("recurring Edge has no Mu decision range")
	}
	decisions := make([]program.Term, count)
	for index := range decisions {
		decision, ok := edge.MuDecisionAt(index)
		if !ok {
			t.Fatalf("recurring Edge MuDecisionAt(%d) is absent", index)
		}
		decisions[index] = decision
	}
	if decision, ok := edge.MuDecisionAt(count); ok || decision != 0 {
		t.Fatalf("recurring Edge MuDecisionAt(%d) = %v/%v past end", count, decision, ok)
	}
	return count, decisions
}

func sameArtifactEdges(left, right artifactEdge) bool {
	if left.from != right.from || left.to != right.to || left.decision != right.decision ||
		left.truthy != right.truthy || left.guarded != right.guarded ||
		left.mu != right.mu || left.recurring != right.recurring ||
		len(left.muDecisions) != len(right.muDecisions) {
		return false
	}
	for index := range left.muDecisions {
		if left.muDecisions[index] != right.muDecisions[index] {
			return false
		}
	}
	return true
}

func requireCallBoundaryReplay(t *testing.T, before, after *program.Program) {
	t.Helper()
	if before.CallCount() != after.CallCount() {
		t.Fatalf("Call count = %d, want %d", after.CallCount(), before.CallCount())
	}
	for index := 0; index < before.CallCount(); index++ {
		left, leftOK := before.CallAt(index)
		right, rightOK := after.CallAt(index)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("Call[%d] identity = %v/%v, want %v/%v", index, right, rightOK, left, leftOK)
		}
		leftCount, leftLive := before.CallLiveDecisionCount(left)
		rightCount, rightLive := after.CallLiveDecisionCount(right)
		if leftLive != rightLive || leftCount != rightCount {
			t.Fatalf("Call[%d] live decisions = %d/%v, want %d/%v", index, rightCount, rightLive, leftCount, leftLive)
		}
		for decisionIndex := 0; decisionIndex < leftCount; decisionIndex++ {
			want, wantOK := before.CallLiveDecisionAt(left, decisionIndex)
			got, gotOK := after.CallLiveDecisionAt(right, decisionIndex)
			if !wantOK || !gotOK || got != want {
				t.Fatalf("Call[%d] decision[%d] = %v/%v, want %v/%v", index, decisionIndex, got, gotOK, want, wantOK)
			}
		}
		wantTail, wantTailOK := before.CallTailExit(left)
		gotTail, gotTailOK := after.CallTailExit(right)
		if gotTailOK != wantTailOK || gotTail != wantTail {
			t.Fatalf("Call[%d] tail exit = %v/%v, want %v/%v", index, gotTail, gotTailOK, wantTail, wantTailOK)
		}
	}
}

func TestArtifactRoundTripRebuildsSealedEdgesDeterministically(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "edge-roundtrip.lua", `
local function child(x)
  if x and next(x) then return x end
end
while keep() do
  if left() or right() then break end
end
repeat tick() until done()
return child({})
`)
	metadata := artifact.Metadata{Provenance: "edge-roundtrip"}
	first, err := artifact.Encode(p, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	second, err := artifact.Encode(p, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeated artifact encoding changed canonical bytes")
	}
	replayed, gotMetadata, err := artifact.Decode(first, contract)
	if err != nil {
		t.Fatal(err)
	}
	if gotMetadata.Provenance != metadata.Provenance || len(gotMetadata.Dependencies) != 0 || replayed.ContentID() != p.ContentID() {
		t.Fatal("artifact replay changed canonical Program identity")
	}
	requireCallBoundaryReplay(t, p, replayed)
	entry, _ := p.Entry()
	replayedEntry, _ := replayed.Entry()
	if got, want := activationEdgeSnapshot(t, replayed, replayedEntry), activationEdgeSnapshot(t, p, entry); len(got) != len(want) {
		t.Fatalf("Entry Edge count = %d, want %d", len(got), len(want))
	} else {
		for index := range want {
			if !sameArtifactEdges(got[index], want[index]) {
				t.Fatalf("Entry Edge[%d] = %#v, want %#v", index, got[index], want[index])
			}
		}
	}
	for index := 0; index < p.FunctionCount(); index++ {
		before, _ := p.FunctionAt(index)
		after, _ := replayed.FunctionAt(index)
		_, beforeBody, _, beforeOK := p.Function(before)
		_, afterBody, _, afterOK := replayed.Function(after)
		if !beforeOK || !afterOK {
			t.Fatalf("Function[%d] Body is absent", index)
		}
		got, want := activationEdgeSnapshot(t, replayed, afterBody), activationEdgeSnapshot(t, p, beforeBody)
		if len(got) != len(want) {
			t.Fatalf("Function[%d] Edge count = %d, want %d", index, len(got), len(want))
		}
		for edgeIndex := range want {
			if !sameArtifactEdges(got[edgeIndex], want[edgeIndex]) {
				t.Fatalf("Function[%d] Edge[%d] = %#v, want %#v", index, edgeIndex, got[edgeIndex], want[edgeIndex])
			}
		}
	}
}
