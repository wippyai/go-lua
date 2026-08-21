package module

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// BuildCompositionRows constructs the Link-lifetime module composition from
// the sealed parent relation and the already-published mount directory. The
// result is construction-time data only: callers publish the returned rows
// into one Snapshot and retain no module builder, cache, or Program pointer.
//
// The parent relation supplies the source Shard, issued Import term, and
// root/cache identities. Publication joins that relation to the mounted
// Program's exact ModuleRequest and to the target root's ModuleKey. No source
// key/literal catalog or authored-name target index is reopened here.
func (c *Component) BuildCompositionRows(linkID identity.ContentID, mounts []programmount.MountedArtifact) (
	[]modulecomposition.ResolvedImport,
	[]modulecomposition.CacheIngress,
	[]modulecomposition.InitGeneration,
	[]modulecomposition.InitOutcome,
	[]modulecomposition.InitTerminal,
	bool,
) {
	var emptyImports []modulecomposition.ResolvedImport
	var emptyCache []modulecomposition.CacheIngress
	var emptyGenerations []modulecomposition.InitGeneration
	var emptyOutcomes []modulecomposition.InitOutcome
	var emptyTerminals []modulecomposition.InitTerminal
	if c == nil || c.authority == nil || c.authority.project == nil || !linkID.Available() || len(mounts) == 0 {
		return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
	}
	entries, entriesOK := c.compositionEntries()
	if !entriesOK {
		return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
	}
	for _, mount := range mounts {
		if !mount.Available() {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
	}

	imports := make([]modulecomposition.ResolvedImport, 0, len(entries))
	caches := make([]modulecomposition.CacheIngress, 0, len(entries))
	generations := make([]modulecomposition.InitGeneration, 0, len(entries))
	outcomes := make([]modulecomposition.InitOutcome, 0)
	terminals := make([]modulecomposition.InitTerminal, 0)
	for _, entry := range entries {
		sourceModuleKey, sourceKeyOK := c.authority.project.ModuleKey(entry.sourceShard)
		targetRoot, targetRootOK := c.root(entry.toRootOrdinal)
		if !sourceKeyOK || !targetRootOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		targetModuleKey, targetKeyOK := c.authority.project.ModuleKey(targetRoot.shard)
		sourceMount, sourceMountOK := mountedProgram(mounts, sourceModuleKey)
		targetMount, targetMountOK := mountedProgram(mounts, targetModuleKey)
		if !targetKeyOK || !sourceMountOK || !targetMountOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		ordinal := keyspace.TermOrdinal(entry.importTerm)
		if ordinal == 0 {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		importIndex := int(ordinal - 1)
		programImport, importOK := sourceMount.Program.ModuleImportAt(importIndex)
		request, requestOK := sourceMount.Program.ModuleRequestFor(importIndex)
		if !importOK || !requestOK || !programImport.ID().Available() || request.ImportID() != programImport.ID() {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		resolved, resolvedOK := modulecomposition.NewResolvedImport(linkID, sourceMount.Program, request, targetModuleKey)
		if !resolvedOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		cacheRow, cacheOK := modulecomposition.NewCacheIngress(resolved, entry.fromRootID, entry.toRootID, entry.actorID, entry.representativeID)
		if !cacheOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		body, bodyOK := targetMount.Program.EntryBody()
		if !bodyOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		generation, generationOK := modulecomposition.NewInitGeneration(cacheRow, targetMount.Program, body)
		if !generationOK {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
		imports = append(imports, resolved)
		caches = append(caches, cacheRow)
		generations = append(generations, generation)
		if !appendInitOutcomes(targetMount.Program, body, generation, &outcomes, &terminals) {
			return emptyImports, emptyCache, emptyGenerations, emptyOutcomes, emptyTerminals, false
		}
	}
	return imports, caches, generations, outcomes, terminals, true
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
