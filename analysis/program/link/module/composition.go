package module

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// BuildCompositionRows constructs the Link-lifetime module composition from
// the sealed parent relation and the already-published mount directory. The
// result is construction-time data only: callers publish the returned rows
// into one Snapshot and retain no module builder, cache, or Program pointer.
//
// The parent relation supplies the source Shard, issued Import term, and
// root/cache identities. Publication joins that relation to the mounted
// Program's exact ModuleRequest and to the target root's ModuleKey. The
// caller supplies Module's already-sealed scalar context directory; no root,
// source key/literal catalog, or authored-name target index is reopened here.
func (c *Component) BuildCompositionRows(linkID identity.ContentID, mounts []programmount.MountedArtifact, contextDirectory executioncontext.Directory) (
	[]modulecomposition.ResolvedImport,
	[]modulecomposition.CacheIngress,
	[]modulecomposition.ModuleCallTransition,
	[]modulecomposition.InitGeneration,
	[]modulecomposition.InitOutcome,
	[]modulecomposition.InitTerminal,
	[]modulecomposition.ModuleExportCallableOrigin,
	bool,
) {
	var emptyImports []modulecomposition.ResolvedImport
	var emptyCache []modulecomposition.CacheIngress
	var emptyTransitions []modulecomposition.ModuleCallTransition
	var emptyGenerations []modulecomposition.InitGeneration
	var emptyOutcomes []modulecomposition.InitOutcome
	var emptyTerminals []modulecomposition.InitTerminal
	var emptyOrigins []modulecomposition.ModuleExportCallableOrigin
	if c == nil || c.authority == nil || c.authority.project == nil || !linkID.Available() || len(mounts) == 0 {
		return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
	}
	entries, entriesOK := c.compositionEntries()
	if !entriesOK {
		return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
	}
	if !contextDirectory.Available() || contextDirectory.LinkID() != linkID {
		return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
	}
	for _, mount := range mounts {
		if !mount.Available() {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
	}

	imports := make([]modulecomposition.ResolvedImport, 0, len(entries))
	caches := make([]modulecomposition.CacheIngress, 0, len(entries))
	transitions := make([]modulecomposition.ModuleCallTransition, 0, len(entries))
	generations := make([]modulecomposition.InitGeneration, 0, len(entries))
	outcomes := make([]modulecomposition.InitOutcome, 0)
	terminals := make([]modulecomposition.InitTerminal, 0)
	origins := make([]modulecomposition.ModuleExportCallableOrigin, 0)
	for _, entry := range entries {
		sourceModuleKey, sourceKeyOK := c.authority.project.ModuleKey(entry.sourceShard)
		targetRoot, targetRootOK := c.root(entry.toRootOrdinal)
		if !sourceKeyOK || !targetRootOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		targetModuleKey, targetKeyOK := c.authority.project.ModuleKey(targetRoot.shard)
		sourceMount, sourceMountOK := mountedProgram(mounts, sourceModuleKey)
		targetMount, targetMountOK := mountedProgram(mounts, targetModuleKey)
		if !targetKeyOK || !sourceMountOK || !targetMountOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		ordinal := keyspace.TermOrdinal(entry.importTerm)
		if ordinal == 0 {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		importIndex := int(ordinal - 1)
		programImport, importOK := sourceMount.Program.ModuleImportAt(importIndex)
		request, requestOK := sourceMount.Program.ModuleRequestFor(importIndex)
		if !importOK || !requestOK || !programImport.ID().Available() || request.ImportID() != programImport.ID() {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		resolved, resolvedOK := modulecomposition.NewResolvedImport(linkID, sourceMount.Program, request, targetModuleKey)
		if !resolvedOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		fromContext, fromContextOK := contextDirectory.ContextForRoot(entry.fromRootID)
		toContext, toContextOK := contextDirectory.ContextForRoot(entry.toRootID)
		if !fromContextOK || !toContextOK || fromContext.LinkID() != linkID || toContext.LinkID() != linkID ||
			fromContext.ActorID() != entry.actorID || fromContext.RepresentativeCacheInstanceID() != entry.representativeID {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		cacheRow, cacheOK := modulecomposition.NewCacheIngress(resolved, entry.fromRootID, entry.toRootID, fromContext, toContext)
		if !cacheOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		body, bodyOK := targetMount.Program.EntryBody()
		if !bodyOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		generation, generationOK := modulecomposition.NewInitGeneration(cacheRow, targetMount.Program, body)
		if !generationOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		transition, transitionOK := contextDirectory.Transition(fromContext.ID(), toContext.ID())
		if !transitionOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		callTransition, callTransitionOK := modulecomposition.NewModuleCallTransition(cacheRow, generation, sourceMount.Program, programImport, transition)
		if !callTransitionOK {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		imports = append(imports, resolved)
		caches = append(caches, cacheRow)
		transitions = append(transitions, callTransition)
		generations = append(generations, generation)
		if !appendInitOutcomes(targetMount.Program, body, generation, &outcomes, &terminals) {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
		if !appendCallableOrigins(targetMount.Program.Program, generation, callTransition, &origins) {
			return emptyImports, emptyCache, emptyTransitions, emptyGenerations, emptyOutcomes, emptyTerminals, emptyOrigins, false
		}
	}
	return imports, caches, transitions, generations, outcomes, terminals, origins, true
}

func (c *Component) root(ordinal uint32) (rootRow, bool) {
	if !live(c) || ordinal == 0 || uint64(ordinal) > uint64(len(c.authority.roots)) {
		return rootRow{}, false
	}
	return c.authority.roots[ordinal-1], true
}

func mountedProgram(mounts []programmount.MountedArtifact, moduleKey identity.ContentID) (programmount.MountedArtifact, bool) {
	if !moduleKey.Available() {
		return programmount.MountedArtifact{}, false
	}
	var found programmount.MountedArtifact
	for _, mount := range mounts {
		if mount.ModuleKey != moduleKey {
			continue
		}
		if found.Available() {
			return programmount.MountedArtifact{}, false
		}
		found = mount
	}
	return found, found.Available()
}

func appendInitOutcomes(mount programmount.Program, body programschema.Body, generation modulecomposition.InitGeneration, outcomes *[]modulecomposition.InitOutcome, terminals *[]modulecomposition.InitTerminal) bool {
	program := mount.Program
	bodyIndex, bodyOK := programBodyIndex(program, body)
	if !bodyOK {
		return false
	}
	for index := 0; index < body.OutcomeCount(); index++ {
		outcome, outcomeOK := program.BodyOutcomeFor(bodyIndex, index)
		if !outcomeOK || outcome.Kind() == programschema.OutcomeReturn {
			if !outcomeOK {
				return false
			}
			continue
		}
		row, rowOK := modulecomposition.NewInitOutcome(generation, mount, outcome)
		if !rowOK {
			return false
		}
		*outcomes = append(*outcomes, row)
		if outcome.Kind() == programschema.OutcomeThrow || outcome.Kind() == programschema.OutcomeCancel {
			terminal, terminalOK := modulecomposition.NewInitTerminal(row)
			if !terminalOK {
				return false
			}
			*terminals = append(*terminals, terminal)
		}
	}
	entries, entriesOK := program.ModuleEntryCount()
	if !entriesOK {
		return false
	}
	for index := 0; index < entries; index++ {
		entry, entryOK := program.ModuleEntryAt(index)
		if !entryOK {
			return false
		}
		row, rowOK := modulecomposition.NewInitOutcomeFromModuleEntry(generation, mount, entry)
		if !rowOK {
			return false
		}
		*outcomes = append(*outcomes, row)
	}
	return true
}

// appendCallableOrigins projects only authenticated exported function paths
// from one target Program.  It does not enumerate contexts: the caller has
// already supplied the exact ModuleCallTransition, and every emitted row
// carries that full transition tuple.
func appendCallableOrigins(program programschema.Program, generation modulecomposition.InitGeneration, transition modulecomposition.ModuleCallTransition, origins *[]modulecomposition.ModuleExportCallableOrigin) bool {
	if !program.Available() || !generation.Available() || !transition.Available() || origins == nil ||
		generation.LinkID() != transition.LinkID() || transition.GenerationID() != generation.ID() {
		return false
	}
	state, stateOK := program.ColdState()
	targets, targetsOK := calltarget.NewView(state)
	allocations, allocationsOK := heapallocation.NewView(state)
	if !stateOK || !targetsOK || !allocationsOK {
		return false
	}
	entryCount, entriesOK := program.ModuleEntryCount()
	if !entriesOK {
		return false
	}
	for entryIndex := 0; entryIndex < entryCount; entryIndex++ {
		entry, entryOK := program.ModuleEntryAt(entryIndex)
		if !entryOK || !entry.Available() {
			return false
		}
		outcome, outcomeOK := modulecomposition.NewInitOutcomeFromModuleEntry(generation, programmount.Program{ModuleKey: generation.ModuleKey(), Program: program}, entry)
		if !outcomeOK {
			return false
		}
		rootWidth, widthOK := entry.RootWidth()
		if !widthOK {
			return false
		}
		for position := uint32(0); position < rootWidth; position++ {
			root, rootOK := program.ModuleEntryRootFunctionFor(entryIndex, int(position))
			if !rootOK {
				continue
			}
			target, allocation, targetOK := targetAndAllocationForFunction(targets, allocations, root.FunctionID())
			if !targetOK {
				return false
			}
			origin, originOK := modulecomposition.NewModuleExportCallableOriginRoot(
				transition, generation, outcome, programmount.Program{ModuleKey: generation.ModuleKey(), Program: program}, entry, root, target, allocation,
			)
			if !originOK {
				return false
			}
			*origins = append(*origins, origin)
		}
		memberOffset, memberCount, memberOK := entry.MemberSpan()
		if !memberOK {
			return false
		}
		members := make([]programschema.ModuleEntryMember, memberCount)
		for index := uint32(0); index < memberCount; index++ {
			member, held := program.ModuleEntryMemberAt(int(memberOffset + index))
			if !held || !member.Available() {
				return false
			}
			members[index] = member
		}
		for index, terminal := range members {
			functionID, functionOK := terminal.ValueID()
			if !functionOK {
				continue
			}
			chain, chainOK := memberChainForTerminal(members, index)
			if !chainOK {
				return false
			}
			target, allocation, targetOK := targetAndAllocationForFunction(targets, allocations, functionID)
			if !targetOK {
				return false
			}
			origin, originOK := modulecomposition.NewModuleExportCallableOriginMember(
				transition, generation, outcome, programmount.Program{ModuleKey: generation.ModuleKey(), Program: program}, entry, chain, target, allocation,
			)
			if !originOK {
				return false
			}
			*origins = append(*origins, origin)
		}
	}
	return true
}

func targetAndAllocationForFunction(targets calltarget.View, allocations heapallocation.View, functionID identity.ContentID) (calltarget.Target, heapallocation.Allocation, bool) {
	if !targets.Available() || !allocations.Available() || !functionID.Available() {
		return calltarget.Target{}, heapallocation.Allocation{}, false
	}
	count, published := targets.Count()
	if !published {
		return calltarget.Target{}, heapallocation.Allocation{}, false
	}
	var target calltarget.Target
	for index := 0; index < count; index++ {
		candidate, held := targets.At(index)
		if !held || candidate.FunctionID() != functionID {
			continue
		}
		if target.Available() {
			return calltarget.Target{}, heapallocation.Allocation{}, false
		}
		target = candidate
	}
	if !target.Available() {
		return calltarget.Target{}, heapallocation.Allocation{}, false
	}
	allocation, allocationOK := allocations.AllocationForID(target.AllocationID())
	return target, allocation, allocationOK && allocation.Role() == heapallocation.RoleClosure && allocation.Form() == heapallocation.FormEmpty
}

func memberChainForTerminal(members []programschema.ModuleEntryMember, terminalIndex int) ([]programschema.ModuleEntryMember, bool) {
	if terminalIndex < 0 || terminalIndex >= len(members) {
		return nil, false
	}
	chain := make([]programschema.ModuleEntryMember, 0, terminalIndex+1)
	current := members[terminalIndex]
	for {
		chain = append(chain, current)
		if current.ParentID() == current.TableID() {
			break
		}
		parent := -1
		for index, candidate := range members {
			if candidate.FieldID() != current.ParentID() {
				continue
			}
			if parent >= 0 {
				return nil, false
			}
			parent = index
		}
		if parent < 0 || len(chain) > len(members) {
			return nil, false
		}
		current = members[parent]
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain, true
}

func programBodyIndex(program programschema.Program, body programschema.Body) (int, bool) {
	count, published := program.BodyCount()
	if !published || !body.Available() {
		return 0, false
	}
	index := -1
	for candidateIndex := 0; candidateIndex < count; candidateIndex++ {
		candidate, held := program.BodyAt(candidateIndex)
		if !held || !candidate.Available() {
			return 0, false
		}
		if candidate != body {
			continue
		}
		if index >= 0 {
			return 0, false
		}
		index = candidateIndex
	}
	return index, index >= 0
}
