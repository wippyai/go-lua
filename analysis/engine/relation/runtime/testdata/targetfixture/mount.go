package targetfixture

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber/lower"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	schemaregion "github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// Build compiles and mounts one family declaration through the target runtime
// path. It keeps only the target mounted witness, geometry, and bootstrap
// root; it constructs no compatibility layer or parallel state store.
func Build(t Probe, spec Spec) World {
	t.Helper()
	if !spec.Identity.Owner().Available() || !spec.Declaration.SchemaID.Available() || spec.Authorities == nil {
		t.Fatal("target fixture incomplete spec")
	}
	declaration := addInitialDeclarations(t, spec.Declaration, spec.Initials)
	schema, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("target fixture compile: %v", err)
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("target fixture certificate: %v", refusal)
	}
	book, lowering := newInventory(t, spec, cert)
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("target fixture store")
	}
	mountByte := spec.MountByte
	if mountByte == 0 {
		mountByte = 0xF1
	}
	fence, ok := address.NewFence(cert.SchemaID(), cert.Digest(), storeID, identity.MountID{mountByte}, identity.Generation(1))
	if !ok {
		t.Fatal("target fixture address fence")
	}
	book.fence = fence
	runtimeFence, ok := binding.NewFence(fence.SchemaID(), fence.MountID(), fence.Generation())
	if !ok {
		t.Fatal("target fixture runtime fence")
	}
	issuer, ok := binding.NewIssuer(runtimeFence)
	if !ok {
		t.Fatal("target fixture runtime issuer")
	}
	authorities, ok := spec.Authorities(issuer)
	if !ok {
		t.Fatal("target fixture typed authorities")
	}
	registry, ok := newValueRegistry(authorities)
	if !ok {
		t.Fatal("target fixture duplicate typed authorities")
	}
	seedFactories, seeds := newSeeds(t, spec.Initials)
	factories := append(append([]binding.Factory(nil), spec.Bindings...), seedFactories...)
	binder := factoryRegistry{factories: factories}
	for index, operation := range cert.Signatures() {
		if _, ok := binding.Admit(binder, operation); !ok {
			t.Fatalf("target fixture binding %d", index)
		}
	}
	for index, typeID := range cert.AlgebraRequirements() {
		if _, ok := registry.Resolve(typeID); !ok {
			t.Fatalf("target fixture algebra %d", index)
		}
	}
	for index, typeID := range cert.EqualityRequirements() {
		if _, ok := registry.ResolveEquality(typeID); !ok {
			t.Fatalf("target fixture equality %d", index)
		}
	}
	if bookValue, ok := address.Bind(cert, book); !ok || !bookValue.Available() {
		t.Fatal("target fixture address book")
	}
	lineageContent, ok := spec.Identity.Content("lineage-owner")
	if !ok {
		t.Fatal("target fixture lineage owner content")
	}
	lineageOwner, ok := model.IssueOwnerID(lineageContent)
	if !ok {
		t.Fatal("target fixture lineage owner")
	}
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("target fixture lineage factory")
	}
	mounted, ok := witness.Specialize(cert, book, binder, registry, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("target fixture mount")
	}
	capability, ok := lower.NewFactory(mounted.RuntimeFence().Mount(), lowering)
	if !ok {
		t.Fatal("target fixture lowering factory")
	}
	view, ok := capability.Bind(mounted)
	if !ok || !view.Available() {
		t.Fatal("target fixture geometry")
	}
	base, ok := database.Bootstrap(mounted, view)
	if !ok || !base.Available() {
		t.Fatal("target fixture bootstrap")
	}
	base = publishSeeds(t, mounted, view, base, issuer, spec.Initials, seeds)
	return World{mounted: mounted, view: view, base: base}
}

func addInitialDeclarations(t Probe, declaration relcompile.Declaration, initials []Initial) relcompile.Declaration {
	t.Helper()
	if len(declaration.Initials) != 0 {
		t.Fatal("target fixture declaration must leave initials to the seed handoff")
	}
	declaration.Initials = make([]plan.Initial, 0, len(initials))
	seen := make(map[signature.Identity]struct{}, len(initials))
	for index, initial := range initials {
		if !initial.Operation.Available() || !initial.Scope.Available() || initial.Cells == nil {
			t.Fatalf("target fixture initial %d", index)
		}
		if initial.Operation.InputLen() != 0 {
			t.Fatalf("target fixture initial %d has inputs", index)
		}
		identity := initial.Operation.Identity()
		if _, duplicate := seen[identity]; duplicate {
			t.Fatalf("target fixture duplicate initial %d", index)
		}
		seen[identity] = struct{}{}
		declaration.Initials = append(declaration.Initials, plan.DefineInitial(identity, initial.Scope))
	}
	return declaration
}

type inventory struct {
	fence         address.Fence
	certificate   certificate.Certificate
	rows          map[model.DenominatorRef][]model.RowID
	partitions    witness.PartitionInventory
	accesses      []arrangement.Access
	resolveExpand func(model.ExpandContract) ([]expand.Vector, bool)
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
	rows, ok := value.rows[ref]
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	relationOwner := ref.Relation().Owner().Content()
	relation := ref.Relation().Content()
	key := ref.Key().Content()
	evidence, ok := identity.DeriveContentID("analysis/engine/relation/runtime/testdata/targetfixture/denominator/v1", relationOwner[:], relation[:], key[:])
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(rows, evidence)
}

// ResolvePartition forwards the exact checked partition to the owner-issued
// source supplied by Spec.  The fixture never infers child postings from a
// denominator row vector or falls back to the global child witness; absent
// source evidence remains unavailable and causes correlated mount refusal.
func (value *inventory) ResolvePartition(partition certificate.CorrelationPartition) (map[model.RowID]witness.DenominatorEvidence, bool) {
	if value == nil || value.partitions == nil {
		return nil, false
	}
	return value.partitions.ResolvePartition(partition)
}

func (value *inventory) ResolveExpand(contract model.ExpandContract) ([]expand.Vector, bool) {
	if value == nil || value.resolveExpand == nil {
		return nil, false
	}
	return value.resolveExpand(contract)
}

func newInventory(t Probe, spec Spec, cert certificate.Certificate) (*inventory, lower.Lowering) {
	t.Helper()
	rows := make(map[model.DenominatorRef][]model.RowID, len(spec.Populations))
	for index, population := range spec.Populations {
		if !population.Denominator.Available() || population.Rows == nil {
			t.Fatalf("target fixture population %d", index)
		}
		if _, duplicate := rows[population.Denominator]; duplicate {
			t.Fatalf("target fixture duplicate population %d", index)
		}
		copyOf := append([]model.RowID(nil), population.Rows...)
		rows[population.Denominator] = copyOf
	}
	lowering := newRegions(t, cert, spec.Scopes)
	return &inventory{certificate: cert, rows: rows, partitions: spec.PartitionInventory, resolveExpand: spec.ResolveExpand}, lowering
}

func newRegions(t Probe, cert certificate.Certificate, scopes []Scope) lower.Lowering {
	t.Helper()
	if !cert.Available() || len(scopes) == 0 {
		t.Fatal("target fixture has no scopes")
	}
	scopeSchemas := cert.Scopes()
	if len(scopeSchemas) == 0 || len(scopeSchemas) != len(scopes) {
		t.Fatal("target fixture scope handoff")
	}
	schemaByID := make(map[model.ScopeID]schemaregion.Region, len(scopeSchemas))
	for index, scopeSchema := range scopeSchemas {
		if !scopeSchema.Available() || !scopeSchema.ID().Available() || !scopeSchema.Region().Available() {
			t.Fatalf("target fixture certificate scope %d", index)
		}
		schemaByID[scopeSchema.ID()] = scopeSchema.Region()
	}
	seenScopes := make(map[model.ScopeID]struct{}, len(scopes))
	for index, scope := range scopes {
		if !scope.ID.Available() || scope.Region == "" {
			t.Fatalf("target fixture scope %d", index)
		}
		if _, duplicate := seenScopes[scope.ID]; duplicate {
			t.Fatalf("target fixture duplicate scope %d", index)
		}
		seenScopes[scope.ID] = struct{}{}
		if _, ok := schemaByID[scope.ID]; !ok {
			t.Fatalf("target fixture undeclared scope %d", index)
		}
	}
	for scopeID := range schemaByID {
		if _, ok := seenScopes[scopeID]; !ok {
			t.Fatal("target fixture missing scope handoff")
		}
	}

	atomIDs := make([]identity.ContentID, 0)
	seenAtoms := make(map[identity.ContentID]struct{})
	for _, formula := range schemaByID {
		for _, node := range formula.Nodes() {
			id := node.Atom.ID()
			if !id.Available() {
				t.Fatal("target fixture unavailable scope atom")
			}
			if _, seen := seenAtoms[id]; seen {
				continue
			}
			seenAtoms[id] = struct{}{}
			atomIDs = append(atomIDs, id)
		}
	}
	sort.Slice(atomIDs, func(left, right int) bool {
		return bytes.Compare(atomIDs[left][:], atomIDs[right][:]) < 0
	})
	atoms := make([]guard.Atom, len(atomIDs))
	if len(atomIDs) == 0 {
		// A schema carrying only True has no logical atom from which to
		// derive a physical guard universe. Keep the historical one-atom
		// mounted scope so state still has a non-empty executable fiber.
		atoms = append(atoms, guard.Atom(1))
	}
	for index := range atomIDs {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatalf("target fixture guards: %v", err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("target fixture support work")
	}
	extents := make(map[identity.ContentID]support.Mask, len(atomIDs))
	for index, id := range atomIDs {
		mask, ok := work.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatalf("target fixture atom extent %d", index)
		}
		extents[id] = mask
	}
	if !work.Seal() {
		t.Fatal("target fixture support seal")
	}
	lowering, ok := lower.New(manager, extents)
	if !ok {
		t.Fatal("target fixture lowering")
	}
	return lowering
}
