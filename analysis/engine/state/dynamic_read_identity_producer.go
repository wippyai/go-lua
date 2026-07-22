package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// DynamicReadIdentityProducer is one opaque, statically declared source whose
// embedded value can be selected by the canonical dynamic-read algebra.  It
// names producer topology, not an identity value: in particular, a heap
// object's own identity is never confused with the identity stored in one of
// its members.
//
// The descriptor is comparable so a symbolic producer fixed point can use it
// as a map key.  Its fields remain private; construction and selection stay
// owned by ProductDomain.
type DynamicReadIdentityProducer struct {
	seal *productDomainSeal
	keys *keyspace.KeySpace
	kind dynamicReadIdentityProducerKind

	path keyspace.Key
	term identity.Term
	fact dynamicindex.Key
	key  DynamicReadIdentityKeyClass
}

type dynamicReadIdentityProducerKind uint8

const (
	dynamicReadIdentityProducerInvalid dynamicReadIdentityProducerKind = iota
	dynamicReadIdentityProducerPathMember
	dynamicReadIdentityProducerPathDynamic
	dynamicReadIdentityProducerHeapMember
	dynamicReadIdentityProducerHeapAnyMember
	dynamicReadIdentityProducerHeapDynamic
)

// ValidFor reports exact ProductDomain and keyspace ownership.
func (p DynamicReadIdentityProducer) ValidFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	if !d.Valid() || p.seal != d.seal || p.keys == nil || p.keys != keys || !keys.Valid() {
		return false
	}
	switch p.kind {
	case dynamicReadIdentityProducerPathMember:
		return p.path.Kind != keyspace.KindInvalid && keys.FormatReadOnly(p.path) != "" && !p.term.Valid()
	case dynamicReadIdentityProducerPathDynamic:
		return p.path.Kind == keyspace.KindInvalid && !p.term.Valid() && dynamicReadIdentityFactDescriptorValid(p, keys)
	case dynamicReadIdentityProducerHeapMember:
		return p.path.Kind != keyspace.KindInvalid && keys.FormatReadOnly(p.path) != "" && p.term.Valid()
	case dynamicReadIdentityProducerHeapAnyMember:
		return p.path.Kind == keyspace.KindInvalid && p.term.Valid()
	case dynamicReadIdentityProducerHeapDynamic:
		return p.path.Kind == keyspace.KindInvalid && p.term.Valid() && dynamicReadIdentityFactDescriptorValid(p, keys)
	default:
		return false
	}
}

func dynamicReadIdentityFactDescriptorValid(p DynamicReadIdentityProducer, keys *keyspace.KeySpace) bool {
	return p.fact.Table.Kind != keyspace.KindInvalid && keys.FormatReadOnly(p.fact.Table) != "" && p.fact.Site != ""
}

// DynamicReadIdentityKeyClass is the frozen topology class of a dynamic write
// key. Exact constants retain their canonical path segment; every other key is
// one wildcard atom. It carries no evolving abstract value.
type DynamicReadIdentityKeyClass struct {
	segment segment.Segment
	exact   bool
}

func DynamicReadIdentityWildcardKeyClass() DynamicReadIdentityKeyClass {
	return DynamicReadIdentityKeyClass{}
}

func DynamicReadIdentityExactKeyClass(key segment.Segment) DynamicReadIdentityKeyClass {
	return DynamicReadIdentityKeyClass{segment: key, exact: true}
}

// PrepareDynamicReadIdentityKeyClass classifies a frozen write-key value with
// the same scalar-key normalizer used by the executable dynamic-read binder.
func (d ProductDomain) PrepareDynamicReadIdentityKeyClass(typeValues *typevalue.Cache, key product.Value) (DynamicReadIdentityKeyClass, error) {
	if !d.Valid() || !product.BelongsToRegistry(d.reg, key) {
		return DynamicReadIdentityKeyClass{}, fmt.Errorf("%w: dynamic-read identity key class", ErrInvalidLaneFactor)
	}
	if exact, ok := typevalue.ExactScalarKeySegment(d.reg, typeValues, key); ok {
		return DynamicReadIdentityExactKeyClass(exact), nil
	}
	return DynamicReadIdentityWildcardKeyClass(), nil
}

// DynamicReadIdentityPublication pairs one producer descriptor with the
// caller-owned ordinal of the ValueTerm which supplies its finite identity
// support.  State owns producer normalization; the transformer owns the
// meaning of Source.
type DynamicReadIdentityPublication struct {
	Producer DynamicReadIdentityProducer
	Source   int
}

// StaticMemberIdentityPublications declares the primary and field-canonical
// path producers written by one already-normalized static member plan. Source
// 0 denotes the value bound later through BindStaticMemberFactorValue.
func (d ProductDomain) StaticMemberIdentityPublications(plan StaticMemberFactorPlan) ([]DynamicReadIdentityPublication, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: invalid static-member identity producer", ErrInvalidLaneFactor)
	}
	out := make([]DynamicReadIdentityPublication, len(plan.targets))
	for index, target := range plan.targets {
		out[index] = DynamicReadIdentityPublication{Producer: DynamicReadIdentityProducer{
			seal: d.seal, keys: plan.keys, kind: dynamicReadIdentityProducerPathMember, path: target,
		}, Source: 0}
	}
	return out, nil
}

// ObjectConstructorIdentitySourceRefs is the detached canonical source
// vocabulary used by ObjectConstructorIdentityPublications. Each publication's
// Source indexes this slice; aliases and repeated writes share one source ref.
func (d ProductDomain) ObjectConstructorIdentitySourceRefs(plan ObjectConstructorPlan) ([]ObjectConstructorValueRef, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: invalid object-constructor identity source vocabulary", ErrInvalidLaneFactor)
	}
	seen := make(map[ObjectConstructorValueRef]struct{})
	refs := make([]ObjectConstructorValueRef, 0)
	for _, object := range plan.objects {
		for _, member := range object.members {
			if _, present := seen[member.source]; present {
				continue
			}
			seen[member.source] = struct{}{}
			refs = append(refs, member.source)
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].object != refs[j].object {
			return refs[i].object < refs[j].object
		}
		return refs[i].member < refs[j].member
	})
	return refs, nil
}

// ObjectConstructorIdentityPublications declares normalized heap-member
// producers without accepting abstract values. Source indexes the canonical
// ObjectConstructorIdentitySourceRefs vocabulary.
func (d ProductDomain) ObjectConstructorIdentityPublications(plan ObjectConstructorPlan) ([]DynamicReadIdentityPublication, error) {
	refs, err := d.ObjectConstructorIdentitySourceRefs(plan)
	if err != nil {
		return nil, err
	}
	ordinals := make(map[ObjectConstructorValueRef]int, len(refs))
	for index, ref := range refs {
		ordinals[ref] = index
	}
	out := make([]DynamicReadIdentityPublication, 0)
	for _, object := range plan.objects {
		for _, member := range object.members {
			source, present := ordinals[member.source]
			if !present {
				return nil, fmt.Errorf("%w: missing constructor identity source", ErrInvalidLaneFactor)
			}
			out = append(out,
				DynamicReadIdentityPublication{Producer: DynamicReadIdentityProducer{
					seal: d.seal, keys: plan.keys, kind: dynamicReadIdentityProducerHeapMember, term: object.id, path: member.key,
				}, Source: source},
				DynamicReadIdentityPublication{Producer: DynamicReadIdentityProducer{
					seal: d.seal, keys: plan.keys, kind: dynamicReadIdentityProducerHeapAnyMember, term: object.id,
				}, Source: source},
			)
		}
	}
	return canonicalDynamicReadIdentityPublications(d, plan.keys, out)
}

// DynamicIndexDynamicReadIdentityProducerDeclarations freezes the stable
// origin topology of dynamic writes. The atom may overdeclare a dependency,
// but it never carries or changes DynamicIndex/heap scalar semantics.
func (d ProductDomain) DynamicIndexDynamicReadIdentityProducerDeclarations(keys *keyspace.KeySpace, factKeys []dynamicindex.Key, keyClass DynamicReadIdentityKeyClass, ownerTerms []identity.Term) ([]DynamicReadIdentityProducer, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return nil, fmt.Errorf("%w: dynamic-index identity declarations", ErrInvalidLaneFactor)
	}
	for _, term := range ownerTerms {
		if !term.Valid() {
			return nil, fmt.Errorf("%w: dynamic-index identity owner declaration", ErrInvalidLaneFactor)
		}
	}
	ownerTerms = canonicalDynamicReadIdentityTerms(append([]identity.Term(nil), ownerTerms...))
	out := make([]DynamicReadIdentityProducer, 0, len(factKeys)*(len(ownerTerms)+1))
	for _, factKey := range factKeys {
		if factKey.Table.Kind == keyspace.KindInvalid || keys.FormatReadOnly(factKey.Table) == "" || factKey.Site == "" {
			return nil, fmt.Errorf("%w: foreign dynamic-index table producer", ErrInvalidLaneFactor)
		}
		out = append(out, DynamicReadIdentityProducer{
			seal: d.seal, keys: keys, kind: dynamicReadIdentityProducerPathDynamic, fact: factKey, key: keyClass,
		})
		for _, owner := range ownerTerms {
			out = append(out, DynamicReadIdentityProducer{
				seal: d.seal, keys: keys, kind: dynamicReadIdentityProducerHeapDynamic, fact: factKey, key: keyClass, term: owner,
			})
		}
	}
	return canonicalDynamicReadIdentityProducers(keys, out), nil
}

// DynamicReadIdentityProducerOwnerDeclarations expands one stable structural
// dynamic-write origin over newly discovered finite symbolic owner terms. It
// changes topology only; stored values remain owned by the existing support
// equation for origin.
func (d ProductDomain) DynamicReadIdentityProducerOwnerDeclarations(origin DynamicReadIdentityProducer, ownerTerms []identity.Term) ([]DynamicReadIdentityProducer, error) {
	if origin.keys == nil || !origin.ValidFor(d, origin.keys) || origin.kind != dynamicReadIdentityProducerPathDynamic {
		return nil, fmt.Errorf("%w: dynamic-read identity owner origin", ErrInvalidLaneFactor)
	}
	for _, term := range ownerTerms {
		if !term.Valid() {
			return nil, fmt.Errorf("%w: dynamic-read identity owner term", ErrInvalidLaneFactor)
		}
	}
	ownerTerms = canonicalDynamicReadIdentityTerms(append([]identity.Term(nil), ownerTerms...))
	out := make([]DynamicReadIdentityProducer, len(ownerTerms))
	for index, term := range ownerTerms {
		out[index] = DynamicReadIdentityProducer{
			seal: d.seal, keys: origin.keys, kind: dynamicReadIdentityProducerHeapDynamic,
			fact: origin.fact, key: origin.key, term: term,
		}
	}
	return canonicalDynamicReadIdentityProducers(origin.keys, out), nil
}

type dynamicReadHeapMemberProducerKey struct {
	term identity.Term
	path keyspace.Key
}

// DynamicReadIdentityProducerIndex is the sealed finite producer topology.
// It is built once from declarations and offers only direct keyed projection;
// a read never scans the coordinate inventory or producer universe.
// The catalog may survive scalar invalidation because it records possible
// equation edges, not live DynamicIndex or heap facts. It has no authority to
// change values, admission, presence, membership, or diagnostics.
type DynamicReadIdentityProducerIndex struct {
	seal        *productDomainSeal
	keys        *keyspace.KeySpace
	pathMembers map[keyspace.Key][]DynamicReadIdentityProducer
	pathDynamic map[keyspace.Key][]DynamicReadIdentityProducer
	heapMembers map[dynamicReadHeapMemberProducerKey][]DynamicReadIdentityProducer
	heapAny     map[identity.Term][]DynamicReadIdentityProducer
	heapDynamic map[identity.Term][]DynamicReadIdentityProducer
}

func (d ProductDomain) SealDynamicReadIdentityProducerIndex(keys *keyspace.KeySpace, producers []DynamicReadIdentityProducer) (DynamicReadIdentityProducerIndex, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return DynamicReadIdentityProducerIndex{}, fmt.Errorf("%w: dynamic-read identity producer index", ErrInvalidLaneFactor)
	}
	out := DynamicReadIdentityProducerIndex{seal: d.seal, keys: keys,
		pathMembers: make(map[keyspace.Key][]DynamicReadIdentityProducer), pathDynamic: make(map[keyspace.Key][]DynamicReadIdentityProducer),
		heapMembers: make(map[dynamicReadHeapMemberProducerKey][]DynamicReadIdentityProducer), heapAny: make(map[identity.Term][]DynamicReadIdentityProducer),
		heapDynamic: make(map[identity.Term][]DynamicReadIdentityProducer)}
	for index, producer := range producers {
		if !producer.ValidFor(d, keys) {
			return DynamicReadIdentityProducerIndex{}, fmt.Errorf("%w: dynamic-read identity producer index item %d", ErrInvalidLaneFactor, index)
		}
		switch producer.kind {
		case dynamicReadIdentityProducerPathMember:
			out.pathMembers[producer.path] = append(out.pathMembers[producer.path], producer)
		case dynamicReadIdentityProducerPathDynamic:
			out.pathDynamic[producer.fact.Table] = append(out.pathDynamic[producer.fact.Table], producer)
		case dynamicReadIdentityProducerHeapMember:
			key := dynamicReadHeapMemberProducerKey{term: producer.term, path: producer.path}
			out.heapMembers[key] = append(out.heapMembers[key], producer)
		case dynamicReadIdentityProducerHeapAnyMember:
			out.heapAny[producer.term] = append(out.heapAny[producer.term], producer)
		case dynamicReadIdentityProducerHeapDynamic:
			out.heapDynamic[producer.term] = append(out.heapDynamic[producer.term], producer)
		}
	}
	return out, nil
}

func (i DynamicReadIdentityProducerIndex) ValidFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	return d.Valid() && i.seal == d.seal && i.keys == keys && keys != nil && keys.Valid() && i.pathMembers != nil &&
		i.pathDynamic != nil && i.heapMembers != nil && i.heapAny != nil && i.heapDynamic != nil
}

// PlanDynamicReadIdentityTopologyProducers projects dependency topology only.
// Impossible membership can discard exact mismatches. Conditional membership
// retains every stable atom for the selected table/owner; the existing
// row-level factor capability and identity substitution remain the sole
// authority for executable values and diagnostics.
func (d ProductDomain) PlanDynamicReadIdentityTopologyProducers(selection DynamicReadSelection, tableTerms []identity.Term, inventory CoordinateFactorInventory, index *DynamicReadIdentityProducerIndex) ([]DynamicReadIdentityProducer, error) {
	if !selection.validFor(d) || !inventory.ValidFor(d, selection.keys) || index == nil || !index.ValidFor(d, selection.keys) {
		return nil, fmt.Errorf("%w: invalid dynamic-read identity demand", ErrInvalidLaneFactor)
	}
	out := make([]DynamicReadIdentityProducer, 0)
	for _, path := range selection.pathMembers {
		out = append(out, index.pathMembers[path]...)
	}
	for table := range selection.tables {
		for _, producer := range index.pathDynamic[table] {
			if selection.membership == DynamicReadMembershipConditional || dynamicReadIdentityKeyClassMatches(selection, producer.key) {
				out = append(out, producer)
			}
		}
	}
	seenTerms := make(map[identity.Term]struct{}, len(tableTerms))
	for _, term := range tableTerms {
		if !term.Valid() {
			return nil, fmt.Errorf("%w: invalid dynamic-read table identity term", ErrInvalidLaneFactor)
		}
		if _, duplicate := seenTerms[term]; duplicate {
			continue
		}
		seenTerms[term] = struct{}{}
		if selection.exactKey {
			for _, path := range selection.heapMembers {
				out = append(out, index.heapMembers[dynamicReadHeapMemberProducerKey{term: term, path: path}]...)
			}
		} else {
			out = append(out, index.heapAny[term]...)
		}
		for _, producer := range index.heapDynamic[term] {
			if selection.membership == DynamicReadMembershipConditional || dynamicReadIdentityKeyClassMatches(selection, producer.key) {
				out = append(out, producer)
			}
		}
	}
	return canonicalDynamicReadIdentityProducers(selection.keys, out), nil
}

func dynamicReadIdentityKeyClassMatches(selection DynamicReadSelection, class DynamicReadIdentityKeyClass) bool {
	return !class.exact || selection.exactKey && class.segment == selection.keySegment
}

// RekeyDynamicReadIdentityProducerFormal transports a producer's structural
// address through the same sealed root morphism used by its factor carrier.
// Heap owner terms are deliberately unchanged here; their set-valued Apply
// image is a separate registered operation below.
func (d ProductDomain) RekeyDynamicReadIdentityProducerFormal(plan CoordinateFormalRootRekey, producer DynamicReadIdentityProducer) (DynamicReadIdentityProducer, error) {
	out, mapped, err := d.TryRekeyDynamicReadIdentityProducerFormal(plan, producer)
	if err != nil {
		return DynamicReadIdentityProducer{}, err
	}
	if !mapped {
		return DynamicReadIdentityProducer{}, fmt.Errorf("%w: dynamic-read producer formal rekey", ErrInvalidLaneFactor)
	}
	return out, nil
}

// TryRekeyDynamicReadIdentityProducerFormal distinguishes a valid producer
// whose structural root is not selected by plan from malformed/foreign input.
func (d ProductDomain) TryRekeyDynamicReadIdentityProducerFormal(plan CoordinateFormalRootRekey, producer DynamicReadIdentityProducer) (DynamicReadIdentityProducer, bool, error) {
	if !plan.validFor(d) || producer.keys == nil || !producer.ValidFor(d, producer.keys) {
		return DynamicReadIdentityProducer{}, false, fmt.Errorf("%w: dynamic-read producer formal rekey", ErrInvalidLaneFactor)
	}
	if producer.keys != plan.from {
		return DynamicReadIdentityProducer{}, false, nil
	}
	out := producer
	out.keys = plan.to
	rekey := func(source keyspace.Key) (keyspace.Key, bool) {
		target, ok := plan.rekey(source)
		return target, ok && target.Kind != keyspace.KindInvalid && plan.to.FormatReadOnly(target) != ""
	}
	switch producer.kind {
	case dynamicReadIdentityProducerPathMember, dynamicReadIdentityProducerHeapMember:
		var mapped bool
		out.path, mapped = rekey(producer.path)
		if !mapped {
			return DynamicReadIdentityProducer{}, false, nil
		}
	case dynamicReadIdentityProducerPathDynamic, dynamicReadIdentityProducerHeapDynamic:
		var mapped bool
		out.fact.Table, mapped = rekey(producer.fact.Table)
		if !mapped {
			return DynamicReadIdentityProducer{}, false, nil
		}
	case dynamicReadIdentityProducerHeapAnyMember:
	default:
		return DynamicReadIdentityProducer{}, false, fmt.Errorf("%w: invalid dynamic-read producer kind", ErrInvalidLaneFactor)
	}
	if !out.ValidFor(d, plan.to) {
		return DynamicReadIdentityProducer{}, false, fmt.Errorf("%w: invalid rekeyed dynamic-read producer", ErrInvalidLaneFactor)
	}
	return out, true, nil
}

// ImageDynamicReadIdentityProducer applies the exact finite identity image of
// one Apply to an already-rekeyed producer. Concrete terms map to themselves;
// a formal term maps to its declared finite alternatives. An empty image is an
// empty producer set, never a request to enumerate all identities.
func (d ProductDomain) ImageDynamicReadIdentityProducer(producer DynamicReadIdentityProducer, image *CoordinateIdentityTermImage) ([]DynamicReadIdentityProducer, error) {
	if !producer.ValidFor(d, producer.keys) || image == nil {
		return nil, fmt.Errorf("%w: dynamic-read producer identity image", ErrInvalidLaneFactor)
	}
	if !producer.term.Valid() {
		return []DynamicReadIdentityProducer{producer}, nil
	}
	terms, exact := image.Image(producer.term)
	if !exact {
		return nil, fmt.Errorf("%w: incomplete dynamic-read producer identity image", ErrInvalidLaneFactor)
	}
	out := make([]DynamicReadIdentityProducer, len(terms))
	for index, term := range terms {
		out[index] = producer
		out[index].term = term
		if !out[index].ValidFor(d, producer.keys) {
			return nil, fmt.Errorf("%w: invalid dynamic-read producer identity alternative", ErrInvalidLaneFactor)
		}
	}
	return canonicalDynamicReadIdentityProducers(producer.keys, out), nil
}

// TransportDynamicReadIdentityProducersFormal applies an exact set of sealed
// structural wires and then the finite owner-identity image. A rooted
// producer is emitted only through the wire which owns and maps its root;
// rootless heap suffixes retain their spelling, and dynamic fact Site is never
// rewritten. Duplicate aliases/images are canonicalized once at the end.
func (d ProductDomain) TransportDynamicReadIdentityProducersFormal(
	destinationDomain ProductDomain,
	wires []CoordinateFormalRootRekey,
	image *CoordinateIdentityTermImage,
	producers []DynamicReadIdentityProducer,
) ([]DynamicReadIdentityProducer, error) {
	if !dynamicReadIdentityTopologyDomainsCompatible(d, destinationDomain) || image == nil || len(wires) == 0 {
		return nil, fmt.Errorf("%w: dynamic-read producer formal transport", ErrInvalidLaneFactor)
	}
	var out []DynamicReadIdentityProducer
	var destination *keyspace.KeySpace
	for index, wire := range wires {
		if !wire.validFor(d) {
			return nil, fmt.Errorf("%w: dynamic-read producer wire %d", ErrInvalidLaneFactor, index)
		}
		if destination == nil {
			destination = wire.to
		} else if destination != wire.to {
			return nil, fmt.Errorf("%w: dynamic-read producer wire destinations", ErrInvalidLaneFactor)
		}
	}
	for index, producer := range producers {
		if producer.keys == nil || !producer.ValidFor(d, producer.keys) {
			return nil, fmt.Errorf("%w: malformed dynamic-read producer %d", ErrInvalidLaneFactor, index)
		}
		matchedSource := false
		for _, wire := range wires {
			if producer.keys != wire.from {
				continue
			}
			matchedSource = true
			rekeyed, mapped, err := d.TryRekeyDynamicReadIdentityProducerFormal(wire, producer)
			if err != nil {
				return nil, err
			}
			if !mapped {
				continue
			}
			imaged, err := d.ImageDynamicReadIdentityProducer(rekeyed, image)
			if err != nil {
				return nil, err
			}
			for index := range imaged {
				imaged[index].seal = destinationDomain.seal
				if !imaged[index].ValidFor(destinationDomain, destination) {
					return nil, fmt.Errorf("%w: invalid destination dynamic-read producer", ErrInvalidLaneFactor)
				}
			}
			out = append(out, imaged...)
		}
		if !matchedSource {
			return nil, fmt.Errorf("%w: dynamic-read producer %d has no source wire", ErrInvalidLaneFactor, index)
		}
	}
	return canonicalDynamicReadIdentityProducers(destination, out), nil
}

// PullbackDynamicReadIdentityProducersFormal computes exact target-side atom
// preimages for caller-side atoms through the same injective wire/image pair.
func (d ProductDomain) PullbackDynamicReadIdentityProducersFormal(
	callerDomain ProductDomain,
	wires []CoordinateFormalRootRekey,
	image *CoordinateIdentityTermImage,
	callers []DynamicReadIdentityProducer,
) ([]DynamicReadIdentityProducer, error) {
	if !dynamicReadIdentityTopologyDomainsCompatible(d, callerDomain) || image == nil || len(wires) == 0 {
		return nil, fmt.Errorf("%w: dynamic-read producer formal pullback", ErrInvalidLaneFactor)
	}
	var destination *keyspace.KeySpace
	for index, wire := range wires {
		if !wire.validFor(d) {
			return nil, fmt.Errorf("%w: dynamic-read producer pullback wire %d", ErrInvalidLaneFactor, index)
		}
		if destination == nil {
			destination = wire.from
		} else if destination != wire.from {
			return nil, fmt.Errorf("%w: dynamic-read producer pullback destinations", ErrInvalidLaneFactor)
		}
	}
	var out []DynamicReadIdentityProducer
	for index, caller := range callers {
		if caller.keys == nil || !caller.ValidFor(callerDomain, caller.keys) {
			return nil, fmt.Errorf("%w: malformed caller dynamic-read producer %d", ErrInvalidLaneFactor, index)
		}
		matchedSource := false
		for _, wire := range wires {
			if caller.keys != wire.to {
				continue
			}
			matchedSource = true
			pulled, mapped, err := d.tryPullbackDynamicReadIdentityProducerFormal(wire, callerDomain, caller)
			if err != nil {
				return nil, err
			}
			if !mapped {
				continue
			}
			if !caller.term.Valid() {
				out = append(out, pulled)
				continue
			}
			for _, binding := range image.Bindings() {
				for _, candidate := range binding.Images {
					if candidate == caller.term {
						alternative := pulled
						alternative.term = binding.Source
						if alternative.ValidFor(d, wire.from) {
							out = append(out, alternative)
						}
					}
				}
			}
		}
		if !matchedSource {
			return nil, fmt.Errorf("%w: caller dynamic-read producer %d has no pullback wire", ErrInvalidLaneFactor, index)
		}
	}
	return canonicalDynamicReadIdentityProducers(destination, out), nil
}

func (d ProductDomain) tryPullbackDynamicReadIdentityProducerFormal(plan CoordinateFormalRootRekey, callerDomain ProductDomain, caller DynamicReadIdentityProducer) (DynamicReadIdentityProducer, bool, error) {
	if !plan.validFor(d) || !dynamicReadIdentityTopologyDomainsCompatible(d, callerDomain) || caller.keys == nil || !caller.ValidFor(callerDomain, caller.keys) {
		return DynamicReadIdentityProducer{}, false, fmt.Errorf("%w: dynamic-read producer formal pullback", ErrInvalidLaneFactor)
	}
	if caller.keys != plan.to {
		return DynamicReadIdentityProducer{}, false, nil
	}
	pullKey := func(target keyspace.Key) (keyspace.Key, bool) {
		if target.Kind == keyspace.KindRootlessSuffix {
			return plan.from.ImportKey(plan.to, target)
		}
		root, ok := plan.to.StructuralRoot(target)
		if !ok {
			return keyspace.Key{}, false
		}
		for _, binding := range plan.roots {
			if binding.to == root {
				return plan.from.WithStructuralRoot(plan.to, target, binding.from)
			}
		}
		return keyspace.Key{}, false
	}
	out := caller
	out.seal = d.seal
	out.keys = plan.from
	switch caller.kind {
	case dynamicReadIdentityProducerPathMember, dynamicReadIdentityProducerHeapMember:
		var mapped bool
		out.path, mapped = pullKey(caller.path)
		if !mapped {
			return DynamicReadIdentityProducer{}, false, nil
		}
	case dynamicReadIdentityProducerPathDynamic, dynamicReadIdentityProducerHeapDynamic:
		var mapped bool
		out.fact.Table, mapped = pullKey(caller.fact.Table)
		if !mapped {
			return DynamicReadIdentityProducer{}, false, nil
		}
	case dynamicReadIdentityProducerHeapAnyMember:
	default:
		return DynamicReadIdentityProducer{}, false, fmt.Errorf("%w: invalid dynamic-read producer pullback kind", ErrInvalidLaneFactor)
	}
	return out, true, nil
}

func dynamicReadIdentityTopologyDomainsCompatible(source, destination ProductDomain) bool {
	return source.Valid() && destination.Valid() && source.reg == destination.reg
}

// DynamicReadIdentityProducerIdentityTerms returns the exact finite owner-term
// demand embedded by static/object topology atoms.
func (d ProductDomain) DynamicReadIdentityProducerIdentityTerms(producers []DynamicReadIdentityProducer) ([]identity.Term, error) {
	var terms []identity.Term
	for index, producer := range producers {
		if producer.keys == nil || !producer.ValidFor(d, producer.keys) {
			return nil, fmt.Errorf("%w: dynamic-read producer term %d", ErrInvalidLaneFactor, index)
		}
		if producer.term.Valid() {
			terms = append(terms, producer.term)
		}
	}
	return canonicalDynamicReadIdentityTerms(terms), nil
}

func canonicalDynamicReadIdentityTerms(input []identity.Term) []identity.Term {
	sort.Slice(input, func(left, right int) bool { return identityTermLess(input[left], input[right]) })
	write := 0
	for _, term := range input {
		if write != 0 && input[write-1] == term {
			continue
		}
		input[write] = term
		write++
	}
	return input[:write]
}

func canonicalDynamicReadIdentityPublications(d ProductDomain, keys *keyspace.KeySpace, input []DynamicReadIdentityPublication) ([]DynamicReadIdentityPublication, error) {
	for index, publication := range input {
		if publication.Source < 0 || !publication.Producer.ValidFor(d, keys) {
			return nil, fmt.Errorf("%w: dynamic-read identity publication %d", ErrInvalidLaneFactor, index)
		}
	}
	sort.Slice(input, func(left, right int) bool {
		if input[left].Producer != input[right].Producer {
			return dynamicReadIdentityProducerLess(keys, input[left].Producer, input[right].Producer)
		}
		return input[left].Source < input[right].Source
	})
	write := 0
	for _, publication := range input {
		if write != 0 && input[write-1] == publication {
			continue
		}
		input[write] = publication
		write++
	}
	return input[:write], nil
}

func canonicalDynamicReadIdentityProducers(keys *keyspace.KeySpace, input []DynamicReadIdentityProducer) []DynamicReadIdentityProducer {
	sort.Slice(input, func(left, right int) bool { return dynamicReadIdentityProducerLess(keys, input[left], input[right]) })
	write := 0
	for _, producer := range input {
		if write != 0 && input[write-1] == producer {
			continue
		}
		input[write] = producer
		write++
	}
	return input[:write]
}

func dynamicReadIdentityProducerLess(keys *keyspace.KeySpace, left, right DynamicReadIdentityProducer) bool {
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	if left.path != right.path {
		return keys.Less(left.path, right.path)
	}
	if left.term != right.term {
		return identityTermLess(left.term, right.term)
	}
	if left.fact.Table != right.fact.Table {
		return keys.Less(left.fact.Table, right.fact.Table)
	}
	if left.fact.Site != right.fact.Site {
		return left.fact.Site < right.fact.Site
	}
	if left.key.exact != right.key.exact {
		return !left.key.exact
	}
	if left.key.segment != right.key.segment {
		if left.key.segment.Kind != right.key.segment.Kind {
			return left.key.segment.Kind < right.key.segment.Kind
		}
		if left.key.segment.Name != right.key.segment.Name {
			return left.key.segment.Name < right.key.segment.Name
		}
		return left.key.segment.Index < right.key.segment.Index
	}
	return false
}
