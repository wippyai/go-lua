package target

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// freezeExactKeys builds Target's only exact-key pool from semantic rows.
// Source owns canonical Lua key normalization; Target owns only its dense
// contract-local handles. No string reconstruction participates in this ABI.
func freezeExactKeys(drafts []operationDraft, boot bootDraft) ([]keyspace.LiteralValue, map[keyspace.LiteralValue]ExactKey, error) {
	set := make(map[keyspace.LiteralValue]struct{})
	add := func(value keyspace.LiteralValue) error {
		normalized, ok := source.NormalizeExactKey(value)
		if !ok || normalized != value {
			return errors.New("target: unnormalized exact key")
		}
		set[value] = struct{}{}
		return nil
	}
	for _, entry := range boot.entries {
		if err := add(entry.key); err != nil {
			return nil, nil, err
		}
	}
	for _, binding := range boot.bindings {
		if err := add(binding.key); err != nil {
			return nil, nil, err
		}
	}
	for _, draft := range drafts {
		for _, binding := range draft.bindings {
			for _, segment := range binding.Owner {
				if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
					return nil, nil, err
				}
			}
			for _, segment := range binding.Member {
				if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
					return nil, nil, err
				}
			}
		}
		for _, edge := range draft.subedges {
			switch edge.callee {
			case SubedgeCalleeCapturedInitialRead:
				if err := add(edge.readKey); err != nil {
					return nil, nil, err
				}
			case SubedgeCalleeMetaKey:
				if err := add(edge.metaKey); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	for _, value := range boot.values {
		if value.kind != InitialValueDeniedOperation {
			continue
		}
		for _, segment := range value.binding.Owner {
			if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
				return nil, nil, err
			}
		}
		for _, segment := range value.binding.Member {
			if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
				return nil, nil, err
			}
		}
	}
	values := make([]keyspace.LiteralValue, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Slice(values, func(left, right int) bool {
		order, ok := source.CompareExactKey(values[left], values[right])
		if !ok {
			panic("target: unnormalized exact key")
		}
		return order < 0
	})
	if _, err := checkedStoredLength("exact key table", len(values)); err != nil {
		return nil, nil, err
	}
	handles := make(map[keyspace.LiteralValue]ExactKey, len(values))
	for index, value := range values {
		handle, err := checkedStoredHandle("exact key table", index)
		if err != nil {
			return nil, nil, err
		}
		handles[value] = ExactKey(handle)
	}
	return values, handles, nil
}

func (c *Contract) appendExactKeys(input []keyspace.LiteralValue) error {
	if _, err := checkedStoredRange("exact key table", len(c.exactKeys), len(input)); err != nil {
		return err
	}
	for _, value := range input {
		normalized, ok := source.NormalizeExactKey(value)
		if !ok || normalized != value {
			return errors.New("target: malformed exact key")
		}
		c.exactKeys = append(c.exactKeys, value)
	}
	return nil
}

// resolveSubedgeInitialReads proves that every capture-once callee is an
// existing boot row and records its sealed root coordinate before Contract
// tables are appended. It never invents a global lookup relation.
func resolveSubedgeInitialReads(drafts []operationDraft, boot bootDraft) error {
	roots := make(map[string]InitialRoot, len(boot.roots))
	for index, root := range boot.roots {
		roots[root.identity] = InitialRoot(index + 1)
	}
	for operation := range drafts {
		for edgeIndex := range drafts[operation].subedges {
			edge := &drafts[operation].subedges[edgeIndex]
			if edge.callee != SubedgeCalleeCapturedInitialRead {
				continue
			}
			root, ok := roots[edge.readRoot]
			if !ok {
				return errors.New("target: captured initial read has unknown root")
			}
			if _, found := lookupInitialEntry(boot.entries, root, edge.readKey); !found {
				return errors.New("target: captured initial read lacks boot entry")
			}
			edge.readRootID = root
		}
	}
	return nil
}
