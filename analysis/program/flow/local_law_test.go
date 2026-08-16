package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
)

func lowerLocalLaw(t *testing.T, name, text string) *program.Program {
	t.Helper()
	result, err := lualower.Lower(lualower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublicLocalRegionsAreOrderedAndOwnerFenced(t *testing.T) {
	first := lowerLocalLaw(t, "local-regions.lua", `
while check() do
  work()
end
`)
	regions := first.Flow().Local().Regions()
	if regions.Count() == 0 {
		t.Fatal("cyclic while did not publish a Local Region")
	}
	region, ok := regions.At(0)
	if !ok || !region.Available() {
		t.Fatal("public Local region enumeration failed")
	}
	if resolved, ok := regions.Resolve(region.ID()); !ok || resolved.ID() != region.ID() {
		t.Fatal("public Local stable identity failed to resolve")
	}
	if region.SuccessorCount() == 0 || region.SiteCount() == 0 {
		t.Fatal("public Region did not retain existing causal traversal")
	}
	successor, successorOK := region.SuccessorAt(0)
	site, siteOK := region.SiteAt(0)
	if !successorOK || !siteOK || !region.ContainsSuccessor(successor) || !region.ContainsSite(site) {
		t.Fatal("public Region traversal lost exact Causal references")
	}
	if owner, ok := regions.ForSuccessor(successor); !ok || owner.ID() != region.ID() {
		t.Fatal("public singular Successor inverse failed")
	}
	if regions.RegionCountForSite(site) == 0 {
		t.Fatal("public multi-valued Site inverse failed")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _ = regions.At(0)
		_, _ = regions.Resolve(region.ID())
		_, _ = region.SuccessorAt(0)
		_, _ = region.SiteAt(0)
		_, _ = regions.RegionAtSite(site, 0)
	}); allocs != 0 {
		t.Fatalf("public Local query allocates %v times", allocs)
	}

	foreign := lowerLocalLaw(t, "local-regions-foreign.lua", `return other()`)
	foreignRegions := foreign.Flow().Local().Regions()
	if _, ok := foreignRegions.ForSuccessor(successor); ok {
		t.Fatal("foreign Flow accepted a Local Successor")
	}
	if foreignRegions.RegionCountForSite(site) != 0 {
		t.Fatal("foreign Flow accepted a Local Site")
	}
}
