package cfg

import (
	"sort"
	"unsafe"

	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// mapPtr returns a uintptr that uniquely identifies a map instance.
func mapPtr(m map[string]basecfg.SymbolID) uintptr {
	return *(*uintptr)(unsafe.Pointer(&m))
}

// ComputeSSAVersions computes SSA versions for all variables and inserts phi nodes.
// Uses Cytron et al. dominance-frontier phi placement + dominator-tree rename.
func (b *Builder) ComputeSSAVersions() {
	assignedSyms := b.collectAssignedSymbols()
	if len(assignedSyms) == 0 {
		return
	}

	defPoints := b.collectDefPoints(assignedSyms)
	domInfo := analysis.ComputeDomInfo(b.Cfg)
	phiSites := b.placePhis(assignedSyms, defPoints, domInfo.DominanceFrontier)
	b.renameSSA(assignedSyms, defPoints, phiSites, domInfo)
}

// collectAssignedSymbols returns the set of SymbolIDs that need SSA versions.
func (b *Builder) collectAssignedSymbols() map[basecfg.SymbolID]string {
	syms := make(map[basecfg.SymbolID]string)

	for _, info := range b.Info {
		switch v := info.(type) {
		case *AssignInfo:
			for _, target := range v.Targets {
				sym, name := assignmentTargetSymbol(target)
				if sym != 0 {
					syms[sym] = name
				}
			}
		case *FuncDefInfo:
			if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
				syms[v.Symbol] = v.Name
			}
		}
	}

	if b.ScopeTracker != nil {
		for sym, declPoint := range b.ScopeTracker.declPoints {
			if declPoint == 0 {
				if name := b.ScopeTracker.symbolNames[sym]; name != "" {
					syms[sym] = name
				}
			}
		}
	}

	if b.Cfg != nil && b.ScopeTracker != nil {
		entry := b.Cfg.Entry()
		if vis := b.ScopeTracker.visibility[entry]; vis != nil {
			for name, sym := range vis {
				if _, exists := syms[sym]; !exists {
					syms[sym] = name
				}
			}
			for name, sym := range b.ScopeTracker.globals {
				if _, inLocal := vis[name]; inLocal {
					continue
				}
				if _, exists := syms[sym]; !exists {
					syms[sym] = name
				}
			}
		}
	}

	return syms
}

// collectDefPoints returns the set of CFG points where each symbol is defined.
func (b *Builder) collectDefPoints(assignedSyms map[basecfg.SymbolID]string) map[basecfg.SymbolID][]basecfg.Point {
	defPoints := make(map[basecfg.SymbolID][]basecfg.Point)
	entry := b.Cfg.Entry()

	if b.ScopeTracker != nil {
		for sym := range assignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				defPoints[sym] = append(defPoints[sym], entry)
			}
		}
	}

	for pi := 0; pi < b.Cfg.Size(); pi++ {
		p := basecfg.Point(pi)
		info := b.Info[p]
		if info == nil {
			continue
		}
		switch v := info.(type) {
		case *AssignInfo:
			for _, target := range v.Targets {
				sym, _ := assignmentTargetSymbol(target)
				if sym == 0 {
					continue
				}
				if _, ok := assignedSyms[sym]; ok {
					defPoints[sym] = append(defPoints[sym], p)
				}
			}
		case *FuncDefInfo:
			if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
				if _, ok := assignedSyms[v.Symbol]; ok {
					defPoints[v.Symbol] = append(defPoints[v.Symbol], p)
				}
			}
		}
	}

	return defPoints
}

// placePhis uses the iterated dominance frontier to determine where phi nodes are needed.
func (b *Builder) placePhis(assignedSyms map[basecfg.SymbolID]string, defPoints map[basecfg.SymbolID][]basecfg.Point, df map[basecfg.Point][]basecfg.Point) map[basecfg.Point]map[basecfg.SymbolID]bool {
	phiSites := make(map[basecfg.Point]map[basecfg.SymbolID]bool)
	if b.ScopeTracker == nil {
		return phiSites
	}

	sortedSyms := make([]basecfg.SymbolID, 0, len(assignedSyms))

	for sym := range assignedSyms {
		sortedSyms = append(sortedSyms, sym)
	}

	sort.Slice(sortedSyms, func(i, j int) bool { return sortedSyms[i] < sortedSyms[j] })
	cfgSize := b.Cfg.Size()
	visibility := b.ScopeTracker.visibility
	globals := b.ScopeTracker.globals
	hasAlreadyMarks := make([]int, cfgSize)
	everOnWorklistMarks := make([]int, cfgSize)
	markEpoch := 0

	for _, sym := range sortedSyms {
		name := assignedSyms[sym]
		defs := defPoints[sym]

		if len(defs) == 0 {
			continue
		}

		markEpoch++
		worklist := make([]basecfg.Point, 0, len(defs))
		for _, d := range defs {
			di := int(d)
			if di < 0 || di >= cfgSize {
				continue
			}
			if everOnWorklistMarks[di] == markEpoch {
				continue
			}
			everOnWorklistMarks[di] = markEpoch
			worklist = append(worklist, d)
		}

		for len(worklist) > 0 {
			d := worklist[len(worklist)-1]
			worklist = worklist[:len(worklist)-1]

			for _, y := range df[d] {
				yi := int(y)
				if yi < 0 || yi >= cfgSize {
					continue
				}
				if hasAlreadyMarks[yi] == markEpoch {
					continue
				}

				vis := visibility[y]
				if vis == nil {
					continue
				}
				resolvedSym, ok := vis[name]
				if !ok {
					resolvedSym, ok = globals[name]
				}
				if !ok || resolvedSym != sym {
					continue
				}

				hasAlreadyMarks[yi] = markEpoch

				if phiSites[y] == nil {
					phiSites[y] = make(map[basecfg.SymbolID]bool)
				}

				phiSites[y][sym] = true

				if everOnWorklistMarks[yi] != markEpoch {
					everOnWorklistMarks[yi] = markEpoch
					worklist = append(worklist, y)
				}
			}
		}
	}

	return phiSites
}

// renameSSA performs dominator-tree preorder traversal to assign SSA version numbers.
func (b *Builder) renameSSA(assignedSyms map[basecfg.SymbolID]string, defPoints map[basecfg.SymbolID][]basecfg.Point, phiSites map[basecfg.Point]map[basecfg.SymbolID]bool, domInfo *analysis.DomInfo) {
	entry := b.Cfg.Entry()

	phiMap := make(map[basecfg.Point]map[basecfg.SymbolID]*PhiInfo, len(phiSites))
	phiOrdered := make(map[basecfg.Point][]basecfg.SymbolID, len(phiSites))
	for p, syms := range phiSites {
		ordered := make([]basecfg.SymbolID, 0, len(syms))
		phiMap[p] = make(map[basecfg.SymbolID]*PhiInfo, len(syms))
		for sym := range syms {
			ordered = append(ordered, sym)
			phiMap[p][sym] = &PhiInfo{
				Point:  p,
				Target: Version{Root: assignedSyms[sym], Symbol: sym},
			}
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
		phiOrdered[p] = ordered
	}

	var seededGlobals []basecfg.SymbolID

	if b.ScopeTracker != nil {
		seededGlobals = make([]basecfg.SymbolID, 0, len(assignedSyms))

		for sym := range assignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				seededGlobals = append(seededGlobals, sym)
			}
		}

		sort.Slice(seededGlobals, func(i, j int) bool { return seededGlobals[i] < seededGlobals[j] })
	}

	// Pre-allocate stacks with capacity based on defPoints + 1 for seeded globals
	stacks := make(map[basecfg.SymbolID][]Version, len(assignedSyms))

	for sym := range assignedSyms {
		// Estimate capacity: 1 for initial def + depth of typical dominator tree path
		capacity := len(defPoints[sym]) + 2

		if capacity > 8 {
			capacity = 8
		}

		stacks[sym] = make([]Version, 0, capacity)
	}

	newVersion := func(sym basecfg.SymbolID) Version {
		b.NextVersionID[sym]++

		return Version{Root: assignedSyms[sym], Symbol: sym, ID: b.NextVersionID[sym]}
	}

	// pushedEntry stores a symbol and its push count for the small array optimization
	type pushedEntry struct {
		sym   basecfg.SymbolID
		count int
	}

	// Cache visible assigned symbols by visibility map pointer to avoid per-point scans.
	visAssignedCache := make(map[uintptr][]basecfg.SymbolID)
	visibility := b.ScopeTracker.visibility
	globals := b.ScopeTracker.globals

	// Track parent's VisibleVersion for sharing when unchanged
	type renameState struct {
		parentVersions map[basecfg.SymbolID]Version
		parentVisPtr   uintptr // pointer to parent's visibility map for identity check
	}

	var rename func(p basecfg.Point, state renameState)
	rename = func(p basecfg.Point, state renameState) {
		// Use small fixed array for common case (most points have 0-4 pushes)
		var pushedArr [8]pushedEntry

		pushedLen := 0

		var pushedMap map[basecfg.SymbolID]int // only allocated if overflow

		addPush := func(sym basecfg.SymbolID) {
			// Check array first
			for i := range pushedLen {
				if pushedArr[i].sym == sym {
					pushedArr[i].count++

					return
				}
			}
			// Add to array if space
			if pushedLen < len(pushedArr) {
				pushedArr[pushedLen] = pushedEntry{sym: sym, count: 1}
				pushedLen++

				return
			}
			// Overflow to map
			if pushedMap == nil {
				pushedMap = make(map[basecfg.SymbolID]int, 16)

				for i := range pushedLen {
					pushedMap[pushedArr[i].sym] = pushedArr[i].count
				}
			}

			pushedMap[sym]++
		}

		if phiSyms := phiOrdered[p]; len(phiSyms) > 0 {
			phis := phiMap[p]
			for _, sym := range phiSyms {
				ver := newVersion(sym)
				phis[sym].Target = ver
				stacks[sym] = append(stacks[sym], ver)
				addPush(sym)
			}
		}

		if p == entry {
			for _, sym := range seededGlobals {
				ver := newVersion(sym)
				stacks[sym] = append(stacks[sym], ver)
				addPush(sym)
			}
		}

		if info := b.Info[p]; info != nil {
			switch v := info.(type) {
			case *AssignInfo:
				for i, target := range v.Targets {
					sym, _ := assignmentTargetSymbol(target)
					if sym == 0 {
						continue
					}
					if _, ok := assignedSyms[sym]; ok {
						ver := newVersion(sym)
						stacks[sym] = append(stacks[sym], ver)
						addPush(sym)

						for len(v.TargetVersions) <= i {
							v.TargetVersions = append(v.TargetVersions, Version{})
						}

						v.TargetVersions[i] = ver
					}
				}
			case *FuncDefInfo:
				if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
					if _, ok := assignedSyms[v.Symbol]; ok {
						ver := newVersion(v.Symbol)
						stacks[v.Symbol] = append(stacks[v.Symbol], ver)
						addPush(v.Symbol)
					}
				}
			}
		}

		var (
			currentVersions map[basecfg.SymbolID]Version
			currentVisPtr   uintptr
		)

		if visLocal := visibility[p]; visLocal != nil {
			currentVisPtr = mapPtr(visLocal)
			visibleAssigned, cached := visAssignedCache[currentVisPtr]

			if !cached {
				visibleAssigned = make([]basecfg.SymbolID, 0, len(assignedSyms))
				for name, resolvedSym := range visLocal {
					if expectedName, ok := assignedSyms[resolvedSym]; ok && expectedName == name {
						visibleAssigned = append(visibleAssigned, resolvedSym)
					}
				}
				for name, resolvedSym := range globals {
					if _, shadowed := visLocal[name]; shadowed {
						continue
					}
					if expectedName, ok := assignedSyms[resolvedSym]; ok && expectedName == name {
						visibleAssigned = append(visibleAssigned, resolvedSym)
					}
				}
				sort.Slice(visibleAssigned, func(i, j int) bool { return visibleAssigned[i] < visibleAssigned[j] })
				visAssignedCache[currentVisPtr] = visibleAssigned
			}

			noPushes := pushedLen == 0 && pushedMap == nil

			// Reuse parent's VisibleVersion if visibility unchanged and no pushes
			if noPushes && state.parentVersions != nil && currentVisPtr == state.parentVisPtr {
				b.VisibleVersion[p] = state.parentVersions
				currentVersions = state.parentVersions
			} else {
				for _, sym := range visibleAssigned {
					if stack := stacks[sym]; len(stack) > 0 {
						if currentVersions == nil {
							currentVersions = make(map[basecfg.SymbolID]Version, len(visibleAssigned))
							b.VisibleVersion[p] = currentVersions
						}

						currentVersions[sym] = stack[len(stack)-1]
					}
				}
			}
		}

		emitPhiOperands := func(succ basecfg.Point) {
			if succPhis := phiMap[succ]; succPhis != nil {
				for sym, phi := range succPhis {
					if stack := stacks[sym]; len(stack) > 0 {
						phi.Operands = append(phi.Operands, PhiOperand{
							From:    p,
							Version: stack[len(stack)-1],
						})
					}
				}
			}
		}

		if b.Cfg.IsBranch(p) {
			for _, succ := range b.Cfg.Successors(p) {
				emitPhiOperands(succ)
			}
		} else if succ := b.Cfg.Successor(p); succ != p {
			emitPhiOperands(succ)
		}

		childState := renameState{
			parentVersions: currentVersions,
			parentVisPtr:   currentVisPtr,
		}
		for _, child := range domInfo.DominatorTree[p] {
			rename(child, childState)
		}

		if pushedMap != nil {
			for sym, count := range pushedMap {
				stacks[sym] = stacks[sym][:len(stacks[sym])-count]
			}
		} else {
			for i := range pushedLen {
				sym := pushedArr[i].sym
				count := pushedArr[i].count
				stacks[sym] = stacks[sym][:len(stacks[sym])-count]
			}
		}
	}

	rename(entry, renameState{})

	for _, p := range b.Cfg.RPO() {
		if phiSyms := phiOrdered[p]; len(phiSyms) > 0 {
			phis := phiMap[p]
			for _, sym := range phiSyms {
				phi := phis[sym]
				if len(phi.Operands) > 0 {
					b.PhiNodes = append(b.PhiNodes, *phi)
				}
			}
		}
	}
}

func assignmentTargetSymbol(target AssignTarget) (basecfg.SymbolID, string) {
	switch target.Kind {
	case TargetIdent:
		if target.Symbol != 0 {
			return target.Symbol, target.Name
		}
	case TargetField, TargetIndex:
		if target.BaseSymbol != 0 {
			return target.BaseSymbol, target.BaseName
		}
	}
	return 0, ""
}
