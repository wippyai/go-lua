package factorycatalog_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding/factorycatalog"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type countingFactory struct {
	operation signature.Signature
	calls     *int
	refuse    bool
	panicBind bool
}

func (factory *countingFactory) Bind(signature.Signature) (binding.Binding, bool) {
	if factory != nil && factory.calls != nil {
		(*factory.calls)++
	}
	if factory == nil || factory.panicBind {
		panic("hostile factory")
	}
	if factory.refuse {
		return nil, false
	}
	return testBinding{operation: factory.operation}, true
}

type testBinding struct{ operation signature.Signature }

func (value testBinding) Signature() signature.Signature { return value.operation }
func (value testBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

func catalogContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("semantic-binding-factory-catalog-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

type catalogFixture struct {
	owner      model.OwnerID
	foreign    model.OwnerID
	operation  model.OperationID
	other      model.OperationID
	foreignOp  model.OperationID
	schema     model.SchemaID
	signature  signature.Signature
	changed    signature.Signature
	versionTwo signature.Signature
}

func newCatalogFixture(t *testing.T) catalogFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(catalogContent(t, "owner"))
	if !ok {
		t.Fatal("issue owner")
	}
	foreign, ok := model.IssueOwnerID(catalogContent(t, "foreign-owner"))
	if !ok {
		t.Fatal("issue foreign owner")
	}
	operation, ok := model.IssueOperationID(owner, catalogContent(t, "operation"))
	if !ok {
		t.Fatal("issue operation")
	}
	other, ok := model.IssueOperationID(owner, catalogContent(t, "other-operation"))
	if !ok {
		t.Fatal("issue other operation")
	}
	foreignOp, ok := model.IssueOperationID(foreign, catalogContent(t, "operation"))
	if !ok {
		t.Fatal("issue foreign operation")
	}
	schema, ok := model.IssueSchemaID(owner, catalogContent(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	accepted, ok := outcome.Singleton(outcome.Produced)
	if !ok {
		t.Fatal("issue outcome set")
	}
	makeSignature := func(id model.OperationID, version uint64, outcomes outcome.Set) signature.Signature {
		value, sealed := signature.Seal(signature.Spec{
			Identity: signature.Identity{Operation: id, Version: version},
			Fence:    signature.Fence{Owner: owner, Schema: schema},
			Outcomes: outcomes,
		})
		if !sealed {
			t.Fatal("seal signature")
		}
		return value
	}
	base := makeSignature(operation, 1, accepted)
	changedOutcomes, ok := outcome.Singleton(outcome.NoCandidate)
	if !ok {
		t.Fatal("issue changed outcome set")
	}
	return catalogFixture{
		owner: owner, foreign: foreign, operation: operation, other: other, foreignOp: foreignOp,
		schema: schema, signature: base,
		changed:    makeSignature(operation, 1, changedOutcomes),
		versionTwo: makeSignature(operation, 2, accepted),
	}
}

func TestNewCatalogRejectsUnavailableDuplicateAndUnfaithfulEntries(t *testing.T) {
	fixture := newCatalogFixture(t)
	cases := []struct {
		name    string
		entries []factorycatalog.Entry
	}{
		{name: "unavailable signature", entries: []factorycatalog.Entry{{Factory: &countingFactory{}}}},
		{name: "unavailable identity", entries: []factorycatalog.Entry{{Signature: signatureFromZero(), Factory: &countingFactory{}}}},
		{name: "nil factory", entries: []factorycatalog.Entry{{Signature: fixture.signature}}},
		{name: "duplicate identity", entries: []factorycatalog.Entry{{Signature: fixture.signature, Factory: &countingFactory{operation: fixture.signature}}, {Signature: fixture.signature, Factory: &countingFactory{operation: fixture.signature}}}},
		{name: "digest drift from factory", entries: []factorycatalog.Entry{{Signature: fixture.signature, Factory: &countingFactory{operation: fixture.changed}}}},
		{name: "factory refusal", entries: []factorycatalog.Entry{{Signature: fixture.signature, Factory: &countingFactory{operation: fixture.signature, refuse: true}}}},
		{name: "factory panic", entries: []factorycatalog.Entry{{Signature: fixture.signature, Factory: &countingFactory{panicBind: true}}}},
	}
	for _, value := range cases {
		t.Run(value.name, func(t *testing.T) {
			catalog, ok := factorycatalog.NewCatalog(value.entries)
			if ok || catalog.Available() {
				t.Fatalf("hostile entry admitted: ok=%t available=%t", ok, catalog.Available())
			}
		})
	}
}

func signatureFromZero() signature.Signature { return signature.Signature{} }

func TestCatalogUsesOnlyExactIdentityThenDigestAndReAdmission(t *testing.T) {
	fixture := newCatalogFixture(t)
	firstCalls, secondCalls := 0, 0
	first := &countingFactory{operation: fixture.signature, calls: &firstCalls}
	secondSignature := newSignatureForOperation(t, fixture, fixture.other, 1)
	second := &countingFactory{operation: secondSignature, calls: &secondCalls}
	catalog, ok := factorycatalog.NewCatalog([]factorycatalog.Entry{
		{Signature: fixture.signature, Factory: first},
		{Signature: secondSignature, Factory: second},
	})
	if !ok || !catalog.Available() || catalog.Count() != 2 {
		t.Fatal("catalog was not sealed")
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("constructor validation calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
	bound, ok := catalog.Bind(fixture.signature)
	if !ok || bound == nil || bound.Signature().Digest() != fixture.signature.Digest() {
		t.Fatal("exact request was not redeemed")
	}
	if firstCalls != 2 || secondCalls != 1 {
		t.Fatalf("exact lookup called factories = %d/%d, want 2/1", firstCalls, secondCalls)
	}

	unknown := newSignatureForOperation(t, fixture, model.OperationID{}, 1)
	if _, ok := catalog.Bind(unknown); ok {
		t.Fatal("unknown operation was admitted")
	}
	if firstCalls != 2 || secondCalls != 1 {
		t.Fatal("unknown lookup called a factory")
	}
	if _, ok := catalog.Bind(fixture.versionTwo); ok {
		t.Fatal("version drift was admitted")
	}
	if _, ok := catalog.Bind(fixture.changed); ok {
		t.Fatal("digest drift was admitted")
	}
	if _, ok := catalog.Bind(newSignatureForOperation(t, fixture, fixture.foreignOp, 1)); ok {
		t.Fatal("foreign operation was admitted")
	}
	if firstCalls != 2 || secondCalls != 1 {
		t.Fatal("rejected requests called a factory")
	}
}

func newSignatureForOperation(t *testing.T, fixture catalogFixture, operation model.OperationID, version uint64) signature.Signature {
	t.Helper()
	accepted, ok := outcome.Singleton(outcome.Produced)
	if !ok {
		t.Fatal("issue outcome set")
	}
	value, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: version},
		Fence:    signature.Fence{Owner: fixture.owner, Schema: fixture.schema},
		Outcomes: accepted,
	})
	if !ok {
		t.Fatal("seal operation")
	}
	return value
}

func TestEmptyCatalogIsClosedAndNeverCallsAFactory(t *testing.T) {
	catalog := factorycatalog.EmptyCatalog()
	if !catalog.Available() || catalog.Count() != 0 {
		t.Fatal("empty catalog is not available")
	}
	if _, ok := catalog.Bind(signature.Signature{}); ok {
		t.Fatal("empty catalog admitted unavailable request")
	}
}
