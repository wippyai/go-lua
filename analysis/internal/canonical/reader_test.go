package canonical

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func TestReaderStrictRoundTripAndOwnedPayloads(t *testing.T) {
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "reader.test", 7); err != nil {
		t.Fatal(err)
	}
	for _, write := range []func() error{
		func() error { return writer.Record(9) }, writer.Nil,
		func() error { return writer.Bool(true) }, func() error { return writer.Uint(42) },
		func() error { return writer.Int(-17) }, func() error { return writer.Float64(-2.5) },
		func() error { return writer.Count(3) }, func() error { return writer.String("a\x00b") },
		func() error { return writer.Bytes([]byte{1, 2, 3}) },
	} {
		if err := write(); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	var reader Reader
	if err := reader.Reset(context.Background(), raw, "reader.test", 7); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.Record(); err != nil || value != 9 {
		t.Fatalf("Record = %d, %v", value, err)
	}
	if err := reader.Nil(); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.Bool(); err != nil || !value {
		t.Fatalf("Bool = %v, %v", value, err)
	}
	if value, err := reader.Uint(); err != nil || value != 42 {
		t.Fatalf("Uint = %d, %v", value, err)
	}
	if value, err := reader.Int(); err != nil || value != -17 {
		t.Fatalf("Int = %d, %v", value, err)
	}
	if value, err := reader.Float64(); err != nil || value != -2.5 {
		t.Fatalf("Float64 = %v, %v", value, err)
	}
	if value, err := reader.Count(); err != nil || value != 3 {
		t.Fatalf("Count = %d, %v", value, err)
	}
	if value, err := reader.String(); err != nil || value != "a\x00b" {
		t.Fatalf("String = %q, %v", value, err)
	}
	owned, err := reader.Bytes()
	if err != nil || !bytes.Equal(owned, []byte{1, 2, 3}) {
		t.Fatalf("Bytes = %x, %v", owned, err)
	}
	for index := range raw {
		raw[index] = 0
	}
	if !bytes.Equal(owned, []byte{1, 2, 3}) {
		t.Fatal("Bytes result aliases reader input")
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestReaderRejectsNoncanonicalVarintsWrongTagsAndTrailingData(t *testing.T) {
	header := func() []byte {
		var writer Writer
		if err := writer.ResetBuffer(context.Background(), "x", 1); err != nil {
			t.Fatal(err)
		}
		raw, err := writer.FinishBytes()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	event := func(tag byte, payload []byte) []byte {
		out := []byte{tag}
		out = binary.AppendUvarint(out, uint64(len(payload)))
		return append(out, payload...)
	}

	for name, raw := range map[string][]byte{
		"overlong-header-length": {tagDomain, 0x81, 0x00, 'x'},
		"overlong-uint":          append(header(), event(tagUint, []byte{0x81, 0x00})...),
		"wrong-tag":              append(header(), event(tagBool, []byte{1})...),
	} {
		t.Run(name, func(t *testing.T) {
			var reader Reader
			if name == "overlong-header-length" {
				if err := reader.Reset(context.Background(), raw, "x", 1); !errors.Is(err, ErrMalformed) {
					t.Fatalf("Reset error = %v", err)
				}
				return
			}
			if err := reader.Reset(context.Background(), raw, "x", 1); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Uint(); !errors.Is(err, ErrMalformed) {
				t.Fatalf("Uint error = %v", err)
			}
		})
	}

	var reader Reader
	raw := append(header(), event(tagUint, []byte{1})...)
	if err := reader.Reset(context.Background(), raw, "x", 1); err != nil {
		t.Fatal(err)
	}
	if err := reader.Finish(); !errors.Is(err, ErrTrailing) {
		t.Fatalf("Finish error = %v", err)
	}
}

func TestReaderForcedFinalAndChunkedPayloadCancellation(t *testing.T) {
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "cancel", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(1); err != nil {
		t.Fatal(err)
	}
	raw, _ := writer.FinishBytes()
	ctx, cancel := context.WithCancel(context.Background())
	var reader Reader
	if err := reader.Reset(ctx, raw, "cancel", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Uint(); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := reader.Finish(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Finish error = %v", err)
	}

	large := bytes.Repeat([]byte{7}, payloadChunkSize*3)
	writer = Writer{}
	if err := writer.ResetBuffer(context.Background(), "cancel", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes(large); err != nil {
		t.Fatal(err)
	}
	raw, _ = writer.FinishBytes()
	step := &readerCancelContext{remaining: 7}
	reader = Reader{}
	if err := reader.Reset(step, raw, "cancel", 1); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.Bytes(); !errors.Is(err, context.Canceled) || value != nil {
		t.Fatalf("Bytes = %x, %v", value, err)
	}
}

type readerCancelContext struct{ remaining int }

func (*readerCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*readerCancelContext) Done() <-chan struct{}       { return nil }
func (c *readerCancelContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}
func (*readerCancelContext) Value(any) any { return nil }

func TestReaderFloatPreservesExactBits(t *testing.T) {
	value := math.Float64frombits(0x8000000000000000)
	var writer Writer
	_ = writer.ResetBuffer(context.Background(), "float", 1)
	_ = writer.Float64(value)
	raw, _ := writer.FinishBytes()
	var reader Reader
	if err := reader.Reset(context.Background(), raw, "float", 1); err != nil {
		t.Fatal(err)
	}
	got, err := reader.Float64()
	if err != nil || math.Float64bits(got) != math.Float64bits(value) {
		t.Fatalf("Float64 bits = %x, %v", math.Float64bits(got), err)
	}
}
