package target

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// freezeExactKeys issues the one contract-wide exact-key owner before either
// operation or boot rows are compiled. Sources may repeat an atom; the owner
// canonicalizes and deduplicates it once.
func freezeExactKeys(drafts []operationDraft, roots []vocabulary.InitialRootSpec, entries []vocabulary.InitialEntrySpec, bindings []vocabulary.InitialBindingSpec) (exactkey.Table, error) {
	values := make([]keyspace.LiteralValue, 0)
	add := func(value keyspace.LiteralValue) error {
		values = append(values, value)
		return nil
	}
	addValue := func(value vocabulary.InitialValueSpec) error {
		if value.Kind != vocabulary.InitialValueDeniedOperation {
			return nil
		}
		for _, segment := range value.Operation.Owner {
			if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
				return err
			}
		}
		for _, segment := range value.Operation.Member {
			if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := addValue(root.Shape.Value); err != nil {
			return exactkey.Table{}, err
		}
	}
	for _, entry := range entries {
		if err := add(entry.Key); err != nil {
			return exactkey.Table{}, err
		}
		if err := addValue(entry.Value); err != nil {
			return exactkey.Table{}, err
		}
	}
	for _, binding := range bindings {
		if err := add(binding.Key); err != nil {
			return exactkey.Table{}, err
		}
	}
	for _, draft := range drafts {
		for _, binding := range draft.bindings {
			for _, segment := range binding.Owner {
				if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
					return exactkey.Table{}, err
				}
			}
			for _, segment := range binding.Member {
				if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: segment}); err != nil {
					return exactkey.Table{}, err
				}
			}
		}
		for _, edge := range draft.subedges {
			switch edge.callee {
			case vocabulary.SubedgeCalleeCapturedInitialRead:
				if err := add(edge.readKey); err != nil {
					return exactkey.Table{}, err
				}
			case vocabulary.SubedgeCalleeMetaKey:
				if err := add(edge.metaKey); err != nil {
					return exactkey.Table{}, err
				}
			}
		}
	}
	return exactkey.Compile(values)
}

// resolveSubedgeInitialReads consumes only boot-owned lookup facts. The
// operation owner does not reopen a boot draft or rebuild its root directory.
func resolveSubedgeInitialReads(drafts []operationDraft, table *bootvalue.Table, keys exactkey.Table) error {
	if table == nil {
		return errors.New("target: unavailable boot table")
	}
	for operation := range drafts {
		for edgeIndex := range drafts[operation].subedges {
			edge := &drafts[operation].subedges[edgeIndex]
			if edge.callee != vocabulary.SubedgeCalleeCapturedInitialRead {
				continue
			}
			root, ok := table.InitialRootByIdentity(edge.readRoot)
			if !ok {
				return errors.New("target: captured initial read has unknown root")
			}
			key, ok := keys.Handle(edge.readKey)
			if !ok {
				return errors.New("target: captured initial read has unknown key")
			}
			if _, _, found := table.InitialEntry(root, key); !found {
				return errors.New("target: captured initial read lacks boot entry")
			}
			edge.readRootID = root
		}
	}
	return nil
}
