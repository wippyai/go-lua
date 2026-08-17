package denominator

import (
	"math"
	"testing"
)

func TestSumCountRowsAddsDuplicateIdentities(t *testing.T) {
	entries := GeneratedRelationEntries()
	if len(entries) < 2 {
		t.Fatal("generated relation catalog is empty")
	}
	left, leftOK := NewCountRow(entries[0].ID(), 4)
	right, rightOK := NewCountRow(entries[0].ID(), 9)
	other, otherOK := NewCountRow(entries[1].ID(), 3)
	if !leftOK || !rightOK || !otherOK {
		t.Fatal("generated relation count row was not admitted")
	}
	first, firstOK := NewCountRows([]CountRow{left})
	second, secondOK := NewCountRows([]CountRow{right, other})
	if !firstOK || !secondOK {
		t.Fatal("owner count rows did not seal")
	}
	summed, summedOK := SumCountRows(first, second)
	if !summedOK {
		t.Fatal("duplicate owner counts were rejected")
	}
	if got, ok := summed.Value(entries[0].ID()); !ok || got != 13 {
		t.Fatalf("summed duplicate = (%d, %t), want (13, true)", got, ok)
	}
	if got, ok := summed.Value(entries[1].ID()); !ok || got != 3 {
		t.Fatalf("summed distinct = (%d, %t), want (3, true)", got, ok)
	}
}

func TestSumCountRowsRejectsOverflow(t *testing.T) {
	entries := GeneratedRelationEntries()
	maximum, maximumOK := NewCountRow(entries[0].ID(), math.MaxUint64)
	one, oneOK := NewCountRow(entries[0].ID(), 1)
	if !maximumOK || !oneOK {
		t.Fatal("overflow fixture row was not admitted")
	}
	left, leftOK := NewCountRows([]CountRow{maximum})
	right, rightOK := NewCountRows([]CountRow{one})
	if !leftOK || !rightOK {
		t.Fatal("overflow fixture rows did not seal")
	}
	if _, ok := SumCountRows(left, right); ok {
		t.Fatal("overflowing owner counts were accepted")
	}
}

func TestGeneratedCountRowsRequireEveryRelationIncludingZero(t *testing.T) {
	entries := GeneratedRelationEntries()
	rows := make([]CountRow, 0, len(entries))
	for _, entry := range entries {
		row, ok := NewCountRow(entry.ID(), 0)
		if !ok {
			t.Fatal("zero relation count row was not admitted")
		}
		rows = append(rows, row)
	}
	complete, completeOK := NewCountRows(rows)
	if !completeOK || !GeneratedCountRowsComplete(complete) {
		t.Fatal("zero-filled generated catalog was not complete")
	}
	missing, missingOK := NewCountRows(rows[:len(rows)-1])
	if !missingOK || GeneratedCountRowsComplete(missing) {
		t.Fatal("incomplete generated catalog was accepted")
	}
}
