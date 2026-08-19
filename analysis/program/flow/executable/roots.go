package executable

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type rootSeed struct {
	result *Result
	work   []keyspace.Term
	entry  keyspace.Term
	roots  [][]keyspace.Term
}

func seedRoots(
	bodies *body.Result,
	forest *containment.Result,
	control *sourcecontrol.Result,
	input validated,
) (rootSeed, error) {
	seed := rootSeed{result: newResult(input.counts, input.source, input.flow, input.static, input.module), roots: make([][]keyspace.Term, int(input.counts[keyspace.FamilyBody]))}
	if input.entry == 0 || !validTerm(input.entry, input.counts) {
		return rootSeed{}, errors.New("program/flow/executable: invalid Entry Body")
	}
	add := func(term keyspace.Term) {
		if seed.result.mark(term) {
			seed.work = append(seed.work, term)
		}
	}

	entryNode, ok := control.Cursor(input.entry, 0)
	if !ok || !control.Reachable(entryNode) {
		return rootSeed{}, errors.New("program/flow/executable: Entry Body is not source-control reachable")
	}
	add(input.entry)

	for ordinal := uint32(1); ordinal <= input.counts[keyspace.FamilyBody]; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		start, startOK := control.Cursor(body, 0)
		if !startOK {
			return rootSeed{}, errors.New("program/flow/executable: Body lacks source-control start")
		}
		if control.Reachable(start) {
			add(body)
		}
		length, lengthOK := bodies.RootCount(body)
		if !lengthOK || length < 0 {
			return rootSeed{}, errors.New("program/flow/executable: Body root denominator is unavailable")
		}
		rootRows := make([]keyspace.Term, 0, length)
		for cursor := 0; cursor < length; cursor++ {
			root, rootOK := bodies.RootAt(body, cursor)
			if !rootOK || !validTerm(root, input.counts) {
				return rootSeed{}, errors.New("program/flow/executable: invalid Body source root")
			}
			node, nodeOK := control.Cursor(body, uint32(cursor))
			if !nodeOK {
				return rootSeed{}, errors.New("program/flow/executable: source root lacks source-control cursor")
			}
			if !control.Reachable(node) || forest.Static(root) || !runtimeRoot(root) {
				continue
			}
			add(root)
			rootRows = append(rootRows, root)
		}
		seed.roots[ordinal-1] = rootRows
	}
	seed.entry = input.entry
	return seed, nil
}

func runtimeRoot(term keyspace.Term) bool {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBreak, keyspace.FamilyGoto, keyspace.FamilyReturn,
		keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall,
		keyspace.FamilyBranch, keyspace.FamilyLoop, keyspace.FamilyBody:
		return true
	default:
		// Body roots are the closed statement vocabulary. Keep this switch
		// explicit so Labels, declarations, faults, and namespace metadata
		// cannot become executable endpoints if a future owner widens roots.
		return false
	}
}
