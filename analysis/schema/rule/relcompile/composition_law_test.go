package relcompile_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/inputscope"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
	"github.com/wippyai/go-lua/domain/composite"
)

// compositionScopeOwner is the one surface entry every decision scope in
// these laws is issued under. A relation input bundle is sealed for one
// issuing owner, so a composition whose scopes are issued by several entries
// states no bundle at all; naming them under one entry is what makes the
// bundle's owner fence meaningful rather than a formality.
var compositionScopeOwner = schema.EntryReference{Surface: schema.SurfaceKindStructure, Key: "composition/scopes"}

// compositionScope installs one decision scope under the issuing owner these
// laws seal against, and answers the authored name of it.
func compositionScope(surfaces *owners, member schema.Key) relcompile.Name {
	return surfaces.scope(relcompile.NewName(compositionScopeOwner, member))
}

// sealedLowering is one production rule catalog lowered through one
// composition: the compilation the ordinals belong to, the placements the
// composition resolved, and the rules it lowered beside them.
//
// Everything the laws below compare against is read from this value rather
// than restated: the expectation for what a bundle answers is what Resolve
// computed, so a law that passed against a rewritten expectation is not
// available.
type sealedLowering struct {
	compilation composite.Compilation
	plans       ruleplan.Catalog
	surfaces    *owners
	composition *relcompile.Composition
	owner       model.OwnerID
	// resolved is what the lowering pass itself answered per dense rule
	// ordinal, read from the resolution rather than back out of the table the
	// composition publishes: an expectation read through the surface under
	// test states nothing about it.
	resolved map[int]relinput.Placement
	// authored is the placement handed to the lowering, by name. It is the
	// independent statement of which scope each port was placed at, so a
	// published port column can be compared against something other than
	// itself.
	authored map[int]relcompile.Placement
	// specs is the authored declaration lowered at each ordinal.
	specs map[int]rule.Spec
	// rules is the relational lowering of each ordinal.
	rules map[int][]relcompile.Rule
}

// lowerSealedCatalog lowers every rule of the sealed production catalog
// through one composition, at the dense ordinal that catalog numbers it with.
//
// The declaration surfaces that would install a rule's entries into the
// identity registry do not exist in production, so the rule census's own
// install pass stands in for them here exactly as it does in the census. What
// the composition publishes is not stood in for: every scope it states is one
// the registry resolved while Resolve lowered the rule.
func lowerSealedCatalog(t *testing.T) sealedLowering {
	t.Helper()
	compilation, built := composite.Build()
	if !built || !compilation.Available() {
		t.Fatal("the production declaration table did not compile")
	}
	plans, held := compilation.RulePlans()
	if !held || !plans.Available() {
		t.Fatal("the sealed compilation carries no rule plans")
	}
	authored := make(map[schema.Key]rule.Spec, len(declared()))
	for _, row := range declared() {
		authored[row.Spec.Key] = row.Spec
	}

	surfaces := newOwners(t)
	composition := relcompile.Compose(surfaces.registry)
	if !composition.Available() {
		t.Fatal("the composition holds no identity registry")
	}
	lowering := sealedLowering{
		compilation: compilation, plans: plans, surfaces: surfaces, composition: composition,
		resolved: map[int]relinput.Placement{}, authored: map[int]relcompile.Placement{},
		specs: map[int]rule.Spec{}, rules: map[int][]relcompile.Rule{},
	}
	for ordinal := 0; ordinal < plans.Count(); ordinal++ {
		plan, planHeld := plans.At(ordinal)
		if !planHeld {
			t.Fatalf("ordinal %d is not held by the catalog that numbered it", ordinal)
		}
		if !plan.Present() {
			continue
		}
		key, keyHeld := composite.RuleKeyAt(compilation, ordinal)
		if !keyHeld {
			t.Fatalf("ordinal %d names no rule", ordinal)
		}
		spec, declaredHere := authored[key]
		if !declaredHere {
			t.Fatalf("rule %s carries a program at ordinal %d and the census declares no row for it", key, ordinal)
		}
		surfaces.install(spec)
		placement := relcompile.Placement{Candidate: compositionScope(surfaces, schema.Key(string(key)+"#candidate"))}
		for port := 0; port < plan.InputCount(); port++ {
			placement.Ports = append(placement.Ports,
				compositionScope(surfaces, schema.Key(fmt.Sprintf("%s#port/%d", key, port))))
		}
		resolution, err := composition.Resolve(ordinal, spec, placement)
		if err != nil {
			t.Fatalf("lower %s at ordinal %d: %v", key, ordinal, err)
		}
		if !resolution.Available() {
			t.Fatalf("lowering %s at ordinal %d resolved no candidate scope", key, ordinal)
		}
		if _, publishedHere := composition.Placement(ordinal); !publishedHere {
			t.Fatalf("the composition lowered %s at ordinal %d and published no placement for it", key, ordinal)
		}
		lowering.resolved[ordinal] = resolution.Placed
		lowering.authored[ordinal] = placement
		lowering.specs[ordinal] = spec
		lowering.rules[ordinal] = resolution.Rules
	}
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "composition.owner"}, compositionScopeOwner)
	if err != nil {
		t.Fatalf("resolve the scope-issuing owner: %v", err)
	}
	lowering.owner = owner
	return lowering
}

// TestEveryRuleOfTheSealedCatalogIsCensused states the corpus the composition
// is total over. A rule ordinal that carries an authored program and no
// census row is a family the matrix does not measure, and lowering it would
// be lowering a declaration nothing reports on.
func TestEveryRuleOfTheSealedCatalogIsCensused(t *testing.T) {
	compilation, built := composite.Build()
	if !built {
		t.Fatal("the production declaration table did not compile")
	}
	plans, held := compilation.RulePlans()
	if !held {
		t.Fatal("the sealed compilation carries no rule plans")
	}
	censused := map[schema.Key]bool{}
	for _, row := range declared() {
		censused[row.Spec.Key] = true
	}
	programs := 0
	for ordinal := 0; ordinal < plans.Count(); ordinal++ {
		plan, _ := plans.At(ordinal)
		if !plan.Present() {
			continue
		}
		programs++
		key, keyHeld := composite.RuleKeyAt(compilation, ordinal)
		if !keyHeld || !censused[key] {
			t.Fatalf("rule %s carries a program at ordinal %d and the census declares no row for it", key, ordinal)
		}
	}
	if programs == 0 {
		t.Fatal("the sealed catalog carries no authored rule program")
	}
}

// TestEveryRuleTheCompositionLoweredIsPlaced states the totality of the
// published table. The bundle covers every ordinal the catalog numbers, an
// ordinal that declared no program is an explicitly absent row, and every
// ordinal that carries one states the placement the composition resolved.
func TestEveryRuleTheCompositionLoweredIsPlaced(t *testing.T) {
	lowering := lowerSealedCatalog(t)
	bundle, refusal := lowering.compilation.InputBundle(lowering.owner, lowering.composition)
	if refusal != nil {
		t.Fatalf("the sealed catalog refused its own composition: %v", refusal)
	}
	if bundle.Count() != lowering.plans.Count() {
		t.Fatalf("bundle rows = %d, want one per rule ordinal = %d", bundle.Count(), lowering.plans.Count())
	}
	placed := 0
	for ordinal := 0; ordinal < lowering.plans.Count(); ordinal++ {
		plan, _ := lowering.plans.At(ordinal)
		row, held := bundle.At(ordinal)
		if !held || !row.Available() {
			t.Fatalf("ordinal %d states no row", ordinal)
		}
		if row.Present() != plan.Present() {
			t.Fatalf("ordinal %d row present = %t, want the plan's own %t", ordinal, row.Present(), plan.Present())
		}
		if !plan.Present() {
			continue
		}
		placed++
		if row.PortCount() != plan.InputCount() {
			t.Fatalf("ordinal %d port width = %d, want the declared input count %d", ordinal, row.PortCount(), plan.InputCount())
		}
		candidate, stated := bundle.CandidateScope(ordinal)
		if !stated || candidate != lowering.resolved[ordinal].Candidate {
			t.Fatalf("ordinal %d candidate scope = %v/%t, want the scope the lowering resolved", ordinal, candidate, stated)
		}
	}
	if placed != len(lowering.resolved) || placed != lowering.composition.Count() {
		t.Fatalf("placed ordinals = %d, want the %d the composition lowered and the %d it counts",
			placed, len(lowering.resolved), lowering.composition.Count())
	}
}

// TestARuleWhoseScopeNoOwnerInstalledIsRefusedAndNeverSkipped states that an
// unresolvable scope is a refusal at both boundaries it can reach. The
// lowering refuses the rule rather than placing it at a default, and the seal
// then refuses the whole table with that ordinal named rather than publishing
// a catalog with a hole in it.
func TestARuleWhoseScopeNoOwnerInstalledIsRefusedAndNeverSkipped(t *testing.T) {
	lowering := lowerSealedCatalog(t)
	ordinal, spec := oneLoweredOrdinal(t, lowering)

	second := newOwners(t)
	composition := relcompile.Compose(second.registry)
	second.install(spec)
	uninstalled := relcompile.NewName(compositionScopeOwner, "never-installed")
	placement := relcompile.Placement{Candidate: uninstalled}
	for port := 0; port < spec.Program.InputCount(); port++ {
		placement.Ports = append(placement.Ports, uninstalled)
	}
	if _, err := composition.Resolve(ordinal, spec, placement); err == nil {
		t.Fatal("a rule was placed at a scope no owner installed")
	}
	if _, published := composition.Placement(ordinal); published {
		t.Fatal("a refused lowering published a placement")
	}

	bundle, refusal := lowering.compilation.InputBundle(lowering.owner, composition)
	if bundle != nil || refusal == nil {
		t.Fatal("a catalog whose rules were never placed sealed a bundle")
	}
	if refusal.Reason() != relinput.ReasonUnplaced {
		t.Fatalf("refusal = %s, want the unplaced rule it could not place", refusal.Reason())
	}
	refused, ruled := refusal.Ordinal()
	if !ruled {
		t.Fatal("the refusal names no rule ordinal")
	}
	// The refusal belongs to a rule that carries a program. An ordinal that
	// declared none is an absent row and is never something to place.
	plan, held := lowering.plans.At(refused)
	if !held || !plan.Present() {
		t.Fatalf("the seal refused ordinal %d, which declares no execution program", refused)
	}
}

// oneLoweredOrdinal answers the first ordinal the composition lowered, which
// is the rule the refusal laws are stated against.
func oneLoweredOrdinal(t *testing.T, lowering sealedLowering) (int, rule.Spec) {
	t.Helper()
	for ordinal := 0; ordinal < lowering.plans.Count(); ordinal++ {
		spec, lowered := lowering.specs[ordinal]
		if lowered {
			return ordinal, spec
		}
	}
	t.Fatal("the composition lowered no rule")
	return 0, rule.Spec{}
}

// TestThePublishedPortOrderIsTheRulesDeclaredReadOrder states the per-read
// discipline the port column carries. Port i of a published row is the scope
// the rule's declared input port i observes, and the joins a read lowered to
// carry that same scope, so a read and the table the mount reads it back from
// cannot disagree about where the read stands.
func TestThePublishedPortOrderIsTheRulesDeclaredReadOrder(t *testing.T) {
	lowering := lowerSealedCatalog(t)
	bundle, refusal := lowering.compilation.InputBundle(lowering.owner, lowering.composition)
	if refusal != nil {
		t.Fatalf("the sealed catalog refused its own composition: %v", refusal)
	}
	site := relcompile.Site{Path: "composition.port"}
	compared, multiport := 0, 0
	for ordinal, authored := range lowering.authored {
		spec := lowering.specs[ordinal]
		if len(authored.Ports) > 1 {
			multiport++
		}
		for port, name := range authored.Ports {
			// The expectation is the identity the registry issued for the
			// name this port was placed at, resolved independently of the
			// table under test.
			placed, err := lowering.surfaces.registry.Scope(site, name)
			if err != nil {
				t.Fatalf("ordinal %d port %d names an unresolvable scope: %v", ordinal, port, err)
			}
			published, stated := bundle.PortScope(ordinal, port)
			if !stated || published != placed {
				t.Fatalf("ordinal %d port %d = %v/%t, want the scope %v that port was placed at", ordinal, port, published, stated, name)
			}
			compared++
		}
		// Every scope a lowered join observes is a port scope the row
		// published, because a read observes the port it was declared on and
		// nothing else.
		for _, lowered := range lowering.rules[ordinal] {
			for index, join := range lowered.Joins {
				if !join.Scope.Available() {
					continue
				}
				if !statesPortScope(bundle, ordinal, len(authored.Ports), join.Scope) {
					t.Fatalf("ordinal %d join %d observes %v, which the published row does not carry at any port", ordinal, index, join.Scope)
				}
			}
		}
		// A read is declared on one port, and a port the declaration does not
		// number is a port the published row must not carry.
		for index := 0; index < spec.Program.JoinCount(); index++ {
			declaration, held := spec.Program.JoinAt(index)
			if !held {
				t.Fatalf("ordinal %d declares no join at %d", ordinal, index)
			}
			port := int(declaration.Read.Input.Uint64())
			if port < 0 || port >= len(authored.Ports) {
				t.Fatalf("ordinal %d join %d reads port %d, outside the %d ports it declared", ordinal, index, port, len(authored.Ports))
			}
			placed, err := lowering.surfaces.registry.Scope(site, authored.Ports[port])
			if err != nil {
				t.Fatalf("ordinal %d port %d names an unresolvable scope: %v", ordinal, port, err)
			}
			if !readObservesScope(lowering.rules[ordinal], placed) {
				t.Fatalf("ordinal %d join %d is declared on port %d and no lowered read observes that port's scope", ordinal, index, port)
			}
		}
	}
	if compared == 0 {
		t.Fatal("no rule of the sealed catalog declares an input port")
	}
	if multiport == 0 {
		t.Fatal("no rule of the sealed catalog declares two input ports, so port order states nothing here")
	}
}

// readObservesScope reports whether any read one rule lowered observes scope.
func readObservesScope(rules []relcompile.Rule, scope model.ScopeID) bool {
	for _, lowered := range rules {
		for _, join := range lowered.Joins {
			if join.Scope == scope {
				return true
			}
		}
		if lowered.Carry != nil && lowered.Carry.Scope == scope {
			return true
		}
	}
	return false
}

// statesPortScope reports whether one published row carries scope at any of
// its ports.
func statesPortScope(bundle *relinput.Bundle, ordinal, ports int, scope model.ScopeID) bool {
	for port := 0; port < ports; port++ {
		published, stated := bundle.PortScope(ordinal, port)
		if stated && published == scope {
			return true
		}
	}
	return false
}

// TestTheSealedBundleAnswersWhatTheLoweringResolved states the end-to-end
// reading. The composition answers where the sealed catalog's rules stand,
// the compilation seals that answer against its own rule ordinals, and the
// mount-side projection reads back exactly the identities the lowering
// resolved - not a fixture restating them.
func TestTheSealedBundleAnswersWhatTheLoweringResolved(t *testing.T) {
	lowering := lowerSealedCatalog(t)
	bundle, refusal := lowering.compilation.InputBundle(lowering.owner, lowering.composition)
	if refusal != nil {
		t.Fatalf("the sealed catalog refused its own composition: %v", refusal)
	}
	if bundle.Catalog() != lowering.plans.Digest() {
		t.Fatal("the bundle is fenced to a catalog other than the one that numbered its ordinals")
	}
	if bundle.Owner() != lowering.owner {
		t.Fatal("the bundle names an issuing owner other than the one its scopes were issued by")
	}

	store, storeIssued := identity.IssueStore()
	if !storeIssued {
		t.Fatal("store not issued")
	}
	frozen, frozeOK := bundle.Freeze(store)
	if !frozeOK {
		t.Fatal("the sealed bundle did not freeze")
	}
	view, opened := relinput.Open(&frozen, bundle.Catalog(), bundle.Owner())
	if !opened {
		t.Fatal("the frozen bundle did not open")
	}
	projection, projected := inputscope.Project(view)
	if !projected {
		t.Fatal("the opened bundle projects nothing to mount")
	}
	if projection.RuleCount() != lowering.plans.Count() {
		t.Fatalf("projected ordinals = %d, want the catalog's %d", projection.RuleCount(), lowering.plans.Count())
	}
	for ordinal, resolved := range lowering.resolved {
		candidate, stated := projection.CandidateScope(ordinal)
		if !stated || candidate != resolved.Candidate {
			t.Fatalf("ordinal %d projects candidate %v/%t, want the scope the lowering resolved", ordinal, candidate, stated)
		}
		width, held := projection.PortCount(ordinal)
		if !held || width != len(resolved.Ports) {
			t.Fatalf("ordinal %d projects %d ports, want the %d it declared", ordinal, width, len(resolved.Ports))
		}
		for port, scope := range resolved.Ports {
			observed, observedHeld := projection.PortScope(ordinal, port)
			if !observedHeld || observed != scope {
				t.Fatalf("ordinal %d port %d projects %v/%t, want the scope the lowering resolved", ordinal, port, observed, observedHeld)
			}
		}
	}
}

// TestTheIssuedRegionEvidenceRoundTripsThroughTheFrozenBundle states where
// region evidence comes from and that freezing does not change it. Every
// scope the table names stands on the identity that scope's own owner issued
// when it installed it, and a physical region is admitted for that scope
// exactly when it projects that identity.
func TestTheIssuedRegionEvidenceRoundTripsThroughTheFrozenBundle(t *testing.T) {
	lowering := lowerSealedCatalog(t)
	bundle, refusal := lowering.compilation.InputBundle(lowering.owner, lowering.composition)
	if refusal != nil {
		t.Fatalf("the sealed catalog refused its own composition: %v", refusal)
	}
	if bundle.RegionCount() == 0 {
		t.Fatal("the bundle names no scope and therefore stands on no evidence")
	}
	// The expectation is the token the declaration surface issued for each
	// scope when it installed it, taken from the surface itself. Nothing here
	// reads the evidence back out of the registry that recorded it.
	names := lowering.scopeNames(t)

	store, storeIssued := identity.IssueStore()
	if !storeIssued {
		t.Fatal("store not issued")
	}
	frozen, frozeOK := bundle.Freeze(store)
	if !frozeOK {
		t.Fatal("the sealed bundle did not freeze")
	}
	view, opened := relinput.Open(&frozen, bundle.Catalog(), bundle.Owner())
	if !opened {
		t.Fatal("the frozen bundle did not open")
	}
	projection, projected := inputscope.Project(view)
	if !projected {
		t.Fatal("the opened bundle projects nothing to mount")
	}
	if projection.ScopeCount() != bundle.RegionCount() {
		t.Fatalf("projected scopes = %d, want the sealed %d", projection.ScopeCount(), bundle.RegionCount())
	}
	for index := 0; index < bundle.RegionCount(); index++ {
		region, held := bundle.RegionAt(index)
		if !held || !region.Available() {
			t.Fatalf("region %d states no scope", index)
		}
		name, named := names[region.Scope()]
		if !named {
			t.Fatalf("region %d names a scope no placement of this catalog stated", index)
		}
		issued := lowering.surfaces.token("scope", name)
		if region.Evidence() != issued {
			t.Fatalf("scope %s stands on %v, want the identity its own owner issued", name, region.Evidence())
		}
		projectedScope, projectedEvidence, projectedHeld := projection.ScopeAt(index)
		if !projectedHeld || projectedScope != region.Scope() || projectedEvidence != region.Evidence() {
			t.Fatalf("scope %d projects %v/%v/%t, want the sealed row", index, projectedScope, projectedEvidence, projectedHeld)
		}
		if !projection.Admits(region.Scope(), issued) {
			t.Fatal("a region projecting the identity its owner issued was not admitted")
		}
		if projection.Admits(region.Scope(), bundle.Catalog()) {
			t.Fatal("a region projecting a foreign identity was admitted")
		}
	}
}

// scopeNames addresses every scope this lowering placed a rule at by the
// authored name it was installed under, so a sealed row can be compared
// against the surface that issued it.
func (lowering sealedLowering) scopeNames(t *testing.T) map[model.ScopeID]relcompile.Name {
	t.Helper()
	site := relcompile.Site{Path: "composition.scope"}
	names := map[model.ScopeID]relcompile.Name{}
	for _, authored := range lowering.authored {
		for _, name := range append([]relcompile.Name{authored.Candidate}, authored.Ports...) {
			scope, err := lowering.surfaces.registry.Scope(site, name)
			if err != nil {
				t.Fatalf("scope %s does not resolve: %v", name, err)
			}
			names[scope] = name
		}
	}
	return names
}
