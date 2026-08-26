package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func carrierLawOwner(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func TestAuthorityIssueIsClosedAndOwnerQualified(t *testing.T) {
	raw := Authority{Carrier: "carrier/law/key", Capability: DecodeOnly}
	if !raw.Available() || raw.Issued() {
		t.Fatal("raw authority must be available but unissued")
	}

	owner := carrierLawOwner("carrier-law/owner")
	issued, ok := Issue(owner, raw)
	if !ok || !issued.Available() || !issued.Issued() {
		t.Fatalf("authority issue = %+v/%t, want one issued authority", issued, ok)
	}
	if _, ok := Issue(owner, issued); ok {
		t.Fatal("an issued authority crossed its owner boundary twice")
	}

	otherOwner, ok := Issue(carrierLawOwner("carrier-law/other"), raw)
	if !ok || otherOwner.ID() == issued.ID() {
		t.Fatal("the same carrier under two owners reused one authority identity")
	}
}

func TestAuthorityIssueRefusesUnavailableInputs(t *testing.T) {
	owner := carrierLawOwner("carrier-law/owner")
	valid := Authority{Carrier: "carrier/law/key", Capability: Equatable}
	cases := []struct {
		name  string
		owner schema.EntryReference
		raw   Authority
	}{
		{name: "owner", owner: schema.EntryReference{}, raw: valid},
		{name: "carrier", owner: owner, raw: Authority{Capability: Equatable}},
		{name: "capability", owner: owner, raw: Authority{Carrier: valid.Carrier}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if issued, ok := Issue(test.owner, test.raw); ok || issued.Available() || issued.Issued() {
				t.Fatalf("invalid issue = %+v/%t", issued, ok)
			}
		})
	}
}

func TestAuthorityCapabilityIsPartOfIdentity(t *testing.T) {
	owner := carrierLawOwner("carrier-law/capability")
	equatable, eqOK := Issue(owner, Authority{Carrier: "carrier/law/key", Capability: Equatable})
	ascending, ascOK := Issue(owner, Authority{Carrier: "carrier/law/key", Capability: Ascending})
	if !eqOK || !ascOK || equatable.ID() == ascending.ID() {
		t.Fatalf("capability mutation reused authority identity: %v/%t and %v/%t", equatable, eqOK, ascending, ascOK)
	}
}
