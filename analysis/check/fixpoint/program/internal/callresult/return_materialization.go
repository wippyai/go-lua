package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefinement "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func materializeReturnRootTypesFromFacts(reg *axis.Registry, typeValues *typevalue.Cache, sum summary.Summary) summary.Summary {
	if reg == nil || len(sum.NormalReturnFacts.PathRefinements) == 0 && len(sum.NormalReturnFacts.PathStaticMembers) == 0 {
		return sum
	}
	maxIndex := len(sum.Returns) - 1
	for _, fact := range sum.NormalReturnFacts.PathRefinements {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	for _, fact := range sum.NormalReturnFacts.PathStaticMembers {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	if maxIndex < 0 {
		return sum
	}
	returnsOwned := false
	if len(sum.Returns) <= maxIndex {
		expanded := make([]product.Value, maxIndex+1)
		copy(expanded, sum.Returns)
		sum.Returns = expanded
		returnsOwned = true
	}
	ensureReturnsOwned := func() {
		if returnsOwned {
			return
		}
		next := make([]product.Value, len(sum.Returns))
		copy(next, sum.Returns)
		sum.Returns = next
		returnsOwned = true
	}
	for index := 0; index <= maxIndex; index++ {
		factType, ok := returnRecordTypeFromFacts(reg, sum.NormalReturnFacts, index)
		if !ok {
			continue
		}
		t := factType
		if existing, existingOK := typevalue.TypeOf(reg, sum.Returns[index]); existingOK {
			merged, mergedOK := mergeReturnRecordFactType(existing, factType)
			if !mergedOK {
				continue
			}
			t = merged
		} else if !returnSlotNeedsFactType(reg, sum.Returns[index]) {
			continue
		}
		next := typeWitnessValue(reg, typeValues, t)
		if product.Equal(reg, sum.Returns[index], next) {
			continue
		}
		ensureReturnsOwned()
		sum.Returns[index] = next
	}
	return sum
}

func returnSlotNeedsFactType(reg *axis.Registry, value product.Value) bool {
	if product.Equal(reg, value, product.Bottom(reg)) {
		return true
	}
	_, ok := typevalue.TypeOf(reg, value)
	return !ok
}

func mergeReturnRecordFactType(existing typ.Type, factType typ.Type) (typ.Type, bool) {
	if existing == nil {
		return factType, true
	}
	base, ok := existing.(*typ.Record)
	if !ok {
		return nil, false
	}
	facts, ok := factType.(*typ.Record)
	if !ok {
		return nil, false
	}
	parts := typ.RecordParts{
		Fields:        append([]typ.Field(nil), base.Fields...),
		StaticMembers: append([]typ.StaticMember(nil), base.StaticMembers...),
		Metatable:     base.Metatable,
		MapKey:        base.MapKey,
		MapValue:      base.MapValue,
		Open:          base.Open,
	}
	for _, field := range facts.Fields {
		parts.Fields = upsertReturnRecordField(parts.Fields, field)
	}
	for _, member := range facts.StaticMembers {
		parts.StaticMembers = upsertReturnRecordStaticMember(parts.StaticMembers, member)
	}
	return typetable.RebuildRecord(parts), true
}

func upsertReturnRecordField(fields []typ.Field, field typ.Field) []typ.Field {
	for i := range fields {
		if fields[i].Name == field.Name {
			fields[i] = field
			return fields
		}
	}
	return append(fields, field)
}

func upsertReturnRecordStaticMember(members []typ.StaticMember, member typ.StaticMember) []typ.StaticMember {
	for i := range members {
		if typ.CompareStaticMembers(members[i], member) == 0 {
			members[i] = member
			return members
		}
	}
	return append(members, member)
}

func returnRecordTypeFromFacts(reg *axis.Registry, facts callboundary.NormalReturnFacts, index int) (typ.Type, bool) {
	var parts typ.RecordParts
	seen := false
	add := func(path pathdom.Path, value product.Value) {
		if path.PlaceholderIndex() != index || len(path.Segments) != 1 {
			return
		}
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || t == nil {
			return
		}
		if name, ok := path.DirectFieldName(); ok {
			payload, optional := typetable.EntryValueShape(t)
			parts.Fields = append(parts.Fields, typ.Field{Name: name, Type: payload, Optional: optional})
			seen = true
			return
		}
		if index, ok := path.DirectIntIndex(); ok {
			payload, optional := typetable.EntryValueShape(t)
			parts.StaticMembers = append(parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: int64(index), Type: payload, Optional: optional})
			seen = true
		}
	}
	for _, fact := range facts.PathRefinements {
		add(fact.Path, fact.Value)
	}
	for _, fact := range facts.PathStaticMembers {
		add(fact.Path, fact.Value)
	}
	if !seen {
		return nil, false
	}
	return typetable.RebuildRecord(parts), true
}

func materializeReturnParamPathAliases(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	got summary.Summary,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	typeValues *typevalue.Cache,
) summary.Summary {
	if ctx.Registry == nil || sources == nil || len(got.ReturnParamPathAliases) == 0 {
		return got
	}
	objects := got.HeapTableObjects
	changed := false
	for _, alias := range got.ReturnParamPathAliases {
		value, ok := returnParamAliasSourceValue(ctx, ks, site, alias.Source, sources, in, read)
		if !ok {
			continue
		}
		if alias.Member == "" {
			if writeDirectReturnParamAliasValue(ctx.Registry, got.Returns, alias.ReturnIndex, value) {
				changed = true
			}
			continue
		}
		if len(objects) == 0 {
			continue
		}
		if writeReturnParamAliasMember(ctx.Registry, ks, objects, got.Returns, alias.ReturnIndex, alias.Member, value) {
			changed = true
		}
	}
	if !changed {
		return got
	}
	got.HeapTableObjects = objects
	return summary.NormalizeOwned(ctx.Registry, got)
}

func writeDirectReturnParamAliasValue(reg *axis.Registry, returns []product.Value, returnIndex int, value product.Value) bool {
	if reg == nil || returnIndex < 0 || returnIndex >= len(returns) {
		return false
	}
	current := returns[returnIndex]
	var next product.Value
	if product.Equal(reg, current, product.Bottom(reg)) || product.Equal(reg, current, product.Top()) {
		next = value
	} else if product.Equal(reg, current, value) {
		return false
	} else if valuerefinement.DeclaredContractAlreadySatisfied(reg, value, current) {
		next = value
	} else if mergedPresence, ok := typevalue.DeclaredTypeFactsPresenceOnly(reg, value, current); ok {
		next = product.WithPresence(reg, value, mergedPresence)
	} else {
		next = valuerefinement.MergeDeclaredContract(reg, value, current)
		if currentPresence := product.PresenceOf(current); !presence.Equal(product.PresenceOf(next), currentPresence) {
			next = product.WithPresence(reg, next, currentPresence)
		}
	}
	if product.Equal(reg, current, next) {
		return false
	}
	returns[returnIndex] = next
	return true
}

func returnParamAliasSourceValue(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	sourceKey pathaddr.PlaceholderKey,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	sourcePath, ok := sourceKey.Path()
	if !ok {
		return product.Value{}, false
	}
	index := sourcePath.PlaceholderIndex()
	source, ok := site.ArgumentSourceAt(index)
	if !ok {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	if len(sourcePath.Segments) == 0 {
		return value, true
	}
	return sourcevalue.HeapMemberFromValue(ctx.Registry, ks, in, value, sourcePath.Segments)
}

func writeReturnParamAliasMember(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	returns []product.Value,
	returnIndex int,
	memberKey pathaddr.SuffixKey,
	value product.Value,
) bool {
	rootID := returnIdentityAt(reg, returns, returnIndex)
	if rootID == (identity.ID{}) {
		return false
	}
	segments, ok := pathaddr.RelativeStaticMemberSuffixSegments(memberKey)
	if !ok || len(segments) == 0 {
		return false
	}
	key, ok := ks.FromRootlessSuffix(segments)
	if !ok {
		return false
	}
	changed := writeHeapObjectStaticMember(reg, objects, rootID, returns[returnIndex], key, value)
	if writeNestedHeapObjectStaticMember(reg, ks, objects, rootID, returns[returnIndex], segments, value) {
		changed = true
	}
	return changed
}

func writeNestedHeapObjectStaticMember(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	rootID identity.ID,
	rootValue product.Value,
	segments []segment.Segment,
	value product.Value,
) bool {
	if len(segments) < 2 {
		return false
	}
	currentID := rootID
	currentValue := rootValue
	for len(segments) > 1 {
		key, ok := ks.FromRootlessSuffix(segments[:1])
		if !ok {
			return false
		}
		object, ok := objects[currentID]
		if !ok {
			return false
		}
		nextValue, ok := object.StaticMember(key)
		if !ok {
			return false
		}
		nextID, ok := product.Get(reg, nextValue, identity.Key).ID()
		if !ok {
			return false
		}
		currentID = nextID
		currentValue = nextValue
		segments = segments[1:]
	}
	key, ok := ks.FromRootlessSuffix(segments)
	if !ok {
		return false
	}
	return writeHeapObjectStaticMember(reg, objects, currentID, currentValue, key, value)
}

func writeHeapObjectStaticMember(
	reg *axis.Registry,
	objects map[identity.ID]heapidentity.TableObject,
	id identity.ID,
	root product.Value,
	key keyspace.Key,
	value product.Value,
) bool {
	if id == (identity.ID{}) || key.Kind == keyspace.KindInvalid {
		return false
	}
	object := objects[id]
	staticMembers := object.StaticMembers()
	if staticMembers == nil {
		staticMembers = make(map[keyspace.Key]product.Value, 1)
	}
	if existing, ok := staticMembers[key]; ok && product.Equal(reg, existing, value) {
		return false
	}
	staticMembers[key] = value
	objectRoot := object.Root()
	if product.Equal(reg, objectRoot, product.Bottom(reg)) && !product.Equal(reg, root, product.Bottom(reg)) {
		objectRoot = root
	}
	objects[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              objectRoot,
		StaticMembers:     staticMembers,
		DynamicIndexFacts: object.DynamicIndexFacts(),
	})
	return true
}

func returnIdentityAt(reg *axis.Registry, returns []product.Value, index int) identity.ID {
	if index < 0 || index >= len(returns) {
		return identity.ID{}
	}
	id, ok := product.Get(reg, returns[index], identity.Key).ID()
	if !ok {
		return identity.ID{}
	}
	return id
}

func summaryReturnValueAt(reg *axis.Registry, sum summary.Summary, index int) (product.Value, bool) {
	if index < 0 || index >= len(sum.Returns) {
		return product.Value{}, false
	}
	value := sum.Returns[index]
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func dropDescendantFactsBelowMaybeAbsentReturns(sum summary.Summary) summary.Summary {
	if len(sum.Returns) == 0 {
		return sum
	}
	maybeAbsent := make(map[int]struct{})
	for i, value := range sum.Returns {
		if !product.DefinitelyPresent(value) {
			maybeAbsent[i] = struct{}{}
		}
	}
	if len(maybeAbsent) == 0 {
		return sum
	}
	sum.NormalReturnFacts = sum.NormalReturnFacts.DropFactsTouchingPaths(func(p pathdom.Path) bool {
		return strictPlaceholderDescendant(p, maybeAbsent)
	})
	return sum
}

func strictPlaceholderDescendant(p pathdom.Path, roots map[int]struct{}) bool {
	if len(p.Segments) == 0 {
		return false
	}
	index := p.PlaceholderIndex()
	if index < 0 {
		return false
	}
	_, ok := roots[index]
	return ok
}

func applyDeclaredSummaryReturns(reg *axis.Registry, typeValues *typevalue.Cache, got summary.Summary, fn *typ.Function) summary.Summary {
	if reg == nil || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
		return got
	}
	return specializeSummaryReturns(reg, typeValues, got, fn.Returns, fn.Returns)
}

func typeWitnessValue(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	if typeValues != nil {
		return typeValues.FromTypeWithWitness(reg, t)
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

func specializeSummaryReturns(reg *axis.Registry, typeValues *typevalue.Cache, got summary.Summary, originalReturns []typ.Type, returns []typ.Type) summary.Summary {
	if reg == nil || len(returns) == 0 {
		return got
	}
	originalReturns = callResultReturnTypes(got, originalReturns)
	returns = callResultReturnTypes(got, returns)
	// A function whose body never returns normally (e.g. a stub `function f(): T
	// error("nyi") end`) has no summary return values, but its declared signature
	// is still the contract callers see. Size the slots to the declared returns and
	// adopt the declared type for any slot the body left empty.
	width := len(got.Returns)
	if len(returns) > width {
		width = len(returns)
	}
	nextReturns := make([]product.Value, width)
	copy(nextReturns, got.Returns)
	changed := width != len(got.Returns)
	heapObjects := got.HeapTableObjects
	heapChanged := false
	for i := range nextReturns {
		if i >= len(returns) {
			break
		}
		ret := returns[i]
		if ret == nil || refinement.ContainsFreeTypeParam(ret) {
			continue
		}
		declared := typeWitnessValue(reg, typeValues, ret)
		if i >= len(got.Returns) {
			// No body return for this slot: adopt the declared return directly.
			nextReturns[i] = declared
			changed = true
			continue
		}
		next := joinInstantiatedReturnValue(reg, nextReturns[i], declared, originalReturnTypeAt(originalReturns, i))
		if nextObjects, ok := clampDeclaredReturnHeapMembers(reg, typeValues, got.HeapKeySpace, heapObjects, next, ret); ok {
			heapObjects = nextObjects
			heapChanged = true
		}
		if product.Equal(reg, nextReturns[i], next) {
			continue
		}
		nextReturns[i] = next
		changed = true
	}
	if !changed && !heapChanged {
		return got
	}
	got.Returns = nextReturns
	if heapChanged {
		got.HeapTableObjects = heapObjects
	}
	return summary.NormalizeOwned(reg, got)
}

func originalReturnTypeAt(returns []typ.Type, index int) typ.Type {
	if index < 0 || index >= len(returns) {
		return nil
	}
	return returns[index]
}

func callResultReturnTypes(got summary.Summary, returns []typ.Type) []typ.Type {
	if len(returns) == 1 && len(got.Returns) > 1 {
		if tuple, ok := returns[0].(*typ.Tuple); ok {
			return append([]typ.Type(nil), tuple.Elements...)
		}
	}
	return returns
}

func joinInstantiatedReturnValue(reg *axis.Registry, value product.Value, declared product.Value, original typ.Type) product.Value {
	if product.Equal(reg, value, product.Top()) {
		return declared
	}
	if typ.IsAny(original) || typ.IsUnknown(original) {
		return value
	}
	if refinement.ContainsFreeTypeParam(original) || valueContainsFreeTypeParam(reg, value) {
		return declared
	}
	if returnValueCarriesUntrustedTopEvidence(reg, value) && declaredReturnHasConcreteContract(reg, declared) {
		return declared
	}
	merged := valuerefinement.MergeDeclaredContract(reg, value, declared)
	return product.WithPresence(reg, merged, product.PresenceOf(declared))
}

func returnValueCarriesUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func declaredReturnHasConcreteContract(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t) && !refinement.ContainsFreeTypeParam(t)
}

func clampDeclaredReturnHeapMembers(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	root product.Value,
	declared typ.Type,
) (map[identity.ID]heapidentity.TableObject, bool) {
	if reg == nil || ks == nil || len(objects) == 0 || declared == nil {
		return objects, false
	}
	id, ok := product.Get(reg, root, identity.Key).ID()
	if !ok {
		return objects, false
	}
	object, ok := objects[id]
	if !ok {
		return objects, false
	}
	contracts := declaredReturnMemberBoundaryContracts(reg, typeValues, ks, declared, object.StaticMembers())
	if len(contracts) == 0 {
		return objects, false
	}
	staticMembers := object.StaticMembers()
	changed := false
	for key, contract := range contracts {
		if existing, ok := staticMembers[key]; ok && product.Equal(reg, existing, contract) {
			continue
		}
		if staticMembers == nil {
			staticMembers = make(map[keyspace.Key]product.Value, len(contracts))
		}
		staticMembers[key] = contract
		changed = true
	}
	if !changed {
		return objects, false
	}
	next := make(map[identity.ID]heapidentity.TableObject, len(objects))
	for objectID, object := range objects {
		next[objectID] = object
	}
	next[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              object.Root(),
		StaticMembers:     staticMembers,
		DynamicIndexFacts: object.DynamicIndexFacts(),
	})
	return next, true
}

func declaredReturnMemberBoundaryContracts(reg *axis.Registry, typeValues *typevalue.Cache, ks *keyspace.KeySpace, declared typ.Type, existing map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(existing) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value)
	for key, current := range existing {
		segments, ok := ks.SuffixSegmentsView(key)
		if !ok || len(segments) == 0 {
			continue
		}
		t, ok := luatypeprojection.ApplySegments(declared, segments)
		if !ok || t == nil {
			continue
		}
		currentType, currentTypeOK := typevalue.TypeOf(reg, current)
		if !declaredTypeContainsBoundaryTop(t) && currentTypeOK && declaredMemberAcceptsCurrent(typeValues, currentType, t) {
			continue
		}
		out[key] = typeWitnessValue(reg, typeValues, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func declaredMemberAcceptsCurrent(typeValues *typevalue.Cache, current, declared typ.Type) bool {
	if typeValues != nil {
		return typeValues.IsSubtype(current, declared)
	}
	return typevalue.NewCache().IsSubtype(current, declared)
}

func declaredTypeContainsBoundaryTop(t typ.Type) bool {
	return refinement.ContainsBoundaryTop(t)
}

func valueContainsFreeTypeParam(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && refinement.ContainsFreeTypeParam(t)
}

// padMissingResultsToNil fills nil for result slots a call site consumes beyond
// the callee's declared return arity. A callee declared to return declaredCount
// values yields nil for any further destructuring target, matching Lua runtime
// semantics. It applies only when the callee's declared return arity is known
// and finite (declaredCount comes from the resolved function type); an
// unresolved or unknown-arity callee never reaches here.
func padMissingResultsToNil(reg *axis.Registry, site factflow.CallSiteView, results []callpayload.CallResult, declaredCount int) []callpayload.CallResult {
	if reg == nil || declaredCount < 0 {
		return results
	}
	present := make(map[int]struct{}, len(results))
	for _, result := range results {
		present[result.Index] = struct{}{}
	}
	nilValue := typevalue.Nil(reg)
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		index := target.ResultIndex()
		if index < declaredCount {
			return true
		}
		if _, ok := present[index]; ok {
			return true
		}
		present[index] = struct{}{}
		results = append(results, callpayload.CallResult{Index: index, Value: nilValue})
		return true
	})
	return results
}
