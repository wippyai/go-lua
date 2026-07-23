package service

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/compiler/source"
)

var benchmarkWholeSourceSpan source.Span

func TestSourceLineIndexMatchesLinearOffsetAt(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		[]byte("one"),
		[]byte("one\ntwo\nthree"),
		[]byte("one\ntwo\n"),
		[]byte("\n\n"),
		[]byte("a\r\nb\xc3\xa9"),
	} {
		index := newSourceLineIndex(data)
		for line := 0; line <= len(data)+3; line++ {
			for column := 0; column <= len(data)+3; column++ {
				wantOffset, wantOK := linearOffsetAt(data, line, column)
				gotOffset, gotOK := index.offsetAt(data, line, column)
				if gotOffset != wantOffset || gotOK != wantOK {
					t.Fatalf("offsetAt(%q, %d, %d) = (%d, %t), want (%d, %t)", data, line, column, gotOffset, gotOK, wantOffset, wantOK)
				}
			}
		}
	}
}

func BenchmarkSourceLineIndexWholeSourceSpan(b *testing.B) {
	data := bytes.Repeat([]byte("local value = call(value)\n"), 8_000)
	index := newSourceLineIndex(data)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		benchmarkWholeSourceSpan = index.wholeSourceSpan(len(data))
	}
}

func TestSourceLineIndexWholeSourceSpan(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		[]byte("one"),
		[]byte("one\ntwo\nthree"),
		[]byte("one\ntwo\n"),
		[]byte("\n\n"),
		[]byte("a\r\nb\xc3\xa9"),
	} {
		line, column := linearLineColumnAt(data, len(data))
		want := source.Span{StartLine: 1, StartCol: 1, EndLine: line, EndCol: column}
		if got := newSourceLineIndex(data).wholeSourceSpan(len(data)); got != want {
			t.Fatalf("wholeSourceSpan(%q) = %+v, want %+v", data, got, want)
		}
	}
}

func linearOffsetAt(data []byte, line, column int) (int, bool) {
	if line < 1 || column < 1 {
		return 0, false
	}
	currentLine, currentColumn := 1, 1
	for offset := 0; offset < len(data); offset++ {
		if currentLine == line && currentColumn == column {
			return offset, true
		}
		if data[offset] == '\n' {
			currentLine++
			currentColumn = 1
			continue
		}
		currentColumn++
	}
	return len(data), currentLine == line && currentColumn == column
}

func linearLineColumnAt(data []byte, target int) (int, int) {
	line, column := 1, 1
	for offset := 0; offset < target; offset++ {
		if data[offset] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}
