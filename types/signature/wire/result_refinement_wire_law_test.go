package wire

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// The result-refinement payload is a published format: a contract member is
// written by one build and read by another, so the bytes are a commitment. These
// laws hold the format from both ends - the exact bytes each arm of the union
// serializes to, the value that comes back when those bytes are read again, and
// the boundary's refusal of everything the union does not admit.

// resultRefinementWireCase is one representative refinement and the exact bytes the
// format writes for it.
type resultRefinementWireCase struct {
	name       string
	kind       RefinementKind
	refinement ResultRefinement
	wire       string
}

// resultRefinementWireCorpus carries one case per admitted kind. A kind added to the
// union without a case is a verdict in the coverage law below, not an untested
// spelling.
func resultRefinementWireCorpus() []resultRefinementWireCase {
	return []resultRefinementWireCase{
		{
			name: "literalArgument",
			kind: RefinementLiteralArgument,
			refinement: LiteralArgumentRefinement{
				Result: 0, Argument: 0, Literal: "#", Type: typ.Integer,
			},
			wire: `{"schema":"go-lua.result.refinement/v1","kind":"refinement.literalArgument",` +
				`"literalArgument":{"result":0,"argument":0,"literal":"#","type":{"kind":"integer"}}}`,
		},
		{
			name: "subjectLength",
			kind: RefinementSubjectLength,
			refinement: SubjectLengthRefinement{
				Result: 0, Subject: 0, Position: 1, Default: 1,
			},
			wire: `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
				`"subjectLength":{"result":0,"subject":0,"position":1,"default":1}}`,
		},
	}
}

// TestResultRefinementWireBytesAreStable states the written commitment: each arm
// serializes to exactly the recorded bytes, kind spelling, arm placement and
// field omission included.
func TestResultRefinementWireBytesAreStable(t *testing.T) {
	for _, testCase := range resultRefinementWireCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.refinement.Kind() != testCase.kind {
				t.Fatalf("refinement declares kind %d, corpus records %d", testCase.refinement.Kind(), testCase.kind)
			}
			data, err := EncodeResultRefinement(testCase.refinement)
			if err != nil {
				t.Fatalf("EncodeResultRefinement: %v", err)
			}
			if string(data) != testCase.wire {
				t.Fatalf("wire is %s, want %s", data, testCase.wire)
			}
		})
	}
}

// TestResultRefinementRoundTripsThroughItsOwnBytes is the other half of the
// commitment: what the format writes it reads back as the same refinement.
func TestResultRefinementRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, testCase := range resultRefinementWireCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			decoded, err := DecodeResultRefinement([]byte(testCase.wire))
			if err != nil {
				t.Fatalf("DecodeResultRefinement: %v", err)
			}
			if decoded.Kind() != testCase.kind {
				t.Fatalf("decoded kind is %d, want %d", decoded.Kind(), testCase.kind)
			}
			if !ResultRefinementEquals(decoded, testCase.refinement) {
				t.Fatalf("decoded refinement is not the one written")
			}
			again, err := EncodeResultRefinement(decoded)
			if err != nil {
				t.Fatalf("EncodeResultRefinement(decoded): %v", err)
			}
			if string(again) != testCase.wire {
				t.Fatalf("re-encoded wire is %s, want %s", again, testCase.wire)
			}
		})
	}
}

// TestResultRefinementUnionIsClosed states that every admitted kind has a
// spelling and a corpus case, so a kind added to the union is unwritable until
// the format states how it is written.
func TestResultRefinementUnionIsClosed(t *testing.T) {
	covered := make(map[RefinementKind]bool, len(resultRefinementWireCorpus()))
	for _, testCase := range resultRefinementWireCorpus() {
		covered[testCase.kind] = true
	}
	for kind := RefinementKind(0); kind < 255; kind++ {
		if !kind.Available() {
			continue
		}
		if !covered[kind] {
			t.Fatalf("refinement kind %d is admitted and has no wire case", kind)
		}
	}
	if RefinementInvalid.Available() {
		t.Fatal("the invalid refinement kind is admitted")
	}
}

// TestResultRefinementRejectsWhatItCannotCarry is the boundary law. The payload
// is authored data that becomes trusted at the decode, so every shape the union
// does not admit is refused rather than read as one it is not.
func TestResultRefinementRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []struct {
		name string
		wire string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"notJSON", "refinement"},
		{"wrongSchema", `{"schema":"go-lua.result.refinement/v0","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1}}`},
		{"unknownKind", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.patternCapture",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1}}`},
		{"missingArm", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength"}`},
		{"foreignArm", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"literalArgument":{"result":0,"argument":0,"literal":"#","type":{"kind":"integer"}}}`},
		{"bothArms", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1},` +
			`"literalArgument":{"result":0,"argument":0,"literal":"#","type":{"kind":"integer"}}}`},
		{"unknownField", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1},"provenance":"none"}`},
		{"twoDocuments", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1}}` +
			`{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":1}}`},
		{"negativeResult", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":-1,"subject":0,"position":1,"default":1}}`},
		{"absentDefaultPosition", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.subjectLength",` +
			`"subjectLength":{"result":0,"subject":0,"position":1,"default":0}}`},
		{"untypedResult", `{"schema":"go-lua.result.refinement/v1","kind":"refinement.literalArgument",` +
			`"literalArgument":{"result":0,"argument":0,"literal":"#"}}`},
	}
	for _, testCase := range rejected {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := DecodeResultRefinement([]byte(testCase.wire)); err == nil {
				t.Fatal("the boundary admitted a payload the union does not carry")
			}
		})
	}
}

// TestResultRefinementRefusesToWriteWhatItCannotState holds the write side to
// the same closure: a refinement that names no result, no predicate or no
// refined type is not published as one that does.
func TestResultRefinementRefusesToWriteWhatItCannotState(t *testing.T) {
	unwritable := []struct {
		name       string
		refinement ResultRefinement
	}{
		{"none", nil},
		{"literalWithoutType", LiteralArgumentRefinement{Result: 0, Argument: 0, Literal: "#"}},
		{"literalNegativeResult", LiteralArgumentRefinement{Result: -1, Argument: 0, Literal: "#", Type: typ.Integer}},
		{"literalNegativeArgument", LiteralArgumentRefinement{Result: 0, Argument: -1, Literal: "#", Type: typ.Integer}},
		{"lengthNegativeResult", SubjectLengthRefinement{Result: -1, Subject: 0, Position: 1, Default: 1}},
		{"lengthNegativeSubject", SubjectLengthRefinement{Result: 0, Subject: -1, Position: 1, Default: 1}},
		{"lengthNegativePosition", SubjectLengthRefinement{Result: 0, Subject: 0, Position: -1, Default: 1}},
		{"lengthAbsentDefault", SubjectLengthRefinement{Result: 0, Subject: 0, Position: 1, Default: 0}},
	}
	for _, testCase := range unwritable {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.refinement != nil && testCase.refinement.Available() {
				t.Fatal("an unstatable refinement reports itself available")
			}
			if _, err := EncodeResultRefinement(testCase.refinement); err == nil {
				t.Fatal("the format wrote a refinement it cannot state")
			}
		})
	}
}
