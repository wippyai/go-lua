package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootAssignmentTransaction is one callback-free N4 descriptor. Sources are
// the complete recursive object-literal/source inventory required by the
// canonical factor plan, in deterministic discovery order.
type RootAssignmentTransaction struct {
	point      cfg.Point
	assignment factflow.RootAssignment
	sources    []factflow.ValueSource
	index      map[factflow.ValueSource]int
}

func PlanRootAssignmentTransaction(facts factflow.Facts, point cfg.Point) (RootAssignmentTransaction, bool) {
	assignment, ok := facts.RootAssignment(point)
	if !ok || point == 0 || assignment.TargetSymbol() == 0 {
		return RootAssignmentTransaction{}, false
	}
	sources := make([]factflow.ValueSource, 0, 1)
	seenSources := make(map[factflow.ValueSource]struct{})
	activeObjects := make(map[factflow.ExprRef]bool)
	doneObjects := make(map[factflow.ExprRef]bool)
	var add func(factflow.ValueSource) bool
	add = func(source factflow.ValueSource) bool {
		if _, exists := seenSources[source]; !exists {
			seenSources[source] = struct{}{}
			sources = append(sources, source)
		}
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		literal, object := facts.ObjectLiteralView(source.ExprRef)
		if !object {
			return true
		}
		if _, identified := literal.Identity(); !identified {
			return false
		}
		if activeObjects[source.ExprRef] {
			return false
		}
		if doneObjects[source.ExprRef] {
			return true
		}
		activeObjects[source.ExprRef] = true
		defer delete(activeObjects, source.ExprRef)
		exact := true
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			exact = add(entry.Source())
			return exact
		})
		if exact {
			if list, present := literal.ListElementSource(); present {
				exact = add(list)
			}
		}
		if exact {
			doneObjects[source.ExprRef] = true
		}
		return exact
	}
	if !add(assignment.Source()) {
		return RootAssignmentTransaction{}, false
	}
	// A dynamic-index root source is one symbolic source program, not merely
	// its resulting value.  Freeze the exact key and modulo-base operands into
	// the same correlated tuple so guarded resolution never consults State or
	// recompiles expression syntax at a leaf.
	rootSource := assignment.Source()
	if rootSource.Kind == factflow.ValueSourceExpression && rootSource.HasExpr {
		if dynamic, dynamicOK := facts.DynamicIndexExpression(rootSource.ExprRef); dynamicOK {
			if !add(dynamic.KeySource()) {
				return RootAssignmentTransaction{}, false
			}
			if base, baseOK := moduloLengthIndexBaseSource(nil, facts, dynamic.KeySource(), dynamic.TablePathRef()); baseOK && !add(base) {
				return RootAssignmentTransaction{}, false
			}
		}
	}
	var index map[factflow.ValueSource]int
	if len(sources) > 8 {
		index = make(map[factflow.ValueSource]int, len(sources))
		for dense, source := range sources {
			index[source] = dense
		}
	}
	return RootAssignmentTransaction{point: point, assignment: assignment, sources: sources, index: index}, true
}

func (t RootAssignmentTransaction) Valid() bool {
	return t.point != 0 && t.assignment.TargetSymbol() != 0 && len(t.sources) != 0 &&
		(t.index == nil || len(t.index) == len(t.sources))
}
func (t RootAssignmentTransaction) Clone() RootAssignmentTransaction {
	t.sources = append([]factflow.ValueSource(nil), t.sources...)
	if len(t.sources) > 8 {
		t.index = make(map[factflow.ValueSource]int, len(t.sources))
		for index, source := range t.sources {
			t.index[source] = index
		}
	} else {
		t.index = nil
	}
	return t
}
func (t RootAssignmentTransaction) Point() cfg.Point        { return t.point }
func (t RootAssignmentTransaction) TargetSymbol() symbol.ID { return t.assignment.TargetSymbol() }
func (t RootAssignmentTransaction) TargetPath() (pathdom.Path, bool) {
	if !t.Valid() {
		return pathdom.Path{}, false
	}
	path := t.assignment.TargetPathRef()
	if path.Symbol == 0 {
		path.Symbol = t.assignment.TargetSymbol()
	}
	return path.Clone(), path.Symbol != 0
}
func (t RootAssignmentTransaction) SourceCount() int { return len(t.sources) }
func (t RootAssignmentTransaction) Source(index int) (factflow.ValueSource, bool) {
	if index < 0 || index >= len(t.sources) {
		return factflow.ValueSource{}, false
	}
	return t.sources[index], true
}

// SourceOrdinal returns the frozen tuple position of source. Dynamic source
// programs use this to bind their key and modulo-base operands without
// reinterpreting expression syntax in the executor.
func (t RootAssignmentTransaction) SourceOrdinal(source factflow.ValueSource) (int, bool) {
	if !t.Valid() {
		return 0, false
	}
	if t.index != nil {
		index, ok := t.index[source]
		return index, ok
	}
	for index, candidate := range t.sources {
		if factflow.ValueSourceEqual(candidate, source) {
			return index, true
		}
	}
	return 0, false
}

type ResolvedRootAssignmentTransaction struct {
	plan   RootAssignmentTransaction
	values []product.Value
}

func (t RootAssignmentTransaction) Bind(reg *axis.Registry, values []product.Value) (ResolvedRootAssignmentTransaction, bool) {
	if reg == nil || !t.Valid() || len(values) != len(t.sources) {
		return ResolvedRootAssignmentTransaction{}, false
	}
	for _, value := range values {
		if !product.BelongsToRegistry(reg, value) {
			return ResolvedRootAssignmentTransaction{}, false
		}
	}
	// values is borrowed only for the synchronous Authority.Apply call. The
	// resolved transaction must never be retained or published.
	return ResolvedRootAssignmentTransaction{plan: t, values: values}, true
}

type RootAssignmentAuthority struct {
	paths         *PathSemanticAuthority
	facts         factflow.Facts
	closedDynamic []ClosedDynamicAllValueInvariant
	domain        state.ProductDomain
}

// ResolvedRootAssignmentPlan is the immutable admission and dependency
// contract for one frozen N4 transaction. Every shape owns the same sealed
// registered factor topology. Visible-path closure is an optional sum arm:
// lexical writes without a visible path still update Values and the other N4
// factors, but cannot invent path coordinates. Source shape is likewise a sum
// inside this descriptor, never an execution-mode selector.
type ResolvedRootAssignmentPlan struct {
	authority          *RootAssignmentAuthority
	transaction        RootAssignmentTransaction
	factors            state.RootAssignmentFactorPlan
	scalars            state.RootAssignmentScalarFactorTransaction
	hasScalars         bool
	targetPath         keyspace.Key
	targetRoot         keyspace.Key
	hasTargetPath      bool
	sourcePath         keyspace.Key
	hasSourcePath      bool
	sourceCellExecutes bool
	publishAlias       bool
	target             symbol.ID
	sourceShape        RootAssignmentSourceShape
	dynamicSource      RootAssignmentDynamicSourcePlan
	objectSource       ObjectLiteralTargetEntryPlan
	formalRekey        state.CoordinateFormalRootRekey
	formalKeys         *keyspace.KeySpace
	isFormal           bool
}

// RekeyFormal maps the complete N4 descriptor into the formal tuple
// namespace in one operation. Every key-bearing sum arm is handled here; the
// transformer never walks fields or mixes lexical and formal transactions.
func (p ResolvedRootAssignmentPlan) RekeyFormal(
	domain state.ProductDomain,
	rekey state.CoordinateFormalRootRekey,
) (ResolvedRootAssignmentPlan, error) {
	if !p.Valid() || !domain.Valid() || p.authority.domain.Registry() != domain.Registry() || p.isFormal {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: invalid root-assignment formal rekey")
	}
	keys, ok := domain.CoordinateFormalDestinationKeySpace(rekey)
	if !ok {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: foreign root-assignment formal rekey")
	}
	out := p
	var err error
	if p.hasTargetPath {
		out.targetPath, err = domain.RekeyStructuralKeyFormal(rekey, p.targetPath)
		if err != nil {
			return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: formal target path: %w", err)
		}
		out.targetRoot, err = domain.RekeyStructuralKeyFormal(rekey, p.targetRoot)
		if err != nil {
			return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: formal target root: %w", err)
		}
	}
	if p.hasSourcePath {
		out.sourcePath, err = domain.RekeyStructuralKeyFormal(rekey, p.sourcePath)
		if err != nil {
			return ResolvedRootAssignmentPlan{}, err
		}
	}
	if p.hasScalars {
		out.scalars, err = domain.RekeyRootAssignmentScalarFactorTransactionFormal(rekey, p.scalars)
		if err != nil {
			return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: formal scalar child: %w", err)
		}
	}
	if p.sourceShape == RootAssignmentSourceDynamicIndex {
		out.dynamicSource, err = p.dynamicSource.RekeyFormal(rekey)
		if err != nil {
			return ResolvedRootAssignmentPlan{}, err
		}
	}
	out.formalRekey, out.formalKeys, out.isFormal = rekey, keys, true
	return out, nil
}

// RootAssignmentSourceShape is the sealed source-program sum for N4. It is
// structural metadata, not a fast/legacy execution selector: every admitted
// arm must be consumed by the same factor-native executor.
type RootAssignmentSourceShape uint8

const (
	RootAssignmentSourceInvalid RootAssignmentSourceShape = iota
	RootAssignmentSourceScalar
	RootAssignmentSourceDynamicIndex
	RootAssignmentSourceObjectLiteral
)

func (a *RootAssignmentAuthority) PrepareResolvedRootAssignmentPlan(transaction RootAssignmentTransaction) (ResolvedRootAssignmentPlan, error) {
	if !a.Valid() || !transaction.Valid() {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: invalid root-assignment component plan")
	}
	factors, err := a.domain.SealRootAssignmentFactorPlan()
	if err != nil {
		return ResolvedRootAssignmentPlan{}, err
	}
	plan := ResolvedRootAssignmentPlan{authority: a, transaction: transaction.Clone(), factors: factors, target: transaction.TargetSymbol()}
	targetPath, ok := rootAssignmentEvidenceTargetPath(transaction.assignment.TargetSymbol(), transaction.assignment.TargetPathRef())
	if !ok {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: root assignment at %d has no target path", transaction.point)
	}
	target, targetVisible := visibility.AddressAt(a.paths.resolver, transaction.point, targetPath).VisibleKeyspaceKey()
	targetRoot, rootVisible := visibility.AddressAt(a.paths.resolver, transaction.point, pathdom.Path{Symbol: transaction.TargetSymbol()}).VisibleKeyspaceKey()
	if targetVisible && rootVisible {
		plan.targetPath = target
		plan.targetRoot = targetRoot
		plan.hasTargetPath = true
	}
	rootSource, _ := transaction.Source(0)
	_, objectSource := a.facts.ObjectLiteralView(rootSource.ExprRef)
	dynamicPlan, dynamicSource, dynamicErr := PrepareRootAssignmentDynamicSourcePlan(
		a.domain, a.paths.resolver, a.facts, transaction.point, targetPath, rootSource,
	)
	if dynamicErr != nil {
		return ResolvedRootAssignmentPlan{}, dynamicErr
	}
	if objectSource && dynamicSource {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: root assignment at %d has ambiguous source shape", transaction.point)
	}
	switch {
	case objectSource:
		objectPlan, objectErr := PrepareObjectLiteralTargetEntryPlan(
			a.domain.Registry(), a.paths.typeValues, a.paths.resolver, a.facts, transaction.point, targetPath, rootSource,
		)
		if objectErr != nil {
			return ResolvedRootAssignmentPlan{}, objectErr
		}
		plan.sourceShape, plan.objectSource = RootAssignmentSourceObjectLiteral, objectPlan
	case dynamicSource:
		plan.sourceShape, plan.dynamicSource = RootAssignmentSourceDynamicIndex, dynamicPlan
	default:
		plan.sourceShape = RootAssignmentSourceScalar
	}
	scalars, present, scalarErr := prepareRootAssignmentRegisteredScalars(
		a.domain.Registry(), a.paths.resolver, a.facts, transaction.point, targetPath, transaction.assignment.Source(), a.domain,
	)
	if scalarErr != nil {
		return ResolvedRootAssignmentPlan{}, scalarErr
	}
	plan.scalars, plan.hasScalars = scalars, present
	source, _ := transaction.Source(0)
	if sourcePath, present := sourcePathFromValueSource(a.paths.resolver, a.facts, source); present && !sourcePath.IsEmpty() {
		if sourceKey, visible := visibility.AddressAt(a.paths.resolver, transaction.point, sourcePath).VisibleKeyspaceKey(); visible {
			plan.sourcePath = sourceKey
			plan.hasSourcePath = true
			// Reflexive assignment carries no equality quotient: x=x is already
			// represented by the target value itself, and PathEvidence deliberately
			// rejects reflexive proofs as non-facts.
			plan.publishAlias = plan.hasTargetPath && plan.targetPath != sourceKey && !covariantExposureSuppressesPathProof(a.facts, a.paths.resolver, transaction.point, source)
		}
	}
	return plan, nil
}

func (p ResolvedRootAssignmentPlan) Valid() bool {
	return p.authority != nil && p.authority.domain.OwnsRootAssignmentFactorPlan(p.factors) &&
		p.transaction.Valid() && p.target != 0 && p.sourceShape >= RootAssignmentSourceScalar && p.sourceShape <= RootAssignmentSourceObjectLiteral &&
		(!p.hasTargetPath || p.targetPath.Kind != keyspace.KindInvalid && p.targetRoot.Kind != keyspace.KindInvalid)
}

func (p ResolvedRootAssignmentPlan) SourceShape() (RootAssignmentSourceShape, bool) {
	return p.sourceShape, p.Valid()
}

// BindResolvedSourcePath completes a scalar N4 descriptor whose source path is
// owned by a later structural linker (currently a lexical call-result frame).
// The executor remains source-kind agnostic: after this binding, the ordinary
// registered path/descendant/equality transaction is complete.
func (p ResolvedRootAssignmentPlan) BindResolvedSourcePath(sourcePath keyspace.Key) (ResolvedRootAssignmentPlan, error) {
	if !p.Valid() || p.hasSourcePath || sourcePath.Kind == keyspace.KindInvalid || p.authority.paths == nil ||
		p.authority.paths.resolver == nil || p.authority.paths.resolver.KeySpace() == nil ||
		p.authority.paths.resolver.KeySpace().FormatReadOnly(sourcePath) == "" {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: invalid resolved root-assignment source path")
	}
	source, ok := p.transaction.Source(0)
	if !ok || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.TargetIndex < 0 {
		return ResolvedRootAssignmentPlan{}, fmt.Errorf("factapply: resolved root-assignment source is not a call result")
	}
	p.sourcePath = sourcePath
	p.hasSourcePath = true
	p.sourceCellExecutes = true
	p.publishAlias = p.hasTargetPath && p.targetPath != sourcePath &&
		!covariantExposureSuppressesPathProof(p.authority.facts, p.authority.paths.resolver, p.transaction.point, source)
	return p, nil
}

func (p ResolvedRootAssignmentPlan) DynamicSourcePlan() (RootAssignmentDynamicSourcePlan, bool) {
	return p.dynamicSource, p.Valid() && p.sourceShape == RootAssignmentSourceDynamicIndex
}

// DynamicSourceDependencies returns the registration-owned input topology for
// the dynamic-index source arm. The transformer binds these lanes by product
// order; it never switches on concrete axis names.
func (p ResolvedRootAssignmentPlan) DynamicSourceDependencies() (state.RootAssignmentDynamicSourceDependencies, bool, error) {
	if !p.Valid() || p.sourceShape != RootAssignmentSourceDynamicIndex {
		return state.RootAssignmentDynamicSourceDependencies{}, false, nil
	}
	dependencies, err := p.authority.domain.SealRootAssignmentDynamicSourceDependencies()
	return dependencies, err == nil, err
}

func (p ResolvedRootAssignmentPlan) ObjectLiteralSourcePlan() (ObjectLiteralTargetEntryPlan, bool) {
	return p.objectSource, p.Valid() && p.sourceShape == RootAssignmentSourceObjectLiteral && p.objectSource.Valid()
}

// FactorPlan returns the ProductDomain-sealed N4 component topology. The same
// plan is present for every admitted root-assignment shape; Componentized only
// reports whether its path-coordinate dependency closure has also been sealed.
// This makes the registered factor inventory foundational rather than an
// optional fast-path attachment.
func (p ResolvedRootAssignmentPlan) FactorPlan() (state.RootAssignmentFactorPlan, bool) {
	if p.authority == nil || !p.authority.domain.OwnsRootAssignmentFactorPlan(p.factors) {
		return state.RootAssignmentFactorPlan{}, false
	}
	return p.factors, true
}

// ScalarFactorTransaction returns the precompiled unary numeric/relational/
// user-lattice N4 transaction. Its source demand is structural and therefore
// frozen once; guarded leaves supply only the registered point/current factors.
func (p ResolvedRootAssignmentPlan) ScalarFactorTransaction() (state.RootAssignmentScalarFactorTransaction, bool) {
	if p.authority == nil || !p.hasScalars || !p.authority.domain.OwnsRootAssignmentScalarFactorTransaction(p.scalars) {
		return state.RootAssignmentScalarFactorTransaction{}, false
	}
	return p.scalars, true
}

// SourcePresenceProof returns the exact point-entry proof whose truth removes
// nil from the source before assignment. Absence means source composition is
// independent of point path evidence.
func (p ResolvedRootAssignmentPlan) SourcePresenceProof() (pathevidence.BranchProof, bool) {
	if !p.Valid() || !p.hasSourcePath {
		return pathevidence.BranchProof{}, false
	}
	return pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: p.sourcePath, Presence: presence.Present()}, true
}

// PublishedEqualityProof returns the optional exact alias proof added after
// destructive target replacement.
func (p ResolvedRootAssignmentPlan) PublishedEqualityProof() (pathevidence.BranchProof, bool) {
	if !p.Valid() || !p.publishAlias || !p.hasSourcePath {
		return pathevidence.BranchProof{}, false
	}
	return pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: p.targetPath, Other: p.sourcePath}, true
}

// ComposeFactorPrimarySourceValue applies the same source-value algebra as
// concrete N4 to the one canonical value produced by the source program.
// sourceDefinitelyPresent is the disjunction
// of the admitted exact presence producers (path proof and, when present, the
// dynamic-index sidecar); it is evidence, not an execution mode.
func (p ResolvedRootAssignmentPlan) ComposeFactorPrimarySourceValue(
	reg *axis.Registry,
	source product.Value,
	sourceDefinitelyPresent bool,
) (product.Value, bool, error) {
	if !p.Valid() || reg == nil || p.authority.domain.Registry() != reg || !product.BelongsToRegistry(reg, source) {
		return product.Value{}, false, fmt.Errorf("factapply: invalid root-assignment factor source")
	}
	declared, hasDeclared := p.transaction.assignment.DeclaredValue()
	declaredContracts := p.transaction.assignment.DeclaredValueContracts()
	declaredOverlays := p.transaction.assignment.DeclaredValueOverlays()
	mode := RootAssignmentDeclaredInvalid
	if declaredContracts {
		mode = RootAssignmentDeclaredContract
	} else if declaredOverlays {
		mode = RootAssignmentDeclaredOverlay
	}
	needSource := !hasDeclared || !declaredContracts || RootAssignmentDeclaredContractNeedsSourceRuntimeIdentity(reg, declared)
	hasSource := needSource && !product.Equal(reg, source, product.Bottom(reg))
	sourceFact, _ := p.transaction.Source(0)
	value, productive := ComposeRootAssignmentSourceValue(reg, source, hasSource, RootAssignmentSourceComposition{
		Declared: declared, DeclaredMode: mode, HasDeclared: hasDeclared,
		SourceCarriesRuntimeIdentity: sourceFact.HasExpr,
		SourceCellExecutes:           p.sourceCellExecutes,
		DefinitelyPresent:            hasSource && sourceDefinitelyPresent,
	})
	return value, productive, nil
}

func (p ResolvedRootAssignmentPlan) TargetValueSlot() (statekey.Value, bool) {
	if !p.Valid() || p.target == 0 {
		return 0, false
	}
	return statekey.SymbolValue(p.target), true
}

func (p ResolvedRootAssignmentPlan) TargetPathKey() (keyspace.Key, bool) {
	return p.targetPath, p.Valid() && p.hasTargetPath
}

func (p ResolvedRootAssignmentPlan) TargetRootKey() (keyspace.Key, bool) {
	return p.targetRoot, p.Valid() && p.hasTargetPath
}

// SourcePathKey exposes the already-rekeyed source spelling for freeze-time
// identity contracts. It is absent for non-path sources.
func (p ResolvedRootAssignmentPlan) SourcePathKey() (keyspace.Key, bool) {
	return p.sourcePath, p.Valid() && p.hasSourcePath
}

func (p ResolvedRootAssignmentPlan) PathKeySpace() (*keyspace.KeySpace, bool) {
	if !p.Valid() {
		return nil, false
	}
	if p.isFormal {
		return p.formalKeys, p.formalKeys != nil && p.formalKeys.Valid()
	}
	if p.authority.paths.resolver.KeySpace() == nil {
		return nil, false
	}
	return p.authority.paths.resolver.KeySpace(), true
}

// PrepareFactorValueWrite seals the exact normalized Values replacement for
// this N4 plan. Both guarded and concrete adapters consume the same state law.
func (p ResolvedRootAssignmentPlan) PrepareFactorValueWrite(value product.Value) (state.RootAssignmentValueWrite, error) {
	if !p.Valid() {
		return state.RootAssignmentValueWrite{}, fmt.Errorf("factapply: invalid root-assignment value write plan")
	}
	slot, _ := p.TargetValueSlot()
	return p.authority.domain.SealRootAssignmentValueWrite(slot, value)
}

// PrepareFactorStablePathEvidence seals the mandatory stable-symbol rewrite
// from an already-open coordinate carrier. It remains meaningful when the
// optional visible-path arm is empty.
func (p ResolvedRootAssignmentPlan) PrepareFactorStablePathEvidence(
	carrier *state.CoordinatePathEvidenceCarrier[statekey.Value],
	value product.Value,
	idempotent bool,
) (state.StableRootPathEvidenceMutation, error) {
	return p.PrepareFactorStablePathEvidenceWithFormalRoots(carrier, value, idempotent, nil)
}

func (p ResolvedRootAssignmentPlan) PrepareFactorStablePathEvidenceWithFormalRoots(
	carrier *state.CoordinatePathEvidenceCarrier[statekey.Value],
	value product.Value,
	idempotent bool,
	formalRoots []formal.Root,
) (state.StableRootPathEvidenceMutation, error) {
	if !p.Valid() || carrier == nil {
		return state.StableRootPathEvidenceMutation{}, fmt.Errorf("factapply: invalid root-assignment stable-path plan")
	}
	snapshot, ok := carrier.SnapshotImplications()
	if !ok {
		return state.StableRootPathEvidenceMutation{}, fmt.Errorf("factapply: root-assignment stable-path snapshot unavailable")
	}
	return PrepareRootAssignmentStablePathEvidenceWithFormalRoots(
		p.authority.domain.Registry(), p.authority.domain, p.keySpace(),
		snapshot, p.target, value, idempotent, formalRoots,
	)
}

// PrepareFactorPathSubtree seals destructive visible-path replacement. The
// false result is the legitimate empty visible-path sum arm.
func (p ResolvedRootAssignmentPlan) PrepareFactorPathSubtree(
	skeleton state.CoordinateFamilySkeleton,
	scalars []state.CoordinateScalarFactor,
) (state.PathSubtreeMutation, bool, error) {
	if !p.Valid() {
		return state.PathSubtreeMutation{}, false, fmt.Errorf("factapply: invalid root-assignment subtree plan")
	}
	if !p.hasTargetPath {
		return state.PathSubtreeMutation{}, false, nil
	}
	mutation, err := p.authority.domain.PrepareCoordinatePathSubtreeMutation(
		skeleton, scalars, p.keySpace().FormatReadOnly(p.targetPath),
	)
	return mutation, err == nil, err
}

// PrepareFactorPathEquality seals the optional post-write quotient using the
// same coordinate carrier that performed destructive replacement.
func (p ResolvedRootAssignmentPlan) PrepareFactorPathEquality(
	carrier *state.CoordinatePathEvidenceCarrier[statekey.Value],
) (state.PathEqualityTransaction, bool, error) {
	proof, ok := p.PublishedEqualityProof()
	if !ok {
		return state.PathEqualityTransaction{}, false, nil
	}
	// Publishing into a proof-Top carrier is a semantic no-op: the concrete
	// lane cannot retain this equality, so no other axis may consume a quotient
	// as though it had.  Distinguish that case from an already-known equality,
	// whose unchanged AddProof still leaves HasProof true and therefore does
	// require the registered participant rewrites.
	staged := carrier.Clone()
	if staged == nil {
		return state.PathEqualityTransaction{}, false, fmt.Errorf("factapply: cannot stage root-assignment path equality")
	}
	if _, valid := staged.AddProof(proof); !valid {
		return state.PathEqualityTransaction{}, false, fmt.Errorf("factapply: root-assignment path equality is not authorized")
	}
	if !staged.HasProof(proof) {
		return state.PathEqualityTransaction{}, false, nil
	}
	transaction, err := p.authority.domain.PrepareCoordinatePathEqualityTransaction(carrier, proof)
	return transaction, err == nil, err
}

// PrepareFactorCompletion derives the registered caller-local completion from
// the frozen root source and explicit fresh-empty predicates. It is the
// general N4 counterpart of call-receiver completion and never reads State.
func (p ResolvedRootAssignmentPlan) PrepareFactorCompletion(
	reg *axis.Registry,
	primarySource product.Value,
	freshEmpty func(pathdom.Path) bool,
) (state.RootAssignmentFactorTransaction, error) {
	if !p.Valid() || reg == nil || reg != p.authority.domain.Registry() || !product.BelongsToRegistry(reg, primarySource) {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: invalid root-assignment factor completion")
	}
	declared, hasDeclared := p.transaction.assignment.DeclaredValue()
	needSource := !hasDeclared || !p.transaction.assignment.DeclaredValueContracts() ||
		RootAssignmentDeclaredContractNeedsSourceRuntimeIdentity(reg, declared)
	hasSource := needSource && !product.Equal(reg, primarySource, product.Bottom(reg))
	targetPath, ok := rootAssignmentEvidenceTargetPath(p.transaction.TargetSymbol(), p.transaction.assignment.TargetPathRef())
	if !ok {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: root-assignment completion has no target path")
	}
	freshTarget := false
	if freshEmpty != nil {
		freshTarget = freshEmpty(pathdom.Path{Symbol: p.transaction.TargetSymbol()})
	}
	completion, err := prepareRootAssignmentCompletion(
		reg, p.authority.paths.resolver, p.authority.paths.typeValues,
		p.transaction.point, targetPath, primarySource, hasSource, freshTarget, freshEmpty, p.authority.closedDynamic,
	)
	if err != nil {
		return state.RootAssignmentFactorTransaction{}, err
	}
	transaction, err := p.authority.domain.SealRootAssignmentCompletion(completion)
	if err != nil || !p.isFormal {
		return transaction, err
	}
	return p.authority.domain.RekeyRootAssignmentCompletionFormal(p.formalRekey, transaction)
}

func (p ResolvedRootAssignmentPlan) keySpace() *keyspace.KeySpace {
	if p.isFormal {
		return p.formalKeys
	}
	return p.authority.paths.resolver.KeySpace()
}

// FactorCompletionFreshEmptyPaths returns the exact detached root-path
// predicates consumed by PrepareFactorCompletion for this frozen plan.
// The target root is present only when a closed-dynamic invariant consumes its
// freshness; source containers are present only when assigning their table.
func (p ResolvedRootAssignmentPlan) FactorCompletionFreshEmptyPaths() ([]pathdom.Path, error) {
	if !p.Valid() {
		return nil, fmt.Errorf("factapply: invalid root-assignment completion query")
	}
	return p.authority.completionFreshEmptyPaths(ResolvedRootAssignmentTransaction{plan: p.transaction})
}

// PathDependencies seals the exact path-family footprint of the frozen N4
// transaction. Destructive target replacement, source observation, normalized
// Values access and optional equality publication are one certificate.
func (p ResolvedRootAssignmentPlan) PathDependencies(domain state.ProductDomain, slots []state.CoordinateSlot) (PathDependencySchedule, error) {
	return p.PathDependenciesWithFormalRoots(domain, slots, nil)
}

// PathDependenciesWithFormalRoots derives the one path-mutation footprint for
// concrete State and rekeyed formal carriers. Formal roots only expand the
// finite certificate for the same stable-root mutation law.
func (p ResolvedRootAssignmentPlan) PathDependenciesWithFormalRoots(domain state.ProductDomain, slots []state.CoordinateSlot, formalRoots []formal.Root) (PathDependencySchedule, error) {
	if !p.Valid() || !p.hasTargetPath || !domain.Valid() || p.authority.domain.Registry() != domain.Registry() {
		return PathDependencySchedule{}, fmt.Errorf("factapply: root-assignment plan has no path dependency component")
	}
	family, ok := domain.PathValueFamily()
	access := domain.RootAssignmentAccess()
	if !ok || !access.Current.Has(family.Lane().ID()) || !access.CurrentWrites.Has(family.Lane().ID()) {
		return PathDependencySchedule{}, fmt.Errorf("factapply: root-assignment path dependency is outside registered access")
	}
	const dependencyID state.CoordinateDependencyID = 1
	seed := state.CoordinateDependencySeed{
		ID: dependencyID, WritePaths: []keyspace.Key{p.targetPath}, DescendantMutationRoots: []keyspace.Key{p.targetRoot},
		StableRootMutations: []symbol.ID{p.target}, FormalStableRoots: append([]formal.Root(nil), formalRoots...),
	}
	if p.hasSourcePath {
		seed.ReadPaths = []keyspace.Key{p.sourcePath}
	}
	if p.publishAlias {
		proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: p.targetPath, Other: p.sourcePath}
		slot, err := domain.PathBranchProofCoordinateSlot(p.keySpace(), proof)
		if err != nil {
			return PathDependencySchedule{}, err
		}
		seed.AddCoordinates = []state.CoordinateSlot{slot}
	}
	plan, err := domain.PlanPathCoordinateDependencies(p.keySpace(), slots, []state.CoordinateDependencySeed{seed})
	if err != nil {
		return PathDependencySchedule{}, err
	}
	return sealPathDependencySchedule(plan, [][]state.CoordinateDependencyID{{dependencyID}})
}

func (p ResolvedRootAssignmentPlan) CoordinateClosure(domain state.ProductDomain, slots []state.CoordinateSlot) ([]int, error) {
	if !p.Valid() || !domain.Valid() || p.authority.domain.Registry() != domain.Registry() {
		return nil, fmt.Errorf("factapply: root-assignment plan has no coordinate component")
	}
	family, ok := domain.PathValueFamily()
	access := domain.RootAssignmentAccess()
	if !ok || !access.Current.Has(family.Lane().ID()) || !access.CurrentWrites.Has(family.Lane().ID()) {
		return nil, fmt.Errorf("factapply: root-assignment path closure is outside registered access")
	}
	paths := make([]keyspace.Key, 0, 2)
	if p.hasTargetPath {
		paths = append(paths, p.targetPath)
	}
	if p.hasSourcePath {
		paths = append(paths, p.sourcePath)
	}
	return domain.PathCoordinateMutationClosure(slots, paths, []symbol.ID{p.target})
}

func NewRootAssignmentAuthority(paths *PathSemanticAuthority, facts factflow.Facts, closed []ClosedDynamicAllValueInvariant, domain state.ProductDomain) *RootAssignmentAuthority {
	if paths == nil || !paths.Valid() || !domain.Valid() {
		return nil
	}
	return &RootAssignmentAuthority{paths: paths, facts: facts, closedDynamic: append([]ClosedDynamicAllValueInvariant(nil), closed...), domain: domain}
}

func (a *RootAssignmentAuthority) Valid() bool {
	return a != nil && a.paths != nil && a.paths.Valid() && a.domain.Valid()
}

// CallReceiverCompletionFreshEmptyPaths returns the exact caller roots whose
// fresh-empty-table predicate is required by this receiver's closed-dynamic
// completion. Empty means guarded execution needs no Values or Heap input for
// N4 completion.
func (a *RootAssignmentAuthority) CallReceiverCompletionFreshEmptyPaths(transaction ResolvedRootAssignmentTransaction) ([]pathdom.Path, error) {
	return a.completionFreshEmptyPaths(transaction)
}

// completionFreshEmptyPaths is the one authority-owned inventory law shared
// by ordinary N4 completion and the materialized call-receiver adapter.
func (a *RootAssignmentAuthority) completionFreshEmptyPaths(transaction ResolvedRootAssignmentTransaction) ([]pathdom.Path, error) {
	if !a.Valid() || !transaction.plan.Valid() || len(transaction.plan.sources) == 0 {
		return nil, fmt.Errorf("factapply: invalid call receiver completion query")
	}
	target := pathdom.Path{Symbol: transaction.plan.TargetSymbol()}
	paths := make([]pathdom.Path, 0, len(a.closedDynamic)+1)
	add := func(candidate pathdom.Path) {
		if candidate.Symbol == 0 || len(candidate.Segments) != 0 {
			return
		}
		for _, existing := range paths {
			if existing.Equal(candidate) {
				return
			}
		}
		paths = append(paths, candidate.Clone())
	}
	for _, invariant := range a.closedDynamic {
		if invariant.Container.Equal(target) {
			add(target)
		}
		if invariant.Table.Equal(target) {
			add(invariant.Container)
		}
	}
	return paths, nil
}

// PrepareCallReceiverFactorTransaction derives and seals the caller-local N4
// completion from only the resolved source value and the explicitly requested
// fresh-empty predicates. It never consumes or constructs a State. The
// returned transaction is applied sequentially to each registered factor via
// ProductDomain.ApplyRootAssignmentCompletionFactor.
func (a *RootAssignmentAuthority) PrepareCallReceiverFactorTransaction(
	reg *axis.Registry,
	transaction ResolvedRootAssignmentTransaction,
	freshEmpty func(pathdom.Path) bool,
) (state.RootAssignmentFactorTransaction, error) {
	if len(transaction.plan.sources) == 0 || len(transaction.values) == 0 {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: call receiver has no resolved source")
	}
	source := transaction.plan.sources[0]
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.TargetIndex < 0 {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: root assignment at %d is not a lexical call receiver", transaction.plan.point)
	}
	queries, err := a.CallReceiverCompletionFreshEmptyPaths(transaction)
	if err != nil {
		return state.RootAssignmentFactorTransaction{}, err
	}
	if len(queries) != 0 && freshEmpty == nil {
		return state.RootAssignmentFactorTransaction{}, fmt.Errorf("factapply: call receiver at %d has no fresh-empty predicate provider", transaction.plan.point)
	}
	targetRoot := pathdom.Path{Symbol: transaction.plan.TargetSymbol()}
	freshTarget := false
	if freshEmpty != nil {
		freshTarget = freshEmpty(targetRoot)
	}
	completion, err := prepareRootAssignmentCompletion(
		reg,
		a.paths.resolver,
		a.paths.typeValues,
		transaction.plan.point,
		transaction.plan.assignment.TargetPathRef(),
		transaction.values[0],
		true,
		freshTarget,
		freshEmpty,
		a.closedDynamic,
	)
	if err != nil {
		return state.RootAssignmentFactorTransaction{}, err
	}
	return a.domain.SealRootAssignmentCompletion(completion)
}
