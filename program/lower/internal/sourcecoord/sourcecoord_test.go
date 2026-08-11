package sourcecoord

import (
	"testing"

	"github.com/wippyai/go-lua/program/source"
)

func TestBuildCoordinateLaw(t *testing.T) {
	const file = "coords.lua"
	cases := []struct {
		name                                 string
		startLine, startCol, endLine, endCol int
		want                                 source.Span
		ok                                   bool
	}{
		{"no position", 0, 0, 0, 0, source.Span{File: file}, true},
		{"open end", 2, 3, 0, 0, source.Span{File: file, StartLine: 2, StartCol: 3}, true},
		{"closed", 2, 3, 4, 5, source.Span{File: file, StartLine: 2, StartCol: 3, EndLine: 4, EndCol: 5}, true},
		{"negative start", -1, 3, 0, 0, source.Span{}, false},
		{"mixed start zero", 0, 3, 0, 0, source.Span{}, false},
		{"mixed start column zero", 3, 0, 0, 0, source.Span{}, false},
		{"negative end", 2, 3, -1, 2, source.Span{}, false},
		{"mixed end zero", 2, 3, 4, 0, source.Span{}, false},
		{"mixed end line zero", 2, 3, 0, 4, source.Span{}, false},
		{"reverse line", 4, 3, 2, 5, source.Span{}, false},
		{"reverse column", 2, 4, 2, 3, source.Span{}, false},
		{"overflow", int(^uint32(0)) + 1, 1, 0, 0, source.Span{}, false},
		{"end overflow", 1, 1, int(^uint32(0)) + 1, 1, source.Span{}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Build(file, test.startLine, test.startCol, test.endLine, test.endCol)
			if got != test.want || ok != test.ok {
				t.Fatalf("Build = %#v/%v, want %#v/%v", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestInvalidIsNotLawfulZeroSpan(t *testing.T) {
	invalid := Invalid("coords.lua")
	if invalid == (source.Span{File: "coords.lua"}) {
		t.Fatal("Invalid returned a lawful all-zero span")
	}
	if invalid.StartLine != 0 || invalid.StartCol == 0 {
		t.Fatalf("Invalid = %#v, want malformed non-zero start column", invalid)
	}
}
