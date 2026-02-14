package cfg

import (
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// mapPtr returns a uintptr that uniquely identifies a map instance.
func mapPtr(m any) uintptr {
	return reflect.ValueOf(m).Pointer()
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
		if vis := b.ScopeTracker.VisibleAt(entry); vis != nil {
			vis.Range(func(name string, sym basecfg.SymbolID) bool {
				if _, exists := syms[sym]; !exists {
					syms[sym] = name
				}

				return true
			})
		}
	}

	return syms
}

// collectDefPoints returns the set of CFG points where each symbol is defined.
func (b *Builder) collectDefPoints(assignedSyms map[basecfg.SymbolID]string) map[basecfg.SymbolID][]basecfg.Point {
	defPoints := make(map[basecfg.SymbolID][]basecfg.Point)
	entry := b.Cfg.Entry()

	for p, info := range b.Info {
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

	if b.ScopeTracker != nil {
		for sym := range assignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				defPoints[sym] = append(defPoints[sym], entry)
			}
		}
	}

	for sym := range defPoints {
		pts := defPoints[sym]
		sort.Slice(pts, func(i, j int) bool { return pts[i] < pts[j] })
	}

	return defPoints
}

// placePhis uses the iterated dominance frontier to determine where phi nodes are needed.
func (b *Builder) placePhis(assignedSyms map[basecfg.SymbolID]string, defPoints map[basecfg.SymbolID][]basecfg.Point, df map[basecfg.Point][]basecfg.Point) map[basecfg.Point]map[basecfg.SymbolID]bool {
	phiSites := make(map[basecfg.Point]map[basecfg.SymbolID]bool)

	sortedSyms := make([]basecfg.SymbolID, 0, len(assignedSyms))

	for sym := range assignedSyms {
		sortedSyms = append(sortedSyms, sym)
	}

	sort.Slice(sortedSyms, func(i, j int) bool { return sortedSyms[i] < sortedSyms[j] })

	for _, sym := range sortedSyms {
		name := assignedSyms[sym]
		defs := defPoints[sym]

		if len(defs) == 0 {
			continue
		}

		hasAlready := make(map[basecfg.Point]bool)
		everOnWorklist := make(map[basecfg.Point]bool)
		worklist := make([]basecfg.Point, len(defs))
		copy(worklist, defs)

		for _, d := range defs {
			everOnWorklist[d] = true
		}

		for len(worklist) > 0 {
			d := worklist[len(worklist)-1]
			worklist = worklist[:len(worklist)-1]

			for _, y := range df[d] {
				if hasAlready[y] {
					continue
				}

				vis := b.ScopeTracker.VisibleAt(y)

				if vis == nil {
					continue
				}

				resolvedSym, ok := vis.Get(name)
				if !ok || resolvedSym != sym {
					continue
				}

				hasAlready[y] = true

				if phiSites[y] == nil {
					phiSites[y] = make(map[basecfg.SymbolID]bool)
				}

				phiSites[y][sym] = true

				if !everOnWorklist[y] {
					everOnWorklist[y] = true
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

	phiMap := make(map[basecfg.Point]map[basecfg.SymbolID]*PhiInfo)
	for p, syms := range phiSites {
		phiMap[p] = make(map[basecfg.SymbolID]*PhiInfo)
		for sym := range syms {
			phiMap[p][sym] = &PhiInfo{
				Point:  p,
				Target: Version{Root: assignedSyms[sym], Symbol: sym},
			}
		}
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

		if phis := phiMap[p]; phis != nil {
			phiSyms := make([]basecfg.SymbolID, 0, len(phis))

			for sym := range phis {
				phiSyms = append(phiSyms, sym)
			}

			sort.Slice(phiSyms, func(i, j int) bool { return phiSyms[i] < phiSyms[j] })

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

		if vis := b.ScopeTracker.VisibleAt(p); vis != nil {
			visMap := vis.ToMap()
			currentVisPtr = mapPtr(visMap)
			visibleAssigned := visAssignedCache[currentVisPtr]

			if visibleAssigned == nil {
				visibleAssigned = make([]basecfg.SymbolID, 0, len(assignedSyms))

				for sym, name := range assignedSyms {
					if resolvedSym, ok := vis.Get(name); ok && resolvedSym == sym {
						visibleAssigned = append(visibleAssigned, sym)
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

		for _, succ := range b.Cfg.Successors(p) {
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
		if phis := phiMap[p]; phis != nil {
			phiSyms := make([]basecfg.SymbolID, 0, len(phis))

			for sym := range phis {
				phiSyms = append(phiSyms, sym)
			}

			sort.Slice(phiSyms, func(i, j int) bool { return phiSyms[i] < phiSyms[j] })

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
