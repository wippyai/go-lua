package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func assemblyTestSpan() source.Span {
	return source.Span{File: "assembly.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
}

func newAssemblyCollector() *Collector {
	return New("assembly.lua", 0, bind.GlobalCensus{})
}

func TestAssemblyModelStartsWithCanonicalSourceIdentity(t *testing.T) {
	c := newAssemblyCollector()
	if c == nil || c.name != "assembly.lua" || c.terminal || c.err != nil {
		t.Fatal("new collector did not start as a live assembly")
	}
	if !validSourceSpan(assemblyTestSpan()) || validSourceSpan(source.Span{StartLine: 2, StartCol: 4, EndLine: 1, EndCol: 1}) {
		t.Fatal("source span validation does not preserve monotonic coordinates")
	}
	if validTerm(keyspace.Term(0)) || validBodyTerm(keyspace.Term(0)) {
		t.Fatal("zero term was admitted as a canonical Term")
	}
}

func TestAssemblyModelRejectsInvalidSourceName(t *testing.T) {
	if c := New("", 0, bind.GlobalCensus{}); c.err == nil || !c.terminal {
		t.Fatal("empty source name did not poison the collector")
	}
}
