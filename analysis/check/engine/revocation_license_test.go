package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

func TestRevokerFamilySubsetsStayDeclarationOwned(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	for _, meta := range loader.modulePackages("/analysis/check/") {
		if meta.ImportPath == modulePath+"/analysis/check/fixpoint/factkey" {
			continue
		}
		for _, construction := range fenceRevokerConstructions(loader.load(meta)) {
			t.Errorf("%s constructs a handwritten revoker-family subset; use the factkey declaration through familyReadLicense", construction)
		}
	}
}

func TestRevocationFenceRejectsHandwrittenFamilyIDSubset(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	tests := map[string]string{
		"rescan4 in-consumer set": `package engine
import "` + modulePath + `/analysis/check/fixpoint/factkey"
var revocationRegression = factkey.RevocationSet{factkey.FamilyHeapIndexRevoke}
func consume() { _ = revocationRegression }
`,
		"type alias literal": `package engine
import "` + modulePath + `/analysis/check/fixpoint/factkey"
type localRevokers = factkey.RevocationSet
var regression = localRevokers{factkey.FamilyHeapTableEscape}
`,
		"unnamed family ID slice": `package engine
import "` + modulePath + `/analysis/check/fixpoint/factkey"
var regression = []factkey.FamilyID{factkey.FamilyHeapIndexRevoke}
`,
		"make and append": `package engine
import "` + modulePath + `/analysis/check/fixpoint/factkey"
func regression() []factkey.FamilyID {
	out := make([]factkey.FamilyID, 0, 1)
	return append(out, factkey.FamilyHeapIndexRevoke)
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", source)
			if constructions := fenceRevokerConstructions(typed); len(constructions) == 0 {
				t.Fatal("type-based revocation fence accepted handwritten subset")
			}
		})
	}
}

func TestDeclaredRevokerInvalidatesLengthFloorFamilyRead(t *testing.T) {
	identity := []byte("sealed-table/license")
	subject := factkey.TaggedIdentityPart(identity)
	floor := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapLengthFloor, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("1"),
		}
	}
	revoker := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapIndexRevoke, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("revoked"),
		}
	}

	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"))); got != 1 {
		t.Fatalf("unrevoked length floor = %d, want 1", got)
	}
	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, revoker("op-00000001"), floor("op-00000002"))); got != 1 {
		t.Fatal("a revoker before the proof invalidated a later publication")
	}
	for _, point := range []string{"op-00000002", "op-00000003"} {
		if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"), revoker(point))); got != 0 {
			t.Fatalf("declared index revoker at %s left length floor %d, want 0", point, got)
		}
	}
}
