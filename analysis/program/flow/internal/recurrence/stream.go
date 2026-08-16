package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func materialize(
	counts [keyspace.FamilyCount]uint32,
	parts components,
	heads []keyspace.Term,
	trace eventTrace,
	work []arcWork,
	sourceID identity.ContentID,
	flowID identity.ContentID,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	result := &Result{
		annotations: make([]Annotation, len(work)),
		sourceID:    sourceID,
		flowID:      flowID,
		staticID:    staticID,
		moduleID:    moduleID,
	}
	for _, family := range [...]keyspace.Family{keyspace.FamilyLabel, keyspace.FamilyLoop} {
		count := counts[family]
		result.headSlots[family] = make([]headSlot, int(count)+1)
	}
	for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
		count := counts[family]
		result.decisionSlots[family] = make([]decisionSlot, int(count)+1)
	}
	componentCounts := make([]uint32, len(parts.sizes))
	for _, event := range trace.events {
		if event.component >= uint32(len(parts.sizes)) || event.component >= uint32(len(heads)) ||
			event.node >= uint32(len(parts.of)) || parts.of[event.node] != event.component || event.component == unassignedComponent {
			return nil, errors.New("program/flow/recurrence: semantic event component is invalid")
		}
		if heads[event.component] != 0 {
			if componentCounts[event.component] == ^uint32(0) {
				return nil, errors.New("program/flow/recurrence: component stream count overflows")
			}
			componentCounts[event.component]++
		}
	}
	componentStart := make([]uint32, len(parts.sizes)+1)
	for component := range componentCounts {
		if uint64(componentStart[component])+uint64(componentCounts[component]) > uint64(^uint32(0)) {
			return nil, errors.New("program/flow/recurrence: component stream offset overflows")
		}
		componentStart[component+1] = componentStart[component] + componentCounts[component]
	}
	result.streams = make([]keyspace.Term, componentStart[len(componentCounts)])
	next := make([]uint32, len(componentCounts))
	copy(next, componentStart[:len(componentCounts)])
	for component, head := range heads {
		if head == 0 {
			continue
		}
		family, ordinal := keyspace.TermFamily(head), keyspace.TermOrdinal(head)
		if (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || ordinal == 0 || uint64(ordinal) >= uint64(len(result.headSlots[family])) {
			return nil, errors.New("program/flow/recurrence: Mu head is not an existing Label or Loop")
		}
		result.headSlots[family][ordinal] = headSlot{start: componentStart[component], end: componentStart[component+1], live: true}
	}
	for _, event := range trace.events {
		component := event.component
		if component >= uint32(len(parts.sizes)) || component >= uint32(len(heads)) || event.node >= uint32(len(parts.of)) ||
			parts.of[event.node] != component || component == unassignedComponent {
			return nil, errors.New("program/flow/recurrence: semantic event component changed during materialization")
		}
		if heads[component] == 0 {
			continue // acyclic decisions are not part of a Mu head stream
		}
		at := next[component]
		if uint64(at) >= uint64(len(result.streams)) {
			return nil, errors.New("program/flow/recurrence: decision stream overflow")
		}
		result.streams[at] = event.term
		next[component]++
		family, ordinal := keyspace.TermFamily(event.term), keyspace.TermOrdinal(event.term)
		if family != keyspace.FamilySelect && family != keyspace.FamilyBranch && family != keyspace.FamilyLoop ||
			ordinal == 0 || uint64(ordinal) >= uint64(len(result.decisionSlots[family])) || result.decisionSlots[family][ordinal].head != 0 {
			return nil, errors.New("program/flow/recurrence: decision stream contains a duplicate")
		}
		result.decisionSlots[family][ordinal] = decisionSlot{head: heads[component], rank: at - componentStart[component]}
	}
	if err := fillRanges(result, heads, trace, work); err != nil {
		return nil, err
	}
	return result, nil
}

// fillRanges translates global event boundaries into each head's local stream
// rank. Events from different SCCs may be interleaved in the semantic walk,
// so arcWork.first/past cannot be copied directly into the materialized
// stream. Boundary hooks keep the conversion deterministic and linear:
// hooks are bucketed by global stamp, then one scan counts events per head.
func fillRanges(result *Result, heads []keyspace.Term, trace eventTrace, work []arcWork) error {
	eventCount := len(trace.events)
	if uint64(eventCount) >= uint64(^uint(0)>>1) || uint64(eventCount) > uint64(^uint32(0))-1 {
		return errors.New("program/flow/recurrence: event denominator overflows boundary stamp")
	}
	stampCount := eventCount + 1 // boundaries include the point after the final event
	startCounts := make([]uint32, stampCount)
	endCounts := make([]uint32, stampCount)
	recurrentCount := 0
	for _, item := range work {
		if !item.recurrent {
			continue
		}
		recurrentCount++
		if item.component >= uint32(len(heads)) || heads[item.component] == 0 || heads[item.component] != item.head ||
			item.first > item.past || uint64(item.past) > uint64(eventCount) {
			return errors.New("program/flow/recurrence: recurrence range boundary is invalid")
		}
		if item.first >= uint32(len(startCounts)) || item.past >= uint32(len(endCounts)) {
			return errors.New("program/flow/recurrence: recurrence range boundary is out of bounds")
		}
		if startCounts[item.first] == ^uint32(0) || endCounts[item.past] == ^uint32(0) {
			return errors.New("program/flow/recurrence: recurrence boundary bucket overflows")
		}
		startCounts[item.first]++
		endCounts[item.past]++
	}
	startOffsets, startHooks, err := boundaryHooks(startCounts, work, true, recurrentCount)
	if err != nil {
		return err
	}
	endOffsets, endHooks, err := boundaryHooks(endCounts, work, false, recurrentCount)
	if err != nil {
		return err
	}
	local := make([]uint32, len(heads))
	started := make([]bool, len(work))
	ended := make([]bool, len(work))
	for stamp := 0; stamp < stampCount; stamp++ {
		for _, raw := range startHooks[startOffsets[stamp]:startOffsets[stamp+1]] {
			index := int(raw)
			item := work[index]
			if item.component >= uint32(len(local)) || heads[item.component] != item.head || started[index] {
				return errors.New("program/flow/recurrence: duplicate or foreign range start hook")
			}
			result.annotations[index].Head = item.head
			result.annotations[index].First = local[item.component]
			started[index] = true
		}
		for _, raw := range endHooks[endOffsets[stamp]:endOffsets[stamp+1]] {
			index := int(raw)
			item := work[index]
			if item.component >= uint32(len(local)) || heads[item.component] != item.head || ended[index] {
				return errors.New("program/flow/recurrence: duplicate or foreign range end hook")
			}
			if !started[index] || result.annotations[index].First > local[item.component] {
				return errors.New("program/flow/recurrence: range end precedes range start")
			}
			result.annotations[index].Past = local[item.component]
			ended[index] = true
		}
		if stamp == eventCount {
			break
		}
		event := trace.events[stamp]
		if event.component >= uint32(len(local)) || heads[event.component] == 0 {
			continue
		}
		if local[event.component] == ^uint32(0) {
			return errors.New("program/flow/recurrence: local decision rank overflows")
		}
		local[event.component]++
	}
	for index, item := range work {
		if !item.recurrent {
			continue
		}
		if !started[index] || !ended[index] {
			return errors.New("program/flow/recurrence: recurrence boundary hook was not discharged")
		}
		start, end, ok := result.headRange(item.head)
		if !ok || result.annotations[index].Past > end-start || result.annotations[index].First > result.annotations[index].Past {
			return errors.New("program/flow/recurrence: recurrence range escapes head stream")
		}
	}
	return nil
}

// boundaryHooks returns a dense [stamp,stamp+1) index over arc ordinals. It
// uses the same stable work order for equal stamps, so replaying a sealed
// source produces byte-identical annotations without a map or sort.
func boundaryHooks(counts []uint32, work []arcWork, starts bool, total int) ([]uint32, []uint32, error) {
	offsets := make([]uint32, len(counts)+1)
	for stamp, count := range counts {
		previous := offsets[stamp]
		if uint64(previous)+uint64(count) > uint64(^uint32(0)) {
			return nil, nil, errors.New("program/flow/recurrence: boundary hook index overflows")
		}
		offsets[stamp+1] = previous + count
	}
	if uint64(offsets[len(counts)]) != uint64(total) {
		return nil, nil, errors.New("program/flow/recurrence: boundary hook denominator disagrees")
	}
	hooks := make([]uint32, total)
	next := make([]uint32, len(counts))
	copy(next, offsets[:len(counts)])
	for index, item := range work {
		if !item.recurrent {
			continue
		}
		stamp := item.past
		if starts {
			stamp = item.first
		}
		if uint64(stamp) >= uint64(len(counts)) {
			return nil, nil, errors.New("program/flow/recurrence: boundary hook stamp is invalid")
		}
		at := next[stamp]
		if uint64(at) >= uint64(len(hooks)) {
			return nil, nil, errors.New("program/flow/recurrence: boundary hook fill overflows")
		}
		hooks[at] = uint32(index)
		next[stamp]++
	}
	return offsets, hooks, nil
}
