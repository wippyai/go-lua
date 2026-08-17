package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func viewFixture() (*Certificate, identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID) {
	s, f, st, m := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}
	return &Certificate{state: &certificateState{sourceID: s, flowID: f, staticID: st, moduleID: m}}, s, f, st, m
}

func TestCertificateViewsRequireExactOwnerQuartet(t *testing.T) {
	certificate, s, f, st, m := viewFixture()
	if _, ok := certificate.VertexCatalog(s, f, st, m); !ok {
		t.Fatal("vertex view unavailable for exact owners")
	}
	if _, ok := certificate.OutcomePhases(s, f, st, m); !ok {
		t.Fatal("outcome view unavailable for exact owners")
	}
	if _, ok := certificate.Causal(s, f, st, m); !ok {
		t.Fatal("causal view unavailable for exact owners")
	}
	foreignSource := s
	foreignSource[0]++
	if _, ok := certificate.VertexCatalog(foreignSource, f, st, m); ok {
		t.Fatal("foreign source obtained a vertex view")
	}
	foreignFlow := f
	foreignFlow[0]++
	if _, ok := certificate.OutcomePhases(s, foreignFlow, st, m); ok {
		t.Fatal("foreign flow obtained an outcome view")
	}
	foreignStatic := st
	foreignStatic[0]++
	if _, ok := certificate.Causal(s, f, foreignStatic, m); ok {
		t.Fatal("foreign static owner obtained a causal view")
	}
}

func TestCertificateViewsRemainOwnerFencedWhenCopied(t *testing.T) {
	certificate, s, f, st, m := viewFixture()
	paths, ok := certificate.Causal(s, f, st, m)
	if !ok {
		t.Fatal("causal view unavailable")
	}
	copyPaths := *paths
	if !copyPaths.Matches(s, f, st, m) {
		t.Fatal("copied view lost its owner fence")
	}
	foreign := m
	foreign[0]++
	if copyPaths.Matches(s, f, st, foreign) {
		t.Fatal("copied view accepted a foreign module")
	}
}
