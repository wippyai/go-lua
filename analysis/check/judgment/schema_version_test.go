package judgment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedJIRSchemaVersion1Hash = "2b89a234f53cf065fed15e277611253e2f2aa0599dd10a0682dc4bd353f4d962"

func TestJIRSchemaVersionPinsCurrentSurface(t *testing.T) {
	got := hashSchemaSurface(jirSchemaSurface())
	want := map[int]string{
		1: expectedJIRSchemaVersion1Hash,
	}[JIRSchemaVersion]
	if want == "" {
		t.Fatalf("no expected JIR schema hash for version %d: bump version constant + journal a D-entry", JIRSchemaVersion)
	}
	if got != want {
		t.Fatalf("JIR schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			JIRSchemaVersion, want, got, strings.Join(jirSchemaSurface(), "\n"))
	}
}

func jirSchemaSurface() []string {
	var out []string
	for _, code := range DefaultRegistry().Codes() {
		spec, ok := DefaultRegistry().Lookup(code)
		if !ok {
			panic("judgment schema pin: registered code disappeared")
		}
		evidence := make([]string, len(spec.RequiredEvidence))
		for i, kind := range spec.RequiredEvidence {
			evidence[i] = evidenceKindName(kind)
		}
		out = append(out, fmt.Sprintf("code:%s|subject:%s|evidence:%s|verdict:%s",
			spec.Code, subjectKindString(spec.SubjectKind), strings.Join(evidence, ","), verdictName(spec.DefaultVerdict)))
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Judgment{}),
		reflect.TypeOf(SubjectRef{}),
		reflect.TypeOf(TypeRef{}),
		reflect.TypeOf(ValueRef{}),
		reflect.TypeOf(OriginRef{}),
		reflect.TypeOf(Evidence{}),
		reflect.TypeOf(EvidenceDetail{}),
		reflect.TypeOf(SpanRef{}),
	} {
		out = append(out, exportedRecordShape(typ)...)
	}
	sort.Strings(out)
	return out
}

func exportedRecordShape(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		out = append(out, fmt.Sprintf("record:%s|field:%s|type:%s", typ.Name(), field.Name, field.Type.String()))
	}
	return out
}

func hashSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func verdictName(v Verdict) string {
	switch v {
	case VerdictUnknown:
		return "unknown"
	case VerdictProven:
		return "proven"
	case VerdictRefuted:
		return "refuted"
	default:
		return fmt.Sprintf("verdict(%d)", v)
	}
}

func evidenceKindName(kind EvidenceKind) string {
	switch kind {
	case EvidenceUnknown:
		return "unknown"
	case EvidenceAbstractFact:
		return "abstract_fact"
	case EvidenceUserAssertion:
		return "user_assertion"
	case EvidenceMissingProof:
		return "missing_proof"
	case EvidencePrecisionBoundary:
		return "precision_boundary"
	default:
		return fmt.Sprintf("evidence(%d)", kind)
	}
}
