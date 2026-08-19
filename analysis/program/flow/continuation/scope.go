package continuation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// scopeNode is one persistent lexical introduction group.  Parent scopes are
// always older nodes, so Cell queries iterate an acyclic chain without a cap or a
// recursive call.  The group cells live in one shared compact pool.
type scopeNode struct {
	parent uint32
	start  uint32
	count  uint32
	total  uint32
}

type scopeStore struct {
	nodes []scopeNode
	terms []keyspace.Term
}

func (projection *cellProjection) count(root uint32) (int, bool) {
	if projection == nil || uint64(root) >= uint64(len(projection.nodes)) {
		return 0, false
	}
	if root == 0 {
		return 0, projection.nodes[0] == (scopeNode{})
	}
	node := projection.nodes[root]
	if node.total == 0 {
		return 0, false
	}
	for current := root; current != 0; {
		if uint64(current) >= uint64(len(projection.nodes)) {
			return 0, false
		}
		node = projection.nodes[current]
		if node.parent >= current || uint64(node.start)+uint64(node.count) > uint64(len(projection.terms)) || node.count == 0 {
			return 0, false
		}
		if node.parent == 0 {
			if node.total != node.count {
				return 0, false
			}
		} else {
			parent := projection.nodes[node.parent]
			if uint64(parent.total)+uint64(node.count) != uint64(node.total) {
				return 0, false
			}
		}
		current = node.parent
	}
	return int(projection.nodes[root].total), true
}

func (projection *cellProjection) at(root, total, index uint32) (keyspace.Term, bool) {
	if projection == nil || root == 0 || uint64(root) >= uint64(len(projection.nodes)) {
		return 0, false
	}
	if index >= total {
		return 0, false
	}
	node := projection.nodes[root]
	for root != 0 {
		if uint64(root) >= uint64(len(projection.nodes)) {
			return 0, false
		}
		node = projection.nodes[root]
		if node.parent >= root || uint64(node.start)+uint64(node.count) > uint64(len(projection.terms)) || node.count == 0 {
			return 0, false
		}
		if index < node.count {
			term := projection.terms[node.start+index]
			if !keyspace.ValidTerm(term, keyspace.FamilyCell, int(projection.counts[keyspace.FamilyCell])) {
				return 0, false
			}
			return term, true
		}
		index -= node.count
		root = node.parent
	}
	return 0, false
}

func newScopeStore() *scopeStore {
	return &scopeStore{nodes: []scopeNode{{}}}
}

func (store *scopeStore) appendGroup(parent uint32, cells []keyspace.Term) (uint32, error) {
	if store == nil || uint64(parent) >= uint64(len(store.nodes)) || len(cells) == 0 {
		return 0, errors.New("program/flow/continuation: invalid lexical Cell group")
	}
	parentNode := store.nodes[parent]
	if uint64(len(cells)) > uint64(^uint32(0)) || uint64(parentNode.total)+uint64(len(cells)) > uint64(^uint32(0)) ||
		uint64(len(store.nodes)) >= uint64(^uint32(0)) || uint64(len(store.terms))+uint64(len(cells)) > uint64(^uint32(0)) {
		return 0, errors.New("program/flow/continuation: lexical Cell scope is too large")
	}
	start := uint32(len(store.terms))
	store.terms = append(store.terms, cells...)
	store.nodes = append(store.nodes, scopeNode{
		parent: parent,
		start:  start,
		count:  uint32(len(cells)),
		total:  parentNode.total + uint32(len(cells)),
	})
	return uint32(len(store.nodes) - 1), nil
}

func compactScopeRoots(store *scopeStore, roots [keyspace.FamilyCount][]uint32) error {
	if store == nil || len(store.nodes) == 0 || store.nodes[0] != (scopeNode{}) {
		return errors.New("program/flow/continuation: invalid lexical scope compaction")
	}
	live := make([]bool, len(store.nodes))
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for _, root := range roots[family] {
			if root == absentRoot {
				continue
			}
			if uint64(root) >= uint64(len(live)) {
				return errors.New("program/flow/continuation: invalid lexical scope root")
			}
			live[root] = true
		}
	}
	for index := len(live) - 1; index > 0; index-- {
		if !live[index] {
			continue
		}
		node := store.nodes[index]
		if node.parent >= uint32(index) || uint64(node.start)+uint64(node.count) > uint64(len(store.terms)) || node.count == 0 || node.total == 0 {
			return errors.New("program/flow/continuation: malformed lexical scope node")
		}
		parent := store.nodes[node.parent]
		if uint64(parent.total)+uint64(node.count) != uint64(node.total) {
			return errors.New("program/flow/continuation: malformed lexical scope total")
		}
		live[node.parent] = true
	}
	remap := make([]uint32, len(store.nodes))
	compact := []scopeNode{{}}
	compactTerms := make([]keyspace.Term, 0, len(store.terms))
	for index := 1; index < len(store.nodes); index++ {
		if !live[index] {
			continue
		}
		node := store.nodes[index]
		if remap[node.parent] == 0 && node.parent != 0 {
			return errors.New("program/flow/continuation: unremapped lexical scope parent")
		}
		if uint64(node.start)+uint64(node.count) > uint64(len(store.terms)) {
			return errors.New("program/flow/continuation: lexical scope Cell range is invalid")
		}
		start := uint32(len(compactTerms))
		compactTerms = append(compactTerms, store.terms[node.start:node.start+node.count]...)
		node.parent = remap[node.parent]
		node.start = start
		if uint64(len(compact)) >= uint64(^uint32(0)) {
			return errors.New("program/flow/continuation: compact lexical scope is too large")
		}
		remap[index] = uint32(len(compact))
		compact = append(compact, node)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal, root := range roots[family] {
			if root == absentRoot || root == 0 {
				continue
			}
			if remap[root] == 0 {
				return errors.New("program/flow/continuation: unremapped lexical scope root")
			}
			roots[family][ordinal] = remap[root]
		}
	}
	store.nodes = compact
	store.terms = compactTerms
	return nil
}
