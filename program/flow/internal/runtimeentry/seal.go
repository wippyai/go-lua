package runtimeentry

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Seal copies the complete normalized Entry directory once. The functional
// successor relation has at most one followed child per occurrence; the
// tri-state walk memoizes each row, making sealing linear in terms plus typed
// Values/Write incidence.
func Seal(sourceView source.View, flow authored.View, control *sourcecontrol.Result, ports *evaluation.Ports,
	exec *executable.Result, staticID, moduleID keyspace.ContentID) (*Result, error) {
	sourceID, flowID := sourceView.Identity().ContentID(), flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		!sourcecontrol.Matches(control, sourceID, flowID, staticID, moduleID) ||
		!evaluation.Matches(ports, sourceID, flowID, staticID, moduleID) ||
		!executable.Matches(exec, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/runtimeentry: owner proofs are unavailable")
	}
	r := &Result{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID, control: control, ports: ports, exec: exec}
	var states [keyspace.FamilyCount][]uint8
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := sourceView.Identity().FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/runtimeentry: Source denominator is invalid")
		}
		if count != 0 {
			r.entries[family] = make([]keyspace.Term, count+1)
			states[family] = make([]uint8, count+1)
		}
	}
	builder := entryBuilder{flow: flow, ports: ports, exec: exec, entries: &r.entries, states: &states}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		plane := r.entries[family]
		for ordinal := 1; ordinal < len(plane); ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if _, err := builder.resolve(term); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

type entryBuilder struct {
	flow    authored.View
	ports   *evaluation.Ports
	exec    *executable.Result
	entries *[keyspace.FamilyCount][]keyspace.Term
	states  *[keyspace.FamilyCount][]uint8
	frames  []entryFrame
}

type entryFrame struct {
	term    keyspace.Term
	family  keyspace.Family
	ordinal uint32
}

// resolve is deliberately iterative. A malformed authored chain can be as
// deep as the complete term denominator; sealing must fail closed without
// consuming the Go call stack. frames is builder-owned scratch and is reused
// across roots, so total work and allocation remain linear.
func (b *entryBuilder) resolve(term keyspace.Term) (keyspace.Term, error) {
	b.frames = b.frames[:0]
	current := term
	for {
		family, ordinal, valid := b.slot(current)
		if !valid {
			b.clearFrames()
			return 0, errors.New("program/flow/runtimeentry: term is outside the denominator")
		}
		switch b.states[family][ordinal] {
		case 2:
			entry := b.entries[family][ordinal]
			if entry != 0 && !b.exec.Executable(entry) {
				b.states[family][ordinal] = 0
				b.entries[family][ordinal] = 0
				b.clearFrames()
				return 0, errors.New("program/flow/runtimeentry: sealed entry is not executable")
			}
			if len(b.frames) != 0 && entry == 0 {
				b.states[family][ordinal] = 0
				b.entries[family][ordinal] = 0
				b.clearFrames()
				return 0, errors.New("program/flow/runtimeentry: followed child has no executable entry")
			}
			b.completeFrames(entry)
			return entry, nil
		case 1:
			// current is either in this exact stack (a cycle) or an uncovered
			// visiting row from a prior failed traversal. Clear both cases.
			b.states[family][ordinal] = 0
			b.entries[family][ordinal] = 0
			b.clearFrames()
			return 0, errors.New("program/flow/runtimeentry: typed entry relation is cyclic or uncovered")
		}

		b.states[family][ordinal] = 1
		b.frames = append(b.frames, entryFrame{term: current, family: family, ordinal: ordinal})
		entry, child, follow := b.step(current)
		if follow {
			current = child
			continue
		}
		b.completeFrames(entry)
		return entry, nil
	}
}

func (b *entryBuilder) slot(term keyspace.Term) (keyspace.Family, uint32, bool) {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family, ordinal, family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 &&
		uint64(ordinal) < uint64(len(b.entries[family]))
}

func (b *entryBuilder) step(term keyspace.Term) (entry, child keyspace.Term, follow bool) {
	if b.exec.Executable(term) {
		if projected, ok := b.ports.Entry(term); ok && b.exec.Executable(projected) {
			return projected, 0, false
		}
	}
	if next, ok := b.firstChild(term); ok {
		return 0, next, true
	}
	if b.exec.Executable(term) {
		return term, 0, false
	}
	return 0, 0, false
}

func (b *entryBuilder) completeFrames(entry keyspace.Term) {
	for index := len(b.frames) - 1; index >= 0; index-- {
		frame := b.frames[index]
		b.entries[frame.family][frame.ordinal] = entry
		b.states[frame.family][frame.ordinal] = 2
	}
	b.frames = b.frames[:0]
}

func (b *entryBuilder) clearFrames() {
	for _, frame := range b.frames {
		b.entries[frame.family][frame.ordinal] = 0
		b.states[frame.family][frame.ordinal] = 0
	}
	b.frames = b.frames[:0]
}

func (b *entryBuilder) firstChild(term keyspace.Term) (keyspace.Term, bool) {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyRead:
		_, child, _, ok := b.flow.Storage().Reads().Get(term)
		return child, ok && keyspace.TermFamily(child) != keyspace.FamilyCell && b.exec.Executable(child)
	case keyspace.FamilyLensExact:
		_, child, sourceTerm, fieldKind, ok := b.flow.Access().Exact().Get(term)
		if !ok {
			return 0, false
		}
		if b.exec.Executable(child) {
			return child, true
		}
		return sourceTerm, fieldKind == kind.FieldExact && sourceTerm != 0 && b.exec.Executable(sourceTerm)
	case keyspace.FamilyLensKey:
		_, base, key, ok := b.flow.Access().Dynamic().Get(term)
		if !ok {
			return 0, false
		}
		if b.exec.Executable(base) {
			return base, true
		}
		return key, key != 0 && b.exec.Executable(key)
	case keyspace.FamilyUnary:
		_, _, child, ok := b.flow.Operators().Unaries().Get(term)
		return child, ok && b.exec.Executable(child)
	case keyspace.FamilyBinary:
		_, _, left, right, ok := b.flow.Operators().Binaries().Get(term)
		if !ok {
			return 0, false
		}
		if b.exec.Executable(left) {
			return left, true
		}
		return right, b.exec.Executable(right)
	case keyspace.FamilySelect:
		_, _, left, right, ok := b.flow.Operators().Selects().Get(term)
		if !ok {
			return 0, false
		}
		if b.exec.Executable(left) {
			return left, true
		}
		return right, b.exec.Executable(right)
	case keyspace.FamilyValues:
		values := b.flow.Values()
		length, ok := values.Len(term)
		if !ok {
			return 0, false
		}
		for index := 0; index < length; index++ {
			member, memberOK := values.Member(term, index)
			if memberOK && b.exec.Executable(member) {
				return member, true
			}
		}
		_, tail, rowOK := values.Get(term)
		return tail, rowOK && tail != 0 && b.exec.Executable(tail)
	case keyspace.FamilyValueClaim:
		_, child, _, ok := b.flow.Claims().Get(term)
		return child, ok && b.exec.Executable(child)
	case keyspace.FamilyBind:
		_, child, ok := b.flow.Storage().Binds().Get(term)
		return child, ok && b.exec.Executable(child)
	case keyspace.FamilyAssign:
		assigns := b.flow.Storage().Assigns()
		count, ok := assigns.WriteCount(term)
		if !ok {
			return 0, false
		}
		for index := 0; index < count; index++ {
			write, writeOK := assigns.WriteAt(term, index)
			if !writeOK {
				return 0, false
			}
			_, target, targetOK := b.flow.Storage().Writes().Get(write)
			if targetOK && keyspace.TermFamily(target) != keyspace.FamilyCell && b.exec.Executable(target) {
				return target, true
			}
		}
		_, child, rowOK := assigns.Get(term)
		return child, rowOK && b.exec.Executable(child)
	case keyspace.FamilyCall:
		_, callee, _, actuals, ok := b.flow.Calls().Get(term)
		if !ok {
			return 0, false
		}
		if b.exec.Executable(callee) {
			return callee, true
		}
		return actuals, actuals != 0 && b.exec.Executable(actuals)
	case keyspace.FamilyTableField:
		_, key, values, fieldKind, ok := b.flow.Fields().Get(term)
		if !ok {
			return 0, false
		}
		if (fieldKind == kind.FieldKey || fieldKind == kind.FieldExact) && b.exec.Executable(key) {
			return key, true
		}
		return values, values != 0 && b.exec.Executable(values)
	case keyspace.FamilyReturn:
		_, child, ok := b.flow.Control().Returns().Get(term)
		return child, ok && b.exec.Executable(child)
	case keyspace.FamilyBranch:
		_, child, _, _, ok := b.flow.Control().Branches().Get(term)
		return child, ok && b.exec.Executable(child)
	case keyspace.FamilyLoop:
		_, bodyTerm, loopKind, controlTerm, ok := b.flow.Control().Loops().Get(term)
		if !ok {
			return 0, false
		}
		if loopKind == kind.LoopRepeat {
			return bodyTerm, b.exec.Executable(bodyTerm)
		}
		return controlTerm, b.exec.Executable(controlTerm)
	default:
		return 0, false
	}
}
