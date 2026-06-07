package flow

import "github.com/wippyai/go-lua/types/constraint"

// BoundaryAddressProjector maps point-local stable addresses into boundary
// paths. Different callers can project to return slots or to direct-call
// parameter slots, while key-presence owns stored-key decoding.
type BoundaryAddressProjector interface {
	PathsFromAddress(StableAddress) []BoundaryPath
}

// KeyPresenceBoundaryProjection configures boundary projection of key-presence
// facts. Pending key arrays are a point-summary postcondition; direct call entry
// projection preserves the historical behavior and leaves them out.
type KeyPresenceBoundaryProjection struct {
	IncludePendingKeyArrays bool
}

// ProjectKeyPresenceBoundaryFacts projects point-local key-presence facts into
// boundary-relative facts using projector as the sole address rebasing authority.
func ProjectKeyPresenceBoundaryFacts(
	f KeyPresenceFacts,
	projector BoundaryAddressProjector,
	cfg KeyPresenceBoundaryProjection,
) BoundaryFacts {
	if f.IsBottom() || projector == nil {
		return BoundaryFactsDomain.Top()
	}
	var keyPresence []BoundaryKeyPresenceFact
	for _, fact := range f.Entries() {
		for _, table := range boundaryPathsFromStoredKey(projector, fact.Table) {
			for _, key := range boundaryPathsFromStoredKey(projector, fact.Key) {
				keyPresence = append(keyPresence, BoundaryKeyPresenceFact{Table: table, Key: key})
			}
		}
	}

	var keyArrays []BoundaryKeyArrayFact
	for _, fact := range f.KeyArrayEntries() {
		for _, array := range boundaryPathsFromStoredKey(projector, fact.Array) {
			for _, table := range boundaryPathsFromStoredKey(projector, fact.Table) {
				keyArrays = append(keyArrays, BoundaryKeyArrayFact{Array: array, Table: table})
			}
		}
	}

	var keyArrayValues []BoundaryKeyArrayValueFact
	for _, fact := range f.KeyArrayValueEntries() {
		for _, array := range boundaryPathsFromStoredKey(projector, fact.Array) {
			for _, table := range boundaryPathsFromStoredKey(projector, fact.Table) {
				keyArrayValues = append(keyArrayValues, BoundaryKeyArrayValueFact{
					Array: array,
					Table: table,
					Value: fact.Value,
				})
			}
		}
	}

	var appendKeys []BoundaryAppendKeyFact
	for _, fact := range f.AppendedKeyEntries() {
		for _, array := range boundaryPathsFromStoredKey(projector, fact.Array) {
			for _, key := range boundaryPathsFromStoredKey(projector, fact.Key) {
				appendKeys = append(appendKeys, BoundaryAppendKeyFact{Array: array, Key: key})
			}
		}
	}

	var appendOrigins []BoundaryAppendElementFieldOriginFact
	for _, fact := range f.AppendElementFieldOriginEntries() {
		field, ok := AppendElementFieldSegments(fact.Field)
		if !ok {
			continue
		}
		sourceField, _ := AppendElementFieldSegments(fact.SourceField)
		for _, array := range boundaryPathsFromStoredKey(projector, fact.Array) {
			for _, source := range boundaryPathsFromStoredKey(projector, fact.Source) {
				appendOrigins = append(appendOrigins, BoundaryAppendElementFieldOriginFact{
					Array:       array,
					Field:       field,
					Source:      source,
					SourceField: sourceField,
				})
			}
		}
	}

	if cfg.IncludePendingKeyArrays {
		appendKeys = projectPendingKeyArraysToBoundary(f, projector, appendKeys)
	}

	return BoundaryFactsOf(keyPresence, keyArrays, keyArrayValues, appendKeys, nil, nil).
		WithAppendElementFieldOrigins(appendOrigins)
}

func projectPendingKeyArraysToBoundary(
	f KeyPresenceFacts,
	projector BoundaryAddressProjector,
	appendKeys []BoundaryAppendKeyFact,
) []BoundaryAppendKeyFact {
	for _, fact := range f.PendingKeyArrayEntries() {
		arrays := boundaryPathsFromStoredKey(projector, fact.Array)
		if len(arrays) == 0 {
			continue
		}
		keys := boundaryPathsFromStoredKey(projector, fact.Key)
		if len(keys) == 0 {
			continue
		}
		var tables []BoundaryPath
		if fact.Table != "" {
			tables = boundaryPathsFromStoredKey(projector, fact.Table)
			if len(tables) == 0 {
				continue
			}
		}
		for _, array := range arrays {
			for _, key := range keys {
				if len(tables) == 0 {
					appendKeys = append(appendKeys, BoundaryAppendKeyFact{Array: array, Key: key})
					continue
				}
				for _, table := range tables {
					appendKeys = append(appendKeys, BoundaryAppendKeyFact{
						Array:    array,
						Key:      key,
						Table:    table,
						HasTable: true,
					})
				}
			}
		}
	}
	return appendKeys
}

func boundaryPathsFromStoredKey(projector BoundaryAddressProjector, key constraint.PathKey) []BoundaryPath {
	addr, ok := StableAddressFromCanonicalKey(key)
	if !ok {
		return nil
	}
	return projector.PathsFromAddress(addr)
}
