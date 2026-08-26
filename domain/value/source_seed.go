package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// sourceSeedMount is Schema's sealed, Link-qualified directory for reusable
// Program ValueSource occurrence IDs. It is built while Schema still owns the
// source census, then consumed by the source rule without reopening Program.
type sourceSeedMount struct {
	module          identity.ContentID
	occurrences     []sourceSeedOccurrence
	occurrenceIndex map[identity.ContentID]uint32
}

type sourceSeedOccurrence struct {
	seed       SourceSeed
	occurrence identity.ContentID
}

// SourceSeedMount is an opaque owner-fenced projection of one exact mounted
// ValueSource directory. It exposes only the ModuleKey and preissued seed/
// occurrence pairs; no Program, Flow, raw Term, or Link Value is observable.
type SourceSeedMount struct {
	schema *Schema
	index  uint32
}

// SourceSeedOccurrence is one opaque mounted substitution from an artifact
// ValueSource occurrence ID to its exact Schema-owned SourceSeed.
type SourceSeedOccurrence struct {
	mount SourceSeedMount
	index uint32
}

func (schema *Schema) SourceSeedMountCount() int {
	if schema == nil || schema.sourceSeedMounts == nil || schema.sourceSeedMountIndex == nil {
		return 0
	}
	return len(schema.sourceSeedMounts)
}

func (schema *Schema) SourceSeedMountAt(index int) (SourceSeedMount, bool) {
	if schema == nil || index < 0 || index >= len(schema.sourceSeedMounts) {
		return SourceSeedMount{}, false
	}
	mount := SourceSeedMount{schema: schema, index: uint32(index + 1)}
	return mount, mount.valid()
}

// SourceSeedMountForModule returns the owner-issued mounted source directory
// for one exact ModuleKey. The lookup is schema-owned and O(1); callers do
// not reconstruct a module-to-occurrence catalog in a hot Rule.
func (schema *Schema) SourceSeedMountForModule(module identity.ContentID) (SourceSeedMount, bool) {
	if schema == nil || !module.Available() || schema.sourceSeedMountIndex == nil {
		return SourceSeedMount{}, false
	}
	index, ok := schema.sourceSeedMountIndex[module]
	if !ok || index == 0 || int(index) > len(schema.sourceSeedMounts) {
		return SourceSeedMount{}, false
	}
	mount := SourceSeedMount{schema: schema, index: index}
	return mount, mount.valid()
}

func (mount SourceSeedMount) valid() bool {
	if mount.schema == nil || mount.index == 0 || int(mount.index) > len(mount.schema.sourceSeedMounts) || mount.schema.sourceSeedMountIndex == nil {
		return false
	}
	row := mount.schema.sourceSeedMounts[mount.index-1]
	index, ok := mount.schema.sourceSeedMountIndex[row.module]
	return ok && index == mount.index && row.module.Available() && row.occurrenceIndex != nil
}

func (mount SourceSeedMount) ModuleID() identity.ContentID {
	if !mount.valid() {
		return identity.ContentID{}
	}
	return mount.schema.sourceSeedMounts[mount.index-1].module
}

func (mount SourceSeedMount) OccurrenceCount() int {
	if !mount.valid() {
		return 0
	}
	return len(mount.schema.sourceSeedMounts[mount.index-1].occurrences)
}

func (mount SourceSeedMount) OccurrenceAt(index int) (SourceSeedOccurrence, bool) {
	if !mount.valid() || index < 0 || index >= len(mount.schema.sourceSeedMounts[mount.index-1].occurrences) {
		return SourceSeedOccurrence{}, false
	}
	occurrence := SourceSeedOccurrence{mount: mount, index: uint32(index + 1)}
	return occurrence, occurrence.valid()
}

// OccurrenceForID returns one exact mounted occurrence through the sealed
// owner index. It is O(1) in both the ModuleKey and artifact occurrence ID.
func (mount SourceSeedMount) OccurrenceForID(id identity.ContentID) (SourceSeedOccurrence, bool) {
	if !mount.valid() || !id.Available() {
		return SourceSeedOccurrence{}, false
	}
	rows := mount.schema.sourceSeedMounts[mount.index-1]
	index, ok := rows.occurrenceIndex[id]
	if !ok || index == 0 || int(index) > len(rows.occurrences) {
		return SourceSeedOccurrence{}, false
	}
	occurrence := SourceSeedOccurrence{mount: mount, index: index}
	return occurrence, occurrence.valid()
}

func (occurrence SourceSeedOccurrence) valid() bool {
	if !occurrence.mount.valid() || occurrence.index == 0 {
		return false
	}
	rows := occurrence.mount.schema.sourceSeedMounts[occurrence.mount.index-1].occurrences
	if int(occurrence.index) > len(rows) {
		return false
	}
	row := rows[occurrence.index-1]
	index, indexed := occurrence.mount.schema.sourceSeedMounts[occurrence.mount.index-1].occurrenceIndex[row.occurrence]
	return indexed && index == occurrence.index && row.occurrence.Available() && row.seed.valid() && row.seed.occurrence == row.occurrence && row.seed.module == occurrence.mount.ModuleID()
}

func (occurrence SourceSeedOccurrence) ID() identity.ContentID {
	if !occurrence.valid() {
		return identity.ContentID{}
	}
	return occurrence.mount.schema.sourceSeedMounts[occurrence.mount.index-1].occurrences[occurrence.index-1].occurrence
}

func (occurrence SourceSeedOccurrence) Seed() (SourceSeed, bool) {
	if !occurrence.valid() {
		return SourceSeed{}, false
	}
	seed := occurrence.mount.schema.sourceSeedMounts[occurrence.mount.index-1].occurrences[occurrence.index-1].seed
	return seed, seed.valid()
}

// SourceSeedForMountedOccurrence is the narrow direct lookup for callers that
// need only the owner-issued source operand. It retains the same exact module
// and occurrence fences as SourceSeedMount/Occurrence handles.
func (schema *Schema) SourceSeedForMountedOccurrence(module, occurrence identity.ContentID) (SourceSeed, bool) {
	mount, mountOK := schema.SourceSeedMountForModule(module)
	row, rowOK := mount.OccurrenceForID(occurrence)
	seed, seedOK := row.Seed()
	return seed, mountOK && rowOK && seedOK
}

// SourceSeedCount is the dense, mount-major census of owner-issued source seeds.
func (schema *Schema) SourceSeedCount() int {
	if schema == nil {
		return 0
	}
	count := 0
	for index := range schema.sourceSeedMounts {
		count += len(schema.sourceSeedMounts[index].occurrences)
	}
	return count
}

// SourceSeedAt returns one dense source seed. Order is sealed mount order,
// then occurrence order inside each mount.
func (schema *Schema) SourceSeedAt(index int) (SourceSeed, bool) {
	if schema == nil || index < 0 {
		return SourceSeed{}, false
	}
	remaining := index
	for mountIndex := range schema.sourceSeedMounts {
		rows := schema.sourceSeedMounts[mountIndex].occurrences
		if remaining < len(rows) {
			seed := rows[remaining].seed
			return seed, seed.valid()
		}
		remaining -= len(rows)
	}
	return SourceSeed{}, false
}

// SourceSeedOrdinal is the exact inverse of SourceSeedAt over this Schema.
func (schema *Schema) SourceSeedOrdinal(seed SourceSeed) (uint32, bool) {
	if schema == nil || !seed.valid() || seed.schema != schema {
		return 0, false
	}
	ordinal := 0
	for mountIndex := range schema.sourceSeedMounts {
		rows := schema.sourceSeedMounts[mountIndex].occurrences
		for index, row := range rows {
			if row.seed.valueID == seed.valueID && row.occurrence == seed.occurrence && row.seed.module == seed.module {
				return uint32(ordinal + index), true
			}
		}
		ordinal += len(rows)
	}
	return 0, false
}

// sealSourceSeedOccurrences resolves Program-owned ValueSource occurrence
// identities exactly once while Value seals its source census. The retained
// directory deliberately contains neither Program/Flow handles nor raw Terms.
func (schema *valueBuilder) sealSourceSeedOccurrences() bool {
	if schema == nil || schema.sealProject() == nil || schema.sourceSeedMounts != nil || schema.artifacts == nil {
		return false
	}
	project := schema.sealProject()
	if project == nil {
		return false
	}
	mounts := project.Mounts()
	schema.sourceSeedMounts = make([]sourceSeedMount, mounts.Count())
	schema.sourceSeedMountIndex = make(map[identity.ContentID]uint32, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		module, moduleOK := project.ModuleKey(shard)
		if !shardOK || !moduleOK || !module.Available() || schema.sourceSeedMountIndex[module] != 0 || !schema.artifacts[module].Available() {
			return false
		}
		schema.sourceSeedMountIndex[module] = uint32(index + 1)
		schema.sourceSeedMounts[index].module = module
	}
	// ProgramArtifact owns the reusable ValueSource occurrence denominator.
	// Boundary owns the concrete mounted Value inverse; joining by ModuleKey
	// and occurrence ID keeps duplicate mounts distinct without terms or
	// positional zips.
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		module := schema.sourceSeedMounts[mountIndex].module
		mount := schema.artifacts[module]
		if !mount.Available() {
			return false
		}
		rows := &schema.sourceSeedMounts[mountIndex]
		rows.occurrenceIndex = make(map[identity.ContentID]uint32)
		program := mount.Program.Program
		count, countOK := program.OccurrenceCount()
		if !countOK {
			return false
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.OccurrenceAt(index)
			if !rowOK || row.Kind() != programschema.OccurrenceValueSource &&
				row.Kind() != programschema.OccurrenceFormalEntry &&
				row.Kind() != programschema.OccurrenceGlobalEntry {
				continue
			}
			id := row.ID()
			if _, duplicate := rows.occurrenceIndex[id]; duplicate {
				return false
			}
			value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, id)
			valueID, valueIDOK := schema.sealBoundary().Values().ID(value)
			seed, seedOK := schema.SourceSeedForValueID(valueID)
			if !seedOK && row.Kind() == programschema.OccurrenceGlobalEntry {
				// Program offers an admission for every unwritten global cell a
				// body reads; Value declines the ones whose binding names no
				// value it can carry - denied and absent bindings among them.
				// A declined admission contributes no fact, which is the same
				// answer as not being offered one.
				delete(rows.occurrenceIndex, id)
				continue
			}
			if !valueOK || !valueIDOK || !seedOK || !id.Available() || seed.occurrence.Available() || seed.module.Available() {
				return false
			}
			seed.module, seed.occurrence = module, id
			rows.occurrences = append(rows.occurrences, sourceSeedOccurrence{seed: seed, occurrence: id})
			rows.occurrenceIndex[id] = uint32(len(rows.occurrences))
		}
	}
	if len(schema.sourceSeedMountIndex) != len(schema.sourceSeedMounts) {
		return false
	}
	for index := range schema.sourceSeedMounts {
		rows := schema.sourceSeedMounts[index]
		if !rows.module.Available() || rows.occurrenceIndex == nil {
			return false
		}
		for _, row := range rows.occurrences {
			if !row.seed.valid() || !row.occurrence.Available() || row.seed.module != rows.module || row.seed.occurrence != row.occurrence {
				return false
			}
		}
	}
	return true
}
