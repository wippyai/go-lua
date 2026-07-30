package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedEmbeddingSchemaVersion1Hash = "dbc44abb291b3d0c2158044cab55832e4b6285e523e217d4134d880f8be4c6cf"

func TestEmbeddingSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := embeddingSchemaSurface()
	got := hashSurface(surface)
	want := map[int]string{
		1: expectedEmbeddingSchemaVersion1Hash,
	}[EmbeddingSchemaVersion]
	if want == "" {
		t.Fatalf("no expected embedding schema hash for version %d: bump version constant + journal a D-entry", EmbeddingSchemaVersion)
	}
	if got != want {
		t.Fatalf("embedding schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s", EmbeddingSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func embeddingSchemaSurface() []string {
	var out []string
	for _, scheme := range []DocumentScheme{DocumentSchemeFile, DocumentSchemeRegistry, DocumentSchemeMem} {
		out = append(out, "document_scheme:"+string(scheme))
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(DocumentID{}),
		reflect.TypeOf(Digest{}),
		reflect.TypeOf(ByteSpan{}),
		reflect.TypeOf(SourceLocation{}),
		reflect.TypeOf(SourceSnapshot{}),
		reflect.TypeOf(UnitImport{}),
		reflect.TypeOf(UnitPlan{}),
		reflect.TypeOf(ResolutionSnapshot{}),
		reflect.TypeOf(SolveSeq(0)),
		reflect.TypeOf(BodyInputDigest(0)),
	} {
		out = append(out, recordShape(typ)...)
	}
	sort.Strings(out)
	return out
}

func recordShape(typ reflect.Type) []string {
	if typ.Kind() != reflect.Struct {
		return []string{fmt.Sprintf("scalar:%s", typ.String())}
	}
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

func hashSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
