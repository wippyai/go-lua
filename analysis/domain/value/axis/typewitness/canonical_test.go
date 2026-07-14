package typewitness

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCanonicalEncodingMatchesEqualAcrossNonrecursiveCorpus(t *testing.T) {
	leftRecord := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
		{Name: "z", Type: typ.NewArray(typ.String), Readonly: true},
		{Name: "a", Type: typ.Number, Optional: true},
	}})
	rightRecord := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
		{Name: "a", Type: typ.Number, Optional: true},
		{Name: "z", Type: typ.NewArray(typ.String), Readonly: true},
	}})
	values := []Value{
		Bottom(), Top(), Of(typ.Nil), Of(typ.Boolean), Of(typ.Integer), Of(typ.Number),
		Of(typ.String), Of(typ.Never), Of(typ.LiteralString("a\x00b")), Of(typ.LiteralInt(-1)),
		Of(typ.NewTuple(typ.String, typ.Number)), Of(typ.NewMap(typ.String, typ.Number)),
		Of(typ.MaterializeUnion([]typ.Type{typ.String, typ.Number})),
		Of(leftRecord), Of(rightRecord),
	}
	for left, a := range values {
		for right, b := range values {
			equal := Equal(a, b)
			sameBytes := bytes.Equal(canonicalTypeWitnessBytes(t, a), canonicalTypeWitnessBytes(t, b))
			if equal != sameBytes {
				t.Fatalf("values %d/%d Equal=%v bytes=%v", left, right, equal, sameBytes)
			}
		}
	}
}

func TestCanonicalEncodingRejectsEveryRecursiveShapeBeforeAxisPayload(t *testing.T) {
	self := typ.NewRecursive("Self", func(self typ.Type) typ.Type { return typ.NewArray(self) })
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(typ.NewTuple(typ.String, right))
	right.SetBody(typ.NewTuple(typ.Number, left))

	for name, value := range map[string]Value{
		"direct":  Of(self),
		"nested":  Of(typ.NewTuple(typ.String, self)),
		"mutual":  Of(left),
		"aliased": Of(typ.NewAlias("Alias", self)),
	} {
		t.Run(name, func(t *testing.T) {
			var writer canonical.Writer
			if err := writer.ResetBuffer(context.Background(), Key.ID(), 1); err != nil {
				t.Fatal(err)
			}
			before, err := writer.FinishBytes()
			if err != nil {
				t.Fatal(err)
			}

			if err := writer.ResetBuffer(context.Background(), Key.ID(), 1); err != nil {
				t.Fatal(err)
			}
			if err := encodeCanonical(&writer, value); !errors.Is(err, ErrNonportableRecursiveIdentity) {
				t.Fatalf("encode error = %v", err)
			}
			after, err := writer.FinishBytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("recursive rejection wrote authority payload:\n%x\n%x", before, after)
			}
		})
	}
}

func TestCanonicalEncodingRejectsUnsupportedAndMalformedValues(t *testing.T) {
	unsupported := &canonicalFakeType{}
	stale := Of(typ.String)
	stale.recursive = &recursiveSignature{}
	for name, value := range map[string]Value{
		"unsupported": {t: unsupported},
		"malformed":   {recursive: &recursiveSignature{}},
		"stale":       stale,
	} {
		t.Run(name, func(t *testing.T) {
			var writer canonical.Writer
			if err := writer.ResetBuffer(context.Background(), Key.ID(), 1); err != nil {
				t.Fatal(err)
			}
			if err := encodeCanonical(&writer, value); err == nil {
				t.Fatal("unsupported/malformed value encoded")
			}
		})
	}
}

func TestCanonicalEncodingPropagatesWriterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, Key.ID(), 1); err != nil {
		t.Fatal(err)
	}
	for range 61 { // Reset writes two events; next Record is event 64.
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	if err := encodeCanonical(&writer, Of(typ.String)); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

type canonicalFakeType struct{}

func (*canonicalFakeType) Kind() kind.Kind              { return kind.String }
func (*canonicalFakeType) String() string               { return "fake" }
func (*canonicalFakeType) Hash() uint64                 { return 1 }
func (f *canonicalFakeType) Equals(other typ.Type) bool { return f == other }

func canonicalTypeWitnessBytes(t testing.TB, value Value) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), Key.ID(), 1); err != nil {
		t.Fatal(err)
	}
	if err := encodeCanonical(&writer, value); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
