package body

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedAllocationSiteFactSchemaVersion3Hash = "3af9fd4f070b681abd277ef255bcedcd6a1b5470f6d2a879f9fb10c84f035893"

func TestAllocationSiteFactSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := allocationSiteFactSchemaSurface()
	got := hashAllocationSiteFactSchemaSurface(surface)
	want := map[int]string{
		3: expectedAllocationSiteFactSchemaVersion3Hash,
	}[AllocationSiteFactSchemaVersion]
	if want == "" {
		t.Fatalf("no expected allocation site fact schema hash for version %d: bump version constant + journal a D-entry", AllocationSiteFactSchemaVersion)
	}
	if got != want {
		t.Fatalf("allocation site fact schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			AllocationSiteFactSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func allocationSiteFactSchemaSurface() []string {
	var out []string
	out = append(out, allocationSiteFactRecordShape(reflect.TypeOf(AllocationSiteFact{}))...)
	out = append(out, allocationSiteFactRecordShape(reflect.TypeOf(StableShapeField{}))...)
	out = append(out, allocationSiteFactRecordShape(reflect.TypeOf(SourceSpan{}))...)
	sort.Strings(out)
	return out
}

func allocationSiteFactRecordShape(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		out = append(out, fmt.Sprintf("record:%s|field:%s|type:%s", typ.Name(), field.Name, field.Type.String()))
	}
	sort.Strings(out)
	return out
}

func hashAllocationSiteFactSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
