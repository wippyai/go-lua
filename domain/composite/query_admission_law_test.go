package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

func TestQueryAdmissionRecoversSealedProjection(t *testing.T) {
	record := mountedRecord(t, "query-admission", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	if len(record.Artifacts) == 0 || !record.Artifacts[0].Available() {
		t.Fatal("sealed mount")
	}
	mount := record.Artifacts[0]
	if mount.Snapshot.PointCount() == 0 {
		t.Fatal("fixture issued no sealed points")
	}
	point, pointOK := mount.Snapshot.PointAt(0)
	pointID := point.ID()
	if !pointOK || !pointID.Available() {
		t.Fatal("sealed point")
	}
	id, idOK := identity.DeriveContentID("analysis/artifact-query/v1", mount.ModuleKey[:], pointID[:], []byte(QueryFamilyValueSummary))
	if !idOK {
		t.Fatal("query identity")
	}
	summary, summaryOK := bound.QueryAdmission(id, mount.ModuleKey, pointID, query.ProjectionSummary)
	exact, exactOK := bound.QueryAdmission(id, mount.ModuleKey, pointID, query.ProjectionExact)
	if !summaryOK || !exactOK {
		t.Fatalf("query admission refused: summary=%v exact=%v", summaryOK, exactOK)
	}
	if summary.ID != id || summary.Mount != mount.ModuleKey || summary.Point != pointID {
		t.Fatal("summary admission lost the sealed site")
	}
	if exact.ID != id || exact.Mount != mount.ModuleKey || exact.Point != pointID {
		t.Fatal("exact admission lost the sealed site")
	}
	if _, ok := bound.QueryAdmission(id, mount.ModuleKey, pointID, ""); ok {
		t.Fatal("empty projection admitted")
	}
}

func TestSelectedQuerySitesExcludeUncalledCallables(t *testing.T) {
	record := mountedRecord(t, "query-sites", `local function dormant(value)
  local retained = value
  return retained
end
return 42`)
	sites, ok := SelectedQuerySites(record.Artifacts)
	if !ok || len(sites) == 0 {
		t.Fatal("selected query sites")
	}
	callable := make(map[identity.ContentID]struct{})
	roots := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		for index := 0; index < mount.Snapshot.BodyTransportCount(); index++ {
			body, bodyOK := mount.Snapshot.BodyTransportAt(index)
			if !bodyOK {
				t.Fatal("sealed body")
			}
			if body.Callable() {
				callable[body.BodyID()] = struct{}{}
			} else {
				roots[body.BodyID()] = struct{}{}
			}
		}
	}
	callablePoints := make(map[identity.ContentID]struct{})
	rootPoints := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		for index := 0; index < mount.Snapshot.OccurrenceCount(); index++ {
			occurrence, occurrenceOK := mount.Snapshot.OccurrenceAt(index)
			body, bodyOK := occurrence.BodyID()
			if !occurrenceOK || !bodyOK {
				continue
			}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK {
					t.Fatal("sealed occurrence point")
				}
				if _, held := callable[body]; held {
					callablePoints[point] = struct{}{}
				}
				if _, held := roots[body]; held {
					rootPoints[point] = struct{}{}
				}
			}
		}
	}
	if len(callablePoints) == 0 || len(rootPoints) == 0 {
		t.Fatal("fixture issued no callable/root occurrence points")
	}
	perPoint := make(map[identity.ContentID]int)
	for _, site := range sites {
		if _, forbidden := callablePoints[site.Point]; forbidden {
			t.Fatal("uncalled callable interior became a query site")
		}
		if _, root := rootPoints[site.Point]; !root {
			t.Fatal("query site escaped the non-callable root")
		}
		perPoint[site.Point]++
	}
	for point, count := range perPoint {
		if count != 2 {
			t.Fatalf("root point %v query lanes = %d", point, count)
		}
	}
}
