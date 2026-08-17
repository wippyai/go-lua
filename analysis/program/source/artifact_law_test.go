package source

import (
	"bytes"
	"encoding/binary"
	"errors"
	"runtime"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactSectionRoundTripRebuildsAuthoredContent(t *testing.T) {
	input, index := contentFixture()
	component := finalizeSource(t, input, index)

	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, "source/artifact-law", 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil {
		t.Fatalf("WriteArtifactSection: %v", err)
	}
	// The section is payload-only: a parent may append its next event before
	// finishing the enclosing stream.
	if err := writer.Record(99); err != nil || writer.Finish() != nil {
		t.Fatalf("finish enclosing stream: %v", err)
	}

	reader, err := framing.NewReader(data.Bytes(), len(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("source/artifact-law", 1); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	if record, err := reader.Record(); err != nil || record != 99 {
		t.Fatalf("following parent event = %d/%v", record, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := len(decoded.Families); got != int(keyspace.FamilyCount-1) {
		t.Fatalf("decoded family rows = %d, want %d", got, keyspace.FamilyCount-1)
	}
	for _, family := range decoded.Families {
		if family.Family == keyspace.FamilyOutcome && len(family.Spans) != 0 {
			t.Fatal("decoded authored section retained derived Outcome spans")
		}
	}
	rebuilt := draftContentID(t, decoded)
	if rebuilt != component.Cold().ContentID() {
		t.Fatalf("rebuild ContentID = %x, want %x", rebuilt, component.Cold().ContentID())
	}
}

func TestArtifactSectionExcludesCommittedOutcomeAndIndex(t *testing.T) {
	input, ordinary := sourceFixture(1)
	withoutOutcome := finalizeSource(t, input, ordinary)
	withOutcomeIndex := ordinary
	withOutcomeIndex.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)}
	withOutcome := finalizeSource(t, input, withOutcomeIndex)

	encode := func(component *Component) []byte {
		var data bytes.Buffer
		var writer framing.Writer
		if err := writer.Reset(&data, "source/outcome-law", 1); err != nil {
			t.Fatal(err)
		}
		if err := WriteArtifactSection(&writer, component.View()); err != nil {
			t.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		return append([]byte(nil), data.Bytes()...)
	}
	if left, right := encode(withoutOutcome), encode(withOutcome); !bytes.Equal(left, right) {
		t.Fatal("committed Outcome/index projection changed authored Source section")
	}
}

func TestArtifactSectionRejectsTruncation(t *testing.T) {
	input, index := keyFaultFixture()
	component := finalizeSource(t, input, index)
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, "source/mutation-law", 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil || writer.Finish() != nil {
		t.Fatalf("write section: %v", err)
	}
	raw := data.Bytes()
	for length := len(raw) - 1; length >= 0; length-- {
		reader, err := framing.NewReader(raw[:length], length)
		if err != nil {
			continue
		}
		if err := reader.Header("source/mutation-law", 1); err != nil {
			continue
		}
		if _, err := ReadArtifactSection(reader); err == nil {
			t.Fatalf("truncated payload of %d bytes was accepted", length)
		}
	}
}

func TestArtifactSectionRejectsNoncanonicalDuplicateAndOutOfOrderExactAtoms(t *testing.T) {
	input, index := keyFaultFixture()
	component := finalizeSource(t, input, index)
	raw := encodeSourceArtifactLaw(t, component, "source/exact-law")
	offsets := sourceArtifactMutationOffsets(t, raw)
	if len(offsets.exactKinds) < 1 || len(offsets.exactStrings) != 2 {
		t.Fatalf("exact atom offsets = %#v, want integer plus two strings", offsets)
	}

	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{
			name: "noncanonical integral float",
			mutate: func(data []byte) {
				// The first exact atom is integer 1. Re-labeling its same-width
				// payload as float 1 violates scalar.Normalize.
				data[offsets.exactKinds[0]] = byte(keyspace.LiteralFloat)
			},
		},
		{
			name: "duplicate",
			mutate: func(data []byte) {
				copy(data[offsets.exactStrings[1]:], data[offsets.exactStrings[0]:offsets.exactStrings[0]+1])
			},
		},
		{
			name: "out of order",
			mutate: func(data []byte) {
				data[offsets.exactStrings[0]], data[offsets.exactStrings[1]] = data[offsets.exactStrings[1]], data[offsets.exactStrings[0]]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := append([]byte(nil), raw...)
			test.mutate(mutated)
			decoded, err := decodeSourceArtifactLaw(t, mutated, "source/exact-law")
			if err == nil {
				t.Fatalf("accepted malformed exact atoms: %#v", decoded)
			}
			if decoded.Name != "" || len(decoded.Families) != 0 || len(decoded.ExactAtoms) != 0 {
				t.Fatalf("malformed exact decode returned partial Input: %#v", decoded)
			}
		})
	}
}

func TestArtifactSectionRejectsInvalidDenseExactKeyOrdinal(t *testing.T) {
	input, index := keyFaultFixture()
	component := finalizeSource(t, input, index)
	raw := encodeSourceArtifactLaw(t, component, "source/key-ordinal-law")
	offsets := sourceArtifactMutationOffsets(t, raw)
	if len(offsets.keyExactOrdinals) == 0 {
		t.Fatal("key exact ordinal offsets are empty")
	}
	mutated := append([]byte(nil), raw...)
	overwriteArtifactUint(t, mutated, offsets.keyExactOrdinals[0], 0)
	decoded, err := decodeSourceArtifactLaw(t, mutated, "source/key-ordinal-law")
	if err == nil {
		t.Fatal("artifact decoder accepted zero dense exact Key ordinal")
	}
	if decoded.Name != "" || len(decoded.Families) != 0 || len(decoded.Keys) != 0 {
		t.Fatalf("invalid dense exact Key decode returned partial Input: %#v", decoded)
	}
}

func TestArtifactSectionRejectsMalformedTermRangeAndFaultKind(t *testing.T) {
	input, index := keyFaultFixture()
	component := finalizeSource(t, input, index)
	raw := encodeSourceArtifactLaw(t, component, "source/semantic-law")
	offsets := sourceArtifactMutationOffsets(t, raw)
	tests := []struct {
		name   string
		offset int
		value  uint64
	}{
		{name: "invalid term family", offset: offsets.bodyFirstTerm, value: uint64(1 << 8)},
		{name: "range row count drift", offset: offsets.bodyFirstRangeCount, value: 8},
		{name: "fault enum", offset: offsets.faultKind, value: 99},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := append([]byte(nil), raw...)
			overwriteArtifactUint(t, mutated, test.offset, test.value)
			decoded, err := decodeSourceArtifactLaw(t, mutated, "source/semantic-law")
			if err == nil {
				t.Fatalf("accepted malformed %s: %#v", test.name, decoded)
			}
			if decoded.Name != "" || len(decoded.Families) != 0 {
				t.Fatalf("malformed %s decode returned partial Input: %#v", test.name, decoded)
			}
		})
	}
}

func TestArtifactSectionRejectsImportDirectBodyTerm(t *testing.T) {
	input, index := exactDirectBodyFixture()
	for at := range input.Families {
		if input.Families[at].Family == keyspace.FamilyImport {
			input.Families[at].Spans = []Span{{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}}
		}
	}
	component := finalizeSource(t, input, index)
	raw := encodeSourceArtifactLaw(t, component, "source/import-law")
	offsets := sourceArtifactMutationOffsets(t, raw)
	mutated := append([]byte(nil), raw...)
	overwriteArtifactUint(t, mutated, offsets.bodyFirstTerm, uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)))
	decoded, err := decodeSourceArtifactLaw(t, mutated, "source/import-law")
	if err == nil {
		t.Fatalf("artifact decoder accepted Import in direct Body order: %#v", decoded)
	}
	if decoded.Name != "" || len(decoded.Families) != 0 {
		t.Fatalf("malformed Import decode returned partial Input: %#v", decoded)
	}
}

func TestArtifactSectionRejectsNonDirectBodyFamilies(t *testing.T) {
	input, index := exactDirectBodyFixture()
	for at := range input.Families {
		family := input.Families[at].Family
		if family == keyspace.FamilyOutcome || sourceDirectFamily(family) || len(input.Families[at].Spans) != 0 {
			continue
		}
		input.Families[at].Spans = []Span{{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}}
	}
	component := finalizeSource(t, input, index)
	raw := encodeSourceArtifactLaw(t, component, "source/non-direct-body-law")
	offsets := sourceArtifactMutationOffsets(t, raw)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome || sourceDirectFamily(family) {
			continue
		}
		t.Run("family-"+strconv.Itoa(int(family)), func(t *testing.T) {
			mutated := append([]byte(nil), raw...)
			overwriteArtifactUint(t, mutated, offsets.bodyFirstTerm, uint64(keyspace.MakeTerm(family, 1)))
			decoded, err := decodeSourceArtifactLaw(t, mutated, "source/non-direct-body-law")
			if err == nil {
				t.Fatalf("artifact decoder accepted non-direct Body family %d: %#v", family, decoded)
			}
			if decoded.Name != "" || len(decoded.Families) != 0 {
				t.Fatalf("malformed family %d decode returned partial Input: %#v", family, decoded)
			}
		})
	}
}

func TestArtifactSectionPreflightsMalformedFirstAndLastWithoutAllocation(t *testing.T) {
	const (
		spanCount = 100_000
		runs      = 2
	)
	for _, badRow := range []int{0, spanCount - 1} {
		t.Run(map[int]string{0: "first", spanCount - 1: "last"}[badRow], func(t *testing.T) {
			data := largeMalformedSpanArtifact(t, spanCount, badRow)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range runs {
				decoded, err := decodeSourceArtifactLaw(t, data, "source/preflight-law")
				if err == nil {
					t.Fatal("malformed large span payload was accepted")
				}
				if decoded.Name != "" || len(decoded.Families) != 0 {
					t.Fatalf("malformed large span decode returned partial Input: %#v", decoded)
				}
			}
			runtime.ReadMemStats(&after)
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(runs)*(1<<20) {
				t.Fatalf("malformed %s payload allocated %d bytes; want bounded full-section preflight", map[int]string{0: "first", spanCount - 1: "last"}[badRow], allocated)
			}
		})
	}
}

func TestArtifactSectionPreflightsMalformedFinalFaultWithoutPriorAllocation(t *testing.T) {
	const (
		nilCount = 100_000
		runs     = 2
	)
	for _, badFault := range []int{0, 1} {
		name := "first"
		if badFault == 1 {
			name = "last"
		}
		t.Run(name, func(t *testing.T) {
			data := largeMalformedFaultArtifact(t, nilCount, badFault)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range runs {
				decoded, err := decodeSourceArtifactLaw(t, data, "source/full-preflight-law")
				if err == nil {
					t.Fatal("malformed fault payload was accepted")
				}
				if decoded.Name != "" || len(decoded.Families) != 0 || len(decoded.Nil) != 0 {
					t.Fatalf("malformed fault decode returned partial Input: %#v", decoded)
				}
			}
			runtime.ReadMemStats(&after)
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(runs)*(1<<20) {
				t.Fatalf("malformed %s fault payload allocated %d bytes; want full-section preflight", name, allocated)
			}
		})
	}
}

func TestArtifactSectionRejectsSpanCountLimitBeforeAllocation(t *testing.T) {
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, "source/limit-law", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(1); err != nil || writer.String("limit.lua") != nil {
		t.Fatal("write identity prefix")
	}
	if err := writer.Count(1); err != nil || writer.Record(2) != nil || writer.Uint(uint64(keyspace.FamilyNil)) != nil {
		t.Fatal("write span prefix")
	}
	if err := writer.Count(uint64(keyspace.MaxTermOrdinal) + 1); err != nil || writer.Finish() != nil {
		t.Fatal("write hostile count")
	}
	reader, err := framing.NewReader(data.Bytes(), len(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("source/limit-law", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadArtifactSection(reader); !errors.Is(err, framing.ErrLimit) {
		t.Fatalf("hostile span count error = %v, want canonical limit", err)
	}
}

type artifactMutationOffsets struct {
	exactKinds          []int
	exactStrings        []int
	keyExactOrdinals    []int
	bodyFirstRangeCount int
	bodyFirstTerm       int
	faultKind           int
}

type artifactEvent struct {
	tag          byte
	payloadStart int
	payloadEnd   int
}

func encodeSourceArtifactLaw(t *testing.T, component *Component, domain string) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func decodeSourceArtifactLaw(t *testing.T, data []byte, domain string) (Input, error) {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(domain, 1); err != nil {
		t.Fatal(err)
	}
	return ReadArtifactSection(reader)
}

func sourceArtifactMutationOffsets(t *testing.T, data []byte) artifactMutationOffsets {
	t.Helper()
	var offsets artifactMutationOffsets
	position := 0
	for position < len(data) {
		event, ok := nextArtifactEvent(data, &position)
		if !ok {
			t.Fatalf("invalid canonical event at byte %d", position)
		}
		if event.tag == 3 {
			record, ok := artifactEventUint(data, event)
			if !ok {
				t.Fatalf("record event at byte %d is not a Uint", event.payloadStart)
			}
			switch record {
			case sourceArtifactRecordOrder:
				// Record 4 starts Body tag, Body range count, and its
				// first row count. The next Uint is the first direct Term.
				family := takeArtifactEvent(t, data, &position)
				if family.tag != 5 {
					t.Fatalf("order family event tag = %d, want Uint", family.tag)
				}
				bodyCount := takeArtifactEvent(t, data, &position)
				if bodyCount.tag != 4 {
					t.Fatalf("order Body count event tag = %d, want Count", bodyCount.tag)
				}
				firstRange := takeArtifactEvent(t, data, &position)
				if firstRange.tag != 4 {
					t.Fatalf("order first range event tag = %d, want Count", firstRange.tag)
				}
				offsets.bodyFirstRangeCount = firstRange.payloadStart
				firstTerm := takeArtifactEvent(t, data, &position)
				if firstTerm.tag != 5 {
					t.Fatalf("order first term event tag = %d, want Uint", firstTerm.tag)
				}
				offsets.bodyFirstTerm = firstTerm.payloadStart
				// Continue scanning from the event after the first term;
				// the metadata offsets are all that this branch needs.
			case sourceArtifactRecordKeys:
				exactCount := artifactCountEvent(t, data, &position)
				keyCount := artifactCountEvent(t, data, &position)
				faultCount := artifactCountEvent(t, data, &position)
				for index := 0; index < exactCount; index++ {
					kind := takeArtifactEvent(t, data, &position)
					if kind.tag != 5 {
						t.Fatalf("exact atom %d kind tag = %d, want Uint", index, kind.tag)
					}
					offsets.exactKinds = append(offsets.exactKinds, kind.payloadStart)
					kindValue, ok := artifactEventUint(data, kind)
					if !ok {
						t.Fatalf("exact atom %d kind is not canonical Uint", index)
					}
					switch keyspace.LiteralKind(kindValue) {
					case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralFloat:
						takeArtifactEvent(t, data, &position)
					case keyspace.LiteralString:
						value := takeArtifactEvent(t, data, &position)
						if value.tag != 8 || value.payloadEnd-value.payloadStart != 1 {
							t.Fatalf("exact atom %d string payload = %d bytes, want one", index, value.payloadEnd-value.payloadStart)
						}
						offsets.exactStrings = append(offsets.exactStrings, value.payloadStart)
					default:
						t.Fatalf("exact atom %d has invalid kind %d", index, kindValue)
					}
				}
				for range keyCount {
					takeArtifactEvent(t, data, &position) // owner
					takeArtifactEvent(t, data, &position) // form
					exact := takeArtifactEvent(t, data, &position)
					if exact.tag != 5 {
						t.Fatalf("key exact ordinal tag = %d, want Uint", exact.tag)
					}
					offsets.keyExactOrdinals = append(offsets.keyExactOrdinals, exact.payloadStart)
				}
				if faultCount > 0 {
					takeArtifactEvent(t, data, &position) // owner
					kind := takeArtifactEvent(t, data, &position)
					if kind.tag != 5 {
						t.Fatalf("fault kind tag = %d, want Uint", kind.tag)
					}
					offsets.faultKind = kind.payloadStart
				}
				return offsets
			}
		}
	}
	t.Fatalf("Source artifact mutation offsets not found")
	return offsets
}

func nextArtifactEvent(data []byte, position *int) (artifactEvent, bool) {
	if position == nil || *position < 0 || *position >= len(data) {
		return artifactEvent{}, false
	}
	start := *position
	tag := data[start]
	if tag < 1 || tag > 8 {
		return artifactEvent{}, false
	}
	start++
	length, size := binary.Uvarint(data[start:])
	if size <= 0 {
		return artifactEvent{}, false
	}
	payloadStart := start + size
	if payloadStart < start || length > uint64(len(data)-payloadStart) {
		return artifactEvent{}, false
	}
	payloadEnd := payloadStart + int(length)
	*position = payloadEnd
	return artifactEvent{tag: tag, payloadStart: payloadStart, payloadEnd: payloadEnd}, true
}

func takeArtifactEvent(t *testing.T, data []byte, position *int) artifactEvent {
	t.Helper()
	event, ok := nextArtifactEvent(data, position)
	if !ok {
		t.Fatalf("invalid canonical event at byte %d", *position)
	}
	return event
}

func artifactEventUint(data []byte, event artifactEvent) (uint64, bool) {
	if event.tag != 3 && event.tag != 5 {
		return 0, false
	}
	value, size := binary.Uvarint(data[event.payloadStart:event.payloadEnd])
	return value, size > 0 && event.payloadStart+size == event.payloadEnd
}

func artifactCountEvent(t *testing.T, data []byte, position *int) int {
	t.Helper()
	event := takeArtifactEvent(t, data, position)
	if event.tag != 4 {
		t.Fatalf("count event tag = %d, want Count", event.tag)
	}
	value, size := binary.Uvarint(data[event.payloadStart:event.payloadEnd])
	if size <= 0 || event.payloadStart+size != event.payloadEnd || value > uint64(^uint(0)>>1) {
		t.Fatalf("invalid count payload %d:%d", event.payloadStart, event.payloadEnd)
	}
	return int(value)
}

func overwriteArtifactUint(t *testing.T, data []byte, payloadStart int, value uint64) {
	t.Helper()
	position := 0
	for position < len(data) {
		event := takeArtifactEvent(t, data, &position)
		if event.payloadStart != payloadStart {
			continue
		}
		var encoded [binary.MaxVarintLen64]byte
		size := binary.PutUvarint(encoded[:], value)
		if size != event.payloadEnd-event.payloadStart {
			t.Fatalf("replacement value %d has width %d, want %d", value, size, event.payloadEnd-event.payloadStart)
		}
		copy(data[event.payloadStart:event.payloadEnd], encoded[:size])
		return
	}
	t.Fatalf("payload offset %d not found", payloadStart)
}

func largeMalformedSpanArtifact(t *testing.T, spanCount, badRow int) []byte {
	t.Helper()
	const domain = "source/preflight-law"
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(sourceArtifactRecordIdentity); err != nil || writer.String("large.lua") != nil || writer.Count(uint64(spanCount+1)) != nil {
		t.Fatal("write large identity")
	}
	if err := writer.Record(sourceArtifactRecordSpans); err != nil {
		t.Fatal(err)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		if err := writer.Uint(uint64(family)); err != nil {
			t.Fatal(err)
		}
		count := 0
		if family == keyspace.FamilyNil {
			count = spanCount
		} else if family == keyspace.FamilyBody {
			count = 1
		}
		if err := writer.Count(uint64(count)); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < count; index++ {
			bad := family == keyspace.FamilyNil && index == badRow
			if bad {
				if writer.Uint(0) != nil || writer.Uint(1) != nil || writer.Uint(1) != nil || writer.Uint(1) != nil {
					t.Fatal("write malformed span")
				}
				continue
			}
			line := uint64(index + 1)
			if writer.Uint(line) != nil || writer.Uint(1) != nil || writer.Uint(line) != nil || writer.Uint(1) != nil {
				t.Fatal("write span")
			}
		}
	}
	if err := writer.Record(sourceArtifactRecordLiterals); err != nil {
		t.Fatal(err)
	}
	for _, family := range []keyspace.Family{keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString} {
		if writer.Uint(uint64(family)) != nil {
			t.Fatal("write literal family")
		}
		count := 0
		if family == keyspace.FamilyNil {
			count = spanCount
		}
		if writer.Count(uint64(count)) != nil {
			t.Fatal("write literal count")
		}
		for range count {
			if writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyBody, 1))) != nil {
				t.Fatal("write literal owner")
			}
		}
	}
	if err := writer.Record(sourceArtifactRecordOrder); err != nil || writer.Uint(uint64(keyspace.FamilyBody)) != nil || writer.Count(1) != nil || writer.Count(0) != nil || writer.Uint(uint64(keyspace.FamilyBind)) != nil || writer.Count(0) != nil || writer.Uint(uint64(keyspace.FamilyFunction)) != nil || writer.Count(0) != nil {
		t.Fatal("write order")
	}
	if err := writer.Record(sourceArtifactRecordKeys); err != nil || writer.Count(0) != nil || writer.Count(0) != nil || writer.Count(0) != nil || writer.Finish() != nil {
		t.Fatal("write keys")
	}
	return append([]byte(nil), data.Bytes()...)
}

func largeMalformedFaultArtifact(t *testing.T, nilCount, badFault int) []byte {
	t.Helper()
	const domain = "source/full-preflight-law"
	const faultCount = 2
	termCount := nilCount + 1 + 2 + faultCount
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !keyspace.TermOrdinalFits(termCount) {
		t.Fatalf("fixture term count %d exceeds ordinal", termCount)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(sourceArtifactRecordIdentity); err != nil || writer.String("large-fault.lua") != nil || writer.Count(uint64(termCount)) != nil {
		t.Fatal("write fault identity")
	}
	if err := writer.Record(sourceArtifactRecordSpans); err != nil {
		t.Fatal(err)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		if writer.Uint(uint64(family)) != nil {
			t.Fatal("write fault span family")
		}
		count := 0
		switch family {
		case keyspace.FamilyNil:
			count = nilCount
		case keyspace.FamilyBody:
			count = 1
		case keyspace.FamilyLabel:
			count = faultCount
		case keyspace.FamilyControlFault:
			count = faultCount
		}
		if writer.Count(uint64(count)) != nil {
			t.Fatal("write fault span count")
		}
		for index := 0; index < count; index++ {
			line := uint64(index + 1)
			if writer.Uint(line) != nil || writer.Uint(1) != nil || writer.Uint(line) != nil || writer.Uint(1) != nil {
				t.Fatal("write fault span")
			}
		}
	}
	if err := writer.Record(sourceArtifactRecordLiterals); err != nil {
		t.Fatal(err)
	}
	for _, family := range []keyspace.Family{keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString} {
		if writer.Uint(uint64(family)) != nil {
			t.Fatal("write fault literal family")
		}
		count := 0
		if family == keyspace.FamilyNil {
			count = nilCount
		}
		if writer.Count(uint64(count)) != nil {
			t.Fatal("write fault literal count")
		}
		for range count {
			if writer.Uint(uint64(body)) != nil {
				t.Fatal("write fault literal owner")
			}
		}
	}
	if err := writer.Record(sourceArtifactRecordOrder); err != nil || writer.Uint(uint64(keyspace.FamilyBody)) != nil || writer.Count(1) != nil || writer.Count(faultCount) != nil {
		t.Fatal("write fault Body order")
	}
	for ordinal := uint32(1); ordinal <= faultCount; ordinal++ {
		if writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyControlFault, ordinal))) != nil {
			t.Fatal("write fault Body term")
		}
	}
	if writer.Uint(uint64(keyspace.FamilyBind)) != nil || writer.Count(0) != nil || writer.Uint(uint64(keyspace.FamilyFunction)) != nil || writer.Count(0) != nil {
		t.Fatal("write fault empty order families")
	}
	if err := writer.Record(sourceArtifactRecordKeys); err != nil || writer.Count(0) != nil || writer.Count(0) != nil || writer.Count(faultCount) != nil {
		t.Fatal("write fault key prefix")
	}
	for index := 0; index < faultCount; index++ {
		kind := uint64(ControlFaultDuplicateLabel)
		if index == badFault {
			kind = 99
		}
		if writer.Uint(uint64(body)) != nil || writer.Uint(kind) != nil || writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyLabel, uint32(index+1)))) != nil || writer.Uint(0) != nil {
			t.Fatal("write fault row")
		}
	}
	if writer.Finish() != nil {
		t.Fatal("finish fault artifact")
	}
	return append([]byte(nil), data.Bytes()...)
}
