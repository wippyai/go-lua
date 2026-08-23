package typeauthority

import (
	"errors"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// familyPrefix is the Family-owned Runtime universe of the eight primitive
// seeds coalesced with every admitted member plane. SealFamily pays this once.
// A family-only SealRuntime copies it; mixed local inputs still ingest fresh
// until the merge remap lands.
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
