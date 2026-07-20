package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestForEachAllocationSiteProjectsDecomposableFact(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`
local opts = { a = 1, b = 2 }
local total = opts.a + opts.b
return total
`, "allocation_site_readmodel.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	resultProgram22, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: reg}})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	result := resultProgram22.RootResult()

	var sites []AllocationSite
	New(result).ForEachAllocationSite(func(site AllocationSite) bool {
		sites = append(sites, site)
		return true
	})
	if len(sites) != 1 {
		t.Fatalf("allocation sites = %d, want 1: %#v", len(sites), sites)
	}
	site := sites[0]
	if site.SchemaVersion != readapi.AllocationSiteSchemaVersion {
		t.Fatalf("schema version = %d, want %d", site.SchemaVersion, readapi.AllocationSiteSchemaVersion)
	}
	if !site.StableShape || len(site.Fields) != 2 {
		t.Fatalf("stable shape/fields = %v/%#v, want two stable fields", site.StableShape, site.Fields)
	}
	if !site.HasPlacement {
		t.Fatalf("site has no placement: %#v", site)
	}
	if !site.Decomposable {
		t.Fatalf("site is not decomposable: %#v", site)
	}
	if !site.FrameLocalUseProof || !site.HasDiesBeforeSuspension || !site.DiesBeforeSuspension || site.BirthPoint == 0 || !site.HasBirthSpan {
		t.Fatalf("site is missing unified use/lifetime facts: %#v", site)
	}
}
