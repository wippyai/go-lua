package inventory

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestFactoryBindIsOneShotEvenWhenTheFirstCertificateIsRejected(t *testing.T) {
	population := testPopulationFor(t, "factory")
	factory, ok := NewFactory(identity.MountID{1}, []Population{population}, nil, nil)
	if !ok || factory == nil {
		t.Fatal("factory")
	}
	if _, ok := factory.Bind(certificate.Certificate{}); ok {
		t.Fatal("unavailable certificate was bound")
	}
	if _, ok := factory.Bind(certificate.Certificate{}); ok {
		t.Fatal("one-shot factory was reused after rejection")
	}
}

func TestResolveIssuesFreshMonotonicHandlesWithoutAnAccessCache(t *testing.T) {
	owner, _ := model.IssueOwnerID(testContent("arrangement-owner"))
	schema, _ := model.IssueSchemaID(owner, testContent("arrangement-schema"))
	relation, _ := model.IssueRelationID(owner, testContent("arrangement-relation"))
	key, _ := model.IssueKeyID(relation, testContent("arrangement-key"))
	access, ok := arrangement.NewKeyAccess(key)
	if !ok {
		t.Fatal("access")
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(schema, testContent("arrangement-certificate"), store, identity.MountID{2}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	value := &bound{fence: fence}
	first, ok := value.Resolve(access)
	if !ok || !first.ValidFor(fence) {
		t.Fatal("first handle")
	}
	second, ok := value.Resolve(access)
	if !ok || !second.ValidFor(fence) || second == first {
		t.Fatal("equal access was incorrectly cached")
	}
	if _, ok := value.Resolve(arrangement.Access{}); ok {
		t.Fatal("invalid access consumed a handle")
	}
	third, ok := value.Resolve(access)
	if !ok || third == second || third == first {
		t.Fatal("handles were not monotonic and exact-once")
	}
}

func TestResolveDenominatorRedeemsRowsAndEvidenceFromTheOwnerCapability(t *testing.T) {
	population := testPopulationFor(t, "denominator")
	value := &bound{populations: []Population{population}}
	evidence, ok := value.ResolveDenominator(population.denominator)
	if !ok || !evidence.Available() {
		t.Fatal("denominator evidence")
	}
	rows := evidence.Rows()
	if rows == nil || len(rows) != len(population.rows) || rows[0] != population.rows[0] {
		t.Fatal("owner rows were not redeemed")
	}
}

type testPopulation struct {
	available   bool
	denominator model.DenominatorRef
	rows        []model.RowID
	evidence    identity.ContentID
}

func (value testPopulation) Available() bool                   { return value.available }
func (value testPopulation) Denominator() model.DenominatorRef { return value.denominator }
func (value testPopulation) Rows() []model.RowID               { return value.rows }
func (value testPopulation) Evidence() identity.ContentID      { return value.evidence }

func testPopulationFor(t *testing.T, label string) testPopulation {
	t.Helper()
	owner, ok := model.IssueOwnerID(testContent(label + "-owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, testContent(label+"-relation"))
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, testContent(label+"-key"))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	row, ok := model.IssueRowID(relation, testContent(label+"-row"))
	if !ok {
		t.Fatal("row")
	}
	return testPopulation{available: true, denominator: denominator, rows: []model.RowID{row}, evidence: testContent(label + "-evidence")}
}

func testContent(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("analysis/relation/mount/inventory/law/v1", []byte(label))
	return value
}
