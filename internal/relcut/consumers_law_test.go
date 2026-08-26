package relcut

import "testing"

// The consumer map a Wave 4 lane restates from is live: it decodes, it is
// internally consistent, and every file and symbol it names is still there.
// A consumer that stopped reading a symbol is as much a defect as one that was
// never recorded, because the lane sizes its work from this file.
func TestConsumerMapIsLive(t *testing.T) {
	consumers, err := LoadConsumers()
	if err != nil {
		t.Fatal(err)
	}
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	findings := ValidateConsumers(consumers, root)
	for _, finding := range findings {
		if finding.Severity == SeverityRefused {
			t.Errorf("%s", finding)
			continue
		}
		t.Logf("%s", finding)
	}
	if Refused(findings) {
		t.Fatal("the consumer map does not describe this tree")
	}
}

// Every surface the map covers is a surface the deletion manifest accounts for.
// A read surface no manifest entry removes is one the cut would leave standing
// with its readers still pointed at it.
func TestEverySurfaceIsInTheManifest(t *testing.T) {
	consumers, err := LoadConsumers()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]struct{}{}
	for _, entry := range manifest.Entries {
		entries[entry.ID] = struct{}{}
	}
	for _, surface := range consumers.Surfaces {
		if surface.ManifestEntry == "" {
			t.Errorf("surface %s names no manifest entry", surface.Package)
			continue
		}
		if _, held := entries[surface.ManifestEntry]; !held {
			t.Errorf("surface %s names unknown manifest entry %s", surface.Package, surface.ManifestEntry)
		}
	}
}

// A consumer the manifest already owns must agree with it: the manifest says
// restate, and the map says what it restates onto. A consumer the manifest does
// not own is the map's own finding and needs no entry, but it may not silently
// claim one.
func TestManifestEntriesAgree(t *testing.T) {
	consumers, err := LoadConsumers()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	dispositions := map[string]Disposition{}
	for _, entry := range manifest.Entries {
		dispositions[entry.ID] = entry.Disposition
	}
	for _, consumer := range consumers.Consumers {
		if consumer.ManifestEntry == "" {
			continue
		}
		disposition, held := dispositions[consumer.ManifestEntry]
		if !held {
			t.Errorf("%s names unknown manifest entry %s", consumer.Path, consumer.ManifestEntry)
			continue
		}
		if disposition == DispositionDelete {
			t.Errorf("%s is mapped as a surviving consumer but its manifest entry %s deletes it", consumer.Path, consumer.ManifestEntry)
		}
	}
}

// Every gap names the consumers it blocks and every gapped consumer names its
// gap. The two directions are the same fact, so a map where they disagree has
// lost a finding rather than recorded one.
func TestGapsCloseBothWays(t *testing.T) {
	consumers, err := LoadConsumers()
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range consumers.Gaps {
		listed := map[string]struct{}{}
		for _, path := range gap.Consumers {
			listed[path] = struct{}{}
		}
		for _, consumer := range consumers.Consumers {
			_, named := listed[consumer.Path]
			cites := consumer.Gap == gap.ID || consumer.WaitsOn == gap.ID
			if cites && !named {
				t.Errorf("gap %s does not list consumer %s that cites it", gap.ID, consumer.Path)
			}
			if !cites && named {
				t.Errorf("gap %s lists consumer %s that does not cite it", gap.ID, consumer.Path)
			}
		}
	}
}

// A map that names a symbol its consumer no longer reads is refused. This is
// the freshness that makes the map worth sizing work from: the surfaces move
// under it, and a silent drift would understate the cut.
func TestStaleConsumerSymbolIsRefused(t *testing.T) {
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	surface := "github.com/wippyai/go-lua/analysis/engine"
	stale := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S6-engine-runtime"}},
		Consumers: []Consumer{{
			Path:          "analysis/analyze.go",
			ManifestEntry: "S1-analyzer-constructor",
			Reads:         []Read{{Package: surface, Symbol: "ThisSymbolNeverExisted", Uses: 1, Class: ReadClassQuery}},
			Target:        Target{Kind: TargetRuntimeComposition, Package: surface, Note: "x"},
		}},
	}
	if !Refused(ValidateConsumers(stale, root)) {
		t.Fatal("a consumer entry naming a symbol nobody declares was accepted")
	}
}

// A consumer file the tree no longer holds is refused for the same reason.
func TestMissingConsumerFileIsRefused(t *testing.T) {
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	surface := "github.com/wippyai/go-lua/analysis/engine"
	missing := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S6-engine-runtime"}},
		Consumers: []Consumer{{
			Path:          "analysis/does-not-exist.go",
			ManifestEntry: "S1-analyzer-constructor",
			Reads:         []Read{{Package: surface, Symbol: "Answer", Uses: 1, Class: ReadClassQuery}},
			Target:        Target{Kind: TargetRuntimeComposition, Package: surface, Note: "x"},
		}},
	}
	if !Refused(ValidateConsumers(missing, root)) {
		t.Fatal("a consumer file that left the tree was accepted")
	}
}

// A gap is the absence of a column, so an entry that claims both is refused,
// and so is one that cites a gap the map never declared. Inventing a column to
// close a gap is the failure this refuses to let pass quietly.
func TestGapAndColumnCannotBothBeClaimed(t *testing.T) {
	surface := "github.com/wippyai/go-lua/analysis/engine"
	both := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S6-engine-runtime"}},
		Gaps:     []Gap{{ID: "G", Distinction: "d", Consumers: []string{"analysis/analyze.go"}}},
		Consumers: []Consumer{{
			Path:          "analysis/analyze.go",
			ManifestEntry: "S1-analyzer-constructor",
			Reads:         []Read{{Package: surface, Symbol: "Answer", Uses: 1, Class: ReadClassQuery}},
			Target:        Target{Kind: TargetGap, Column: "invented", Note: "x"},
			Gap:           "G",
		}},
	}
	if !Refused(ValidateConsumers(both, "")) {
		t.Fatal("a gap entry that also named a column was accepted")
	}
	unknown := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S6-engine-runtime"}},
		Consumers: []Consumer{{
			Path:          "analysis/analyze.go",
			ManifestEntry: "S1-analyzer-constructor",
			Reads:         []Read{{Package: surface, Symbol: "Answer", Uses: 1, Class: ReadClassQuery}},
			Target:        Target{Kind: TargetGap, Note: "x"},
			Gap:           "NoSuchGap",
		}},
	}
	if !Refused(ValidateConsumers(unknown, "")) {
		t.Fatal("a consumer citing an undeclared gap was accepted")
	}
}

// Every prerequisite a consumer waits on is one the cut can deliver: a manifest
// entry that lands, or a gap the map declares. A prerequisite that names
// neither is a move deferred to nobody, which reads as scheduled work and is
// not.
func TestEveryPrerequisiteIsDeliverable(t *testing.T) {
	consumers, err := LoadConsumers()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]struct{}{}
	for _, entry := range manifest.Entries {
		entries[entry.ID] = struct{}{}
	}
	for _, consumer := range consumers.Consumers {
		if consumer.WaitsOn == "" {
			continue
		}
		if _, held := entries[consumer.WaitsOn]; held {
			continue
		}
		if _, held := consumers.Gap(consumer.WaitsOn); held {
			continue
		}
		t.Errorf("%s waits on %s, which is neither a manifest entry nor a declared gap", consumer.Path, consumer.WaitsOn)
	}
}

// A consumer pointed at a surface that answers none of its reads, and naming no
// prerequisite, is refused. This is the shape a bridge arrives in: the entry
// claims the move is available, the surface does not carry the distinction, and
// the difference is made up in the consumer.
func TestUnreachableMoveIsRefused(t *testing.T) {
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	surface := "github.com/wippyai/go-lua/analysis/engine"
	target := "github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	unreachable := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S6-engine-runtime"}},
		Consumers: []Consumer{{
			Path:          "analysis/compile.go",
			ManifestEntry: "S1-analyzer-constructor",
			Reads:         []Read{{Package: surface, Symbol: "PublishColumn", Uses: 1, Class: ReadClassDeclaration}},
			Target:        Target{Kind: TargetDeclaredSurface, Package: target, Note: "x"},
		}},
	}
	if !Refused(ValidateConsumers(unreachable, root)) {
		t.Fatal("a consumer whose target answers none of its reads was accepted with no prerequisite")
	}
	reachable := unreachable
	reachable.Gaps = []Gap{{
		ID:          "G",
		Distinction: "a production entry into the declared rule surface",
		Consumers:   []string{"analysis/compile.go"},
	}}
	reachable.Consumers = []Consumer{unreachable.Consumers[0]}
	reachable.Consumers[0].WaitsOn = "G"
	if Refused(ValidateConsumers(reachable, root)) {
		t.Fatal("a consumer that names the prerequisite its move waits on was refused")
	}
}

// The other direction: an entry whose target already declares every symbol it
// reads is at its post-cut home, and a prerequisite there would defer a move
// that is done.
func TestSettledConsumerNamingAPrerequisiteIsRefused(t *testing.T) {
	root, err := RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	surface := "github.com/wippyai/go-lua/analysis/engine/operand"
	settled := ConsumerMap{
		Surfaces: []Surface{{Package: surface, ManifestEntry: "S2-reducer-operand-abi"}},
		Gaps:     []Gap{{ID: "G", Distinction: "d", Consumers: []string{"domain/placement/suspension/reducer.go"}}},
		Consumers: []Consumer{{
			Path:          "domain/placement/suspension/reducer.go",
			ManifestEntry: "",
			Reads:         []Read{{Package: surface, Symbol: "SummaryVector", Uses: 1, Class: ReadClassRowABI}},
			Target:        Target{Kind: TargetRowABI, Package: surface, Note: "already at its post-cut home"},
			WaitsOn:       "G",
		}},
	}
	if !Refused(ValidateConsumers(settled, root)) {
		t.Fatal("an entry already at its post-cut home was accepted while waiting on a prerequisite")
	}
}
