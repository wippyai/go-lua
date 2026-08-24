package lifecycle

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

type artifactIdentityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type artifactIdentityOperations []artifactIdentityOperation

func (operations *artifactIdentityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, artifactIdentityOperation{kind: 'i', id: id})
	return true
}

func (operations *artifactIdentityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, artifactIdentityOperation{kind: 'u', value: value})
	return true
}

func (operations *artifactIdentityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, artifactIdentityOperation{kind: 'b', value: encoded})
	return true
}

func TestArtifactIdentityFieldsPreserveEmptySegmentOrder(t *testing.T) {
	view := lifecycleLawView(t, Publication{}, lifecycleLawID(t, "artifact-identity-empty"))
	var got artifactIdentityOperations
	if !view.WriteArtifactIdentityFields(&got) {
		t.Fatal("write lifecycle identity fields")
	}
	u := func(value uint64) artifactIdentityOperation {
		return artifactIdentityOperation{kind: 'u', value: value}
	}
	want := artifactIdentityOperations{
		u(StorageCellLifetimeLawVersion), u(0),
		u(SubjectYieldBoundaryLawVersion), u(0),
		u(SubjectLivenessSpanLawVersion), u(0),
		u(SubjectEventLawVersion), u(0),
		u(AliasRouteScopeLawVersion), u(0), u(0),
		u(AliasCandidateLawVersion), u(0),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identity operations = %#v, want %#v", got, want)
	}
}

func TestArtifactIdentityFieldsCommitStorageLifetime(t *testing.T) {
	cell := lifecycleLawID(t, "artifact-identity-cell")
	frame, frameOK := NewStorageCellLifetime(cell, StorageLifetimeFrame)
	module, moduleOK := NewStorageCellLifetime(cell, StorageLifetimeModule)
	closure, closureOK := NewStorageCellLifetime(cell, StorageLifetimeClosure)
	if !frameOK || !moduleOK || !closureOK {
		t.Fatal("storage lifetime")
	}
	frameView := lifecycleLawView(t, Publication{StorageCellLifetimes: []StorageCellLifetime{frame}}, lifecycleLawID(t, "artifact-identity-frame"))
	moduleView := lifecycleLawView(t, Publication{StorageCellLifetimes: []StorageCellLifetime{module}}, lifecycleLawID(t, "artifact-identity-module"))
	closureView := lifecycleLawView(t, Publication{StorageCellLifetimes: []StorageCellLifetime{closure}}, lifecycleLawID(t, "artifact-identity-closure"))
	var frameFields, moduleFields, closureFields artifactIdentityOperations
	if !frameView.WriteArtifactIdentityFields(&frameFields) || !moduleView.WriteArtifactIdentityFields(&moduleFields) || !closureView.WriteArtifactIdentityFields(&closureFields) {
		t.Fatal("write lifecycle identity fields")
	}
	if reflect.DeepEqual(frameFields, moduleFields) || reflect.DeepEqual(frameFields, closureFields) || reflect.DeepEqual(moduleFields, closureFields) {
		t.Fatal("storage lifetime change did not change lifecycle identity stream")
	}
}
