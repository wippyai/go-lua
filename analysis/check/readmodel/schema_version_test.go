package readmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedClosureCaptureSchemaVersion2Hash = "532d4d413d3030c9c805d5477e63a3e7ebb09abe1b9e4ee185ac6aa2d201bfe4"
const expectedClosureCaptureSchemaVersion3Hash = expectedClosureCaptureSchemaVersion2Hash

func TestClosureCaptureSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := closureCaptureSchemaSurface()
	got := hashSchemaSurface(surface)
	want := map[int]string{
		2: expectedClosureCaptureSchemaVersion2Hash,
		3: expectedClosureCaptureSchemaVersion3Hash,
	}[ClosureCaptureSchemaVersion]
	if want == "" {
		t.Fatalf("no expected closure capture schema hash for version %d: bump version constant + journal a D-entry", ClosureCaptureSchemaVersion)
	}
	if got != want {
		t.Fatalf("closure capture schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			ClosureCaptureSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func closureCaptureSchemaSurface() []string {
	return recordShape(reflect.TypeOf(ClosureCapture{}))
}

const expectedHoistableLoadSchemaVersion1Hash = "de555f5570cac9bfb6226da3431450773d716972c57f6703d7a0c621f461155c"
const expectedHoistableLoadSchemaVersion2Hash = "29b2060219b97ab0d3a349f92698bcd7686f5cb5315228717321d2eda7cc7853"

func TestHoistableLoadSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := hoistableLoadSchemaSurface()
	got := hashSchemaSurface(surface)
	want := map[int]string{
		1: expectedHoistableLoadSchemaVersion1Hash,
		2: expectedHoistableLoadSchemaVersion2Hash,
	}[HoistableLoadSchemaVersion]
	if want == "" {
		t.Fatalf("no expected hoistable load schema hash for version %d: bump version constant + journal a D-entry", HoistableLoadSchemaVersion)
	}
	if got != want {
		t.Fatalf("hoistable load schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			HoistableLoadSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func hoistableLoadSchemaSurface() []string {
	return recordShape(reflect.TypeOf(HoistableLoad{}))
}

const expectedAllocationSiteSchemaVersion2Hash = "19a6e20ad929c0a05f4da0d890da7b1f4ab1585f7c5f75f256118e9c160c0bbf"
const expectedAllocationSiteSchemaVersion3Hash = expectedAllocationSiteSchemaVersion2Hash

func TestAllocationSiteSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := allocationSiteSchemaSurface()
	got := hashSchemaSurface(surface)
	want := map[int]string{
		2: expectedAllocationSiteSchemaVersion2Hash,
		3: expectedAllocationSiteSchemaVersion3Hash,
	}[AllocationSiteSchemaVersion]
	if want == "" {
		t.Fatalf("no expected allocation site schema hash for version %d: bump version constant + journal a D-entry", AllocationSiteSchemaVersion)
	}
	if got != want {
		t.Fatalf("allocation site schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			AllocationSiteSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func allocationSiteSchemaSurface() []string {
	var out []string
	out = append(out, recordShape(reflect.TypeOf(AllocationSite{}))...)
	out = append(out, recordShape(reflect.TypeOf(AllocationField{}))...)
	sort.Strings(out)
	return out
}

const expectedSendSafetySchemaVersion1Hash = "1df75fbd66ac98165c35bb81f6dff6369e73fcafd0470843aa3787e1e57335bd"
const expectedSendSafetySchemaVersion2Hash = expectedSendSafetySchemaVersion1Hash

func TestSendSafetySchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := recordShape(reflect.TypeOf(SendSafety{}))
	got := hashSchemaSurface(surface)
	want := map[int]string{
		1: expectedSendSafetySchemaVersion1Hash,
		2: expectedSendSafetySchemaVersion2Hash,
	}[SendSafetySchemaVersion]
	if want == "" {
		t.Fatalf("no expected send-safety schema hash for version %d: bump version constant + journal a D-entry", SendSafetySchemaVersion)
	}
	if got != want {
		t.Fatalf("send-safety schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			SendSafetySchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func TestSendSafetyUnknownSchemaFailsClosed(t *testing.T) {
	if (SendSafety{}).SchemaValid() || !(SendSafety{SchemaVersion: SendSafetySchemaVersion}).SchemaValid() {
		t.Fatal("send-safety schema validation did not fail closed")
	}
}

func recordShape(typ reflect.Type) []string {
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

func hashSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
