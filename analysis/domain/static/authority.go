package static

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// evaluatorLaw is part of Static's sealed identity.  Mounted-artifact
// admission is a production contract change, so advance the live evaluator
// law monotonically and use the same constant for every identity projection.
const evaluatorLaw = "wippy.analysis.static/evaluator/v10"

func staticLawContentID(targetID identity.ContentID) (id identity.ContentID) {
	if !targetID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	_, _ = h.Write([]byte(evaluatorLaw))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(targetID[:])
	copy(id[:], h.Sum(nil))
	return id
}

type evaluationKey struct {
	reference typeauthority.StaticTypeRef
	resolver  identity.ContentID
	env       identity.ContentID
	operation target.Operation
}

type evaluationBase struct {
	reference typeauthority.StaticTypeRef
	resolver  identity.ContentID
}

// coordinateKey is the complete, portable meaning of one Static query cell.
// The dense Coordinate that exposes it remains authority-local.
type coordinateKey struct {
	reference   typeauthority.StaticTypeRef
	namespace   identity.ContentID
	environment identity.ContentID
	operation   target.Operation
}

type coordinateRow struct {
	key    coordinateKey
	result Value
}

// Authority is the sole Link-scoped owner of Static judgments and Pack
// classes. It is immutable after Seal. The hot representation is dense exact
// handles projected onto ClassSet's canonical semantic-union descriptors;
// decoded typ graphs are construction-only.
type Authority struct {
	// linkID is the exact mounted owner fence. No Link is retained.
	linkID identity.ContentID
	types  *typeauthority.Authority
	id     identity.ContentID
	target *target.Contract
	lawID  identity.ContentID

	results          []resultRow // 0=bottom, 1=top, semantic values follow.
	closedByBytes    map[string]Value
	symbolicByKey    map[Symbolic]Value
	ranks            []uint64
	valueClasses     []Class
	classCanonical   []Value
	memo             map[evaluationKey]Value
	coordinates      []coordinateRow
	coordinateIndex  map[coordinateKey]uint32
	typeOfOutputs    map[identity.ContentID]Coordinate
	operands         map[identity.ContentID]ContainedOperand
	namespaceIDs     map[identity.ContentID]struct{}
	runtimeKinds     runtimeKindTable
	runtimeKindMask  []runtimekind.Set // exact RuntimeTypeOf masks by Static result.
	runtimeKindValue []bool            // exact RuntimeTypeOf result membership.
	typeArguments    typeArgumentFormalTable

	classes    *ClassSet
	runtime    *typeauthority.Runtime
	typeValues []typeValueRow
	mounts     []MountedArtifact
	valueIDs   map[mountedValueKey]identity.ContentID // construction-only scalar substitution
}

// MountedValueID is the complete scalar Link substitution receipt for one
// artifact semantic occurrence.  It intentionally carries no Boundary Value:
// Static consumes this relation while sealing and retains only its own issued
// result identities thereafter.
type MountedValueID struct {
	ModuleID   identity.ContentID
	SemanticID identity.ContentID
	ValueID    identity.ContentID
}

type mountedValueKey struct {
	module   identity.ContentID
	semantic identity.ContentID
}

// MountContext is the complete Link-local substitution receipt Static needs
// during construction. It contains scalar identity, target semantics, and
// detached mounted-value identities only; it never exposes or retains a Link
// or Boundary authority.
type MountContext struct {
	LinkID   identity.ContentID
	Target   *target.Contract
	ValueIDs []MountedValueID
}

func sealMounted(context MountContext, types *typeauthority.Authority, mounts []MountedArtifact) (*Authority, *typeauthority.Runtime, error) {
	if !context.LinkID.Available() || context.Target == nil || !context.Target.ContentID().Available() || types == nil || types.LinkID() != context.LinkID {
		return nil, nil, errors.New("static: Link/type authority mismatch")
	}
	valueIDs, valuesOK := sealMountedValueIDs(context.ValueIDs)
	if !valuesOK {
		return nil, nil, errors.New("static: malformed mounted-value substitution receipt")
	}
	a := &Authority{
		linkID: context.LinkID, types: types, valueIDs: valueIDs,
		results:       []resultRow{{kind: KindBottom}, {kind: KindTop}},
		closedByBytes: make(map[string]Value), symbolicByKey: make(map[Symbolic]Value),
		memo:            make(map[evaluationKey]Value),
		coordinateIndex: make(map[coordinateKey]uint32), operands: make(map[identity.ContentID]ContainedOperand),
		namespaceIDs: make(map[identity.ContentID]struct{}), mounts: append([]MountedArtifact(nil), mounts...),
	}
	targetID := context.Target.ContentID()
	a.target = context.Target
	a.lawID = staticLawContentID(targetID)
	if !a.lawID.Available() {
		return nil, nil, errors.New("static: unavailable evaluator law identity")
	}
	if err := a.sealHotProjections(context.Target); err != nil {
		return nil, nil, err
	}
	if err := a.sealContainedOperands(); err != nil {
		return nil, nil, err
	}
	if err := a.sealCoordinates(); err != nil {
		return nil, nil, err
	}
	if err := a.sealTypeOfOutputs(); err != nil {
		return nil, nil, err
	}
	if len(mounts) == 0 || !a.sealMountedTypeArgumentFormals() {
		return nil, nil, errors.New("static: malformed call type-argument formal relation")
	}
	runtimeKinds, err := sealRuntimeKinds(a)
	if err != nil {
		return nil, nil, err
	}
	a.runtimeKinds = runtimeKinds
	classes, runtime, err := sealClassSet(a)
	if err != nil {
		return nil, nil, err
	}
	a.classes = classes
	a.runtime = runtime
	for index := range a.results {
		a.results[index].runtime = typeauthority.RuntimeInput{}
	}
	if err := a.sealClassProjection(); err != nil {
		return nil, nil, err
	}
	// Drop every construction-only index. Hot Static retains only semantic
	// result bytes, sealed query coordinates, and finite relations.
	a.types = nil
	if len(mounts) != 0 && !types.DetachConstructionAuthority() {
		return nil, nil, errors.New("static: failed to detach type construction authority")
	}
	a.closedByBytes = nil
	a.symbolicByKey = nil
	a.memo = nil
	a.operands = nil
	a.typeOfOutputs = nil
	a.mounts = nil
	// Mounted-value substitution is construction-only and must not retain any
	// Link coordinate, even indirectly through a capability-bearing wrapper.
	a.valueIDs = nil
	return a, runtime, nil
}

func sealMountedValueIDs(rows []MountedValueID) (map[mountedValueKey]identity.ContentID, bool) {
	sealed := make(map[mountedValueKey]identity.ContentID, len(rows))
	for _, row := range rows {
		if !row.ModuleID.Available() || !row.SemanticID.Available() || !row.ValueID.Available() {
			return nil, false
		}
		key := mountedValueKey{module: row.ModuleID, semantic: row.SemanticID}
		if _, duplicate := sealed[key]; duplicate {
			return nil, false
		}
		sealed[key] = row.ValueID
	}
	return sealed, true
}

// MountedArtifact is the closed Link substitution receipt Static admits at
// its production boundary.  The artifact is immutable and reusable by
// ProgramID; ModuleID is the Link-local mount identity.  Static's inference
// remains the owner of all judgments, while this seam authenticates that the
// mounted rows belong to the exact Program occurrence being linked.
type MountedArtifact struct {
	Artifact    *programartifact.Artifact
	ModuleID    identity.ContentID
	ProgramID   identity.ContentID
	NamespaceID identity.ContentID
}

// SealMountedArtifacts is the production Static ingress.  It admits only
// closed, owner-issued artifacts and rejects duplicate mounts before any
// inference begins.  The old Link-only Seal remains available to construction
// and law fixtures while callers migrate; production analysis must use this
// mounted seam so Static never receives source graph authority.
func SealMountedArtifacts(context MountContext, types *typeauthority.Authority, mounts []MountedArtifact) (*Authority, *typeauthority.Runtime, error) {
	if len(mounts) == 0 {
		return nil, nil, errors.New("static: no mounted artifacts")
	}
	seenModules := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.Artifact == nil || !mount.Artifact.Available() || !mount.ModuleID.Available() || !mount.ProgramID.Available() {
			return nil, nil, errors.New("static: unavailable mounted artifact")
		}
		if mount.Artifact.CompileKey().ProgramID() != mount.ProgramID {
			return nil, nil, errors.New("static: mounted artifact/program mismatch")
		}
		if !mount.NamespaceID.Available() {
			return nil, nil, errors.New("static: unavailable mounted namespace")
		}
		for rowIndex := 0; rowIndex < mount.Artifact.StaticTypeArgumentCount(); rowIndex++ {
			row, rowOK := mount.Artifact.StaticTypeArgumentAt(rowIndex)
			if !rowOK || !row.Available() {
				return nil, nil, errors.New("static: malformed mounted type-argument row")
			}
		}
		for rowIndex := 0; rowIndex < mount.Artifact.StaticTypeValueCount(); rowIndex++ {
			row, rowOK := mount.Artifact.StaticTypeValueAt(rowIndex)
			if !rowOK || !row.Available() {
				return nil, nil, errors.New("static: malformed mounted TypeValue row")
			}
		}
		if _, duplicate := seenModules[mount.ModuleID]; duplicate {
			return nil, nil, errors.New("static: duplicate mounted module")
		}
		seenModules[mount.ModuleID] = struct{}{}
	}
	return sealMounted(context, types, mounts)
}

func (a *Authority) ContentID() identity.ContentID {
	if a == nil {
		return identity.ContentID{}
	}
	return a.id
}

func (a *Authority) LinkID() identity.ContentID {
	if a == nil {
		return identity.ContentID{}
	}
	return a.linkID
}

// Result returns the exact result admitted for one Authority-local contextual
// coordinate. A portable Ref alone is deliberately insufficient: the same
// authored type may be evaluated under several sealed namespaces and rho.
func (a *Authority) Result(coordinate Coordinate) (Value, bool) {
	index, ok := a.CoordinateIndex(coordinate)
	if !ok {
		return Value{}, false
	}
	return a.coordinates[index].result, true
}

// CoordinateCount is the exact authored Static selector family. The result
// algebra may contain closure values and residuals that are not coordinates;
// those never manufacture Factor keys.
func (a *Authority) CoordinateCount() int {
	if a == nil {
		return 0
	}
	return len(a.coordinates)
}

// CoordinateAt traverses authored selectors in typeauthority's canonical Link
// order. An empty Static forest therefore exposes the exact empty range.
func (a *Authority) CoordinateAt(index int) (Coordinate, bool) {
	if a == nil || index < 0 || index >= len(a.coordinates) || uint64(index) > uint64(^uint32(0)) {
		return Coordinate{}, false
	}
	return Coordinate{authority: a, index: uint32(index)}, true
}

// CoordinateIndex maps only a Coordinate issued by this exact Authority to
// its private dense Factor position. Equal portable references from another
// sealed Authority are semantic identities, never local solver coordinates.
func (a *Authority) CoordinateIndex(coordinate Coordinate) (uint32, bool) {
	if a == nil || coordinate.authority != a || uint64(coordinate.index) >= uint64(len(a.coordinates)) {
		return 0, false
	}
	return coordinate.index, true
}

// Owns reports homogeneous Static-carrier admission without interpreting a
// coordinate. Coordinate membership is checked independently by the owner.
func (a *Authority) Owns(value Value) bool {
	if a == nil || value.owner != a {
		return false
	}
	if value.isDerived() {
		return true
	}
	return uint64(value.index) < uint64(len(a.results))
}

// Fingerprint is the allocation-free stable identity of an admitted result
// inside this sealed Authority. Result order is canonical for the Link.
func (a *Authority) Fingerprint(value Value) uint64 {
	if !a.Owns(value) {
		return 0
	}
	if value.isDerived() {
		return a.classes.Fingerprint(value.class)
	}
	return uint64(value.index) + 1
}

func (a *Authority) ClosedType(value Value) (typ.Type, bool) {
	if a == nil || value.owner != a || !value.IsClosed() {
		return nil, false
	}
	if value.isDerived() {
		if value.class.descriptor == nil || len(value.class.descriptor.closed) == 0 {
			return nil, false
		}
		decoded, err := typ.DecodeCanonicalStructural(context.Background(), value.class.descriptor.closed)
		return decoded, err == nil && decoded != nil
	}
	decoded, err := typ.DecodeCanonicalStructural(context.Background(), a.results[value.index].closed)
	return decoded, err == nil && decoded != nil
}

// Compatibility proves the two distinct closed-type judgments needed by
// Static occurrence Rules.  Both operands must be exact Values issued by this
// Authority; equal-content Values from an independently resealed Authority
// are foreign even when their Link and result bytes match.  Static releases
// its construction-time typ graph at Seal, so this is an intentionally cold
// projection through the already-admitted canonical bytes.  It retains no
// pair relation or decoded graph: the raw checker remains the sole proof
// owner, and recursive termination is supplied by that checker.
//
// The first result is subtype.IsSubtype's strict-subtype judgment (including
// its existing equality/coinductive laws); the second is the separate
// fresh-constructor assignment judgment.  Fresh assignment subsumes a proved
// subtype, so the common positive path performs only the first proof.
func (a *Authority) Compatibility(sub, super Value) (strictSubtype, freshAssignment bool) {
	if a == nil || !a.Owns(sub) || !a.Owns(super) || !sub.IsClosed() || !super.IsClosed() {
		return false, false
	}
	if sub.isDerived() || super.isDerived() {
		leftClass, leftOK := a.classes.ClassForStatic(sub)
		rightClass, rightOK := a.classes.ClassForStatic(super)
		if !leftOK || !rightOK {
			return false, false
		}
		if a.classes.Equal(leftClass, rightClass) {
			return true, true
		}
		if a.classes.LessOrEq(leftClass, rightClass) {
			return true, true
		}
		return false, false
	}
	// A repeated exact owner handle is the sealed equality case.  This keeps
	// the hot equality path allocation-free without manufacturing a relation
	// table or decoding a graph that cannot affect the answer.
	if sub.index == super.index {
		return true, true
	}
	left, leftOK := a.ClosedType(sub)
	right, rightOK := a.ClosedType(super)
	if !leftOK || !rightOK {
		return false, false
	}
	strictSubtype = subtype.IsSubtype(left, right)
	if strictSubtype {
		return true, true
	}
	return false, subtype.IsFreshAssignable(left, right)
}

func (a *Authority) Bottom() Value { return Value{owner: a, index: 0} }
func (a *Authority) Top() Value    { return Value{owner: a, index: 1} }

func (a *Authority) Equal(left, right Value) bool {
	if a == nil || left.owner != a || right.owner != a || !a.Owns(left) || !a.Owns(right) {
		return false
	}
	if left.isDerived() || right.isDerived() {
		return left.isDerived() && right.isDerived() && a.classes.Equal(left.class, right.class)
	}
	return left.index == right.index
}
func (a *Authority) Same(left, right Value) bool { return a.Equal(left, right) }

func (a *Authority) LessOrEq(left, right Value) bool {
	if a == nil || left.owner != a || right.owner != a || !a.Owns(left) || !a.Owns(right) {
		return false
	}
	if (!left.isDerived() && left.index == 0) || (!right.isDerived() && right.index == 1) {
		return true
	}
	if (!left.isDerived() && left.index == 1) || (!right.isDerived() && right.index == 0) {
		return false
	}
	if left.isDerived() || right.isDerived() {
		leftClass, leftOK := a.classes.ClassForStatic(left)
		rightClass, rightOK := a.classes.ClassForStatic(right)
		return leftOK && rightOK && a.classes.LessOrEq(leftClass, rightClass)
	}
	if left.index == right.index {
		return true
	}
	if a.results[left.index].kind != KindClosed || a.results[right.index].kind != KindClosed {
		return false
	}
	leftClass, rightClass := a.valueClasses[left.index], a.valueClasses[right.index]
	if !a.classes.LessOrEq(leftClass, rightClass) {
		return false
	}
	// Coverage-equivalent exact Values remain distinct in Static. Only the
	// canonical representative receives incoming edges. In the universal
	// class that representative is Unknown, yielding the exact directed
	// Any -> Unknown rule without treating raw mutual subtyping as equality.
	if a.classes.Equal(leftClass, rightClass) {
		return rightClass.descriptor == nil &&
			uint64(rightClass.index) < uint64(len(a.classCanonical)) && a.classCanonical[rightClass.index].index == right.index
	}
	return rightClass.descriptor == nil && uint64(rightClass.index) < uint64(len(a.classCanonical)) && a.classCanonical[rightClass.index].index == right.index
}

func (a *Authority) Join(left, right Value) Value {
	if a == nil || left.owner != a || right.owner != a || !a.Owns(left) || !a.Owns(right) {
		panic("static: foreign result reached sealed algebra")
	}
	if a.Equal(left, right) {
		return left
	}
	if !left.isDerived() && left.index == 0 {
		return right
	}
	if !right.isDerived() && right.index == 0 {
		return left
	}
	if (!left.isDerived() && left.index == 1) || (!right.isDerived() && right.index == 1) {
		return a.Top()
	}
	if !left.IsClosed() || !right.IsClosed() {
		return a.Top()
	}
	leftClass, leftOK := a.classes.ClassForStatic(left)
	rightClass, rightOK := a.classes.ClassForStatic(right)
	if !leftOK || !rightOK {
		panic("static: Static value lacks ClassSet projection")
	}
	joinedClass := a.classes.Join(leftClass, rightClass)
	if !left.isDerived() && !right.isDerived() && uint64(left.index) < uint64(len(a.runtimeKindMask)) && uint64(right.index) < uint64(len(a.runtimeKindMask)) {
		leftKinds, rightKinds := a.runtimeKindMask[left.index], a.runtimeKindMask[right.index]
		if leftKinds != 0 && rightKinds != 0 {
			kinds := leftKinds | rightKinds
			if result, ok := a.runtimeKinds.lookup(a, kinds); ok {
				candidate, candidateOK := a.classes.ClassForStatic(result)
				if candidateOK && a.classes.Equal(joinedClass, candidate) {
					return result
				}
			}
		}
	}
	if !a.classes.owns(joinedClass) {
		panic("static: malformed ClassSet union")
	}
	if joinedClass.descriptor != nil {
		return Value{owner: a, index: ^uint32(0), class: joinedClass}
	}
	if uint64(joinedClass.index) < uint64(len(a.classCanonical)) {
		result := a.classCanonical[joinedClass.index]
		if result.owner == a {
			return result
		}
	}
	panic("static: ClassSet row lacks Static canonical carrier")
}

func (a *Authority) Widen(previous, next Value) Value { return a.Join(previous, next) }
func (a *Authority) WidenRank(value Value) uint64 {
	if a == nil || value.owner != a || !a.Owns(value) {
		return 0
	}
	if value.isDerived() {
		rank, ok := staticExactRank(a.classes.Rank(value.class), false)
		if !ok {
			return 0
		}
		return rank
	}
	return uint64(a.ranks[value.index])
}

func (a *Authority) Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{Bottom: a.Bottom, Top: a.Top, Equal: a.Equal, Same: a.Same,
		LessOrEq: a.LessOrEq, Join: a.Join, Widen: a.Widen}
}

func (a *Authority) Classes() *ClassSet {
	if a == nil {
		return nil
	}
	return a.classes
}

// sealClassProjection binds exact Static results to the shared semantic-union
// ClassSet once. Exact Values remain distinct (notably Any and Unknown), while
// the Pack projection and all joins use the one ClassSet descriptor authority.
func (a *Authority) sealClassProjection() error {
	if a == nil || a.classes == nil {
		return errors.New("static: ClassSet projection unavailable")
	}
	a.valueClasses = make([]Class, len(a.results))
	a.classCanonical = make([]Value, len(a.classes.rows))
	unknown, _ := typ.EncodeCanonical(context.Background(), typ.Unknown)
	for index := 2; index < len(a.results); index++ {
		if a.results[index].kind != KindClosed {
			continue
		}
		value := Value{owner: a, index: uint32(index)}
		class, ok := a.classes.ClassForStatic(value)
		if !ok {
			return fmt.Errorf("static: result %d lacks ClassSet projection", index)
		}
		a.valueClasses[index] = class
		prior := a.classCanonical[class.index]
		if prior.owner == nil || (bytes.Equal(a.results[index].closed, unknown) && !bytes.Equal(a.results[prior.index].closed, unknown)) ||
			(!bytes.Equal(a.results[index].closed, unknown) && !bytes.Equal(a.results[prior.index].closed, unknown) && bytes.Compare(a.results[index].closed, a.results[prior.index].closed) < 0) {
			a.classCanonical[class.index] = value
		}
	}
	a.ranks = make([]uint64, len(a.results))
	for index := 2; index < len(a.results); index++ {
		if a.results[index].kind == KindClosed {
			class := a.valueClasses[index]
			if !a.classes.owns(class) || class.descriptor != nil || uint64(class.index) >= uint64(len(a.classCanonical)) {
				return errors.New("static: closed result rank class unavailable")
			}
			canonical := a.classCanonical[class.index]
			if canonical.owner != a {
				return errors.New("static: closed result rank canonical unavailable")
			}
			rank, ok := staticExactRank(a.classes.Rank(class), canonical.index != uint32(index))
			if !ok {
				return errors.New("static: exact result rank overflow")
			}
			a.ranks[index] = rank
		} else {
			a.ranks[index] = 1
		}
	}
	var maximum uint64
	for index, rank := range a.ranks {
		if index == 0 || index == 1 {
			continue
		}
		if rank > maximum {
			maximum = rank
		}
	}
	if maximum == ^uint64(0) {
		return errors.New("static: result rank overflow")
	}
	if len(a.ranks) > 0 {
		// Lattice order runs downward through this measure: Bottom is the
		// greatest rank, ordinary values are finite interior points, and Top
		// is the absorbing minimum. This makes every strict widening step
		// (including a direct step to Top) descend.
		a.ranks[0] = maximum + 1
	}
	if len(a.ranks) > 1 {
		a.ranks[1] = 0
	}
	return nil
}

// Static keeps exact Any/Unknown (and any future coverage-equivalent authored
// spellings) distinct while ClassSet gives them one extensional class. The low
// digit orders a noncanonical exact spelling above its canonical carrier;
// the ideal-complement Class rank remains the authoritative high digit.
func staticExactRank(classRank uint64, noncanonical bool) (uint64, bool) {
	if classRank == 0 || classRank > (^uint64(0)-1)/2 {
		return 0, false
	}
	rank := classRank * 2
	if noncanonical {
		rank++
	}
	return rank, true
}

func (a *Authority) addClosed(value typ.Type) (Value, error) {
	if value == nil || typ.ContainsTypeParam(value) || !typ.IsGraphClosed(value) || typ.ValidateStaticGenericRecurrence(value) != nil {
		return Value{}, errors.New("static: non-closed type reached result admission")
	}
	runtimeInput, runtimeOK := a.types.RuntimeInputForType(value)
	if !runtimeOK {
		return Value{}, errors.New("static: closed type lacks scoped Runtime input")
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return Value{}, fmt.Errorf("static: encode closed result: %w", err)
	}
	if len(encoded) == 0 {
		return Value{}, errors.New("static: empty closed result encoding")
	}
	if existing, ok := a.closedByBytes[string(encoded)]; ok {
		return existing, nil
	}
	index, err := denseOrdinal(len(a.results))
	if err != nil {
		return Value{}, fmt.Errorf("static: closed result handle: %w", err)
	}
	result := Value{owner: a, index: index}
	a.results = append(a.results, resultRow{kind: KindClosed, closed: append([]byte(nil), encoded...), runtime: runtimeInput})
	a.closedByBytes[string(encoded)] = result
	return result, nil
}

func (a *Authority) addSymbolic(symbolic Symbolic) (Value, error) {
	if a == nil || symbolic.reason == 0 || !symbolic.namespace.Available() || !symbolic.law.Available() || !symbolic.dependency.Available() || !symbolic.exactSource() {
		return Value{}, errors.New("static: invalid symbolic result")
	}
	if existing, ok := a.symbolicByKey[symbolic]; ok {
		return existing, nil
	}
	index, err := denseOrdinal(len(a.results))
	if err != nil {
		return Value{}, fmt.Errorf("static: symbolic result handle: %w", err)
	}
	result := Value{owner: a, index: index}
	a.results = append(a.results, resultRow{kind: KindSymbolic, symbolic: symbolic})
	a.symbolicByKey[symbolic] = result
	return result, nil
}

func (a *Authority) addInvalid(site Symbolic, fault Fault) (Value, error) {
	if a == nil || fault == 0 || !site.namespace.Available() || !site.law.Available() || !site.dependency.Available() || !site.exactOperand() {
		return Value{}, errors.New("static: invalid diagnostic result")
	}
	for index := 2; index < len(a.results); index++ {
		row := a.results[index]
		if row.kind == KindInvalid && row.fault == fault && row.symbolic == site {
			return Value{owner: a, index: uint32(index)}, nil
		}
	}
	index, err := denseOrdinal(len(a.results))
	if err != nil {
		return Value{}, fmt.Errorf("static: invalid result handle: %w", err)
	}
	result := Value{owner: a, index: index}
	a.results = append(a.results, resultRow{kind: KindInvalid, symbolic: site, fault: fault})
	return result, nil
}

func denseOrdinal(length int) (uint32, error) {
	if length < 0 || uint64(length) > uint64(math.MaxUint32) {
		return 0, errors.New("dense ordinal is not representable")
	}
	return uint32(length), nil
}

func (a *Authority) contentID() (id identity.ContentID) {
	h := sha256.New()
	h.Write([]byte(evaluatorLaw))
	h.Write([]byte{0})
	linkID := a.linkID
	h.Write(linkID[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(a.results)))
	h.Write(word[:])
	for index, row := range a.results {
		h.Write([]byte{byte(row.kind), byte(row.fault), byte(row.symbolic.reason)})
		resultID, ok := staticResultIdentity(Value{owner: a, index: uint32(index)})
		if !ok {
			return identity.ContentID{}
		}
		h.Write(resultID[:])
	}
	// The semantic carrier is committed by ClassSet's canonical descriptors;
	// Static does not maintain a second upper-closure or pair-join authority.
	if a.classes != nil {
		classesID := a.classes.ContentID()
		h.Write(classesID[:])
	}
	for _, rank := range a.ranks {
		binary.BigEndian.PutUint64(word[:], uint64(rank))
		h.Write(word[:])
	}
	for _, result := range a.runtimeKinds.result {
		binary.BigEndian.PutUint64(word[:], uint64(result))
		h.Write(word[:])
	}
	binary.BigEndian.PutUint64(word[:], uint64(len(a.coordinates)))
	h.Write(word[:])
	for _, coordinate := range a.coordinates {
		owner := coordinate.key.reference.Owner()
		h.Write(owner[:])
		node := coordinate.key.reference.NodeID()
		h.Write(node[:])
		h.Write(coordinate.key.namespace[:])
		resultID, ok := staticResultIdentity(coordinate.result)
		if !ok {
			return identity.ContentID{}
		}
		h.Write(resultID[:])
	}
	// Static owns the complete executable TypeValue occurrence interpretation.
	// These portable row identities commit both exact Runtime and explicit Other
	// outcomes without teaching Runtime about Program occurrence facts.
	binary.BigEndian.PutUint64(word[:], uint64(len(a.typeValues)))
	h.Write(word[:])
	for _, row := range a.typeValues {
		if !row.id.Available() {
			return identity.ContentID{}
		}
		h.Write(row.id[:])
	}
	copy(id[:], h.Sum(nil))
	return id
}

// staticResultIdentity is the portable semantic image of an already-admitted
// Static result. It never serializes Authority-local dense coordinates.
func staticResultIdentity(value Value) (identity.ContentID, bool) {
	if value.owner == nil || value.isDerived() || uint64(value.index) >= uint64(len(value.owner.results)) {
		return identity.ContentID{}, false
	}
	row := value.owner.results[value.index]
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/result\x00\x01"))
	_, _ = h.Write([]byte{byte(row.kind), byte(row.fault), byte(row.symbolic.reason)})
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(row.closed)))
	_, _ = h.Write(word[:])
	_, _ = h.Write(row.closed)
	if row.kind == KindSymbolic || row.kind == KindInvalid {
		writeSymbolic(h, row.symbolic)
	}
	var id identity.ContentID
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

func writeSymbolic(h interface{ Write([]byte) (int, error) }, value Symbolic) {
	owner := value.reference.Owner()
	h.Write(owner[:])
	node := value.reference.NodeID()
	h.Write(node[:])
	h.Write(value.sourceOwner[:])
	h.Write(value.source[:])
	h.Write(value.namespace[:])
	h.Write(value.law[:])
	h.Write(value.dependency[:])
	h.Write([]byte{byte(value.reason)})
	h.Write(value.subject.linkID[:])
	h.Write(value.subject.id[:])
}
