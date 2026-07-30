package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type PathValuePresenceImplicationStep struct {
	implication factflow.PathValuePresenceImplication
}

func (s PathValuePresenceImplicationStep) Implication() factflow.PathValuePresenceImplication {
	return detachPathValuePresenceImplication(s.implication)
}

// PathValuePresenceImplicationTransaction is the immutable ordered N2
// publication/closure program for one CFG point.
type PathValuePresenceImplicationTransaction struct {
	point cfg.Point
	steps []PathValuePresenceImplicationStep
}

// PresenceImplicationPlan is the one canonical publication/consequence
// program used by path-value and call-return producers.
// Both whole-State execution and guarded coordinate execution consume this
// frozen publication inventory; neither adapter is allowed to reconstruct the
// implication relation independently.
type PresenceImplicationPlan struct {
	reg          *axis.Registry
	keys         *keyspace.KeySpace
	access       presenceKeyAccess
	resolver     *visibility.Resolver
	point        cfg.Point
	publications []pathevidence.PathPresenceImplication
	barriers     ConcretePresenceImplicationBarriers
}

// PreparePresenceImplicationPlan seals one already-resolved publication batch
// in this authority's concrete keyspace. It is the sole constructor for a
// concrete presence plan: keyspace ownership, canonical row order and lexical
// accessibility are established together.
func (a *PathSemanticAuthority) PreparePresenceImplicationPlan(
	reg *axis.Registry,
	point cfg.Point,
	publications []pathevidence.PathPresenceImplication,
	barriers ConcretePresenceImplicationBarriers,
) (PresenceImplicationPlan, error) {
	if reg == nil || !a.Valid() {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: invalid resolved presence implication authority")
	}
	keys := a.resolver.KeySpace()
	// Publication order is semantic when descendant invalidation barriers are
	// selected. Validate each immutable row through the family canonicalizer,
	// but do not globally sort or deduplicate the producer's ordered program.
	canonical := make([]pathevidence.PathPresenceImplication, len(publications))
	for index, publication := range publications {
		row, ok := pathevidence.CanonicalPathPresenceImplications(reg, keys, []pathevidence.PathPresenceImplication{publication})
		if !ok || len(row) != 1 || row[0] != publication {
			return PresenceImplicationPlan{}, fmt.Errorf("factapply: invalid resolved presence implication %d", index)
		}
		canonical[index] = publication
	}
	access, err := freezePresenceKeyAccess(a.resolver, point, keys, presenceImplicationPaths(canonical))
	if err != nil {
		return PresenceImplicationPlan{}, err
	}
	return PresenceImplicationPlan{
		reg: reg, keys: keys, access: access, resolver: a.resolver, point: point,
		publications: canonical, barriers: barriers,
	}, nil
}

// RekeyFormal transports the complete frozen presence relation through the
// same registered root substitution as its coordinate inventory. Publication
// endpoints and lexical accessibility cross together; no destination-side
// resolver reconstructs source visibility.
func (p PresenceImplicationPlan) RekeyFormal(
	domain state.ProductDomain,
	rekey state.CoordinateFormalRootRekey,
) (PresenceImplicationPlan, error) {
	if p.reg == nil || p.keys == nil || !p.keys.Valid() || !p.access.valid() || domain.Registry() != p.reg {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: invalid presence implication formal rekey")
	}
	target, ok := domain.CoordinateFormalDestinationKeySpace(rekey)
	if !ok {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: presence implication formal destination is absent")
	}
	mapKey := func(source keyspace.Key) (keyspace.Key, error) {
		return domain.RekeyStructuralKeyFormal(rekey, source)
	}
	rows := make([]pathevidence.PathPresenceImplication, len(p.publications))
	for index, source := range p.publications {
		row := source
		var err error
		row.Trigger, err = mapKey(source.Trigger)
		if err != nil {
			return PresenceImplicationPlan{}, err
		}
		if source.HasTriggerPathEqual {
			row.TriggerOther, err = mapKey(source.TriggerOther)
			if err != nil {
				return PresenceImplicationPlan{}, err
			}
		}
		row.Target, err = mapKey(source.Target)
		if err != nil {
			return PresenceImplicationPlan{}, err
		}
		rows[index] = row
	}
	canonical, valid := pathevidence.CanonicalPathPresenceImplications(p.reg, target, rows)
	if !valid {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: rekeyed presence implication inventory is invalid")
	}
	access, err := p.access.rekeyFormal(domain, rekey)
	if err != nil {
		return PresenceImplicationPlan{}, err
	}
	return PresenceImplicationPlan{
		reg: p.reg, keys: target, access: access, point: p.point,
		publications: canonical, barriers: p.barriers,
	}, nil
}

// SealCoordinateFactorInventory seals producer-declared coordinates in the
// authority's canonical keyspace.  Consumers receive an immutable inventory,
// never a parallel slot slice whose completeness depends on call-site
// discipline.
func (a *PathSemanticAuthority) SealCoordinateFactorInventory(
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) (state.CoordinateFactorInventory, error) {
	if !a.Valid() || !domain.Valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: invalid coordinate factor inventory authority")
	}
	return domain.SealCoordinateFactorInventory(a.resolver.KeySpace(), slots)
}

func (a *PathSemanticAuthority) CoordinateFactorInventoryFromPreparedState(
	domain state.ProductDomain,
	prepared state.State,
) (state.CoordinateFactorInventory, error) {
	if !a.Valid() || !domain.Valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: invalid prepared-state coordinate inventory authority")
	}
	return domain.CoordinateFactorInventoryFromPreparedState(a.resolver.KeySpace(), prepared)
}

func (a *PathSemanticAuthority) UnionCoordinateFactorInventories(
	domain state.ProductDomain,
	inventories ...state.CoordinateFactorInventory,
) (state.CoordinateFactorInventory, error) {
	if !a.Valid() || !domain.Valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: invalid coordinate factor inventory authority")
	}
	return domain.UnionCoordinateFactorInventories(a.resolver.KeySpace(), inventories...)
}

func (a *PathSemanticAuthority) CloseCoordinateFactorInventory(
	domain state.ProductDomain,
	seed state.CoordinateFactorInventory,
) (state.CoordinateFactorInventory, error) {
	closed, _, err := a.closeCoordinateFactorInventory(domain, seed)
	return closed, err
}

// closeCoordinateFactorInventory also returns the number of exact universe
// constructions. Keeping this counter local to construction makes the width
// law testable without timing, global instrumentation or a second algorithm.
func (a *PathSemanticAuthority) closeCoordinateFactorInventory(
	domain state.ProductDomain,
	seed state.CoordinateFactorInventory,
) (state.CoordinateFactorInventory, int, error) {
	if !a.Valid() || !domain.Valid() || domain.Registry() == nil ||
		!seed.ValidFor(domain, a.resolver.KeySpace()) {
		return state.CoordinateFactorInventory{}, 0, fmt.Errorf("factapply: invalid coordinate factor inventory authority")
	}
	// Inventory closure is a property of the exact sealed coordinate universe,
	// not of the syntax node which happened to publish each row. Publication
	// annotations remain transaction-owned reducer/delta projections. The
	// descriptor below has no publications and therefore builds only the
	// family-owned graph induced by rows already present in the closed inventory.
	universe := PresenceImplicationPlan{
		reg: domain.Registry(), keys: a.resolver.KeySpace(), resolver: a.resolver,
	}
	current := seed
	universeConstructions := 0
	for {
		closed, err := domain.CloseCoordinateFactorInventory(a.resolver.KeySpace(), current)
		if err != nil {
			return state.CoordinateFactorInventory{}, universeConstructions, err
		}
		if _, registered := domain.PathEvidenceCoordinateFamily(); !registered {
			return closed, universeConstructions, nil
		}
		universeConstructions++
		dependency, dependencyErr := universe.DependencyBlocks(domain, closed)
		if dependencyErr != nil {
			return state.CoordinateFactorInventory{}, universeConstructions, dependencyErr
		}
		inventory, inventoryErr := domain.SealCoordinateFactorInventory(a.resolver.KeySpace(), dependency.Slots())
		if inventoryErr != nil {
			return state.CoordinateFactorInventory{}, universeConstructions, inventoryErr
		}
		next, err := domain.UnionCoordinateFactorInventories(a.resolver.KeySpace(), closed, inventory)
		if err != nil {
			return state.CoordinateFactorInventory{}, universeConstructions, err
		}
		// next is the set union with closed, so equal cardinality proves exact
		// inventory equality rather than relying on slot count accidentally.
		if next.Len() == closed.Len() {
			return closed, universeConstructions, nil
		}
		current = next
	}
}

// CoordinateFactorInventory returns this producer's complete declared
// coordinate set. Dependency closure remains a separate body-wide operation:
// it must see the union of every producer before branch topology is frozen.
func (p PresenceImplicationPlan) CoordinateFactorInventory(
	domain state.ProductDomain,
) (state.CoordinateFactorInventory, error) {
	if p.reg == nil || p.keys == nil || !p.keys.Valid() || !domain.Valid() || domain.Registry() != p.reg {
		return state.CoordinateFactorInventory{}, fmt.Errorf("factapply: invalid presence coordinate inventory plan")
	}
	slots := make([]state.CoordinateSlot, 0, len(p.publications))
	for _, publication := range p.publications {
		slot, err := domain.PresenceImplicationCoordinateSlot(p.keys, publication)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, slot)
	}
	return domain.SealCoordinateFactorInventory(p.keys, slots)
}

// SealCoordinateFactorInventory imports already-owned coordinates into this
// plan's keyspace and unions them with the plan's publications. It is used by
// invocation-rebased producers whose exact coordinate names are unavailable
// at lexical freeze time.
func (p PresenceImplicationPlan) SealCoordinateFactorInventory(
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) (state.CoordinateFactorInventory, error) {
	producer, err := p.CoordinateFactorInventory(domain)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	imported, err := domain.SealCoordinateFactorInventory(p.keys, slots)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return domain.UnionCoordinateFactorInventories(p.keys, producer, imported)
}

func PlanPathValuePresenceImplicationTransaction(facts factflow.Facts, point cfg.Point) PathValuePresenceImplicationTransaction {
	implications := facts.PathValuePresenceImplications(point)
	steps := make([]PathValuePresenceImplicationStep, len(implications))
	for index, implication := range implications {
		steps[index] = PathValuePresenceImplicationStep{implication: implication}
	}
	return PathValuePresenceImplicationTransaction{point: point, steps: steps}
}

func (t PathValuePresenceImplicationTransaction) Point() cfg.Point { return t.point }
func (t PathValuePresenceImplicationTransaction) Len() int         { return len(t.steps) }
func (t PathValuePresenceImplicationTransaction) Clone() PathValuePresenceImplicationTransaction {
	out := PathValuePresenceImplicationTransaction{point: t.point, steps: make([]PathValuePresenceImplicationStep, len(t.steps))}
	for index, step := range t.steps {
		out.steps[index] = PathValuePresenceImplicationStep{implication: step.Implication()}
	}
	return out
}
func (t PathValuePresenceImplicationTransaction) HasPublicationSteps() bool {
	return len(t.steps) != 0
}

func (t PathValuePresenceImplicationTransaction) Step(index int) (PathValuePresenceImplicationStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return PathValuePresenceImplicationStep{}, false
	}
	return PathValuePresenceImplicationStep{implication: t.steps[index].Implication()}, true
}

func detachPathValuePresenceImplication(implication factflow.PathValuePresenceImplication) factflow.PathValuePresenceImplication {
	switch {
	case implication.HasTriggerPathEqual():
		return factflow.NewPathEqualityValueRefinementImplication(
			implication.TriggerPath(), implication.TriggerOtherPath(), implication.TargetPath(), implication.TargetValue(),
		)
	case implication.HasTriggerPresence():
		return factflow.NewPathTruthyValueRefinementImplication(
			implication.TriggerPath(), implication.TriggerValue(), implication.TargetPath(), implication.TargetValue(),
		)
	case implication.HasTargetValue():
		return factflow.NewPathValueRefinementImplication(
			implication.TriggerPath(), implication.TriggerValue(), implication.TargetPath(), implication.TargetValue(),
		)
	default:
		return factflow.NewPathValuePresenceImplication(
			implication.TriggerPath(), implication.TriggerValue(), implication.TargetPath(), implication.TargetPresence(),
		)
	}
}

func (t PathValuePresenceImplicationTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, step := range t.steps {
		implication := step.implication
		if !product.BelongsToRegistry(reg, implication.TriggerValue()) ||
			!product.BelongsToRegistry(reg, implication.TargetValue()) {
			return false
		}
	}
	return true
}

// PreparePathValuePresenceImplications resolves lexical paths once into the
// immutable keyspace relation shared by concrete and coordinate execution.
func (a *PathSemanticAuthority) PreparePathValuePresenceImplications(
	reg *axis.Registry,
	transaction PathValuePresenceImplicationTransaction,
) (PresenceImplicationPlan, error) {
	if reg == nil || !a.Valid() || !transaction.Valid(reg) {
		return PresenceImplicationPlan{}, fmt.Errorf("factapply: invalid path-value presence implication authority")
	}
	publications := make([]pathevidence.PathPresenceImplication, 0, transaction.Len())
	for index := 0; index < transaction.Len(); index++ {
		step, _ := transaction.Step(index)
		implication, ok := pathValuePresenceImplicationAt(
			transfer.NodeContext{Registry: reg, Point: transaction.point},
			a.resolver,
			step.implication,
		)
		if ok {
			publications = append(publications, implication)
		}
	}
	return a.PreparePresenceImplicationPlan(
		reg, transaction.point, publications, ConcretePresenceImplicationDescendantInvalidationBarriers,
	)
}
