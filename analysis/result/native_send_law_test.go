package result

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/sendsafety"
)

func TestNativeSendSafetyIsOneDeclaredTypedColumn(t *testing.T) {
	point, pointOK := identity.DeriveContentID("analysis/result/test/send-point", []byte{1})
	if !pointOK {
		t.Fatal("point identity refused")
	}
	content := nativePublicationContent{sendSafety: sendsafety.VerdictIsolated, points: []identity.ContentID{point}}
	if !content.valid(nativePublicationFamilySendSafety) {
		t.Fatal("proved send verdict did not satisfy its native family law")
	}
	if ordinal, published := content.column(NativePublicationColumnSendSafety); !published || ordinal != uint16(sendsafety.VerdictIsolated) {
		t.Fatalf("send column = (%d, %v), want (%d, true)", ordinal, published, sendsafety.VerdictIsolated)
	}
	if category := NativePublicationColumnSendSafety.Category(); category != structure.CategoryNativeSendSafety {
		t.Fatalf("send category = %v, want %v", category, structure.CategoryNativeSendSafety)
	}
	if (nativePublicationContent{points: []identity.ContentID{point}}).valid(nativePublicationFamilySendSafety) {
		t.Fatal("missing verdict authenticated a send-safety row")
	}
	if content.valid(nativePublicationFamilyRepresentation) {
		t.Fatal("send verdict authenticated as a value representation")
	}
	declared := make(map[uint16]string)
	for _, spec := range structure.NativePublicationSpecs() {
		if spec.Category == structure.CategoryNativeSendSafety {
			declared[spec.Ordinal] = spec.Spelling
		}
	}
	for _, verdict := range sendsafety.Catalog() {
		if got := declared[verdict.Ordinal()]; got == "" {
			t.Fatalf("verdict ordinal %d has no declared spelling", verdict.Ordinal())
		}
	}
	if len(declared) != len(sendsafety.Catalog()) {
		t.Fatalf("declared verdict count = %d, want %d", len(declared), len(sendsafety.Catalog()))
	}
}

func TestNativeSendBodyJoinUsesProgramBodyNotInputPointMembership(t *testing.T) {
	mount, mountOK := identity.DeriveContentID("analysis/result/test/send-mount", []byte{1})
	body, bodyOK := identity.DeriveContentID("analysis/result/test/send-body", []byte{1})
	source, sourceOK := identity.DeriveContentID("analysis/result/test/send-source", []byte{1})
	point, pointOK := identity.DeriveContentID("analysis/result/test/send-input-point", []byte{1})
	if !mountOK || !bodyOK || !sourceOK || !pointOK {
		t.Fatal("send geometry identities refused")
	}
	geometry := Geometry{
		source:      source,
		bodies:      []GeometryBody{{key: artifactResultBody{mount: mount, body: body}}},
		values:      map[geometryValueKey]identity.ContentID{{mount: mount, value: point}: point},
		PointBodies: map[Point][]int{},
	}
	if indexes := geometry.PointBodies[Point{Mount: mount, Point: point}]; len(indexes) != 0 {
		t.Fatal("law fixture unexpectedly gave the stage input point a result-body membership")
	}
	if index, joined := nativeSendBodyIndex(geometry, mount, body); !joined || index != 0 {
		t.Fatalf("send body join = %d/%v, want 0/true without input-point membership", index, joined)
	}
}
