package body

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
)

// Seal proves Body structural authority over the live Source and authored
// views. It derives only parent/root/activation/loop projections; all other
// Flow geometry belongs to later verticals.
func Seal(preimage source.Preimage, view authored.View, staticView staticquery.View, entry keyspace.Term) (*Result, error) {
	if !staticView.Available() {
		return nil, errors.New("program/flow/body: Static view expired")
	}
	identity, order, faults := preimage.Identity(), preimage.Order(), preimage.Faults()
	sourceID := identity.ContentID()
	flowID := view.ContentID()
	if !sourceID.Available() || !flowID.Available() {
		return nil, errors.New("program/flow/body: owner identity unavailable")
	}
	counts, err := liveCounts(identity, faults, view, staticView)
	if err != nil {
		return nil, err
	}
	bodyCount := counts[keyspace.FamilyBody]
	if bodyCount == 0 || !validTerm(entry, counts, keyspace.FamilyBody) {
		return nil, errors.New("program/flow/body: invalid Entry Body")
	}

	functions := view.Functions()
	calls := view.Calls()
	storage := view.Storage()
	binds := storage.Binds()
	assigns := storage.Assigns()
	control := view.Control()
	returns := control.Returns()
	breaks := control.Breaks()
	branches := control.Branches()
	loops := control.Loops()
	gotos := control.Gotos()
	labels := control.Labels()
	staticDeclarations := staticView.Declarations()
	aliases := staticDeclarations.Aliases()
	interfaces := staticDeclarations.Interfaces()
	if err := exactFamily(functions.Count(), counts, keyspace.FamilyFunction); err != nil {
		return nil, err
	}
	if err := exactFamily(binds.Count(), counts, keyspace.FamilyBind); err != nil {
		return nil, err
	}
	if err := exactFamily(assigns.Count(), counts, keyspace.FamilyAssign); err != nil {
		return nil, err
	}
	if err := exactFamily(calls.Count(), counts, keyspace.FamilyCall); err != nil {
		return nil, err
	}
	if err := exactFamily(returns.Count(), counts, keyspace.FamilyReturn); err != nil {
		return nil, err
	}
	if err := exactFamily(breaks.Count(), counts, keyspace.FamilyBreak); err != nil {
		return nil, err
	}
	if err := exactFamily(gotos.Count(), counts, keyspace.FamilyGoto); err != nil {
		return nil, err
	}
	if err := exactFamily(branches.Count(), counts, keyspace.FamilyBranch); err != nil {
		return nil, err
	}
	if err := exactFamily(loops.Count(), counts, keyspace.FamilyLoop); err != nil {
		return nil, err
	}
	if err := exactFamily(labels.Count(), counts, keyspace.FamilyLabel); err != nil {
		return nil, err
	}
	if err := exactFamily(faults.Count(), counts, keyspace.FamilyControlFault); err != nil {
		return nil, err
	}
	if err := exactFamily(aliases.Count(), counts, keyspace.FamilyTypeAlias); err != nil {
		return nil, err
	}
	if err := exactFamily(interfaces.Count(), counts, keyspace.FamilyTypeInterface); err != nil {
		return nil, err
	}

	if err := validateOrdinals(functions, binds, assigns, calls, returns, breaks, gotos, branches, loops, labels, faults, aliases, interfaces, counts); err != nil {
		return nil, err
	}

	parents := make([]keyspace.Term, bodyCount+1)
	functionAt := make([]keyspace.Term, bodyCount+1)
	loopAt := make([]keyspace.Term, bodyCount+1)
	roots := make([]keyspace.Term, 0)
	rootOffsets := make([]uint32, bodyCount+1)
	rootSeen := newRootSeen(counts)

	// Source order is the sole direct Body parent authority and the sole root
	// ordering authority. Every direct source term is checked against the
	// Source identity and the closed source admission set; only the nine
	// executable statement families become roots.
	for bodyOrdinal := 1; bodyOrdinal <= bodyCount; bodyOrdinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal))
		if uint64(len(roots)) > uint64(^uint32(0)) {
			return nil, errors.New("program/flow/body: root pool overflow")
		}
		rootOffsets[bodyOrdinal-1] = uint32(len(roots))
		length, ok := order.BodyLen(body)
		if !ok || length < 0 {
			return nil, errors.New("program/flow/body: Body order view is not live")
		}
		for index := 0; index < length; index++ {
			term, ok := order.BodyAt(body, index)
			if !ok || !validTerm(term, counts, keyspace.TermFamily(term)) {
				return nil, errors.New("program/flow/body: invalid direct source Term")
			}
			family := keyspace.TermFamily(term)
			if !sourceDirectFamily(family) {
				return nil, errors.New("program/flow/body: forbidden direct source Term family")
			}
			if family == keyspace.FamilyBody {
				if err := setParent(parents, term, body); err != nil {
					return nil, err
				}
				if uint64(len(roots)) >= uint64(^uint32(0)) {
					return nil, errors.New("program/flow/body: root pool overflow")
				}
				roots = append(roots, term)
				continue
			}
			seen := rootSeen[family]
			ordinal := keyspace.TermOrdinal(term)
			if seen == nil || ordinal == 0 || uint64(ordinal) > uint64(len(seen)) || seen[ordinal-1] {
				return nil, errors.New("program/flow/body: duplicate direct statement root")
			}
			owner, ok := directOwner(term, binds, assigns, calls, returns, breaks, gotos, branches, loops, labels, faults, aliases, interfaces)
			if !ok || owner != body {
				return nil, errors.New("program/flow/body: statement root owner mismatch")
			}
			seen[ordinal-1] = true
			if RootFamily(family) {
				if uint64(len(roots)) >= uint64(^uint32(0)) {
					return nil, errors.New("program/flow/body: root pool overflow")
				}
				roots = append(roots, term)
			}
		}
		if uint64(len(roots)) > uint64(^uint32(0)) {
			return nil, errors.New("program/flow/body: root pool overflow")
		}
		rootOffsets[bodyOrdinal] = uint32(len(roots))
	}
	if err := completeRootSeen(rootSeen, counts); err != nil {
		return nil, err
	}

	// The remaining three authorities are authored Flow rows. Their relation
	// hosts are Body terms; each child receives exactly one parent slot.
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return nil, errors.New("program/flow/body: Function view is not live")
		}
		owner, child, _, ok := functions.Get(function)
		if !ok || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(child, counts, keyspace.FamilyBody) || owner == child {
			return nil, errors.New("program/flow/body: invalid Function Body authority")
		}
		if err := setParent(parents, child, owner); err != nil {
			return nil, err
		}
		childOrdinal := keyspace.TermOrdinal(child)
		if functionAt[childOrdinal] != 0 {
			return nil, errors.New("program/flow/body: duplicate Function Body authority")
		}
		functionAt[childOrdinal] = function
	}
	for index := 0; index < branches.Count(); index++ {
		branch, ok := branches.At(index)
		if !ok {
			return nil, errors.New("program/flow/body: Branch view is not live")
		}
		owner, _, whenTrue, whenFalse, ok := branches.Get(branch)
		if !ok || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(whenTrue, counts, keyspace.FamilyBody) ||
			!validTerm(whenFalse, counts, keyspace.FamilyBody) || owner == whenTrue || owner == whenFalse || whenTrue == whenFalse {
			return nil, errors.New("program/flow/body: invalid Branch Body authority")
		}
		if err := setParent(parents, whenTrue, owner); err != nil {
			return nil, err
		}
		if err := setParent(parents, whenFalse, owner); err != nil {
			return nil, err
		}
	}
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok {
			return nil, errors.New("program/flow/body: Loop view is not live")
		}
		owner, child, kind, _, ok := loops.Get(loop)
		if !ok || !validLoopKind(kind) || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(child, counts, keyspace.FamilyBody) || owner == child {
			return nil, errors.New("program/flow/body: invalid Loop Body authority")
		}
		if err := setParent(parents, child, owner); err != nil {
			return nil, err
		}
		childOrdinal := keyspace.TermOrdinal(child)
		if loopAt[childOrdinal] != 0 {
			return nil, errors.New("program/flow/body: duplicate Loop Body authority")
		}
		loopAt[childOrdinal] = loop
	}

	entryOrdinal := keyspace.TermOrdinal(entry)
	if parents[entryOrdinal] != 0 {
		return nil, errors.New("program/flow/body: Entry has a parent")
	}
	zeroParents := 0
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		if parents[ordinal] == 0 {
			zeroParents++
			if uint32(ordinal) != entryOrdinal {
				return nil, errors.New("program/flow/body: orphan Body")
			}
		}
	}
	if zeroParents != 1 {
		return nil, errors.New("program/flow/body: Body forest has multiple roots")
	}

	activation, nearestLoop, pre, post, err := walk(parents, functionAt, loopAt, entryOrdinal)
	if err != nil {
		return nil, err
	}

	return &Result{
		sourceID:    sourceID,
		flowID:      flowID,
		entry:       entry,
		parents:     parents,
		roots:       roots,
		rootOffsets: rootOffsets,
		activation:  activation,
		nearestLoop: nearestLoop,
		pre:         pre,
		post:        post,
	}, nil
}

func liveCounts(identity source.Identity, faults source.Faults, view authored.View, staticView staticquery.View) ([keyspace.FamilyCount]int, error) {
	var counts [keyspace.FamilyCount]int
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 || !view.ContentID().Available() {
		return counts, errors.New("program/flow/body: owner view expired")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/body: invalid Source family cardinality")
		}
		counts[family] = count
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) {
		return counts, errors.New("program/flow/body: Source family cardinality mismatch")
	}
	return counts, nil
}

func exactFamily(actual int, counts [keyspace.FamilyCount]int, family keyspace.Family) error {
	if actual != counts[family] {
		return errors.New("program/flow/body: authored family cardinality mismatch")
	}
	return nil
}

func validateOrdinals(
	functions authored.Functions,
	binds authored.Binds,
	assigns authored.Assigns,
	calls authored.Calls,
	returns authored.Returns,
	breaks authored.Breaks,
	gotos authored.Gotos,
	branches authored.Branches,
	loops authored.Loops,
	labels authored.Labels,
	faults source.Faults,
	aliases staticdecl.Aliases,
	interfaces staticdecl.Interfaces,
	counts [keyspace.FamilyCount]int,
) error {
	for index := 0; index < functions.Count(); index++ {
		term, ok := functions.At(index)
		if !ok || !validExact(term, keyspace.FamilyFunction, index, counts) {
			return errors.New("program/flow/body: invalid Function ordinal")
		}
	}
	for index := 0; index < binds.Count(); index++ {
		term, ok := binds.At(index)
		if !ok || !validExact(term, keyspace.FamilyBind, index, counts) {
			return errors.New("program/flow/body: invalid Bind ordinal")
		}
	}
	for index := 0; index < assigns.Count(); index++ {
		term, ok := assigns.At(index)
		if !ok || !validExact(term, keyspace.FamilyAssign, index, counts) {
			return errors.New("program/flow/body: invalid Assign ordinal")
		}
	}
	for index := 0; index < calls.Count(); index++ {
		term, ok := calls.At(index)
		if !ok || !validExact(term, keyspace.FamilyCall, index, counts) {
			return errors.New("program/flow/body: invalid Call ordinal")
		}
	}
	for index := 0; index < returns.Count(); index++ {
		term, ok := returns.At(index)
		if !ok || !validExact(term, keyspace.FamilyReturn, index, counts) {
			return errors.New("program/flow/body: invalid Return ordinal")
		}
	}
	for index := 0; index < breaks.Count(); index++ {
		term, ok := breaks.At(index)
		if !ok || !validExact(term, keyspace.FamilyBreak, index, counts) {
			return errors.New("program/flow/body: invalid Break ordinal")
		}
	}
	for index := 0; index < gotos.Count(); index++ {
		term, ok := gotos.At(index)
		if !ok || !validExact(term, keyspace.FamilyGoto, index, counts) {
			return errors.New("program/flow/body: invalid Goto ordinal")
		}
	}
	for index := 0; index < branches.Count(); index++ {
		term, ok := branches.At(index)
		if !ok || !validExact(term, keyspace.FamilyBranch, index, counts) {
			return errors.New("program/flow/body: invalid Branch ordinal")
		}
	}
	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok || !validExact(term, keyspace.FamilyLoop, index, counts) {
			return errors.New("program/flow/body: invalid Loop ordinal")
		}
	}
	for index := 0; index < labels.Count(); index++ {
		term, ok := labels.At(index)
		if !ok || !validExact(term, keyspace.FamilyLabel, index, counts) {
			return errors.New("program/flow/body: invalid Label ordinal")
		}
	}
	for index := 0; index < faults.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyControlFault, uint32(index+1))
		if !validExact(term, keyspace.FamilyControlFault, index, counts) {
			return errors.New("program/flow/body: invalid ControlFault ordinal")
		}
	}
	for index := 0; index < aliases.Count(); index++ {
		term, ok := aliases.At(index)
		if !ok || !validExact(term, keyspace.FamilyTypeAlias, index, counts) {
			return errors.New("program/flow/body: invalid TypeAlias ordinal")
		}
	}
	for index := 0; index < interfaces.Count(); index++ {
		term, ok := interfaces.At(index)
		if !ok || !validExact(term, keyspace.FamilyTypeInterface, index, counts) {
			return errors.New("program/flow/body: invalid TypeInterface ordinal")
		}
	}
	return nil
}

func setParent(parents []keyspace.Term, child, parent keyspace.Term) error {
	if keyspace.TermFamily(child) != keyspace.FamilyBody || keyspace.TermFamily(parent) != keyspace.FamilyBody || child == parent {
		return errors.New("program/flow/body: invalid Body parent authority")
	}
	childOrdinal, parentOrdinal := keyspace.TermOrdinal(child), keyspace.TermOrdinal(parent)
	if childOrdinal == 0 || parentOrdinal == 0 || uint64(childOrdinal) >= uint64(len(parents)) || uint64(parentOrdinal) >= uint64(len(parents)) || parents[childOrdinal] != 0 {
		return errors.New("program/flow/body: duplicate or shared Body authority")
	}
	parents[childOrdinal] = parent
	return nil
}

func sourceDirectFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall, keyspace.FamilyBranch,
		keyspace.FamilyLoop, keyspace.FamilyBody, keyspace.FamilyReturn, keyspace.FamilyBreak,
		keyspace.FamilyGoto, keyspace.FamilyLabel, keyspace.FamilyControlFault,
		keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface:
		return true
	default:
		return false
	}
}

// RootFamily reports the closed direct-statement family admitted to the Body
// root relation. Downstream structural passes use this owner-defined law
// instead of repeating a second family switch.
func RootFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall, keyspace.FamilyBranch,
		keyspace.FamilyLoop, keyspace.FamilyBody, keyspace.FamilyReturn, keyspace.FamilyBreak,
		keyspace.FamilyGoto:
		return true
	default:
		return false
	}
}

func validLoopKind(loopKind kind.LoopKind) bool {
	return loopKind >= kind.LoopWhile && loopKind <= kind.LoopGenericFor
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]int, family keyspace.Family) bool {
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		uint64(keyspace.TermOrdinal(term)) <= uint64(counts[family])
}

func validExact(term keyspace.Term, family keyspace.Family, index int, counts [keyspace.FamilyCount]int) bool {
	return validTerm(term, counts, family) && keyspace.TermOrdinal(term) == uint32(index+1)
}

func directOwner(
	term keyspace.Term,
	binds authored.Binds,
	assigns authored.Assigns,
	calls authored.Calls,
	returns authored.Returns,
	breaks authored.Breaks,
	gotos authored.Gotos,
	branches authored.Branches,
	loops authored.Loops,
	labels authored.Labels,
	faults source.Faults,
	aliases staticdecl.Aliases,
	interfaces staticdecl.Interfaces,
) (keyspace.Term, bool) {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBind:
		owner, _, ok := binds.Get(term)
		return owner, ok
	case keyspace.FamilyAssign:
		owner, _, ok := assigns.Get(term)
		return owner, ok
	case keyspace.FamilyCall:
		owner, _, _, _, ok := calls.Get(term)
		return owner, ok
	case keyspace.FamilyReturn:
		owner, _, ok := returns.Get(term)
		return owner, ok
	case keyspace.FamilyBreak:
		owner, _, ok := breaks.Get(term)
		return owner, ok
	case keyspace.FamilyGoto:
		owner, _, ok := gotos.Get(term)
		return owner, ok
	case keyspace.FamilyBranch:
		owner, _, _, _, ok := branches.Get(term)
		return owner, ok
	case keyspace.FamilyLoop:
		owner, _, _, _, ok := loops.Get(term)
		return owner, ok
	case keyspace.FamilyLabel:
		owner, ok := labels.Get(term)
		return owner, ok
	case keyspace.FamilyControlFault:
		fault, ok := faults.At(term)
		return fault.Owner, ok
	case keyspace.FamilyTypeAlias:
		owner, _, _, _, ok := aliases.Get(term)
		return owner, ok
	case keyspace.FamilyTypeInterface:
		owner, _, _, ok := interfaces.Get(term)
		return owner, ok
	default:
		return 0, false
	}
}

func newRootSeen(counts [keyspace.FamilyCount]int) [keyspace.FamilyCount][]bool {
	var seen [keyspace.FamilyCount][]bool
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall, keyspace.FamilyBranch,
		keyspace.FamilyLoop, keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyLabel, keyspace.FamilyControlFault, keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface,
	} {
		seen[family] = make([]bool, counts[family])
	}
	return seen
}

func completeRootSeen(seen [keyspace.FamilyCount][]bool, counts [keyspace.FamilyCount]int) error {
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyBranch,
		keyspace.FamilyLoop, keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyLabel, keyspace.FamilyControlFault, keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface,
	} {
		if len(seen[family]) != counts[family] {
			return errors.New("program/flow/body: root cardinality mismatch")
		}
		for _, present := range seen[family] {
			if !present {
				return errors.New("program/flow/body: authored statement lacks direct source root")
			}
		}
	}
	// Calls deliberately have dual structural modes. A direct Call is still
	// admitted by sourceDirectFamily/RootFamily and remains subject to the
	// direct-owner and duplicate checks above. A nested Call is not a Source
	// direct root; its enclosing expression/containment relation closes it.
	// Consequently Call is omitted from this direct-root completeness pass.
	return nil
}

func walk(parents, functionAt, loopAt []keyspace.Term, entry uint32) ([]keyspace.Term, []keyspace.Term, []uint32, []uint32, error) {
	bodyCount := len(parents) - 1
	if bodyCount <= 0 || len(functionAt) != len(parents) || len(loopAt) != len(parents) ||
		entry == 0 || int(entry) > bodyCount || uint64(bodyCount) > uint64(^uint32(0))/2 {
		return nil, nil, nil, nil, errors.New("program/flow/body: invalid Body interval denominator")
	}
	start := make([]uint32, bodyCount+2)
	for child := uint32(1); int(child) <= bodyCount; child++ {
		parent := parents[child]
		if parent == 0 {
			continue
		}
		parentOrdinal := keyspace.TermOrdinal(parent)
		if parentOrdinal == 0 || uint64(parentOrdinal) > uint64(bodyCount) {
			return nil, nil, nil, nil, errors.New("program/flow/body: invalid Body parent index")
		}
		start[parentOrdinal+1]++
	}
	for index := 1; index < len(start); index++ {
		start[index] += start[index-1]
	}
	next := append([]uint32(nil), start[:bodyCount+1]...)
	children := make([]uint32, bodyCount-1)
	for child := uint32(1); int(child) <= bodyCount; child++ {
		parent := parents[child]
		if parent == 0 {
			continue
		}
		parentOrdinal := keyspace.TermOrdinal(parent)
		at := next[parentOrdinal]
		if uint64(at) >= uint64(len(children)) {
			return nil, nil, nil, nil, errors.New("program/flow/body: Body child index overflow")
		}
		children[at] = child
		next[parentOrdinal]++
	}

	activation := make([]keyspace.Term, bodyCount+1)
	nearestLoop := make([]keyspace.Term, bodyCount+1)
	pre := make([]uint32, bodyCount+1)
	post := make([]uint32, bodyCount+1)
	seen := make([]bool, bodyCount+1)
	seen[entry] = true
	visited := 1
	maxTimestamp := uint64(bodyCount) * 2
	timestamp := uint64(1)
	pre[entry] = uint32(timestamp)
	type frame struct {
		body uint32
		next uint32
	}
	stack := make([]frame, 0)
	stack = append(stack, frame{body: entry, next: start[entry]})
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		end := start[top.body+1]
		if top.next >= end {
			if timestamp >= maxTimestamp {
				return nil, nil, nil, nil, errors.New("program/flow/body: Body interval overflow")
			}
			timestamp++
			post[top.body] = uint32(timestamp)
			stack = stack[:len(stack)-1]
			continue
		}
		child := children[top.next]
		top.next++
		if child == 0 || child > uint32(bodyCount) || seen[child] || keyspace.TermOrdinal(parents[child]) != top.body {
			return nil, nil, nil, nil, errors.New("program/flow/body: invalid or cyclic Body forest")
		}
		if timestamp >= maxTimestamp {
			return nil, nil, nil, nil, errors.New("program/flow/body: Body interval overflow")
		}
		seen[child] = true
		visited++
		timestamp++
		pre[child] = uint32(timestamp)
		active := activation[top.body]
		loop := nearestLoop[top.body]
		if functionAt[child] != 0 {
			active = functionAt[child]
			loop = 0
		}
		if loopAt[child] != 0 {
			loop = loopAt[child]
		}
		activation[child] = active
		nearestLoop[child] = loop
		stack = append(stack, frame{body: child, next: start[child]})
	}
	if visited != bodyCount {
		return nil, nil, nil, nil, errors.New("program/flow/body: unreachable or cyclic Body")
	}
	if timestamp != maxTimestamp {
		return nil, nil, nil, nil, errors.New("program/flow/body: malformed Body intervals")
	}
	return activation, nearestLoop, pre, post, nil
}
