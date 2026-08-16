package artifact_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/internal/canonical"
)

func TestArtifactRoundTripRetainsTypedMetamethodCandidates(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "metamethod-candidates.lua", `
local function f(a, b, ...)
  local n = -a
  local l = #a
  local bn = ~a
  local ar = a + b
  local bw = a & b
  local c = a .. b
  local e = a ~= b
  local o = a >= b
  local r = a[b]
  a[b] = b
  return a:b(...)
end
`)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "metamethod-candidates"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, metadata, err := artifact.Decode(encoded, contract)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ContentID() != p.ContentID() {
		t.Fatalf("replayed ContentID = %v, want %v", replayed.ContentID(), p.ContentID())
	}
	for _, family := range []struct {
		name     string
		count    func(*program.Program) int
		at       func(*program.Program, int) (program.Term, bool)
		contains func(*program.Program, program.Term) bool
	}{
		{"UnaryNumeric", (*program.Program).UnaryNumericCount, (*program.Program).UnaryNumericAt, (*program.Program).UnaryNumeric},
		{"Length", (*program.Program).LengthCount, (*program.Program).LengthAt, (*program.Program).Length},
		{"Arithmetic", (*program.Program).ArithmeticCount, (*program.Program).ArithmeticAt, (*program.Program).Arithmetic},
		{"Bitwise", (*program.Program).BitwiseCount, (*program.Program).BitwiseAt, (*program.Program).Bitwise},
		{"Concat", (*program.Program).ConcatCount, (*program.Program).ConcatAt, (*program.Program).Concat},
		{"Equality", (*program.Program).EqualityCount, (*program.Program).EqualityAt, (*program.Program).Equality},
		{"Order", (*program.Program).OrderCount, (*program.Program).OrderAt, (*program.Program).Order},
		{"IndexGet", (*program.Program).IndexGetCount, (*program.Program).IndexGetAt, (*program.Program).IndexGet},
		{"IndexSet", (*program.Program).IndexSetCount, (*program.Program).IndexSetAt, (*program.Program).IndexSet},
		{"Callable", (*program.Program).CallableCount, (*program.Program).CallableAt, (*program.Program).Callable},
	} {
		if got, want := family.count(replayed), family.count(p); got != want {
			t.Fatalf("%sCount replayed = %d, want %d", family.name, got, want)
		}
		for index := 0; index < family.count(p); index++ {
			before, beforeOK := family.at(p, index)
			after, afterOK := family.at(replayed, index)
			if !beforeOK || !afterOK || before != after || !family.contains(replayed, after) {
				t.Fatalf("%s[%d] = %v/%v, want existing source Term %v/%v", family.name, index, after, afterOK, before, beforeOK)
			}
		}
	}
	reencoded, err := artifact.Encode(replayed, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("candidate artifact section changed canonical round-trip bytes")
	}
}

// The following identifiers are the fixed artifact candidate-section schema,
// not incidental byte offsets.  The test deliberately rewrites this section
// with canonical Reader/Writer frames and opens each result only through the
// public artifact API.
const artifactCandidateRecord uint64 = 10

const (
	artifactCandidateUnaryNumeric = iota
	artifactCandidateLength
	artifactCandidateArithmetic
	artifactCandidateBitwise
	artifactCandidateConcat
	artifactCandidateEquality
	artifactCandidateOrder
	artifactCandidateIndexGet
	artifactCandidateIndexSet
	artifactCandidateCallable
	artifactCandidateFamilyCount
)

type artifactCandidateWireTerm struct {
	tag   uint64
	index uint64
}

type artifactCandidateWireFamily struct {
	count   uint64
	sources []artifactCandidateWireTerm
}

type artifactCandidateWireSection struct {
	families [artifactCandidateFamilyCount]artifactCandidateWireFamily
}

func (section artifactCandidateWireSection) clone() artifactCandidateWireSection {
	copy := section
	for index := range section.families {
		copy.families[index].sources = append([]artifactCandidateWireTerm(nil), section.families[index].sources...)
	}
	return copy
}

// TestArtifactRejectsMalformedTypedMetamethodCandidatePayload mutates only
// the typed candidate section of an otherwise canonical artifact. Every case
// keeps canonical framing and the original authored Program rows intact, so
// public Decode necessarily reaches candidate payload validation or resealing.
func TestArtifactRejectsMalformedTypedMetamethodCandidatePayload(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "candidate-malformed.lua", `
local function f(a, b, ...)
  local neg = -a
  local bitNot = ~a
  local length = #a
  local add = a + b
  local sub = a - b
  local bitAnd = a & b
  local concat = a .. b
  local equal = a == b
  local notEqual = a ~= b
  local less = a < b
  local lessEqual = a <= b
  local indexed = a[b]
  a[b] = b
  a:b(b, ...)
  return a(b, ...)
end
return f
`)
	if p.UnaryNumericCount() != 2 || p.ArithmeticCount() != 2 || p.ConcatCount() != 1 {
		t.Fatalf("candidate fixture lost public source families: unary=%d arithmetic=%d concat=%d", p.UnaryNumericCount(), p.ArithmeticCount(), p.ConcatCount())
	}
	firstInvalidBinaryIndex := uint64(p.BinaryCount() + 1)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "candidate-malformed"})
	if err != nil {
		t.Fatal(err)
	}
	_, section, _ := artifactCandidateSection(t, encoded)
	if len(section.families[artifactCandidateUnaryNumeric].sources) < 2 ||
		len(section.families[artifactCandidateArithmetic].sources) < 2 ||
		len(section.families[artifactCandidateConcat].sources) != 1 {
		t.Fatal("candidate fixture does not expose the required schema rows")
	}

	for _, test := range []struct {
		name   string
		mutate func(*artifactCandidateWireSection)
	}{
		{
			name: "wrong family count",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateLength]
				family.sources = nil
				family.count = 0
			},
		},
		{
			name: "missing source term",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateArithmetic]
				family.sources = append([]artifactCandidateWireTerm(nil), family.sources[1:]...)
				family.count = uint64(len(family.sources))
			},
		},
		{
			name: "extra source term",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateArithmetic]
				family.sources = append(family.sources, section.families[artifactCandidateConcat].sources[0])
				family.count = uint64(len(family.sources))
			},
		},
		{
			name: "duplicate source term",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateArithmetic]
				family.sources = append(family.sources, family.sources[0])
				family.count = uint64(len(family.sources))
			},
		},
		{
			name: "out of order source term",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateArithmetic]
				family.sources[0], family.sources[1] = family.sources[1], family.sources[0]
			},
		},
		{
			name: "wrong source tag",
			mutate: func(section *artifactCandidateWireSection) {
				section.families[artifactCandidateArithmetic].sources[0] = section.families[artifactCandidateUnaryNumeric].sources[0]
			},
		},
		{
			name: "wrong family membership",
			mutate: func(section *artifactCandidateWireSection) {
				section.families[artifactCandidateArithmetic].sources[1] = section.families[artifactCandidateConcat].sources[0]
			},
		},
		{
			name: "corrupted candidate range index coverage",
			mutate: func(section *artifactCandidateWireSection) {
				family := &section.families[artifactCandidateArithmetic]
				term := family.sources[0]
				term.index = firstInvalidBinaryIndex
				family.sources[0] = term
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			malformed := artifactMutateCandidateSection(t, encoded, test.mutate)
			artifactMustRejectPublic(t, malformed, contract)
		})
	}
}

const (
	artifactCandidateTestDomain  = "artifact-candidate-test"
	artifactCandidateTestVersion = 1
)

func artifactCandidateSection(t *testing.T, data []byte) ([]byte, artifactCandidateWireSection, []byte) {
	t.Helper()
	start := artifactCandidateSectionStart(t, data)
	reader := artifactCandidateReader(t, data[start:])
	record, err := reader.Record()
	if err != nil || record != artifactCandidateRecord {
		t.Fatalf("candidate section record = %d/%v", record, err)
	}
	var section artifactCandidateWireSection
	for family := range section.families {
		count, err := reader.Count()
		if err != nil || count > uint64(reader.Remaining())/2 {
			t.Fatalf("candidate family %d count = %d/%v", family, count, err)
		}
		section.families[family].count = count
		section.families[family].sources = make([]artifactCandidateWireTerm, 0, int(count))
		for index := uint64(0); index < count; index++ {
			tag, tagErr := reader.Uint()
			termIndex, indexErr := reader.Uint()
			if tagErr != nil || indexErr != nil {
				t.Fatalf("candidate family %d source %d = %v/%v", family, index, tagErr, indexErr)
			}
			section.families[family].sources = append(section.families[family].sources, artifactCandidateWireTerm{tag: tag, index: termIndex})
		}
	}
	consumed := len(data[start:]) - reader.Remaining()
	if consumed <= 0 || start+consumed > len(data) {
		t.Fatal("candidate section has invalid canonical extent")
	}
	return append([]byte(nil), data[:start]...), section, append([]byte(nil), data[start+consumed:]...)
}

func artifactMutateCandidateSection(
	t *testing.T,
	data []byte,
	mutate func(*artifactCandidateWireSection),
) []byte {
	t.Helper()
	prefix, section, suffix := artifactCandidateSection(t, data)
	section = section.clone()
	mutate(&section)
	payload := artifactCandidateSectionBytes(t, section)
	result := make([]byte, 0, len(prefix)+len(payload)+len(suffix))
	result = append(result, prefix...)
	result = append(result, payload...)
	result = append(result, suffix...)
	if _, err := canonical.Scan(result, len(result)); err != nil {
		t.Fatalf("candidate mutation broke canonical framing: %v", err)
	}
	_ = artifactCandidateSectionStart(t, result)
	// The replacement remains a complete candidate schema section before the
	// public decoder sees it; rejection therefore cannot be a generic framing
	// failure before candidate payload.
	artifactCandidateSection(t, result)
	return result
}

func artifactCandidateSectionBytes(t *testing.T, section artifactCandidateWireSection) []byte {
	t.Helper()
	var stream bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&stream, artifactCandidateTestDomain, artifactCandidateTestVersion); err != nil {
		t.Fatal(err)
	}
	headerLen := stream.Len()
	if err := writer.Record(artifactCandidateRecord); err != nil {
		t.Fatal(err)
	}
	for _, family := range section.families {
		if err := writer.Count(family.count); err != nil {
			t.Fatal(err)
		}
		for _, source := range family.sources {
			if err := writer.Uint(source.tag); err != nil {
				t.Fatal(err)
			}
			if err := writer.Uint(source.index); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), stream.Bytes()[headerLen:]...)
}

func artifactCandidateSectionStart(t *testing.T, data []byte) int {
	t.Helper()
	if _, err := canonical.Scan(data, len(data)); err != nil {
		t.Fatalf("candidate source artifact is not canonical framing: %v", err)
	}
	for _, frame := range artifactCandidateFrames(t, data) {
		reader := artifactCandidateReader(t, data[frame.start:frame.end])
		kind, err := reader.Record()
		if err == nil && kind == artifactCandidateRecord {
			if err := reader.Finish(); err != nil {
				t.Fatal(err)
			}
			return frame.start
		}
	}
	t.Fatal("artifact has no candidate schema section")
	return 0
}

type artifactCandidateFrame struct{ start, end int }

func artifactCandidateFrames(t *testing.T, data []byte) []artifactCandidateFrame {
	t.Helper()
	frames := make([]artifactCandidateFrame, 0, 64)
	for offset := 0; offset < len(data); {
		start := offset
		offset++ // canonical.Scan above already validated the event tag.
		length, width := binary.Uvarint(data[offset:])
		if width <= 0 {
			t.Fatal("candidate frame length is not a canonical uvarint")
		}
		offset += width
		if length > uint64(len(data)-offset) {
			t.Fatal("candidate frame payload exceeds artifact")
		}
		offset += int(length)
		frames = append(frames, artifactCandidateFrame{start: start, end: offset})
	}
	return frames
}

func artifactCandidateReader(t *testing.T, payload []byte) *canonical.Reader {
	t.Helper()
	var stream bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&stream, artifactCandidateTestDomain, artifactCandidateTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	reader, err := canonical.NewReader(stream.Bytes(), stream.Len())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(artifactCandidateTestDomain, artifactCandidateTestVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}
