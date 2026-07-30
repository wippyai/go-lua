package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestPublishedRelationClosureRoundTripsEveryVMChannel(t *testing.T) {
	production := PublishedRelationClosure{
		Values:               []equation.Fact{{Key: "value/point-1", Value: []byte("value")}},
		Outcomes:             []equation.Fact{{Key: "outcome/call-2", Value: []byte("outcome")}},
		DiagnosticCandidates: []equation.Fact{{Key: "diagnostic/3", Value: []byte("candidate")}},
		AllocationRekeys:     []equation.AllocationRekey{{From: "allocation/template", To: "allocation/runtime"}},
	}
	if err := production.RequireComplete(); err != nil {
		t.Fatal(err)
	}
	closure := production.ToOutputClosure()
	if len(closure.Values) != 1 || len(closure.Outcomes) != 1 || len(closure.Diagnostics) != 1 || len(closure.AllocationRekeys) != 1 {
		t.Fatalf("lossy VM closure: %#v", closure)
	}
	roundTrip := PublishedRelationClosureFromOutputClosure(closure)
	if !production.Equal(roundTrip) {
		t.Fatalf("production closure did not round trip: %#v != %#v", production, roundTrip)
	}
	closure.Values[0].Value[0] = 'X'
	if production.Values[0].Value[0] == 'X' {
		t.Fatal("closure conversion retained mutable production bytes")
	}
}

func TestPublishedRelationClosureRejectsAbsentChannel(t *testing.T) {
	if err := (PublishedRelationClosure{Values: []equation.Fact{}, Outcomes: []equation.Fact{}}).RequireComplete(); err == nil {
		t.Fatal("absent diagnostic/allocation channels accepted")
	}
}
