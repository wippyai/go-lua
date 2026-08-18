package link_test

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkartifact "github.com/wippyai/go-lua/analysis/program/link/artifact"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// frozenFixtureLinkFiles is the parser-valid source denominator fixed by the
// canonical Program cut.  Unlike the lower-only census, this gate also seals
// the closed sibling-module and actor/cache Link environment for every source.
const frozenFixtureLinkFiles = testfixture.FrozenLuaFileCount

type fixtureLinkFailure struct {
	summary string
	paths   []string
}

// TestFixtureProjectLinksClosedSiblingModules is a representative law for the
// generic fixture-project input builder.  It proves that a sibling module is
// registered through the normal Link module-cache ingress, not through a
// fixture-specific resolver or an ambient historical registry.
func TestFixtureProjectLinksClosedSiblingModules(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	project, err := fixtureCorpus(t).Project("modules/simple-export")
	if err != nil {
		t.Fatalf("fixture project: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("Link Seal %s: %v", project.Name(), err)
	}
	if err := roundTripFixtureProject(linked, contract); err != nil {
		t.Fatalf("Link artifact %s: %v", project.Name(), err)
	}
}

// TestFrozenFixtureCorpusLinks seals every checked-in Lua source through the
// sole parser -> binder -> Program -> Link path.  A fixture directory is one
// closed project: its manifest file inventory, when present, is the authored
// module environment; otherwise its complete local Lua inventory is used.
// The generic input builder derives only Link's required actor/cache ingress
// rows from sealed Program Imports.  It does not consult a diagnostic oracle,
// bytecode compiler, runtime, legacy analyzer, allowlist, or fixture adapter.
func TestFrozenFixtureCorpusLinks(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	projects := fixtureCorpus(t).Projects()
	denominator := 0
	failures := make(map[string]*fixtureLinkFailure)
	for _, project := range projects {
		denominator += project.FileCount()
		project := project
		t.Run(project.Name(), func(t *testing.T) {
			err := checkFrozenFixtureLinkProject(contract, project)
			if err == nil {
				return
			}
			summary := fixtureLinkFailureSummary(err)
			cluster := failures[summary]
			if cluster == nil {
				cluster = &fixtureLinkFailure{summary: summary}
				failures[summary] = cluster
			}
			for index := 0; index < project.FileCount(); index++ {
				path, pathOK := project.FileAt(index)
				if !pathOK {
					t.Fatalf("fixture project %s has malformed file index %d", project.Name(), index)
				}
				cluster.paths = append(cluster.paths, path)
			}
		})
	}
	if denominator != frozenFixtureLinkFiles {
		t.Fatalf("frozen Program-to-Link denominator = %d Lua files; want exactly %d (corpus changes require an explicit target-contract update)", denominator, frozenFixtureLinkFiles)
	}
	if len(failures) == 0 {
		t.Logf("Program-to-Link fixture denominator: %d/%d", denominator, denominator)
		return
	}

	clusters := make([]*fixtureLinkFailure, 0, len(failures))
	failed := 0
	for _, cluster := range failures {
		sort.Strings(cluster.paths)
		failed += len(cluster.paths)
		clusters = append(clusters, cluster)
	}
	sort.Slice(clusters, func(left, right int) bool {
		if len(clusters[left].paths) != len(clusters[right].paths) {
			return len(clusters[left].paths) > len(clusters[right].paths)
		}
		return clusters[left].summary < clusters[right].summary
	})
	var report strings.Builder
	fmt.Fprintf(&report, "Program-to-Link fixture numerator: %d/%d sealed; %d/%d failed parser -> bind -> Program -> Link Seal:\n", denominator-failed, denominator, failed, denominator)
	for _, cluster := range clusters {
		fmt.Fprintf(&report, "  %d x %s", len(cluster.paths), cluster.summary)
		for _, path := range cluster.paths {
			fmt.Fprintf(&report, "\n    %s", path)
		}
		report.WriteByte('\n')
	}
	t.Fatal(report.String())
}

func checkFrozenFixtureLinkProject(contract *target.Contract, project testfixture.CorpusProject) error {
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		return err
	}
	rows, err := linked.CountRows()
	if err != nil {
		return err
	}
	return assertCompleteCountRows(rows)
}

func roundTripFixtureProject(linked *link.Link, contract *target.Contract) error {
	encoded, err := linkartifact.Encode(linked)
	if err != nil {
		return fmt.Errorf("encode Link artifact: %w", err)
	}
	mounts := linked.Project().Mounts()
	programs := make(map[identity.ContentID]*program.Program, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, programOK := mounts.Program(shard)
		if !shardOK || !programOK || mounted == nil || !mounted.ContentID().Available() {
			return fmt.Errorf("Link artifact input has unavailable Program")
		}
		programs[mounted.ContentID()] = mounted
	}
	replayed, err := linkartifact.Decode(encoded, contract, programs)
	if err != nil {
		return fmt.Errorf("decode Link artifact: %w", err)
	}
	if replayed.ContentID() != linked.ContentID() {
		return fmt.Errorf("Link artifact changed ContentID")
	}
	reencoded, err := linkartifact.Encode(replayed)
	if err != nil {
		return fmt.Errorf("re-encode Link artifact: %w", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		return fmt.Errorf("Link artifact changed canonical bytes")
	}
	rows, err := replayed.CountRows()
	if err != nil {
		return fmt.Errorf("replayed Link denominator: %w", err)
	}
	return assertCompleteCountRows(rows)
}

func assertCompleteCountRows(rows denominator.CountRows) error {
	if !denominator.GeneratedCountRowsComplete(rows) {
		return fmt.Errorf("Link denominator rows = %d/%t, want exact generated relation coverage", rows.Count(), rows.Available())
	}
	return nil
}

func fixtureLinkFailureSummary(err error) string {
	return err.Error()
}
