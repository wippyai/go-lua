package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func receiptFixture() (*Certificate, identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID) {
	s, f, st, m := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}
	return &Certificate{state: &certificateState{sourceID: s, flowID: f, staticID: st, moduleID: m}}, s, f, st, m
}

func TestTypedReceiptsShareTerminalConsumptionAcrossCopies(t *testing.T) {
	certificate, s, f, st, m := receiptFixture()
	copyCertificate := *certificate
	vertex, ok := certificate.IssueVertexCatalogReceipt()
	if !ok {
		t.Fatal("vertex issuance unavailable")
	}
	copyVertex := *vertex
	foreign := s
	foreign[0]++
	if _, ok := vertex.Consume(foreign, f, st, m); ok {
		t.Fatal("foreign vertex consume succeeded")
	}
	if _, ok := copyVertex.Consume(s, f, st, m); ok {
		t.Fatal("foreign vertex consume left copied receipt live")
	}
	if _, ok := copyCertificate.IssueVertexCatalogReceipt(); ok {
		t.Fatal("copied Certificate reissued vertex receipt")
	}
	causal, ok := certificate.IssueCausalReceipt()
	if !ok {
		t.Fatal("causal issuance unavailable")
	}
	copyCausal := *causal
	if _, ok := copyCausal.Consume(s, f, st, m); !ok {
		t.Fatal("exact causal consume failed")
	}
	if _, ok := causal.Consume(s, f, st, m); ok {
		t.Fatal("copied causal receipt consumed twice")
	}
}
