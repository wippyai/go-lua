package lsp

import (
	"math/rand"
	"testing"
	"unicode/utf8"
)

func TestTextBufferIncrementalUTF16Edit(t *testing.T) {
	buffer := newTextBuffer([]byte("a😀b\n猫\r\n"))
	if _, err := buffer.offsetForPosition(Position{Line: 0, Character: 2}); err == nil {
		t.Fatal("position in the middle of a surrogate pair was accepted")
	}
	if err := buffer.apply([]TextDocumentContentChangeEvent{{
		Range: &Range{Start: Position{Line: 0, Character: 1}, End: Position{Line: 0, Character: 3}},
		Text:  "é",
	}}); err != nil {
		t.Fatalf("apply UTF-16 edit: %v", err)
	}
	if got, want := string(buffer.bytes()), "aéb\n猫\r\n"; got != want {
		t.Fatalf("edited buffer = %q, want %q", got, want)
	}
}

func TestUTF16PositionRoundTripsGeneratedMultibyteText(t *testing.T) {
	random := rand.New(rand.NewSource(20260710))
	parts := []string{"a", "é", "猫", "😀", "���", "\t", " ", "\n", "\r\n"}
	for iteration := 0; iteration < 500; iteration++ {
		count := random.Intn(80)
		text := ""
		for index := 0; index < count; index++ {
			text += parts[random.Intn(len(parts))]
		}
		buffer := newTextBuffer([]byte(text))
		for _, offset := range validInsertionOffsets(buffer) {
			position, err := buffer.positionForOffset(offset)
			if err != nil {
				t.Fatalf("iteration %d offset %d in %q -> position: %v", iteration, offset, text, err)
			}
			roundTrip, err := buffer.offsetForPosition(position)
			if err != nil {
				t.Fatalf("iteration %d position %#v in %q -> offset: %v", iteration, position, text, err)
			}
			if roundTrip != offset {
				t.Fatalf("iteration %d %q: offset %d -> %#v -> %d", iteration, text, offset, position, roundTrip)
			}
		}
	}
}

func validInsertionOffsets(buffer textBuffer) []int {
	seen := make(map[int]struct{})
	for line := range buffer.lineStarts {
		start, end := buffer.lineBounds(line)
		for offset := start; offset <= end; {
			seen[offset] = struct{}{}
			if offset == end {
				break
			}
			_, size := utf8.DecodeRune(buffer.content[offset:end])
			offset += size
		}
	}
	result := make([]int, 0, len(seen))
	for offset := range seen {
		result = append(result, offset)
	}
	return result
}
