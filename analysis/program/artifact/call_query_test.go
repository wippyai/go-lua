package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestCallQueryTermAdmissionKeepsFamiliesDistinct(t *testing.T) {
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !validArtifactTerm(call) || !validArtifactCallTerm(call) || validArtifactCallTerm(body) {
		t.Fatal("call query term admission crossed family boundaries")
	}
	if validArtifactTerm(0) || validArtifactCallTerm(0) {
		t.Fatal("zero query term was admitted")
	}
}
