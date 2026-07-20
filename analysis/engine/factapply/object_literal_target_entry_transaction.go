package factapply

import (
	"context"
	"errors"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

// ObjectLiteralTargetEntryPlan is the detached source program for one object
// literal assignment. It owns no State and admits every source through one
// dense, uncapped ordinal inventory. Concrete and guarded callers therefore
// bind the same ObjectLiteralPlan operands before consuming the resolved
// object-constructor and ordered target-entry terms.
type ObjectLiteralTargetEntryPlan struct {
	valid      bool
	registry   *axis.Registry
	typeValues *typevalue.Cache
	keys       *keyspace.KeySpace
	point      cfg.Point
	target     pathdom.Path
	sources    []factflow.ValueSource
	sourceAt   map[factflow.ValueSource]int
	objects    []objectLiteralTargetObjectPlan
	rootObject int
	entries    []objectLiteralTargetWritePlan
	listFloor  int64
}

var errObjectLiteralTargetGraphUnconstructable = errors.New("object-literal target graph is not constructable")

type objectLiteralTargetObjectPlan struct {
	literal      luasourcevalue.ObjectLiteralPlan
	localSources []int
	members      []objectLiteralTargetMemberPlan
	identity     identity.ID
	stableShape  bool
}

type objectLiteralTargetMemberPlan struct {
	suffix      []segment.Segment
	source      int
	expected    product.Value
	hasExpected bool
}

type objectLiteralTargetWritePlan struct {
	target        pathdom.Path
	source        int
	expected      product.Value
	hasExpected   bool
	sourcePath    pathdom.Path
	hasSourcePath bool
	suppressProof bool
}

// PrepareObjectLiteralTargetEntryPlan freezes the complete recursive literal
// graph, canonical ObjectLiteralPlans, and exact root-entry publication order.
// Cycles and identityless objects fail before any source value is requested.
func PrepareObjectLiteralTargetEntryPlan(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	point cfg.Point,
	target pathdom.Path,
	root factflow.ValueSource,
) (ObjectLiteralTargetEntryPlan, error) {
	if reg == nil || resolver == nil || point <= 0 || target.IsEmpty() || target.Symbol == 0 || !root.HasExpr {
		return ObjectLiteralTargetEntryPlan{}, fmt.Errorf("factapply: invalid object-literal target-entry plan")
	}
	rootLiteral, ok := facts.ObjectLiteralView(root.ExprRef)
	if !ok {
		return ObjectLiteralTargetEntryPlan{}, fmt.Errorf("factapply: missing root object literal")
	}
	plan := ObjectLiteralTargetEntryPlan{
		valid: true, registry: reg, typeValues: typeValues, keys: resolver.KeySpace(), point: point,
		target: target.Clone(), sourceAt: make(map[factflow.ValueSource]int),
		rootObject: -1, listFloor: objectLiteralContiguousListLengthFloor(rootLiteral),
	}
	addSource := func(source factflow.ValueSource) int {
		if ordinal, exists := plan.sourceAt[source]; exists {
			return ordinal
		}
		ordinal := len(plan.sources)
		plan.sources = append(plan.sources, source)
		plan.sourceAt[source] = ordinal
		return ordinal
	}
	active := make(map[factflow.ExprRef]bool)
	done := make(map[factflow.ExprRef]bool)
	objectAt := make(map[factflow.ExprRef]int)
	var freeze func(factflow.ValueSource) error
	freeze = func(source factflow.ValueSource) error {
		if !source.HasExpr {
			return nil
		}
		literal, object := facts.ObjectLiteralView(source.ExprRef)
		if !object {
			return nil
		}
		if active[source.ExprRef] {
			return fmt.Errorf("%w: cycle", errObjectLiteralTargetGraphUnconstructable)
		}
		if done[source.ExprRef] {
			return nil
		}
		id, identified := literal.Identity()
		if !identified || id == (identity.ID{}) {
			return fmt.Errorf("%w: missing identity", errObjectLiteralTargetGraphUnconstructable)
		}
		literalPlan, compiled := luasourcevalue.CompileObjectLiteralPlanCached(reg, typeValues, literal)
		if !compiled {
			return fmt.Errorf("factapply: object-literal source plan compilation failed")
		}
		objectPlan := objectLiteralTargetObjectPlan{literal: literalPlan.Clone(), identity: id}
		_, hasListTail := literal.ListElementSource()
		objectPlan.stableShape = literal.StaticStringKeysComplete() && !hasListTail
		active[source.ExprRef] = true
		defer delete(active, source.ExprRef)
		var freezeErr error
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			entrySource := entry.Source()
			ordinal := addSource(entrySource)
			if err := freeze(entrySource); err != nil {
				freezeErr = err
				return false
			}
			expected, hasExpected := entry.Expected()
			objectPlan.members = append(objectPlan.members, objectLiteralTargetMemberPlan{
				suffix: entry.SuffixSegments(), source: ordinal, expected: expected, hasExpected: hasExpected,
			})
			return true
		})
		if freezeErr != nil {
			return freezeErr
		}
		if list, present := literal.ListElementSource(); present {
			addSource(list)
			if err := freeze(list); err != nil {
				return err
			}
		}
		objectPlan.localSources = make([]int, literalPlan.ValueSourceCount())
		for index := range objectPlan.localSources {
			local, sourceOK := literalPlan.ValueSourceAt(index)
			ordinal, indexed := plan.sourceAt[local]
			if !sourceOK || !indexed {
				return fmt.Errorf("factapply: malformed object-literal source inventory")
			}
			objectPlan.localSources[index] = ordinal
		}
		objectAt[source.ExprRef] = len(plan.objects)
		plan.objects = append(plan.objects, objectPlan)
		done[source.ExprRef] = true
		return nil
	}
	addSource(root)
	if err := freeze(root); err != nil {
		return ObjectLiteralTargetEntryPlan{}, err
	}
	rootObject, rootObjectOK := objectAt[root.ExprRef]
	if !rootObjectOK || rootObject < 0 || rootObject >= len(plan.objects) {
		return ObjectLiteralTargetEntryPlan{}, fmt.Errorf("%w: missing lexical root", errObjectLiteralTargetGraphUnconstructable)
	}
	plan.rootObject = rootObject
	rootLiteral.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryTarget, appendOK := entry.AppendSuffixTo(target)
		if !appendOK {
			return true
		}
		expected, hasExpected := entry.Expected()
		write := objectLiteralTargetWritePlan{
			target: entryTarget.Clone(), source: addSource(entry.Source()), expected: expected, hasExpected: hasExpected,
		}
		if sourcePath, sourceOK := sourcePathFromValueSource(resolver, facts, entry.Source()); sourceOK && !sourcePath.IsEmpty() && sourcePath.Symbol != 0 {
			write.sourcePath, write.hasSourcePath = sourcePath.Clone(), true
			write.suppressProof = covariantExposureSuppressesPathProof(facts, resolver, point, entry.Source())
		}
		plan.entries = append(plan.entries, write)
		return true
	})
	return plan, nil
}

func (p ObjectLiteralTargetEntryPlan) Valid() bool {
	return p.valid && p.registry != nil && p.keys != nil && p.keys.Valid() && p.point > 0 && !p.target.IsEmpty() && p.target.Symbol != 0 && len(p.sources) != 0 && p.rootObject >= 0 && p.rootObject < len(p.objects)
}

func (p ObjectLiteralTargetEntryPlan) ValueSourceCount() int {
	if !p.Valid() {
		return 0
	}
	return len(p.sources)
}

func (p ObjectLiteralTargetEntryPlan) ValueSourceAt(index int) (factflow.ValueSource, bool) {
	if !p.Valid() || index < 0 || index >= len(p.sources) {
		return factflow.ValueSource{}, false
	}
	return p.sources[index], true
}

func (p ObjectLiteralTargetEntryPlan) ValueSourceIndex(source factflow.ValueSource) (int, bool) {
	if !p.Valid() {
		return 0, false
	}
	index, ok := p.sourceAt[source]
	return index, ok
}

// MatchesRootAssignmentSourceInventory proves that this object topology uses
// the already-frozen N4 source discovery order. The object plan never owns a
// second operand order; ObjectLiteralPlan local operands map by ValueSource
// identity into this exact root transaction inventory.
func (p ObjectLiteralTargetEntryPlan) MatchesRootAssignmentSourceInventory(transaction RootAssignmentTransaction) bool {
	if !p.Valid() || !transaction.Valid() || p.point != transaction.Point() || len(p.sources) != transaction.SourceCount() {
		return false
	}
	for index, source := range p.sources {
		other, ok := transaction.Source(index)
		if !ok || !factflow.ValueSourceEqual(source, other) {
			return false
		}
	}
	return true
}

// ResolvedObjectLiteralTargetEntryTransaction is the finite semantic output
// of one correlated source row. Its ObjectConstructorPlan is the registered
// heap topology law; entries are the exact ordered path-replacement terms.
// No whole State, callback, Facts, or source resolver is retained.
type ResolvedObjectLiteralTargetEntryTransaction struct {
	valid       bool
	registry    *axis.Registry
	keys        *keyspace.KeySpace
	point       cfg.Point
	target      pathdom.Path
	constructor state.ObjectConstructorPlan
	values      []state.ObjectConstructorValues
	rootObject  int
	entries     []ResolvedPathStoreWrite
	listFloor   int64
}

// ResolveObjectLiteralTargetEntryTransaction binds one exact correlated row.
// Unavailable members and target entries are omitted, matching concrete
// object publication, while root values are composed by ObjectLiteralPlan.
func ResolveObjectLiteralTargetEntryTransaction(
	reg *axis.Registry,
	plan ObjectLiteralTargetEntryPlan,
	row []luasourcevalue.ObjectLiteralPlanValue,
) (ResolvedObjectLiteralTargetEntryTransaction, error) {
	if reg == nil || reg != plan.registry || !plan.Valid() || len(row) != len(plan.sources) {
		return ResolvedObjectLiteralTargetEntryTransaction{}, fmt.Errorf("factapply: invalid object-literal target-entry row")
	}
	for index := range row {
		if row[index].Available && !product.BelongsToRegistry(reg, row[index].Value) {
			return ResolvedObjectLiteralTargetEntryTransaction{}, fmt.Errorf("factapply: foreign object-literal source value")
		}
	}
	shapes := make([]state.ObjectConstructorShape, len(plan.objects))
	values := make([]state.ObjectConstructorValues, len(plan.objects))
	for objectIndex, object := range plan.objects {
		local := make([]luasourcevalue.ObjectLiteralPlanValue, len(object.localSources))
		for localIndex, globalIndex := range object.localSources {
			local[localIndex] = row[globalIndex]
		}
		root, composed := luasourcevalue.ComposeObjectLiteralPlanCached(reg, plan.typeValues, object.literal, local)
		if !composed {
			return ResolvedObjectLiteralTargetEntryTransaction{}, fmt.Errorf("factapply: object-literal root composition failed")
		}
		shape := state.ObjectConstructorShape{Identity: identity.ConcreteTerm(object.identity), StableShape: object.stableShape}
		objectValues := state.ObjectConstructorValues{Root: root}
		for _, member := range object.members {
			if _, validSuffix := plan.keys.FromRootlessSuffix(member.suffix); !validSuffix || !row[member.source].Available {
				continue
			}
			value := row[member.source].Value
			if member.hasExpected {
				value = overlayExpectedObjectEntryValue(reg, plan.typeValues, value, member.expected)
			}
			shape.MemberSuffixes = append(shape.MemberSuffixes, append([]segment.Segment(nil), member.suffix...))
			objectValues.Members = append(objectValues.Members, value)
		}
		shapes[objectIndex], values[objectIndex] = shape, objectValues
	}
	domain := state.RegisteredProductDomain(reg)
	var constructor state.ObjectConstructorPlan
	if len(shapes) != 0 {
		var err error
		constructor, err = domain.PrepareObjectConstructorPlan(plan.keys, shapes)
		if err != nil {
			return ResolvedObjectLiteralTargetEntryTransaction{}, err
		}
	}
	transaction := ResolvedObjectLiteralTargetEntryTransaction{
		valid: true, registry: reg, keys: plan.keys, point: plan.point, target: plan.target.Clone(),
		constructor: constructor, values: values, rootObject: plan.rootObject, listFloor: plan.listFloor,
	}
	for _, entry := range plan.entries {
		if !row[entry.source].Available {
			continue
		}
		value := row[entry.source].Value
		if entry.hasExpected {
			value = overlayExpectedObjectEntryValue(reg, plan.typeValues, value, entry.expected)
		}
		transaction.entries = append(transaction.entries, ResolvedPathStoreWrite{
			Target: entry.target.Clone(), Value: value,
			SourcePath: entry.sourcePath.Clone(), HasSourcePath: entry.hasSourcePath, SuppressProof: entry.suppressProof,
		})
	}
	return transaction, nil
}

// ResolveGuardedObjectLiteralTargetEntryTransaction binds the guarded
// constructor row.  A guarded source term is an exact value decision: unlike
// the callback-based concrete resolver it has no separate "unavailable"
// outcome.  Keeping this conversion beside the canonical resolver prevents
// the symbolic executor from reimplementing object-literal composition,
// expected-member overlays, discriminants, or constructor ordering.
func ResolveGuardedObjectLiteralTargetEntryTransaction(
	reg *axis.Registry,
	plan ObjectLiteralTargetEntryPlan,
	values []product.Value,
) (ResolvedObjectLiteralTargetEntryTransaction, error) {
	row := make([]luasourcevalue.ObjectLiteralPlanValue, len(values))
	for index, value := range values {
		row[index] = luasourcevalue.ObjectLiteralPlanValue{Value: value, Available: true}
	}
	return ResolveObjectLiteralTargetEntryTransaction(reg, plan, row)
}

// PrepareGuardedObjectConstructor resolves the canonical all-present guarded
// row and seals its registered constructor topology in the execution
// authority's KeySpace.  Lexical fact plans and invocation worlds may own
// isomorphic but pointer-distinct keyspaces; suffix syntax, rather than an
// accidental pointer identity, is the authority for importing the plan.
func (p ObjectLiteralTargetEntryPlan) PrepareGuardedObjectConstructor(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	values []product.Value,
) (PreparedGuardedObjectConstructor, error) {
	if !p.Valid() || !domain.Valid() || domain.Registry() != p.registry || keys == nil || !keys.Valid() {
		return PreparedGuardedObjectConstructor{}, fmt.Errorf("factapply: invalid guarded object constructor authority")
	}
	resolved, err := ResolveGuardedObjectLiteralTargetEntryTransaction(p.registry, p, values)
	if err != nil {
		return PreparedGuardedObjectConstructor{}, err
	}
	// resolved is local and its freshly allocated rows are transferred into
	// the prepared result. Do not clone the complete member vector at this hot
	// guarded leaf boundary.
	rows := resolved.values
	if !resolved.constructor.Valid() || len(rows) != len(p.objects) {
		return PreparedGuardedObjectConstructor{}, fmt.Errorf("factapply: guarded object constructor row is incomplete")
	}
	root, rootPresent := resolved.RootSourceValue()
	if !rootPresent {
		return PreparedGuardedObjectConstructor{}, fmt.Errorf("factapply: guarded object constructor root is missing")
	}
	constructor, err := p.PrepareObjectConstructorPlan(domain, keys)
	if err != nil {
		return PreparedGuardedObjectConstructor{}, err
	}
	return PreparedGuardedObjectConstructor{registry: p.registry, constructor: constructor, values: rows, root: root}, nil
}

// PrepareObjectConstructorPlan seals the value-independent object topology
// owned by this source plan into keys. Static operator footprints and guarded
// execution consume this same plan, so a registered constructor coordinate
// cannot be written without first being declared by the operator.
func (p ObjectLiteralTargetEntryPlan) PrepareObjectConstructorPlan(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
) (state.ObjectConstructorPlan, error) {
	if !p.Valid() || !domain.Valid() || domain.Registry() != p.registry || keys == nil || !keys.Valid() {
		return state.ObjectConstructorPlan{}, fmt.Errorf("factapply: invalid object constructor topology authority")
	}
	shapes := make([]state.ObjectConstructorShape, len(p.objects))
	for objectIndex, object := range p.objects {
		shape := state.ObjectConstructorShape{Identity: identity.ConcreteTerm(object.identity), StableShape: object.stableShape}
		shape.MemberSuffixes = make([][]segment.Segment, len(object.members))
		for memberIndex, member := range object.members {
			shape.MemberSuffixes[memberIndex] = append([]segment.Segment(nil), member.suffix...)
		}
		shapes[objectIndex] = shape
	}
	return domain.PrepareObjectConstructorPlan(keys, shapes)
}

// PreparedGuardedObjectConstructor is the single resolved output of the
// canonical ObjectLiteralPlan composition. Constructor application and the
// root-assignment source term consume this same result; no caller may resolve
// the lexical root independently from its members.
type PreparedGuardedObjectConstructor struct {
	registry    *axis.Registry
	constructor state.ObjectConstructorPlan
	values      []state.ObjectConstructorValues
	root        product.Value
}

func (p PreparedGuardedObjectConstructor) Valid() bool {
	if p.registry == nil || !p.constructor.Valid() || len(p.values) == 0 || !product.BelongsToRegistry(p.registry, p.root) {
		return false
	}
	for _, object := range p.values {
		if !product.BelongsToRegistry(p.registry, object.Root) {
			return false
		}
		for _, member := range object.Members {
			if !product.BelongsToRegistry(p.registry, member) {
				return false
			}
		}
	}
	return true
}

func (p PreparedGuardedObjectConstructor) ObjectConstructor() (state.ObjectConstructorPlan, []state.ObjectConstructorValues, bool) {
	if !p.Valid() {
		return state.ObjectConstructorPlan{}, nil, false
	}
	// Values are borrowed for the synchronous ProductDomain constructor call;
	// neither the prepared result nor ApplyObjectConstructor mutates them.
	return p.constructor, p.values, true
}

func (p PreparedGuardedObjectConstructor) RootSourceValue() (product.Value, bool) {
	return p.root, p.Valid()
}

func (t ResolvedObjectLiteralTargetEntryTransaction) Valid(reg *axis.Registry) bool {
	if !t.valid || reg == nil || reg != t.registry || t.keys == nil || !t.keys.Valid() || t.point <= 0 || t.target.IsEmpty() || t.target.Symbol == 0 || t.listFloor < 0 || t.constructor.Valid() != (len(t.values) != 0) || t.rootObject < 0 || t.rootObject >= len(t.values) {
		return false
	}
	for _, entry := range t.entries {
		if entry.Target.IsEmpty() || entry.Target.Symbol == 0 || !product.BelongsToRegistry(reg, entry.Value) || entry.HasSourcePath != (!entry.SourcePath.IsEmpty() && entry.SourcePath.Symbol != 0) {
			return false
		}
	}
	for _, object := range t.values {
		if !product.BelongsToRegistry(reg, object.Root) {
			return false
		}
		for _, member := range object.Members {
			if !product.BelongsToRegistry(reg, member) {
				return false
			}
		}
	}
	return true
}

// RootSourceValue returns the lexical root's value from the same canonical
// object-plan composition that produced every constructor row. The explicit
// ordinal is frozen while traversing syntax; it is never inferred from
// post-order, child order, or identity deduplication.
func (t ResolvedObjectLiteralTargetEntryTransaction) RootSourceValue() (product.Value, bool) {
	if !t.Valid(t.registry) {
		return product.Value{}, false
	}
	return t.values[t.rootObject].Root, true
}

func (t ResolvedObjectLiteralTargetEntryTransaction) EntryCount() int { return len(t.entries) }

func (t ResolvedObjectLiteralTargetEntryTransaction) EntryAt(index int) (ResolvedPathStoreWrite, bool) {
	if index < 0 || index >= len(t.entries) {
		return ResolvedPathStoreWrite{}, false
	}
	return cloneResolvedPathStoreWrite(t.entries[index]), true
}

// ObjectConstructor returns the registered constructor topology and detached
// concrete operand rows. Guarded execution consumes the same topology with
// correlated scalar operands instead of these concrete rows.
func (t ResolvedObjectLiteralTargetEntryTransaction) ObjectConstructor() (state.ObjectConstructorPlan, []state.ObjectConstructorValues, bool) {
	if !t.constructor.Valid() {
		return state.ObjectConstructorPlan{}, nil, false
	}
	values := make([]state.ObjectConstructorValues, len(t.values))
	for index := range t.values {
		values[index] = state.ObjectConstructorValues{Root: t.values[index].Root, Members: append([]product.Value(nil), t.values[index].Members...)}
	}
	return t.constructor, values, true
}

// ApplyConcreteObjectLiteralTargetEntryTransaction is the concrete adapter for
// the semantic transaction. The constructor is applied by the registered
// object law and ordered entries by the existing resolved path-store law.
// Failure or cancellation returns the exact input, never a published prefix.
func ApplyConcreteObjectLiteralTargetEntryTransaction(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	transaction ResolvedObjectLiteralTargetEntryTransaction,
	input state.State,
) (state.State, error) {
	if ctx.Registry == nil || resolver == nil || ctx.Point != transaction.point || resolver.KeySpace() != transaction.keys || !transaction.Valid(ctx.Registry) {
		return input, fmt.Errorf("factapply: invalid object-literal target-entry transaction")
	}
	if err := objectLiteralTransactionCanceled(ctx); err != nil {
		return input, err
	}
	out := input
	if transaction.constructor.Valid() {
		domain := state.RegisteredProductDomain(ctx.Registry)
		var err error
		out, err = domain.ApplyObjectConstructor(transaction.constructor, transaction.values, input)
		if err != nil {
			return input, err
		}
	}
	if err := objectLiteralTransactionCanceled(ctx); err != nil {
		return input, err
	}
	object := ResolvedPathStoreObject{
		Entries: append([]ResolvedPathStoreWrite(nil), transaction.entries...), ListFloor: transaction.listFloor,
	}
	req := ResolvedPathStoreRequest{Context: ctx, Resolver: resolver, Input: input, Output: out, Transaction: ResolvedPathStoreTransaction{
		Point: transaction.point, Assignment: ResolvedPathStoreWrite{Target: transaction.target, Value: product.Bottom(ctx.Registry)}, HasAssignment: true,
	}}
	out = applyResolvedPathStoreObject(req, out, object)
	if err := objectLiteralTransactionCanceled(ctx); err != nil {
		return input, err
	}
	return out, nil
}

func objectLiteralTransactionCanceled(ctx transfer.NodeContext) error {
	if ctx.Context != nil {
		if err := ctx.Context.Err(); err != nil {
			return err
		}
	}
	if token := tokenOf(ctx.Session); token != nil && token.Canceled() {
		return context.Canceled
	}
	return nil
}
