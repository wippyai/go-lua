package arithmetic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

type inventory struct {
	fence       address.Fence
	certificate certificate.Certificate
	ids         IDs
	accesses    []arrangement.Access
}

func (value *inventory) Fence() address.Fence { return value.fence }

func (value *inventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	for index, relation := range value.certificate.Relations() {
		if relation.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	for index, column := range value.certificate.Columns() {
		if column.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) ResolveKey(id model.KeyID) (uint64, bool) {
	for index, key := range value.certificate.Keys() {
		if key.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	for index, scope := range value.certificate.Scopes() {
		if scope.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	for index, expression := range value.certificate.Expressions() {
		if expression.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	for index, dependency := range value.certificate.Dependencies() {
		if dependency.ID() == id {
			return uint64(index + 1), true
		}
	}
	return 0, false
}

func (value *inventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range value.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(value.fence, uint64(index+1))
		}
	}
	value.accesses = append(value.accesses, access)
	return arrangement.NewHandle(value.fence, uint64(len(value.accesses)))
}

func (value *inventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	var rows []model.RowID
	switch ref {
	case mustDenominatorValue(value.ids.Candidate, value.ids.CandidateKey):
		rows = []model.RowID{value.ids.CandidateA, value.ids.CandidateB}
	case mustDenominatorValue(value.ids.Source, value.ids.SourceKey):
		rows = []model.RowID{value.ids.SourceA, value.ids.SourceB}
	case mustDenominatorValue(value.ids.Output, value.ids.OutputKey):
		rows = []model.RowID{value.ids.OutputA, value.ids.OutputB}
	default:
		return witness.DenominatorEvidence{}, false
	}
	relationContent := ref.Relation().Content()
	evidence, ok := identity.DeriveContentID(declarationDomain+"/denominator", relationContent[:])
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(rows, evidence)
}
func (value *inventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

func mustDenominatorValue(relation model.RelationID, key model.KeyID) model.DenominatorRef {
	value, _ := model.NewDenominatorRef(relation, key)
	return value
}

func newMounted(t testing.TB, declaration Declaration, mountByte byte, bindings factory) (witness.Mounted, witness.Scope, *guard.Manager, support.Mask) {
	t.Helper()
	cert := mustCertificate(t, declaration.Schema)
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("arithmetic store")
	}
	fence, ok := address.NewFence(cert.SchemaID(), cert.Digest(), storeID, identity.MountID{mountByte}, identity.Generation(1))
	if !ok {
		t.Fatal("arithmetic fence")
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatalf("arithmetic guard manager: %v", err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("arithmetic support")
	}
	mask, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		t.Fatal("arithmetic scope mask")
	}
	book := &inventory{
		fence: fence, certificate: cert, ids: declaration.IDs,
	}
	lineageOwner := issue(t, "lineage-owner", model.IssueOwnerID)
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("arithmetic lineage factory")
	}
	registry := algebraRegistry{value: opaqueAlgebra{typeID: declaration.IDs.Type}}
	bookValue, bookOK := address.Bind(cert, book)
	if !bookOK || !bookValue.Available() {
		t.Fatal("arithmetic address book")
	}
	arranged, arrangedOK := arrangement.Derive(cert, bookValue, book, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !arrangedOK || !arranged.Available() || !arranged.ValidFor(bookValue) {
		t.Fatalf("arithmetic arrangement: ok=%v available=%v valid=%v", arrangedOK, arranged.Available(), arranged.ValidFor(bookValue))
	}
	mounted, ok := witness.Specialize(cert, book, bindings, registry, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("arithmetic mount")
	}
	scope, ok := mounted.Scope(declaration.IDs.Scope)
	if !ok {
		t.Fatal("arithmetic mounted scope")
	}
	return mounted, scope, manager, mask
}

func mustCertificate(t testing.TB, schema plan.ExecutionSchema) certificate.Certificate {
	t.Helper()
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("arithmetic certificate: %v", refusal)
	}
	return cert
}
