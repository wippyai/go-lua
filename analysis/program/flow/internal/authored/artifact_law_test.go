package authored

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
)

func TestArtifactSectionRoundTripInjectsCountsAndPreservesContentID(t *testing.T) {
	cases := []struct {
		name  string
		input Input
	}{
		{name: "values-tables-functions", input: flowFixtureInput()},
		{name: "access-storage", input: accessStorageFixtureInput()},
		{name: "functions-calls", input: functionCallFixtureInput()},
		{name: "operators", input: operatorFixtureInput()},
		{name: "control", input: controlFixtureInput()},
		{name: "claims", input: claimFixtureInput()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			view := buildArtifactView(t, test.input)
			first := encodeArtifactSection(t, view)
			second := encodeArtifactSection(t, view)
			if !bytes.Equal(first, second) {
				t.Fatal("equal authored components produced different artifact bytes")
			}

			reader, err := framing.NewReader(first, len(first))
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Header("program/flow-test", 1); err != nil {
				t.Fatal(err)
			}
			decoded, err := ReadArtifactSection(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Finish(); err != nil {
				t.Fatal(err)
			}
			if decoded.Counts != ([keyspace.FamilyCount]uint32{}) {
				t.Fatalf("decoded Counts = %#v; want root-injection zero", decoded.Counts)
			}
			decoded.Counts = test.input.Counts
			replayed := buildArtifactView(t, decoded)
			if replayed.Cold().ContentID() != view.Cold().ContentID() {
				t.Fatalf("replayed ContentID = %x; want %x", replayed.Cold().ContentID(), view.Cold().ContentID())
			}
		})
	}
}

func TestArtifactSectionPayloadOnlyLeavesSentinelsAndUsesNineRecords(t *testing.T) {
	view := buildArtifactView(t, Input{})
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(0x51); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, view); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(0x52); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if records, err := canonicalRecordCount(buffer.Bytes()); err != nil || records != 11 {
		t.Fatalf("canonical record count = %d, %v; want 11 (two sentinels plus nine authored records)", records, err)
	}

	reader, err := framing.NewReader(buffer.Bytes(), buffer.Len())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if got, err := reader.Record(); err != nil || got != 0x51 {
		t.Fatalf("prefix sentinel = %d, %v; want 0x51", got, err)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Counts != ([keyspace.FamilyCount]uint32{}) {
		t.Fatalf("decoded Counts = %#v; want zero", decoded.Counts)
	}
	if got, err := reader.Record(); err != nil || got != 0x52 {
		t.Fatalf("suffix sentinel = %d, %v; want 0x52", got, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactSectionRejectsMalformedTermCountRangeEnumOrderAndTruncation(t *testing.T) {
	cases := []struct {
		name  string
		write func(*framing.Writer)
	}{
		{
			name: "term-family-zero-with-ordinal",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(1)
				_ = writer.Uint(0)
				_ = writer.Uint(1)
			},
		},
		{
			name: "term-ordinal-over-max",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(1)
				_ = writer.Uint(uint64(keyspace.FamilyNil))
				_ = writer.Uint(uint64(keyspace.MaxTermOrdinal) + 1)
			},
		},
		{
			name: "count-over-max",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(uint64(keyspace.MaxTermOrdinal) + 1)
			},
		},
		{
			name: "range-end-out-of-pool",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(0)
				_ = writer.Count(1)
				writeZeroTerm(writer)
				writeZeroTerm(writer)
				_ = writer.Uint(0)
				_ = writer.Uint(1)
			},
		},
		{
			name: "range-end-over-max",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(0)
				_ = writer.Count(1)
				writeZeroTerm(writer)
				writeZeroTerm(writer)
				_ = writer.Uint(0)
				_ = writer.Uint(uint64(keyspace.MaxTermOrdinal) + 1)
			},
		},
		{
			name: "invalid-field-kind",
			write: func(writer *framing.Writer) {
				_ = writer.Record(1)
				_ = writer.Count(0)
				_ = writer.Count(0)
				_ = writer.Record(2)
				_ = writer.Count(1)
				writeZeroTerm(writer)
				writeZeroTerm(writer)
				writeZeroTerm(writer)
				_ = writer.Uint(5)
			},
		},
		{
			name:  "record-order",
			write: func(writer *framing.Writer) { _ = writer.Record(2) },
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := readMalformedArtifact(test.write); err == nil {
				t.Fatal("malformed artifact section was accepted")
			}
		})
	}

	valid := encodeArtifactSection(t, buildArtifactView(t, Input{}))
	if len(valid) < 2 {
		t.Fatal("valid artifact section unexpectedly short")
	}
	truncated := valid[:len(valid)-1]
	reader, err := framing.NewReader(truncated, len(truncated))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArtifactSection(reader); err == nil {
		t.Fatal("truncated artifact section was accepted")
	}
}

func TestArtifactSectionRejectsNoncanonicalAndRemainingHostilePayloads(t *testing.T) {
	if err := readMalformedArtifact(func(writer *framing.Writer) {
		_ = writer.Record(1)
		_ = writer.Count(uint64(keyspace.MaxTermOrdinal))
	}); err == nil {
		t.Fatal("hostile Remaining count was accepted")
	}

	var header bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&header, "program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	// Record(1) with an overlong uvarint payload is not framing.
	noncanonical := append(append([]byte(nil), header.Bytes()...), 3, 2, 0x81, 0x00)
	reader, err := framing.NewReader(noncanonical, len(noncanonical))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArtifactSection(reader); err == nil {
		t.Fatal("noncanonical uvarint was accepted")
	}
}

func TestArtifactSectionPreflightsMalformedFirstAndLastWithoutRowAllocation(t *testing.T) {
	const rowCount = 1 << 18
	for _, malformedLast := range []bool{false, true} {
		name := "malformed-first"
		if malformedLast {
			name = "malformed-last"
		}
		t.Run(name, func(t *testing.T) {
			data := hostileValuesArtifact(rowCount, malformedLast)
			if allocation := artifactDecodeAllocation(data); allocation > 4<<20 {
				t.Fatalf("malformed %s decode allocated %d bytes; want preflight before row allocation", name, allocation)
			}
		})
	}
}

func flowFixtureInput() Input {
	input, _ := flowFixture()
	return input
}

func accessStorageFixtureInput() Input {
	input, _ := accessStorageFixture()
	return input
}

func functionCallFixtureInput() Input {
	input, _ := functionCallFixture()
	return input
}

func operatorFixtureInput() Input {
	input, _ := operatorFixture()
	return input
}

func controlFixtureInput() Input {
	input, _ := controlFixture()
	return input
}

func claimFixtureInput() Input {
	input, _ := claimFixture()
	return input
}

func buildArtifactView(t *testing.T, input Input) View {
	t.Helper()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	view, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func encodeArtifactSection(t *testing.T, view View) []byte {
	t.Helper()
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, view); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func readMalformedArtifact(write func(*framing.Writer)) error {
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/flow-test", 1); err != nil {
		return err
	}
	write(&writer)
	if err := writer.Finish(); err != nil {
		return err
	}
	reader, err := framing.NewReader(buffer.Bytes(), buffer.Len())
	if err != nil {
		return err
	}
	if err := reader.Header("program/flow-test", 1); err != nil {
		return err
	}
	_, err = ReadArtifactSection(reader)
	return err
}

func hostileValuesArtifact(rowCount int, malformedLast bool) []byte {
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/flow-test", 1); err != nil {
		return nil
	}
	_ = writer.Record(1)
	_ = writer.Count(0)
	_ = writer.Count(uint64(rowCount))
	for index := 0; index < rowCount; index++ {
		malformed := index == 0
		if malformedLast {
			malformed = index == rowCount-1
		}
		if malformed {
			_ = writer.Uint(0)
			_ = writer.Uint(1)
		} else {
			writeZeroTerm(&writer)
		}
		writeZeroTerm(&writer)
		_ = writer.Uint(0)
		_ = writer.Uint(0)
	}
	_ = writer.Finish()
	return append([]byte(nil), buffer.Bytes()...)
}

func artifactDecodeAllocation(data []byte) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 3 {
		reader, err := framing.NewReader(data, len(data))
		if err != nil {
			continue
		}
		if err := reader.Header("program/flow-test", 1); err != nil {
			continue
		}
		_, _ = ReadArtifactSection(reader)
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / 3
}

func canonicalRecordCount(data []byte) (int, error) {
	count := 0
	for offset := 0; offset < len(data); {
		tag := data[offset]
		offset++
		length, size := binary.Uvarint(data[offset:])
		if size <= 0 {
			return 0, errInvalidArtifactSection
		}
		offset += size
		if length > uint64(len(data)-offset) {
			return 0, errInvalidArtifactSection
		}
		offset += int(length)
		if tag == 3 {
			count++
		}
	}
	return count, nil
}

func writeZeroTerm(writer *framing.Writer) {
	_ = writer.Uint(0)
	_ = writer.Uint(0)
}
