package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type CoordinateFormalRootBinding struct {
	Source           keyspace.Key
	Target           formal.Root
	ResolverVersions bool
}

// CoordinateFormalInverseRootBinding selects the exact concrete structural
// root represented by one formal root at a named publication environment.
// The target may be a resolver-version root; selection is frozen by the body
// publication plan, never inferred from a factor payload.
type CoordinateFormalInverseRootBinding struct {
	Source formal.Root
	Target keyspace.Key
}

type CoordinateFormalRootRekey struct {
	seal          *productDomainSeal
	sourceOwner   lexicalidentity.StableLexicalBodyID
	targetOwner   lexicalidentity.StableLexicalBodyID
	formalSource  bool
	formalTarget  bool
	from          *keyspace.KeySpace
	to            *keyspace.KeySpace
	roots         boundaryPathMap
	rootIndex     map[keyspace.Key]keyspace.Key
	resolverIndex map[keyspace.Key]keyspace.Key
}

// CoordinateFormalPublicationProjection is the sealed existential boundary
// from one formal body coordinate space to one concrete point environment.
// The inverse owns only roots visible at that point; selection removes every
// coordinate whose registered key law cannot be expressed through that
// inverse before any factor is rekeyed.
type CoordinateFormalPublicationProjection struct {
	seal      *productDomainSeal
	inverse   CoordinateFormalRootRekey
	selection BoundaryFactorSelection
}

func (d ProductDomain) SealCoordinateFormalRootRekey(owner lexicalidentity.StableLexicalBodyID, from, to *keyspace.KeySpace, bindings []CoordinateFormalRootBinding) (CoordinateFormalRootRekey, error) {
	if !d.Valid() || owner == (lexicalidentity.StableLexicalBodyID{}) || from == nil || !from.Valid() || to == nil || !to.Valid() {
		return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal coordinate rekey authority", ErrInvalidLaneFactor)
	}
	plan := CoordinateFormalRootRekey{
		seal: d.seal, targetOwner: owner, formalTarget: true, from: from, to: to,
		roots: make(boundaryPathMap, 0, len(bindings)), rootIndex: make(map[keyspace.Key]keyspace.Key, len(bindings)),
		resolverIndex: make(map[keyspace.Key]keyspace.Key, len(bindings)),
	}
	type sourceMode struct {
		root     keyspace.Key
		resolver bool
	}
	sources := make(map[sourceMode]struct{}, len(bindings))
	targets := make(map[formal.Root]keyspace.Key, len(bindings))
	var sourceModeSet bool
	for index, binding := range bindings {
		root, ok := from.StructuralRoot(binding.Source)
		if !ok || !binding.Target.Valid() || binding.Target.Owner() != owner {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal coordinate binding %d", ErrInvalidLaneFactor, index)
		}
		sourceFormal, formalSource := from.DescribeFormalRoot(root)
		if !sourceModeSet {
			plan.formalSource = formalSource
			sourceModeSet = true
		} else if plan.formalSource != formalSource {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: mixed concrete and formal coordinate source namespaces", ErrInvalidLaneFactor)
		}
		if formalSource {
			if plan.sourceOwner == (lexicalidentity.StableLexicalBodyID{}) {
				plan.sourceOwner = sourceFormal.Owner()
			}
			if !sourceFormal.Valid() || sourceFormal.Owner() != plan.sourceOwner {
				return CoordinateFormalRootRekey{}, fmt.Errorf("%w: foreign formal coordinate source owner", ErrInvalidLaneFactor)
			}
		}
		mode := sourceMode{root: root, resolver: binding.ResolverVersions}
		if _, duplicate := sources[mode]; duplicate {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: duplicate formal coordinate source binding", ErrInvalidLaneFactor)
		}
		if prior, collision := targets[binding.Target]; collision && prior != root {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: non-injective formal coordinate target binding", ErrInvalidLaneFactor)
		}
		target, exact := to.InternFormalRoot(binding.Target)
		if !exact {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal coordinate target binding %d", ErrInvalidLaneFactor, index)
		}
		plan.roots = append(plan.roots, boundaryPathBinding{from: root, to: target})
		if binding.ResolverVersions {
			plan.resolverIndex[root] = target
		} else {
			plan.rootIndex[root] = target
		}
		sources[mode] = struct{}{}
		targets[binding.Target] = root
	}
	sort.Slice(plan.roots, func(i, j int) bool { return from.Less(plan.roots[i].from, plan.roots[j].from) })
	return plan, nil
}

func (p CoordinateFormalRootRekey) validFor(d ProductDomain) bool {
	return d.Valid() && p.seal == d.seal &&
		(!p.formalTarget || p.targetOwner != (lexicalidentity.StableLexicalBodyID{})) &&
		(!p.formalSource || p.sourceOwner != (lexicalidentity.StableLexicalBodyID{})) &&
		p.from != nil && p.from.Valid() && p.to != nil && p.to.Valid() && p.roots != nil && p.rootIndex != nil && p.resolverIndex != nil &&
		len(p.rootIndex)+len(p.resolverIndex) == len(p.roots)
}

// OwnsCoordinateFormalRootRekey reports whether plan is a complete sealed
// forward or inverse structural-root transport owned by d.
func (d ProductDomain) OwnsCoordinateFormalRootRekey(plan CoordinateFormalRootRekey) bool {
	return plan.validFor(d)
}

// OwnsCoordinateFormalPublicationProjection reports exact ProductDomain
// ownership of a point-frozen publication boundary.
func (d ProductDomain) OwnsCoordinateFormalPublicationProjection(projection CoordinateFormalPublicationProjection) bool {
	return d.Valid() && projection.seal == d.seal && projection.inverse.validFor(d) && projection.selection.valid() &&
		projection.selection.keys == projection.inverse.from && (!projection.selection.exactCoordinates ||
		projection.selection.coordinates.ValidFor(d, projection.inverse.from))
}

// CoordinateFormalRoots returns the exact formal root vocabulary admitted by
// plan in canonical key order. It exposes identities only, never payload or
// family inventory, so a publication owner can bind a concrete environment
// without reaching into registered axes.
func (d ProductDomain) CoordinateFormalRoots(plan CoordinateFormalRootRekey) ([]formal.Root, error) {
	if !plan.validFor(d) || !plan.formalTarget {
		return nil, fmt.Errorf("%w: formal coordinate root vocabulary", ErrInvalidLaneFactor)
	}
	out := make([]formal.Root, len(plan.roots))
	for index, binding := range plan.roots {
		root, exact := plan.to.DescribeFormalRoot(binding.to)
		if !exact || !root.Valid() || root.Owner() != plan.targetOwner {
			return nil, fmt.Errorf("%w: malformed formal coordinate root vocabulary", ErrInvalidLaneFactor)
		}
		out[index] = root
	}
	return out, nil
}

func (p CoordinateFormalRootRekey) rekey(source keyspace.Key) (keyspace.Key, bool) {
	if p.from == nil || p.to == nil {
		return keyspace.Key{}, false
	}
	if source.Kind == keyspace.KindRootlessSuffix {
		return p.to.ImportKey(p.from, source)
	}
	root, rooted := p.from.StructuralRoot(source)
	target, found := p.rootIndex[root]
	if source.Kind == keyspace.KindResolverSym {
		// Structural lexical storage and point-visible SSA storage are distinct
		// formal vocabularies. Resolver versions select the sealed Middle image;
		// they never fall through to the structural Input image.
		if lexical, present := p.from.LookupResolverKey(source.Sym, 0, nil); present {
			if lexicalRoot, exact := p.from.StructuralRoot(lexical); exact {
				target, found = p.resolverIndex[lexicalRoot]
			}
		}
	}
	if found {
		return p.to.WithStructuralRoot(p.from, source, target)
	}
	if existing, formalSource := p.from.DescribeFormalRoot(root); rooted && p.formalTarget && formalSource && existing.Owner() == p.targetOwner &&
		(!p.formalSource || existing.Owner() == p.sourceOwner) {
		// Same-owner formal transport preserves the existing namespace exactly.
		return p.to.ImportKey(p.from, source)
	}
	return keyspace.Key{}, false
}

// SealCoordinateFormalPublicationInverse derives the exact inverse of a
// forward body schema and binds only its resolver-version roots to one point
// environment. Structural Input/Output roots use the source root already
// sealed into the forward law; callers must not rediscover or respell them.
func (d ProductDomain) SealCoordinateFormalPublicationInverse(plan CoordinateFormalRootRekey, bindings []CoordinateFormalInverseRootBinding) (CoordinateFormalRootRekey, error) {
	return d.sealCoordinateFormalPublicationInverse(plan, bindings, true)
}

func (d ProductDomain) sealCoordinateFormalPublicationInverse(plan CoordinateFormalRootRekey, bindings []CoordinateFormalInverseRootBinding, complete bool) (CoordinateFormalRootRekey, error) {
	if !plan.validFor(d) || !plan.formalTarget || plan.to == nil || plan.from == nil {
		return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal publication inverse authority", ErrInvalidLaneFactor)
	}
	selected := make(map[formal.Root]keyspace.Key, len(bindings))
	resolverTargets := make(map[formal.Root]struct{}, len(plan.resolverIndex))
	for source, target := range plan.resolverIndex {
		root, exact := plan.to.DescribeFormalRoot(target)
		if !exact || !root.Valid() || root.Owner() != plan.targetOwner || source.Kind == keyspace.KindInvalid {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: malformed resolver publication schema", ErrInvalidLaneFactor)
		}
		resolverTargets[root] = struct{}{}
	}
	for index, binding := range bindings {
		if !binding.Source.Valid() || binding.Source.Owner() != plan.targetOwner {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal publication source %d", ErrInvalidLaneFactor, index)
		}
		if _, resolver := resolverTargets[binding.Source]; !resolver {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal publication source %d is not resolver-backed", ErrInvalidLaneFactor, index)
		}
		root, exact := plan.from.StructuralRoot(binding.Target)
		if !exact || root != binding.Target {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal publication target %d", ErrInvalidLaneFactor, index)
		}
		if _, duplicate := selected[binding.Source]; duplicate {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: duplicate formal publication source", ErrInvalidLaneFactor)
		}
		selected[binding.Source] = root
	}
	inverse := CoordinateFormalRootRekey{
		seal: d.seal, sourceOwner: plan.targetOwner, formalSource: true,
		from: plan.to, to: plan.from, roots: make(boundaryPathMap, 0, len(plan.roots)),
		rootIndex: make(map[keyspace.Key]keyspace.Key, len(plan.roots)), resolverIndex: make(map[keyspace.Key]keyspace.Key),
	}
	targets := make(map[keyspace.Key]formal.Root, len(plan.roots))
	for _, binding := range plan.roots {
		root, exact := plan.to.DescribeFormalRoot(binding.to)
		if !exact || !root.Valid() || root.Owner() != plan.targetOwner {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: malformed formal publication schema", ErrInvalidLaneFactor)
		}
		target, present := selected[root]
		if !present {
			if structuralTarget, structural := plan.rootIndex[binding.from]; structural && structuralTarget == binding.to {
				target = binding.from
				present = true
			}
		}
		if !present && complete {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: incomplete resolver publication environment", ErrInvalidLaneFactor)
		}
		if !present {
			continue
		}
		if prior, collision := targets[target]; collision && prior != root {
			return CoordinateFormalRootRekey{}, fmt.Errorf("%w: non-injective formal publication target", ErrInvalidLaneFactor)
		}
		targets[target] = root
		inverse.roots = append(inverse.roots, boundaryPathBinding{from: binding.to, to: target})
		inverse.rootIndex[binding.to] = target
		delete(selected, root)
	}
	if len(selected) != 0 {
		return CoordinateFormalRootRekey{}, fmt.Errorf("%w: formal publication environment has foreign roots", ErrInvalidLaneFactor)
	}
	sort.Slice(inverse.roots, func(i, j int) bool { return inverse.from.Less(inverse.roots[i].from, inverse.roots[j].from) })
	if !inverse.validFor(d) {
		return CoordinateFormalRootRekey{}, fmt.Errorf("%w: incomplete formal publication inverse", ErrInvalidLaneFactor)
	}
	return inverse, nil
}

// SealCoordinateFormalPublicationProjection derives the point-owned inverse
// and its exact coordinate projection once, before execution. Missing
// resolver roots are existentially hidden: registered family laws decide
// which coordinate slots remain expressible, and the ordinary publication
// path never performs per-slot fallback or error suppression.
func (d ProductDomain) SealCoordinateFormalPublicationProjection(
	plan CoordinateFormalRootRekey,
	inventory CoordinateFactorInventory,
	bindings []CoordinateFormalInverseRootBinding,
) (CoordinateFormalPublicationProjection, error) {
	if !inventory.ValidFor(d, plan.to) {
		return CoordinateFormalPublicationProjection{}, fmt.Errorf("%w: formal publication coordinate inventory", ErrInvalidLaneFactor)
	}
	inverse, err := d.sealCoordinateFormalPublicationInverse(plan, bindings, false)
	if err != nil {
		return CoordinateFormalPublicationProjection{}, err
	}
	selected := make([]CoordinateSlot, 0, inventory.Len())
	for _, slot := range inventory.Slots() {
		coordinate, coordinateErr := d.validateCoordinateFamily(slot.family)
		if coordinateErr != nil || d.validateCoordinateSlotFor(coordinate, slot, plan.to) != nil {
			return CoordinateFormalPublicationProjection{}, fmt.Errorf("%w: malformed formal publication coordinate", ErrInvalidLaneFactor)
		}
		key, ok := coordinate.ops.formalRekey.key(slot.key, inverse)
		if ok && key != nil && coordinate.ops.keyValid(key, inverse.to) {
			selected = append(selected, slot)
		}
	}
	// A lexical publication materializes the complete stabilized State, not a
	// call-boundary reachability slice. Identity coordinates therefore remain
	// universally selected: concrete entry identities pass through unchanged,
	// while allocation templates must survive until the subsequent allocation
	// quotient gives them their invocation identity. Structural coordinates
	// remain restricted to the point-expressible inverse selected above.
	selection, err := SealBoundaryFactorSelection(inverse.from, nil, nil, true)
	if err != nil {
		return CoordinateFormalPublicationProjection{}, err
	}
	selection, err = d.selectBoundaryFactorCoordinates(selection, selected)
	if err != nil {
		return CoordinateFormalPublicationProjection{}, err
	}
	exact, err := d.SealCoordinateFactorInventory(inverse.from, selected)
	if err != nil {
		return CoordinateFormalPublicationProjection{}, err
	}
	selection.coordinates, selection.exactCoordinates = exact, true
	return CoordinateFormalPublicationProjection{seal: d.seal, inverse: inverse, selection: selection}, nil
}

// RekeyOrdinaryLaneFactorFormalPublication transports an ordinary lane
// through the point-owned inverse. Ordinary lanes have no independently
// addressable coordinate inventory; their registered structural rekey law
// remains the sole authority.
func (d ProductDomain) RekeyOrdinaryLaneFactorFormalPublication(projection CoordinateFormalPublicationProjection, source LaneFactor) (LaneFactor, error) {
	if !d.OwnsCoordinateFormalPublicationProjection(projection) {
		return LaneFactor{}, fmt.Errorf("%w: ordinary formal publication projection", ErrInvalidLaneFactor)
	}
	return d.RekeyOrdinaryLaneFactorFormal(projection.inverse, source)
}

// RekeyCoordinateLaneFactorFormalPublication first performs the registered
// existential boundary projection and then the registered formal rekey. Thus
// invocation-local coordinates never escape a body, while all internal
// fibers remain available to the relation fixpoint and point publications.
func (d ProductDomain) RekeyCoordinateLaneFactorFormalPublication(projection CoordinateFormalPublicationProjection, source LaneFactor) (LaneFactor, error) {
	if !d.OwnsCoordinateFormalPublicationProjection(projection) {
		return LaneFactor{}, fmt.Errorf("%w: coordinate formal publication projection", ErrInvalidLaneFactor)
	}
	projected, err := d.ProjectBoundaryFactor(projection.selection, source)
	if err != nil {
		return LaneFactor{}, err
	}
	return d.RekeyCoordinateLaneFactorFormal(projection.inverse, projected)
}

// RekeyStructuralKeyFormal applies the same sealed structural-root
// substitution used by every registered coordinate family to one exact key.
// This is the scalar edge of the coordinate rekey authority: callers may use
// it to address a factor-native transaction in the destination keyspace
// without restating root binding or manufacturing a path spelling.
func (d ProductDomain) RekeyStructuralKeyFormal(plan CoordinateFormalRootRekey, source keyspace.Key) (keyspace.Key, error) {
	if !plan.validFor(d) || source.Kind == keyspace.KindInvalid || plan.from.FormatReadOnly(source) == "" {
		return keyspace.Key{}, fmt.Errorf("%w: formal structural key rekey plan", ErrInvalidLaneFactor)
	}
	target, ok := plan.rekey(source)
	if !ok || target.Kind == keyspace.KindInvalid || plan.to.FormatReadOnly(target) == "" {
		return keyspace.Key{}, fmt.Errorf("%w: formal structural key rekey", ErrInvalidLaneFactor)
	}
	return target, nil
}

// RekeyOrdinaryLaneFactorFormal transports one complete non-coordinate lane
// through its mandatory catalog declaration. Structural lanes reuse their one
// registered boundary image law with the exact formal-root relation; truly
// key-independent lanes retain the original immutable factor.
func (d ProductDomain) RekeyOrdinaryLaneFactorFormal(plan CoordinateFormalRootRekey, source LaneFactor) (LaneFactor, error) {
	if !plan.validFor(d) {
		return LaneFactor{}, fmt.Errorf("%w: ordinary formal rekey plan", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateFactor(source)
	if err != nil || runtime.lane.slotFactored || len(runtime.coordinates) != 0 {
		return LaneFactor{}, fmt.Errorf("%w: formal rekey requires an ordinary lane", ErrInvalidLaneFactor)
	}
	switch runtime.formalRekey.kind {
	case laneFormalRekeyIndependent:
		return source, nil
	case laneFormalRekeyStructural:
		roots := append(boundaryPathMap(nil), plan.roots...)
		inverse := make(boundaryPathMap, len(plan.roots))
		for index, binding := range plan.roots {
			inverse[index] = boundaryPathBinding{from: binding.to, to: binding.from}
		}
		sort.Slice(roots, func(i, j int) bool { return plan.from.Less(roots[i].from, roots[j].from) })
		sort.Slice(inverse, func(i, j int) bool { return plan.to.Less(inverse[i].from, inverse[j].from) })
		ctx := boundaryRebaseContext{
			reg: d.reg, fromKeys: plan.from, toKeys: plan.to, roots: roots,
			formalRekey: &plan,
			quotient: boundaryInverseQuotient{
				paths: make(map[keyspace.Key][]keyspace.Key), stateKeys: make(map[pathaddr.StateKey][]pathaddr.StateKey),
				identities: make(map[identity.Term][]identity.Term), identityStructural: true,
				inverseFrom: plan.to, inverseTo: plan.from, inverseRoots: inverse,
			},
		}
		payload, ok := runtime.ops.boundaryRebase(&ctx, source.payload)
		if !ok {
			return LaneFactor{}, fmt.Errorf("%w: ordinary lane %q formal rekey", ErrInvalidLaneFactor, runtime.lane.id)
		}
		return LaneFactor{lane: runtime.lane, payload: payload}, nil
	default:
		return LaneFactor{}, fmt.Errorf("%w: ordinary lane %q has no formal rekey law", ErrInvalidLaneFactor, runtime.lane.id)
	}
}

// RekeyCoordinateLaneFactorFormal transports one complete coordinate-factored
// lane through the registered family inventory. Every family contributes its
// skeleton and scalar rekey law; adding a family therefore extends this
// operation without a lane/family switch or State reconstruction.
func (d ProductDomain) RekeyCoordinateLaneFactorFormal(plan CoordinateFormalRootRekey, source LaneFactor) (LaneFactor, error) {
	if !plan.validFor(d) {
		return LaneFactor{}, fmt.Errorf("%w: formal coordinate lane rekey plan", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateFactor(source)
	if err != nil || len(runtime.coordinates) == 0 {
		return LaneFactor{}, fmt.Errorf("%w: formal coordinate lane factor", ErrInvalidLaneFactor)
	}
	skeletons := make([]CoordinateFamilySkeleton, len(runtime.coordinates))
	factors := make([][]CoordinateScalarFactor, len(runtime.coordinates))
	for familyIndex, coordinate := range runtime.coordinates {
		skeleton, scalars, decomposeErr := d.DecomposeCoordinateFamily(source, coordinate.family, plan.from)
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		skeletons[familyIndex], err = d.RekeyCoordinateSkeletonFormal(plan, skeleton)
		if err != nil {
			return LaneFactor{}, err
		}
		factors[familyIndex] = make([]CoordinateScalarFactor, len(scalars))
		for scalarIndex, scalar := range scalars {
			factors[familyIndex][scalarIndex], err = d.RekeyCoordinateScalarFormal(plan, scalar)
			if err != nil {
				return LaneFactor{}, err
			}
		}
		// An injective root substitution may permute canonical root order. Sort
		// with the registered destination-family order before exact composition.
		sort.Slice(factors[familyIndex], func(left, right int) bool {
			return coordinate.ops.keyLess(factors[familyIndex][left].slot.key, factors[familyIndex][right].slot.key, plan.to)
		})
	}
	return d.ComposeCoordinateFamilies(runtime.lane, plan.to, skeletons, factors)
}

func (d ProductDomain) RekeyCoordinateSlotFormal(plan CoordinateFormalRootRekey, source CoordinateSlot) (CoordinateSlot, error) {
	if !plan.validFor(d) || source.keys != plan.from {
		return CoordinateSlot{}, fmt.Errorf("%w: formal coordinate slot rekey plan", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(source.family)
	if err != nil {
		return CoordinateSlot{}, err
	}
	key, ok := coordinate.ops.formalRekey.key(source.key, plan)
	if !ok || key == nil || !coordinate.ops.keyValid(key, plan.to) {
		return CoordinateSlot{}, fmt.Errorf("%w: formal coordinate key rekey", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: coordinate.family, keys: plan.to, key: key}, nil
}

func (d ProductDomain) RekeyCoordinateSkeletonFormal(plan CoordinateFormalRootRekey, source CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error) {
	if !plan.validFor(d) || source.keys != plan.from {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: formal coordinate skeleton rekey plan", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(source.family)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	payload, ok := coordinate.ops.formalRekey.skeleton(source.payload, plan)
	if !ok || payload == nil {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: formal coordinate skeleton rekey", ErrInvalidLaneFactor)
	}
	return CoordinateFamilySkeleton{family: coordinate.family, keys: plan.to, payload: payload}, nil
}

func (d ProductDomain) RekeyCoordinateScalarFormal(plan CoordinateFormalRootRekey, source CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	slot, err := d.RekeyCoordinateSlotFormal(plan, source.slot)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	coordinate, _ := d.validateCoordinateFamily(slot.family)
	payload, ok := coordinate.ops.importScalar(source.payload)
	if !ok || payload == nil || !coordinate.ops.scalarValid(slot.key, payload) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: formal coordinate scalar rekey", ErrInvalidLaneFactor)
	}
	return CoordinateScalarFactor{slot: slot, payload: payload}, nil
}

type coordinateFormalRekeyKind uint8

const (
	coordinateFormalRekeyInvalid coordinateFormalRekeyKind = iota
	coordinateFormalRekeyKeyIndependent
	coordinateFormalRekeyStructural
)

type coordinateFormalRekeyPolicy struct {
	kind     coordinateFormalRekeyKind
	skeleton func(coordinateSkeletonPayload, CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool)
	key      func(coordinateKeyPayload, CoordinateFormalRootRekey) (coordinateKeyPayload, bool)
}

func coordinateFormalRekeyPolicyComplete(policy coordinateFormalRekeyPolicy) bool {
	return (policy.kind == coordinateFormalRekeyKeyIndependent || policy.kind == coordinateFormalRekeyStructural) && policy.skeleton != nil && policy.key != nil
}

func keyIndependentCoordinateFormalRekey() coordinateFormalRekeyPolicy {
	return coordinateFormalRekeyPolicy{
		kind: coordinateFormalRekeyKeyIndependent,
		skeleton: func(source coordinateSkeletonPayload, _ CoordinateFormalRootRekey) (coordinateSkeletonPayload, bool) {
			return source, source != nil
		},
		key: func(source coordinateKeyPayload, _ CoordinateFormalRootRekey) (coordinateKeyPayload, bool) {
			return source, source != nil
		},
	}
}
