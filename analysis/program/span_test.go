package program

import "testing"

func TestSpanQueriesRequirePublishedEvaluationGeometry(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-span-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if span, ok := published.Span(0); ok || span.Available() {
		t.Fatalf("Span(0) = %#v/%v; want unavailable", span, ok)
	}
	if site, ok := published.FinishSite(0); ok || site.Available() {
		t.Fatalf("FinishSite(0) = %#v/%v; want unavailable", site, ok)
	}
	if span, entry, finish, ok := published.EvaluationSpan(0); ok || span.Available() || entry != 0 || finish != 0 {
		t.Fatalf("EvaluationSpan(0) = %x/%v/%v/%v; want unavailable", span, entry, finish, ok)
	}
	if published.OwnsSpan(Span{}) {
		t.Fatal("zero Span passed the Program ownership fence")
	}
}
