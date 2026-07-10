package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const expectedManifestWireVersion1Hash = "ce85d4c63edb72848c7f8b1d270d8b2d719b22dc31017250366e61b01717e759"

func TestManifestWireVersionPinsCurrentSurface(t *testing.T) {
	surface := manifestWireSchemaSurface()
	got := hashManifestWireSurface(surface)
	want := map[int]string{
		1: expectedManifestWireVersion1Hash,
	}[WireFormatVersion]
	if want == "" {
		t.Fatalf("no expected manifest wire hash for version %d: bump WireFormatVersion + journal a D-entry\nhash: %s\nsurface:\n%s", WireFormatVersion, got, strings.Join(surface, "\n"))
	}
	if got != want {
		t.Fatalf("manifest wire surface changed: bump WireFormatVersion + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s", WireFormatVersion, want, got, strings.Join(surface, "\n"))
	}
}

func manifestWireSchemaSurface() []string {
	seen := make(map[reflect.Type]struct{})
	var surface []string
	var walk func(reflect.Type)
	walk = func(current reflect.Type) {
		for current.Kind() == reflect.Pointer || current.Kind() == reflect.Slice || current.Kind() == reflect.Array || current.Kind() == reflect.Map {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct || current.PkgPath() != reflect.TypeOf(manifestWire{}).PkgPath() {
			return
		}
		if _, ok := seen[current]; ok {
			return
		}
		seen[current] = struct{}{}
		for i := 0; i < current.NumField(); i++ {
			field := current.Field(i)
			surface = append(surface, fmt.Sprintf("record:%s|field:%s|type:%s|json:%s", current.Name(), field.Name, field.Type.String(), field.Tag.Get("json")))
			walk(field.Type)
		}
	}
	walk(reflect.TypeOf(manifestWire{}))
	sort.Strings(surface)
	return surface
}

func hashManifestWireSurface(surface []string) string {
	hash := sha256.New()
	for _, line := range surface {
		_, _ = hash.Write([]byte(line))
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
