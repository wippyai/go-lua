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
const expectedJIRSchemaVersion2Hash = "45bdab7d703e0a2b56bbaf12a9eb606e179846d74a3eb6839a16c166766b5f6c"
const expectedJIRSchemaVersion3Hash = "43c55c71372da0f616d81685086ce8951f0c2cc00fa3e141917cba3e6d7384ff"
const expectedJIRSchemaVersion4Hash = "f50e5b8a2c383943ff3e876507791a078119ddbd55b8aa1149b60eb6e31c8f5a"
const expectedJIRSchemaVersion5Hash = "e4bf901a0cf23196860c191514aff2aad8b0aafb1387b696af0e9d009ecb3722"
const expectedJIRSchemaVersion6Hash = "ef8025767752746b3aacfdac58d2b772cdeb6968cff2d14a0e3695cc48d11ae8"

func TestJIRSchemaVersionPinsCurrentSurface(t *testing.T) {
	got := hashSchemaSurface(jirSchemaSurface())
	want := map[int]string{
		1: expectedJIRSchemaVersion1Hash,
		2: expectedJIRSchemaVersion2Hash,
		3: expectedJIRSchemaVersion3Hash,
		4: expectedJIRSchemaVersion4Hash,
		5: expectedJIRSchemaVersion5Hash,
		6: expectedJIRSchemaVersion6Hash,
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
		diagnosticCodes := make([]string, len(spec.DiagnosticCodes))
		for i, code := range spec.DiagnosticCodes {
			diagnosticCodes[i] = string(code)
		}
		out = append(out, fmt.Sprintf("code:%s|family:%s|subject:%s|evidence:%s|verdict:%s|policy:%s|diagnostics:%s|diagnostic_default:%s|render:%s",
			spec.Code,
			spec.Family,
			subjectKindString(spec.SubjectKind),
			strings.Join(evidence, ","),
			verdictName(spec.DefaultVerdict),
			spec.Policy,
			strings.Join(diagnosticCodes, ","),
			spec.DiagnosticDefault,
			spec.Render))
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Judgment{}),
		reflect.TypeOf(SubjectRef{}),
		reflect.TypeOf(SubjectAnchor{}),
		reflect.TypeOf(TypeRef{}),
		reflect.TypeOf(ValueRef{}),
		reflect.TypeOf(OriginRef{}),
		reflect.TypeOf(Evidence{}),
		reflect.TypeOf(EvidenceDetail{}),
		reflect.TypeOf(EvidenceCause{}),
		reflect.TypeOf(EvidenceCauseParams{}),
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
