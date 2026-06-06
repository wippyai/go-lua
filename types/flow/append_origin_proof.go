package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
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

// AppendKeyArrayPathConsequences is the producer-facing path form for append
// key-array consequences. Tables and pending destinations may remain
// address-native when they came from prior flow queries.
type AppendKeyArrayPathConsequences struct {
	ArrayPath constraint.Path
	KeyPath   constraint.Path
	HasKey    bool
	KeyValue  product.AbstractValue

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

// AppendKeyArrayPathPreservationQuery selects append preservation consequences
// from structured paths.
type AppendKeyArrayPathPreservationQuery struct {
	ArrayPath      constraint.Path
	KeyPath        constraint.Path
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
		for _, use := range state.ValueOrigins.OriginsCoveringAddress(array) {
			if use.Origin.Kind != ValueOriginIndexedIterator || use.Origin.VarIndex != 1 || len(use.Remainder) == 0 {
				continue
			}
			source, ok := StableAddressFromKey(use.Origin.Source)
			if !ok {
				continue
			}
			nextPrefix := cloneAddressSegments(use.Remainder)
			nextPrefix = append(nextPrefix, prefix...)
			add(source, nextPrefix)
		}
		for _, aliasUse := range state.PathAliases.AliasesCoveringAddress(array) {
			source, ok := StableAddressFromKey(aliasUse.Alias.Source)
			if !ok {
				continue
			}
			source, ok = source.Append(aliasUse.Remainder)
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

func ApplyAppendKeyArrayPathConsequences(out *PointState, proof AppendKeyArrayPathConsequences) bool {
	array, arrayOK := StableAddressOfPath(proof.ArrayPath)
	if !arrayOK {
		return false
	}
	key := StableAddress{}
	keyOK := false
	if proof.HasKey {
		key, keyOK = StableAddressOfPath(proof.KeyPath)
	}
	return ApplyAppendKeyArrayConsequences(out, AppendKeyArrayConsequences{
		Array:    array,
		Key:      key,
		HasKey:   proof.HasKey && keyOK,
		KeyValue: proof.KeyValue,
		Tables:   proof.Tables,
		Pending:  proof.Pending,
	})
}

// AppendHistoryBaseWithoutEvents reports whether Array still has a tracked
// append-history base with no recorded append events.
func AppendHistoryBaseWithoutEvents(state PointState, array StableAddress) bool {
	arrayKey := array.Key()
	if arrayKey == "" || !state.KeyPresence.HasAppendHistoryBase(arrayKey) {
		return false
	}
	for _, event := range state.KeyPresence.AppendHistoryEventEntries() {
		if event.Array == arrayKey {
			return false
		}
	}
	return true
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
	existingTables := state.KeyPresence.KeyArrayTables(arrayKey)
	var out []StableAddress
	if q.HasExplicitTable {
		tableKey := q.ExplicitTable.Key()
		if tableKey == "" {
			return nil
		}
		hasExisting := false
		for _, existing := range existingTables {
			if existing == tableKey {
				hasExisting = true
				break
			}
		}
		if q.FreshEmpty || hasExisting {
			out = add(out, q.ExplicitTable)
		}
		return out
	}
	for _, tableKey := range existingTables {
		table, ok := StableAddressFromKey(tableKey)
		if ok {
			out = add(out, table)
		}
	}
	if q.FreshEmpty {
		for _, table := range q.WrittenTables {
			out = add(out, table)
		}
		for _, fact := range state.KeyPresence.Entries() {
			if fact.Key != keyKey {
				continue
			}
			table, ok := StableAddressFromKey(fact.Table)
			if ok {
				out = add(out, table)
			}
		}
	}
	return out
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
	existingTables := state.KeyPresence.KeyArrayTables(arrayKey)
	canSeedFromEmpty := len(existingTables) == 0 && q.FreshEmptySeed
	var out AppendKeyArraySelection
	if canSeedFromEmpty {
		out.Pending = addPending(out.Pending, PendingKeyArrayDestination{})
	}
	for _, tableKey := range existingTables {
		table, ok := StableAddressFromKey(tableKey)
		if !ok {
			continue
		}
		if state.KeyPresence.Has(tableKey, keyKey) {
			out.Tables = addTable(out.Tables, table)
			continue
		}
		out.Pending = addPending(out.Pending, PendingKeyArrayDestination{
			Table:    table,
			HasTable: true,
		})
	}
	for _, fact := range state.KeyPresence.Entries() {
		if fact.Key != keyKey {
			continue
		}
		hasExisting := false
		for _, existing := range existingTables {
			if existing == fact.Table {
				hasExisting = true
				break
			}
		}
		if !hasExisting && !canSeedFromEmpty {
			continue
		}
		table, ok := StableAddressFromKey(fact.Table)
		if ok {
			out.Tables = addTable(out.Tables, table)
		}
	}
	return out
}

func AppendKeyArrayPathPreservation(state PointState, q AppendKeyArrayPathPreservationQuery) AppendKeyArraySelection {
	array, arrayOK := StableAddressOfPath(q.ArrayPath)
	key, keyOK := StableAddressOfPath(q.KeyPath)
	if !arrayOK || !keyOK {
		return AppendKeyArraySelection{}
	}
	return AppendKeyArrayPreservation(state, AppendKeyArrayPreservationQuery{
		Array:          array,
		Key:            key,
		FreshEmptySeed: q.FreshEmptySeed,
	})
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
	for _, use := range state.ValueOrigins.OriginsCoveringAddress(source) {
		routed, ok := StableAddressFromKey(use.Origin.Source)
		if !ok {
			continue
		}
		switch use.Origin.Kind {
		case ValueOriginIndexedIterator:
			if use.Origin.VarIndex == 1 && len(use.Remainder) > 0 {
				add(routed, use.Remainder)
			}
		case ValueOriginAssignmentAlias:
			routed, ok = routed.Append(use.Remainder)
			if ok {
				add(routed, nil)
			}
		}
	}
	for _, aliasUse := range state.PathAliases.AliasesCoveringAddress(source) {
		routed, ok := StableAddressFromKey(aliasUse.Alias.Source)
		if !ok {
			continue
		}
		routed, ok = routed.Append(aliasUse.Remainder)
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
	for _, aliasUse := range state.PathAliases.AliasesCoveringAddress(field) {
		source, ok := StableAddressFromKey(aliasUse.Alias.Source)
		if !ok {
			continue
		}
		source, ok = source.Append(aliasUse.Remainder)
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
	for _, sourceUse := range out.KeyPresence.AppendElementFieldSources(originUse.Origin.Source, originUse.Remainder) {
		source, ok := StableAddressFromKey(sourceUse.Origin.Source)
		if !ok {
			continue
		}
		sourceField := cloneAddressSegments(sourceUse.SourceField)
		if len(sourceField) > 0 {
			sourceField = append(sourceField, sourceUse.FieldRemainder...)
		} else {
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
