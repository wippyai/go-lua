package pathevidence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CoordinateSkeleton is the value-less quotient of the complete coupled path
// evidence carrier. The four Bottom markers stay together: Reachable and
// boundary-root writes change them atomically, so registering four unrelated
// coordinate families would admit states that Lane itself cannot produce.
type CoordinateSkeleton struct {
	refinementsBottom              bool
	staticMembersBottom            bool
	proofsBottom                   bool
	pathPresenceImplicationsBottom bool
}

// CoordinateKind identifies one of the four must sub-carriers. It is an
// ordering tag inside the package-owned composite family, not a State axis.
type CoordinateKind uint8

const (
	coordinateRefinement CoordinateKind = iota + 1
	coordinateStaticMember
	coordinateBranchProof
	coordinatePathPresenceImplication
)

// CoordinateKey is an opaque, comparable coordinate of the coupled family.
// Implication coordinates contain only their structural address. Product
// values are clause payload and live in CoordinateScalar, so a provider may
// vary them per DD leaf without changing the sealed coordinate universe.
type CoordinateKey struct {
	kind        CoordinateKind
	path        keyspace.Key
	proof       BranchProof
	implication PathPresenceImplication
}

// CoordinateDescriptorKind is the sealed structural role of one path-evidence
// coordinate. It exposes dependency shape without exposing the private key
// representation or scalar storage.
type CoordinateDescriptorKind uint8

const (
	CoordinateDescriptorInvalid CoordinateDescriptorKind = iota
	CoordinateDescriptorRefinement
	CoordinateDescriptorStaticMember
	CoordinateDescriptorBranchProof
	CoordinateDescriptorPresenceImplication
)

type CoordinateDescriptor struct {
	Kind        CoordinateDescriptorKind
	Path        keyspace.Key
	Proof       BranchProof
	Implication PathPresenceImplication
}

func DescribeCoordinate(source CoordinateKey) (CoordinateDescriptor, bool) {
	switch source.kind {
	case coordinateRefinement:
		return CoordinateDescriptor{Kind: CoordinateDescriptorRefinement, Path: source.path}, true
	case coordinateStaticMember:
		return CoordinateDescriptor{Kind: CoordinateDescriptorStaticMember, Path: source.path}, true
	case coordinateBranchProof:
		return CoordinateDescriptor{Kind: CoordinateDescriptorBranchProof, Proof: source.proof}, true
	case coordinatePathPresenceImplication:
		return CoordinateDescriptor{Kind: CoordinateDescriptorPresenceImplication, Implication: source.implication}, true
	default:
		return CoordinateDescriptor{}, false
	}
}

// StableRootMutationRemovesCoordinate reports whether the canonical
// stable-root invalidation law removes an explicitly present coordinate.
// It lets the product carrier authorize the exact finite write set before
// applying the persistent lane rewrite, without decomposing the lane twice.
func StableRootMutationRemovesCoordinate(
	source CoordinateKey,
	target symbol.ID,
	preserveAll bool,
	preserve func(PathPresenceImplication) bool,
) bool {
	if target == 0 {
		return false
	}
	matches := func(path keyspace.Key) bool { return stableKeyBelongsToSymbol(path, target) }
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		return matches(source.path)
	case coordinateBranchProof:
		return branchProofMatchesPath(source.proof, matches)
	case coordinatePathPresenceImplication:
		if preserveAll || preserve != nil && preserve(source.implication) {
			return false
		}
		return pathPresenceImplicationMatchesPath(source.implication, matches)
	default:
		return false
	}
}

func PresenceImplicationCoordinate(implication PathPresenceImplication) CoordinateKey {
	key, _ := implicationCoordinateParts(implication)
	return key
}

func RefinementCoordinate(path keyspace.Key) CoordinateKey {
	return CoordinateKey{kind: coordinateRefinement, path: path}
}

// StaticMemberCoordinate returns the canonical exact-path static-member
// evidence coordinate.
func StaticMemberCoordinate(path keyspace.Key) CoordinateKey {
	return CoordinateKey{kind: coordinateStaticMember, path: path}
}

// BranchProofCoordinate returns the scalar identity published by one branch
// proof. Construction remains inside the owning family; callers receive only
// the opaque product-level CoordinateSlot sealed from this key.
func BranchProofCoordinate(proof BranchProof) CoordinateKey {
	return CoordinateKey{kind: coordinateBranchProof, proof: proof}
}

// CoordinateScalar is the lifted optional scalar for one CoordinateKey.
// present=false is the omitted-coordinate Top. Value-bearing map coordinates
// additionally carry the exact product-lattice value. Set coordinates use the
// two-point must-membership lattice (present <= absent).
type CoordinateScalar struct {
	present            bool
	valueBearing       bool
	implicationBearing bool
	clauseBottom       bool
	value              product.Value
	clauses            []coordinateImplicationClause
}

type coordinateImplicationClause struct {
	trigger product.Value
	target  product.Value
}

// CoordinateScalarValue observes an explicitly present value-bearing scalar.
// Omitted coordinates and set-valued coordinates have no product value.
func CoordinateScalarValue(key CoordinateKey, scalar CoordinateScalar) (product.Value, bool) {
	if (key.kind != coordinateRefinement && key.kind != coordinateStaticMember) || !scalar.present || !scalar.valueBearing {
		return product.Value{}, false
	}
	return scalar.value, true
}

func implicationCoordinateParts(implication PathPresenceImplication) (CoordinateKey, CoordinateScalar) {
	key := implication
	clause := coordinateImplicationClause{}
	if key.HasTriggerValue {
		clause.trigger, key.TriggerValue = key.TriggerValue, product.Value{}
	}
	if key.HasTargetValue {
		clause.target, key.TargetValue = key.TargetValue, product.Value{}
	}
	return CoordinateKey{kind: coordinatePathPresenceImplication, implication: key}, CoordinateScalar{
		present: true, implicationBearing: true, clauses: []coordinateImplicationClause{clause},
	}
}

func implicationFromCoordinate(key CoordinateKey, clause coordinateImplicationClause) PathPresenceImplication {
	value := key.implication
	if value.HasTriggerValue {
		value.TriggerValue = clause.trigger
	}
	if value.HasTargetValue {
		value.TargetValue = clause.target
	}
	return value
}

func implicationCoordinateKeyValid(key CoordinateKey) bool {
	if key.kind != coordinatePathPresenceImplication || !validPathPresenceImplicationShape(key.implication) {
		return false
	}
	return (!key.implication.HasTriggerValue || key.implication.TriggerValue == (product.Value{})) &&
		(!key.implication.HasTargetValue || key.implication.TargetValue == (product.Value{}))
}

func implicationClauseLess(a, b coordinateImplicationClause) bool {
	if order := comparePathPresenceImplicationProducts(a.trigger, b.trigger); order != 0 {
		return order < 0
	}
	return comparePathPresenceImplicationProducts(a.target, b.target) < 0
}

func canonicalImplicationClauses(in []coordinateImplicationClause) []coordinateImplicationClause {
	out := append([]coordinateImplicationClause(nil), in...)
	sort.Slice(out, func(i, j int) bool { return implicationClauseLess(out[i], out[j]) })
	write := 0
	for _, clause := range out {
		if write != 0 && out[write-1] == clause {
			continue
		}
		out[write], write = clause, write+1
	}
	return out[:write]
}

func implicationClausesCanonical(in []coordinateImplicationClause) bool {
	if len(in) == 0 {
		return false
	}
	for index := 1; index < len(in); index++ {
		if !implicationClauseLess(in[index-1], in[index]) {
			return false
		}
	}
	return true
}

func implicationClauseSlicesEqual(_ *axis.Registry, a, b []coordinateImplicationClause) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func implicationClausesContain(_ *axis.Registry, superset, subset []coordinateImplicationClause) bool {
	for si, pi := 0, 0; pi < len(subset); {
		if si == len(superset) {
			return false
		}
		switch {
		case superset[si] == subset[pi]:
			si, pi = si+1, pi+1
		case implicationClauseLess(superset[si], subset[pi]):
			si++
		default:
			return false
		}
	}
	return true
}

func intersectImplicationClauses(_ *axis.Registry, a, b []coordinateImplicationClause) []coordinateImplicationClause {
	out := make([]coordinateImplicationClause, 0, min(len(a), len(b)))
	for ai, bi := 0, 0; ai < len(a) && bi < len(b); {
		switch {
		case a[ai] == b[bi]:
			out = append(out, a[ai])
			ai, bi = ai+1, bi+1
		case implicationClauseLess(a[ai], b[bi]):
			ai++
		default:
			bi++
		}
	}
	return out
}

func unionImplicationClauses(a, b []coordinateImplicationClause) []coordinateImplicationClause {
	out := make([]coordinateImplicationClause, 0, len(a)+len(b))
	for ai, bi := 0, 0; ai < len(a) || bi < len(b); {
		switch {
		case ai == len(a):
			out = append(out, b[bi:]...)
			bi = len(b)
		case bi == len(b):
			out = append(out, a[ai:]...)
			ai = len(a)
		case a[ai] == b[bi]:
			out = append(out, a[ai])
			ai, bi = ai+1, bi+1
		case implicationClauseLess(a[ai], b[bi]):
			out = append(out, a[ai])
			ai++
		default:
			out = append(out, b[bi])
			bi++
		}
	}
	return out
}

// CoordinateEntry is one explicitly present coordinate in canonical order.
type CoordinateEntry struct {
	Key    CoordinateKey
	Scalar CoordinateScalar
}

// CoordinateRoot is one ordered boundary path-root write.
type CoordinateRoot struct {
	Path  keyspace.Key
	Value product.Value
}

// VisitCoordinateValueDependencies reports the concrete or formal Values roots
// named by one coordinate key. Scalar payloads contain abstract values, but
// root alignment is determined solely by the structural paths in the key.
func VisitCoordinateValueDependencies(source CoordinateKey, keys *keyspace.KeySpace, visit func(statekey.ValueDependency)) {
	if keys == nil || !keys.Valid() || visit == nil {
		return
	}
	visitPath := func(path keyspace.Key) {
		if dependency, ok := PathValueDependency(keys, path); ok {
			visit(dependency)
		}
	}
	switch source.kind {
	case coordinateRefinement, coordinateStaticMember:
		visitPath(source.path)
	case coordinateBranchProof:
		visitPath(source.proof.Path)
		visitPath(source.proof.Other)
	case coordinatePathPresenceImplication:
		visitPath(source.implication.Trigger)
		visitPath(source.implication.TriggerOther)
		visitPath(source.implication.Target)
	}
}

// PathValueDependency maps one structural path root to the exact Values-root
// vocabulary it names. Static suffixes never change the root dependency:
// field-sensitive evidence remains incident to its complete concrete or formal
// root rather than inventing a field-shaped Values cell.
func PathValueDependency(keys *keyspace.KeySpace, path keyspace.Key) (statekey.ValueDependency, bool) {
	if keys == nil || !keys.Valid() {
		return statekey.ValueDependency{}, false
	}
	if root, ok := keys.DescribeFormalRoot(path); ok {
		dependency := statekey.FormalDependency(root)
		return dependency, dependency.Valid()
	}
	var value statekey.Value
	switch path.Kind {
	case keyspace.KindResolverSym, keyspace.KindUnversionedSym, keyspace.KindStableSym:
		value = statekey.SymbolValue(path.Sym)
	case keyspace.KindRetSlot:
		value = statekey.ReturnSlot(int(path.Root))
	default:
		return statekey.ValueDependency{}, false
	}
	dependency := statekey.ConcreteDependency(value)
	return dependency, dependency.Valid()
}

// CoordinatePathClosure marks the transitive path-evidence component incident
// to seeds. It is the family-owned dependency law used by operation plans
// whose semantics read/write a finite set of structural paths.
func CoordinatePathClosure(keys []CoordinateKey, ks *keyspace.KeySpace, seeds []keyspace.Key) ([]bool, bool) {
	return coordinatePathClosure(keys, ks, seeds, nil)
}

// CoordinatePathMutationClosure additionally treats stable and unversioned
// root spellings of symbols as wildcard seeds. Root assignment invalidation
// owns this non-structural relation; KeySpace prefix comparison deliberately
// does not equate those spellings.
func CoordinatePathMutationClosure(keys []CoordinateKey, ks *keyspace.KeySpace, seeds []keyspace.Key, roots []symbol.ID) ([]bool, bool) {
	rootSet := make(map[symbol.ID]struct{}, len(roots))
	for _, root := range roots {
		if root != 0 {
			rootSet[root] = struct{}{}
		}
	}
	return coordinatePathClosure(keys, ks, seeds, rootSet)
}

type coordinatePathClosureWork struct {
	coordinateSelections int
	pathActivations      int
	adjacencyVisits      int
	trieNodeExpansions   int
}

type coordinatePathClosureRoot struct {
	kind keyspace.KeyKind
	sym  symbol.ID
	ver  uint32
	root uint32
}

type coordinatePathClosureNode struct {
	parent          *coordinatePathClosureNode
	children        map[segment.Segment]*coordinatePathClosureNode
	childrenOrder   []*coordinatePathClosureNode
	coordinates     []int
	exactVisited    bool
	subtreeExpanded bool
	queued          bool
}

type coordinatePathClosureCoordinate struct {
	paths [3]keyspace.Key
	nodes [3]*coordinatePathClosureNode
	count int
}

func coordinatePathClosure(keys []CoordinateKey, ks *keyspace.KeySpace, seeds []keyspace.Key, rootSymbols map[symbol.ID]struct{}) ([]bool, bool) {
	selected, ok, _ := coordinatePathClosureIndexed(keys, ks, seeds, rootSymbols, false)
	return selected, ok
}

func coordinatePathClosureWithWork(keys []CoordinateKey, ks *keyspace.KeySpace, seeds []keyspace.Key, rootSymbols map[symbol.ID]struct{}) ([]bool, bool, coordinatePathClosureWork) {
	return coordinatePathClosureIndexed(keys, ks, seeds, rootSymbols, true)
}

func coordinatePathClosureIndexed(
	keys []CoordinateKey,
	ks *keyspace.KeySpace,
	seeds []keyspace.Key,
	rootSymbols map[symbol.ID]struct{},
	recordWork bool,
) ([]bool, bool, coordinatePathClosureWork) {
	if ks == nil || !ks.Valid() {
		return nil, false, coordinatePathClosureWork{}
	}
	var work coordinatePathClosureWork
	coordinates := make([]coordinatePathClosureCoordinate, len(keys))
	roots := make(map[coordinatePathClosureRoot]*coordinatePathClosureNode)
	pathsOf := func(source CoordinateKey) (coordinatePathClosureCoordinate, bool) {
		if !CoordinateKeyValid(source, ks, nil) {
			return coordinatePathClosureCoordinate{}, false
		}
		var out coordinatePathClosureCoordinate
		add := func(path keyspace.Key) {
			out.paths[out.count] = path
			out.count++
		}
		switch source.kind {
		case coordinateRefinement, coordinateStaticMember:
			add(source.path)
		case coordinateBranchProof:
			add(source.proof.Path)
			add(source.proof.Other)
		case coordinatePathPresenceImplication:
			add(source.implication.Trigger)
			add(source.implication.TriggerOther)
			add(source.implication.Target)
		default:
			return coordinatePathClosureCoordinate{}, false
		}
		return out, true
	}

	normalized := func(path keyspace.Key) (coordinatePathClosureRoot, []segment.Segment, bool) {
		segments, ok := ks.SegmentsView(path)
		if !ok || !ks.HasPrefix(path, path) {
			return coordinatePathClosureRoot{}, nil, false
		}
		root := coordinatePathClosureRoot{kind: path.Kind, sym: path.Sym, ver: path.Ver, root: path.Root}
		if path.Kind != keyspace.KindResolverSym {
			return root, segments, true
		}
		var canonical []segment.Segment
		for index, value := range segments {
			if value.Kind != segment.SegmentIndexString {
				continue
			}
			if canonical == nil {
				canonical = append([]segment.Segment(nil), segments...)
			}
			canonical[index] = segment.Segment{Kind: segment.SegmentField, Name: value.Name}
		}
		if canonical != nil {
			segments = canonical
		}
		return root, segments, true
	}

	insert := func(root coordinatePathClosureRoot, segments []segment.Segment) *coordinatePathClosureNode {
		node := roots[root]
		if node == nil {
			node = &coordinatePathClosureNode{}
			roots[root] = node
		}
		for _, value := range segments {
			child := node.children[value]
			if child == nil {
				if node.children == nil {
					node.children = make(map[segment.Segment]*coordinatePathClosureNode)
				}
				child = &coordinatePathClosureNode{parent: node}
				node.children[value] = child
				node.childrenOrder = append(node.childrenOrder, child)
			}
			node = child
		}
		return node
	}

	for index, source := range keys {
		coordinate, ok := pathsOf(source)
		if !ok {
			return nil, false, coordinatePathClosureWork{}
		}
		for pathIndex := 0; pathIndex < coordinate.count; pathIndex++ {
			root, segments, structural := normalized(coordinate.paths[pathIndex])
			if !structural {
				continue
			}
			node := insert(root, segments)
			node.coordinates = append(node.coordinates, index)
			coordinate.nodes[pathIndex] = node
		}
		coordinates[index] = coordinate
	}

	selected := make([]bool, len(keys))
	queue := make([]*coordinatePathClosureNode, 0, len(keys))
	enqueueNode := func(node *coordinatePathClosureNode) {
		if node == nil || node.queued {
			return
		}
		node.queued = true
		queue = append(queue, node)
	}
	selectCoordinate := func(index int) {
		if selected[index] {
			return
		}
		selected[index] = true
		if recordWork {
			work.coordinateSelections++
		}
		coordinate := coordinates[index]
		for pathIndex := 0; pathIndex < coordinate.count; pathIndex++ {
			enqueueNode(coordinate.nodes[pathIndex])
		}
	}
	visitExact := func(node *coordinatePathClosureNode) {
		if node == nil || node.exactVisited {
			return
		}
		node.exactVisited = true
		for _, index := range node.coordinates {
			if recordWork {
				work.adjacencyVisits++
			}
			selectCoordinate(index)
		}
	}
	expandSubtree := func(start *coordinatePathClosureNode) {
		stack := []*coordinatePathClosureNode{start}
		for len(stack) != 0 {
			last := len(stack) - 1
			node := stack[last]
			stack = stack[:last]
			if node == nil || node.subtreeExpanded {
				continue
			}
			node.subtreeExpanded = true
			if recordWork {
				work.trieNodeExpansions++
			}
			visitExact(node)
			for index := len(node.childrenOrder) - 1; index >= 0; index-- {
				stack = append(stack, node.childrenOrder[index])
			}
		}
	}
	activateNode := func(node *coordinatePathClosureNode) {
		if node == nil || node.subtreeExpanded {
			return
		}
		if recordWork {
			work.pathActivations++
		}
		for ancestor := node; ancestor != nil; ancestor = ancestor.parent {
			visitExact(ancestor)
		}
		expandSubtree(node)
	}
	activateSeed := func(path keyspace.Key) {
		root, segments, structural := normalized(path)
		if !structural {
			return
		}
		node := roots[root]
		if node == nil {
			return
		}
		visitExact(node)
		for _, value := range segments {
			child := node.children[value]
			if child == nil {
				return
			}
			node = child
			visitExact(node)
		}
		expandSubtree(node)
	}

	for _, seed := range seeds {
		activateSeed(seed)
	}
	if len(rootSymbols) != 0 {
		for index, coordinate := range coordinates {
			for pathIndex := 0; pathIndex < coordinate.count; pathIndex++ {
				path := coordinate.paths[pathIndex]
				if path.Segs != 0 || (path.Kind != keyspace.KindStableSym && path.Kind != keyspace.KindUnversionedSym) {
					continue
				}
				if _, matches := rootSymbols[path.Sym]; !matches {
					continue
				}
				selectCoordinate(index)
				break
			}
		}
	}
	for head := 0; head < len(queue); head++ {
		activateNode(queue[head])
	}
	return selected, true, work
}

// CoordinateBottom returns the exact family Bottom quotient.
func CoordinateBottom() CoordinateSkeleton {
	return CoordinateSkeleton{
		refinementsBottom: true, staticMembersBottom: true,
		proofsBottom: true, pathPresenceImplicationsBottom: true,
	}
}

// CoordinateTop returns the exact family Top quotient.
func CoordinateTop() CoordinateSkeleton { return CoordinateSkeleton{} }

// Reachable clears all four coupled Bottom markers, matching Lane.Reachable.
func (s CoordinateSkeleton) Reachable() CoordinateSkeleton { return CoordinateSkeleton{} }

// DecomposeCoordinates atomically transposes all four must sub-carriers into
// one skeleton and a strict total-order inventory of explicit coordinates.
func DecomposeCoordinates(lane Lane, ks *keyspace.KeySpace) (CoordinateSkeleton, []CoordinateEntry, bool) {
	if ks == nil || !ks.Valid() {
		return CoordinateSkeleton{}, nil, false
	}
	skeleton := CoordinateSkeleton{
		refinementsBottom: lane.refinementsBottom, staticMembersBottom: lane.staticMembersBottom,
		proofsBottom: lane.proofsBottom, pathPresenceImplicationsBottom: lane.pathPresenceImplicationsBottom,
	}
	entries := make([]CoordinateEntry, 0, len(lane.refinements)+len(lane.staticMembers)+len(lane.proofs)+len(lane.pathPresenceImplications))
	if !lane.refinementsBottom {
		for path, value := range lane.refinements {
			entries = append(entries, CoordinateEntry{
				Key:    CoordinateKey{kind: coordinateRefinement, path: path},
				Scalar: CoordinateScalar{present: true, valueBearing: true, value: value},
			})
		}
	}
	if !lane.staticMembersBottom {
		for path, value := range lane.staticMembers {
			entries = append(entries, CoordinateEntry{
				Key:    CoordinateKey{kind: coordinateStaticMember, path: path},
				Scalar: CoordinateScalar{present: true, valueBearing: true, value: value},
			})
		}
	}
	if !lane.proofsBottom {
		for proof := range lane.proofs {
			entries = append(entries, CoordinateEntry{
				Key:    CoordinateKey{kind: coordinateBranchProof, proof: proof},
				Scalar: CoordinateScalar{present: true},
			})
		}
	}
	if !lane.pathPresenceImplicationsBottom {
		implications := make(map[CoordinateKey]CoordinateScalar, len(lane.pathPresenceImplications))
		for implication := range lane.pathPresenceImplications {
			key, scalar := implicationCoordinateParts(implication)
			if current, present := implications[key]; present {
				current.clauses = append(current.clauses, scalar.clauses...)
				implications[key] = current
			} else {
				implications[key] = scalar
			}
		}
		for key, scalar := range implications {
			scalar.clauses = canonicalImplicationClauses(scalar.clauses)
			entries = append(entries, CoordinateEntry{Key: key, Scalar: scalar})
		}
	}
	for _, entry := range entries {
		if !CoordinateKeyValid(entry.Key, ks, nil) {
			return CoordinateSkeleton{}, nil, false
		}
	}
	sort.Slice(entries, func(i, j int) bool { return CoordinateKeyLess(entries[i].Key, entries[j].Key, ks) })
	return skeleton, entries, true
}

// ComposeCoordinates is the exact inverse of DecomposeCoordinates. The entire
// inventory is validated before any Lane maps are allocated or published.
func ComposeCoordinates(skeleton CoordinateSkeleton, entries []CoordinateEntry, reg *axis.Registry, ks *keyspace.KeySpace) (Lane, bool) {
	if reg == nil || ks == nil || !ks.Valid() {
		return Lane{}, false
	}
	for index, entry := range entries {
		if !CoordinateKeyValid(entry.Key, ks, reg) || !CoordinateScalarValid(entry.Key, entry.Scalar, reg) || !entry.Scalar.present {
			return Lane{}, false
		}
		if index != 0 && !CoordinateKeyLess(entries[index-1].Key, entry.Key, ks) {
			return Lane{}, false
		}
		switch entry.Key.kind {
		case coordinateRefinement, coordinateStaticMember, coordinateBranchProof, coordinatePathPresenceImplication:
		default:
			return Lane{}, false
		}
	}
	out := Lane{
		refinementsBottom: skeleton.refinementsBottom, staticMembersBottom: skeleton.staticMembersBottom,
		proofsBottom: skeleton.proofsBottom, pathPresenceImplicationsBottom: skeleton.pathPresenceImplicationsBottom,
	}
	for _, entry := range entries {
		if coordinateKindBottom(skeleton, entry.Key.kind) {
			continue
		}
		switch entry.Key.kind {
		case coordinateRefinement:
			if out.refinements == nil {
				out.refinements = make(map[keyspace.Key]product.Value)
			}
			out.refinements[entry.Key.path] = entry.Scalar.value
		case coordinateStaticMember:
			if out.staticMembers == nil {
				out.staticMembers = make(map[keyspace.Key]product.Value)
			}
			out.staticMembers[entry.Key.path] = entry.Scalar.value
		case coordinateBranchProof:
			if out.proofs == nil {
				out.proofs = make(map[BranchProof]struct{})
			}
			out.proofs[entry.Key.proof] = struct{}{}
			out.equalityRootMask.merge(equalityProofRootMask(entry.Key.proof))
		case coordinatePathPresenceImplication:
			if out.pathPresenceImplications == nil {
				out.pathPresenceImplications = make(map[PathPresenceImplication]struct{})
			}
			for _, clause := range entry.Scalar.clauses {
				out.pathPresenceImplications[implicationFromCoordinate(entry.Key, clause)] = struct{}{}
			}
		}
	}
	return out, true
}

// CoordinateSkeletonEqual reports exact equality of all four Bottom markers.
func CoordinateSkeletonEqual(a, b CoordinateSkeleton) bool { return a == b }

// CoordinateSkeletonLessOrEq is the componentwise Bottom <= Top order.
func CoordinateSkeletonLessOrEq(a, b CoordinateSkeleton) bool {
	return bottomMarkerLessOrEq(a.refinementsBottom, b.refinementsBottom) &&
		bottomMarkerLessOrEq(a.staticMembersBottom, b.staticMembersBottom) &&
		bottomMarkerLessOrEq(a.proofsBottom, b.proofsBottom) &&
		bottomMarkerLessOrEq(a.pathPresenceImplicationsBottom, b.pathPresenceImplicationsBottom)
}

func bottomMarkerLessOrEq(a, b bool) bool { return a || !b }

// CoordinateSkeletonJoin is componentwise must-lane join.
func CoordinateSkeletonJoin(a, b CoordinateSkeleton) CoordinateSkeleton {
	return CoordinateSkeleton{
		refinementsBottom:              a.refinementsBottom && b.refinementsBottom,
		staticMembersBottom:            a.staticMembersBottom && b.staticMembersBottom,
		proofsBottom:                   a.proofsBottom && b.proofsBottom,
		pathPresenceImplicationsBottom: a.pathPresenceImplicationsBottom && b.pathPresenceImplicationsBottom,
	}
}

// CoordinateSkeletonMeet is the exact componentwise greatest lower bound of
// the four explicit Bottom/Top markers.
func CoordinateSkeletonMeet(a, b CoordinateSkeleton) CoordinateSkeleton {
	return CoordinateSkeleton{
		refinementsBottom:              a.refinementsBottom || b.refinementsBottom,
		staticMembersBottom:            a.staticMembersBottom || b.staticMembersBottom,
		proofsBottom:                   a.proofsBottom || b.proofsBottom,
		pathPresenceImplicationsBottom: a.pathPresenceImplicationsBottom || b.pathPresenceImplicationsBottom,
	}
}

// CoordinateSkeletonNarrow matches the lane's forward-only narrowing law.
func CoordinateSkeletonNarrow(previous, _ CoordinateSkeleton) CoordinateSkeleton { return previous }

// CoordinateSkeletonHash is a collision-only fingerprint; equality remains
// authoritative at the state registration boundary.
func CoordinateSkeletonHash(value CoordinateSkeleton) uint64 {
	var hash uint64
	if value.refinementsBottom {
		hash |= 1
	}
	if value.staticMembersBottom {
		hash |= 2
	}
	if value.proofsBottom {
		hash |= 4
	}
	if value.pathPresenceImplicationsBottom {
		hash |= 8
	}
	return hash
}

// CoordinateKeyValid checks keyspace ownership and every embedded product
// value. A nil registry performs only structural validation for decomposition;
// registration-owned operations always pass their registry.
func CoordinateKeyValid(key CoordinateKey, ks *keyspace.KeySpace, reg *axis.Registry) bool {
	if ks == nil || !ks.Valid() {
		return false
	}
	validRequired := func(path keyspace.Key) bool {
		if path.Kind == keyspace.KindInvalid {
			return false
		}
		_, ok := ks.SegmentsView(path)
		return ok
	}
	validOptional := func(path keyspace.Key) bool {
		return path.Kind == keyspace.KindInvalid || validRequired(path)
	}
	switch key.kind {
	case coordinateRefinement, coordinateStaticMember:
		return validRequired(key.path)
	case coordinateBranchProof:
		return key.proof.Kind != 0 && validRequired(key.proof.Path) && validOptional(key.proof.Other)
	case coordinatePathPresenceImplication:
		value := key.implication
		if !implicationCoordinateKeyValid(key) || !validRequired(value.Trigger) || !validOptional(value.TriggerOther) || !validRequired(value.Target) {
			return false
		}
		return true
	default:
		return false
	}
}

// CoordinateKeyLess is the strict total order used by decomposition and
// publication. CoordinateKind keeps the four sub-carriers disjoint.
func CoordinateKeyLess(a, b CoordinateKey, ks *keyspace.KeySpace) bool {
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	switch a.kind {
	case coordinateRefinement, coordinateStaticMember:
		return a.path != b.path && ks.Less(a.path, b.path)
	case coordinateBranchProof:
		return a.proof != b.proof && branchProofLess(ks, a.proof, b.proof)
	case coordinatePathPresenceImplication:
		return a.implication != b.implication && pathPresenceImplicationLess(ks, a.implication, b.implication)
	default:
		return false
	}
}

// CoordinateKeyHash is consistent with exact CoordinateKey equality. The
// KeySpace-owned dense path fields are sufficient inside one family arena;
// product-bearing implication fields additionally use their canonical product
// hashes. Equality remains authoritative for the deliberately collision-only
// result.
func CoordinateKeyHash(reg *axis.Registry, _ *keyspace.KeySpace, key CoordinateKey) uint64 {
	h := internal.MixHash(internal.FnvString("path-evidence.coordinate"), uint64(key.kind))
	mixPath := func(path keyspace.Key) {
		h = internal.MixHash(h, uint64(path.Kind))
		h = internal.MixHash(h, uint64(path.Sym))
		h = internal.MixHash(h, uint64(path.Ver))
		h = internal.MixHash(h, uint64(path.Root))
		h = internal.MixHash(h, uint64(path.Segs))
		if path.Canon {
			h = internal.MixHash(h, 1)
		}
	}
	switch key.kind {
	case coordinateRefinement, coordinateStaticMember:
		mixPath(key.path)
	case coordinateBranchProof:
		h = internal.MixHash(h, uint64(key.proof.Kind))
		mixPath(key.proof.Path)
		h = internal.MixHash(h, uint64(key.proof.Presence))
		mixPath(key.proof.Other)
	case coordinatePathPresenceImplication:
		value := key.implication
		mixPath(value.Trigger)
		mixPath(value.TriggerOther)
		h = internal.MixHash(h, uint64(value.TriggerPresence))
		// Product-valued clauses are scalar payload, never coordinate identity.
		if value.HasTriggerPresence {
			h = internal.MixHash(h, 1)
		}
		if value.HasTriggerPathEqual {
			h = internal.MixHash(h, 2)
		}
		if value.HasTriggerTruthiness {
			h = internal.MixHash(h, 4)
			if value.TriggerTruthy {
				h = internal.MixHash(h, 8)
			}
		}
		mixPath(value.Target)
		h = internal.MixHash(h, uint64(value.TargetPresence))
	}
	return h
}

// ImportCoordinateKey re-interns every structural path while preserving the
// complete must fact and its embedded product values.
func ImportCoordinateKey(key CoordinateKey, from, to *keyspace.KeySpace, reg *axis.Registry) (CoordinateKey, bool) {
	if !CoordinateKeyValid(key, from, reg) || to == nil || !to.Valid() {
		return CoordinateKey{}, false
	}
	key, ok := MapCoordinateKeyPaths(key, func(source keyspace.Key) (keyspace.Key, bool) {
		return to.ImportKey(from, source)
	})
	if !ok || !CoordinateKeyValid(key, to, reg) {
		return CoordinateKey{}, false
	}
	return key, true
}

// MapCoordinateKeyPaths applies one structural root mapper to every path in a
// coordinate. It is the single traversal used by ordinary keyspace import and
// formal-root rekey; product registration owns the mapper itself.
func MapCoordinateKeyPaths(key CoordinateKey, mapKey func(keyspace.Key) (keyspace.Key, bool)) (CoordinateKey, bool) {
	if mapKey == nil {
		return CoordinateKey{}, false
	}
	var ok bool
	switch key.kind {
	case coordinateRefinement, coordinateStaticMember:
		key.path, ok = mapKey(key.path)
	case coordinateBranchProof:
		key.proof.Path, ok = mapKey(key.proof.Path)
		if ok {
			if key.proof.Other.Kind != keyspace.KindInvalid {
				key.proof.Other, ok = mapKey(key.proof.Other)
			}
		}
	case coordinatePathPresenceImplication:
		key.implication.Trigger, ok = mapKey(key.implication.Trigger)
		if ok {
			if key.implication.TriggerOther.Kind != keyspace.KindInvalid {
				key.implication.TriggerOther, ok = mapKey(key.implication.TriggerOther)
			}
		}
		if ok {
			key.implication.Target, ok = mapKey(key.implication.Target)
		}
	}
	if !ok {
		return CoordinateKey{}, false
	}
	return key, true
}

// CoordinateDefault returns the exact omitted-coordinate scalar under
// skeleton. Reachable must carriers omit to Top/absent. Bottom carriers omit to
// scalar Bottom: present product.Bottom for maps and present membership for
// sets. This dependency is essential for Bottom join finite == finite.
func CoordinateDefault(skeleton CoordinateSkeleton, key CoordinateKey, reg *axis.Registry) CoordinateScalar {
	valueBearing := key.kind == coordinateRefinement || key.kind == coordinateStaticMember
	implicationBearing := key.kind == coordinatePathPresenceImplication
	if !coordinateKindBottom(skeleton, key.kind) {
		return CoordinateScalar{valueBearing: valueBearing, implicationBearing: implicationBearing}
	}
	scalar := CoordinateScalar{present: true, valueBearing: valueBearing, implicationBearing: implicationBearing}
	if valueBearing {
		scalar.value = product.Bottom(reg)
	} else if implicationBearing {
		scalar.clauseBottom = true
	}
	return scalar
}

// CoordinateKeySupported reports whether key belongs to a non-Bottom
// sub-carrier of skeleton.
func CoordinateKeySupported(skeleton CoordinateSkeleton, key CoordinateKey) bool {
	return !coordinateKindBottom(skeleton, key.kind)
}

// CoordinateScalarValid checks the scalar tag against its owning key and the
// registry provenance of a present product value.
func CoordinateScalarValid(key CoordinateKey, scalar CoordinateScalar, reg *axis.Registry) bool {
	wantsValue := key.kind == coordinateRefinement || key.kind == coordinateStaticMember
	wantsImplication := key.kind == coordinatePathPresenceImplication
	if scalar.valueBearing != wantsValue || scalar.implicationBearing != wantsImplication || scalar.valueBearing && scalar.implicationBearing {
		return false
	}
	if !scalar.present {
		return len(scalar.clauses) == 0 && !scalar.clauseBottom
	}
	if wantsValue {
		return reg != nil && product.BelongsToRegistry(reg, scalar.value)
	}
	if wantsImplication && scalar.clauseBottom {
		return len(scalar.clauses) == 0
	}
	if !wantsImplication || reg == nil || len(scalar.clauses) == 0 || !implicationClausesCanonical(scalar.clauses) {
		return !wantsImplication && len(scalar.clauses) == 0
	}
	for _, clause := range scalar.clauses {
		if key.implication.HasTriggerValue && !product.BelongsToRegistry(reg, clause.trigger) ||
			key.implication.HasTargetValue && !product.BelongsToRegistry(reg, clause.target) {
			return false
		}
	}
	return true
}

// CoordinateScalarBelongsTo reports target-registry provenance independently
// of a key, for cross-ProductDomain scalar import.
func CoordinateScalarBelongsTo(scalar CoordinateScalar, reg *axis.Registry) bool {
	if !scalar.present {
		return true
	}
	if scalar.valueBearing {
		return reg != nil && product.BelongsToRegistry(reg, scalar.value)
	}
	if !scalar.implicationBearing {
		return true
	}
	if scalar.clauseBottom {
		return len(scalar.clauses) == 0
	}
	if reg == nil || !implicationClausesCanonical(scalar.clauses) {
		return false
	}
	for _, clause := range scalar.clauses {
		if clause.trigger != (product.Value{}) && !product.BelongsToRegistry(reg, clause.trigger) ||
			clause.target != (product.Value{}) && !product.BelongsToRegistry(reg, clause.target) {
			return false
		}
	}
	return true
}

// CoordinateScalarHash is consistent with CoordinateScalarEqual.
func CoordinateScalarHash(reg *axis.Registry, scalar CoordinateScalar) uint64 {
	hash := uint64(1469598103934665603)
	mix := func(value uint64) { hash = (hash ^ value) * 1099511628211 }
	if scalar.present {
		mix(1)
	}
	if scalar.valueBearing {
		mix(2)
		if scalar.present {
			mix(product.Hash(reg, scalar.value))
		}
	}
	if scalar.implicationBearing {
		mix(4)
		if scalar.clauseBottom {
			mix(8)
		}
		for _, clause := range scalar.clauses {
			mix(product.CanonicalHash(clause.trigger))
			mix(product.CanonicalHash(clause.target))
		}
	}
	return hash
}

func CoordinateScalarEqual(reg *axis.Registry, a, b CoordinateScalar) bool {
	assertCoordinateScalarPair(a, b)
	if a.present != b.present {
		return false
	}
	if !a.present {
		return true
	}
	if a.valueBearing {
		return product.Equal(reg, a.value, b.value)
	}
	return !a.implicationBearing || a.clauseBottom == b.clauseBottom && implicationClauseSlicesEqual(reg, a.clauses, b.clauses)
}

func CoordinateScalarLessOrEq(reg *axis.Registry, a, b CoordinateScalar) bool {
	assertCoordinateScalarPair(a, b)
	if !a.present {
		return !b.present
	}
	if !b.present {
		return true
	}
	if a.valueBearing {
		return product.LessOrEq(reg, a.value, b.value)
	}
	if !a.implicationBearing {
		return true
	}
	if a.clauseBottom {
		return true
	}
	if b.clauseBottom {
		return false
	}
	return implicationClausesContain(reg, a.clauses, b.clauses)
}

func CoordinateScalarJoin(reg *axis.Registry, a, b CoordinateScalar) CoordinateScalar {
	assertCoordinateScalarPair(a, b)
	if !a.present || !b.present {
		return CoordinateScalar{valueBearing: a.valueBearing, implicationBearing: a.implicationBearing}
	}
	if a.valueBearing {
		a.value = product.Join(reg, a.value, b.value)
	} else if a.implicationBearing {
		if a.clauseBottom {
			return b
		}
		if b.clauseBottom {
			return a
		}
		a.clauses = intersectImplicationClauses(reg, a.clauses, b.clauses)
		if len(a.clauses) == 0 {
			return CoordinateScalar{implicationBearing: true}
		}
	}
	return a
}

func CoordinateScalarMeet(reg *axis.Registry, a, b CoordinateScalar) CoordinateScalar {
	assertCoordinateScalarPair(a, b)
	if !a.present {
		return b
	}
	if !b.present {
		return a
	}
	if a.valueBearing {
		a.value = product.Meet(reg, a.value, b.value)
	} else if a.implicationBearing {
		if a.clauseBottom || b.clauseBottom {
			return CoordinateScalar{present: true, implicationBearing: true, clauseBottom: true}
		}
		a.clauses = unionImplicationClauses(a.clauses, b.clauses)
	}
	return a
}

func CoordinateScalarWiden(reg *axis.Registry, previous, next CoordinateScalar) CoordinateScalar {
	assertCoordinateScalarPair(previous, next)
	if !previous.present || !next.present {
		return CoordinateScalar{valueBearing: previous.valueBearing, implicationBearing: previous.implicationBearing}
	}
	if previous.valueBearing {
		previous.value = product.Widen(reg, previous.value, next.value)
	} else if previous.implicationBearing {
		if previous.clauseBottom {
			return next
		}
		if next.clauseBottom {
			return previous
		}
		previous.clauses = intersectImplicationClauses(reg, previous.clauses, next.clauses)
		if len(previous.clauses) == 0 {
			return CoordinateScalar{implicationBearing: true}
		}
	}
	return previous
}

// CoordinateScalarNarrow matches the current forward-only PathEvidence lane:
// publication retains the stabilized ascending scalar.
func CoordinateScalarNarrow(_ *axis.Registry, previous, next CoordinateScalar) CoordinateScalar {
	assertCoordinateScalarPair(previous, next)
	return previous
}

func assertCoordinateScalarPair(a, b CoordinateScalar) {
	if a.valueBearing != b.valueBearing || a.implicationBearing != b.implicationBearing {
		panic("pathevidence: coordinate scalar kind mismatch")
	}
}

// CoordinateKeyTouches reports whether boundary replacement owns any path in
// this coordinate's complete coupled fact.
func CoordinateKeyTouches(key CoordinateKey, touches func(keyspace.Key) bool) bool {
	if touches == nil {
		return false
	}
	switch key.kind {
	case coordinateRefinement, coordinateStaticMember:
		return touches(key.path)
	case coordinateBranchProof:
		return touches(key.proof.Path) || touches(key.proof.Other)
	case coordinatePathPresenceImplication:
		value := key.implication
		return touches(value.Trigger) || touches(value.TriggerOther) || touches(value.Target)
	default:
		return false
	}
}

// ApplyCoordinateSkeletonBoundary is the exact Bottom-marker part of
// Lane.ApplyBoundary: either Bottom operand makes the corresponding must
// sub-carrier Bottom.
func ApplyCoordinateSkeletonBoundary(destination, fragment CoordinateSkeleton) CoordinateSkeleton {
	return CoordinateSkeleton{
		refinementsBottom:              destination.refinementsBottom || fragment.refinementsBottom,
		staticMembersBottom:            destination.staticMembersBottom || fragment.staticMembersBottom,
		proofsBottom:                   destination.proofsBottom || fragment.proofsBottom,
		pathPresenceImplicationsBottom: destination.pathPresenceImplicationsBottom || fragment.pathPresenceImplicationsBottom,
	}
}

// ApplyCoordinateScalarBoundary selects the exact owner of one output
// coordinate after closure replacement.
func ApplyCoordinateScalarBoundary(key CoordinateKey, destination, fragment CoordinateScalar, touches func(keyspace.Key) bool) CoordinateScalar {
	if fragment.present {
		return fragment
	}
	if CoordinateKeyTouches(key, touches) {
		return fragment
	}
	return destination
}

// ApplyCoordinateRoots applies path-root writes without rebuilding Lane. It
// clears all four Bottom markers atomically and returns a canonical inventory.
func ApplyCoordinateRoots(skeleton CoordinateSkeleton, entries []CoordinateEntry, reg *axis.Registry, ks *keyspace.KeySpace, roots []CoordinateRoot, establishesReachability bool) (CoordinateSkeleton, []CoordinateEntry, bool) {
	if reg == nil || ks == nil || !ks.Valid() {
		return CoordinateSkeleton{}, nil, false
	}
	byKey := make(map[CoordinateKey]CoordinateScalar, len(entries)+len(roots))
	for _, entry := range entries {
		if coordinateKindBottom(skeleton, entry.Key.kind) {
			continue
		}
		byKey[entry.Key] = entry.Scalar
	}
	bottom := product.Bottom(reg)
	for _, root := range roots {
		key := CoordinateKey{kind: coordinateRefinement, path: root.Path}
		value := root.Value
		if !CoordinateKeyValid(key, ks, reg) || !product.BelongsToRegistry(reg, value) {
			return CoordinateSkeleton{}, nil, false
		}
		if product.Equal(reg, value, bottom) {
			delete(byKey, key)
		} else {
			byKey[key] = CoordinateScalar{present: true, valueBearing: true, value: value}
		}
	}
	if establishesReachability || len(roots) != 0 {
		skeleton = skeleton.Reachable()
	}
	out := make([]CoordinateEntry, 0, len(byKey))
	for key, scalar := range byKey {
		out = append(out, CoordinateEntry{Key: key, Scalar: scalar})
	}
	sort.Slice(out, func(i, j int) bool { return CoordinateKeyLess(out[i].Key, out[j].Key, ks) })
	return skeleton, out, true
}

func coordinateKindBottom(skeleton CoordinateSkeleton, kind CoordinateKind) bool {
	switch kind {
	case coordinateRefinement:
		return skeleton.refinementsBottom
	case coordinateStaticMember:
		return skeleton.staticMembersBottom
	case coordinateBranchProof:
		return skeleton.proofsBottom
	case coordinatePathPresenceImplication:
		return skeleton.pathPresenceImplicationsBottom
	default:
		return true
	}
}
