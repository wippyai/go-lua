package continuation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

var continuationSubjects = [...]keyspace.Family{
	keyspace.FamilyUnary,
	keyspace.FamilyBinary,
	keyspace.FamilyRead,
	keyspace.FamilyWrite,
	keyspace.FamilyCall,
	keyspace.FamilyLoop,
}

type inputProof struct {
	counts   [keyspace.FamilyCount]uint32
	entry    keyspace.Term
	source   source.View
	flow     authored.View
	staticID identity.ContentID
	moduleID identity.ContentID
	bodies   *body.Result
	binding  binding.Result
	exec     *executable.Result
	cand     *candidates.Result
	causal   *causal.Result
}

func validateInputs(
	sourceView source.View,
	flow authored.View,
	staticID identity.ContentID,
	moduleID identity.ContentID,
	bodies *body.Result,
	bindingResult binding.Result,
	executableResult *executable.Result,
	candidateResult *candidates.Result,
	causalResult *causal.Result,
) (inputProof, error) {
	input := inputProof{source: sourceView, flow: flow, staticID: staticID, moduleID: moduleID, bodies: bodies, binding: bindingResult,
		exec: executableResult, cand: candidateResult, causal: causalResult}
	identity := sourceView.Identity()
	sourceID, flowID := identity.ContentID(), flow.Cold().ContentID()
	if !sourceID.Available() || identity.Name() == "" || identity.TermCount() == 0 || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return input, errors.New("program/flow/continuation: owner provenance is unavailable")
	}
	if bodies == nil || !body.Matches(bodies, sourceID, flowID) || executableResult == nil ||
		!executable.Matches(executableResult, sourceID, flowID, staticID, moduleID) || candidateResult == nil ||
		!candidates.Matches(candidateResult, sourceID, flowID, staticID, moduleID) || causalResult == nil ||
		!causal.Matches(causalResult, sourceID, flowID, staticID, moduleID) || !binding.Matches(&bindingResult, sourceID, flowID) {
		return input, errors.New("program/flow/continuation: typed prerequisite provenance disagrees")
	}
	counts, err := continuationCounts(identity)
	if err != nil {
		return input, err
	}
	input.counts = counts
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		actual := authored.FamilyCount(flow, family)
		if actual >= 0 && uint64(actual) != uint64(counts[family]) {
			return input, errors.New("program/flow/continuation: authored family cardinality mismatch")
		}
	}
	if bodies.BodyCount() != int(counts[keyspace.FamilyBody]) ||
		bindingResult.CellCount() != int(counts[keyspace.FamilyCell]) {
		return input, errors.New("program/flow/continuation: Body or Cell cardinality mismatch")
	}
	entry, err := findEntry(bodies, counts[keyspace.FamilyBody])
	if err != nil {
		return input, err
	}
	input.entry = entry
	if err := validateCells(flow, bindingResult, bodies, counts, entry); err != nil {
		return input, err
	}
	return input, nil
}

func continuationCounts(sourceIdentity source.Identity) ([keyspace.FamilyCount]uint32, error) {
	var counts [keyspace.FamilyCount]uint32
	if sourceIdentity.ContentID() == (identity.ContentID{}) || sourceIdentity.Name() == "" || sourceIdentity.TermCount() == 0 {
		return counts, errors.New("program/flow/continuation: Source identity is unavailable")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := sourceIdentity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/continuation: invalid Source family cardinality")
		}
		counts[family] = uint32(count)
		total += uint64(count)
	}
	if total != uint64(sourceIdentity.TermCount()) || counts[keyspace.FamilyBody] == 0 {
		return counts, errors.New("program/flow/continuation: Source family cardinality mismatch")
	}
	return counts, nil
}

func findEntry(bodies *body.Result, bodyCount uint32) (keyspace.Term, error) {
	var entry keyspace.Term
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		parent, hasParent := bodies.Parent(term)
		if hasParent {
			if parent == 0 || parent == term {
				return 0, errors.New("program/flow/continuation: malformed Body parent")
			}
			continue
		}
		if entry != 0 {
			return 0, errors.New("program/flow/continuation: Body forest has multiple entries")
		}
		entry = term
	}
	if entry == 0 {
		return 0, errors.New("program/flow/continuation: Entry Body is unavailable")
	}
	return entry, nil
}

func validateCells(
	flow authored.View,
	bindingResult binding.Result,
	bodies *body.Result,
	counts [keyspace.FamilyCount]uint32,
	entry keyspace.Term,
) error {
	cells := flow.Storage().Cells()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyCell]; ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, ordinal)
		role, roleOK := bindingResult.Role(cell)
		host, hostOK := bindingResult.Host(cell)
		cellKind, cellBody, key, rowOK := cells.Get(cell)
		if !roleOK || !hostOK || !rowOK {
			return errors.New("program/flow/continuation: Binding Cell row is unavailable")
		}
		switch role {
		case kind.CellGlobal:
			if cellKind != authored.CellGlobal || cellBody != 0 || key == 0 || host != 0 {
				return errors.New("program/flow/continuation: malformed global Cell role")
			}
		case kind.CellLocal:
			if !validLocalCell(cellKind, key, host, cellBody, counts, flow) {
				return errors.New("program/flow/continuation: malformed Bind Cell role")
			}
		case kind.CellFormal, kind.CellFunctionVararg, kind.CellCapture:
			if !validFunctionCell(cellKind, key, host, cellBody, counts, flow, bodies) {
				return errors.New("program/flow/continuation: malformed Function Cell role")
			}
		case kind.CellLoop:
			if !validLoopCell(cellKind, key, host, cellBody, counts, flow, bodies) {
				return errors.New("program/flow/continuation: malformed Loop Cell role")
			}
		case kind.CellChunkVararg:
			if cellKind != authored.CellLocal || key != 0 || host != entry || cellBody != entry {
				return errors.New("program/flow/continuation: malformed chunk Vararg Cell role")
			}
		default:
			return errors.New("program/flow/continuation: invalid Binding Cell role")
		}
	}
	return nil
}

func validLocalCell(cellKind authored.CellKind, key keyspace.Key, host, body keyspace.Term, counts [keyspace.FamilyCount]uint32, flow authored.View) bool {
	if cellKind != authored.CellLocal || key != 0 || !keyspace.ValidTerm(host, keyspace.FamilyBind, int(counts[keyspace.FamilyBind])) ||
		!keyspace.ValidTerm(body, keyspace.FamilyBody, int(counts[keyspace.FamilyBody])) {
		return false
	}
	owner, _, ok := flow.Storage().Binds().Get(host)
	return ok && owner == body
}

func validFunctionCell(cellKind authored.CellKind, key keyspace.Key, host, body keyspace.Term, counts [keyspace.FamilyCount]uint32, flow authored.View, bodies *body.Result) bool {
	if cellKind != authored.CellLocal || key != 0 || !keyspace.ValidTerm(host, keyspace.FamilyFunction, int(counts[keyspace.FamilyFunction])) ||
		!keyspace.ValidTerm(body, keyspace.FamilyBody, int(counts[keyspace.FamilyBody])) {
		return false
	}
	_, functionBody, _, ok := flow.Functions().Get(host)
	if !ok || functionBody != body {
		return false
	}
	activation, activationOK := bodies.Activation(body)
	return activationOK && activation == host
}

func validLoopCell(cellKind authored.CellKind, key keyspace.Key, host, body keyspace.Term, counts [keyspace.FamilyCount]uint32, flow authored.View, bodies *body.Result) bool {
	if cellKind != authored.CellLocal || key != 0 || !keyspace.ValidTerm(host, keyspace.FamilyLoop, int(counts[keyspace.FamilyLoop])) ||
		!keyspace.ValidTerm(body, keyspace.FamilyBody, int(counts[keyspace.FamilyBody])) {
		return false
	}
	_, loopBody, _, _, ok := flow.Control().Loops().Get(host)
	parent, parentOK := bodies.Parent(body)
	return ok && loopBody == body && parentOK && parent == parentBody(flow, host)
}

func parentBody(flow authored.View, loop keyspace.Term) keyspace.Term {
	owner, _, _, _, ok := flow.Control().Loops().Get(loop)
	if !ok {
		return 0
	}
	return owner
}

func subjectFrom(exec *executable.Result, cand *candidates.Result, term keyspace.Term) bool {
	if exec == nil || cand == nil || !exec.Contains(term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCall:
		return true
	case keyspace.FamilyUnary:
		return cand.UnaryNumeric().Contains(term) || cand.Length().Contains(term)
	case keyspace.FamilyBinary:
		return cand.Arithmetic().Contains(term) || cand.Bitwise().Contains(term) || cand.Concat().Contains(term) ||
			cand.Equality().Contains(term) || cand.Order().Contains(term)
	case keyspace.FamilyRead:
		return cand.IndexGet().Contains(term)
	case keyspace.FamilyWrite:
		return cand.IndexSet().Contains(term)
	case keyspace.FamilyLoop:
		return cand.GenericLoop().Contains(term)
	default:
		return false
	}
}

func validatePublishedProjection(result *Result, counts [keyspace.FamilyCount]uint32) error {
	if result == nil {
		return errors.New("program/flow/continuation: published projection is malformed")
	}
	cellRecords, cellOK := validateCellStore(result.cells, counts)
	guardRecords, guardOK := validateGuardChain(result.guards, counts)
	if !cellOK || !guardOK {
		return errors.New("program/flow/continuation: published projection is malformed")
	}
	result.cells.records = cellRecords
	result.guards.records = guardRecords
	return nil
}

func validateCellStore(projection cellProjection, counts [keyspace.FamilyCount]uint32) ([keyspace.FamilyCount][]cellRootRecord, bool) {
	var records [keyspace.FamilyCount][]cellRootRecord
	if projection.counts != counts || len(projection.nodes) == 0 || projection.nodes[0] != (scopeNode{}) {
		return records, false
	}
	for index := 1; index < len(projection.nodes); index++ {
		node := projection.nodes[index]
		if node.parent >= uint32(index) || uint64(node.parent) >= uint64(len(projection.nodes)) || node.count == 0 || node.total == 0 ||
			uint64(node.start)+uint64(node.count) > uint64(len(projection.terms)) {
			return records, false
		}
		parent := projection.nodes[node.parent]
		if uint64(parent.total)+uint64(node.count) != uint64(node.total) {
			return records, false
		}
		for offset := uint32(0); offset < node.count; offset++ {
			term := projection.terms[node.start+offset]
			if !keyspace.ValidTerm(term, keyspace.FamilyCell, int(counts[keyspace.FamilyCell])) {
				return records, false
			}
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if !subjectFamily(family) && len(projection.roots[family]) != 0 {
			return records, false
		}
	}
	for _, family := range continuationSubjects {
		if uint64(len(projection.roots[family])) != uint64(counts[family])+1 {
			return records, false
		}
		if projection.roots[family][0] != absentRoot {
			return records, false
		}
		records[family] = make([]cellRootRecord, len(projection.roots[family]))
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			root := projection.roots[family][ordinal]
			if root == absentRoot {
				continue
			}
			if uint64(root) >= uint64(len(projection.nodes)) {
				return records, false
			}
			count, ok := (&projection).count(root)
			if !ok || count < 0 || uint64(count) > uint64(^uint32(0)) {
				return records, false
			}
			records[family][ordinal] = cellRootRecord{root: root, count: uint32(count), present: true, node: projection.nodes[root]}
		}
	}
	return records, true
}

func validateGuardChain(projection guardProjection, counts [keyspace.FamilyCount]uint32) ([keyspace.FamilyCount][]guardRootRecord, bool) {
	var records [keyspace.FamilyCount][]guardRootRecord
	if projection.counts != counts || len(projection.nodes) == 0 || projection.nodes[0] != (guardNode{}) {
		return records, false
	}
	for index := 1; index < len(projection.nodes); index++ {
		node := projection.nodes[index]
		if node.count == 0 || node.prev >= uint32(index) || node.jump >= uint32(index) ||
			uint64(node.prev) >= uint64(len(projection.nodes)) || uint64(node.jump) >= uint64(len(projection.nodes)) {
			return records, false
		}
		parent := projection.nodes[node.prev]
		if parent.count == ^uint32(0) || parent.count+1 != node.count || (parent.count != 0 && parent.term >= node.term) {
			return records, false
		}
		if _, ok := guardRank(node.term, counts); !ok {
			return records, false
		}
		target := node.count - guardLowbit(node.count)
		expected, ok := guardAncestor(projection.nodes[:index], node.prev, target)
		if !ok || node.jump != expected {
			return records, false
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		admitted := subjectFamily(family) || projection.families[family]
		if !admitted {
			if len(projection.roots[family]) != 0 {
				return records, false
			}
			continue
		}
		if uint64(len(projection.roots[family])) != uint64(counts[family])+1 {
			return records, false
		}
		if projection.roots[family][0] != absentRoot {
			return records, false
		}
		records[family] = make([]guardRootRecord, len(projection.roots[family]))
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			root := projection.roots[family][ordinal]
			if root == absentRoot {
				continue
			}
			if uint64(root) >= uint64(len(projection.nodes)) {
				return records, false
			}
			node := projection.nodes[root]
			records[family][ordinal] = guardRootRecord{root: root, count: node.count, present: true, node: node}
		}
	}
	return records, true
}
