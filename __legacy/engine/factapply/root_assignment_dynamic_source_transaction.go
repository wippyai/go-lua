package factapply

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// RootAssignmentDynamicSourcePlan is the frozen structural demand of a root
// assignment whose source is a dynamic-index read. Valuation-specific facts
// are supplied separately, so the plan owns no State or source callback.
type RootAssignmentDynamicSourcePlan struct {
	domain      state.ProductDomain
	resolver    *visibility.Resolver
	point       cfg.Point
	targetPath  pathdom.Path
	dynamic     factflow.DynamicIndexExpression
	targetState pathaddr.StateKey
	targetKey   keyspace.Key
	readKey     pathaddr.StateKey
	containers  []keyspace.Key
	moduloBase  factflow.ValueSource
	hasModulo   bool
	formalRekey state.CoordinateFormalRootRekey
	formalKeys  *keyspace.KeySpace
	isFormal    bool
	sealed      bool
}

// RekeyFormal seals every key-bearing child of the dynamic-source arm into
// the same formal namespace as its factor operands. Lexical source syntax is
// retained only to resolve valuation-dependent static member names; the
// resulting key is mapped through the sealed root authority before exposure.
func (p RootAssignmentDynamicSourcePlan) RekeyFormal(plan state.CoordinateFormalRootRekey) (RootAssignmentDynamicSourcePlan, error) {
	if !p.sealed || !p.domain.Valid() || p.resolver == nil || p.isFormal {
		return RootAssignmentDynamicSourcePlan{}, fmt.Errorf("factapply: invalid dynamic root-assignment formal rekey")
	}
	keys, ok := p.domain.CoordinateFormalDestinationKeySpace(plan)
	if !ok {
		return RootAssignmentDynamicSourcePlan{}, fmt.Errorf("factapply: foreign dynamic root-assignment formal rekey")
	}
	mapKey := func(source keyspace.Key) (keyspace.Key, error) {
		if source.Kind == keyspace.KindInvalid {
			return source, nil
		}
		return p.domain.RekeyStructuralKeyFormal(plan, source)
	}
	mapStateKey := func(source pathaddr.StateKey) (pathaddr.StateKey, error) {
		if source == "" {
			return "", nil
		}
		key, present := p.resolver.KeySpace().InternStateKey(source)
		if !present {
			return "", fmt.Errorf("factapply: unresolved dynamic root-assignment formal state key")
		}
		mapped, err := mapKey(key)
		if err != nil {
			return "", err
		}
		return pathaddr.StateKey(keys.FormatReadOnly(mapped)), nil
	}
	out := p
	var err error
	out.targetKey, err = mapKey(p.targetKey)
	if err != nil {
		return RootAssignmentDynamicSourcePlan{}, err
	}
	out.targetState, err = mapStateKey(p.targetState)
	if err != nil {
		return RootAssignmentDynamicSourcePlan{}, err
	}
	out.readKey, err = mapStateKey(p.readKey)
	if err != nil {
		return RootAssignmentDynamicSourcePlan{}, err
	}
	out.containers = make([]keyspace.Key, len(p.containers))
	for index, container := range p.containers {
		out.containers[index], err = mapKey(container)
		if err != nil {
			return RootAssignmentDynamicSourcePlan{}, err
		}
	}
	out.formalRekey, out.formalKeys, out.isFormal = plan, keys, true
	return out, nil
}

// RootAssignmentDynamicSourceInputs are the explicit factor/value operands
// consumed when resolving one dynamic-source plan.
type RootAssignmentDynamicSourceInputs struct {
	KeyValue                product.Value
	HasKeyValue             bool
	ModuloBaseValue         product.Value
	HasModuloBaseValue      bool
	TableDefinitelyNonEmpty bool
	DynamicIndexFactor      state.LaneFactor
	KeyMembershipFactor     state.LaneFactor
}

// RootAssignmentDynamicSourceTransaction is one valuation-resolved N4
// sidecar. The dynamic delta is independent of root productivity; equality is
// a post-write publication and is therefore exposed separately.
type RootAssignmentDynamicSourceTransaction struct {
	domain            state.ProductDomain
	dynamic           state.RootAssignmentDynamicSourceTransaction
	equality          pathevidence.BranchProof
	hasEquality       bool
	definitelyPresent bool
	formal            bool
	sealed            bool
}

func PrepareRootAssignmentDynamicSourcePlan(
	domain state.ProductDomain,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	point cfg.Point,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) (RootAssignmentDynamicSourcePlan, bool, error) {
	if !domain.Valid() || resolver == nil || resolver.KeySpace() == nil || point == 0 || targetPath.Symbol == 0 {
		return RootAssignmentDynamicSourcePlan{}, false, fmt.Errorf("factapply: invalid dynamic root-assignment plan authority")
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return RootAssignmentDynamicSourcePlan{}, false, nil
	}
	dynamic, ok := facts.DynamicIndexExpression(source.ExprRef)
	if !ok {
		return RootAssignmentDynamicSourcePlan{}, false, nil
	}
	plan := RootAssignmentDynamicSourcePlan{
		domain: domain, resolver: resolver, point: point,
		targetPath: targetPath.Clone(), dynamic: dynamic, sealed: true,
	}
	plan.targetState, _ = visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	plan.targetKey, _ = visibility.AddressAt(resolver, point, targetPath).VisibleKeyspaceKey()
	if keyPath, keyOK := dynamicIndexExpressionKeyPath(resolver, facts, dynamic); keyOK {
		plan.readKey, _ = visibility.AddressAt(resolver, point, keyPath).VisibleStateKey()
	}
	forEachDynamicIndexTableKeyAt(resolver, point, dynamic.TablePathRef(), func(container keyspace.Key) bool {
		plan.containers = append(plan.containers, container)
		return true
	})
	plan.moduloBase, plan.hasModulo = moduloLengthIndexBaseSource(domain.Registry(), facts, dynamic.KeySource(), dynamic.TablePathRef())
	return plan, true, nil
}

// KeyValueInput returns the exact source operand whose valuation selects the
// dynamic member. It is kept separate from root-assignment source ordinal 0,
// which is the already-resolved dynamic-read result.
func (p RootAssignmentDynamicSourcePlan) KeyValueInput() (factflow.ValueSource, bool) {
	if !p.sealed {
		return factflow.ValueSource{}, false
	}
	return p.dynamic.KeySource(), true
}

// ModuloLengthPresenceInput returns the exact structural operands whose
// valuation can prove a modulo-length key in range.
func (p RootAssignmentDynamicSourcePlan) ModuloLengthPresenceInput() (pathdom.Path, factflow.ValueSource, bool) {
	if !p.sealed || !p.hasModulo {
		return pathdom.Path{}, factflow.ValueSource{}, false
	}
	return p.dynamic.TablePathRef().Clone(), p.moduloBase, true
}

// KeyValueSource returns the exact dynamic-key operand whose value controls
// static projection and optional post-write equality.
func (p RootAssignmentDynamicSourcePlan) KeyValueSource() (factflow.ValueSource, bool) {
	if !p.sealed {
		return factflow.ValueSource{}, false
	}
	return p.dynamic.KeySource(), true
}

// StaticKeyEqualityProof returns the exact alias publication induced by an
// exact string key. Both concrete and guarded execution call this same law;
// callers may also use it to freeze the finite coordinate inventory before a
// guarded transaction is evaluated.
func (p RootAssignmentDynamicSourcePlan) StaticKeyEqualityProof(keyValue product.Value) (pathevidence.BranchProof, bool, error) {
	if !p.sealed || !p.domain.Valid() || p.resolver == nil || !product.BelongsToRegistry(p.domain.Registry(), keyValue) {
		return pathevidence.BranchProof{}, false, fmt.Errorf("factapply: invalid dynamic root-assignment equality query")
	}
	name, exact := staticStringKey(p.domain.Registry(), keyValue)
	if !exact || p.targetKey.Kind == keyspace.KindInvalid {
		return pathevidence.BranchProof{}, false, nil
	}
	sourcePath := p.dynamic.TablePathRef().IndexStr(name)
	sourceKey, visible := visibility.AddressAt(p.resolver, p.point, sourcePath).VisibleKeyspaceKey()
	if visible && p.isFormal {
		var err error
		sourceKey, err = p.domain.RekeyStructuralKeyFormal(p.formalRekey, sourceKey)
		if err != nil {
			return pathevidence.BranchProof{}, false, err
		}
	}
	if !visible || sourceKey == p.targetKey {
		return pathevidence.BranchProof{}, false, nil
	}
	return pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathEqual, Path: p.targetKey, Other: sourceKey,
	}, true, nil
}

func (p RootAssignmentDynamicSourcePlan) Resolve(ctx context.Context, inputs RootAssignmentDynamicSourceInputs) (RootAssignmentDynamicSourceTransaction, error) {
	if ctx == nil || !p.sealed || !p.domain.Valid() || p.resolver == nil {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: invalid dynamic root-assignment resolution")
	}
	if err := ctx.Err(); err != nil {
		return RootAssignmentDynamicSourceTransaction{}, err
	}
	reg := p.domain.Registry()
	if inputs.HasKeyValue && !product.BelongsToRegistry(reg, inputs.KeyValue) ||
		inputs.HasModuloBaseValue && !product.BelongsToRegistry(reg, inputs.ModuloBaseValue) {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("factapply: foreign dynamic root-assignment value")
	}
	config := state.RootAssignmentDynamicSourceConfig{}
	if p.targetState != "" {
		for _, container := range p.containers {
			if err := ctx.Err(); err != nil {
				return RootAssignmentDynamicSourceTransaction{}, err
			}
			if p.readKey != "" {
				config.ReadOrigins = append(config.ReadOrigins, state.DynamicIndexReadOrigin{
					Value: p.targetState, Container: container, Key: p.readKey,
				})
			}
			tables, err := p.domain.RootAssignmentDynamicSourceCommonTables(container, inputs.DynamicIndexFactor, inputs.KeyMembershipFactor)
			if err != nil {
				return RootAssignmentDynamicSourceTransaction{}, err
			}
			for _, table := range tables {
				config.KeyMemberships = append(config.KeyMemberships, state.PathKeyMembership(p.targetState, table))
			}
		}
	}
	dynamic, err := p.domain.SealRootAssignmentDynamicSource(config)
	if err != nil {
		return RootAssignmentDynamicSourceTransaction{}, err
	}
	transaction := RootAssignmentDynamicSourceTransaction{domain: p.domain, dynamic: dynamic, formal: p.isFormal, sealed: true}
	transaction.definitelyPresent = p.hasModulo && inputs.TableDefinitelyNonEmpty && inputs.HasModuloBaseValue &&
		typevalue.HasIntegerType(reg, inputs.ModuloBaseValue)
	if inputs.HasKeyValue {
		proof, publish, equalityErr := p.StaticKeyEqualityProof(inputs.KeyValue)
		if equalityErr != nil {
			return RootAssignmentDynamicSourceTransaction{}, equalityErr
		}
		transaction.equality, transaction.hasEquality = proof, publish
	}
	if err := ctx.Err(); err != nil {
		return RootAssignmentDynamicSourceTransaction{}, err
	}
	return transaction, nil
}

func (t RootAssignmentDynamicSourceTransaction) Valid() bool {
	return t.sealed && t.domain.Valid()
}

func (t RootAssignmentDynamicSourceTransaction) DefinitelyPresent() bool {
	return t.Valid() && t.definitelyPresent
}

func (t RootAssignmentDynamicSourceTransaction) PublishedEqualityProof() (pathevidence.BranchProof, bool) {
	return t.equality, t.Valid() && t.hasEquality
}

// ComposeSourceValue delegates nil/presence refinement to the canonical root
// source algebra.
func (t RootAssignmentDynamicSourceTransaction) ComposeSourceValue(
	source product.Value,
	hasSource bool,
	composition RootAssignmentSourceComposition,
) (product.Value, bool, error) {
	if !t.Valid() {
		return product.Value{}, false, fmt.Errorf("factapply: invalid dynamic root-assignment transaction")
	}
	composition.DefinitelyPresent = composition.DefinitelyPresent || t.definitelyPresent
	value, productive := ComposeRootAssignmentSourceValue(t.domain.Registry(), source, hasSource, composition)
	return value, productive, nil
}

// ApplyDynamicSourceFactor publishes the Applied-independent origin and
// membership delta through its registered factor law.
func (t RootAssignmentDynamicSourceTransaction) ApplyDynamicSourceFactor(current state.LaneFactor) (state.LaneFactor, error) {
	if !t.Valid() {
		return state.LaneFactor{}, fmt.Errorf("factapply: invalid dynamic root-assignment transaction")
	}
	return t.domain.ApplyRootAssignmentDynamicSourceFactor(t.dynamic, current)
}

// PrepareCoordinatePathEquality seals the optional post-write equality from
// the caller's already-open path carrier. An unproductive root assignment
// skips this method but still applies ApplyDynamicSourceFactor.
func (t RootAssignmentDynamicSourceTransaction) PrepareCoordinatePathEquality(carrier *state.CoordinatePathEvidenceCarrier[statekey.Value]) (state.PathEqualityTransaction, bool, error) {
	if !t.Valid() {
		return state.PathEqualityTransaction{}, false, fmt.Errorf("factapply: invalid dynamic root-assignment transaction")
	}
	if !t.hasEquality {
		return state.PathEqualityTransaction{}, false, nil
	}
	transaction, err := t.domain.PrepareCoordinatePathEqualityTransaction(carrier, t.equality)
	if err != nil {
		// Dynamic keys can resolve to an unbounded set of static names. The
		// concrete carrier owns the exact persistent coordinate, while the
		// formally rekeyed carrier deliberately has only its finite frozen
		// inventory. Preserve the point-local equality there without inventing
		// a persistent coordinate outside that inventory.
		if t.formal {
			transaction, transientErr := t.domain.PrepareCoordinateTransientPathEqualityTransaction(carrier, t.equality)
			if transientErr == nil {
				return transaction, true, nil
			}
		}
		return state.PathEqualityTransaction{}, false, err
	}
	return transaction, true, nil
}
