package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedEmbeddingSchemaVersion4Hash = "86245f49bb352abbf80af6202ad2c87109161447a90fa7225a8b099553bc5203"
const expectedEmbeddingSchemaVersion5Hash = "16e4a866075ba21bcc484e63aea1dd4e2af3dfc51b85246063400dd28892d8ad"
const expectedEmbeddingSchemaVersion6Hash = "a1112020c817cd67811254b0e2ec97e4dfb597b233e3bca490475b5d56f3c6a0"
const expectedEmbeddingSchemaVersion7Hash = "480a16702f2de13beb92bcd3ddec8dbcc81de63f4d56dc318d0bef74e208b03d"

func TestEmbeddingSchemaVersionPinsSemanticQuerySurface(t *testing.T) {
	lines := embeddingSchemaSurface()
	got := hashEmbeddingSchema(lines)
	want := map[int]string{4: expectedEmbeddingSchemaVersion4Hash, 5: expectedEmbeddingSchemaVersion5Hash, 6: expectedEmbeddingSchemaVersion6Hash, 7: expectedEmbeddingSchemaVersion7Hash}[EmbeddingSchemaVersion]
	if want == "" {
		t.Fatalf("no expected embedding schema hash for version %d: bump EmbeddingSchemaVersion", EmbeddingSchemaVersion)
	}
	if got != want {
		t.Fatalf("embedding schema surface changed: bump EmbeddingSchemaVersion\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s", EmbeddingSchemaVersion, want, got, strings.Join(lines, "\n"))
	}
}

func embeddingSchemaSurface() []string {
	types := []reflect.Type{
		reflect.TypeOf(SourceSpan{}),
		reflect.TypeOf(SourceLocation{}),
		reflect.TypeOf(BinderOccurrence{}),
		reflect.TypeOf(BinderInfo{}),
		reflect.TypeOf(BinderOccurrencesRequest{}),
		reflect.TypeOf(BinderOccurrencesResponse{}),
		reflect.TypeOf(SemanticToken{}),
		reflect.TypeOf(SemanticTokensRequest{}),
		reflect.TypeOf(SemanticTokensResponse{}),
		reflect.TypeOf(SourcePosition{}),
		reflect.TypeOf(PositionLookupRequest{}),
		reflect.TypeOf(ExpressionType{}),
		reflect.TypeOf(EnclosingBody{}),
		reflect.TypeOf(PositionLookupResponse{}),
		reflect.TypeOf(JudgmentsByAnchorRequest{}),
		reflect.TypeOf(JudgmentsByAnchorResponse{}),
		reflect.TypeOf(JudgmentPresentation{}),
		reflect.TypeOf(DocumentSymbol{}),
		reflect.TypeOf(DocumentSymbolsRequest{}),
		reflect.TypeOf(DocumentSymbolsResponse{}),
		reflect.TypeOf(CalleeIdentity{}),
		reflect.TypeOf(CallRelation{}),
		reflect.TypeOf(BodyCallRelations{}),
		reflect.TypeOf(CallRelationsRequest{}),
		reflect.TypeOf(CallRelationsResponse{}),
		reflect.TypeOf(RepairAction{}),
		reflect.TypeOf(RepairEdit{}),
		reflect.TypeOf(RepairPayload{}),
		reflect.TypeOf(RepairActionsRequest{}),
		reflect.TypeOf(RepairActionsResponse{}),
	}
	var out []string
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath == "" {
				out = append(out, fmt.Sprintf("record:%s|field:%s|type:%s", typ.Name(), field.Name, field.Type.String()))
			}
		}
	}
	sort.Strings(out)
	return out
}

func hashEmbeddingSchema(lines []string) string {
	hash := sha256.New()
	for _, line := range lines {
		hash.Write([]byte(line))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
