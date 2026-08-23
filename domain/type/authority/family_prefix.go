package typeauthority

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/subtype"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// familyPrefix is the Family-owned Runtime universe of the eight primitive
// seeds coalesced with every admitted member plane. SealFamily pays this once.
// A family-only SealRuntime copies it. Mixed local inputs merge in key order
// and extend the prefix subtype matrix for pairs that touch a new closed row.
type familyPrefix struct {
	rows            []runtimeRow
	variants        []runtimeChild
	construction    []typ.Type
	keys            []runtimeSourceKey
	memberMaps      [][]uint32
	nilRow          uint32
	booleanRow      uint32
	numberRow       uint32
	integerRow      uint32
	stringRow       uint32
	anyRow          uint32
	unknownRow      uint32
	neverRow        uint32
	closedPositions []int32
	closedRows      []uint32
	subtypeStride   int
	subtypeBits     []uint64
}

func newFamilyPrefix(members []familyMember) (*familyPrefix, error) {
	if runtimePrimitiveSources.err != nil {
		return nil, runtimePrimitiveSources.err
	}
	runtime := &Runtime{}
	builder := runtimeBuilder{runtime: runtime}
	inputs := make([]runtimeCanonicalInput, len(members))
	for index, member := range members {
		if !member.graph.Sealed() || !member.canonicalID.Available() {
			return nil, errors.New("typeauthority: unsealed Runtime family member")
		}
		inputs[index] = runtimeCanonicalInput{
			input:     RuntimeInput{graph: member.graph},
			identity:  member.canonicalID,
			positions: []int{index},
		}
	}
	if _, err := builder.ingestFresh(inputs); err != nil {
		return nil, err
	}
	if err := builder.sealRuntimeKinds(); err != nil {
		return nil, err
	}
	if err := builder.sealSubtypeRelation(); err != nil {
		return nil, err
	}
	return &familyPrefix{
		rows:            runtime.rows,
		variants:        runtime.variants,
		construction:    builder.construction,
		keys:            builder.keys,
		memberMaps:      builder.sourceMaps,
		nilRow:          runtime.nilRow,
		booleanRow:      runtime.booleanRow,
		numberRow:       runtime.numberRow,
		integerRow:      runtime.integerRow,
		stringRow:       runtime.stringRow,
		anyRow:          runtime.anyRow,
		unknownRow:      runtime.unknownRow,
		neverRow:        runtime.neverRow,
		closedPositions: runtime.closedPositions,
		closedRows:      runtime.closedRows,
		subtypeStride:   runtime.subtypeStride,
		subtypeBits:     runtime.subtypeBits,
	}, nil
}

func sharedFamilyPrefix(inputs []runtimeCanonicalInput) (*familyPrefix, []int) {
	members := make([]int, len(inputs))
	var prefix *familyPrefix
	for index, input := range inputs {
		members[index] = -1
		if input.input.prefix == nil {
			continue
		}
		if prefix == nil {
			prefix = input.input.prefix
		} else if prefix != input.input.prefix {
			return nil, nil
		}
		members[index] = input.input.prefixMember
	}
	return prefix, members
}

func familyPrefixHasLocal(members []int) bool {
	for _, member := range members {
		if member < 0 {
			return true
		}
	}
	return false
}

func (b *runtimeBuilder) installFamilyPrefix(prefix *familyPrefix, inputs []runtimeCanonicalInput, members []int) ([]RuntimeInner, error) {
	if b == nil || b.runtime == nil || prefix == nil || len(b.runtime.rows) != 0 {
		return nil, errors.New("typeauthority: invalid Runtime prefix install")
	}
	if len(members) != len(inputs) || len(prefix.memberMaps) == 0 {
		return nil, errors.New("typeauthority: Runtime family prefix maps unavailable")
	}
	b.runtime.rows = copyRuntimeRows(prefix.rows, b.runtime)
	b.runtime.variants = copyRuntimeVariants(prefix.variants, b.runtime)
	b.construction = append([]typ.Type(nil), prefix.construction...)
	b.keys = append([]runtimeSourceKey(nil), prefix.keys...)
	b.runtime.nilRow, b.runtime.booleanRow = prefix.nilRow, prefix.booleanRow
	b.runtime.numberRow, b.runtime.integerRow = prefix.numberRow, prefix.integerRow
	b.runtime.stringRow, b.runtime.anyRow = prefix.stringRow, prefix.anyRow
	b.runtime.unknownRow, b.runtime.neverRow = prefix.unknownRow, prefix.neverRow
	b.runtime.closedPositions = append([]int32(nil), prefix.closedPositions...)
	b.runtime.closedRows = append([]uint32(nil), prefix.closedRows...)
	b.runtime.subtypeStride = prefix.subtypeStride
	b.runtime.subtypeBits = append([]uint64(nil), prefix.subtypeBits...)
	b.runtime.runtimeKindsPublished = true
	b.sourceMaps = make([][]uint32, len(inputs))
	for index, member := range members {
		if member < 0 || member >= len(prefix.memberMaps) {
			return nil, errors.New("typeauthority: Runtime family prefix member")
		}
		b.sourceMaps[index] = prefix.memberMaps[member]
	}
	canonicalInners := make([]RuntimeInner, len(inputs))
	for inputIndex, input := range inputs {
		root, rootOK := input.input.graph.RootOrdinal()
		if !rootOK || uint64(root) >= uint64(len(b.sourceMaps[inputIndex])) {
			return nil, errors.New("typeauthority: Runtime prefix root mapping")
		}
		row := b.sourceMaps[inputIndex][root]
		if row == 0 {
			return nil, errors.New("typeauthority: Runtime prefix root unmapped")
		}
		canonicalInners[inputIndex] = RuntimeInner{owner: b.runtime, index: row}
	}
	return canonicalInners, nil
}

func (b *runtimeBuilder) mergeFamilyPrefix(prefix *familyPrefix, inputs []runtimeCanonicalInput, members []int) ([]RuntimeInner, error) {
	if b == nil || b.runtime == nil || prefix == nil || len(b.runtime.rows) != 0 {
		return nil, errors.New("typeauthority: invalid Runtime prefix merge")
	}
	if len(members) != len(inputs) || len(prefix.keys) != len(prefix.rows) {
		return nil, errors.New("typeauthority: Runtime family prefix keys unavailable")
	}
	b.sourceMaps = make([][]uint32, len(inputs))
	locals := make([]runtimeSource, 0)
	for inputIndex, input := range inputs {
		if members[inputIndex] >= 0 {
			continue
		}
		sources, err := collectRuntimeInputSources(inputIndex, input.input)
		if err != nil {
			return nil, err
		}
		b.sourceMaps[inputIndex] = make([]uint32, len(sources))
		locals = append(locals, sources...)
	}
	sort.SliceStable(locals, func(left, right int) bool {
		return runtimeSourceKeyCompare(locals[left].key, locals[right].key) < 0 ||
			(runtimeSourceKeyCompare(locals[left].key, locals[right].key) == 0 && runtimeSourceTieLess(locals[left], locals[right]))
	})
	remap := make([]uint32, len(prefix.rows)+1)
	oldOfNew := make([]uint32, 0, len(prefix.rows)+len(locals))
	localReps := make([]runtimeSource, 0)
	prefixIndex, localIndex := 0, 0
	for prefixIndex < len(prefix.keys) || localIndex < len(locals) {
		cmp := 0
		switch {
		case localIndex >= len(locals):
			cmp = -1
		case prefixIndex >= len(prefix.keys):
			cmp = 1
		default:
			cmp = runtimeSourceKeyCompare(prefix.keys[prefixIndex], locals[localIndex].key)
		}
		switch {
		case cmp < 0:
			if _, err := b.emitPrefixRow(prefix, prefixIndex, remap, &oldOfNew); err != nil {
				return nil, err
			}
			prefixIndex++
		case cmp > 0:
			rep, err := b.emitLocalGroup(locals, &localIndex, &oldOfNew)
			if err != nil {
				return nil, err
			}
			localReps = append(localReps, rep)
		default:
			ordinal, err := b.emitPrefixRow(prefix, prefixIndex, remap, &oldOfNew)
			if err != nil {
				return nil, err
			}
			for localIndex < len(locals) && runtimeSourceKeyCompare(prefix.keys[prefixIndex], locals[localIndex].key) == 0 {
				local := locals[localIndex]
				if local.input >= 0 {
					b.sourceMaps[local.input][local.ordinal] = ordinal
				}
				localIndex++
			}
			prefixIndex++
		}
	}
	if err := b.rewriteMergedPrefixEdges(prefix, remap, oldOfNew); err != nil {
		return nil, err
	}
	for index, member := range members {
		if member < 0 {
			continue
		}
		if member >= len(prefix.memberMaps) {
			return nil, errors.New("typeauthority: Runtime family prefix member")
		}
		oldMap := prefix.memberMaps[member]
		mapped := make([]uint32, len(oldMap))
		for ordinal, row := range oldMap {
			mapped[ordinal] = remap[row]
			if mapped[ordinal] == 0 {
				return nil, errors.New("typeauthority: Runtime family prefix remap")
			}
		}
		b.sourceMaps[index] = mapped
	}
	b.runtime.nilRow = remap[prefix.nilRow]
	b.runtime.booleanRow = remap[prefix.booleanRow]
	b.runtime.numberRow = remap[prefix.numberRow]
	b.runtime.integerRow = remap[prefix.integerRow]
	b.runtime.stringRow = remap[prefix.stringRow]
	b.runtime.anyRow = remap[prefix.anyRow]
	b.runtime.unknownRow = remap[prefix.unknownRow]
	b.runtime.neverRow = remap[prefix.neverRow]
	if b.runtime.nilRow == 0 || b.runtime.booleanRow == 0 || b.runtime.neverRow == 0 {
		return nil, errors.New("typeauthority: Runtime prefix seed remap")
	}
	for _, rep := range localReps {
		if err := b.installReceiptEdges(int(rep.row)-1, rep); err != nil {
			return nil, err
		}
	}
	canonicalInners := make([]RuntimeInner, len(inputs))
	for inputIndex, input := range inputs {
		root, rootOK := input.input.graph.RootOrdinal()
		if !rootOK || uint64(root) >= uint64(len(b.sourceMaps[inputIndex])) {
			return nil, errors.New("typeauthority: Runtime merged prefix root mapping")
		}
		row := b.sourceMaps[inputIndex][root]
		if row == 0 {
			return nil, errors.New("typeauthority: Runtime merged prefix root unmapped")
		}
		canonicalInners[inputIndex] = RuntimeInner{owner: b.runtime, index: row}
	}
	b.prefix = prefix
	b.oldOfNew = oldOfNew
	return canonicalInners, nil
}

func (b *runtimeBuilder) emitPrefixRow(prefix *familyPrefix, prefixIndex int, remap []uint32, oldOfNew *[]uint32) (uint32, error) {
	ordinal, err := runtimeDenseOrdinal(len(b.runtime.rows))
	if err != nil {
		return 0, err
	}
	remap[prefixIndex+1] = ordinal
	*oldOfNew = append(*oldOfNew, uint32(prefixIndex+1))
	b.runtime.rows = append(b.runtime.rows, prefix.rows[prefixIndex])
	b.construction = append(b.construction, prefix.construction[prefixIndex])
	b.keys = append(b.keys, prefix.keys[prefixIndex])
	return ordinal, nil
}

func (b *runtimeBuilder) emitLocalGroup(locals []runtimeSource, localIndex *int, oldOfNew *[]uint32) (runtimeSource, error) {
	first := locals[*localIndex]
	ordinal, err := runtimeDenseOrdinal(len(b.runtime.rows))
	if err != nil {
		return runtimeSource{}, err
	}
	row := runtimeRow{form: first.node.Kind, canonicalID: identity.ContentID(first.node.Identity)}
	if !first.node.Closed {
		row.scopedID = runtimeSourceIdentity(first.key)
	}
	if !row.canonicalID.Available() || (!first.node.Closed && !row.scopedID.Available()) {
		return runtimeSource{}, errors.New("typeauthority: Runtime source identity unavailable")
	}
	b.runtime.rows = append(b.runtime.rows, row)
	b.construction = append(b.construction, runtimeSourceValue(first.value))
	b.keys = append(b.keys, first.key)
	*oldOfNew = append(*oldOfNew, 0)
	first.row = ordinal
	for *localIndex < len(locals) && runtimeSourceKeyCompare(first.key, locals[*localIndex].key) == 0 {
		local := locals[*localIndex]
		if local.input >= 0 {
			b.sourceMaps[local.input][local.ordinal] = ordinal
		}
		*localIndex++
	}
	return first, nil
}

func (b *runtimeBuilder) rewriteMergedPrefixEdges(prefix *familyPrefix, remap []uint32, oldOfNew []uint32) error {
	b.runtime.variants = make([]runtimeChild, len(prefix.variants))
	for index, child := range prefix.variants {
		rewritten, err := remapRuntimeChild(child, b.runtime, remap)
		if err != nil {
			return err
		}
		b.runtime.variants[index] = rewritten
	}
	for index := range b.runtime.rows {
		old := oldOfNew[index]
		if old == 0 {
			continue
		}
		rewritten, err := remapRuntimeChild(prefix.rows[old-1].inner, b.runtime, remap)
		if err != nil {
			return err
		}
		b.runtime.rows[index].inner = rewritten
	}
	return nil
}

func remapRuntimeChild(child runtimeChild, owner *Runtime, remap []uint32) (runtimeChild, error) {
	if !child.present {
		return child, nil
	}
	if child.inner.index == 0 || int(child.inner.index) >= len(remap) || remap[child.inner.index] == 0 {
		return runtimeChild{}, errors.New("typeauthority: Runtime prefix child remap")
	}
	child.inner.owner = owner
	child.inner.index = remap[child.inner.index]
	return child, nil
}

func collectRuntimeInputSources(inputIndex int, input RuntimeInput) ([]runtimeSource, error) {
	nodes := input.graph.Nodes()
	if len(nodes) == 0 {
		return nil, errors.New("typeauthority: Runtime receipt source empty")
	}
	plane, planeOK := input.graph.SourcePlane()
	if !planeOK || len(plane) != len(nodes) {
		return nil, errors.New("typeauthority: Runtime receipt source unavailable")
	}
	sources := make([]runtimeSource, 0, len(nodes))
	for ordinal, node := range nodes {
		if err := validateRuntimeSourceNode(nodes, node); err != nil {
			return nil, err
		}
		value := plane[ordinal]
		if value == nil {
			return nil, errors.New("typeauthority: Runtime receipt source unavailable")
		}
		key, err := runtimeSourceKeyForNode(nodes, node)
		if err != nil {
			return nil, err
		}
		sources = append(sources, runtimeSource{
			input: inputIndex, ordinal: uint32(ordinal), seed: -1,
			node: node, value: runtimeSourceValue(value), key: key,
		})
	}
	return sources, nil
}

func (b *runtimeBuilder) extendPrefixSubtypeRelation() error {
	if b == nil || b.runtime == nil || b.prefix == nil || len(b.oldOfNew) != len(b.runtime.rows) {
		return errors.New("typeauthority: malformed Runtime prefix relation extend")
	}
	if len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime relation source")
	}
	runtime := b.runtime
	runtime.closedPositions = make([]int32, len(runtime.rows))
	runtime.closedRows = make([]uint32, 0, len(runtime.rows))
	for index := range runtime.rows {
		runtime.closedPositions[index] = -1
		if runtime.rows[index].scopedID.Available() {
			continue
		}
		if b.construction[index] == nil {
			return errors.New("typeauthority: closed Runtime row lacks a relation source graph")
		}
		runtime.closedPositions[index] = int32(len(runtime.closedRows))
		runtime.closedRows = append(runtime.closedRows, uint32(index+1))
	}
	count := len(runtime.closedRows)
	if count == 0 {
		return errors.New("typeauthority: empty Runtime relation universe")
	}
	runtime.subtypeStride = (count + 63) / 64
	bits := make([]uint64, count*runtime.subtypeStride)
	var prover subtype.Batch
	for leftPosition, leftRow := range runtime.closedRows {
		left := b.construction[leftRow-1]
		base := leftPosition * runtime.subtypeStride
		oldLeft := b.oldOfNew[leftRow-1]
		for rightPosition, rightRow := range runtime.closedRows {
			oldRight := b.oldOfNew[rightRow-1]
			if oldLeft != 0 && oldRight != 0 {
				leftClosed := b.prefix.closedPositions[oldLeft-1]
				rightClosed := b.prefix.closedPositions[oldRight-1]
				if leftClosed >= 0 && rightClosed >= 0 {
					word := b.prefix.subtypeBits[int(leftClosed)*b.prefix.subtypeStride+int(rightClosed)>>6]
					if word&(1<<(uint(rightClosed)&63)) != 0 {
						bits[base+rightPosition>>6] |= 1 << (uint(rightPosition) & 63)
					}
					continue
				}
			}
			if prover.IsSubtype(left, b.construction[rightRow-1]) {
				bits[base+rightPosition>>6] |= 1 << (uint(rightPosition) & 63)
			}
		}
	}
	runtime.subtypeBits = bits
	return nil
}

func copyRuntimeRows(rows []runtimeRow, owner *Runtime) []runtimeRow {
	copied := append([]runtimeRow(nil), rows...)
	for index := range copied {
		copied[index].inner = rewriteRuntimeChild(copied[index].inner, owner)
	}
	return copied
}

func copyRuntimeVariants(variants []runtimeChild, owner *Runtime) []runtimeChild {
	copied := append([]runtimeChild(nil), variants...)
	for index := range copied {
		copied[index] = rewriteRuntimeChild(copied[index], owner)
	}
	return copied
}

func rewriteRuntimeChild(child runtimeChild, owner *Runtime) runtimeChild {
	if !child.present {
		return child
	}
	child.inner.owner = owner
	return child
}

func (b *runtimeBuilder) subtypeRelationInstalled() bool {
	return b != nil && b.runtime != nil && len(b.runtime.subtypeBits) != 0 && len(b.runtime.closedRows) != 0
}
