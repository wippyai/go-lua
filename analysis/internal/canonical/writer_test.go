package canonical

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"testing"
	"time"
)

func TestWriterFramesEveryEventWithoutConcatenationAmbiguity(t *testing.T) {
	left := encodeStrings(t, "ab", "c")
	right := encodeStrings(t, "a", "bc")
	if bytes.Equal(left, right) {
		t.Fatal("length-framed string sequences collided")
	}

	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "domain\x00tail", 300); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.Record(9))
	mustWrite(t, writer.Nil())
	mustWrite(t, writer.Bool(false))
	mustWrite(t, writer.Bool(true))
	mustWrite(t, writer.Uint(math.MaxUint64))
	mustWrite(t, writer.Int(math.MinInt64))
	mustWrite(t, writer.Float64(math.Float64frombits(0x7ff8000000000042)))
	mustWrite(t, writer.Count(7))
	mustWrite(t, writer.String(""))
	mustWrite(t, writer.Bytes(nil))
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	events := parseEvents(t, encoded)
	wantTags := []byte{tagDomain, tagVersion, tagRecord, tagNil, tagBool, tagBool, tagUint, tagInt, tagFloat64, tagCount, tagString, tagBytes}
	if len(events) != len(wantTags) {
		t.Fatalf("event count = %d, want %d", len(events), len(wantTags))
	}
	for index, tag := range wantTags {
		if events[index].tag != tag {
			t.Fatalf("event %d tag = %d, want %d", index, events[index].tag, tag)
		}
	}
	if string(events[0].payload) != "domain\x00tail" {
		t.Fatalf("domain payload = %q", events[0].payload)
	}
	if version, n := binary.Uvarint(events[1].payload); n <= 0 || version != 300 {
		t.Fatalf("version payload = %x, decoded %d/%d", events[1].payload, version, n)
	}
	if len(events[3].payload) != 0 || len(events[10].payload) != 0 || len(events[11].payload) != 0 {
		t.Fatal("zero-length nil/string/bytes payloads were not independently framed")
	}
	sameUint := encodeSingleUintOrCount(t, false, 7)
	sameCount := encodeSingleUintOrCount(t, true, 7)
	if bytes.Equal(sameUint, sameCount) {
		t.Fatal("semantic uint and structural count collided")
	}
}

func TestWriterWireSchemaV1Golden(t *testing.T) {
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "", math.MaxUint64); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.Record(math.MaxUint64))
	mustWrite(t, writer.Nil())
	mustWrite(t, writer.Bool(false))
	mustWrite(t, writer.Bool(true))
	mustWrite(t, writer.Uint(math.MaxUint64))
	mustWrite(t, writer.Int(math.MinInt64))
	mustWrite(t, writer.Float64(math.Inf(-1)))
	mustWrite(t, writer.Count(math.MaxUint64))
	mustWrite(t, writer.String("a\x00b"))
	mustWrite(t, writer.Bytes([]byte{0x00, 0xff}))
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	// External literal: changing an existing tag, framing rule, byte order, or
	// primitive encoding requires an explicit schema decision, not a regenerated
	// expectation.
	const want = "0100" +
		"020affffffffffffffffff01" +
		"030affffffffffffffffff01" +
		"0400" +
		"050100" +
		"050101" +
		"060affffffffffffffffff01" +
		"070affffffffffffffffff01" +
		"0808fff0000000000000" +
		"090affffffffffffffffff01" +
		"0a03610062" +
		"0b0200ff"
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("wire schema drifted:\n got %s\nwant %s", got, want)
	}
}

func TestInitializedWriterCopiesAliasOneBufferedSession(t *testing.T) {
	var original Writer
	if err := original.ResetBuffer(context.Background(), "copy.buffer", 1); err != nil {
		t.Fatal(err)
	}
	copy := original
	mustWrite(t, copy.String("written-through-copy"))
	mustWrite(t, original.Uint(7))
	got, err := copy.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	if again, err := original.FinishBytes(); err != nil || !bytes.Equal(got, again) {
		t.Fatalf("aliased FinishBytes = %x, %v; want %x", again, err, got)
	}
	events := parseEvents(t, got)
	if len(events) != 4 || events[2].tag != tagString || string(events[2].payload) != "written-through-copy" || events[3].tag != tagUint {
		t.Fatalf("copied buffered writer omitted or reordered events: %#v", events)
	}
}

func TestInitializedWriterCopiesAliasOneStreamingSession(t *testing.T) {
	var sink bytes.Buffer
	var original Writer
	if err := original.Reset(context.Background(), &sink, "copy.stream", 1); err != nil {
		t.Fatal(err)
	}
	copy := original
	mustWrite(t, original.String("original"))
	mustWrite(t, copy.String("copy"))
	if err := copy.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := original.Finish(); err != nil {
		t.Fatal(err)
	}
	events := parseEvents(t, sink.Bytes())
	if len(events) != 4 || string(events[2].payload) != "original" || string(events[3].payload) != "copy" {
		t.Fatalf("copied streaming writer did not share one exact stream: %#v", events)
	}
}

func TestZeroWriterCopiesRemainIndependent(t *testing.T) {
	var left Writer
	right := left
	if err := left.ResetBuffer(context.Background(), "left", 1); err != nil {
		t.Fatal(err)
	}
	if err := right.ResetBuffer(context.Background(), "right", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, left.String("L"))
	mustWrite(t, right.String("R"))
	leftBytes, leftErr := left.FinishBytes()
	rightBytes, rightErr := right.FinishBytes()
	if leftErr != nil || rightErr != nil || bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("zero copies were not independent: %x/%v %x/%v", leftBytes, leftErr, rightBytes, rightErr)
	}
}

func TestWriterReleasesGiantOwnedBufferBeforeTinyReuse(t *testing.T) {
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "retention", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.Bytes(make([]byte, 8<<20)))
	large, err := writer.FinishBytes()
	if err != nil || len(large) < 8<<20 {
		t.Fatalf("large FinishBytes length/error = %d/%v", len(large), err)
	}
	if writer.state.buffer.Cap() <= maxRetainedBufferCapacity {
		t.Fatalf("fixture buffer capacity = %d, want above release threshold", writer.state.buffer.Cap())
	}
	large = nil

	if err := writer.ResetBuffer(context.Background(), "tiny", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.Bool(true))
	if _, err := writer.FinishBytes(); err != nil {
		t.Fatal(err)
	}
	if capacity := writer.state.buffer.Cap(); capacity > maxRetainedBufferCapacity {
		t.Fatalf("8MiB buffer retained after tiny reuse: capacity=%d", capacity)
	}
}

func TestWriterBufferStreamAndHashAreByteIdentical(t *testing.T) {
	var buffered Writer
	if err := buffered.ResetBuffer(context.Background(), "stream.identity", 4); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, &buffered)
	want, err := buffered.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}

	var sink bytes.Buffer
	var streamed Writer
	if err := streamed.Reset(context.Background(), &sink, "stream.identity", 4); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, &streamed)
	if err := streamed.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, sink.Bytes()) {
		t.Fatalf("buffer/stream mismatch\n%x\n%x", want, sink.Bytes())
	}

	hash := sha256.New()
	var hashed Writer
	if err := hashed.Reset(context.Background(), hash, "stream.identity", 4); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, &hashed)
	if err := hashed.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := hash.Sum(nil); !bytes.Equal(got, sha256Bytes(want)) {
		t.Fatalf("hash sink covered different bytes: %x, want %x", got, sha256Bytes(want))
	}
	if got, err := hashed.FinishBytes(); !errors.Is(err, ErrNotBuffered) || got != nil {
		t.Fatalf("external FinishBytes = %x, %v", got, err)
	}
}

func TestWriterCancellationReturnsNoBufferedAuthority(t *testing.T) {
	t.Run("first", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var writer Writer
		if err := writer.ResetBuffer(ctx, "cancel.first", 1); !errors.Is(err, context.Canceled) {
			t.Fatalf("ResetBuffer error = %v", err)
		}
		if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
			t.Fatalf("FinishBytes = %x, %v", got, err)
		}
	})

	t.Run("periodic-events", func(t *testing.T) {
		ctx := &cancelAfterChecks{cancelAt: 2}
		var writer Writer
		if err := writer.ResetBuffer(ctx, "cancel.events", 1); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < contextEventPeriod; index++ {
			if err := writer.Uint(uint64(index)); err != nil {
				break
			}
		}
		if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
			t.Fatalf("FinishBytes = %x, %v", got, err)
		}
	})

	t.Run("large-bytes-chunks", func(t *testing.T) {
		ctx := &cancelAfterChecks{cancelAt: 3}
		var writer Writer
		if err := writer.ResetBuffer(ctx, "cancel.bytes", 1); err != nil {
			t.Fatal(err)
		}
		value := bytes.Repeat([]byte{0xa5}, 3*payloadChunkSize)
		if err := writer.Bytes(value); !errors.Is(err, context.Canceled) {
			t.Fatalf("Bytes error = %v", err)
		}
		if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
			t.Fatalf("FinishBytes = %x, %v", got, err)
		}
	})

	t.Run("final", func(t *testing.T) {
		ctx := &cancelAfterChecks{cancelAt: 2}
		var writer Writer
		if err := writer.ResetBuffer(ctx, "cancel.final", 1); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, writer.Uint(1))
		if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
			t.Fatalf("FinishBytes = %x, %v", got, err)
		}
	})
}

func TestWriterExternalPrefixIsNotAuthorityAfterCancellation(t *testing.T) {
	ctx := &cancelAfterChecks{cancelAt: 3}
	var sink bytes.Buffer
	var writer Writer
	if err := writer.Reset(ctx, &sink, "cancel.external", 1); err != nil {
		t.Fatal(err)
	}
	value := bytes.Repeat([]byte{0x6c}, 3*payloadChunkSize)
	if err := writer.Bytes(value); !errors.Is(err, context.Canceled) {
		t.Fatalf("Bytes error = %v", err)
	}
	if sink.Len() == 0 {
		t.Fatal("fixture did not leave the expected non-authoritative external prefix")
	}
	if err := writer.Finish(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Finish error = %v", err)
	}
	if writer.Err() != context.Canceled {
		t.Fatalf("poison error = %v", writer.Err())
	}
}

func TestWriterPoisonIsStickyAndResetRestoresReuse(t *testing.T) {
	wantErr := errors.New("sink failed")
	sink := &failingWriter{remaining: 5, err: wantErr}
	var writer Writer
	if err := writer.Reset(context.Background(), sink, "poison", 1); !errors.Is(err, wantErr) {
		t.Fatalf("Reset error = %v", err)
	}
	written := sink.written
	if err := writer.Uint(1); !errors.Is(err, wantErr) {
		t.Fatalf("post-poison error = %v", err)
	}
	if sink.written != written {
		t.Fatal("poisoned writer emitted additional bytes")
	}
	if err := writer.Finish(); !errors.Is(err, wantErr) {
		t.Fatalf("Finish error = %v", err)
	}

	if err := writer.ResetBuffer(context.Background(), "reused", 2); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.String("healthy"))
	if got, err := writer.FinishBytes(); err != nil || len(got) == 0 {
		t.Fatalf("reused FinishBytes = %x, %v", got, err)
	}
}

func TestWriterReturnedBytesRemainOwnedAcrossReuse(t *testing.T) {
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "ownership", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.String("first"))
	first, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := append([]byte(nil), first...)

	if err := writer.ResetBuffer(context.Background(), "ownership", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.String("a much larger second value that forces different buffer contents"))
	second, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, snapshot) || bytes.Equal(first, second) {
		t.Fatal("returned bytes alias reusable writer storage or distinct streams collided")
	}
	first[0] ^= 0xff
	if bytes.Equal(first, snapshot) || !bytes.Equal(second, mustEncodeOwnershipSecond(t)) {
		t.Fatal("caller mutation leaked into writer reuse")
	}
}

func TestWriterHandlesShortWritesAndRejectsZeroProgress(t *testing.T) {
	var sink bytes.Buffer
	var writer Writer
	if err := writer.Reset(context.Background(), &shortWriter{dst: &sink, max: 1}, "short", 1); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, &writer)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}

	var stuck Writer
	if err := stuck.Reset(context.Background(), zeroWriter{}, "stuck", 1); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero-progress Reset error = %v", err)
	}
	if err := stuck.Finish(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero-progress Finish error = %v", err)
	}
}

func BenchmarkWriter(b *testing.B) {
	payload := bytes.Repeat([]byte("canonical-payload"), 16)
	b.Run("buffer", func(b *testing.B) {
		var writer Writer
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for index := 0; index < b.N; index++ {
			if err := writer.ResetBuffer(context.Background(), "benchmark", 1); err != nil {
				b.Fatal(err)
			}
			benchmarkEvents(b, &writer, payload)
			if value, err := writer.FinishBytes(); err != nil || len(value) == 0 {
				b.Fatal(err)
			}
		}
	})
	b.Run("sha256-stream", func(b *testing.B) {
		hash := sha256.New()
		var writer Writer
		var digest [sha256.Size]byte
		b.ReportAllocs()
		b.SetBytes(int64(len(payload)))
		for index := 0; index < b.N; index++ {
			hash.Reset()
			if err := writer.Reset(context.Background(), hash, "benchmark", 1); err != nil {
				b.Fatal(err)
			}
			benchmarkEvents(b, &writer, payload)
			if err := writer.Finish(); err != nil {
				b.Fatal(err)
			}
			hash.Sum(digest[:0])
		}
	})
}

type framedEvent struct {
	tag     byte
	payload []byte
}

func parseEvents(t testing.TB, encoded []byte) []framedEvent {
	t.Helper()
	var events []framedEvent
	for len(encoded) > 0 {
		tag := encoded[0]
		length, width := binary.Uvarint(encoded[1:])
		if width <= 0 {
			t.Fatalf("malformed event length in %x", encoded)
		}
		header := 1 + width
		if length > uint64(len(encoded)-header) {
			t.Fatalf("event length %d exceeds remaining %d", length, len(encoded)-header)
		}
		end := header + int(length)
		events = append(events, framedEvent{tag: tag, payload: append([]byte(nil), encoded[header:end]...)})
		encoded = encoded[end:]
	}
	return events
}

func encodeStrings(t testing.TB, values ...string) []byte {
	t.Helper()
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "ambiguity", 1); err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		mustWrite(t, writer.String(value))
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func encodeSingleUintOrCount(t testing.TB, count bool, value uint64) []byte {
	t.Helper()
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "ambiguity", 1); err != nil {
		t.Fatal(err)
	}
	if count {
		mustWrite(t, writer.Count(value))
	} else {
		mustWrite(t, writer.Uint(value))
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeFixture(t testing.TB, writer *Writer) {
	t.Helper()
	mustWrite(t, writer.Record(1))
	mustWrite(t, writer.Count(4))
	mustWrite(t, writer.String("a\x00b"))
	mustWrite(t, writer.Int(-42))
	mustWrite(t, writer.Bool(true))
	mustWrite(t, writer.Bytes([]byte{0, 1, 2, 0xff}))
}

func benchmarkEvents(b *testing.B, writer *Writer, payload []byte) {
	b.Helper()
	if err := writer.Record(1); err != nil {
		b.Fatal(err)
	}
	if err := writer.Count(3); err != nil {
		b.Fatal(err)
	}
	if err := writer.String("payload"); err != nil {
		b.Fatal(err)
	}
	if err := writer.Uint(99); err != nil {
		b.Fatal(err)
	}
	if err := writer.Bytes(payload); err != nil {
		b.Fatal(err)
	}
}

func mustEncodeOwnershipSecond(t testing.TB) []byte {
	t.Helper()
	var writer Writer
	if err := writer.ResetBuffer(context.Background(), "ownership", 1); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, writer.String("a much larger second value that forces different buffer contents"))
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mustWrite(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func sha256Bytes(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

type cancelAfterChecks struct {
	checks   int
	cancelAt int
}

func (*cancelAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*cancelAfterChecks) Done() <-chan struct{}       { return nil }
func (*cancelAfterChecks) Value(any) any               { return nil }
func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

type failingWriter struct {
	remaining int
	written   int
	err       error
}

func (w *failingWriter) Write(value []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	count := len(value)
	if count > w.remaining {
		count = w.remaining
	}
	w.remaining -= count
	w.written += count
	if count < len(value) {
		return count, w.err
	}
	return count, nil
}

type shortWriter struct {
	dst *bytes.Buffer
	max int
}

func (w *shortWriter) Write(value []byte) (int, error) {
	if len(value) > w.max {
		value = value[:w.max]
	}
	return w.dst.Write(value)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
