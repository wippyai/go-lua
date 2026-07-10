package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

const expectedDebugMapSchemaVersion1Hash = "64069d7a1c02b93619ad3dc4d6195e79a08fa133fcae48193403217f23c90375"
const expectedDebugMapSchemaVersion2Hash = "53ecf1350f0bcebc3e8aeea90ba26113c6a590d52310aceda1275e1ad27d1d6f"

func TestDebugMapSchemaVersionPinsCurrentSurface(t *testing.T) {
	surface := debugMapSchemaSurface()
	got := hashDebugMapSchemaSurface(surface)
	want := map[int]string{
		1: expectedDebugMapSchemaVersion1Hash,
		2: expectedDebugMapSchemaVersion2Hash,
	}[DebugMapSchemaVersion]
	if want == "" {
		t.Fatalf("no expected debug-map schema hash for version %d: bump version constant + journal a D-entry\nhash: %s\nsurface:\n%s", DebugMapSchemaVersion, got, strings.Join(surface, "\n"))
	}
	if got != want {
		t.Fatalf("debug-map schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s", DebugMapSchemaVersion, want, got, strings.Join(surface, "\n"))
	}
}

func debugMapSchemaSurface() []string {
	var out []string
	for _, record := range []reflect.Type{
		reflect.TypeOf(BodyDebugMap{}),
		reflect.TypeOf(StaticArtifactID{}),
		reflect.TypeOf(StaticArtifact{}),
		reflect.TypeOf(body.DebugMapEntry{}),
		reflect.TypeOf(body.DebugAnchor{}),
		reflect.TypeOf(body.DbgLocal{}),
		reflect.TypeOf(wir.DebugPointID{}),
	} {
		out = append(out, debugMapRecordShape(record)...)
	}
	for _, phase := range []wir.DebugPhase{
		wir.DebugPhaseBefore,
		wir.DebugPhaseAfter,
		wir.DebugPhaseCall,
		wir.DebugPhaseReturn,
		wir.DebugPhaseSuspend,
	} {
		out = append(out, fmt.Sprintf("phase:%d:%s", phase, phase))
	}
	out = append(out,
		"encoding:go-lua-debug-map",
		"encoding:uint64:big-endian",
		"encoding:string:uint64-byte-length+utf8",
		"encoding:entry:ordinal,phase,source-span,anchor,may-suspend,visible",
		"artifact-id:static-artifact-v1|unit|body|profile|engine|debug-map",
	)
	sort.Strings(out)
	return out
}

func debugMapRecordShape(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath == "" {
			out = append(out, fmt.Sprintf("record:%s|field:%s|type:%s", typ.Name(), field.Name, field.Type.String()))
		}
	}
	return out
}

func hashDebugMapSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		_, _ = h.Write([]byte(line))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
