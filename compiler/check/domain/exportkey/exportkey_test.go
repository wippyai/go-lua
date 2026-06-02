package exportkey

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type symbolSourceStub struct {
	names map[cfg.SymbolID]string
	paths map[cfg.SymbolID]constraint.Path
}

func (s symbolSourceStub) NameOf(sym cfg.SymbolID) string {
	return s.names[sym]
}

func (s symbolSourceStub) FuncDefPathForSymbol(sym cfg.SymbolID) (constraint.Path, bool) {
	path, ok := s.paths[sym]
	return path, ok
}

func TestFromTargetPathKeepsStructuralMemberKind(t *testing.T) {
	tests := []struct {
		name   string
		path   constraint.Path
		want   constraint.Segment
		wantOK bool
	}{
		{
			name:   "direct root export",
			path:   constraint.Path{Root: "run", Symbol: 1},
			want:   constraint.Segment{Kind: constraint.SegmentField, Name: "run"},
			wantOK: true,
		},
		{
			name: "dot field export",
			path: constraint.Path{
				Root:     "M",
				Symbol:   1,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "run"}},
			},
			want:   constraint.Segment{Kind: constraint.SegmentField, Name: "run"},
			wantOK: true,
		},
		{
			name: "string index export",
			path: constraint.Path{
				Root:     "M",
				Symbol:   1,
				Segments: []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "run-key"}},
			},
			want:   constraint.Segment{Kind: constraint.SegmentIndexString, Name: "run-key"},
			wantOK: true,
		},
		{
			name: "integer index export",
			path: constraint.Path{
				Root:     "M",
				Symbol:   1,
				Segments: []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}},
			},
			want:   constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1},
			wantOK: true,
		},
		{
			name: "deeper path rejected",
			path: constraint.Path{
				Root: "M", Symbol: 1,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "api"},
					{Kind: constraint.SegmentField, Name: "run"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromTargetPath("", tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("key=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFromGraphSymbolRejectsDottedNameFallback(t *testing.T) {
	source := symbolSourceStub{
		names: map[cfg.SymbolID]string{
			1: "M.run",
			2: "localRun",
		},
	}
	if _, ok := FromGraphSymbol("", source, 1); ok {
		t.Fatal("dotted display name fallback must be rejected")
	}
	got, ok := FromGraphSymbol("", source, 2)
	if !ok {
		t.Fatal("direct source name fallback was rejected")
	}
	if got != (constraint.Segment{Kind: constraint.SegmentField, Name: "localRun"}) {
		t.Fatalf("fallback key=%#v, want localRun field", got)
	}
}
