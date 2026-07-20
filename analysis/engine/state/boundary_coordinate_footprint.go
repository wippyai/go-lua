package state

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryCoordinateFootprintPlan is the incremental Horn consequence plan
// for one boundary frame. Advance consumes monotone coordinate inventories and
// returns exactly the destination coordinates proved since the prior advance.
// The plan contains topology only: it never constructs a State, family
// skeleton, or scalar payload.
type BoundaryCoordinateFootprintPlan struct {
	data *boundaryCoordinateFootprintData
}

// BoundaryCoordinateFootprintTrace is a detached diagnostic snapshot of the
// finite closure proof. It is inert unless requested by the transformer trace
// hook and exposes no mutable plan state.
type BoundaryCoordinateFootprintTrace struct {
	RootMap                            BoundaryRootMap
	SourceRoots                        []BoundaryFactorRoot
	SourceSeen, DestinationSeen, Image CoordinateFactorInventory
	Targets                            []BoundaryCoordinateFootprintTargetTrace
}

type BoundaryCoordinateFootprintTargetTrace struct {
	Slot                            CoordinateSlot
	Required, Satisfied             int
	Emitted                         bool
	RequiredFibers, SatisfiedFibers []string
}

func (p BoundaryCoordinateFootprintPlan) TraceSnapshot() BoundaryCoordinateFootprintTrace {
	if p.data == nil {
		return BoundaryCoordinateFootprintTrace{}
	}
	out := BoundaryCoordinateFootprintTrace{
		RootMap: append(BoundaryRootMap(nil), p.data.rootMap...), SourceRoots: append([]BoundaryFactorRoot(nil), p.data.sourceRoots...),
		SourceSeen: p.data.sourceSeen, DestinationSeen: p.data.destinationSeen, Image: p.data.image,
	}
	for _, bucket := range p.data.targetBuckets {
		for _, target := range bucket {
			required := make([]string, len(target.required))
			for index, fiber := range target.required {
				required[index] = fmt.Sprintf("%#v", fiber)
			}
			satisfied := make([]string, 0, len(target.satisfied))
			for fiber := range target.satisfied {
				satisfied = append(satisfied, fmt.Sprintf("%#v", fiber))
			}
			sort.Strings(required)
			sort.Strings(satisfied)
			out.Targets = append(out.Targets, BoundaryCoordinateFootprintTargetTrace{Slot: target.slot, Required: len(target.required), Satisfied: len(target.satisfied), Emitted: target.emitted, RequiredFibers: required, SatisfiedFibers: satisfied})
		}
	}
	return out
}

type boundaryCoordinateFootprintData struct {
	domain, sourceDomain ProductDomain
	authority            *BoundaryAllocationAuthority
	identityTerms        *CoordinateIdentityTermImage
	destinationKeys      *keyspace.KeySpace
	rootMap              BoundaryRootMap
	existentials         BoundaryExistentialNamespace
	sourceRoots          []BoundaryFactorRoot

	bound       bool
	sourceKeys  *keyspace.KeySpace
	selection   BoundaryFactorSelection
	roots       boundaryPathMap
	slots       map[statekey.Value][]statekey.Value
	aliases     [][2]keyspace.Key
	targets     []BoundaryFactorTarget
	quotient    boundaryInverseQuotient
	destination BoundaryClosure

	families             []boundaryFootprintFamily
	sourceInputSeen      CoordinateFactorInventory
	sourceSeen           CoordinateFactorInventory
	destinationInputSeen CoordinateFactorInventory
	destinationSeen      CoordinateFactorInventory
	image                CoordinateFactorInventory
	waits                map[boundaryAffectedAtom][]*boundaryFootprintWait
	zeroPremises         []CoordinateSlot
	targetBuckets        map[boundaryFootprintTargetHash][]*boundaryFootprintTarget
	identitySources      map[identity.Term][]boundaryFootprintIdentitySource
}

type boundaryFootprintFamily struct {
	source, destination *coordinateFamilyRuntime
}

type boundaryFootprintTargetHash struct {
	family CoordinateFamily
	hash   uint64
}

type boundaryFootprintTarget struct {
	slot      CoordinateSlot
	required  []coordinateFiberPayload
	satisfied map[coordinateFiberPayload]struct{}
	emitted   bool
}

type boundaryFootprintWait struct {
	slot     CoordinateSlot
	selector boundaryAffectedSelector
	emitted  bool
}

// boundaryFootprintIdentitySource is one already-admitted projected source
// coordinate whose registered key law names a formal identity term. The
// reverse incidence index lets an image extension wake only affected slots;
// no image advance scans or rebuilds the source inventory.
type boundaryFootprintIdentitySource struct {
	family int
	key    coordinateKeyPayload
}

// PrepareBoundaryCoordinateFootprintPlan freezes the frame-static boundary
// relation and the registry-owned family correspondence. Source keyspace
// authority is intentionally bound by the first Advance, where the source
// inventory supplies it; subsequent advances must retain that authority.
func (d ProductDomain) PrepareBoundaryCoordinateFootprintPlan(
	sourceDomain ProductDomain,
	authority *BoundaryAllocationAuthority,
	destinationKeys *keyspace.KeySpace,
	rootMap BoundaryRootMap,
	existentials BoundaryExistentialNamespace,
	sourceRoots []BoundaryFactorRoot,
	identityTerms ...*CoordinateIdentityTermImage,
) (BoundaryCoordinateFootprintPlan, error) {
	if !d.Valid() || !sourceDomain.Valid() || d.reg != sourceDomain.reg || authority == nil || destinationKeys == nil || !destinationKeys.Valid() {
		return BoundaryCoordinateFootprintPlan{}, fmt.Errorf("%w: boundary coordinate footprint frame is unowned", ErrInvalidLaneFactor)
	}
	transport, err := authority.BindTransport(destinationKeys, rootMap, existentials)
	if err != nil {
		return BoundaryCoordinateFootprintPlan{}, err
	}
	families := make([]boundaryFootprintFamily, 0)
	for _, sourceLane := range sourceDomain.LaneInventory() {
		sourceFamilies, familyErr := sourceDomain.CoordinateFamilies(sourceLane)
		if familyErr != nil {
			return BoundaryCoordinateFootprintPlan{}, familyErr
		}
		for _, sourceFamily := range sourceFamilies {
			sourceRuntime, sourceErr := sourceDomain.validateCoordinateFamily(sourceFamily)
			destinationRuntime, destinationErr := d.coordinateRuntimeForImport(sourceFamily)
			if sourceErr != nil || destinationErr != nil {
				return BoundaryCoordinateFootprintPlan{}, fmt.Errorf("%w: destination coordinate family %q is absent", ErrInvalidLaneFactor, sourceFamily.ID())
			}
			families = append(families, boundaryFootprintFamily{source: sourceRuntime, destination: destinationRuntime})
		}
	}
	var termImage *CoordinateIdentityTermImage
	if len(identityTerms) > 1 {
		return BoundaryCoordinateFootprintPlan{}, fmt.Errorf("%w: boundary coordinate footprint has multiple identity images", ErrInvalidLaneFactor)
	}
	if len(identityTerms) == 1 {
		termImage = identityTerms[0]
	}
	data := &boundaryCoordinateFootprintData{
		domain: d, sourceDomain: sourceDomain, authority: authority,
		identityTerms:   termImage,
		destinationKeys: destinationKeys, rootMap: append(BoundaryRootMap(nil), transport.roots...),
		existentials: existentials, sourceRoots: append([]BoundaryFactorRoot(nil), sourceRoots...),
		families: families, waits: make(map[boundaryAffectedAtom][]*boundaryFootprintWait),
		targetBuckets:   make(map[boundaryFootprintTargetHash][]*boundaryFootprintTarget),
		identitySources: make(map[identity.Term][]boundaryFootprintIdentitySource),
	}
	return BoundaryCoordinateFootprintPlan{data: data}, nil
}

// Advance admits monotone source and destination inventories. All fallible
// family operations are staged before the plan is mutated, so a rejected
// advance leaves the prior plan byte-for-byte usable. Successful advances are
// linearly consumed: callers retain the returned plan even when exactAdded is
// empty because premise progress may still have advanced.
func (p BoundaryCoordinateFootprintPlan) Advance(source, destination CoordinateFactorInventory) (BoundaryCoordinateFootprintPlan, CoordinateFactorInventory, error) {
	if p.data == nil {
		return BoundaryCoordinateFootprintPlan{}, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint plan is empty", ErrInvalidLaneFactor)
	}
	return p.advance(source, destination, p.data.identityTerms, false)
}

// AdvanceWithIdentityImage admits the full current finite image of formal
// identity sources. The image must monotonically extend the prior successful
// image. Only previously admitted source coordinates incident to a source
// whose image grew are replayed, and only their newly imaged keys contribute.
// Rejection is transactional: the prior plan and prior image remain usable.
func (p BoundaryCoordinateFootprintPlan) AdvanceWithIdentityImage(
	source, destination CoordinateFactorInventory,
	current *CoordinateIdentityTermImage,
) (BoundaryCoordinateFootprintPlan, CoordinateFactorInventory, error) {
	if current == nil {
		return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint identity image is absent", ErrInvalidLaneFactor)
	}
	return p.advance(source, destination, current, true)
}

func (p BoundaryCoordinateFootprintPlan) advance(
	source, destination CoordinateFactorInventory,
	currentImage *CoordinateIdentityTermImage,
	explicitImage bool,
) (BoundaryCoordinateFootprintPlan, CoordinateFactorInventory, error) {
	if p.data == nil {
		return BoundaryCoordinateFootprintPlan{}, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint plan is empty", ErrInvalidLaneFactor)
	}
	d := p.data
	if !source.ValidFor(d.sourceDomain, source.KeySpace()) || !destination.ValidFor(d.domain, d.destinationKeys) {
		return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint inventory is foreign", ErrInvalidLaneFactor)
	}
	if d.bound && source.KeySpace() != d.sourceKeys {
		return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint source keyspace changed", ErrInvalidLaneFactor)
	}
	nextPlan := p
	if !d.bound {
		copyData := *d
		copyData.waits = make(map[boundaryAffectedAtom][]*boundaryFootprintWait)
		copyData.targetBuckets = make(map[boundaryFootprintTargetHash][]*boundaryFootprintTarget)
		copyData.identitySources = make(map[identity.Term][]boundaryFootprintIdentitySource)
		if err := copyData.bind(source.KeySpace()); err != nil {
			return p, CoordinateFactorInventory{}, err
		}
		d = &copyData
		nextPlan.data = d
	}
	priorImage := d.identityTerms
	var imageDelta *CoordinateIdentityTermImage
	if explicitImage {
		if priorImage == nil {
			if d.sourceSeen.Len() != 0 {
				return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint identity image began after source admission", ErrInvalidLaneFactor)
			}
			priorImage, _ = NewCoordinateIdentityTermImage(nil)
		}
		var monotone bool
		imageDelta, monotone = currentImage.Delta(priorImage)
		if !monotone {
			return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint identity image is not monotone", ErrInvalidLaneFactor)
		}
	}
	sourceDelta, err := coordinateInventoryDelta(d.sourceDomain, source.KeySpace(), d.sourceInputSeen, source)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	destinationDelta, err := coordinateInventoryDelta(d.domain, d.destinationKeys, d.destinationInputSeen, destination)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	// Completion is unary and registered per family. Closing only the exact raw
	// delta computes every new consequence without revisiting an old premise.
	completedDelta, err := d.sourceDomain.CloseCoordinateFactorInventory(d.sourceKeys, sourceDelta)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	completedDelta, err = coordinateInventorySubtract(d.sourceDomain, d.sourceKeys, completedDelta, d.sourceSeen)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	completedDestinationDelta, err := d.domain.CloseCoordinateFactorInventory(d.destinationKeys, destinationDelta)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	completedDestinationDelta, err = coordinateInventorySubtract(d.domain, d.destinationKeys, completedDestinationDelta, d.destinationSeen)
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	nextSelection, err := d.sourceDomain.selectBoundaryFactorCoordinates(d.selection, completedDelta.Slots())
	if err != nil {
		return p, CoordinateFactorInventory{}, err
	}
	nextRoots, err := completeBoundaryRootMap(d.sourceKeys, d.destinationKeys, nextSelection.closure, d.roots, d.existentials)
	if err != nil {
		return p, CoordinateFactorInventory{}, fmt.Errorf("state: boundary coordinate footprint root completion: %w", err)
	}
	nextDestination, err := rebaseBoundaryClosure(d.sourceKeys, d.destinationKeys, nextSelection.closure, nextRoots, d.slots, d.authority)
	if err != nil {
		return p, CoordinateFactorInventory{}, fmt.Errorf("state: boundary coordinate footprint closure rebase: %w", err)
	}
	for _, binding := range d.rootMap {
		if binding.ToSlot != 0 {
			nextDestination.slots[binding.ToSlot] = struct{}{}
		}
		if binding.To.Kind != keyspace.KindInvalid {
			nextDestination.paths[binding.To] = struct{}{}
		}
	}
	nextQuotient := d.quotient
	nextQuotient.inverseRoots = invertBoundaryPathMap(nextRoots)
	rebaseCtx := boundaryRebaseContext{
		reg: d.domain.reg, fromKeys: d.sourceKeys, toKeys: d.destinationKeys,
		roots: nextRoots, slots: d.slots, allocations: d.authority,
		quotient: nextQuotient, fromClosure: nextSelection.closure, toClosure: nextDestination,
		identityImaged: currentImage != nil,
	}
	projectCtx := boundaryProjectContext{reg: d.sourceDomain.reg, keys: d.sourceKeys, closure: nextSelection.closure}

	type stagedContribution struct {
		family   *coordinateFamilyRuntime
		key      coordinateKeyPayload
		fiber    coordinateFiberPayload
		required []coordinateFiberPayload
	}
	type stagedWait struct {
		slot     CoordinateSlot
		selector boundaryAffectedSelector
	}
	type stagedIdentitySource struct {
		term   identity.Term
		source boundaryFootprintIdentitySource
	}
	contributions := make([]stagedContribution, 0, completedDelta.Len())
	waiters := make([]stagedWait, 0, completedDestinationDelta.Len())
	identitySources := make([]stagedIdentitySource, 0)
	immediate := make([]CoordinateSlot, 0)
	stageIdentityKey := func(pair boundaryFootprintFamily, identityKey coordinateKeyPayload) error {
		if identityKey == nil || !pair.source.ops.keyValid(identityKey, d.sourceKeys) {
			return fmt.Errorf("state: coordinate footprint identity image emitted invalid key in family %q", pair.source.family.id)
		}
		fiber := pair.source.boundary.sourceFiber(identityKey)
		if fiber == nil {
			return fmt.Errorf("state: coordinate footprint source fiber is empty in family %q", pair.source.family.id)
		}
		keys, mapped := pair.destination.boundary.rebaseKeys(&rebaseCtx, identityKey)
		if !mapped {
			var terms []string
			pair.source.ops.returnIdentity.visitInventoryTerms(identityKey, func(term identity.Term) bool {
				terms = append(terms, term.String())
				return true
			})
			return fmt.Errorf("state: coordinate footprint key rebase failed in family %q terms=%v imaged=%t", pair.destination.family.id, terms, currentImage != nil)
		}
		for _, key := range keys {
			if key == nil || !pair.destination.ops.keyValid(key, d.destinationKeys) {
				return fmt.Errorf("state: coordinate footprint emitted invalid key in family %q", pair.destination.family.id)
			}
			required, valid := pair.destination.boundaryTargetRequiredFibers(&rebaseCtx, key, fiber)
			if !valid || len(required) == 0 {
				return fmt.Errorf("state: coordinate footprint target admission failed in family %q", pair.destination.family.id)
			}
			contributions = append(contributions, stagedContribution{family: pair.destination, key: key, fiber: fiber, required: append([]coordinateFiberPayload(nil), required...)})
		}
		return nil
	}
	collectIdentityKeys := func(pair boundaryFootprintFamily, projected coordinateKeyPayload, image *CoordinateIdentityTermImage) ([]coordinateKeyPayload, error) {
		if image == nil {
			return []coordinateKeyPayload{projected}, nil
		}
		keys := make([]coordinateKeyPayload, 0, 1)
		if !pair.source.ops.returnIdentity.imageInventoryKey(projected, image, func(key coordinateKeyPayload) bool {
			if key == nil || !pair.source.ops.keyValid(key, d.sourceKeys) {
				return false
			}
			keys = append(keys, key)
			return true
		}) {
			return nil, fmt.Errorf("state: coordinate footprint identity image failed in family %q", pair.source.family.id)
		}
		sort.SliceStable(keys, func(left, right int) bool {
			return pair.source.ops.keyLess(keys[left], keys[right], d.sourceKeys)
		})
		unique := keys[:0]
		for _, key := range keys {
			if len(unique) == 0 || !pair.source.ops.keyEqual(unique[len(unique)-1], key) {
				unique = append(unique, key)
			}
		}
		return unique, nil
	}
	stageProjectedKey := func(pair boundaryFootprintFamily, projected coordinateKeyPayload, image *CoordinateIdentityTermImage) error {
		keys, err := collectIdentityKeys(pair, projected, image)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := stageIdentityKey(pair, key); err != nil {
				return err
			}
		}
		return nil
	}
	for familyIndex, pair := range d.families {
		slots, slotsErr := completedDelta.familySlots(pair.source.family)
		if slotsErr != nil {
			return p, CoordinateFactorInventory{}, slotsErr
		}
		for _, slot := range slots {
			projected, keep, valid := pair.source.boundary.projectKey(&projectCtx, slot.key)
			if !valid {
				return p, CoordinateFactorInventory{}, fmt.Errorf("state: coordinate footprint projection failed in family %q", pair.source.family.id)
			}
			if !keep {
				continue
			}
			if currentImage != nil {
				seenTerms := make(map[identity.Term]struct{})
				if !pair.source.ops.returnIdentity.visitInventoryTerms(projected, func(term identity.Term) bool {
					if _, formal := term.Formal(); !formal {
						return term.Valid()
					}
					if _, seen := seenTerms[term]; !seen {
						seenTerms[term] = struct{}{}
						identitySources = append(identitySources, stagedIdentitySource{
							term: term, source: boundaryFootprintIdentitySource{family: familyIndex, key: projected},
						})
					}
					return true
				}) {
					return p, CoordinateFactorInventory{}, fmt.Errorf("state: coordinate footprint identity incidence failed in family %q", pair.source.family.id)
				}
			}
			if err := stageProjectedKey(pair, projected, currentImage); err != nil {
				return p, CoordinateFactorInventory{}, err
			}
		}
		destinationSlots, slotsErr := completedDestinationDelta.familySlots(pair.destination.family)
		if slotsErr != nil {
			return p, CoordinateFactorInventory{}, slotsErr
		}
		for _, slot := range destinationSlots {
			builder := newBoundaryAffectedSelectorBuilder(d.destinationKeys)
			pair.destination.boundary.affectedSelector(builder, slot.key)
			selector, selectorErr := builder.seal()
			if selectorErr != nil {
				return p, CoordinateFactorInventory{}, selectorErr
			}
			if selector.affected(nextDestination) {
				immediate = append(immediate, slot)
			} else {
				waiters = append(waiters, stagedWait{slot: slot, selector: selector})
			}
		}
	}
	if imageDelta != nil {
		type replayBucket struct {
			family int
			hash   uint64
		}
		replayed := make(map[replayBucket][]coordinateKeyPayload)
		for _, binding := range imageDelta.Bindings() {
			for _, source := range d.identitySources[binding.Source] {
				if source.family < 0 || source.family >= len(d.families) {
					return p, CoordinateFactorInventory{}, fmt.Errorf("%w: boundary coordinate footprint identity incidence is malformed", ErrInvalidLaneFactor)
				}
				pair := d.families[source.family]
				bucket := replayBucket{family: source.family, hash: pair.source.ops.keyHash(source.key, d.sourceKeys)}
				duplicate := false
				for _, prior := range replayed[bucket] {
					if pair.source.ops.keyEqual(prior, source.key) {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				replayed[bucket] = append(replayed[bucket], source.key)
				currentKeys, currentErr := collectIdentityKeys(pair, source.key, currentImage)
				if currentErr != nil {
					return p, CoordinateFactorInventory{}, currentErr
				}
				priorKeys, priorErr := collectIdentityKeys(pair, source.key, priorImage)
				if priorErr != nil {
					return p, CoordinateFactorInventory{}, priorErr
				}
				for currentIndex, priorIndex := 0, 0; currentIndex < len(currentKeys); currentIndex++ {
					for priorIndex < len(priorKeys) && pair.source.ops.keyLess(priorKeys[priorIndex], currentKeys[currentIndex], d.sourceKeys) {
						priorIndex++
					}
					if priorIndex < len(priorKeys) && pair.source.ops.keyEqual(priorKeys[priorIndex], currentKeys[currentIndex]) {
						continue
					}
					if err := stageIdentityKey(pair, currentKeys[currentIndex]); err != nil {
						return p, CoordinateFactorInventory{}, err
					}
				}
			}
		}
	}

	// No family callback below this line can fail: commit all staged premises.
	priorDestination := d.destination
	d.selection, d.roots, d.destination, d.quotient = nextSelection, nextRoots, nextDestination, nextQuotient
	d.sourceSeen, _ = d.sourceDomain.UnionCoordinateFactorInventories(d.sourceKeys, d.sourceSeen, completedDelta)
	d.sourceInputSeen = source
	d.destinationSeen, _ = d.domain.UnionCoordinateFactorInventories(d.destinationKeys, d.destinationSeen, completedDestinationDelta)
	d.destinationInputSeen = destination
	if explicitImage {
		d.identityTerms = currentImage
	}
	for _, incidence := range identitySources {
		d.identitySources[incidence.term] = append(d.identitySources[incidence.term], incidence.source)
	}
	for _, contribution := range contributions {
		target := d.findOrAddTarget(contribution.family, contribution.key, contribution.required)
		target.satisfied[contribution.fiber] = struct{}{}
		if !target.emitted && boundaryFootprintTargetComplete(target) {
			target.emitted = true
			immediate = append(immediate, target.slot)
		}
	}
	for _, waiter := range waiters {
		entry := &boundaryFootprintWait{slot: waiter.slot, selector: waiter.selector}
		for index := 0; index < waiter.selector.incidenceCount(); index++ {
			atom, _ := waiter.selector.incidence(index)
			d.waits[atom] = append(d.waits[atom], entry)
		}
	}
	for _, atom := range boundaryClosureAddedAtoms(priorDestination, nextDestination) {
		for _, waiter := range d.waits[atom] {
			if !waiter.emitted && waiter.selector.affected(nextDestination) {
				waiter.emitted = true
				immediate = append(immediate, waiter.slot)
			}
		}
		delete(d.waits, atom)
	}
	// Root and post heads are seeded exactly once by bind.
	immediate = append(immediate, d.takeZeroPremises()...)
	added, err := d.domain.SealCoordinateFactorInventory(d.destinationKeys, immediate)
	if err != nil {
		panic("state: prevalidated coordinate footprint emission failed: " + err.Error())
	}
	added, err = d.domain.CloseCoordinateFactorInventory(d.destinationKeys, added)
	if err != nil {
		panic("state: registered coordinate footprint completion failed: " + err.Error())
	}
	added, err = coordinateInventorySubtract(d.domain, d.destinationKeys, added, d.image)
	if err != nil {
		panic("state: coordinate footprint image subtraction failed: " + err.Error())
	}
	d.image, _ = d.domain.UnionCoordinateFactorInventories(d.destinationKeys, d.image, added)
	return nextPlan, added, nil
}

func (d *boundaryCoordinateFootprintData) bind(sourceKeys *keyspace.KeySpace) error {
	selection, err := SealBoundaryFactorSelection(sourceKeys, d.sourceRoots, nil, true)
	if err != nil {
		return err
	}
	roots := make(BoundaryRoots, len(d.sourceRoots))
	for index, root := range d.sourceRoots {
		roots[index] = BoundaryRoot{Slot: root.Slot, Path: root.Path, Value: product.Bottom(d.domain.reg)}
	}
	artifact := BoundaryArtifact{reg: d.domain.reg, keys: sourceKeys, closure: selection.closure, roots: roots}
	rootPaths, rootSlots, aliases, rootCount, ok := buildBoundaryRootRelation(artifact, d.destinationKeys, d.rootMap)
	if !ok {
		return fmt.Errorf("state: invalid boundary coordinate footprint root relation")
	}
	effective, err := completeBoundaryRootMap(sourceKeys, d.destinationKeys, selection.closure, rootPaths, d.existentials)
	if err != nil {
		return err
	}
	// Resolver-version companions are frame-static. Seal them now so an AND
	// denominator cannot grow after a destination consequence is published.
	for _, binding := range rootPaths {
		if binding.from.Kind != keyspace.KindResolverSym || binding.from.Segs != 0 {
			continue
		}
		bare := sourceKeys.FromPath(pathdom.Path{Symbol: binding.from.Sym})
		effective, err = completeVersionInsensitiveSymbolRoot(sourceKeys, d.destinationKeys, bare, effective, rootPaths)
		if err != nil {
			return err
		}
	}
	destination, err := rebaseBoundaryClosure(sourceKeys, d.destinationKeys, selection.closure, effective, rootSlots, d.authority)
	if err != nil {
		return err
	}
	targets := make([]BoundaryFactorTarget, rootCount)
	set := make([]bool, rootCount)
	for _, binding := range d.rootMap {
		if set[binding.ToRoot] && (targets[binding.ToRoot].Slot != binding.ToSlot || targets[binding.ToRoot].Path != binding.To) {
			return fmt.Errorf("state: boundary coordinate footprint root collision")
		}
		targets[binding.ToRoot].Slot, targets[binding.ToRoot].Path = binding.ToSlot, binding.To
		targets[binding.ToRoot].Sources = append(targets[binding.ToRoot].Sources, binding.FromRoot)
		set[binding.ToRoot] = true
		if binding.ToSlot != 0 {
			destination.slots[binding.ToSlot] = struct{}{}
		}
		if binding.To.Kind != keyspace.KindInvalid {
			destination.paths[binding.To] = struct{}{}
		}
	}
	quotient, ok := buildBoundaryInverseQuotient(sourceKeys, d.destinationKeys, selection.closure, effective, rootSlots, d.authority)
	if !ok {
		return fmt.Errorf("state: boundary coordinate footprint inverse relation failed")
	}
	if quotient.identities == nil {
		quotient.identities = make(map[identity.Term][]identity.Term)
	}
	for target, preimages := range d.authority.preimages {
		term := identity.ConcreteTerm(target)
		values := append([]identity.Term(nil), preimages...)
		values = append(values, term)
		quotient.identities[term] = values
	}
	d.sourceKeys, d.selection, d.roots, d.slots = sourceKeys, selection, effective, rootSlots
	d.aliases, d.targets, d.quotient, d.destination = aliases, targets, quotient, destination
	sourceEmpty := &coordinateFactorInventorySet{}
	destinationEmpty := &coordinateFactorInventorySet{}
	d.sourceSeen = CoordinateFactorInventory{seal: d.sourceDomain.seal, keys: sourceKeys, set: sourceEmpty, completed: sourceEmpty}
	d.sourceInputSeen = d.sourceSeen
	d.destinationSeen = CoordinateFactorInventory{seal: d.domain.seal, keys: d.destinationKeys, set: destinationEmpty, completed: destinationEmpty}
	d.destinationInputSeen = d.destinationSeen
	d.image = d.destinationSeen
	d.bound = true
	return d.seedZeroPremises()
}

func (d *boundaryCoordinateFootprintData) takeZeroPremises() []CoordinateSlot {
	out := d.zeroPremises
	d.zeroPremises = nil
	return out
}

func (d *boundaryCoordinateFootprintData) seedZeroPremises() error {
	ctx := &boundaryApplyContext{reg: d.domain.reg, keys: d.destinationKeys, closure: d.destination}
	for _, pair := range d.families {
		for _, entry := range pair.destination.boundary.postEntries(d.aliases) {
			// postEntries is the registered static incidence law. Unlike a runtime
			// scalar wire, it is independent of a family skeleton by contract; its
			// key/scalar pair is nevertheless fully representation-validated here.
			if entry.key == nil || entry.scalar == nil || !pair.destination.ops.keyValid(entry.key, d.destinationKeys) || !pair.destination.ops.scalarValid(entry.key, entry.scalar) {
				return fmt.Errorf("state: invalid coordinate footprint post entry")
			}
			d.zeroPremises = append(d.zeroPremises, CoordinateSlot{family: pair.destination.family, keys: d.destinationKeys, key: entry.key})
		}
		for _, target := range d.targets {
			key, claimed, valid := pair.destination.boundary.rootSlot(ctx, target)
			if !valid {
				return fmt.Errorf("state: coordinate footprint root claim failed in family %q", pair.destination.family.id)
			}
			if claimed {
				d.zeroPremises = append(d.zeroPremises, CoordinateSlot{family: pair.destination.family, keys: d.destinationKeys, key: key})
			}
		}
	}
	return nil
}

func (d *boundaryCoordinateFootprintData) findOrAddTarget(family *coordinateFamilyRuntime, key coordinateKeyPayload, required []coordinateFiberPayload) *boundaryFootprintTarget {
	bucketKey := boundaryFootprintTargetHash{family: family.family, hash: family.ops.keyHash(key, d.destinationKeys)}
	for _, target := range d.targetBuckets[bucketKey] {
		if family.ops.keyEqual(target.slot.key, key) {
			return target
		}
	}
	target := &boundaryFootprintTarget{slot: CoordinateSlot{family: family.family, keys: d.destinationKeys, key: key}, required: required, satisfied: make(map[coordinateFiberPayload]struct{}, len(required))}
	d.targetBuckets[bucketKey] = append(d.targetBuckets[bucketKey], target)
	return target
}

func boundaryFootprintTargetComplete(target *boundaryFootprintTarget) bool {
	for _, required := range target.required {
		if _, ok := target.satisfied[required]; !ok {
			return false
		}
	}
	return len(target.required) != 0
}

func invertBoundaryPathMap(in boundaryPathMap) boundaryPathMap {
	out := make(boundaryPathMap, len(in))
	for index, binding := range in {
		out[index] = boundaryPathBinding{from: binding.to, to: binding.from}
	}
	return out
}

func coordinateInventoryDelta(d ProductDomain, keys *keyspace.KeySpace, prior, next CoordinateFactorInventory) (CoordinateFactorInventory, error) {
	if !prior.ValidFor(d, keys) || !next.ValidFor(d, keys) {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate inventory delta authority", ErrInvalidLaneFactor)
	}
	added, err := coordinateInventorySubtract(d, keys, next, prior)
	if err != nil {
		return CoordinateFactorInventory{}, err
	}
	missing, err := coordinateInventorySubtract(d, keys, prior, next)
	if err != nil || missing.Len() != 0 {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate inventories are not monotone", ErrInvalidLaneFactor)
	}
	return added, nil
}

func coordinateInventorySubtract(d ProductDomain, keys *keyspace.KeySpace, whole, remove CoordinateFactorInventory) (CoordinateFactorInventory, error) {
	if !whole.ValidFor(d, keys) || !remove.ValidFor(d, keys) {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate inventory subtraction authority", ErrInvalidLaneFactor)
	}
	out := make([]CoordinateSlot, 0, whole.Len())
	for _, bucket := range whole.set.families {
		coordinate, err := d.validateCoordinateFamily(bucket.family)
		if err != nil {
			return CoordinateFactorInventory{}, err
		}
		removed, err := remove.familySlots(bucket.family)
		if err != nil {
			return CoordinateFactorInventory{}, err
		}
		for wholeIndex, removeIndex := 0, 0; wholeIndex < len(bucket.slots); wholeIndex++ {
			slot := bucket.slots[wholeIndex]
			for removeIndex < len(removed) && coordinate.ops.keyLess(removed[removeIndex].key, slot.key, keys) {
				removeIndex++
			}
			if removeIndex >= len(removed) || !coordinate.ops.keyEqual(slot.key, removed[removeIndex].key) {
				out = append(out, slot)
			}
		}
	}
	return d.SealCoordinateFactorInventory(keys, out)
}

func boundaryClosureAddedAtoms(prior, next BoundaryClosure) []boundaryAffectedAtom {
	var out []boundaryAffectedAtom
	for value := range next.paths {
		if _, ok := prior.paths[value]; !ok {
			out = append(out, boundaryAffectedAtom{kind: boundaryAffectedAtomPath, path: value})
		}
	}
	for value := range next.slots {
		if _, ok := prior.slots[value]; !ok {
			out = append(out, boundaryAffectedAtom{kind: boundaryAffectedAtomSlot, slot: value})
		}
	}
	for value := range next.identities {
		if _, ok := prior.identities[value]; !ok {
			out = append(out, boundaryAffectedAtom{kind: boundaryAffectedAtomIdentity, identity: value})
		}
	}
	for value := range next.heapSuffixes {
		if _, ok := prior.heapSuffixes[value]; !ok {
			out = append(out, boundaryAffectedAtom{kind: boundaryAffectedAtomHeapSuffix, heapSuffix: value})
		}
	}
	return out
}
