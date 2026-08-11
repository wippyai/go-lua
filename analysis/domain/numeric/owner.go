package numeric

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

type keyRow struct {
	root Root
	ref  RootRef
}

type scalarPair struct {
	leftZero, rightZero bool
	left, right         Scalar
}

type literalIdentity struct {
	integer bool
	whole   int64
	float   uint64
}

// New seals one homogeneous Numeric/equality family from Link's exact finite
// structural ranges. No caller supplies an ordinal, capacity, pair, or
// threshold vocabulary.
func New(source *link.Link) (*Algebra, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	if !binaryPrimitivesAvailable(source) {
		return nil, false
	}
	algebra := &Algebra{
		source: source, linkID: source.ContentID(),
		keyIndex: make(map[RootRef]uint32), scalarAtoms: make(map[Scalar]Atom),
	}
	support, pairs, thresholds, supportOK := collectSupport(source)
	if !supportOK || uint64(len(support.scalars)+1) > uint64(math.MaxUint32) {
		return nil, false
	}
	// Slot one is the mathematical zero atom. It is a domain constant, never a
	// fabricated Boundary Value. Every remaining slot is one exact neutral scalar
	// occurrence.
	if !seal(algebra, len(support.scalars)+1) {
		return nil, false
	}
	algebra.atomScalars = make([]Scalar, len(algebra.atoms))
	algebra.atomScalarRefs = make([]scalarRef, len(algebra.atoms))
	algebra.atomLiterals = make([]numericLiteral, len(algebra.atoms))
	algebra.atomLiterals[0] = numericLiteral{kind: literalInteger}
	algebra.literalAtoms = append(algebra.literalAtoms, 1)
	for index, scalar := range support.scalars {
		atom := algebra.atoms[index+1]
		algebra.atomScalars[index+1] = scalar
		algebra.atomScalarRefs[index+1] = support.refs[index]
		algebra.scalarAtoms[scalar] = atom
		integer, number, isInteger, literal := scalarNumberLiteral(source, scalar)
		if literal && isInteger {
			algebra.atomLiterals[index+1] = numericLiteral{kind: literalInteger, integer: integer}
			algebra.literalAtoms = append(algebra.literalAtoms, atom.slot)
		} else if literal {
			algebra.atomLiterals[index+1] = numericLiteral{kind: literalFloat, float: number}
			algebra.literalAtoms = append(algebra.literalAtoms, atom.slot)
		}
	}
	sealedTemplates := make([]template, 0, len(pairs))
	for _, pair := range pairs {
		left, leftOK := algebra.atomFor(pair.leftZero, pair.left)
		right, rightOK := algebra.atomFor(pair.rightZero, pair.right)
		if !leftOK || !rightOK {
			return nil, false
		}
		sealedTemplates = append(sealedTemplates, template{Left: left, Right: right, Thresholds: thresholds[pair]})
	}
	// Re-seal the pair-dependent structures now that opaque Atom handles exist.
	algebra.pairs = nil
	algebra.thresholds = nil
	algebra.pairIndex = nil
	algebra.components = nil
	if !sealPairs(algebra, sealedTemplates) {
		return nil, false
	}
	if !algebra.buildKeys(support.roots) {
		return nil, false
	}
	algebra.fingerprint = algebra.schemaHash()
	algebra.content = numericContentID(algebra.linkID)
	algebra.bottom = Value{algebra: algebra, bottom: true}
	algebra.initScratch()
	algebra.defaultValue, supportOK = algebra.normalize(Value{algebra: algebra})
	if !supportOK {
		return nil, false
	}
	return algebra, algebra.Valid()
}

func sealPairs(owner *Algebra, templates []template) bool {
	owner.pairIndex = make(map[pairIndex]int, len(templates))
	canonical := append([]template(nil), templates...)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Left.slot != canonical[j].Left.slot {
			return canonical[i].Left.slot < canonical[j].Left.slot
		}
		return canonical[i].Right.slot < canonical[j].Right.slot
	})
	for _, item := range canonical {
		if !item.Left.Valid() || !item.Right.Valid() || item.Left.owner != owner || item.Right.owner != owner {
			return false
		}
		chain, ok := sealThresholds(item.Thresholds)
		if !ok {
			return false
		}
		key := pairIndex{left: item.Left, right: item.Right}
		if _, duplicate := owner.pairIndex[key]; duplicate {
			return false
		}
		if uint64(len(owner.pairs)) == uint64(math.MaxUint32) {
			return false
		}
		owner.pairIndex[key] = len(owner.pairs)
		owner.pairs = append(owner.pairs, pairRow{left: item.Left, right: item.Right})
		owner.thresholds = append(owner.thresholds, chain)
	}
	owner.components = sealComponents(owner)
	return true
}

func (algebra *Algebra) initScratch() {
	algebra.scratch.New = func() any {
		return &scratch{
			masks: make([]Eligibility, len(algebra.atoms)), equal: make([]uint64, wordCount(len(algebra.pairs))),
			unequal: make([]uint64, wordCount(len(algebra.pairs))), integral: make([]uint64, wordCount(len(algebra.pairs))), bounds: make([]uint16, len(algebra.pairs)),
			parents: make([]int, len(algebra.atoms)), integralClass: make([]bool, len(algebra.atoms)), literalClass: make([]int, len(algebra.atoms)), originalEqual: make([]uint64, wordCount(len(algebra.pairs))),
			originalUnequal: make([]uint64, wordCount(len(algebra.pairs))), originalBounds: make([]uint16, len(algebra.pairs)),
			originalIntegral: make([]uint64, wordCount(len(algebra.pairs))),
			classPairs:       make([]classPair, 0, len(algebra.pairs)), distance: make([]int64, len(algebra.atoms)),
			potential: make([]int64, len(algebra.atoms)), edges: make([]edge, 0, len(algebra.pairs)),
			activeRoots: make([]int, 0, len(algebra.atoms)), heap: make([]heapEntry, 0, len(algebra.pairs)),
		}
	}
}

type scalarSupport struct {
	scalars []Scalar
	refs    []scalarRef
	roots   map[RootRef][]Scalar
}

// scalarRef is Scalar's replay form. Its coordinates are Program-owned; the
// Link identity merely fences replay to this sealed project.
type scalarRef struct {
	root RootRef
	term keyspace.Term
}

type binaryTermView interface {
	Count() int
	At(index int) (keyspace.Term, bool)
}

type bodyScalarIndex struct {
	parents []uint32
	groups  [][]Scalar
}

// appendAncestorGroups appends the direct scalar groups on one body's lexical
// chain. The dense ordinal walk is also the construction proof: every visit
// advances to a validated parent, and no malformed chain can exceed the body
// denominator.
func appendAncestorGroups(dst []Scalar, body keyspace.Term, parents []uint32, groups [][]Scalar) ([]Scalar, int, bool) {
	if len(parents) == 0 || len(parents) != len(groups) || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return dst, 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	bodyCount := uint32(len(parents) - 1)
	if ordinal == 0 || ordinal > bodyCount {
		return dst, 0, false
	}
	start := len(dst)
	visits := 0
	current := ordinal
	for current != 0 {
		if visits >= int(bodyCount) || current > bodyCount {
			return dst[:start], visits, false
		}
		dst = append(dst, groups[current]...)
		visits++
		parent := parents[current]
		if parent == current || parent > bodyCount {
			return dst[:start], visits, false
		}
		current = parent
	}
	return dst, visits, true
}

func primitiveBinaryOperator(op flowkind.BinaryOp) bool {
	switch op {
	case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
		flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow,
		flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
		flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight,
		flowkind.BinaryEqual, flowkind.BinaryNotEqual,
		flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater, flowkind.BinaryGreaterEqual:
		return true
	default:
		return false
	}
}

func primitiveScalar(p *program.Program, source *link.Link, shard linkproject.Shard, body, term keyspace.Term, known map[Scalar]Scalar) (Scalar, bool) {
	if p == nil || source == nil || body == 0 || term == 0 {
		return Scalar{}, false
	}
	positionedBody, _, _, positioned := p.Source().Index().Position(term)
	if !positioned || positionedBody != body || !p.Flow().Executable().Contains(term) {
		return Scalar{}, false
	}
	scalar := Scalar{source: source, shard: shard, body: body, term: term}
	actual, present := known[scalar]
	return actual, present
}

// collectSupport consumes Boundary's exact Value/Origin projection together
// with the mounted Source/Flow views. Every declared edge is
// constant arity; roots are not turned into a quadratic all-pairs clique, and
// no authored or candidate Binary bucket is rescanned.
func collectSupport(source *link.Link) (scalarSupport, []scalarPair, map[scalarPair][]int64, bool) {
	project := source.Project()
	if project == nil || source.Boundary() == nil {
		return scalarSupport{}, nil, nil, false
	}
	mounts := project.Mounts()
	boundaryValues := source.Boundary().Values()
	support := scalarSupport{roots: make(map[RootRef][]Scalar)}
	// Each mounted shard owns a dense body-ordinal index of direct scalar
	// occurrences. The index is seal-local; roots later walk only their body
	// parent chain and append each direct group once.
	bodyIndexes := make(map[linkproject.Shard]bodyScalarIndex, mounts.Count())
	pairSet := make(map[scalarPair][]int64)
	literals := make(map[literalIdentity]Scalar)
	scalarByOccurrence := make(map[Scalar]Scalar)
	add := func(pair scalarPair, extras ...int64) {
		pairSet[pair] = append(pairSet[pair], append([]int64{-1, 0, 1}, extras...)...)
	}
	addBoth := func(left, right Scalar) {
		add(scalarPair{left: left, right: right})
		if left != right {
			add(scalarPair{left: right, right: left})
		}
	}
	add(scalarPair{leftZero: true, rightZero: true})
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		projectShard, ok := mounts.At(shardIndex)
		if !ok {
			return scalarSupport{}, nil, nil, false
		}
		p, ok := mounts.Program(projectShard)
		if !ok || p == nil {
			return scalarSupport{}, nil, nil, false
		}
		shard := projectShard
		canonicalIndex, shardOK := mounts.Index(shard)
		if !shardOK || uint64(canonicalIndex+1) > uint64(math.MaxUint32) {
			return scalarSupport{}, nil, nil, false
		}
		shardOrdinal := uint32(canonicalIndex + 1)
		sourceView, flowView := p.Source(), p.Flow()
		bodyCount := sourceView.Identity().FamilyCount(keyspace.FamilyBody)
		parents := make([]uint32, bodyCount+1)
		groups := make([][]Scalar, bodyCount+1)
		for index := 0; index < bodyCount; index++ {
			body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
			parent, hasParent := sourceView.Index().BodyParent(body)
			if !hasParent {
				if parent != 0 {
					return scalarSupport{}, nil, nil, false
				}
			} else if keyspace.TermFamily(parent) != keyspace.FamilyBody || keyspace.TermOrdinal(parent) == 0 || keyspace.TermOrdinal(parent) > uint32(bodyCount) || parent == body {
				return scalarSupport{}, nil, nil, false
			} else {
				parents[index+1] = keyspace.TermOrdinal(parent)
			}
			if flowView.Executable().Contains(body) {
				support.roots[RootRef{linkID: source.ContentID(), shardOrdinal: shardOrdinal, body: body}] = nil
			}
		}
		bodyIndexes[shard] = bodyScalarIndex{parents: parents, groups: groups}
	}
	for index := 0; index < boundaryValues.Count(); index++ {
		value, ok := boundaryValues.At(index)
		if !ok {
			return scalarSupport{}, nil, nil, false
		}
		shard, term, ok := boundaryValues.Origin(value)
		if !ok {
			return scalarSupport{}, nil, nil, false
		}
		if _, projectOK := mounts.Index(shard); !projectOK {
			return scalarSupport{}, nil, nil, false
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil || !p.Flow().Executable().Contains(term) {
			continue
		}
		body, _, _, ok := p.Source().Index().Position(term)
		if !ok || !p.Flow().Executable().Contains(body) {
			continue
		}
		scalar := Scalar{source: source, shard: shard, body: body, term: term}
		canonicalIndex, shardOK := mounts.Index(shard)
		if !shardOK || uint64(canonicalIndex+1) > uint64(math.MaxUint32) {
			return scalarSupport{}, nil, nil, false
		}
		ref := RootRef{linkID: source.ContentID(), shardOrdinal: uint32(canonicalIndex + 1), body: body}
		if _, exists := support.roots[ref]; !exists {
			continue
		}
		bodyOrdinal := keyspace.TermOrdinal(body)
		bodyIndex, indexed := bodyIndexes[shard]
		groups := bodyIndex.groups
		if keyspace.TermFamily(body) != keyspace.FamilyBody || bodyOrdinal == 0 || !indexed || int(bodyOrdinal) >= len(groups) {
			return scalarSupport{}, nil, nil, false
		}
		support.scalars = append(support.scalars, scalar)
		groups[bodyOrdinal] = append(groups[bodyOrdinal], scalar)
		scalarByOccurrence[scalar] = scalar
		support.refs = append(support.refs, scalarRef{root: ref, term: term})
		add(scalarPair{left: scalar, right: scalar})
		if integer, ok := scalarIntegralLiteral(source, scalar); ok {
			add(scalarPair{left: scalar, rightZero: true}, integer)
			if integer != math.MinInt64 {
				add(scalarPair{leftZero: true, right: scalar}, -integer)
			}
		}
		if integer, number, isInteger, ok := scalarNumberLiteral(source, scalar); ok {
			identity := literalIdentity{integer: isInteger, whole: integer, float: math.Float64bits(number)}
			if prior, seen := literals[identity]; seen {
				addBoth(prior, scalar)
			} else {
				literals[identity] = scalar
			}
		}
	}
	// A body observes the exact scalar occurrences owned by its lexical
	// ancestors as well as its own direct Source.Index position. This is the
	// finite root-local support needed by branch bodies; it introduces no new
	// Body identity and never promotes a non-executable/rootless body.
	for ref := range support.roots {
		projectShard, shardOK := mounts.At(int(ref.shardOrdinal) - 1)
		if !shardOK {
			return scalarSupport{}, nil, nil, false
		}
		bodyIndex, indexed := bodyIndexes[projectShard]
		if !indexed {
			return scalarSupport{}, nil, nil, false
		}
		var ok bool
		support.roots[ref], _, ok = appendAncestorGroups(support.roots[ref], ref.body, bodyIndex.parents, bodyIndex.groups)
		if !ok {
			return scalarSupport{}, nil, nil, false
		}
	}
	// Primitive binary rows are the sealed Numeric pair authority. Their raw
	// operation operands remain distinct from Comparison's normalized view;
	// both are projected here so every direct consumer can obtain an existing
	// pair without reopening authored or candidate buckets.
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, ok := mounts.At(shardIndex)
		if !ok {
			return scalarSupport{}, nil, nil, false
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			return scalarSupport{}, nil, nil, false
		}
		primitives := p.Flow().BinaryPrimitives()
		if !primitives.Available() {
			return scalarSupport{}, nil, nil, false
		}
		consume := func(view binaryTermView) bool {
			for index := 0; index < view.Count(); index++ {
				binary, present := view.At(index)
				if !present {
					return false
				}
				primitive, retained := primitives.Primitive(binary)
				operation, operationOK := primitive.Operation()
				result, sourceOK := primitive.Source()
				if !retained || !operationOK || !sourceOK || result != binary ||
					!primitiveBinaryOperator(operation.Op) ||
					keyspace.TermFamily(operation.Owner) != keyspace.FamilyBody ||
					keyspace.TermOrdinal(operation.Owner) == 0 ||
					!p.Flow().Executable().Contains(operation.Owner) {
					return false
				}
				resultScalar, resultOK := primitiveScalar(p, source, shard, operation.Owner, binary, scalarByOccurrence)
				leftScalar, leftOK := primitiveScalar(p, source, shard, operation.Owner, operation.Left, scalarByOccurrence)
				rightScalar, rightOK := primitiveScalar(p, source, shard, operation.Owner, operation.Right, scalarByOccurrence)
				if !resultOK || !leftOK || !rightOK {
					return false
				}
				addBoth(resultScalar, leftScalar)
				addBoth(resultScalar, rightScalar)
				addBoth(leftScalar, rightScalar)
				if comparison, comparisonOK := primitive.Comparison(); comparisonOK {
					if keyspace.TermFamily(comparison.Branch) != keyspace.FamilyBranch || keyspace.TermOrdinal(comparison.Branch) == 0 || !p.Flow().Executable().Contains(comparison.Branch) ||
						keyspace.TermFamily(comparison.TrueBody) != keyspace.FamilyBody || keyspace.TermOrdinal(comparison.TrueBody) == 0 ||
						keyspace.TermFamily(comparison.FalseBody) != keyspace.FamilyBody || keyspace.TermOrdinal(comparison.FalseBody) == 0 ||
						!p.Flow().Executable().Contains(comparison.TrueBody) || !p.Flow().Executable().Contains(comparison.FalseBody) {
						return false
					}
					comparisonLeft, leftPositioned := primitiveScalar(p, source, shard, operation.Owner, comparison.Left, scalarByOccurrence)
					comparisonRight, rightPositioned := primitiveScalar(p, source, shard, operation.Owner, comparison.Right, scalarByOccurrence)
					if !leftPositioned || !rightPositioned {
						return false
					}
					addBoth(comparisonLeft, comparisonRight)
				}
				// Exact authored integer +/- constants get their finite
				// translation thresholds at seal time; dynamic and floating
				// arithmetic retains only the fixed relation vocabulary.
				if operation.Op == flowkind.BinaryAdd || operation.Op == flowkind.BinarySub {
					if integer, _, isInteger, literal := scalarNumberLiteral(source, rightScalar); literal && isInteger {
						delta := integer
						if operation.Op == flowkind.BinarySub {
							if integer == math.MinInt64 {
								continue
							}
							delta = -integer
						}
						add(scalarPair{left: resultScalar, right: leftScalar}, delta)
						if resultScalar != leftScalar {
							add(scalarPair{left: leftScalar, right: resultScalar}, -delta)
						}
					}
				}
			}
			return true
		}
		if !consume(primitives.Arithmetic()) || !consume(primitives.Bitwise()) ||
			!consume(primitives.Equality()) || !consume(primitives.Order()) {
			return scalarSupport{}, nil, nil, false
		}
	}
	pairs := make([]scalarPair, 0, len(pairSet))
	for pair := range pairSet {
		pairs = append(pairs, pair)
	}
	return support, pairs, pairSet, true
}

func scalarNumberLiteral(source *link.Link, scalar Scalar) (integer int64, number float64, isInteger, ok bool) {
	project := source.Project()
	if project == nil {
		return 0, 0, false, false
	}
	mounts := project.Mounts()
	if _, ok := mounts.Index(scalar.shard); !ok {
		return 0, 0, false, false
	}
	p, present := mounts.Program(scalar.shard)
	if !present || p == nil {
		return 0, 0, false, false
	}
	literals := p.Source().Literals()
	ordinal := keyspace.TermOrdinal(scalar.term)
	if ordinal == 0 {
		return 0, 0, false, false
	}
	switch keyspace.TermFamily(scalar.term) {
	case keyspace.FamilyInteger:
		term, owner, value, present := literals.Integers().At(int(ordinal - 1))
		if present && term == scalar.term && owner == scalar.body {
			return value, 0, true, true
		}
	case keyspace.FamilyFloat:
		term, owner, bits, present := literals.Floats().At(int(ordinal - 1))
		if present && term == scalar.term && owner == scalar.body {
			return 0, math.Float64frombits(bits), false, true
		}
	}
	return 0, 0, false, false
}

func binaryPrimitivesAvailable(source *link.Link) bool {
	if source == nil || source.Project() == nil {
		return false
	}
	mounts := source.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			return false
		}
		program, ok := mounts.Program(shard)
		if !ok || program == nil || !program.Flow().BinaryPrimitives().Available() {
			return false
		}
	}
	return mounts.Count() != 0
}

func scalarIntegralLiteral(source *link.Link, scalar Scalar) (int64, bool) {
	integer, number, isInteger, ok := scalarNumberLiteral(source, scalar)
	if !ok {
		return 0, false
	}
	if isInteger {
		return integer, true
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number ||
		number < -9223372036854775808.0 || number >= 9223372036854775808.0 {
		return 0, false
	}
	return int64(number), true
}

func (algebra *Algebra) atomFor(zero bool, scalar Scalar) (Atom, bool) {
	if zero {
		return algebra.atoms[0], true
	}
	atom, ok := algebra.scalarAtoms[scalar]
	return atom, ok
}

func (algebra *Algebra) buildKeys(rootSupport map[RootRef][]Scalar) bool {
	pairAdjacency := make([][]uint32, len(algebra.atoms)+1)
	for index, pair := range algebra.pairs {
		slot := uint32(index + 1)
		pairAdjacency[pair.left.slot] = append(pairAdjacency[pair.left.slot], slot)
		if pair.right != pair.left {
			pairAdjacency[pair.right.slot] = append(pairAdjacency[pair.right.slot], slot)
		}
	}
	refs := make([]RootRef, 0, len(rootSupport))
	for ref := range rootSupport {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].shardOrdinal != refs[j].shardOrdinal {
			return refs[i].shardOrdinal < refs[j].shardOrdinal
		}
		return refs[i].body < refs[j].body
	})
	for _, ref := range refs {
		projectShard, shardOK := algebra.source.Project().Mounts().At(int(ref.shardOrdinal) - 1)
		if !shardOK {
			return false
		}
		root := Root{source: algebra.source, shard: projectShard, body: ref.body}
		if algebra.keyIndex[ref] != 0 || uint64(len(algebra.keys)) == uint64(math.MaxUint32) {
			return false
		}
		atoms := []uint32{1} // the sole mathematical zero
		for _, scalar := range rootSupport[ref] {
			atom, found := algebra.scalarAtoms[scalar]
			if !found {
				return false
			}
			atoms = append(atoms, atom.slot)
		}
		sort.Slice(atoms, func(i, j int) bool { return atoms[i] < atoms[j] })
		atoms = uniqueUint32(atoms)
		pairs := make([]uint32, 0)
		for _, atomSlot := range atoms {
			for _, pairSlot := range pairAdjacency[atomSlot] {
				pair := algebra.pairs[pairSlot-1]
				if containsUint32(atoms, pair.left.slot) && containsUint32(atoms, pair.right.slot) {
					pairs = append(pairs, pairSlot)
				}
			}
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i] < pairs[j] })
		pairs = uniqueUint32(pairs)
		algebra.keys = append(algebra.keys, keyRow{root: root, ref: ref})
		algebra.keyAtoms = append(algebra.keyAtoms, atoms)
		algebra.keyPairs = append(algebra.keyPairs, pairs)
		algebra.keyIndex[ref] = uint32(len(algebra.keys))
	}
	return true
}

func (algebra *Algebra) Valid() bool {
	return algebra != nil && algebra.source != nil && algebra.linkID.Available() && algebra.content.Available()
}

// Link returns the exact sealed Link that issued this Algebra's coordinates.
// It is an ownership fence, not a replay identity: independently sealed
// same-content Links remain distinct live authorities and cannot be mixed with
// this Algebra's operands.  Replay continues to use LinkID and the immutable
// Algebra tables below.
func (algebra *Algebra) Link() *link.Link {
	if !algebra.Valid() {
		return nil
	}
	return algebra.source
}

func (algebra *Algebra) ContentID() keyspace.ContentID {
	if !algebra.Valid() {
		return keyspace.ContentID{}
	}
	return algebra.content
}
func (algebra *Algebra) KeyCount() int {
	if !algebra.Valid() {
		return 0
	}
	return len(algebra.keys)
}
func (algebra *Algebra) KeyAt(index int) (Key, bool) {
	if !algebra.Valid() || index < 0 || index >= len(algebra.keys) {
		return Key{}, false
	}
	return Key{owner: algebra, slot: uint32(index + 1)}, true
}
func (algebra *Algebra) RootFor(shard linkproject.Shard, body keyspace.Term) (Root, bool) {
	if !algebra.Valid() || body == 0 {
		return Root{}, false
	}
	project := algebra.source.Project()
	if project == nil {
		return Root{}, false
	}
	mounts := project.Mounts()
	if _, ok := mounts.Index(shard); !ok {
		return Root{}, false
	}
	p, ok := mounts.Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(body) {
		return Root{}, false
	}
	root := Root{source: algebra.source, shard: shard, body: body}
	_, ok = algebra.KeyFor(root)
	return root, ok
}

func (algebra *Algebra) ScalarFor(shard linkproject.Shard, body, term keyspace.Term) (Scalar, bool) {
	if !algebra.Valid() || body == 0 || term == 0 {
		return Scalar{}, false
	}
	project := algebra.source.Project()
	if project == nil {
		return Scalar{}, false
	}
	mounts := project.Mounts()
	if _, ok := mounts.Index(shard); !ok {
		return Scalar{}, false
	}
	p, ok := mounts.Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(body) || !p.Flow().Executable().Contains(term) {
		return Scalar{}, false
	}
	positionedBody, _, _, positioned := p.Source().Index().Position(term)
	if !positioned || positionedBody != body {
		return Scalar{}, false
	}
	scalar := Scalar{source: algebra.source, shard: shard, body: body, term: term}
	_, ok = algebra.AtomFor(scalar)
	return scalar, ok
}

func (algebra *Algebra) KeyFor(root Root) (Key, bool) {
	if !algebra.Valid() || root.source != algebra.source {
		return Key{}, false
	}
	if _, ok := algebra.source.Project().Mounts().Index(root.shard); !ok {
		return Key{}, false
	}
	index, ok := algebra.source.Project().Mounts().Index(root.shard)
	if !ok || uint64(index+1) > uint64(math.MaxUint32) {
		return Key{}, false
	}
	ref := RootRef{linkID: algebra.linkID, shardOrdinal: uint32(index + 1), body: root.body}
	key := Key{owner: algebra, slot: algebra.keyIndex[ref]}
	return key, key.Valid()
}
func (algebra *Algebra) validKey(key Key) bool {
	return algebra != nil && key.owner == algebra && key.slot != 0 && uint64(key.slot) <= uint64(len(algebra.keys))
}
func (algebra *Algebra) Zero() (Atom, bool) {
	if !algebra.Valid() {
		return Atom{}, false
	}
	return algebra.atoms[0], true
}

// AtomFor returns the exact sealed atom for one Program scalar occurrence.
func (algebra *Algebra) AtomFor(scalar Scalar) (Atom, bool) {
	if !algebra.Valid() {
		return Atom{}, false
	}
	atom, ok := algebra.scalarAtoms[scalar]
	return atom, ok
}

func (algebra *Algebra) Pair(left, right Atom) (Pair, bool) {
	if !algebra.Valid() || left.owner != algebra || right.owner != algebra {
		return Pair{}, false
	}
	index, ok := algebra.pairIndex[pairIndex{left: left, right: right}]
	if !ok {
		return Pair{}, false
	}
	return Pair{owner: algebra, slot: uint32(index + 1)}, true
}

func uniqueUint32(values []uint32) []uint32 {
	if len(values) == 0 {
		return values
	}
	write := 1
	for read := 1; read < len(values); read++ {
		if values[read] == values[write-1] {
			continue
		}
		values[write] = values[read]
		write++
	}
	return values[:write]
}

func containsUint32(values []uint32, wanted uint32) bool {
	index := sort.Search(len(values), func(index int) bool { return values[index] >= wanted })
	return index < len(values) && values[index] == wanted
}

func numericContentID(linkID keyspace.ContentID) keyspace.ContentID {
	var payload [56]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x6e756d657269632d)
	binary.BigEndian.PutUint64(payload[40:48], 3)
	binary.BigEndian.PutUint64(payload[48:56], 3) // primitive comparison branch support
	return sha256.Sum256(payload[:])
}
