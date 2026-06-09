package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

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
	paths := newBoundaryAddressPathCache(projector)
	return projectKeyPresenceBoundaryFactsWithPaths(f, paths, cfg)
}

func projectKeyPresenceBoundaryFactsWithPaths(
	f KeyPresenceFacts,
	paths boundaryAddressPathCache,
	cfg KeyPresenceBoundaryProjection,
) BoundaryFacts {
	if f.IsBottom() || paths.projector == nil {
		return BoundaryFactsDomain.Top()
	}
	var keyPresence []BoundaryKeyPresenceFact
	f.ForEachAddress(func(tableAddr, keyAddr StableAddress) bool {
		for _, table := range paths.fromAddress(tableAddr) {
			for _, key := range paths.fromAddress(keyAddr) {
				keyPresence = append(keyPresence, BoundaryKeyPresenceFact{Table: table, Key: key})
			}
		}
		return true
	})

	var keyArrays []BoundaryKeyArrayFact
	f.ForEachKeyArrayAddress(func(arrayAddr, tableAddr StableAddress) bool {
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, table := range paths.fromAddress(tableAddr) {
				keyArrays = append(keyArrays, BoundaryKeyArrayFact{Array: array, Table: table})
			}
		}
		return true
	})

	var keyArrayValues []BoundaryKeyArrayValueFact
	f.ForEachKeyArrayValueAddress(func(arrayAddr, tableAddr StableAddress, value product.AbstractValue) bool {
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, table := range paths.fromAddress(tableAddr) {
				keyArrayValues = append(keyArrayValues, BoundaryKeyArrayValueFact{
					Array: array,
					Table: table,
					Value: value,
				})
			}
		}
		return true
	})

	var appendKeys []BoundaryAppendKeyFact
	f.ForEachAppendedKeyAddress(func(arrayAddr, keyAddr StableAddress) bool {
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, key := range paths.fromAddress(keyAddr) {
				appendKeys = append(appendKeys, BoundaryAppendKeyFact{Array: array, Key: key})
			}
		}
		return true
	})

	var appendBases []BoundaryAppendHistoryBaseFact
	for _, fact := range f.AppendHistoryBaseEntries() {
		arrayAddr, ok := StableAddressFromCanonicalKey(fact.Array)
		if !ok {
			continue
		}
		for _, array := range paths.fromAddress(arrayAddr) {
			appendBases = append(appendBases, BoundaryAppendHistoryBaseFact{Array: array})
		}
	}

	var appendEvents []BoundaryAppendHistoryEventFact
	for _, fact := range f.AppendHistoryEventEntries() {
		arrayAddr, arrayOK := StableAddressFromCanonicalKey(fact.Array)
		keyAddr, keyOK := StableAddressFromCanonicalKey(fact.Key)
		if !arrayOK || !keyOK {
			continue
		}
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, key := range paths.fromAddress(keyAddr) {
				appendEvents = append(appendEvents, BoundaryAppendHistoryEventFact{Array: array, Key: key})
			}
		}
	}

	var appendCoverage []BoundaryAppendHistoryCoverageFact
	tableCoverageSeen := make(map[constraint.PathKey]struct{})
	var appendTableCoverage []BoundaryAppendHistoryTableCoverageFact
	for _, fact := range f.AppendHistoryCoverageEntries() {
		arrayAddr, arrayOK := StableAddressFromCanonicalKey(fact.Array)
		keyAddr, keyOK := StableAddressFromCanonicalKey(fact.Key)
		tableAddr, tableOK := StableAddressFromCanonicalKey(fact.Table)
		if !arrayOK || !tableOK {
			continue
		}
		if value, ok := f.AppendHistoryCoverageValue(fact.Array, fact.Table); ok && !value.IsZero() {
			coverageKey := fact.Array + "\x00" + fact.Table
			if _, seen := tableCoverageSeen[coverageKey]; !seen {
				tableCoverageSeen[coverageKey] = struct{}{}
				for _, array := range paths.fromAddress(arrayAddr) {
					for _, table := range paths.fromAddress(tableAddr) {
						appendTableCoverage = append(appendTableCoverage, BoundaryAppendHistoryTableCoverageFact{
							Array: array,
							Table: table,
							Value: value,
						})
					}
				}
			}
		}
		if !keyOK {
			continue
		}
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, key := range paths.fromAddress(keyAddr) {
				for _, table := range paths.fromAddress(tableAddr) {
					appendCoverage = append(appendCoverage, BoundaryAppendHistoryCoverageFact{
						Array: array,
						Key:   key,
						Table: table,
						Value: fact.Value,
					})
				}
			}
		}
	}

	var appendOrigins []BoundaryAppendElementFieldOriginFact
	f.ForEachAppendElementFieldOriginAddress(func(arrayAddr StableAddress, field []constraint.Segment, sourceAddr StableAddress, sourceField []constraint.Segment) bool {
		for _, array := range paths.fromAddress(arrayAddr) {
			for _, source := range paths.fromAddress(sourceAddr) {
				appendOrigins = append(appendOrigins, BoundaryAppendElementFieldOriginFact{
					Array:       array,
					Field:       cloneAddressSegments(field),
					Source:      source,
					SourceField: cloneAddressSegments(sourceField),
				})
			}
		}
		return true
	})

	if cfg.IncludePendingKeyArrays {
		appendKeys = projectPendingKeyArraysToBoundary(f, paths, appendKeys)
	}

	return BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:         keyPresence,
		KeyArrays:           keyArrays,
		KeyArrayValues:      keyArrayValues,
		AppendKeys:          appendKeys,
		AppendBases:         appendBases,
		AppendEvents:        appendEvents,
		AppendCoverage:      appendCoverage,
		AppendTableCoverage: appendTableCoverage,
		AppendOrigins:       appendOrigins,
	})
}

func projectPendingKeyArraysToBoundary(
	f KeyPresenceFacts,
	paths boundaryAddressPathCache,
	appendKeys []BoundaryAppendKeyFact,
) []BoundaryAppendKeyFact {
	f.ForEachPendingKeyArrayAddress(func(arrayAddr StableAddress, tableAddr StableAddress, hasTable bool, keyAddr StableAddress) bool {
		arrays := paths.fromAddress(arrayAddr)
		if len(arrays) == 0 {
			return true
		}
		keys := paths.fromAddress(keyAddr)
		if len(keys) == 0 {
			return true
		}
		var tables []BoundaryPath
		if hasTable {
			tables = paths.fromAddress(tableAddr)
			if len(tables) == 0 {
				return true
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
		return true
	})
	return appendKeys
}

type boundaryAddressPathCache struct {
	projector BoundaryAddressProjector
	paths     map[constraint.PathKey][]BoundaryPath
}

func newBoundaryAddressPathCache(projector BoundaryAddressProjector) boundaryAddressPathCache {
	return boundaryAddressPathCache{
		projector: projector,
		paths:     make(map[constraint.PathKey][]BoundaryPath),
	}
}

func (c boundaryAddressPathCache) fromAddress(addr StableAddress) []BoundaryPath {
	key := addr.Key()
	if key == "" || c.projector == nil {
		return nil
	}
	if cached, ok := c.paths[key]; ok {
		return cached
	}
	paths := c.projector.PathsFromAddress(addr)
	if len(paths) != 0 {
		paths = append([]BoundaryPath(nil), paths...)
	}
	c.paths[key] = paths
	return paths
}
