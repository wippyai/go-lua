package sequence

// GroupedInputs is the synchronous, indexed input to AssembleGrouped. Groups
// are distinct source facts; every fixed slot and optional tail names one
// group. The layout is consumed during the call and never retained.
//
// Group identity intentionally stays outside sequence. The Pack domain knows
// only that equal indices denote one sampled source, which is the minimum
// information required to retain Lua expression-list correlation.
type GroupedInputs interface {
	GroupCount() int
	GroupAt(int) Value
	FixedCount() int
	FixedGroupAt(int) uint32
	TailGroup() (uint32, bool)
}

// AssembleGrouped is the correlated form of the one Lua Values law. It
// samples each distinct input group once, then uses that same realization for
// every slot naming the group. In particular it is not equivalent to applying
// Assemble to a list which happens to contain the same Value more than once.
//
// The only unbounded source-language component remains the final expression.
// For a correlated P = h*·s used as both one fixed slot and the tail, this
// emits [s,s] and h·h·h*·s, never the impossible [h,s]. Group identity and
// layout are cold caller data; this package owns only the Pack-language law.
func AssembleGrouped[I GroupedInputs](labels Labels, input I) Value {
	if labels == nil {
		return Bottom()
	}
	groups := input.GroupCount()
	if groups < 0 {
		return Bottom()
	}
	fixedCount := input.FixedCount()
	if fixedCount < 0 {
		return Bottom()
	}

	tailRaw, hasTail := input.TailGroup()
	tail := -1
	if hasTail {
		var ok bool
		tail, ok = groupedIndex(tailRaw, groups)
		if !ok {
			return Bottom()
		}
	}
	// A Bottom source makes the relation Bottom without inspecting or
	// materializing slot layout. This preserves the established short-circuit
	// allocation behavior for unreachable Values paths.
	for index := 0; index < groups; index++ {
		if input.GroupAt(index).IsBottom() {
			return Bottom()
		}
	}
	fixedGroups := make([]int, fixedCount)
	tailFixed := false
	for slot := range fixedGroups {
		index, ok := groupedIndex(input.FixedGroupAt(slot), groups)
		if !ok {
			return Bottom()
		}
		fixedGroups[slot] = index
		tailFixed = tailFixed || index == tail
	}
	choices := make([][]groupChoice, groups)
	for index := range choices {
		value := input.GroupAt(index)
		isTail := index == tail
		choices[index] = groupedChoices(labels, value, isTail, isTail && tailFixed)
		if len(choices[index]) == 0 {
			return Bottom()
		}
	}

	selected := make([]groupChoice, groups)
	modes := make([]Mode, 0)
	var choose func(int)
	choose = func(index int) {
		if index != groups {
			for _, choice := range choices[index] {
				selected[index] = choice
				choose(index + 1)
			}
			return
		}
		prefix := make([]Handle, len(fixedGroups))
		for slot, group := range fixedGroups {
			prefix[slot] = selected[group].scalar
		}
		if !hasTail {
			modes = append(modes, Mode{kind: ModeClosed, closed: closedWordFromSharedFlat(prefix)})
			return
		}
		choice := selected[tail]
		if choice.tailTop {
			modes = append(modes, Mode{kind: ModeOpaque, prefix: prefix})
			return
		}
		modes = append(modes, prependGroupedPrefix(prefix, choice.tail))
	}
	choose(0)
	return normalize(labels, modes)
}

func groupedIndex(raw uint32, count int) (int, bool) {
	index := int(raw)
	return index, count >= 0 && index >= 0 && index < count && uint32(index) == raw
}

type groupChoice struct {
	scalar  Handle
	tail    Mode
	tailTop bool
}

func groupedChoices(labels Labels, value Value, tail, tailFixed bool) []groupChoice {
	if value.IsTop() {
		if tail {
			if tailFixed {
				return []groupChoice{
					{scalar: labels.Nil(), tail: Mode{kind: ModeClosed}},
					{scalar: labels.TypeTop(), tail: Mode{kind: ModeOpaque, prefix: []Handle{labels.TypeTop()}}},
				}
			}
			return []groupChoice{{scalar: labels.TypeTop(), tailTop: true}}
		}
		return []groupChoice{{scalar: labels.TypeTop()}}
	}
	choices := make([]groupChoice, 0, value.ModeCount()*2)
	for _, mode := range value.modes {
		if tail {
			appendTailChoices(labels, &choices, mode, tailFixed)
			continue
		}
		appendScalarChoices(labels, &choices, mode)
	}
	return choices
}

func appendScalarChoices(labels Labels, out *[]groupChoice, mode Mode) {
	if mode.kind == ModeClosed {
		label, ok := mode.closed.At(0)
		if !ok {
			label = labels.Nil()
		}
		*out = append(*out, groupChoice{scalar: label})
		return
	}
	if len(mode.prefix) != 0 {
		*out = append(*out, groupChoice{scalar: mode.prefix[0]})
		return
	}
	if mode.kind == ModeKnown {
		*out = append(*out, groupChoice{scalar: mode.tail})
	} else {
		*out = append(*out, groupChoice{scalar: labels.TypeTop()})
	}
	if len(mode.suffix) != 0 {
		*out = append(*out, groupChoice{scalar: mode.suffix[0]})
	} else {
		*out = append(*out, groupChoice{scalar: labels.Nil()})
	}
}

// appendTailChoices conditions an open tail on the same zero-or-positive
// realization used by every fixed occurrence of that group. A nonempty fixed
// prefix already determines scalar(), so no split is needed.
func appendTailChoices(labels Labels, out *[]groupChoice, mode Mode, fixed bool) {
	if !fixed {
		*out = append(*out, groupChoice{tail: mode})
		return
	}
	if mode.kind == ModeClosed || len(mode.prefix) != 0 {
		label, ok := groupedFirst(labels, mode)
		if !ok {
			return
		}
		*out = append(*out, groupChoice{scalar: label, tail: mode})
		return
	}

	zeroScalar := labels.Nil()
	if len(mode.suffix) != 0 {
		zeroScalar = mode.suffix[0]
	}
	zero := Mode{kind: ModeClosed, closed: closedWordFromFlat(mode.suffix)}
	*out = append(*out, groupChoice{scalar: zeroScalar, tail: zero})

	if mode.kind == ModeKnown {
		positive := Mode{kind: ModeKnown, prefix: []Handle{mode.tail}, tail: mode.tail, suffix: mode.suffix}
		*out = append(*out, groupChoice{scalar: mode.tail, tail: positive})
		return
	}
	positive := Mode{kind: ModeOpaque, prefix: []Handle{labels.TypeTop()}, suffix: mode.suffix}
	*out = append(*out, groupChoice{scalar: labels.TypeTop(), tail: positive})
}

func groupedFirst(labels Labels, mode Mode) (Handle, bool) {
	if mode.kind == ModeClosed {
		label, ok := mode.closed.At(0)
		if !ok {
			return labels.Nil(), true
		}
		return label, true
	}
	if len(mode.prefix) != 0 {
		return mode.prefix[0], true
	}
	return Handle{}, false
}

func prependGroupedPrefix(prefix []Handle, tail Mode) Mode {
	switch tail.kind {
	case ModeClosed:
		return Mode{kind: ModeClosed, closed: concatClosedWords(closedWordFromSharedFlat(prefix), tail.closed)}
	case ModeKnown:
		return Mode{kind: ModeKnown, prefix: appendClosedPrefix(closedWordFromSharedFlat(prefix), tail.prefix), tail: tail.tail, suffix: tail.suffix}
	case ModeOpaque:
		return Mode{kind: ModeOpaque, prefix: appendClosedPrefix(closedWordFromSharedFlat(prefix), tail.prefix), suffix: tail.suffix}
	default:
		return Mode{}
	}
}
