package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

const querySiteFormula = "analysis/artifact-query/v1"

// QuerySite is one selected-point query identity derived from sealed ingress
// bodies. Result reads Snapshot by the publication key attach captures for
// this site.
type QuerySite struct {
	ID         identity.ContentID
	Mount      identity.ContentID
	Point      identity.ContentID
	Family     schema.Key
	Authority  schema.Key
	Projection schema.Key
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

// SelectedQuerySites derives mount-qualified query identities from semantic
// occurrences in selected sealed bodies. Non-callable roots are always
// selected. A callable body is selected only when a sealed DirectFunctions
// join from an already-selected body names it.
func SelectedQuerySites(compilation Compilation, mounts []programmount.MountedArtifact) ([]QuerySite, bool) {
	state := compilation.catalog
	families, familiesOK := selectedPointQueryIssuance(state)
	if !familiesOK || len(mounts) == 0 {
		return nil, false
	}
	sites := make([]QuerySite, 0)
	expected := 0
	for _, mount := range mounts {
		if !mount.Available() {
			return nil, false
		}
		program := mount.Program
		bodyEntries := make(map[identity.ContentID][]identity.ContentID)
		callable := make(map[identity.ContentID]struct{})
		rootBodies := make(map[identity.ContentID][]identity.ContentID)
		bodyCount, bodiesPublished := mount.Program.BodyCount()
		if !bodiesPublished {
			return nil, false
		}
		for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
			body, bodyOK := mount.Program.BodyAt(bodyIndex)
			if !bodyOK || !body.ID().Available() {
				return nil, false
			}
			entries := make([]identity.ContentID, body.EntryCount())
			for entryIndex := range entries {
				entryRow, entryOK := mount.Program.BodyEntryFor(bodyIndex, entryIndex)
				entry := entryRow.PointID()
				if !entryOK || !entry.Available() {
					return nil, false
				}
				entries[entryIndex] = entry
			}
			if len(entries) == 0 {
				return nil, false
			}
			bodyEntries[body.ID()] = entries
			if body.Callable() {
				callable[body.ID()] = struct{}{}
				continue
			}
			rootBodies[body.ID()] = entries
		}
		if len(rootBodies) == 0 {
			return nil, false
		}
		selectedBodies := make(map[identity.ContentID][]identity.ContentID, len(rootBodies))
		for body, entries := range rootBodies {
			selectedBodies[body] = entries
		}
		for changed := true; changed; {
			changed = false
			callCount, callsPublished := program.CallCount()
			if !callsPublished {
				return nil, false
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
					return nil, false
				}
				selectedBodies[target] = entries
				changed = true
			}
		}
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		pointCount, pointsPublished := programschema.PointFamily().Count(&program.Frozen, catalog)
		if !program.Available() || !catalogOK || !pointsPublished {
			return nil, false
		}
		pointIDs := make(map[identity.ContentID]struct{}, pointCount)
		for index := 0; index < pointCount; index++ {
			point, ok := programschema.PointFamily().At(&program.Frozen, catalog, index)
			if !ok || !point.ID().Available() {
				return nil, false
			}
			pointIDs[point.ID()] = struct{}{}
		}
		observed := make(map[identity.ContentID]struct{})
		observedBodies := make(map[identity.ContentID]struct{}, len(selectedBodies))
		occurrenceCount, occurrencesPublished := program.OccurrenceCount()
		if !occurrencesPublished {
			return nil, false
		}
		for occurrenceIndex := 0; occurrenceIndex < occurrenceCount; occurrenceIndex++ {
			occurrence, occurrenceOK := program.OccurrenceAt(occurrenceIndex)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK {
				return nil, false
			}
			if !bodyOK {
				continue
			}
			if _, selected := selectedBodies[body]; !selected {
				continue
			}
			_, pointCount, spanOK := occurrence.PointSpan()
			if !spanOK {
				return nil, false
			}
			for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
				pointRow, pointOK := program.OccurrencePointFor(occurrenceIndex, pointIndex)
				point := pointRow.PointID()
				if !pointOK || !pointRow.Available() || !point.Available() {
					return nil, false
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
			return nil, false
		}
		regions := make([][]identity.ContentID, regionCount)
		regionsByPoint := make(map[identity.ContentID][]int, pointCount)
		for regionIndex := 0; regionIndex < regionCount; regionIndex++ {
			region, regionOK := programschema.RegionFamily().At(&program.Frozen, catalog, regionIndex)
			offset, count, spanOK := region.MemberSpan()
			if !regionOK || !spanOK || count == 0 {
				return nil, false
			}
			members := make([]identity.ContentID, count)
			for memberIndex := uint32(0); memberIndex < count; memberIndex++ {
				member, memberOK := programschema.RegionMemberFamily().At(&program.Frozen, catalog, int(offset+memberIndex))
				point := member.ID()
				if !memberOK || !point.Available() {
					return nil, false
				}
				if _, known := pointIDs[point]; !known {
					return nil, false
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
		for index := 0; index < pointCount; index++ {
			point, ok := programschema.PointFamily().At(&program.Frozen, catalog, index)
			if !ok || !point.ID().Available() {
				return nil, false
			}
			pointID := point.ID()
			if _, selected := observed[pointID]; !selected {
				continue
			}
			expected++
			for _, family := range families {
				id, idOK := identity.DeriveContentID(querySiteFormula, mount.ModuleKey[:], pointID[:], []byte(family.Family))
				if !idOK {
					return nil, false
				}
				sites = append(sites, QuerySite{
					ID: id, Mount: mount.ModuleKey, Point: pointID,
					Family: family.Family, Authority: family.Authority, Projection: family.Projection,
				})
			}
		}
	}
	return sites, expected > 0 && len(sites) == len(families)*expected
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
		if !family.Family.Available() || !family.Authority.Available() || !family.Projection.Available() {
			return nil, false
		}
		if family.Projection != query.ProjectionSummary && family.Projection != query.ProjectionExact {
			return nil, false
		}
		selected = append(selected, family)
	}
	return selected, len(selected) > 0
}

// QueryAdmissions seals one engine admission row per selected-point site.
func (bound *ProgramBinding) QueryAdmissions(sites []QuerySite) ([]engine.ProgramQueryAdmission, bool) {
	if bound == nil || len(sites) == 0 {
		return nil, false
	}
	rows := make([]engine.ProgramQueryAdmission, 0, len(sites))
	for _, site := range sites {
		admitted, ok := bound.QueryAdmission(site.ID, site.Mount, site.Point, site.Family)
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
func (bound *ProgramBinding) QueryPublications(committed *engine.CommittedProgram, sites []QuerySite) ([]QueryPublication, bool) {
	if bound == nil || committed == nil || len(sites) == 0 {
		return nil, false
	}
	publications := make([]QueryPublication, 0, len(sites))
	for _, site := range sites {
		position, positioned := queryPositionForFamily(bound.catalog, site.Family)
		if !positioned || position < 0 || bound.catalog == nil || position >= len(bound.catalog.queries) || position >= len(bound.catalog.queryContributors) {
			return nil, false
		}
		registration := bound.catalog.queries[position]
		contributor := bound.catalog.queryContributors[position]
		if registration == nil || !contributor.complete() || registration.Key() != site.Family ||
			registration.Population() != query.PopulationSelectedPoint || registration.Projection() != site.Projection {
			return nil, false
		}
		if !contributor.contract.Available() || contributor.contract.FamilyID() != identity.ContentID(registration.ID()) || contributor.contract.Codec() != registration.Freezer() {
			return nil, false
		}
		query, resolved := committed.Query(site.ID)
		if !resolved {
			return nil, false
		}
		key, keyed := query.PublicationKey()
		if !keyed {
			return nil, false
		}
		publications = append(publications, QueryPublication{Site: site, Key: key, contract: &bound.catalog.queryContributors[position].contract, encode: contributor.encode, ordinal: uint32(position + 1)})
	}
	return publications, true
}
