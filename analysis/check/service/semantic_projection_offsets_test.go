package service

import "testing"

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
