package summary

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

const expectedBoundaryLaneSchemaVersion1Hash = "295b4898ffcf5b94fd86ff73110bd7d46851c4e0e504341fd67cd99aa161d2d6"
const expectedBoundaryLaneSchemaVersion2Hash = "b79023173a0d826ee18ea187dd0313a32e44feef9084d624176c9dcf0c9a5d68"
const expectedBoundaryLaneSchemaVersion3Hash = "18a15fa6136cd972726bc3939b71eb59a2b96d6061c6b6d7ac590992fae27c45"
const expectedBoundaryLaneSchemaVersion4Hash = "cf858b4d4e46c662c9cd273dfc6fb38ba98cf5c9bf6711e46106e212ee96af6d"
const expectedBoundaryLaneSchemaVersion5Hash = "658ec606f3d883466aa8bf56b59244b49cae9be1bb5e0c9ea475b694d4226ef1"
const expectedBoundaryLaneSchemaVersion6Hash = "ce07898129fbc4ede1504819fd3f199780d47e543f7c13d4759f6dd7303d72ca"

func TestBoundaryLaneSchemaVersionPinsCurrentSurface(t *testing.T) {
	got := hashSchemaSurface(boundaryLaneSchemaSurface())
	want := map[int]string{
		1: expectedBoundaryLaneSchemaVersion1Hash,
		2: expectedBoundaryLaneSchemaVersion2Hash,
		3: expectedBoundaryLaneSchemaVersion3Hash,
		4: expectedBoundaryLaneSchemaVersion4Hash,
		5: expectedBoundaryLaneSchemaVersion5Hash,
		6: expectedBoundaryLaneSchemaVersion6Hash,
	}[BoundaryLaneSchemaVersion]
	if want == "" {
		t.Fatalf("no expected boundary lane schema hash for version %d: bump version constant + journal a D-entry", BoundaryLaneSchemaVersion)
	}
	if got != want {
		t.Fatalf("boundary lane schema surface changed: bump version constant + journal a D-entry\nversion: %d\nwant hash: %s\ngot hash:  %s\nsurface:\n%s",
			BoundaryLaneSchemaVersion, want, got, strings.Join(boundaryLaneSchemaSurface(), "\n"))
	}
}

func boundaryLaneSchemaSurface() []string {
	var out []string
	for _, d := range SummaryFactDescriptors() {
		out = append(out, fmt.Sprintf("summary:%s|slot:%t|wire:%s", d.Kind, d.Ops.Slot(), wireRef(d.WireRef)))
	}
	for _, d := range callboundary.NormalReturnFactDescriptors() {
		out = append(out, fmt.Sprintf("normal-return:%s|field:%s|filters_by_path:%t|wire:%s",
			d.Kind, d.Ops.FieldName(), d.Ops.FiltersByPath(), wireRef(d.WireRef)))
	}
	for _, d := range callpayload.CallOutcomeDescriptors() {
		out = append(out, fmt.Sprintf("call-outcome:%s|post_return:%t|wire:%s", d.Kind, d.Ops.PostReturn(), wireRef(d.WireRef)))
	}
	sort.Strings(out)
	return out
}

func wireRef(ref []string) string {
	if len(ref) == 0 {
		return "-"
	}
	copied := append([]string(nil), ref...)
	sort.Strings(copied)
	return strings.Join(copied, ",")
}

func hashSchemaSurface(lines []string) string {
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
