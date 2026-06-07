package flow

import (
	"github.com/wippyai/go-lua/types/access"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AppendOriginDestination is a routed destination for appended element-field
// origins. FieldPrefix is the suffix already accumulated when the append is
// reached through an iterator or assignment alias.
type AppendOriginDestination struct {
	Array       StableAddress
	FieldPrefix []constraint.Segment
}

// AppendOriginSource is a routed source for one appended field origin.
// SourceField is non-empty when the source path denotes the containing element
// and the field suffix is relative to that element.
type AppendOriginSource struct {
	Source      StableAddress
	SourceField []constraint.Segment
}

// AppendElementFieldOriginPathSource is producer-facing syntax evidence for one
// field carried by an appended element. Flow owns routing the source through
// aliases/origins and projecting it to every live append destination.
type AppendElementFieldOriginPathSource struct {
	Field      []constraint.Segment
	SourcePath constraint.Path
}

// AppendElementFieldOriginSource is the normalized, routed source set for one
// field carried by an appended element.
type AppendElementFieldOriginSource struct {
	Field   []constraint.Segment
	Sources []AppendOriginSource
}

// PendingKeyArrayDestination is a delayed append consequence. When HasTable is
// false, the pending fact waits for any table that is later proven to contain
// Key.
type PendingKeyArrayDestination struct {
	Table    StableAddress
	HasTable bool
}

// AppendKeyArrayConsequences is the reduced-product transaction for appending a
// key into an array whose elements may be table keys.
type AppendKeyArrayConsequences struct {
	Array    StableAddress
	Key      StableAddress
	HasKey   bool
	KeyValue product.AbstractValue

	Tables  []StableAddress
	Pending []PendingKeyArrayDestination
}

// AppendKeyArrayTableQuery selects the tables for which appending Key into
// Array proves or delays key-array provenance.
type AppendKeyArrayTableQuery struct {
	Array            StableAddress
	Key              StableAddress
	ExplicitTable    StableAddress
	HasExplicitTable bool
	WrittenTables    []StableAddress
	FreshEmpty       bool
}

// appendKeyArrayPathTableQuery is the structured-path form of
// AppendKeyArrayTableQuery.
type appendKeyArrayPathTableQuery struct {
	ArrayPath         constraint.Path
	KeyPath           constraint.Path
	ExplicitTablePath constraint.Path
	HasExplicitTable  bool
	WrittenTablePaths []constraint.Path
	FreshEmpty        bool
}

// AppendKeyReplayPathTransaction is the source-facing transaction for an append
// write. It is normalized against the pre-write point state before the address
// transaction applies invalidation and replay.
type AppendKeyReplayPathTransaction struct {
	ArrayPath           constraint.Path
	KeyPath             constraint.Path
	ExplicitTablePath   constraint.Path
	HasExplicitTable    bool
	WrittenTablePaths   []constraint.Path
	FreshEmpty          bool
	PreserveHistoryBase bool
}

// AppendKeyReplayTransaction is the address-space append-key replay
// transaction. All pre-write evidence has already been selected by the path
// boundary, so application cannot accidentally read facts after invalidation.
type AppendKeyReplayTransaction struct {
	Array               StableAddress
	Key                 StableAddress
	KeyValue            product.AbstractValue
	PreserveHistoryBase bool
	Tables              []StableAddress
}

// AppendElementMutationPathTransaction is the source-facing transaction for
// appending one element to a collection. It is normalized against pre-write
// point state before the address transaction applies invalidation and replay.
type AppendElementMutationPathTransaction struct {
	Footprint access.WriteFootprint
	ArrayPath constraint.Path

	ElementPath  constraint.Path
	ElementValue product.AbstractValue

	FieldSources []AppendElementFieldOriginPathSource
}

// AppendElementMutationTransaction is the address-space append mutation
// transaction. All pre-write evidence has already been selected by the path
// boundary, so applying it cannot accidentally read facts after invalidation.
type AppendElementMutationTransaction struct {
	Footprint access.WriteFootprint
	Array     StableAddress

	PreserveHistoryBase bool
	Destinations        []AppendOriginDestination

	Element      StableAddress
	HasElement   bool
	ElementValue product.AbstractValue

	FieldSources []AppendElementFieldOriginSource
	Selection    AppendKeyArraySelection
}

// AppendKeyArraySelection is the direct plus delayed consequence set for an
// append into a possible key array.
type AppendKeyArraySelection struct {
	Tables  []StableAddress
	Pending []PendingKeyArrayDestination
}

// AppendKeyArrayPreservationQuery selects consequences for appending Key into
// an array whose existing elements may already be keys of some table.
type AppendKeyArrayPreservationQuery struct {
	Array          StableAddress
	Key            StableAddress
	FreshEmptySeed bool
}

// AppendOriginDestinations follows value-origin and path-alias facts to every
// array whose appended element field may be observed through array.
func AppendOriginDestinations(state PointState, array StableAddress, fieldPrefix []constraint.Segment) []AppendOriginDestination {
	if array.Key() == "" {
		return nil
	}
	seen := map[string]bool{}
	var destinations []AppendOriginDestination
	var add func(StableAddress, []constraint.Segment)
	add = func(array StableAddress, prefix []constraint.Segment) {
		key := array.Key()
		seenKey := string(key) + "/" + string(AppendElementFieldPathKey(prefix))
		if key == "" || seen[seenKey] {
			return
		}
		seen[seenKey] = true
		destinations = append(destinations, AppendOriginDestination{
			Array:       array,
			FieldPrefix: cloneAddressSegments(prefix),
		})
		for _, route := range state.ValueOrigins.indexedIteratorValueSourceRoutesCoveringAddress(array) {
			nextPrefix := cloneAddressSegments(route.remainder)
			nextPrefix = append(nextPrefix, prefix...)
			add(route.source, nextPrefix)
		}
		for _, route := range state.PathAliases.sourceRoutesCoveringAddress(array) {
			source, ok := route.appendedSource()
			if ok {
				add(source, prefix)
			}
		}
	}
	add(array, fieldPrefix)
	return destinations
}

func AppendOriginDestinationsPath(state PointState, arrayPath constraint.Path, fieldPrefix []constraint.Segment) []AppendOriginDestination {
	array, ok := StableAddressOfPath(arrayPath)
	if !ok {
		return nil
	}
	return AppendOriginDestinations(state, array, fieldPrefix)
}

// ApplyAppendKeyArrayConsequences applies direct and delayed key-array facts
// after an append. When a matching index-write readback fact exists, it also
// publishes the value-carrying key-array proof.
func ApplyAppendKeyArrayConsequences(out *PointState, proof AppendKeyArrayConsequences) bool {
	if out == nil || proof.Array.Key() == "" || (len(proof.Tables) == 0 && len(proof.Pending) == 0) {
		return false
	}
	changed := false
	for _, table := range proof.Tables {
		if table.Key() == "" {
			continue
		}
		changed = ApplyKeyArrayProof(out, KeyArrayProof{
			Array: proof.Array,
			Table: table,
		}) || changed
		if !proof.HasKey || proof.Key.Key() == "" {
			continue
		}
		keyType := product.ProjectValueOrUnknown(proof.KeyValue)
		if keyType == nil {
			keyType = typ.Unknown
		}
		value, ok := out.IndexWrites.AdmissionAtAddress(IndexWriteAddressQuery{
			Target:     table,
			KeyPath:    proof.Key,
			HasKeyPath: true,
			KeyValue:   product.FromType(keyType),
		})
		if !ok || value.IsZero() {
			continue
		}
		changed = ApplyKeyArrayValueProof(out, KeyArrayValueProof{
			Array:        proof.Array,
			Table:        table,
			Value:        value,
			AppendKey:    proof.Key,
			HasAppendKey: true,
		}) || changed
	}
	for _, pending := range proof.Pending {
		if !proof.HasKey || proof.Key.Key() == "" {
			continue
		}
		pendingProof := PendingKeyArrayProof{
			Array: proof.Array,
			Key:   proof.Key,
		}
		if pending.HasTable {
			if pending.Table.Key() == "" {
				continue
			}
			pendingProof.Table = pending.Table
			pendingProof.HasTable = true
		}
		changed = ApplyPendingKeyArrayProof(out, pendingProof) || changed
	}
	return changed
}

// AppendHistoryBaseWithoutEvents reports whether Array still has a tracked
// append-history base with no recorded append events.
func AppendHistoryBaseWithoutEvents(state PointState, array StableAddress) bool {
	return state.KeyPresence.HasAppendHistoryBaseWithoutEventsAddress(array)
}

func AppendHistoryBaseWithoutEventsPath(state PointState, arrayPath constraint.Path) bool {
	array, ok := StableAddressOfPath(arrayPath)
	return ok && AppendHistoryBaseWithoutEvents(state, array)
}

// AppendKeyArrayTables selects concrete key-array consequence tables from the
// current key-presence state and boundary write evidence.
func AppendKeyArrayTables(state PointState, q AppendKeyArrayTableQuery) []StableAddress {
	arrayKey := q.Array.Key()
	keyKey := q.Key.Key()
	if arrayKey == "" || keyKey == "" {
		return nil
	}
	add := func(out []StableAddress, table StableAddress) []StableAddress {
		if table.Key() == "" {
			return out
		}
		for _, existing := range out {
			if existing.Equal(table) {
				return out
			}
		}
		return append(out, table)
	}
	existingTables := state.KeyPresence.KeyArrayTableAddresses(q.Array)
	var out []StableAddress
	if q.HasExplicitTable {
		tableKey := q.ExplicitTable.Key()
		if tableKey == "" {
			return nil
		}
		hasExisting := false
		for _, existing := range existingTables {
			if existing.Key == tableKey {
				hasExisting = true
				break
			}
		}
		if q.FreshEmpty || hasExisting {
			out = add(out, q.ExplicitTable)
		}
		return out
	}
	for _, tableUse := range existingTables {
		out = add(out, tableUse.Address)
	}
	if q.FreshEmpty {
		for _, table := range q.WrittenTables {
			out = add(out, table)
		}
		for _, tableUse := range state.KeyPresence.TablesWithKeyAddress(q.Key) {
			out = add(out, tableUse.Address)
		}
	}
	return out
}

func appendKeyArrayTablesOfPath(state PointState, q appendKeyArrayPathTableQuery) []StableAddress {
	array, arrayOK := StableAddressOfPath(q.ArrayPath)
	key, keyOK := StableAddressOfPath(q.KeyPath)
	if !arrayOK || !keyOK {
		return nil
	}
	addressQuery := AppendKeyArrayTableQuery{
		Array:            array,
		Key:              key,
		HasExplicitTable: q.HasExplicitTable,
		FreshEmpty:       q.FreshEmpty,
	}
	if q.HasExplicitTable {
		table, ok := StableAddressOfPath(q.ExplicitTablePath)
		if !ok {
			return nil
		}
		addressQuery.ExplicitTable = table
	}
	if len(q.WrittenTablePaths) > 0 {
		addressQuery.WrittenTables = make([]StableAddress, 0, len(q.WrittenTablePaths))
		for _, tablePath := range q.WrittenTablePaths {
			table, ok := StableAddressOfPath(tablePath)
			if ok {
				addressQuery.WrittenTables = append(addressQuery.WrittenTables, table)
			}
		}
	}
	return AppendKeyArrayTables(state, addressQuery)
}

// AppendKeyReplayTransactionOfPath selects pre-write evidence for an append-key
// replay and lowers stable paths once.
func AppendKeyReplayTransactionOfPath(state PointState, tx AppendKeyReplayPathTransaction) (AppendKeyReplayTransaction, bool) {
	if tx.ArrayPath.IsEmpty() || tx.KeyPath.IsEmpty() {
		return AppendKeyReplayTransaction{}, false
	}
	array, arrayOK := StableAddressOfPath(tx.ArrayPath)
	key, keyOK := StableAddressOfPath(tx.KeyPath)
	if !arrayOK || !keyOK {
		return AppendKeyReplayTransaction{}, false
	}
	facts := PointFactsOf(state)
	keyValue, _ := facts.PathValue(tx.KeyPath)
	return AppendKeyReplayTransaction{
		Array:               array,
		Key:                 key,
		KeyValue:            keyValue,
		PreserveHistoryBase: tx.PreserveHistoryBase || tx.FreshEmpty || facts.HasAppendHistoryBase(tx.ArrayPath),
		Tables: appendKeyArrayTablesOfPath(state, appendKeyArrayPathTableQuery{
			ArrayPath:         tx.ArrayPath,
			KeyPath:           tx.KeyPath,
			ExplicitTablePath: tx.ExplicitTablePath,
			HasExplicitTable:  tx.HasExplicitTable,
			WrittenTablePaths: tx.WrittenTablePaths,
			FreshEmpty:        tx.FreshEmpty,
		}),
	}, true
}

// ApplyAppendKeyReplayPathTransaction normalizes a source-level append replay
// against pre-write state, then applies the address transaction.
func ApplyAppendKeyReplayPathTransaction(out *PointState, tx AppendKeyReplayPathTransaction) bool {
	if out == nil {
		return false
	}
	normalized, ok := AppendKeyReplayTransactionOfPath(*out, tx)
	if !ok {
		return false
	}
	return ApplyAppendKeyReplayTransaction(out, normalized)
}

// ApplyAppendKeyReplayTransaction applies all address-fact consequences of a
// local append-key replay using pre-selected evidence.
func ApplyAppendKeyReplayTransaction(out *PointState, tx AppendKeyReplayTransaction) bool {
	if out == nil || tx.Array.Key() == "" || tx.Key.Key() == "" {
		return false
	}
	changed := ApplyAddressWriteInvalidation(out, AddressWriteInvalidation{Write: tx.Array})
	if tx.PreserveHistoryBase {
		changed = ApplyAppendHistoryBaseProof(out, AppendHistoryBaseProof{Array: tx.Array}) || changed
	}
	changed = ApplyAppendKeyProof(out, AppendKeyProof{
		Array: tx.Array,
		Key:   tx.Key,
	}) || changed
	if len(tx.Tables) > 0 {
		changed = ApplyAppendKeyArrayConsequences(out, AppendKeyArrayConsequences{
			Array:    tx.Array,
			Key:      tx.Key,
			HasKey:   true,
			KeyValue: tx.KeyValue,
			Tables:   tx.Tables,
		}) || changed
	}
	return changed
}

// AppendElementMutationTransactionOfPath selects pre-write evidence for a
// source-facing append mutation transaction and lowers stable paths once.
func AppendElementMutationTransactionOfPath(state PointState, tx AppendElementMutationPathTransaction) (AppendElementMutationTransaction, bool) {
	if tx.ArrayPath.IsEmpty() {
		return AppendElementMutationTransaction{}, false
	}
	array, arrayOK := StableAddressOfPath(tx.ArrayPath)
	if !arrayOK {
		return AppendElementMutationTransaction{}, false
	}
	facts := PointFactsOf(state)
	out := AppendElementMutationTransaction{
		Footprint:           tx.Footprint,
		Array:               array,
		PreserveHistoryBase: facts.HasAppendHistoryBase(tx.ArrayPath),
		Destinations:        AppendOriginDestinations(state, array, nil),
		ElementValue:        tx.ElementValue,
		FieldSources:        appendElementFieldOriginSourcesOfPath(state, tx.FieldSources),
	}
	freshEmptySeed := AppendFreshEmptySeedPath(state, tx.ArrayPath)
	if !tx.ElementPath.IsEmpty() {
		if key, ok := StableAddressOfPath(tx.ElementPath); ok {
			out.Element = key
			out.HasElement = true
			out.Selection = AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
				Array:          array,
				Key:            key,
				FreshEmptySeed: freshEmptySeed,
			})
		}
	}
	return out, true
}

// ApplyAppendElementMutationPathTransaction normalizes a source-level append
// mutation against pre-write state, then applies the address transaction.
func ApplyAppendElementMutationPathTransaction(out *PointState, tx AppendElementMutationPathTransaction) bool {
	if out == nil {
		return false
	}
	normalized, ok := AppendElementMutationTransactionOfPath(*out, tx)
	if !ok {
		return false
	}
	return ApplyAppendElementMutationTransaction(out, normalized)
}

// ApplyAppendElementMutationTransaction applies all address-fact consequences of
// a local append mutation using pre-selected evidence.
func ApplyAppendElementMutationTransaction(out *PointState, tx AppendElementMutationTransaction) bool {
	if out == nil || tx.Array.Key() == "" {
		return false
	}
	changed := ApplyAccessMutation(out, AccessMutation{
		Footprint:     tx.Footprint,
		StaticMembers: true,
		Conditions:    true,
		AddressFacts:  true,
	})
	if tx.PreserveHistoryBase {
		changed = ApplyAppendHistoryBaseProof(out, AppendHistoryBaseProof{Array: tx.Array}) || changed
	}
	if tx.HasElement {
		changed = ApplyAppendKeyProof(out, AppendKeyProof{Array: tx.Array, Key: tx.Element}) || changed
	}
	changed = applyAppendElementFieldOriginSources(out, tx.Destinations, tx.FieldSources) || changed
	if tx.HasElement {
		changed = replayAppendElementFieldOriginUses(out, tx.Destinations, tx.Element) || changed
	}
	if len(tx.Selection.Tables) > 0 || len(tx.Selection.Pending) > 0 {
		changed = ApplyAppendKeyArrayConsequences(out, AppendKeyArrayConsequences{
			Array:    tx.Array,
			Key:      tx.Element,
			HasKey:   tx.HasElement,
			KeyValue: tx.ElementValue,
			Tables:   tx.Selection.Tables,
			Pending:  tx.Selection.Pending,
		}) || changed
	}
	return changed
}

func appendElementFieldOriginSourcesOfPath(state PointState, sources []AppendElementFieldOriginPathSource) []AppendElementFieldOriginSource {
	if len(sources) == 0 {
		return nil
	}
	var out []AppendElementFieldOriginSource
	for _, source := range sources {
		if len(source.Field) == 0 || source.SourcePath.IsEmpty() {
			continue
		}
		routed := AppendOriginSourcesPath(state, source.SourcePath)
		if len(routed) == 0 {
			continue
		}
		out = append(out, AppendElementFieldOriginSource{
			Field:   cloneAddressSegments(source.Field),
			Sources: routed,
		})
	}
	return out
}

func applyAppendElementFieldOriginSources(
	out *PointState,
	destinations []AppendOriginDestination,
	sources []AppendElementFieldOriginSource,
) bool {
	if out == nil || len(destinations) == 0 || len(sources) == 0 {
		return false
	}
	changed := false
	for _, source := range sources {
		if len(source.Field) == 0 || len(source.Sources) == 0 {
			continue
		}
		for _, dst := range destinations {
			field := append(cloneAddressSegments(dst.FieldPrefix), source.Field...)
			for _, src := range source.Sources {
				changed = ApplyAppendElementFieldOriginProof(out, AppendElementFieldOriginProof{
					Array:       dst.Array,
					Field:       field,
					Source:      src.Source,
					SourceField: src.SourceField,
				}) || changed
			}
		}
	}
	return changed
}

func replayAppendElementFieldOriginUses(out *PointState, destinations []AppendOriginDestination, element StableAddress) bool {
	if out == nil || len(destinations) == 0 || element.Key() == "" {
		return false
	}
	changed := false
	for _, field := range AppendElementFieldOriginFields(*out) {
		elementField, ok := element.Append(field)
		if !ok {
			continue
		}
		for _, originUse := range AppendElementFieldOriginUses(*out, elementField) {
			changed = ApplyAppendElementFieldOriginUse(out, destinations, field, originUse) || changed
		}
	}
	return changed
}

// AppendFreshEmptySeedPath reports whether appending to array can seed key-array
// provenance from an empty collection/history base.
func AppendFreshEmptySeedPath(state PointState, arrayPath constraint.Path) bool {
	array, ok := StableAddressOfPath(arrayPath)
	if !ok {
		return false
	}
	facts := PointFactsOf(state)
	return facts.HasEmptyKeyArray(arrayPath) ||
		AppendHistoryBaseWithoutEvents(state, array) ||
		pathValueIsFreshEmptySequence(facts, arrayPath)
}

func pathValueIsFreshEmptySequence(facts PointFacts, path constraint.Path) bool {
	av, ok := facts.PathValue(path)
	if !ok || av.IsZero() || !av.DefinitelyPresent() {
		return false
	}
	return productValueIsFreshEmptySequence(av)
}

func productValueIsFreshEmptySequence(av product.AbstractValue) bool {
	t := unwrap.Alias(av.ProjectValue())
	switch v := t.(type) {
	case *typ.Array:
		return v.Fresh || typ.IsNever(v.Element)
	case *typ.Record:
		return len(v.Fields) == 0 &&
			len(v.StaticMembers) == 0 &&
			!v.HasMapComponent() &&
			!v.Open &&
			v.Metatable == nil
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !productTypeIsFreshEmptySequence(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func productTypeIsFreshEmptySequence(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return v.Fresh || typ.IsNever(v.Element)
	case *typ.Record:
		return len(v.Fields) == 0 &&
			len(v.StaticMembers) == 0 &&
			!v.HasMapComponent() &&
			!v.Open &&
			v.Metatable == nil
	default:
		return false
	}
}

// AppendKeyArrayPreservation selects direct and delayed key-array consequences
// for a local append into Array.
func AppendKeyArrayPreservation(state PointState, q AppendKeyArrayPreservationQuery) AppendKeyArraySelection {
	arrayKey := q.Array.Key()
	keyKey := q.Key.Key()
	if arrayKey == "" || keyKey == "" {
		return AppendKeyArraySelection{}
	}
	addTable := func(out []StableAddress, table StableAddress) []StableAddress {
		if table.Key() == "" {
			return out
		}
		for _, existing := range out {
			if existing.Equal(table) {
				return out
			}
		}
		return append(out, table)
	}
	addPending := func(out []PendingKeyArrayDestination, pending PendingKeyArrayDestination) []PendingKeyArrayDestination {
		if pending.HasTable && pending.Table.Key() == "" {
			return out
		}
		for _, existing := range out {
			if existing.HasTable == pending.HasTable && (!existing.HasTable || existing.Table.Equal(pending.Table)) {
				return out
			}
		}
		return append(out, pending)
	}
	existingTables := state.KeyPresence.KeyArrayTableAddresses(q.Array)
	canSeedFromEmpty := len(existingTables) == 0 && q.FreshEmptySeed
	var out AppendKeyArraySelection
	if canSeedFromEmpty {
		out.Pending = addPending(out.Pending, PendingKeyArrayDestination{})
	}
	for _, tableUse := range existingTables {
		if state.KeyPresence.HasAddresses(tableUse.Address, q.Key) {
			out.Tables = addTable(out.Tables, tableUse.Address)
			continue
		}
		out.Pending = addPending(out.Pending, PendingKeyArrayDestination{
			Table:    tableUse.Address,
			HasTable: true,
		})
	}
	for _, tableUse := range state.KeyPresence.TablesWithKeyAddress(q.Key) {
		hasExisting := false
		for _, existing := range existingTables {
			if existing.Key == tableUse.Key {
				hasExisting = true
				break
			}
		}
		if !hasExisting && !canSeedFromEmpty {
			continue
		}
		out.Tables = addTable(out.Tables, tableUse.Address)
	}
	return out
}

// AppendOriginSources follows value-origin and path-alias facts backward from
// source to every path that may have supplied an appended element field.
func AppendOriginSources(state PointState, source StableAddress) []AppendOriginSource {
	if source.Key() == "" {
		return nil
	}
	var sources []AppendOriginSource
	add := func(source StableAddress, sourceField []constraint.Segment) {
		if source.Key() == "" {
			return
		}
		sources = append(sources, AppendOriginSource{
			Source:      source,
			SourceField: cloneAddressSegments(sourceField),
		})
	}
	add(source, nil)
	for _, route := range state.ValueOrigins.indexedIteratorValueSourceRoutesCoveringAddress(source) {
		add(route.source, route.remainder)
	}
	for _, route := range state.ValueOrigins.assignmentAliasSourceRoutesCoveringAddress(source) {
		routed, ok := route.appendedSource()
		if ok {
			add(routed, nil)
		}
	}
	for _, route := range state.PathAliases.sourceRoutesCoveringAddress(source) {
		routed, ok := route.appendedSource()
		if ok {
			add(routed, nil)
		}
	}
	return sources
}

func AppendOriginSourcesPath(state PointState, sourcePath constraint.Path) []AppendOriginSource {
	source, ok := StableAddressOfPath(sourcePath)
	if !ok {
		return nil
	}
	return AppendOriginSources(state, source)
}

// AppendElementFieldOriginUses returns value-origin uses for an appended
// element field, including origins reached through path aliases.
func AppendElementFieldOriginUses(state PointState, field StableAddress) []ValueOriginUse {
	if field.Key() == "" {
		return nil
	}
	uses := append([]ValueOriginUse(nil), state.ValueOrigins.OriginsCoveringAddress(field)...)
	for _, route := range state.PathAliases.sourceRoutesCoveringAddress(field) {
		source, ok := route.appendedSource()
		if !ok {
			continue
		}
		uses = append(uses, state.ValueOrigins.OriginsCoveringAddress(source)...)
	}
	return uses
}

func AppendElementFieldOriginUsesPath(state PointState, fieldPath constraint.Path) []ValueOriginUse {
	field, ok := StableAddressOfPath(fieldPath)
	if !ok {
		return nil
	}
	return AppendElementFieldOriginUses(state, field)
}

// AppendElementFieldOriginFields returns the recorded appended-element fields as
// structured suffixes. Flow owns the fact-key parsing so transfer can replay
// append-origin implications without inspecting KeyPresence storage keys.
func AppendElementFieldOriginFields(state PointState) [][]constraint.Segment {
	seen := map[constraint.PathKey]struct{}{}
	var out [][]constraint.Segment
	for _, fact := range state.KeyPresence.AppendElementFieldOriginEntries() {
		field, ok := AppendElementFieldSegments(fact.Field)
		if !ok || len(field) == 0 {
			continue
		}
		key := AppendElementFieldPathKey(field)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneAddressSegments(field))
	}
	return out
}

// ApplyAppendElementFieldOriginUse replays a prior field-origin use into the
// current append destinations.
func ApplyAppendElementFieldOriginUse(
	out *PointState,
	destinations []AppendOriginDestination,
	field []constraint.Segment,
	originUse ValueOriginUse,
) bool {
	if out == nil {
		return false
	}
	if originUse.Origin.Kind != ValueOriginIndexedIterator || originUse.Origin.VarIndex != 1 || len(originUse.Remainder) == 0 {
		return false
	}
	before := out.KeyPresence
	originSource, ok := originUse.SourceAddress()
	if !ok {
		return false
	}
	for _, sourceUse := range out.KeyPresence.AppendElementFieldSourceAddresses(AppendElementFieldSourceQuery{
		Array: originSource,
		Field: originUse.Remainder,
	}) {
		source := sourceUse.Source
		sourceField := cloneAddressSegments(sourceUse.SourceField)
		if len(sourceField) > 0 {
			sourceField = append(sourceField, sourceUse.FieldRemainder...)
		} else {
			var ok bool
			source, ok = source.Append(sourceUse.FieldRemainder)
			if !ok {
				continue
			}
		}
		for _, dst := range destinations {
			dstField := cloneAddressSegments(dst.FieldPrefix)
			dstField = append(dstField, field...)
			ApplyAppendElementFieldOriginProof(out, AppendElementFieldOriginProof{
				Array:       dst.Array,
				Field:       dstField,
				Source:      source,
				SourceField: sourceField,
			})
		}
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}
