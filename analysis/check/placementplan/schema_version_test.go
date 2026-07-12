package placementplan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedSchemaVersion1Hash = "c1ed8e6857e508d568ae958088bda02c81d91253ab6fafce8b5a11f2d9ab804c"

func TestSchemaVersionPinsCompilerSurface(t *testing.T) {
	var surface []string
	surface = append(surface, placementRecordShape(reflect.TypeOf(Plan{}))...)
	surface = append(surface, placementRecordShape(reflect.TypeOf(Entry{}))...)
	sort.Strings(surface)
	got := placementSchemaHash(surface)
	want := map[int]string{1: expectedSchemaVersion1Hash}[SchemaVersion]
	if want == "" || got != want {
		t.Fatalf("placement-plan schema changed: bump SchemaVersion + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			SchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func placementRecordShape(typ reflect.Type) []string {
	var out []string
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath == "" {
			out = append(out, fmt.Sprintf("record:%s|field:%s|type:%s", typ.Name(), field.Name, field.Type.String()))
		}
	}
	sort.Strings(out)
	return out
}

func placementSchemaHash(surface []string) string {
	h := sha256.New()
	for _, line := range surface {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
