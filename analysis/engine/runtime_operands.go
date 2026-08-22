// runtime_operands.go seals the fused operand plane and its two source
// transposes, and marks operands when a publication or a candidate mints a
// new value for one.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// buildStateOperandPlane is the contextual counterpart of buildOperandPlane.
// Its source axis is StateOrdinal and its producer axis is the compact
// admitted stateGroupIndex row, so a mounted runtime never allocates a dense
// StateCount×GroupCount operand product.
func buildStateOperandPlane(runtime *solverRuntime, _ factorSourceColumn, regions []runtimeRegion) (*operandPlane, bool) {
	if runtime == nil || !runtime.artifactBacked || runtime.graph == nil || !runtime.producerRows.valid() {
		return nil, false
	}
	rowCount, points := len(runtime.producerRows.rows), runtime.stateCount()
	if points <= 0 {
		return nil, false
	}
	plane := &operandPlane{groupBase: make([]int32, rowCount+1), windows: make([]int32, len(regions)*int(operandKindCount)+1), ingressSource: make([]bool, points), regions: len(regions)}
	total := 0
	for rowIndex, row := range runtime.producerRows.rows {
		plane.groupBase[rowIndex] = int32(total)
		if row.group < 0 || row.group >= len(runtime.producers) {
			return nil, false
		}
		width := runtime.producers[row.group].group.InputCount() + 1
		if width <= 0 || total > operandPlaneMax-width {
			return nil, false
		}
		total += width
	}
	plane.groupBase[rowCount] = int32(total)
	for index, region := range regions {
		for kind := operandKind(0); kind < operandKindCount; kind++ {
			width := len(region.operandRow(kind))
			at := index*int(operandKindCount) + int(kind)
			plane.windows[at] = int32(total)
			if total > operandPlaneMax-width {
				return nil, false
			}
			total += width
		}
	}
	plane.windows[len(regions)*int(operandKindCount)] = int32(total)
	plane.total = total
	pointCounts := make([]int32, points+1)
	groupCounts := make([]int32, rowCount+1)
	visit := func(record func(source int, byGroup bool, ordinal uint32, window int32) bool) bool {
		for rowIndex, row := range runtime.producerRows.rows {
			if row.state < 0 || int(row.state) >= points || row.group < 0 || row.group >= len(runtime.producers) {
				return false
			}
			producer := runtime.producers[row.group]
			base := plane.groupBase[rowIndex]
			for inputIndex := 0; inputIndex < producer.group.InputCount(); inputIndex++ {
				input, ok := producer.group.InputAt(inputIndex)
				graphPoint, indexed := runtime.graph.PointIndex(input.Point())
				source, sourceOK := runtime.stateForGraphPoint(int(row.state), graphPoint)
				if !ok || !indexed || !sourceOK || source < 0 || source >= points || !record(source, false, uint32(base)+uint32(inputIndex), operandGroupWindow) {
					return false
				}
			}
			if producer.environment != nil {
				input, ok := producer.group.EnvironmentInput()
				graphPoint, indexed := runtime.graph.PointIndex(input.Point())
				source, sourceOK := runtime.stateForGraphPoint(int(row.state), graphPoint)
				if !ok || !indexed || !sourceOK || source < 0 || source >= points || !record(source, false, uint32(base)+uint32(producer.group.InputCount()), operandGroupWindow) {
					return false
				}
			}
		}
		for regionIndex, region := range regions {
			window := int32(regionIndex * int(operandKindCount))
			for position, row := range region.external {
				if row < 0 || row >= rowCount || !record(row, true, uint32(plane.windows[window+int32(operandExternalProducer)])+uint32(position), window+int32(operandExternalProducer)) {
					return false
				}
			}
			for position, row := range region.back {
				if row < 0 || row >= rowCount || !record(row, true, uint32(plane.windows[window+int32(operandBackProducer)])+uint32(position), window+int32(operandBackProducer)) {
					return false
				}
			}
			environmentRows := [2]struct {
				edges []int
				kind  operandKind
			}{{region.environmentExternal, operandExternalEnvironment}, {region.environmentBack, operandBackEnvironment}}
			for _, inputRow := range environmentRows {
				for position, edgeIndex := range inputRow.edges {
					if edgeIndex < 0 || edgeIndex >= len(runtime.environments) {
						return false
					}
					edge := runtime.environments[edgeIndex]
					source, sourceOK := runtime.stateForGraphPoint(region.head, edge.source)
					if !sourceOK || source < 0 || source >= points || !record(source, false, uint32(plane.windows[window+int32(inputRow.kind)])+uint32(position), window+int32(inputRow.kind)) {
						return false
					}
				}
			}
			contextRows := [2]struct {
				transports []int
				kind       operandKind
			}{{region.contextExternal, operandExternalContext}, {region.contextBack, operandBackContext}}
			for _, inputRow := range contextRows {
				for position, transportIndex := range inputRow.transports {
					if transportIndex < 0 || transportIndex >= len(runtime.contextTransports) {
						return false
					}
					transport := runtime.contextTransports[transportIndex]
					source, sourceOK := runtime.contextTransportSourceState(transport.to, transport.sourcePoint)
					if !sourceOK || source != transport.from {
						return false
					}
					if source < 0 || source >= points || !record(source, false, uint32(plane.windows[window+int32(inputRow.kind)])+uint32(position), window+int32(inputRow.kind)) {
						return false
					}
				}
			}
			factorRows := [2]struct {
				edges []int
				kind  operandKind
			}{{region.factorExternal, operandExternalFactor}, {region.factorBack, operandBackFactor}}
			for _, inputRow := range factorRows {
				for position, factorRow := range inputRow.edges {
					if factorRow < 0 || factorRow >= len(runtime.stateFactorRows) {
						return false
					}
					source := runtime.stateFactorRows[factorRow].source
					if source < 0 || source >= points || !record(source, false, uint32(plane.windows[window+int32(inputRow.kind)])+uint32(position), window+int32(inputRow.kind)) {
						return false
					}
				}
			}
			for position, source := range region.points {
				if source < 0 || source >= points || !record(source, false, uint32(plane.windows[window+int32(operandRegionPoint)])+uint32(position), window+int32(operandRegionPoint)) {
					return false
				}
			}
		}
		return true
	}
	if !visit(func(source int, byGroup bool, _ uint32, _ int32) bool {
		if byGroup {
			groupCounts[source+1]++
		} else {
			pointCounts[source+1]++
		}
		return true
	}) {
		return nil, false
	}
	for index := 1; index < len(pointCounts); index++ {
		pointCounts[index] += pointCounts[index-1]
	}
	for index := 1; index < len(groupCounts); index++ {
		groupCounts[index] += groupCounts[index-1]
	}
	plane.byPoint = operandTranspose{offsets: pointCounts, operand: make([]uint32, pointCounts[points]), window: make([]int32, pointCounts[points])}
	plane.byGroup = operandTranspose{offsets: groupCounts, operand: make([]uint32, groupCounts[rowCount]), window: make([]int32, groupCounts[rowCount])}
	pointCursor, groupCursor := make([]int32, points), make([]int32, rowCount)
	if !visit(func(source int, byGroup bool, ordinal uint32, window int32) bool {
		transpose, cursor := &plane.byPoint, pointCursor
		if byGroup {
			transpose, cursor = &plane.byGroup, groupCursor
		}
		at := transpose.offsets[source] + cursor[source]
		if at < transpose.offsets[source] || int(at) >= len(transpose.operand) || at >= transpose.offsets[source+1] {
			return false
		}
		transpose.operand[at], transpose.window[at] = ordinal, window
		if !byGroup && window >= 0 && operandKind(int(window)%int(operandKindCount)).ingress() {
			plane.ingressSource[source] = true
		}
		cursor[source]++
		return true
	}) {
		return nil, false
	}
	return plane, true
}

// operandKind names the nine region rows the recurrence reads. They are the
// rows the epoch used to shadow with parallel version vectors; the vectors are
// gone and the rows are now positions in one fused plane.
type operandKind uint8

const (
	operandExternalProducer operandKind = iota
	operandBackProducer
	operandExternalEnvironment
	operandBackEnvironment
	operandExternalContext
	operandBackContext
	operandExternalFactor
	operandBackFactor
	operandRegionPoint
	operandKindCount
)

// external reports whether this row feeds the Region's external ingress. The
// remaining producer/environment/factor rows are back ingress; the point row
// is the Region's own interior publication surface.
func (kind operandKind) external() bool {
	return kind == operandExternalProducer || kind == operandExternalEnvironment || kind == operandExternalContext || kind == operandExternalFactor
}

func (kind operandKind) back() bool {
	return kind == operandBackProducer || kind == operandBackEnvironment || kind == operandBackContext || kind == operandBackFactor
}

// operandTranspose is the inverse of the forward operand rows, stored as
// parallel columns over one source plane: offsets is the CSR row directory,
// operand carries the fused ordinal, and window carries the forward row that
// owns it so a mark reaches its sole reader without a search. A negative
// window names a Group input row, whose reader is the Group itself.
type operandTranspose struct {
	offsets []int32
	operand []uint32
	window  []int32
}

const operandGroupWindow int32 = -1

// operandPlaneMax bounds the fused plane at the width its int32 window
// directory and uint32 ordinals can address.
const operandPlaneMax = int(^uint32(0) >> 1)

// operandPlane is the one sealed plane over every value the recurrence and
// the producer candidates read as an operand. Group input operands occupy the
// low ordinals and keep their positions across an activation epoch; region
// operands occupy the high ordinals and are re-derived whenever the region
// rows are, because a selected FactorEdge changes their widths.
//
// Region and position are never stored per operand: both are recovered from
// the half-open window this plane is built from.
type operandPlane struct {
	// groupBase[group] opens that Group's input window. The window is one
	// wider than the input count: the last position is the designated
	// environment input, which is absent for most Groups and is then never
	// marked.
	groupBase []int32
	// windows[region*operandKindCount+kind] opens one region row; the row ends
	// at the next entry. The directory is one flat row so a kind is an offset,
	// never a second indirection.
	windows []int32
	byPoint operandTranspose
	byGroup operandTranspose
	// ingressSource marks the Points that source at least one Region ingress
	// operand. Only those publications are read by a recurrence accumulator,
	// so only those must classify their authored-coverage axis as well as
	// their state axis.
	ingressSource []bool
	total         int
	regions       int
}

// ingress reports whether this row is one a recurrence accumulator reads.
func (kind operandKind) ingress() bool { return kind.external() || kind.back() }

// factorSourceColumn is the only factor-edge column the operand plane reads:
// the source Point of one edge index. An overlay serves the column of the
// frontier it will publish, so the plane is derived from the rows before
// they are assigned to the runtime.
type factorSourceColumn struct {
	installed    []runtimeFactorEdge
	additions    []preparedFactorAddition
	replacements []preparedFactorReplacement
}

func installedFactorSources(edges []runtimeFactorEdge) factorSourceColumn {
	return factorSourceColumn{installed: edges}
}

func (column factorSourceColumn) source(index int) (int, bool) {
	if index < 0 {
		return 0, false
	}
	if index >= len(column.installed) {
		addition := index - len(column.installed)
		if addition >= len(column.additions) {
			return 0, false
		}
		return column.additions[addition].edge.source, true
	}
	for _, replacement := range column.replacements {
		if replacement.index == index {
			return replacement.edge.source, true
		}
	}
	return column.installed[index].source, true
}

// operandRow is the one authority for which region row an operand kind is
// built from. The plane scatters a row in exactly this order, so a reader that
// compares two frontiers reads the same positions the plane addresses.
func (region *runtimeRegion) operandRow(kind operandKind) []int {
	switch kind {
	case operandExternalProducer:
		return region.external
	case operandBackProducer:
		return region.back
	case operandExternalEnvironment:
		return region.environmentExternal
	case operandBackEnvironment:
		return region.environmentBack
	case operandExternalContext:
		return region.contextExternal
	case operandBackContext:
		return region.contextBack
	case operandExternalFactor:
		return region.factorExternal
	case operandBackFactor:
		return region.factorBack
	case operandRegionPoint:
		return region.points
	}
	return nil
}

// regionRowCarry is the change-fact a re-derived plane carries a region under.
// Nothing survives a rebuild that this does not prove: it is the sole reason
// an epoch may keep a tick, an exact row or a retained accumulator across a
// frontier installation.
type regionRowCarry uint8

const (
	// regionRowRetained: every row of this region is the row the previous
	// frontier sealed. The region kept its operands, so it keeps its ticks and
	// its episode.
	regionRowRetained regionRowCarry = iota
	// regionRowExtended: every previous operand is still at its own position
	// and the frontier only appended. Adding a join term ascends the row, so a
	// retained accumulator still bounds it from below; the appended positions
	// are the ones that read as changed.
	regionRowExtended
	// regionRowRebuilt: an operand moved position or changed its source. The
	// previous row bounds nothing here, so every operand reads as changed and
	// the evidence axis refuses reuse.
	regionRowRebuilt
)

func (carry regionRowCarry) worse(other regionRowCarry) regionRowCarry {
	if other > carry {
		return other
	}
	return carry
}

// regionFrontierCarry classifies the regions of an installed frontier against
// the regions the running epoch was sealed over.
//
// A recurrence is named by its head Point, never by its region ordinal: one
// installation re-derives the schedule, so a newly demanded cycle can take an
// ordinal an existing region held. previousOf carries that correspondence -
// one entry per installed region, naming the region it continues or -1 for a
// region this frontier opens - and carry names what the frontier did to its
// rows.
//
// repointed names the factor edge indexes whose source the frontier moved.
// Such an edge keeps its row position and its width, so width alone cannot see
// it; it is the one change a positional comparison must be told about.
func regionFrontierCarry(previous, next []runtimeRegion, activePrevious, activeNext []bool, repointed map[int]struct{}) ([]int, []regionRowCarry, bool) {
	if len(activePrevious) != len(previous) || len(activeNext) != len(next) {
		return nil, nil, false
	}
	continues := make(map[int]int, len(previous))
	for index := range previous {
		if !activePrevious[index] || !previous[index].active {
			continue
		}
		if _, duplicate := continues[previous[index].head]; duplicate {
			return nil, nil, false
		}
		continues[previous[index].head] = index
	}
	previousOf := make([]int, len(next))
	carries := make([]regionRowCarry, len(next))
	for index := range next {
		previousOf[index], carries[index] = -1, regionRowRebuilt
		if !activeNext[index] {
			continue
		}
		source, continued := continues[next[index].head]
		if !continued || regionFrontierParentHead(previous, source) != regionFrontierParentHead(next, index) {
			continue
		}
		previousOf[index] = source
		carry := regionRowRetained
		for kind := operandKind(0); kind < operandKindCount; kind++ {
			carry = carry.worse(operandRowCarry(previous[source].operandRow(kind), next[index].operandRow(kind), kind, repointed))
		}
		carries[index] = carry
	}
	return previousOf, carries, true
}

// regionFrontierParentHead names one region's enclosing recurrence by the head
// Point of its parent, so a nesting change is visible across two frontiers
// whose region ordinals are not the same index space.
func regionFrontierParentHead(regions []runtimeRegion, index int) int {
	if index < 0 || index >= len(regions) {
		return -1
	}
	parent := regions[index].parent
	if parent < 0 || parent >= len(regions) {
		return -1
	}
	return regions[parent].head
}

// operandRowCarry classifies one row. The frontier only ever appends, so a
// previous row that is not a prefix of the next one is a reordering, and a
// reordering invalidates every position the tick space addresses.
func operandRowCarry(previous, next []int, kind operandKind, repointed map[int]struct{}) regionRowCarry {
	if len(next) < len(previous) {
		return regionRowRebuilt
	}
	for position, member := range previous {
		if next[position] != member {
			return regionRowRebuilt
		}
		if kind != operandExternalFactor && kind != operandBackFactor {
			continue
		}
		if _, moved := repointed[member]; moved {
			return regionRowRebuilt
		}
	}
	if len(next) != len(previous) {
		return regionRowExtended
	}
	return regionRowRetained
}

// buildOperandPlane counts once and scatters once over the already-sealed
// region rows. It runs exactly where those rows are built, so a rebuilt
// activation epoch pays one extra pass over rows it is rebuilding anyway.
func buildOperandPlane(graph *equation.Graph, producers []runtimeProducer, environments []runtimeEnvironment, factorEdges factorSourceColumn, regions []runtimeRegion) (*operandPlane, bool) {
	if graph == nil || len(producers) != graph.GroupCount() {
		return nil, false
	}
	points := graph.PointCount()
	plane := &operandPlane{groupBase: make([]int32, len(producers)+1), windows: make([]int32, len(regions)*int(operandKindCount)+1), ingressSource: make([]bool, points), regions: len(regions)}
	total := 0
	for index, producer := range producers {
		plane.groupBase[index] = int32(total)
		width := producer.group.InputCount() + 1
		if width <= 0 || total > operandPlaneMax-width {
			return nil, false
		}
		total += width
	}
	plane.groupBase[len(producers)] = int32(total)
	for index, region := range regions {
		for kind := operandKind(0); kind < operandKindCount; kind++ {
			width := len(region.operandRow(kind))
			plane.windows[index*int(operandKindCount)+int(kind)] = int32(total)
			if width < 0 || total > operandPlaneMax-width {
				return nil, false
			}
			total += width
		}
	}
	plane.windows[len(regions)*int(operandKindCount)] = int32(total)
	plane.total = total

	pointCounts := make([]int32, points+1)
	groupCounts := make([]int32, len(producers)+1)
	// One count pass and one scatter pass share this visitor, so the two
	// passes cannot disagree about which operand belongs to which source.
	visit := func(record func(source int, byGroup bool, ordinal uint32, window int32) bool) bool {
		for groupIndex, producer := range producers {
			base := plane.groupBase[groupIndex]
			for inputIndex := 0; inputIndex < producer.group.InputCount(); inputIndex++ {
				input, inputOK := producer.group.InputAt(inputIndex)
				if !inputOK {
					return false
				}
				source, indexed := graph.PointIndex(input.Point())
				if !indexed || source < 0 || source >= points {
					return false
				}
				if !record(source, false, uint32(base)+uint32(inputIndex), operandGroupWindow) {
					return false
				}
			}
			if producer.environment == nil {
				continue
			}
			input, inputOK := producer.group.EnvironmentInput()
			if !inputOK {
				return false
			}
			source, indexed := graph.PointIndex(input.Point())
			if !indexed || source < 0 || source >= points {
				return false
			}
			if !record(source, false, uint32(base)+uint32(producer.group.InputCount()), operandGroupWindow) {
				return false
			}
		}
		for regionIndex, region := range regions {
			window := int32(regionIndex * int(operandKindCount))
			for position, group := range region.external {
				if group < 0 || group >= len(producers) {
					return false
				}
				if !record(group, true, uint32(plane.windows[window+int32(operandExternalProducer)])+uint32(position), window+int32(operandExternalProducer)) {
					return false
				}
			}
			for position, group := range region.back {
				if group < 0 || group >= len(producers) {
					return false
				}
				if !record(group, true, uint32(plane.windows[window+int32(operandBackProducer)])+uint32(position), window+int32(operandBackProducer)) {
					return false
				}
			}
			environmentRows := [2]struct {
				edges []int
				kind  operandKind
			}{{region.environmentExternal, operandExternalEnvironment}, {region.environmentBack, operandBackEnvironment}}
			for _, row := range environmentRows {
				for position, edge := range row.edges {
					if edge < 0 || edge >= len(environments) {
						return false
					}
					source := environments[edge].source
					if source < 0 || source >= points {
						return false
					}
					if !record(source, false, uint32(plane.windows[window+int32(row.kind)])+uint32(position), window+int32(row.kind)) {
						return false
					}
				}
			}
			factorRows := [2]struct {
				edges []int
				kind  operandKind
			}{{region.factorExternal, operandExternalFactor}, {region.factorBack, operandBackFactor}}
			for _, row := range factorRows {
				for position, edge := range row.edges {
					source, resolved := factorEdges.source(edge)
					if !resolved || source < 0 || source >= points {
						return false
					}
					if !record(source, false, uint32(plane.windows[window+int32(row.kind)])+uint32(position), window+int32(row.kind)) {
						return false
					}
				}
			}
			for position, point := range region.points {
				if point < 0 || point >= points {
					return false
				}
				if !record(point, false, uint32(plane.windows[window+int32(operandRegionPoint)])+uint32(position), window+int32(operandRegionPoint)) {
					return false
				}
			}
		}
		return true
	}

	if !visit(func(source int, byGroup bool, _ uint32, _ int32) bool {
		if byGroup {
			groupCounts[source+1]++
			return true
		}
		pointCounts[source+1]++
		return true
	}) {
		return nil, false
	}
	for index := 1; index < len(pointCounts); index++ {
		pointCounts[index] += pointCounts[index-1]
	}
	for index := 1; index < len(groupCounts); index++ {
		groupCounts[index] += groupCounts[index-1]
	}
	plane.byPoint = operandTranspose{offsets: pointCounts, operand: make([]uint32, pointCounts[points]), window: make([]int32, pointCounts[points])}
	plane.byGroup = operandTranspose{offsets: groupCounts, operand: make([]uint32, groupCounts[len(producers)]), window: make([]int32, groupCounts[len(producers)])}
	pointCursor := make([]int32, points)
	groupCursor := make([]int32, len(producers))
	if !visit(func(source int, byGroup bool, ordinal uint32, window int32) bool {
		transpose, cursor := &plane.byPoint, pointCursor
		if byGroup {
			transpose, cursor = &plane.byGroup, groupCursor
		}
		at := transpose.offsets[source] + cursor[source]
		if at < transpose.offsets[source] || int(at) >= len(transpose.operand) || at >= transpose.offsets[source+1] {
			return false
		}
		transpose.operand[at] = ordinal
		transpose.window[at] = window
		if !byGroup && window >= 0 && operandKind(int(window)%int(operandKindCount)).ingress() {
			plane.ingressSource[source] = true
		}
		cursor[source]++
		return true
	}) {
		return nil, false
	}
	return plane, true
}

// sourceOperandOrdinals borrows the window of operands one published Point
// mints a new value for. The slices are plane-owned and are never retained.
func (plane *operandPlane) sourceOperandOrdinals(point int) ([]uint32, []int32, bool) {
	if plane == nil || point < 0 || point+1 >= len(plane.byPoint.offsets) {
		return nil, nil, false
	}
	begin, end := plane.byPoint.offsets[point], plane.byPoint.offsets[point+1]
	if begin < 0 || end < begin || int(end) > len(plane.byPoint.operand) {
		return nil, nil, false
	}
	return plane.byPoint.operand[begin:end], plane.byPoint.window[begin:end], true
}

// groupOperandOrdinals is the same borrowed window for a replaced Group
// candidate, whose value no Point publication mints.
func (plane *operandPlane) groupOperandOrdinals(group int) ([]uint32, []int32, bool) {
	if plane == nil || group < 0 || group+1 >= len(plane.byGroup.offsets) {
		return nil, nil, false
	}
	begin, end := plane.byGroup.offsets[group], plane.byGroup.offsets[group+1]
	if begin < 0 || end < begin || int(end) > len(plane.byGroup.operand) {
		return nil, nil, false
	}
	return plane.byGroup.operand[begin:end], plane.byGroup.window[begin:end], true
}

// groupOperandAt is the fused ordinal of one Group input operand. Position
// equals the input index; the designated environment input occupies the
// position immediately after the last ordinary input.
func (plane *operandPlane) groupOperandAt(group, position int) (uint32, bool) {
	if plane == nil || group < 0 || group+1 >= len(plane.groupBase) || position < 0 {
		return 0, false
	}
	begin, end := int(plane.groupBase[group]), int(plane.groupBase[group+1])
	if begin+position >= end {
		return 0, false
	}
	return uint32(begin + position), true
}

// regionWindow opens one forward row. The half-open interval is the row's
// complete membership; position is the offset inside it.
func (plane *operandPlane) regionWindow(region int, kind operandKind) (int, int, bool) {
	if plane == nil || region < 0 || region >= plane.regions || kind >= operandKindCount {
		return 0, 0, false
	}
	at := region*int(operandKindCount) + int(kind)
	begin, end := int(plane.windows[at]), int(plane.windows[at+1])
	if begin < 0 || end < begin || end > plane.total {
		return 0, 0, false
	}
	return begin, end, true
}

// operandRegion recovers the owning region, row and position of one fused
// ordinal from the window directory. Nothing on the marking path calls it:
// it exists for the restart diagnostic, which reports the row a stale operand
// belongs to.
func (plane *operandPlane) operandRegion(ordinal uint32) (int, operandKind, int, bool) {
	if plane == nil || int(ordinal) >= plane.total || int(ordinal) < int(plane.groupBase[len(plane.groupBase)-1]) {
		return 0, 0, 0, false
	}
	low, high := 0, len(plane.windows)-1
	for low < high {
		middle := (low + high + 1) / 2
		if plane.windows[middle] <= int32(ordinal) {
			low = middle
		} else {
			high = middle - 1
		}
	}
	region, kind := low/int(operandKindCount), operandKind(low%int(operandKindCount))
	if region >= plane.regions {
		return 0, 0, 0, false
	}
	return region, kind, int(ordinal) - int(plane.windows[low]), true
}

// operandEpoch is the epoch-local change layer over the sealed plane. One
// monotone clock stamps every mark; each reader keeps the clock value it last
// read at, so one plane serves every Region and every Group without a shared
// reset and without a per-reader mark set.
type operandEpoch struct {
	plane *operandPlane
	tick  []uint64
	clock uint64
}

// openable is open's whole fallible half. A caller that must complete every
// admission before it begins mutating admits the plane here and commits it
// with openAdmitted.
func (state *operandEpoch) openable(plane *operandPlane) bool {
	return state != nil && plane != nil
}

// open installs a plane over this epoch. It is the first open of an epoch:
// no reader has closed an interface epoch yet, so the whole plane starts
// unmarked. A re-derived frontier goes through reopen, which must carry what
// its readers already remember.
func (state *operandEpoch) open(plane *operandPlane) bool {
	if !state.openable(plane) {
		return false
	}
	_ = state.openAdmitted(plane, nil, nil)
	return true
}

// openAdmitted is open's commit half; openable must have admitted the plane.
//
// carry is the per-region change-fact of a re-derived frontier, one entry per
// region, and nil for a first open. Group input ordinals keep their positions
// across a rebuild, so their ticks are copied whenever the prefix width is
// unchanged. Region ordinals are re-derived, so their ticks are copied exactly
// as far as carry proves the row unchanged and stamped at the live clock
// everywhere else: a stamped operand reads as changed, which is the only
// fail-closed answer, while a zeroed one would read as unchanged and hide a
// mark its reader has not yet folded.
func (state *operandEpoch) openAdmitted(plane *operandPlane, previousOf []int, carry []regionRowCarry) []uint8 {
	if state.clock < 1 {
		state.clock = 1
	}
	previous := state.plane
	width := int(plane.groupBase[len(plane.groupBase)-1])
	retained := previous != nil && width == int(previous.groupBase[len(previous.groupBase)-1]) && width <= len(state.tick) && width <= plane.total
	ticks := make([]uint64, plane.total)
	if retained {
		copy(ticks, state.tick[:width])
	} else if previous != nil {
		// The Group input rows moved, so no reader's stamp can be compared
		// against a tick in the new space. Stamping the whole prefix at the
		// live clock makes every input read as changed, which is the only
		// fail-closed answer: a zeroed prefix would read as unchanged.
		for index := 0; index < width; index++ {
			ticks[index] = state.clock
		}
	}
	var stamped []uint8
	if previous != nil {
		stamped = state.carryRegionTicks(plane, previous, ticks, previousOf, carry)
	}
	state.plane, state.tick = plane, ticks
	return stamped
}

// carryRegionTicks copies the region half of the tick space into the plane the
// frontier re-derived. A retained row keeps every tick; an extended row keeps
// the ticks of the operands it kept and stamps the appended tail; anything
// else is stamped whole. The returned rows carry one bit per stamped operand
// kind, which is the mark the region reading that row still owes its evidence
// axis.
func (state *operandEpoch) carryRegionTicks(plane, previous *operandPlane, ticks []uint64, previousOf []int, carry []regionRowCarry) []uint8 {
	stamped := make([]uint8, plane.regions)
	for region := 0; region < plane.regions; region++ {
		fact, source := regionRowRebuilt, -1
		if region < len(carry) && region < len(previousOf) {
			fact, source = carry[region], previousOf[region]
		}
		for kind := operandKind(0); kind < operandKindCount; kind++ {
			begin, end, ok := plane.regionWindow(region, kind)
			if !ok {
				continue
			}
			carried := 0
			if fact != regionRowRebuilt && source >= 0 {
				if oldBegin, oldEnd, oldOK := previous.regionWindow(source, kind); oldOK && oldEnd <= len(state.tick) && oldEnd-oldBegin <= end-begin {
					copy(ticks[begin:end], state.tick[oldBegin:oldEnd])
					carried = oldEnd - oldBegin
				}
			}
			if begin+carried < end {
				stamped[region] |= 1 << uint(kind)
			}
			for ordinal := begin + carried; ordinal < end; ordinal++ {
				ticks[ordinal] = state.clock
			}
		}
	}
	return stamped
}

// advance closes one reader's epoch: the returned stamp is strictly below
// every mark that follows it, and at or above every mark that preceded it.
//
// An exhausted clock hands back the floor stamp instead of its own position.
// Every existing mark then reads as newer than the reader, which refuses
// every reuse; handing back the exhausted position would read as older than
// nothing and admit them all.
func (state *operandEpoch) advance() uint64 {
	if state == nil || state.clock == ^uint64(0) {
		return 0
	}
	at := state.clock
	state.clock++
	return at
}

func (state *operandEpoch) changedSince(ordinal uint32, at uint64) bool {
	return state != nil && int(ordinal) < len(state.tick) && state.tick[ordinal] > at
}

// markSourceOperands records that one published Point minted a new value for
// every operand that reads it, and routes the classified evidence to the
// Region that owns each operand.
func (epoch *executorEpoch) markSourceOperands(point int, evidence change.Set) bool {
	if epoch == nil || epoch.operands.plane == nil {
		return false
	}
	ordinals, windows, ok := epoch.operands.plane.sourceOperandOrdinals(point)
	if !ok {
		return false
	}
	return epoch.markOperands(ordinals, windows, evidence)
}

// markGroupOperands records the same fact for a replaced producer candidate,
// whose recurrence operand no Point publication mints.
func (epoch *executorEpoch) markGroupOperands(group int, evidence change.Set) bool {
	if epoch == nil || epoch.operands.plane == nil {
		return false
	}
	if epoch.runtime != nil && epoch.runtime.artifactBacked {
		if epoch.currentState < 0 {
			return false
		}
		row, ok := epoch.runtime.producerRows.row(contextfiber.StateOrdinal(epoch.currentState), group)
		if !ok {
			return false
		}
		group = row
	}
	ordinals, windows, ok := epoch.operands.plane.groupOperandOrdinals(group)
	if !ok {
		return false
	}
	return epoch.markOperands(ordinals, windows, evidence)
}

func (epoch *executorEpoch) markOperands(ordinals []uint32, windows []int32, evidence change.Set) bool {
	if len(ordinals) != len(windows) {
		return false
	}
	clock := epoch.operands.clock
	for index, ordinal := range ordinals {
		if int(ordinal) >= len(epoch.operands.tick) {
			return false
		}
		epoch.operands.tick[ordinal] = clock
		window := windows[index]
		if window < 0 {
			continue
		}
		region, kind := int(window)/int(operandKindCount), operandKind(int(window)%int(operandKindCount))
		if region < 0 || region >= len(epoch.regions) {
			return false
		}
		state := &epoch.regions[region]
		switch {
		case kind.external():
			state.externalAt = clock
			state.pending = state.pending.Union(evidence)
		case kind.back():
			state.backAt = clock
			state.pending = state.pending.Union(evidence)
		default:
			state.pointsAt = clock
		}
	}
	return true
}

// changedRegionOperands appends the positions of one region row whose operand
// moved since at. It is the delta the recurrence fold consumes in place of
// the complete row.
func (epoch *executorEpoch) changedRegionOperands(region int, kind operandKind, row []int, at uint64, into []int) ([]int, bool) {
	begin, end, ok := epoch.operands.plane.regionWindow(region, kind)
	if !ok || end-begin != len(row) {
		return into, false
	}
	for position := range row {
		if epoch.operands.changedSince(uint32(begin+position), at) {
			into = append(into, row[position])
		}
	}
	return into, true
}

// producerInputChanged reports whether the Group input at position minted a
// new value after the candidate cache stamped at closed its input epoch. It
// replaces the ordered input-version snapshot each cache used to copy and
// diff elementwise.
func (epoch *executorEpoch) producerInputChanged(group, position int, at uint64) bool {
	if epoch == nil || epoch.runtime == nil {
		return false
	}
	if epoch.runtime.artifactBacked {
		if epoch.currentState < 0 {
			return false
		}
		row, ok := epoch.runtime.producerRows.row(contextfiber.StateOrdinal(epoch.currentState), group)
		if !ok {
			return false
		}
		group = row
	}
	ordinal, ok := epoch.operands.plane.groupOperandAt(group, position)
	return ok && epoch.operands.changedSince(ordinal, at)
}

// producerInputsChanged reports whether any ordinary input moved. The
// designated environment input is a separate question, because the executor
// classifies it against a different order law.
func (epoch *executorEpoch) producerInputsChanged(group, inputs int, at uint64) bool {
	for position := 0; position < inputs; position++ {
		if epoch.producerInputChanged(group, position, at) {
			return true
		}
	}
	return false
}

// regionContains answers recurrence membership from the frontier-local region
// rows instead of the base graph's private region table. Region ordinals are
// per-frontier and unstable: runtime.regions and runtime.pointRegion are
// installed atomically by the same activation overlay, so the ordinal asked
// about and the ancestry walked always belong to one frontier, while the base
// graph may not contain the ordinal at all.
//
// WTO intervals are laminar, so containment is exactly ancestor-or-self of the
// Point's innermost region. The walk is bounded by the region count: a parent
// chain that does not terminate inside it is corruption, and fails closed.
func (epoch *executorEpoch) regionContains(region int, point equation.Point) (bool, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || region < 0 || region >= len(epoch.runtime.regions) {
		return false, false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointRegion) {
		return false, false
	}
	if epoch.runtime.artifactBacked {
		if epoch.currentState < 0 {
			return false, false
		}
		pointIndex, indexed = epoch.runtime.stateForGraphPoint(epoch.currentState, pointIndex)
		if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointRegion) {
			return false, false
		}
	}
	at := epoch.runtime.pointRegion[pointIndex]
	for steps := 0; at != schedule.NoRegion; steps++ {
		if at < 0 || at >= len(epoch.runtime.regions) || steps > len(epoch.runtime.regions) {
			return false, false
		}
		if at == region {
			return true, true
		}
		at = epoch.runtime.regions[at].parent
	}
	return false, true
}

// classifiesCoverage reports whether a publication of this Point is read by a
// recurrence accumulator, and therefore owes the authored-coverage half of its
// classification. Every other publication carries only the state-axis evidence
// its own operation issued.
func (state *operandEpoch) classifiesCoverage(point int) bool {
	return state != nil && state.plane != nil && point >= 0 && point < len(state.plane.ingressSource) && state.plane.ingressSource[point]
}
