package state

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

// PathReplacementConfig is the resolver-free syntax of one destructive path
// assignment.  Every key belongs to Keys; Source and UserSource are meaningful
// exactly when their corresponding presence bit is set.
type PathReplacementConfig struct {
	Keys          *keyspace.KeySpace
	Target        keyspace.Key
	Source        keyspace.Key
	HasSource     bool
	Value         product.Value
	PairedStatic  bool
	UserSource    keyspace.Key
	HasUserSource bool
}

// PathReplacementValueReader is the neutral concrete/formal Values view used
// while sealing the finite pre-state footprint.  ResolvePath must implement the
// registered ProductDomain path-resolution law; it is not an evaluator hook.
type PathReplacementValueReader interface {
	ReadPathReplacementValue(statekey.ValueDependency) (product.Value, bool)
}

// ResolveFactorPathValue is the public carrier-neutral boundary to the one
// registered structural read law. It admits concrete or formal Values roots
// through PathReplacementValueReader and never reconstructs State.
func (d ProductDomain) ResolveFactorPathValue(
	keys *keyspace.KeySpace,
	target keyspace.Key,
	values PathReplacementValueReader,
	factors []LaneFactor,
) (product.Value, bool) {
	if !d.Valid() || keys == nil || !keys.Valid() || target.Kind == keyspace.KindInvalid || values == nil {
		return product.Value{}, false
	}
	return d.resolvePathReplacementValue(keys, target, values, factors)
}

// PathReplacementHeapMutation is one pre-state-owned heap rewrite.
type PathReplacementHeapMutation struct {
	Owner           identity.ID
	Suffix          keyspace.Key
	DescendantsOnly bool
}

// PathReplacement is the immutable ProductDomain-owned endomorphism sealed
// from one correlated pre-state.  Its slices are detached, canonical and may
// safely be retained by a relation program.
type PathReplacementTransaction struct {
	seal            *productDomainSeal
	keys            *keyspace.KeySpace
	target          keyspace.Key
	source          keyspace.Key
	hasSource       bool
	value           product.Value
	subtreePrefixes []pathdom.PathKey
	writeKeys       []keyspace.Key
	rootWrites      []statekey.ValueDependency
	heapMutations   []PathReplacementHeapMutation
	userTarget      keyspace.Key
	userSource      keyspace.Key
	hasUserSource   bool
	quotient        pathevidence.EqualityQuotient
	hasQuotient     bool
}

func (d ProductDomain) ownsPathReplacement(t PathReplacementTransaction) bool {
	return d.Valid() && t.seal == d.seal && t.keys != nil && t.keys.Valid() &&
		t.target.Kind != keyspace.KindInvalid && product.BelongsToRegistry(d.reg, t.value)
}

func pathReplacementBinding(runtime *productLaneRuntime) (pathReplacementLaneBinding, bool) {
	if runtime == nil {
		return pathReplacementLaneBinding{}, false
	}
	law, ok := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathReplacement)
	return law.pathReplacement, ok && law.pathReplacement.declared && law.pathReplacement.apply != nil
}

func (d ProductDomain) PathReplacementReadLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 8)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		binding, ok := pathReplacementBinding(runtime)
		if ok && (binding.pointRead || binding.currentRead) && !runtime.lane.slotFactored {
			out = append(out, runtime.lane)
		}
	}
	return out
}

func (d ProductDomain) PathReplacementWriteLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 8)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		binding, ok := pathReplacementBinding(runtime)
		if ok && binding.write && !runtime.lane.slotFactored {
			out = append(out, runtime.lane)
		}
	}
	return out
}

// PreparePathReplacement snapshots all alias and heap ownership decisions
// before any participant is changed. current must contain the exact registered
// read inventory; no State is assembled.
func (d ProductDomain) PreparePathReplacement(config PathReplacementConfig, values PathReplacementValueReader, current []LaneFactor) (PathReplacementTransaction, error) {
	if !d.Valid() || config.Keys == nil || !config.Keys.Valid() || values == nil ||
		config.Target.Kind == keyspace.KindInvalid || !product.BelongsToRegistry(d.reg, config.Value) ||
		config.HasSource != (config.Source.Kind != keyspace.KindInvalid) ||
		config.HasUserSource != (config.UserSource.Kind != keyspace.KindInvalid) {
		return PathReplacementTransaction{}, fmt.Errorf("state: invalid path replacement")
	}
	want := d.PathReplacementReadLanes()
	if len(current) != len(want) {
		return PathReplacementTransaction{}, fmt.Errorf("%w: path replacement read inventory", ErrIncompleteLaneFactors)
	}
	byOrdinal := make(map[LaneOrdinal]LaneFactor, len(current))
	for i, factor := range current {
		runtime, err := d.validateFactor(factor)
		if err != nil || runtime.lane != want[i] {
			return PathReplacementTransaction{}, fmt.Errorf("%w: path replacement read factor %d", ErrIncompleteLaneFactors, i)
		}
		byOrdinal[runtime.lane.ordinal] = factor
	}
	pathFamily, ok := d.PathEvidenceCoordinateFamily()
	if !ok {
		return PathReplacementTransaction{}, fmt.Errorf("state: path replacement requires path evidence")
	}
	pathFactor, ok := byOrdinal[pathFamily.lane.ordinal]
	if !ok {
		return PathReplacementTransaction{}, fmt.Errorf("%w: path replacement path carrier", ErrIncompleteLaneFactors)
	}
	pathLane := typedLaneFactorValue[pathevidence.Lane](pathFactor.payload)
	targetPath := config.Keys.FormatReadOnly(config.Target)
	prefixes, ok := pathLane.PathKeySubtreeInvalidationPrefixes(config.Keys, targetPath)
	if !ok || len(prefixes) == 0 {
		return PathReplacementTransaction{}, fmt.Errorf("state: unresolved path replacement target")
	}
	aliases := append([]keyspace.Key{config.Target}, pathLane.EquivalentKeyspaceKeys(config.Keys, config.Target)...)
	for _, raw := range prefixes {
		if key, found := config.Keys.FromPathKey(raw); found {
			aliases = append(aliases, key)
		}
	}
	aliases = canonicalPathReplacementKeys(config.Keys, aliases)
	rootWrites := make([]statekey.ValueDependency, 0, len(aliases))
	for _, key := range aliases {
		if dependency, found := pathevidence.PathValueDependency(config.Keys, key); found {
			rootWrites = append(rootWrites, dependency)
		}
	}
	rootWrites = canonicalPathReplacementDependencies(rootWrites)
	descendantsOnly := config.PairedStatic && !typevalue.HasOnlyNilType(d.reg, config.Value)
	heapMutations := make([]PathReplacementHeapMutation, 0)
	for _, key := range aliases {
		segments, valid := config.Keys.SegmentsView(key)
		if !valid {
			continue
		}
		for split := 0; split <= len(segments); split++ {
			ownerKey, valid := config.Keys.StructuralRoot(key)
			if !valid {
				continue
			}
			for _, segment := range segments[:split] {
				ownerKey, valid = config.Keys.AppendSegment(ownerKey, segment)
				if !valid {
					break
				}
			}
			if !valid {
				continue
			}
			owner, found := d.resolvePathReplacementValue(config.Keys, ownerKey, values, current)
			id, found := identityvalue.ExactID(d.reg, owner)
			if !found {
				continue
			}
			suffix, suffixOK := config.Keys.FromRootlessSuffix(segments[split:])
			if !suffixOK {
				continue
			}
			heapMutations = append(heapMutations, PathReplacementHeapMutation{Owner: id, Suffix: suffix, DescendantsOnly: descendantsOnly})
		}
	}
	heapMutations = canonicalPathReplacementHeapMutations(config.Keys, heapMutations)
	tx := PathReplacementTransaction{seal: d.seal, keys: config.Keys, target: config.Target, source: config.Source, hasSource: config.HasSource,
		value: config.Value, subtreePrefixes: append([]pathdom.PathKey(nil), prefixes...), writeKeys: aliases,
		rootWrites: rootWrites, heapMutations: heapMutations, userTarget: config.Target,
		userSource: config.UserSource, hasUserSource: config.HasUserSource}
	_, quotient, hasQuotient, valid := applyPathReplacementPathEvidence(d, pathLane, tx)
	if !valid {
		return PathReplacementTransaction{}, fmt.Errorf("state: path replacement proof preparation failed")
	}
	tx.quotient, tx.hasQuotient = quotient, hasQuotient
	return tx, nil
}

func canonicalPathReplacementKeys(keys *keyspace.KeySpace, in []keyspace.Key) []keyspace.Key {
	seen := make(map[keyspace.Key]struct{}, len(in)*2)
	out := make([]keyspace.Key, 0, len(in)*2)
	for _, key := range in {
		for _, candidate := range []keyspace.Key{key} {
			if candidate.Kind == keyspace.KindInvalid {
				continue
			}
			if _, duplicate := seen[candidate]; !duplicate {
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
			if canonical, ok := keys.FieldCanonical(candidate); ok {
				if _, duplicate := seen[canonical]; !duplicate {
					seen[canonical] = struct{}{}
					out = append(out, canonical)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return keys.Less(out[i], out[j]) })
	return out
}

func canonicalPathReplacementDependencies(in []statekey.ValueDependency) []statekey.ValueDependency {
	seen := make(map[statekey.ValueDependency]struct{}, len(in))
	out := in[:0]
	for _, dependency := range in {
		if dependency.Valid() {
			if _, duplicate := seen[dependency]; !duplicate {
				seen[dependency] = struct{}{}
				out = append(out, dependency)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		leftConcrete, leftIsConcrete := out[i].Concrete()
		rightConcrete, rightIsConcrete := out[j].Concrete()
		if leftIsConcrete != rightIsConcrete {
			return leftIsConcrete
		}
		if leftIsConcrete {
			return leftConcrete < rightConcrete
		}
		leftFormal, _ := out[i].Formal()
		rightFormal, _ := out[j].Formal()
		return leftFormal.Less(rightFormal)
	})
	return out
}

// resolvePathReplacementValue is the factor-native structural resolver used
// only while sealing pre-state heap ownership.  It delegates every member read
// to the registered DynamicRead demand binder; it contains no lane inventory or
// alternate abstract-value semantics.
func (d ProductDomain) resolvePathReplacementValue(keys *keyspace.KeySpace, target keyspace.Key, values PathReplacementValueReader, factors []LaneFactor) (product.Value, bool) {
	plan, err := d.SealDynamicReadFactorProjectionPlan(keys)
	if err != nil {
		return product.Value{}, false
	}
	projection, err := d.BindDynamicReadFactorProjection(plan, factors)
	if err != nil {
		return product.Value{}, false
	}
	return d.resolvePathReplacementValueFromFactorProjection(keys, target, values, &projection)
}

func (d ProductDomain) resolvePathReplacementValueFromFactorProjection(
	keys *keyspace.KeySpace,
	target keyspace.Key,
	values PathReplacementValueReader,
	projection *DynamicReadFactorProjection,
) (product.Value, bool) {
	segments, ok := keys.SegmentsView(target)
	if !ok {
		return product.Value{}, false
	}
	root, ok := keys.StructuralRoot(target)
	if !ok {
		return product.Value{}, false
	}
	dependency, ok := pathevidence.PathValueDependency(keys, root)
	if !ok {
		return product.Value{}, false
	}
	current, ok := values.ReadPathReplacementValue(dependency)
	if !ok || product.Equal(d.reg, current, product.Bottom(d.reg)) {
		return product.Value{}, false
	}
	parent := root
	for _, part := range segments {
		keyValue, scalarOK := staticPathReplacementSegmentValue(d.reg, part)
		if !scalarOK {
			return product.Value{}, false
		}
		query := DynamicReadQuery{KeySpace: keys, TableValue: current, TablePath: parent, KeyValue: keyValue}
		// Concrete dynamic-index/key-membership carriers use StateKey syntax.
		// Formal structural roots remain represented by TablePath and the exact
		// Values dependency; manufacturing a concrete StateKey would collapse
		// the two namespaces.
		if parentState, concrete := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(parent)); concrete {
			query.TableKeys = []pathaddr.StateKey{parentState}
		}
		evidence, err := d.ProjectDynamicReadEvidenceFromFactorProjection(query, projection)
		if err != nil {
			return product.Value{}, false
		}
		member, memberOK := keys.AppendSegment(parent, part)
		if !memberOK {
			return product.Value{}, false
		}
		candidates := []keyspace.Key{member}
		if canonical, canonicalOK := keys.FieldCanonical(member); canonicalOK && canonical != member {
			candidates = append(candidates, canonical)
		}
		found := false
		for _, candidate := range candidates {
			if value, readable := evidence.PathValue(candidate); readable && !product.Equal(d.reg, value, product.Bottom(d.reg)) {
				current, found = value, true
				break
			}
		}
		if !found && evidence.HasValue {
			current, found = evidence.Value, true
		}
		if !found && evidence.HasHeapObject {
			if suffix, suffixOK := keys.FromRootlessSuffix([]segment.Segment{part}); suffixOK {
				if value, readable := evidence.HeapObject.StaticMember(suffix); readable && !product.Equal(d.reg, value, product.Bottom(d.reg)) {
					current, found = value, true
				}
				if !found {
					if canonical, canonicalOK := heapidentity.FieldCanonicalStaticMemberSuffixKey(keys, []segment.Segment{part}); canonicalOK {
						if value, readable := evidence.HeapObject.StaticMember(canonical); readable && !product.Equal(d.reg, value, product.Bottom(d.reg)) {
							current, found = value, true
						}
					}
				}
			}
		}
		if !found {
			return product.Value{}, false
		}
		parent = member
	}
	return current, true
}

func staticPathReplacementSegmentValue(reg *axis.Registry, part segment.Segment) (product.Value, bool) {
	switch part.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typevalue.LiteralString(reg, part.Name), true
	case segment.SegmentIndexInt:
		return typevalue.LiteralInt(reg, int64(part.Index)), true
	default:
		return product.Value{}, false
	}
}

func canonicalPathReplacementHeapMutations(keys *keyspace.KeySpace, in []PathReplacementHeapMutation) []PathReplacementHeapMutation {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Owner.Kind != in[j].Owner.Kind {
			return in[i].Owner.Kind < in[j].Owner.Kind
		}
		if in[i].Owner.Site != in[j].Owner.Site {
			return in[i].Owner.Site < in[j].Owner.Site
		}
		if in[i].Owner.Index != in[j].Owner.Index {
			return in[i].Owner.Index < in[j].Owner.Index
		}
		if in[i].Suffix != in[j].Suffix {
			return keys.Less(in[i].Suffix, in[j].Suffix)
		}
		return !in[i].DescendantsOnly && in[j].DescendantsOnly
	})
	out := in[:0]
	for _, mutation := range in {
		if len(out) == 0 || out[len(out)-1] != mutation {
			out = append(out, mutation)
		}
	}
	return out
}

// ApplyPathReplacementValues applies the Values phase to either concrete or
// formal slot syntax. resolve is the sole vocabulary binding.
func ApplyPathReplacementValues[K comparable](domain ProductDomain, transaction PathReplacementTransaction, current ValueFactor[K], resolve func(statekey.ValueDependency) (K, bool)) (ValueFactor[K], error) {
	if !domain.ownsPathReplacement(transaction) || resolve == nil {
		return ValueFactor[K]{}, fmt.Errorf("state: foreign path replacement Values")
	}
	if current.Top {
		return current, nil
	}
	out := make(map[K]product.Value, len(current.Values))
	for key, value := range current.Values {
		out[key] = value
	}
	for _, dependency := range transaction.rootWrites {
		key, ok := resolve(dependency)
		if !ok {
			return ValueFactor[K]{}, fmt.Errorf("state: unresolved path replacement Values root")
		}
		value, present := out[key]
		if !present || product.Equal(domain.reg, value, product.Bottom(domain.reg)) {
			continue
		}
		out[key] = product.Set(domain.reg, value, variantorigin.Key, variantorigin.Top())
	}
	return ValueFactor[K]{Values: out}, nil
}

// ApplyPathReplacementFactor stages one registered lane rewrite.  pointEntry
// and current must name the same lane; no State is reconstructed.
func (d ProductDomain) ApplyPathReplacementFactor(transaction PathReplacementTransaction, pointEntry, current LaneFactor) (LaneFactor, error) {
	if !d.ownsPathReplacement(transaction) {
		return LaneFactor{}, fmt.Errorf("state: foreign path replacement factor")
	}
	runtime, err := d.validateFactorPair(pointEntry, current)
	if err != nil {
		return LaneFactor{}, err
	}
	binding, declared := pathReplacementBinding(runtime)
	if !declared || !binding.write {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not write path replacement", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := binding.apply(d, pointEntry.payload, current.payload, transaction)
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected path replacement", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

type concretePathReplacementValues struct{ values ValueLaneFactor }

func (r concretePathReplacementValues) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	key, ok := dependency.Concrete()
	if !ok || r.values.Top {
		return product.Value{}, false
	}
	value, found := r.values.Values[key]
	return value, found
}

// ApplyConcretePathReplacement is the whole-State adapter for the same sealed
// factor transaction.  It projects only registered reads, stages every Values
// and lane result, and installs them atomically after all validation succeeds.
func (d ProductDomain) ApplyConcretePathReplacement(config PathReplacementConfig, pointEntry, current State) (State, bool, error) {
	if !d.Valid() {
		return State{}, false, fmt.Errorf("state: invalid path replacement domain")
	}
	pointEntry = d.Normalize(pointEntry)
	current = d.Normalize(current)
	currentResidual, currentValues := DecomposeValueLane(d.lattice, current)
	readLanes := d.PathReplacementReadLanes()
	readFactors, err := d.DecomposeLanes(currentResidual, readLanes)
	if err != nil {
		return State{}, false, err
	}
	transaction, err := d.PreparePathReplacement(config, concretePathReplacementValues{values: currentValues}, readFactors)
	if err != nil {
		return State{}, false, err
	}
	nextValues, err := ApplyPathReplacementValues(d, transaction, currentValues, func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() })
	if err != nil {
		return State{}, false, err
	}
	writes := d.PathReplacementWriteLanes()
	currentWrites, err := d.DecomposeLanes(currentResidual, writes)
	if err != nil {
		return State{}, false, err
	}
	pointResidual, _ := DecomposeValueLane(d.lattice, pointEntry)
	pointWrites, err := d.DecomposeLanes(pointResidual, writes)
	if err != nil {
		return State{}, false, err
	}
	staged := make([]LaneFactor, len(writes))
	for i := range writes {
		staged[i], err = d.ApplyPathReplacementFactor(transaction, pointWrites[i], currentWrites[i])
		if err != nil {
			return State{}, false, err
		}
	}
	out := currentResidual
	for i, factor := range staged {
		runtime, validateErr := d.validateFactorFor(&d.factorLanes[int(writes[i].ordinal)], factor)
		if validateErr != nil {
			return State{}, false, validateErr
		}
		runtime.ops.install(&out, factor.payload)
	}
	out.canonical = true
	out = RecomposeValueLane(d.reg, d.lattice, out, nextValues)
	return out, true, nil
}

func applyPathReplacementPathEvidence(d ProductDomain, lane pathevidence.Lane, tx PathReplacementTransaction) (pathevidence.Lane, pathevidence.EqualityQuotient, bool, bool) {
	lane = lane.InvalidatePathKeySubtreePrefixes(tx.keys, tx.subtreePrefixes)
	edit := pathevidence.EditLane(d.reg, lane)
	for _, key := range tx.writeKeys {
		edit.WritePathKey(key, tx.value)
	}
	lane, _ = edit.Done()
	if tx.hasSource {
		copies := make([]struct {
			key   keyspace.Key
			value product.Value
		}, 0)
		lane.ForEachPathStaticMember(func(member keyspace.Key, value product.Value) bool {
			suffix, ok := tx.keys.ExactRemainderAfterPrefix(member, tx.source)
			if !ok || len(suffix) == 0 {
				return true
			}
			destination := tx.target
			for _, segment := range suffix {
				destination, ok = tx.keys.AppendSegment(destination, segment)
				if !ok {
					break
				}
			}
			if ok {
				copies = append(copies, struct {
					key   keyspace.Key
					value product.Value
				}{destination, value})
			}
			return true
		})
		for _, copy := range copies {
			lane, _ = lane.WritePathStaticMember(copy.key, copy.value)
		}
		if tx.source != tx.target {
			lane, _ = lane.AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: tx.target, Other: tx.source})
		}
	}
	quotient, ok := lane.SealEqualityQuotient(tx.keys)
	return lane, quotient, tx.hasSource && tx.source != tx.target && ok, true
}

func applyPathReplacementPathLane(d ProductDomain, _ pathevidence.Lane, current pathevidence.Lane, tx PathReplacementTransaction) (pathevidence.Lane, bool, bool) {
	next, _, _, valid := applyPathReplacementPathEvidence(d, current, tx)
	return next, true, valid
}

func applyPathReplacementHeapLane(d ProductDomain, _ heapTableIdentityLane, current heapTableIdentityLane, tx PathReplacementTransaction) (heapTableIdentityLane, bool, bool) {
	changed := false
	for _, mutation := range tx.heapMutations {
		object := current.read(d.reg, mutation.Owner)
		objectChanged := false
		root := object.Root()
		nextRoot := product.Set(d.reg, root, typewitness.Key, typewitness.Top())
		nextRoot = product.Set(d.reg, nextRoot, variantorigin.Key, variantorigin.Top())
		if !product.Equal(d.reg, root, nextRoot) {
			object = object.WithRoot(nextRoot)
			objectChanged = true
		}
		segments, ok := tx.keys.SuffixSegmentsView(mutation.Suffix)
		if !ok {
			return current, false, false
		}
		var local bool
		if mutation.DescendantsOnly {
			object, local = object.WithoutStaticMemberDescendants(tx.keys, segments)
		} else {
			object, local = object.WithoutStaticMemberSubtree(tx.keys, segments)
		}
		objectChanged = objectChanged || local
		if mutation.DescendantsOnly {
			object, local = object.WithoutDynamicIndexFactDescendants(tx.keys, segments)
		} else {
			object, local = object.WithoutDynamicIndexFactSubtree(tx.keys, segments)
		}
		objectChanged = objectChanged || local
		if objectChanged {
			current = current.with(mutation.Owner, object)
			changed = true
		}
	}
	return current, changed, true
}

func applyPathReplacementUserLane(d ProductDomain, point, current userLatticeLane, tx PathReplacementTransaction) (userLatticeLane, bool, bool) {
	rt := userlattice.RuntimeFor(d.reg)
	changed := false
	for i := 0; i < rt.Len(); i++ {
		axis := rt.AxisAt(i)
		value := axis.Bottom()
		if tx.hasUserSource {
			value = axis.Assign(point.read(axis, tx.userSource))
		}
		var local bool
		current, local = current.write(axis, tx.userTarget, value)
		changed = changed || local
	}
	return current, changed, true
}

func applyPathReplacementValuesLane(_ ProductDomain, _ valueLane, current valueLane, _ PathReplacementTransaction) (valueLane, bool, bool) {
	return current, false, true
}

func applyPathReplacementDynamicLane(d ProductDomain, _ dynamicIndexLane, current dynamicIndexLane, tx PathReplacementTransaction) (dynamicIndexLane, bool, bool) {
	next, changed, valid := current.clearPathKeySubtree(tx.keys, tx.keys.FormatReadOnly(tx.target))
	if !valid {
		return current, false, false
	}
	if tx.hasQuotient {
		var quotientChanged bool
		next, quotientChanged, valid = applyPathEqualityDynamicIndex(next, d.reg, tx.keys, tx.quotient)
		changed = changed || quotientChanged
	}
	return next, changed, valid
}

func applyPathReplacementMembershipLane(d ProductDomain, _ keyMembershipLane, current keyMembershipLane, tx PathReplacementTransaction) (keyMembershipLane, bool, bool) {
	next, changed, valid := current.clearPathKeySubtree(tx.keys, tx.keys.FormatReadOnly(tx.target))
	if !valid {
		return current, false, false
	}
	if tx.hasQuotient {
		var quotientChanged bool
		next, quotientChanged, valid = applyPathEqualityKeyMemberships(next, d.reg, tx.keys, tx.quotient)
		changed = changed || quotientChanged
	}
	return next, changed, valid
}

func applyPathReplacementLenFloorLane(_ ProductDomain, _ lenFloorLane, current lenFloorLane, tx PathReplacementTransaction) (lenFloorLane, bool, bool) {
	next, changed := current.clearPathKeySubtrees(tx.keys, tx.subtreePrefixes)
	return next, changed, true
}

func applyPathReplacementTypestateLane(d ProductDomain, _ typestate.Store, current typestate.Store, tx PathReplacementTransaction) (typestate.Store, bool, bool) {
	if !tx.hasQuotient {
		return current, false, true
	}
	return applyPathEqualityTypestates(current, d.reg, tx.keys, tx.quotient)
}
