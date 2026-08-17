package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestExactScalarSummaryCommitsLiteralAndRole(t *testing.T) {
	occurrence, subject, body := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}
	literal := keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7}
	row := ExactScalarSummaryRow{occurrence: occurrence, subject: subject, body: body, role: ExactScalarSummaryLeft, literal: literal}
	row.id = exactScalarSummaryID(occurrence, subject, body, row.role, literal)
	got, ok := row.Literal()
	if !row.Available() || !ok || got != literal {
		t.Fatal("valid exact scalar summary unavailable")
	}
	row.role = ExactScalarSummaryRight
	if row.Available() {
		t.Fatal("summary admitted a role mutation")
	}
}
