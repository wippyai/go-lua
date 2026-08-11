package canonical

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestWriterFramesVersionedEventsAndStreamsShortWrites(t *testing.T) {
	var sink shortWriter
	var writer Writer
	if err := writer.Reset(&sink, "program/test", 1); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if err := writer.Record(9); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := writer.Bytes([]byte{0, 1, 2}); err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	// domain, version, record, and byte payload are independently framed.
	want := []byte{tagDomain, 12, 'p', 'r', 'o', 'g', 'r', 'a', 'm', '/', 't', 'e', 's', 't', tagVersion, 1, 1, tagRecord, 1, 9, tagBytes, 3, 0, 1, 2}
	if !bytes.Equal(sink.Bytes(), want) {
		t.Fatalf("stream = %x, want %x", sink.Bytes(), want)
	}
	if err := writer.Uint(1); !errors.Is(err, ErrFinished) {
		t.Fatalf("write after Finish = %v, want ErrFinished", err)
	}
}

func TestStringEncodingIsByteIdenticalAndConstantAllocation(t *testing.T) {
	var sink shortWriter
	var writer Writer
	if err := writer.Reset(&sink, "d", 1); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if err := writer.String("\x00key\xff"); err != nil {
		t.Fatalf("String: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	want := []byte{tagDomain, 1, 'd', tagVersion, 1, 1, tagString, 5, 0, 'k', 'e', 'y', 0xff}
	if !bytes.Equal(sink.Bytes(), want) {
		t.Fatalf("string bytes = %x, want %x", sink.Bytes(), want)
	}

	measure := func(width int) float64 {
		return testing.AllocsPerRun(100, func() {
			var sink nonStringSink
			var writer Writer
			if err := writer.Reset(&sink, "d", 1); err != nil {
				panic(err)
			}
			for index := 0; index < width; index++ {
				if err := writer.String("repeated-key"); err != nil {
					panic(err)
				}
			}
			if err := writer.Finish(); err != nil {
				panic(err)
			}
		})
	}
	narrow, wide := measure(1), measure(4096)
	if wide != narrow {
		t.Fatalf("String allocations scale with width: narrow=%f wide=%f", narrow, wide)
	}
}

func TestStringFallbackChunksPreserveFrameAndStopOnPayloadFailure(t *testing.T) {
	payload := strings.Repeat("chunk/", 120) // 720 bytes: crosses two scratch boundaries.
	var sink shortWriter                     // deliberately does not implement io.StringWriter.
	var writer Writer
	if err := writer.Reset(&sink, "d", 1); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if err := writer.String(payload); err != nil {
		t.Fatalf("String: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	want := []byte{tagDomain, 1, 'd', tagVersion, 1, 1, tagString}
	want = binary.AppendUvarint(want, uint64(len(payload)))
	want = append(want, payload...)
	if !bytes.Equal(sink.Bytes(), want) {
		t.Fatalf("chunked frame differs: got %d bytes, want %d", len(sink.Bytes()), len(want))
	}

	// Reset consumes six bytes. The next three are String's header; this then
	// fails partway through the second fixed-scratch payload chunk.
	failing := &chunkFaultSink{remaining: 6 + 3 + stringScratchSize + 17}
	writer = Writer{}
	if err := writer.Reset(failing, "d", 1); err != nil {
		t.Fatalf("failure sink Reset: %v", err)
	}
	if err := writer.String(payload); !errors.Is(err, errChunkPayload) {
		t.Fatalf("chunk payload error = %v", err)
	}
	if err := writer.String("later"); !errors.Is(err, errChunkPayload) {
		t.Fatalf("writer resumed after payload error: %v", err)
	}
	if err := writer.Finish(); !errors.Is(err, errChunkPayload) {
		t.Fatalf("Finish after payload error = %v", err)
	}
}

func TestWriterRejectsUninitializedAndFailedResetWithoutStaleStream(t *testing.T) {
	var writer Writer
	if err := writer.Finish(); !errors.Is(err, ErrNilDestination) {
		t.Fatalf("uninitialized Finish = %v, want ErrNilDestination", err)
	}
	var sink bytes.Buffer
	if err := writer.Reset(&sink, "first", 1); err != nil {
		t.Fatalf("first Reset: %v", err)
	}
	if err := writer.Reset(nil, "bad", 1); !errors.Is(err, ErrNilDestination) {
		t.Fatalf("failed Reset = %v, want ErrNilDestination", err)
	}
	if err := writer.Uint(1); !errors.Is(err, ErrNilDestination) {
		t.Fatalf("write after failed Reset = %v, want ErrNilDestination", err)
	}
	if err := writer.Finish(); !errors.Is(err, ErrNilDestination) {
		t.Fatalf("Finish after failed Reset = %v, want ErrNilDestination", err)
	}
}

func TestWriterStopsAfterPartialWriteErrorOrZeroProgress(t *testing.T) {
	for _, sink := range []struct {
		name string
		io   *faultWriter
	}{
		{name: "partial error", io: &faultWriter{limit: 3, err: errors.New("sink failed")}},
		{name: "zero progress", io: &faultWriter{limit: 0}},
	} {
		t.Run(sink.name, func(t *testing.T) {
			var writer Writer
			if err := writer.Reset(sink.io, "program/test", 1); err == nil {
				t.Fatal("Reset unexpectedly succeeded")
			}
			if err := writer.Uint(1); err == nil {
				t.Fatal("writer accepted an event after sink failure")
			}
		})
	}
}

func TestReaderRequiresExactFramingAndCompleteConsumption(t *testing.T) {
	var data bytes.Buffer
	var writer Writer
	if err := writer.Reset(&data, "reader/law", 7); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(4); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(99); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}

	reader, err := NewReader(data.Bytes(), len(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("reader/law", 7); err != nil {
		t.Fatal(err)
	}
	if record, err := reader.Record(); err != nil || record != 4 {
		t.Fatalf("Record = %d/%v", record, err)
	}
	if value, err := reader.Uint(); err != nil || value != 99 {
		t.Fatalf("Uint = %d/%v", value, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}

	for _, malformed := range [][]byte{
		data.Bytes()[:len(data.Bytes())-1],
		append(append([]byte(nil), data.Bytes()...), 0),
		{tagDomain, 0x80},
		{tagDomain, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	} {
		reader, err := NewReader(malformed, len(malformed))
		if err != nil {
			continue
		}
		if err := reader.Header("reader/law", 7); err == nil {
			if _, err := reader.Record(); err == nil {
				if _, err := reader.Uint(); err == nil && reader.Finish() == nil {
					t.Fatal("malformed stream was accepted")
				}
			}
		}
	}
}

func TestReaderEnforcesDeclaredPayloadLimit(t *testing.T) {
	data := []byte{tagBytes, 3, 1, 2, 3}
	reader, err := NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Bytes(2); !errors.Is(err, ErrLimit) {
		t.Fatalf("Bytes limit = %v", err)
	}
	if _, err := NewReader(data, len(data)-1); !errors.Is(err, ErrLimit) {
		t.Fatalf("stream limit = %v", err)
	}
}

type shortWriter struct{ data bytes.Buffer }

func (w *shortWriter) Write(value []byte) (int, error) {
	if len(value) > 1 {
		value = value[:1]
	}
	return w.data.Write(value)
}

func (w *shortWriter) Bytes() []byte { return w.data.Bytes() }

type nonStringSink struct{ count int }

func (w *nonStringSink) Write(value []byte) (int, error) {
	w.count += len(value)
	return len(value), nil
}

var errChunkPayload = errors.New("chunk payload failure")

type chunkFaultSink struct{ remaining int }

func (w *chunkFaultSink) Write(value []byte) (int, error) {
	if w.remaining == 0 {
		return 0, errChunkPayload
	}
	if len(value) <= w.remaining {
		w.remaining -= len(value)
		return len(value), nil
	}
	n := w.remaining
	w.remaining = 0
	return n, errChunkPayload
}

type faultWriter struct {
	limit int
	err   error
}

func (w *faultWriter) Write(value []byte) (int, error) {
	if w.limit == 0 {
		return 0, w.err
	}
	n := w.limit
	if n > len(value) {
		n = len(value)
	}
	w.limit -= n
	return n, w.err
}
