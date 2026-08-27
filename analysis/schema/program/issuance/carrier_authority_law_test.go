package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

func TestSubjectLivenessTypeOwnsOneDecodeAuthority(t *testing.T) {
	entries, ok := Entries()
	if !ok {
		t.Fatal("Program issuance declarations refused construction")
	}

	var typeEntry *schemaissuance.Entry
	var relationEntry *schemaissuance.Entry
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Key() == TypeSubjectLiveness {
			if typeEntry != nil {
				t.Fatal("subject-liveness type was declared more than once")
			}
			typeEntry = entry
		}
		if entry.Key() == RelationOccurrenceSubjectLiveness {
			relationEntry = entry
		}
	}
	if typeEntry == nil || typeEntry.Kind() != schemaissuance.KindType {
		t.Fatalf("subject-liveness type = %#v, want one KindType entry", typeEntry)
	}
	if relationEntry == nil || relationEntry.Kind() != schemaissuance.KindRelation {
		t.Fatalf("subject-liveness candidate relation = %#v, want KindRelation", relationEntry)
	}
	if relationEntry.AuthorityCount() != 0 {
		t.Fatalf("subject-liveness candidate relation owns %d authorities", relationEntry.AuthorityCount())
	}

	if typeEntry.AuthorityCount() != 1 {
		t.Fatalf("subject-liveness type authorities = %d, want one", typeEntry.AuthorityCount())
	}
	authority, authorityOK := typeEntry.CarrierAuthority(CarrierSubjectLiveness)
	if !authorityOK || !authority.Available() || !authority.Issued() {
		t.Fatalf("subject-liveness carrier authority unavailable: %+v/%t", authority, authorityOK)
	}
	owner := schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: TypeSubjectLiveness}
	want, wantOK := carrier.Issue(owner, carrier.Authority{Carrier: CarrierSubjectLiveness, Capability: carrier.DecodeOnly})
	if !wantOK {
		t.Fatal("subject-liveness authority could not be issued for its owner")
	}
	relationOwner := schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: RelationOccurrenceSubjectLiveness}
	other, otherOK := carrier.Issue(relationOwner, carrier.Authority{Carrier: CarrierSubjectLiveness, Capability: carrier.DecodeOnly})
	if !otherOK || want.ID() == other.ID() {
		t.Fatal("subject-liveness carrier authority reused the candidate relation as owner")
	}
	if authority.Carrier != CarrierSubjectLiveness || authority.Capability != carrier.DecodeOnly || authority.ID() != want.ID() {
		t.Fatalf("subject-liveness authority = %+v, want carrier=%q capability=DecodeOnly id=%v", authority, CarrierSubjectLiveness, want.ID())
	}
	if authorityAgain, ok := typeEntry.AuthorityAt(0); !ok || authorityAgain.ID() != authority.ID() {
		t.Fatal("subject-liveness authority storage is not deterministic")
	}
	if unknown, ok := typeEntry.CarrierAuthority("carrier/program/not-subject-liveness"); ok || unknown.Available() {
		t.Fatal("unknown Program carrier authority was admitted")
	}
}
