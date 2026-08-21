package signature

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/domain/placement"
)

const expectedEscapeVocabVersion1Hash = "2074fbf780df680d4b2fdf63c34dafd97570684152743d64379ede084cca43ed"
const expectedEscapeVocabVersion2Hash = "bd16be7718c6a78946c7f26a8c87253b0c5b031e84098dc81d262065735751f7"
const expectedEscapeVocabVersion3Hash = "2074fbf780df680d4b2fdf63c34dafd97570684152743d64379ede084cca43ed"

func TestEscapeVocabVersionPinsCurrentSurface(t *testing.T) {
	got := hashSchemaSurface(escapeVocabularySurface(t))
	want := map[int]string{
		1: expectedEscapeVocabVersion1Hash,
		2: expectedEscapeVocabVersion2Hash,
		3: expectedEscapeVocabVersion3Hash,
	}[EscapeVocabVersion]
	if want == "" {
		t.Fatalf("no expected escape vocabulary hash for version %d: bump version constant + journal a D-entry", EscapeVocabVersion)
	}
	if got != want {
		t.Fatalf("escape vocabulary surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			EscapeVocabVersion, want, got, strings.Join(escapeVocabularySurface(t), "\n"))
	}
}

func escapeVocabularySurface(t *testing.T) []string {
	t.Helper()
	var out []string
	for escape := placement.None; escape.ValidManifest(); escape++ {
		name := escape.Name()
		out = append(out, "escape-kind:"+strings.ToUpper(name[:1])+name[1:])
	}
	for _, descriptor := range capability.All() {
		if descriptor.Family != "ownership" {
			continue
		}
		out = append(out, fmt.Sprintf("ownership:%s|symbol:%s|status:%s", descriptor.ID, descriptor.Symbol, descriptor.Status))
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
