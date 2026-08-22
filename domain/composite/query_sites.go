package composite

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

const querySiteFormula = "analysis/artifact-query/v2"

// QuerySite is one selected-point query identity derived from sealed ingress
// bodies. Result reads Snapshot by the publication key attach captures for
// this site.
type QuerySite struct {
	ID              identity.ContentID
	Mount           identity.ContentID
	Context         executioncontext.Context
	Point           identity.ContentID
	Family          schema.Key
	Authority       schema.Key
	RegistrationID  schema.EntryID
	Projection      schema.Key
	selectedOrdinal uint32
}

// SelectedQueryTable is the sealed selected-point site table. The rows and
// cardinality are owned here rather than carried by a per-site pointer or a
// wrapper around a caller-owned slice. Callers can inspect bounded copies with
// Count and At, while admissions and publications consume the complete table.
type SelectedQueryTable struct {
	compilation identity.ContentID
	digest      identity.ContentID
	rows        []QuerySite
	total       int
}

// QueryPublication is the Result-facing address of one attached query site.
type QueryPublication struct {
	Site QuerySite
	Key  identity.ContentID

	// The encoder is a transient hot-side capability. It converts one borrowed
	// typed Answer into an owned engine cell and is never retained by Result.
	contract *engine.CanonicalResultContract
	encode   func(engine.Answer) (bool, uint64, []byte, bool)
	ordinal  uint32
}

// CanonicalCell invokes this sealed family's owner encoder exactly once and
// closes the typed Answer into an immutable engine-owned cell.
func (publication QueryPublication) CanonicalCell(answer engine.Answer) (engine.CanonicalResultCell, bool) {
	if !publication.Key.Available() || publication.contract == nil || !publication.contract.Available() || publication.encode == nil || !answer.Available() {
		return engine.CanonicalResultCell{}, false
	}
	present, rows, payload, encoded := publication.encode(answer)
	if !encoded {
		return engine.CanonicalResultCell{}, false
	}
	return engine.NewCanonicalResultCell(*publication.contract, present, rows, payload)
}

func (publication QueryPublication) FamilyID() identity.ContentID {
	if publication.contract == nil {
		return identity.ContentID{}
	}
	return publication.contract.FamilyID()
}

func (publication QueryPublication) Codec() identity.SemanticKey {
	if publication.contract == nil {
		return identity.SemanticKey{}
	}
	return publication.contract.Codec()
}

func (publication QueryPublication) Contract() engine.CanonicalResultContract {
	if publication.contract == nil {
		return engine.CanonicalResultContract{}
	}
	return *publication.contract
}

func (publication QueryPublication) FamilyOrdinal() uint32 { return publication.ordinal }

func querySiteID(mount, contextID, pointID identity.ContentID, family schema.Key, registrationID schema.EntryID) (identity.ContentID, bool) {
	if !mount.Available() || !contextID.Available() || !pointID.Available() || !family.Available() || !registrationID.Available() {
		return identity.ContentID{}, false
	}
	// RegistrationID is a separate sealed witness. The semantic location
	// remains stable across registration-contract changes; the contract itself
	// is fenced by RegistrationID at admission/publication boundaries.
	return identity.DeriveContentID(querySiteFormula, mount[:], contextID[:], pointID[:], []byte(family))
}

// Available is the zero-value fence for a selected query table.
func (table SelectedQueryTable) Available() bool {
	return table.total > 0 && table.total == len(table.rows) && table.compilation.Available() && table.digest.Available()
}

// Count returns the sealed row cardinality.
func (table SelectedQueryTable) Count() int {
	if !table.Available() {
		return 0
	}
	return table.total
}

// At returns one bounded copy of a sealed row.
func (table SelectedQueryTable) At(index int) (QuerySite, bool) {
	if !table.Available() || index < 0 || index >= table.total {
		return QuerySite{}, false
	}
	return table.rows[index], true
}

func querySiteTableSeal(rows []QuerySite) (identity.ContentID, bool) {
	if len(rows) == 0 {
		return identity.ContentID{}, false
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(rows)))
	parts := make([][]byte, 0, len(rows)+1)
	parts = append(parts, count[:])
	for index, site := range rows {
		if !site.ID.Available() || !site.Mount.Available() || !site.Point.Available() || !site.Context.Available() || site.Context.ModuleKey() != site.Mount || !site.Family.Available() || !site.Authority.Available() || !site.RegistrationID.Available() || !site.Projection.Available() || site.selectedOrdinal == 0 {
			return identity.ContentID{}, false
		}
		contextID := site.Context.ID()
		var ordinal [8]byte
		var position [8]byte
		binary.BigEndian.PutUint64(ordinal[:], uint64(site.selectedOrdinal))
		binary.BigEndian.PutUint64(position[:], uint64(index))
		row, rowOK := identity.DeriveContentID("analysis/artifact-query/row/v1", site.ID[:], site.Mount[:], contextID[:], site.Point[:], []byte(site.Family), []byte(site.Authority), site.RegistrationID[:], []byte(site.Projection), ordinal[:], position[:])
		if !rowOK {
			return identity.ContentID{}, false
		}
		parts = append(parts, row[:])
	}
	return identity.DeriveContentID("analysis/artifact-query/table/v1", parts...)
}

func sealSelectedQueryTable(compilation *catalog, sites []QuerySite, families []IssuedQuery) (SelectedQueryTable, bool) {
	if compilation == nil || !compilation.digest.Available() || len(sites) == 0 || len(families) == 0 {
		return SelectedQueryTable{}, false
	}
	rows := append([]QuerySite(nil), sites...)
	for familyIndex, family := range families {
		if !family.Family.Available() || !family.Authority.Available() || !family.RegistrationID.Available() || !family.Projection.Available() || family.SelectedOrdinal == 0 || family.SelectedOrdinal != uint32(familyIndex+1) {
			return SelectedQueryTable{}, false
		}
	}
	for index := range rows {
		site := &rows[index]
		if !site.ID.Available() || !site.Mount.Available() || !site.Point.Available() || !site.Context.Available() || site.Context.ModuleKey() != site.Mount || !site.Family.Available() || !site.Authority.Available() || !site.RegistrationID.Available() || !site.Projection.Available() {
			return SelectedQueryTable{}, false
		}
		familyFound := false
		for _, family := range families {
			if family.Family != site.Family {
				continue
			}
			if family.Authority != site.Authority || family.RegistrationID != site.RegistrationID || family.Projection != site.Projection {
				return SelectedQueryTable{}, false
			}
			site.selectedOrdinal = family.SelectedOrdinal
			familyFound = true
			break
		}
		if !familyFound {
			return SelectedQueryTable{}, false
		}
		expectedID, expectedOK := querySiteID(site.Mount, site.Context.ID(), site.Point, site.Family, site.RegistrationID)
		if !expectedOK || expectedID != site.ID {
			return SelectedQueryTable{}, false
		}
	}
	digest, digestOK := querySiteTableSeal(rows)
	if !digestOK {
		return SelectedQueryTable{}, false
	}
	return SelectedQueryTable{compilation: compilation.digest, digest: digest, rows: rows, total: len(rows)}, true
}

func validateSelectedQueryTable(bound *ProgramBinding, table SelectedQueryTable) bool {
	if bound == nil || bound.catalog == nil || !table.Available() || table.compilation != bound.compilation.Digest() || len(table.rows) != table.total {
		return false
	}
	digest, digestOK := querySiteTableSeal(table.rows)
	if !digestOK || digest != table.digest {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(table.rows))
	for _, site := range table.rows {
		if _, duplicate := seen[site.ID]; duplicate {
			return false
		}
		seen[site.ID] = struct{}{}
		registration, _, registered := queryRegistrationForFamily(bound.catalog, site.Family)
		if !registered || registration == nil || registration.EntryID() != site.RegistrationID || registration.Population() != query.PopulationSelectedPoint || registration.Projection() != site.Projection {
			return false
		}
		ordinal, ordinalOK := QuerySelectedRegistrationOrdinal(bound.compilation, site.Family)
		if !ordinalOK || ordinal != site.selectedOrdinal {
			return false
		}
		expectedID, expectedOK := querySiteID(site.Mount, site.Context.ID(), site.Point, site.Family, site.RegistrationID)
		if !expectedOK || expectedID != site.ID {
			return false
		}
	}
	return len(seen) == table.total && table.total > 0
}

// SelectedQuerySites derives mount- and context-qualified query identities
// from semantic occurrences in selected sealed bodies. Non-callable roots are
// always selected. A callable body is selected only when a sealed
// DirectFunctions join from an already-selected body names it. Contexts are
// expanded here, at the owner boundary; the engine receives one singular
// context on every admitted query row.
func SelectedQuerySites(compilation Compilation, mounts []programmount.MountedArtifact, contexts executioncontext.Directory) (SelectedQueryTable, bool) {
	state := compilation.catalog
	families, familiesOK := selectedPointQueryIssuance(state)
	if !familiesOK || len(mounts) == 0 || !contexts.Available() {
		return SelectedQueryTable{}, false
	}
	sites := make([]QuerySite, 0)
	expected := 0
	for _, mount := range mounts {
		if !mount.Available() {
			return SelectedQueryTable{}, false
		}
		eligibleContexts := make([]executioncontext.Context, 0, contexts.ContextCount())
		for contextIndex := 0; contextIndex < contexts.ContextCount(); contextIndex++ {
			context, contextOK := contexts.ContextAt(contextIndex)
			if !contextOK || !context.Available() {
				return SelectedQueryTable{}, false
			}
			if context.ModuleKey() == mount.ModuleKey {
				eligibleContexts = append(eligibleContexts, context)
			}
		}
		// A mount with no exact context is not an admitted query lane. In
		// particular, never synthesize a default context or borrow one from a
		// different module.
		if len(eligibleContexts) == 0 {
			return SelectedQueryTable{}, false
		}
		program := mount.Program
		bodyEntries := make(map[identity.ContentID][]identity.ContentID)
		callable := make(map[identity.ContentID]struct{})
		rootBodies := make(map[identity.ContentID][]identity.ContentID)
		bodyCount, bodiesPublished := mount.Program.BodyCount()
		if !bodiesPublished {
			return SelectedQueryTable{}, false
		}
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, bodyOK := mount.Program.BodyAt(bodyIndex)
			if !bodyOK || !body.ID().Available() {
				return SelectedQueryTable{}, false
			}
			entries := make([]identity.ContentID, body.EntryCount())
			for entryIndex := range entries {
				entryRow, entryOK := mount.Program.BodyEntryFor(bodyIndex, entryIndex)
				entry := entryRow.PointID()
				if !entryOK || !entry.Available() {
					return SelectedQueryTable{}, false
				}
				entries[entryIndex] = entry
			}
			if len(entries) == 0 {
				return SelectedQueryTable{}, false
			}
			bodyEntries[body.ID()] = entries
			if body.Callable() {
				callable[body.ID()] = struct{}{}
				continue
			}
			rootBodies[body.ID()] = entries
		}
		if len(rootBodies) == 0 {
			return SelectedQueryTable{}, false
		}
		selectedBodies := make(map[identity.ContentID][]identity.ContentID, len(rootBodies))
		for body, entries := range rootBodies {
			selectedBodies[body] = entries
		}
		for changed := true; changed; {
			changed = false
			callCount, callsPublished := program.CallCount()
			if !callsPublished {
				return SelectedQueryTable{}, false
			}
			for callIndex := 0; callIndex < callCount; callIndex++ {
				call, callOK := program.CallAt(callIndex)
				target, targetOK := call.DirectTargetBody()
				if !callOK || !targetOK {
					continue
				}
				if _, ownerSelected := selectedBodies[call.BodyID()]; !ownerSelected {
					continue
				}
				if _, already := selectedBodies[target]; already {
					continue
				}
				entries, known := bodyEntries[target]
				if _, isCallable := callable[target]; !known || !isCallable {
					return SelectedQueryTable{}, false
				}
				selectedBodies[target] = entries
				changed = true
			}
		}
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		pointCount, pointsPublished := programschema.PointFamily().Count(&program.Frozen, catalog)
		if !program.Available() || !catalogOK || !pointsPublished {
			return SelectedQueryTable{}, false
		}
		pointIDs := make(map[identity.ContentID]struct{}, pointCount)
		for index := 0; index < pointCount; index++ {
			point, ok := programschema.PointFamily().At(&program.Frozen, catalog, index)
			if !ok || !point.ID().Available() {
				return SelectedQueryTable{}, false
			}
			pointIDs[point.ID()] = struct{}{}
		}
		observed := make(map[identity.ContentID]struct{})
		observedBodies := make(map[identity.ContentID]struct{}, len(selectedBodies))
		occurrenceCount, occurrencesPublished := program.OccurrenceCount()
		if !occurrencesPublished {
			return SelectedQueryTable{}, false
		}
		for occurrenceIndex := 0; occurrenceIndex < occurrenceCount; occurrenceIndex++ {
			occurrence, occurrenceOK := program.OccurrenceAt(occurrenceIndex)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK {
				return SelectedQueryTable{}, false
			}
			if !bodyOK {
				continue
			}
			if _, selected := selectedBodies[body]; !selected {
				continue
			}
			_, pointCount, spanOK := occurrence.PointSpan()
			if !spanOK {
				return SelectedQueryTable{}, false
			}
			for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
				pointRow, pointOK := program.OccurrencePointFor(occurrenceIndex, pointIndex)
				point := pointRow.PointID()
				if !pointOK || !pointRow.Available() || !point.Available() {
					return SelectedQueryTable{}, false
				}
				if _, known := pointIDs[point]; !known {
					continue
				}
				observed[point] = struct{}{}
				observedBodies[body] = struct{}{}
			}
		}
		// Synthetic execution cuts (call stages, local successors, and other
		// declaration-framed points) do not belong to an occurrence's authored
		// PointSpan. They do belong to the same canonical WTO region. Close the
		// selected occurrence points over Region membership so query families
		// observe every executable cut in a selected body without naming any
		// particular staging form here.
		regionCount, regionsPublished := programschema.RegionFamily().Count(&program.Frozen, catalog)
		if !regionsPublished {
			return SelectedQueryTable{}, false
		}
		regions := make([][]identity.ContentID, regionCount)
		regionsByPoint := make(map[identity.ContentID][]int, pointCount)
		for regionIndex := 0; regionIndex < regionCount; regionIndex++ {
			region, regionOK := programschema.RegionFamily().At(&program.Frozen, catalog, regionIndex)
			offset, count, spanOK := region.MemberSpan()
			if !regionOK || !spanOK || count == 0 {
				return SelectedQueryTable{}, false
			}
			members := make([]identity.ContentID, count)
			for memberIndex := uint32(0); memberIndex < count; memberIndex++ {
				member, memberOK := programschema.RegionMemberFamily().At(&program.Frozen, catalog, int(offset+memberIndex))
				point := member.ID()
				if !memberOK || !point.Available() {
					return SelectedQueryTable{}, false
				}
				if _, known := pointIDs[point]; !known {
					return SelectedQueryTable{}, false
				}
				members[memberIndex] = point
				regionsByPoint[point] = append(regionsByPoint[point], regionIndex)
			}
			regions[regionIndex] = members
		}
		queue := make([]identity.ContentID, 0, len(observed))
		for point := range observed {
			queue = append(queue, point)
		}
		activatedRegions := make([]bool, len(regions))
		for cursor := 0; cursor < len(queue); cursor++ {
			for _, regionIndex := range regionsByPoint[queue[cursor]] {
				if activatedRegions[regionIndex] {
					continue
				}
				activatedRegions[regionIndex] = true
				for _, point := range regions[regionIndex] {
					if _, present := observed[point]; present {
						continue
					}
					observed[point] = struct{}{}
					queue = append(queue, point)
				}
			}
		}
		for body, entries := range selectedBodies {
			if _, present := observedBodies[body]; present {
				continue
			}
			for _, entry := range entries {
				observed[entry] = struct{}{}
			}
		}
		for _, context := range eligibleContexts {
			contextID := context.ID()
			if !contextID.Available() {
				return SelectedQueryTable{}, false
			}
			for index := 0; index < pointCount; index++ {
				point, ok := programschema.PointFamily().At(&program.Frozen, catalog, index)
				if !ok || !point.ID().Available() {
					return SelectedQueryTable{}, false
				}
				pointID := point.ID()
				if _, selected := observed[pointID]; !selected {
					continue
				}
				expected++
				for _, family := range families {
					id, idOK := querySiteID(mount.ModuleKey, contextID, pointID, family.Family, family.RegistrationID)
					if !idOK {
						return SelectedQueryTable{}, false
					}
					sites = append(sites, QuerySite{
						ID: id, Mount: mount.ModuleKey, Context: context, Point: pointID,
						Family: family.Family, Authority: family.Authority,
						RegistrationID: family.RegistrationID, Projection: family.Projection,
					})
				}
			}
		}
	}
	if expected == 0 || len(sites) != len(families)*expected {
		return SelectedQueryTable{}, false
	}
	table, tableOK := sealSelectedQueryTable(state, sites, families)
	if !tableOK {
		return SelectedQueryTable{}, false
	}
	return table, true
}

func selectedPointQueryIssuance(state *catalog) ([]IssuedQuery, bool) {
	issued := queryIssuance(state)
	if len(issued) == 0 {
		return nil, false
	}
	selected := make([]IssuedQuery, 0, len(issued))
	for _, family := range issued {
		if family.Population != query.PopulationSelectedPoint {
			continue
		}
		if !family.Family.Available() || !family.Authority.Available() || !family.RegistrationID.Available() || !family.Projection.Available() {
			return nil, false
		}
		if family.Projection != query.ProjectionSummary && family.Projection != query.ProjectionExact {
			return nil, false
		}
		if family.SelectedOrdinal != uint32(len(selected)+1) {
			return nil, false
		}
		selected = append(selected, family)
	}
	return selected, len(selected) > 0
}

// QueryAdmissions seals one engine admission row per selected-point site in
// the complete canonical table.
func (bound *ProgramBinding) QueryAdmissions(table SelectedQueryTable) ([]engine.ProgramQueryAdmission, bool) {
	if bound == nil || !validateSelectedQueryTable(bound, table) {
		return nil, false
	}
	rows := make([]engine.ProgramQueryAdmission, 0, table.total)
	for _, site := range table.rows {
		admitted, ok := bound.QueryAdmission(site.ID, site.Mount, site.Point, site.Family, site.Context)
		if !ok {
			return nil, false
		}
		rows = append(rows, admitted)
	}
	return rows, true
}

// QueryPublications reads the snapshot address every selected-point site
// publishes under from the committed program. The rows are the program's own:
// this pass names sites and never mints a publication of its own.
func (bound *ProgramBinding) QueryPublications(committed *engine.CommittedProgram, table SelectedQueryTable) ([]QueryPublication, bool) {
	if bound == nil || committed == nil || !validateSelectedQueryTable(bound, table) {
		return nil, false
	}
	publications := make([]QueryPublication, 0, table.total)
	for _, site := range table.rows {
		position, positioned := queryPositionForFamily(bound.catalog, site.Family)
		if !positioned || position < 0 || bound.catalog == nil || position >= len(bound.catalog.queries) || position >= len(bound.catalog.queryContributors) {
			return nil, false
		}
		registration, _, registered := queryRegistrationForFamily(bound.catalog, site.Family)
		contributor := bound.catalog.queryContributors[position]
		if !registered || registration == nil || registration != bound.catalog.queries[position] || !contributor.complete() || registration.Key() != site.Family ||
			registration.EntryID() != site.RegistrationID || registration.Population() != query.PopulationSelectedPoint || registration.Projection() != site.Projection {
			return nil, false
		}
		if !contributor.queryResultPublication.contract.Available() || contributor.queryResultPublication.contract.FamilyID() != identity.ContentID(registration.EntryID()) || contributor.queryResultPublication.contract.Codec() != registration.Freezer() {
			return nil, false
		}
		query, resolved := committed.Query(site.ID)
		if !resolved {
			return nil, false
		}
		if query.ContextID() != site.Context.ID() {
			return nil, false
		}
		key, keyed := query.PublicationKey()
		if !keyed {
			return nil, false
		}
		canonicalOrdinal, ordinalOK := QuerySelectedRegistrationOrdinal(bound.compilation, site.Family)
		if !ordinalOK || site.selectedOrdinal == 0 || site.selectedOrdinal != canonicalOrdinal {
			return nil, false
		}
		publications = append(publications, QueryPublication{Site: site, Key: key, contract: &bound.catalog.queryContributors[position].queryResultPublication.contract, encode: contributor.queryResultPublication.encode, ordinal: site.selectedOrdinal})
	}
	return publications, true
}
