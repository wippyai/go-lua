package lsp

import (
	"errors"
	"fmt"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/embedding"
)

// textBuffer is the server-owned overlay. Its source bytes are the only bytes
// used for LSP UTF-16/byte conversion and materialized checker input.
type textBuffer struct {
	content    []byte
	lineStarts []int
}

func newTextBuffer(content []byte) textBuffer {
	b := textBuffer{content: append([]byte(nil), content...)}
	b.reindex()
	return b
}

func (b *textBuffer) bytes() []byte { return append([]byte(nil), b.content...) }

func (b *textBuffer) digest() embedding.Digest { return embedding.DigestBytes(b.content) }

func (b *textBuffer) reindex() {
	b.lineStarts = []int{0}
	for index, value := range b.content {
		if value == '\n' {
			b.lineStarts = append(b.lineStarts, index+1)
		}
	}
}

func (b *textBuffer) apply(changes []TextDocumentContentChangeEvent) error {
	if len(changes) == 0 {
		return errors.New("lsp: didChange requires at least one content change")
	}
	for _, change := range changes {
		if change.Range == nil {
			b.content = []byte(change.Text)
			b.reindex()
			continue
		}
		start, err := b.offsetForPosition(change.Range.Start)
		if err != nil {
			return fmt.Errorf("lsp: change range start: %w", err)
		}
		end, err := b.offsetForPosition(change.Range.End)
		if err != nil {
			return fmt.Errorf("lsp: change range end: %w", err)
		}
		if end < start {
			return errors.New("lsp: change range ends before it starts")
		}
		if change.RangeLength != nil && *change.RangeLength != utf16Units(b.content[start:end]) {
			return fmt.Errorf("lsp: rangeLength %d does not match UTF-16 range length %d", *change.RangeLength, utf16Units(b.content[start:end]))
		}
		next := make([]byte, 0, len(b.content)-(end-start)+len(change.Text))
		next = append(next, b.content[:start]...)
		next = append(next, change.Text...)
		next = append(next, b.content[end:]...)
		b.content = next
		b.reindex()
	}
	return nil
}

// offsetForPosition projects an LSP UTF-16 position onto a byte offset. A
// position between a surrogate pair is rejected instead of silently snapping.
func (b *textBuffer) offsetForPosition(position Position) (int, error) {
	if position.Line < 0 || position.Character < 0 || position.Line >= len(b.lineStarts) {
		return 0, errors.New("position is outside the document")
	}
	start, end := b.lineBounds(position.Line)
	offset := start
	units := 0
	for offset < end {
		if units == position.Character {
			return offset, nil
		}
		runeValue, size := utf8.DecodeRune(b.content[offset:end])
		width := utf16.RuneLen(runeValue)
		if width < 0 {
			width = 1
		}
		if position.Character > units && position.Character < units+width {
			return 0, errors.New("position splits a UTF-16 surrogate pair")
		}
		units += width
		offset += size
	}
	if units == position.Character {
		return offset, nil
	}
	return 0, errors.New("character is outside the line")
}

// positionForOffset projects a byte offset at a UTF-8 code point boundary to
// an LSP UTF-16 position.
func (b *textBuffer) positionForOffset(offset int) (Position, error) {
	if offset < 0 || offset > len(b.content) {
		return Position{}, errors.New("byte offset is outside the document")
	}
	line := sort.Search(len(b.lineStarts), func(index int) bool { return b.lineStarts[index] > offset }) - 1
	if line < 0 {
		line = 0
	}
	start, end := b.lineBounds(line)
	if offset > end {
		// A newline byte maps to the end of its preceding line. The byte after a
		// newline is the next line start and is handled by the search above.
		offset = end
	}
	current := start
	units := 0
	for current < offset {
		runeValue, size := utf8.DecodeRune(b.content[current:end])
		if current+size > offset {
			return Position{}, errors.New("byte offset splits a UTF-8 code point")
		}
		width := utf16.RuneLen(runeValue)
		if width < 0 {
			width = 1
		}
		units += width
		current += size
	}
	return Position{Line: line, Character: units}, nil
}

func (b *textBuffer) rangeForSpan(span embedding.ByteSpan) (Range, error) {
	if !span.Valid() || span.EndByte > len(b.content) {
		return Range{}, errors.New("source byte span is outside the overlay")
	}
	start, err := b.positionForOffset(span.StartByte)
	if err != nil {
		return Range{}, err
	}
	end, err := b.positionForOffset(span.EndByte)
	if err != nil {
		return Range{}, err
	}
	return Range{Start: start, End: end}, nil
}

// rangeForCompilerSpan converts the checker's one-indexed byte columns only
// after the source has been identified as this exact overlay. It exists for
// legacy evidence spans, whose semantic locations predate the DTO migration.
func (b *textBuffer) rangeForCompilerSpan(startLine, startColumn, endLine, endColumn int) (Range, error) {
	start, err := b.byteOffsetForCompilerPosition(startLine, startColumn)
	if err != nil {
		return Range{}, err
	}
	end := start
	if endLine > 0 && endColumn > 0 {
		end, err = b.byteOffsetForCompilerPosition(endLine, endColumn)
		if err != nil {
			return Range{}, err
		}
	}
	return b.rangeForSpan(embedding.ByteSpan{StartByte: start, EndByte: end})
}

func (b *textBuffer) byteOffsetForCompilerPosition(line, column int) (int, error) {
	if line <= 0 || column <= 0 || line > len(b.lineStarts) {
		return 0, errors.New("compiler position is outside the overlay")
	}
	start, end := b.lineBounds(line - 1)
	offset := start + column - 1
	if offset < start || offset > end {
		return 0, errors.New("compiler column is outside the line")
	}
	return offset, nil
}

func (b *textBuffer) lineBounds(line int) (int, int) {
	start := b.lineStarts[line]
	end := len(b.content)
	if line+1 < len(b.lineStarts) {
		end = b.lineStarts[line+1] - 1 // exclude LF
	}
	if end > start && b.content[end-1] == '\r' {
		end-- // CRLF is one logical line break for LSP positions.
	}
	return start, end
}

func utf16Units(data []byte) int {
	units := 0
	for len(data) > 0 {
		runeValue, size := utf8.DecodeRune(data)
		width := utf16.RuneLen(runeValue)
		if width < 0 {
			width = 1
		}
		units += width
		data = data[size:]
	}
	return units
}
