package semanticsource

import (
	"errors"
	"testing"
)

func TestPublicationRangeSealsExactOrderIncludingZeroRows(t *testing.T) {
	definitions := []RelationDef{
		generatedDefinition(t, OriginProgramSourceProvenance, 0),
		generatedDefinition(t, OriginProgramSourceOrder, 0),
	}
	rows := []Publication{
		mustPublication(t, definitions[0], 0),
		mustPublication(t, definitions[1], 7),
	}
	rangeValue, err := SealPublicationRange(definitions, rows)
	if err != nil {
		t.Fatalf("SealPublicationRange: %v", err)
	}
	if !rangeValue.Valid() || rangeValue.Count() != len(definitions) {
		t.Fatalf("range valid/count = %v/%d, want true/%d", rangeValue.Valid(), rangeValue.Count(), len(definitions))
	}
	for index, want := range rows {
		got, ok := rangeValue.At(index)
		if !ok || got != want {
			t.Fatalf("At(%d) = %#v/%v, want %#v/true", index, got, ok, want)
		}
	}
	if _, ok := rangeValue.At(rangeValue.Count()); ok {
		t.Fatal("At(Count) accepted a row")
	}
	if _, ok := rangeValue.At(-1); ok {
		t.Fatal("At(-1) accepted a row")
	}
	first, ok := rangeValue.Digest()
	if !ok {
		t.Fatal("sealed range has no digest")
	}
	second, ok := rangeValue.Snapshot().Digest()
	if !ok || first != second {
		t.Fatalf("snapshot digest = %x/%v, want %x/true", second, ok, first)
	}
}

func TestPublicationRangeRejectsMalformedOrderAndOwnership(t *testing.T) {
	definitions := []RelationDef{
		generatedDefinition(t, OriginProgramSourceProvenance, 0),
		generatedDefinition(t, OriginProgramSourceOrder, 0),
	}
	rows := []Publication{
		mustPublication(t, definitions[0], 0),
		mustPublication(t, definitions[1], 0),
	}
	if _, err := SealPublicationRange([]RelationDef{{}, definitions[1]}, rows); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("malformed definition error = %v, want %v", err, ErrInvalidDefinition)
	}
	tests := []struct {
		name string
		rows []Publication
		want error
	}{
		{
			name: "reordered",
			rows: []Publication{rows[1], rows[0]},
			want: ErrPublicationOrder,
		},
		{
			name: "duplicate",
			rows: []Publication{rows[0], rows[0]},
			want: ErrDuplicatePublication,
		},
		{
			name: "missing",
			rows: rows[:1],
			want: ErrMissingPublication,
		},
		{
			name: "foreign",
			rows: []Publication{mustPublication(t, generatedDefinition(t, OriginProgramFlowValues, 0), 0), rows[1]},
			want: ErrUnexpectedPublication,
		},
		{
			name: "invalid row",
			rows: []Publication{{}, rows[1]},
			want: ErrInvalidPublication,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := SealPublicationRange(definitions, test.rows); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	if _, err := SealPublicationRange(definitions, nil); !errors.Is(err, ErrMissingPublication) {
		t.Fatalf("nil rows error = %v, want %v", err, ErrMissingPublication)
	}
}

func TestPublicationRangeCursorCapturesCountAndReplaysDigest(t *testing.T) {
	definitions := []RelationDef{
		generatedDefinition(t, OriginProgramSourceProvenance, 0),
		generatedDefinition(t, OriginProgramSourceOrder, 0),
	}
	rangeValue, err := SealPublicationRange(definitions, []Publication{
		mustPublication(t, definitions[0], 0),
		mustPublication(t, definitions[1], 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor := rangeValue.Cursor()
	shrunk := rangeValue
	shrunk.count = 1
	if cursor.count != 2 || shrunk.Count() != 0 {
		t.Fatalf("cursor/range count snapshot = %d/%d, want 2/0 after malformed copy", cursor.count, shrunk.Count())
	}
	for index := 0; index < 2; index++ {
		if _, ok := cursor.Next(); !ok {
			t.Fatalf("cursor ended at %d", index)
		}
	}
	if _, ok := cursor.Next(); ok {
		t.Fatal("cursor yielded past snapshotted Count")
	}
	first, ok := rangeValue.Digest()
	if !ok {
		t.Fatal("range digest unavailable")
	}
	replay, err := SealPublicationRange(definitions, rangeValue.Publications())
	if err != nil {
		t.Fatal(err)
	}
	second, ok := replay.Digest()
	if !ok || first != second {
		t.Fatalf("replay digest = %x/%v, want %x/true", second, ok, first)
	}
}

func TestPublicationRangeRejectsSharedRowAndDigestTamper(t *testing.T) {
	definitions := []RelationDef{
		generatedDefinition(t, OriginProgramSourceProvenance, 0),
		generatedDefinition(t, OriginProgramSourceOrder, 0),
	}
	rangeValue, err := SealPublicationRange(definitions, []Publication{
		mustPublication(t, definitions[0], 0),
		mustPublication(t, definitions[1], 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	rowTampered := rangeValue
	rowTampered.rows = append([]Publication(nil), rangeValue.rows...)
	rowTampered.rows[1] = mustPublication(t, definitions[1], 3)
	if rowTampered.Valid() {
		t.Fatal("range accepted a sealed-row count mutation")
	}
	if rowTampered.Count() != 0 {
		t.Fatalf("row-tampered Count = %d, want 0", rowTampered.Count())
	}
	if _, ok := rowTampered.Digest(); ok {
		t.Fatal("row-tampered range returned a digest")
	}

	digestTampered := rangeValue
	digestTampered.digest[0] ^= 0xff
	if digestTampered.Valid() {
		t.Fatal("range accepted a cached digest mutation")
	}
	if digestTampered.Count() != 0 {
		t.Fatalf("digest-tampered Count = %d, want 0", digestTampered.Count())
	}
	if _, ok := digestTampered.Digest(); ok {
		t.Fatal("digest-tampered range returned a digest")
	}
}
