package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
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
}

// SelectedQuerySites derives mount-qualified query identities from semantic
// occurrences in selected sealed bodies. Non-callable roots are always
// selected. A callable body is selected only when a sealed DirectFunctions
// join from an already-selected body names it.
func SelectedQuerySites(mounts []programmount.MountedArtifact) ([]QuerySite, bool) {
	families, familiesOK := selectedPointQueryIssuance()
	if !familiesOK || len(mounts) == 0 {
		return nil, false
	}
	sites := make([]QuerySite, 0)
	expected := 0
	for _, mount := range mounts {
		if !mount.Available() {
			return nil, false
		}
		snapshot := mount.Snapshot
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
			for callIndex := 0; callIndex < snapshot.CallCount(); callIndex++ {
				call, callOK := snapshot.CallAt(callIndex)
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
		pointIDs := make(map[identity.ContentID]struct{}, snapshot.PointCount())
		for index := 0; index < snapshot.PointCount(); index++ {
			point, ok := snapshot.PointAt(index)
			if !ok || !point.ID().Available() {
				return nil, false
			}
			pointIDs[point.ID()] = struct{}{}
		}
		observed := make(map[identity.ContentID]struct{})
		observedBodies := make(map[identity.ContentID]struct{}, len(selectedBodies))
		for occurrenceIndex := 0; occurrenceIndex < snapshot.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := snapshot.OccurrenceAt(occurrenceIndex)
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
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return nil, false
				}
				if _, known := pointIDs[point]; !known {
					continue
				}
				observed[point] = struct{}{}
				observedBodies[body] = struct{}{}
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
		for index := 0; index < snapshot.PointCount(); index++ {
			point, ok := snapshot.PointAt(index)
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

func selectedPointQueryIssuance() ([]IssuedQuery, bool) {
	issued := QueryIssuance()
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
		admitted, ok := bound.QueryAdmission(site.ID, site.Mount, site.Point, site.Projection)
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
		query, resolved := committed.Query(site.ID)
		if !resolved {
			return nil, false
		}
		key, keyed := query.PublicationKey()
		if !keyed {
			return nil, false
		}
		publications = append(publications, QueryPublication{Site: site, Key: key})
	}
	return publications, true
}
