//go:build zzsolveprobe

package oracle

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	engineprobe "github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/canonical"
)

// ZZPROBE: solver-ladder stage 0 measurement lane. It runs the frozen corpus
// serially through the existing diagnostic spine and attributes every global
// solve counter to the fixture that produced it, which a concurrent walk
// cannot do. Compiled only with -tags zzsolveprobe; the default oracle test
// binary never links this file.
//
// The classes it reports are the operations an apply-cache or a region memo
// would have to serve:
//
//	evaluates  -- one Rule Group fold over its transported input vector
//	             (executorEpoch.evaluate, runtime_point_fold.go:71)
//	folds      -- one canonical Point RHS transaction
//	             (foldPointTermSetsWithBoundary, runtime_point_fold.go:360)
//	rhs        -- one Region head E+B refold
//	             (regionRHS, runtime_region_interface.go:20)
//	refreshes  -- one Point candidate replacement pass
//	             (refreshPoint, runtime_point_refresh.go:202)
//	cellPairs  -- one pairwise lattice join inside a many-way merge
//	             (semantic.JoinContributionsMany)
//
// Distinct-key counts for the first four classes need call-site hooks that do
// not exist yet; only cellPairs is key-instrumented today (semantic's
// zzProbeCellPair hook, readable with -tags typprobe).

type zzProbeSolverSample struct {
	name  string
	class string
	err   bool

	seal, compile, solve time.Duration

	epochs, passes, refreshes, evaluates, folds, rhs, restarts uint64
	evaluateFailures, activations                              uint64
	publications, semanticPubs, rawPubs, rawOnly, bumps        uint64
	ifaceRefresh, ifaceDone, ifaceFallback                     uint64
	maxQueue, maxEpisode                                       uint64

	dbgFolds, dbgFoldTerms, dbgFoldMax               uint64
	reuseAdmit, reuseRefuse, reuseTerms, rebuildTerm uint64
	dropRestart, dropNarrowPhase, dropNarrowFold     uint64

	refuseNotAscent, refuseNoAccumulator, refuseColdEpisode, refuseDroppedAccum  uint64
	refusePendingUnknown, refusePendingDescend, refuseNotOwned, refuseChangedRow uint64
	carryRetained, carryExtended, carryRebuilt, carryOpened                      uint64

	mergeMany, cells, cellPairs, cellWidth, maxOperand uint64

	regionsTotal, regionsWidenFactorFree               uint64
	backEnvTerms, backFactorTerms, backGroupTerms      uint64
	regionsPureTransport, regionsLinearCandidate       uint64
	regionInteriorPointsMax, regionInteriorPointsTotal uint64

	status    string
	signature string
	errText   string
}

func zzProbeSolverSampleOne(t *testing.T, project corpusHarnessProject) zzProbeSolverSample {
	sample := zzProbeSolverSample{name: project.name}
	engineprobe.DbgEngineReset()
	engineprobe.DbgMergeReset()
	run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessDiagnosticMode())
	sample.class, sample.err = class, err != nil
	sample.signature = zzProbeFailureSignature(run, class)
	sample.errText = zzProbeFlatten(err)
	if run == nil {
		return sample
	}
	sample.status = corpusHarnessStatusName(run.status)
	sample.seal, sample.compile, sample.solve = run.cost.seal, run.cost.compile, run.cost.solve
	engine := run.solveDiagnostics.Engine
	sample.epochs, sample.passes = engine.Epochs, engine.EpochPasses
	sample.refreshes, sample.evaluates = engine.Refreshes, engine.Evaluates
	sample.evaluateFailures, sample.activations = engine.EvaluateFailures, engine.Activations
	sample.folds, sample.rhs, sample.restarts = engine.Folds, engine.RegionRHS, engine.Restarts
	sample.publications, sample.semanticPubs = engine.Publications, engine.SemanticPublications
	sample.rawPubs, sample.rawOnly, sample.bumps = engine.RawPublications, engine.RawOnlyPublications, engine.VersionBumps
	sample.ifaceRefresh, sample.ifaceDone, sample.ifaceFallback = engine.InterfaceRefreshes, engine.InterfaceRefreshCompleted, engine.InterfaceRefreshFallbacks
	sample.maxQueue, sample.maxEpisode = engine.MaxQueue, engine.MaxEpisode

	counters := engineprobe.DbgEngine()
	sample.dbgFolds, sample.dbgFoldTerms, sample.dbgFoldMax = counters.Folds, counters.FoldTerms, counters.FoldMaxTerms
	sample.reuseAdmit, sample.reuseRefuse = counters.ReuseAdmit, counters.ReuseRefuse
	sample.reuseTerms, sample.rebuildTerm = counters.ReuseTerms, counters.RebuildTerms
	sample.dropRestart, sample.dropNarrowPhase, sample.dropNarrowFold = counters.DropRestart, counters.DropNarrowPhase, counters.DropNarrowFold
	sample.refuseNotAscent, sample.refuseNoAccumulator = counters.RefuseNotAscent, counters.RefuseNoAccumulator
	sample.refuseColdEpisode, sample.refuseDroppedAccum = counters.RefuseColdEpisode, counters.RefuseDroppedAccum
	sample.refusePendingUnknown, sample.refusePendingDescend = counters.RefusePendingUnknown, counters.RefusePendingDescend
	sample.refuseNotOwned, sample.refuseChangedRow = counters.RefuseNotOwned, counters.RefuseChangedRow
	sample.carryRetained, sample.carryExtended = counters.CarryRetained, counters.CarryExtended
	sample.carryRebuilt, sample.carryOpened = counters.CarryRebuilt, counters.CarryOpened

	sample.mergeMany, sample.cells, sample.cellPairs, sample.cellWidth, sample.maxOperand = engineprobe.DbgMerge()

	sample.regionsTotal, sample.regionsWidenFactorFree = counters.RegionsTotal, counters.RegionsWidenFactorFree
	sample.backEnvTerms, sample.backFactorTerms, sample.backGroupTerms = counters.BackEnvTerms, counters.BackFactorTerms, counters.BackGroupTerms
	sample.regionsPureTransport, sample.regionsLinearCandidate = counters.RegionsPureTransport, counters.RegionsLinearCandidate
	sample.regionInteriorPointsMax, sample.regionInteriorPointsTotal = counters.RegionInteriorPointsMax, counters.RegionInteriorPointsTotal
	return sample
}

func zzProbeSolverLine(sample zzProbeSolverSample) string {
	return fmt.Sprintf(
		"%-52s solve=%-10s compile=%-10s class=%-14s err=%t | epochs=%d passes=%d refreshes=%d evaluates=%d folds=%d rhs=%d restarts=%d "+
			"pubs=%d sem=%d raw=%d rawOnly=%d bumps=%d iface=%d/%d/%d maxQueue=%d maxEpisode=%d | "+
			"dbgFolds=%d foldTerms=%d foldMax=%d reuse=%d/%d reuseTerms=%d rebuildTerms=%d drops=%d/%d/%d | "+
			"refuse{notAscent=%d noAccum=%d cold=%d dropped=%d pendUnknown=%d pendDescend=%d notOwned=%d changedRow=%d} carry{%d/%d/%d/%d} | "+
			"mergeMany=%d cells=%d cellPairs=%d cellWidth=%d maxOperand=%d | "+
			"regions=%d widenFree=%d pureTransport=%d linearCandidate=%d back{env=%d factor=%d group=%d} interior{max=%d total=%d}",
		sample.name, sample.solve.Round(time.Microsecond), sample.compile.Round(time.Microsecond), sample.class, sample.err,
		sample.epochs, sample.passes, sample.refreshes, sample.evaluates, sample.folds, sample.rhs, sample.restarts,
		sample.publications, sample.semanticPubs, sample.rawPubs, sample.rawOnly, sample.bumps,
		sample.ifaceRefresh, sample.ifaceDone, sample.ifaceFallback, sample.maxQueue, sample.maxEpisode,
		sample.dbgFolds, sample.dbgFoldTerms, sample.dbgFoldMax, sample.reuseAdmit, sample.reuseRefuse,
		sample.reuseTerms, sample.rebuildTerm, sample.dropRestart, sample.dropNarrowPhase, sample.dropNarrowFold,
		sample.refuseNotAscent, sample.refuseNoAccumulator, sample.refuseColdEpisode, sample.refuseDroppedAccum,
		sample.refusePendingUnknown, sample.refusePendingDescend, sample.refuseNotOwned, sample.refuseChangedRow,
		sample.carryRetained, sample.carryExtended, sample.carryRebuilt, sample.carryOpened,
		sample.mergeMany, sample.cells, sample.cellPairs, sample.cellWidth, sample.maxOperand,
		sample.regionsTotal, sample.regionsWidenFactorFree, sample.regionsPureTransport, sample.regionsLinearCandidate,
		sample.backEnvTerms, sample.backFactorTerms, sample.backGroupTerms,
		sample.regionInteriorPointsMax, sample.regionInteriorPointsTotal)
}

// zzProbeFlatten renders one error as a single bounded line, so a dump row
// stays one record.
func zzProbeFlatten(err error) string {
	if err == nil {
		return ""
	}
	text := strings.Join(strings.Fields(err.Error()), " ")
	if len(text) > 600 {
		text = text[:600] + "..."
	}
	return text
}

// zzProbeSolveFailure renders one engine boundary with its decoded site name.
// The engine publishes the site as an opaque digest; the decoder below mints
// every declared boundary preimage once and recovers the name by lookup, which
// is the same identity the engine minted, never a parallel classification.
func zzProbeSolveFailure(failure engineprobe.SolveFailure) string {
	if !failure.Available() {
		return "none"
	}
	return failure.String() + "(" + zzProbeDecodeSite(failure.Site) + ")"
}

// zzProbeFailureSignature is the compact refusal coordinate one fixture
// reports: the analysis envelope that stopped, the binder verdict under it,
// and every engine boundary identity it carried.
func zzProbeFailureSignature(run *corpusHarnessRun, class string) string {
	if run == nil {
		return "phase=none"
	}
	diagnostics := run.solveDiagnostics
	if class == "compile" || diagnostics.Phase == anadiag.AnalyzeDiagnosticPhaseNone {
		diagnostics = run.compileDiagnostics
	}
	engineFailure := diagnostics.Engine.Failure
	// The solve path publishes AnalyzeDiagnostics.Rule as unknown for every
	// engine failure, because rule-slot capabilities are opaque at that
	// boundary. The sealed rule table classifies the engine's own rule key, so
	// the owning rule is recoverable here from the same authority the analyzer
	// would consult.
	failedRule := composite.DiagnosticRuleForSemantic(run.compilation, engineFailure.Rule())
	return fmt.Sprintf(
		"phase=%s reason=%s rule=%s axis=%s astage=%s binding=%s brstage=%s issuance=%s vseal=%s alloc=%s "+
			"aseal=%s@%d alower=%s acommit=%s@%d obsattach=%s constr=%s "+
			"efail={avail=%t reason=%d site=%s owner=%s point=%v group=%v member=%v rule=%v}",
		diagnostics.Phase, diagnostics.Reason, diagnostics.Rule, diagnostics.Axis,
		diagnostics.AssembleStage, diagnostics.Binding, diagnostics.BindingRuleStage,
		diagnostics.ItemIssuance, diagnostics.ValueSeal, diagnostics.AllocationCatalog,
		zzProbeSolveFailure(diagnostics.AssembleSeal), diagnostics.AssembleOrdinal,
		zzProbeSolveFailure(diagnostics.AssembleLowering),
		zzProbeSolveFailure(diagnostics.AssembleCommit), diagnostics.AssembleScheduleOrdinal,
		zzProbeSolveFailure(diagnostics.ObservationAttach),
		zzProbeSolveFailure(diagnostics.Construction),
		engineFailure.Available(), engineFailure.Reason(),
		zzProbeSolveFailure(engineFailure.Failure()), failedRule,
		engineFailure.Point().Available(), engineFailure.Group().Available(),
		engineFailure.Member().Available(), engineFailure.Rule().Available())
}

// The site decoder. Every engine boundary identity is a framed digest over a
// closed preimage, so the whole declared boundary space is enumerable: mint
// each declared preimage once and the observed digest names itself. The
// preimages restated here are the engine's own framings; a boundary the engine
// adds without updating this table decodes as its hex prefix, never as a
// neighbouring name.
const (
	zzProbeSiteDomain          = "engine/solve-failure-site"
	zzProbeSiteVersion         = 1
	zzProbeEquationSiteDomain  = "analysis/engine/equation/seal-failure-site"
	zzProbeEquationSiteVersion = 1
)

// zzProbeBoundarySites are the refused/stalled site names of the engine's
// solveBoundary table. Disposition is not part of a site preimage, so a
// refused and a stalled boundary of one name share one identity.
var zzProbeBoundarySites = []string{
	"activation-contribution", "activation-epoch", "activation-instance", "activation-product",
	"activation-reads", "derivation", "fold", "preflight", "publication", "result", "checkpoint",
	"carrier", "decode", "factor-row", "freeze", "observation-row", "projection", "query-row",
	"root", "shape", "support", "unit", "work",
	"acyclic-fold-begin", "acyclic-fold-context-transport", "acyclic-fold-environment",
	"acyclic-fold-factor-admission", "acyclic-fold-factor-projection", "acyclic-fold-factor-state",
	"acyclic-fold-factor-transport", "acyclic-fold-factor-validation", "acyclic-fold-finish",
	"acyclic-fold-inputs", "acyclic-fold-point", "acyclic-fold-producer", "acyclic-point-base",
	"acyclic-publication", "candidate", "candidate-order", "candidate-order-descent",
	"candidate-order-region", "candidate-order-stable-inputs", "demand-commit",
	"region-ascent-monotone", "region-discharge", "region-interface", "region-merge",
	"region-publication", "region-rhs", "validation",
	"narrow", "postfix", "visit", "narrow-no-progress", "postfix-stalled", "visit-no-progress",
}

var zzProbeFamilyNames = [...]string{"none", "compile", "execution", "refresh", "schedule", "observation"}

// zzProbeSealStageNames is the published ProgramSealStage table.
var zzProbeSealStageNames = [...]string{
	"none", "admission", "topology-seal", "query-address", "observation-address",
	"factor-bind", "member-bind", "program-seal", "solver-mint",
}

// zzProbeConstructionStepNames is the construction predicate table refused
// under a seal stage.
var zzProbeConstructionStepNames = [...]string{
	"none", "binding", "source-plane", "declaration-shape", "mount-row", "bootstrap-row",
	"point-row", "point-order", "edge-row", "bootstrap-transport", "member-issuance",
	"member-row", "member-group", "activation-row", "query-row", "candidate-row",
	"duplicate-identity", "topology-seal", "graph", "schedule", "directory",
}

var zzProbeSealPhaseNames = [...]string{
	"none", "sources", "artifact-rows", "link-issuance", "mounted-issuance",
	"activation-issuance", "rule-row", "query-batch",
}

var zzProbeAssemblyNames = [...]string{
	"none", "input", "schema", "snapshot", "rows", "structural-rows",
	"snapshot-transport", "snapshot-mount", "snapshot-artifact", "snapshot-namespace",
	"snapshot-topology", "snapshot-topology-mount", "snapshot-topology-point",
	"snapshot-bootstrap", "snapshot-topology-rule",
}

var zzProbeObservationSealNames = [...]string{
	"none", "arguments", "compilation", "binding", "projection", "point",
	"mapping", "factor", "unit", "duplicate",
}

var zzProbeArtifactRowNames = [...]string{"none", "owner", "point", "bootstrap"}

// zzProbeEquationSites are equation's own seal boundaries, whose family and
// leading site bytes enter the program-seal source preimage.
var zzProbeEquationSites = map[uint64][]string{
	1: {"batch-identity", "formal-coverage", "occurrence-identity", "occurrence-row",
		"operand-identity", "operand-row", "precondition", "site-identity", "site-row",
		"target-environment-edge", "target-environment-input", "target-factor-edge",
		"target-group", "target-group-input", "target-input", "target-rule",
		"target-state", "target-summary", "target-weak"},
	2: {"activation-directory", "activation-rows", "activation-triggers", "catalog",
		"deferred-queries", "input", "instances", "operand-realms", "points", "reissue", "targets"},
	3: {"activation", "assembly", "catalog", "catalog-usage", "decisions", "environment-edges",
		"factor-edges", "groups", "input", "instances", "point-ranks", "points", "queries",
		"row-directory"},
	4: {"graph-key-identity", "graph-key-schedule", "graph-key-structure", "topology-key"},
}

const zzProbeSealRowOrdinalBound = 2048

var (
	zzProbeSiteOnce  sync.Once
	zzProbeSiteTable map[identity.ContentID]string
)

func zzProbeFramed(domain string, version uint64, encode func(*canonical.DigestWriter) bool) (identity.ContentID, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(domain, version) != nil || !encode(&writer) || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	digest := writer.Sum()
	if digest == [32]byte{} {
		return identity.ContentID{}, false
	}
	return identity.ContentID(digest), true
}

func zzProbeBoundaryDigest(family uint64, site string, transport uint64) (identity.ContentID, bool) {
	return zzProbeFramed(zzProbeSiteDomain, zzProbeSiteVersion, func(writer *canonical.DigestWriter) bool {
		return writer.Uint(family) == nil && writer.Bytes([]byte(site)) == nil && writer.Uint(transport) == nil
	})
}

func zzProbeAuthorityDigest(family uint64, authority string, ordinals ...uint64) (identity.ContentID, bool) {
	return zzProbeFramed(zzProbeSiteDomain, zzProbeSiteVersion, func(writer *canonical.DigestWriter) bool {
		if writer.Uint(family) != nil || writer.Bytes([]byte(authority)) != nil || writer.Count(uint64(len(ordinals))) != nil {
			return false
		}
		for _, ordinal := range ordinals {
			if writer.Uint(ordinal) != nil {
				return false
			}
		}
		return true
	})
}

func zzProbeName(table []string, ordinal uint64) string {
	if ordinal < uint64(len(table)) {
		return table[ordinal]
	}
	return strconv.FormatUint(ordinal, 10)
}

func zzProbeRegisterSite(table map[identity.ContentID]string, site identity.ContentID, name string, ok bool) {
	if !ok || !site.Available() {
		return
	}
	if _, taken := table[site]; taken {
		return
	}
	table[site] = name
}

func zzProbeBuildSiteTable() map[identity.ContentID]string {
	table := make(map[identity.ContentID]string, 1<<18)
	for family := uint64(1); family < uint64(len(zzProbeFamilyNames)); family++ {
		prefix := zzProbeFamilyNames[family] + ":"
		for _, site := range zzProbeBoundarySites {
			for transport := uint64(0); transport <= 13; transport++ {
				name := prefix + site
				if transport != 0 {
					name += "+transport" + strconv.FormatUint(transport, 10)
				}
				digest, ok := zzProbeBoundaryDigest(family, site, transport)
				zzProbeRegisterSite(table, digest, name, ok)
			}
		}
	}
	// compile / "program-seal": the published seal stage, alone and refused at
	// one construction step.
	for stage := uint64(1); stage < uint64(len(zzProbeSealStageNames)); stage++ {
		digest, ok := zzProbeAuthorityDigest(1, "program-seal", stage)
		zzProbeRegisterSite(table, digest, "seal-stage:"+zzProbeSealStageNames[stage], ok)
		for step := uint64(0); step < uint64(len(zzProbeConstructionStepNames)); step++ {
			digest, ok := zzProbeAuthorityDigest(1, "program-seal", stage, step)
			zzProbeRegisterSite(table, digest,
				"construct:"+zzProbeSealStageNames[stage]+"/"+zzProbeConstructionStepNames[step], ok)
		}
	}
	// compile / "runtime-program-seal": one ordered runtime seal phase.
	for phase := uint64(0); phase < 64; phase++ {
		digest, ok := zzProbeAuthorityDigest(1, "runtime-program-seal", phase)
		zzProbeRegisterSite(table, digest, "runtime-seal-phase:"+strconv.FormatUint(phase, 10), ok)
	}
	// compile / "program-assembly": one lowering boundary.
	for failure := uint64(1); failure < uint64(len(zzProbeAssemblyNames)); failure++ {
		digest, ok := zzProbeAuthorityDigest(1, "program-assembly", failure)
		zzProbeRegisterSite(table, digest, "assembly:"+zzProbeAssemblyNames[failure], ok)
	}
	// observation / "observation-seal": one optional-observation predicate.
	for failure := uint64(1); failure < uint64(len(zzProbeObservationSealNames)); failure++ {
		digest, ok := zzProbeAuthorityDigest(5, "observation-seal", failure)
		zzProbeRegisterSite(table, digest, "observation-seal:"+zzProbeObservationSealNames[failure], ok)
	}
	zzProbeAddDeclarationSealSites(table)
	return table
}

// zzProbeAddDeclarationSealSites mints the five-ordinal declaration seal
// boundary: phase, row ordinal, the equation source family and leading site
// bytes, and the artifact row predicate. Only the Sources phase carries a
// source boundary and only the ArtifactRows phase an artifact predicate, so
// the enumerated space stays the space the engine can actually mint.
func zzProbeAddDeclarationSealSites(table map[identity.ContentID]string) {
	register := func(phase, ordinal, sourceFamily, sourceSite, artifact uint64, name string) {
		digest, ok := zzProbeAuthorityDigest(1, "program-seal", phase, ordinal, sourceFamily, sourceSite, artifact)
		zzProbeRegisterSite(table, digest, name, ok)
	}
	familyNames := map[uint64]string{1: "source", 2: "topology", 3: "compile", 4: "identity"}
	type equationSource struct {
		family, leading uint64
		name            string
	}
	sources := make([]equationSource, 0, 64)
	for family, sites := range zzProbeEquationSites {
		for _, site := range sites {
			digest, ok := zzProbeFramed(zzProbeEquationSiteDomain, zzProbeEquationSiteVersion,
				func(writer *canonical.DigestWriter) bool {
					return writer.Uint(family) == nil && writer.Bytes([]byte(site)) == nil
				})
			if !ok {
				continue
			}
			leading := uint64(0)
			for index := 0; index < 8; index++ {
				leading = leading<<8 | uint64(digest[index])
			}
			sources = append(sources, equationSource{family: family, leading: leading, name: familyNames[family] + "." + site})
		}
	}
	for phase := uint64(1); phase < uint64(len(zzProbeSealPhaseNames)); phase++ {
		phaseName := "declaration-seal:" + zzProbeSealPhaseNames[phase]
		for ordinal := uint64(0); ordinal < zzProbeSealRowOrdinalBound; ordinal++ {
			suffix := "#" + strconv.FormatUint(ordinal, 10)
			register(phase, ordinal, 0, 0, 0, phaseName+suffix)
			if phase == 1 {
				for _, source := range sources {
					register(phase, ordinal, source.family, source.leading, 0,
						phaseName+suffix+"/equation:"+source.name)
				}
			}
			if phase == 2 {
				for artifact := uint64(1); artifact < uint64(len(zzProbeArtifactRowNames)); artifact++ {
					register(phase, ordinal, 0, 0, artifact,
						phaseName+suffix+"/artifact:"+zzProbeArtifactRowNames[artifact])
				}
			}
		}
	}
}

// zzProbeDecodeSite names one opaque boundary identity, or reports its hex
// prefix when the identity is outside the enumerated declaration space.
func zzProbeDecodeSite(site identity.ContentID) string {
	if !site.Available() {
		return "unavailable"
	}
	zzProbeSiteOnce.Do(func() { zzProbeSiteTable = zzProbeBuildSiteTable() })
	if name, known := zzProbeSiteTable[site]; known {
		return name
	}
	return "undecoded:" + hex.EncodeToString(site[:6])
}

// TestZZProbeSolverLadderSiteTable prints the decoder's own inventory, so the
// enumerated boundary space is auditable without a corpus walk.
func TestZZProbeSolverLadderSiteTable(t *testing.T) {
	zzProbeSiteOnce.Do(func() { zzProbeSiteTable = zzProbeBuildSiteTable() })
	t.Logf("ZZPROBE site table entries=%d", len(zzProbeSiteTable))
}

// zzProbeDumpLine is the per-fixture dump record: one line, key=value fields,
// the failure signature last.
func zzProbeDumpLine(sample zzProbeSolverSample) string {
	engineDigest := sample.signature
	return fmt.Sprintf(
		"fixture=%s\tclass=%s\tstatus=%s\terr=%t\tsolve=%s\tcompile=%s\tseal=%s\tepochs=%d\tpasses=%d\tevaluates=%d\tfails=%d\tfolds=%d\trestarts=%d\tsig=%s\terrtext=%s",
		sample.name, zzProbeOr(sample.class, "-"), zzProbeOr(sample.status, "-"), sample.err,
		sample.solve.Round(time.Microsecond), sample.compile.Round(time.Microsecond), sample.seal.Round(time.Microsecond),
		sample.epochs, sample.passes, sample.evaluates, sample.evaluateFailures, sample.folds, sample.restarts,
		engineDigest, zzProbeOr(sample.errText, "-"))
}

func zzProbeOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// TestZZProbeSolverLadderWalk walks the corpus serially and reports the
// fixtures whose solve costs the most, with the per-class operation volumes
// that cost is made of. ZZPROBE_SHARD selects a fixture-name prefix;
// ZZPROBE_TOP sets how many rows are printed (default 30).
func TestZZProbeSolverLadderWalk(t *testing.T) {
	prefix := os.Getenv("ZZPROBE_SHARD")
	only := make(map[string]bool)
	for _, name := range strings.Split(os.Getenv("ZZPROBE_ONLY"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			only[name] = true
		}
	}
	projects := corpusHarnessProjects(t)
	selected := make([]corpusHarnessProject, 0, len(projects))
	for _, project := range projects {
		if len(only) != 0 {
			if only[project.name] {
				selected = append(selected, project)
			}
			continue
		}
		if prefix == "" || strings.HasPrefix(project.name, prefix) {
			selected = append(selected, project)
		}
	}
	if len(selected) == 0 {
		t.Fatalf("ZZPROBE_SHARD=%q ZZPROBE_ONLY=%q selects no fixture", prefix, os.Getenv("ZZPROBE_ONLY"))
	}
	if len(only) != 0 && len(selected) != len(only) {
		t.Fatalf("ZZPROBE_ONLY names %d fixtures, the corpus enumerates %d of them", len(only), len(selected))
	}
	top := 30
	if value, err := strconv.Atoi(os.Getenv("ZZPROBE_TOP")); err == nil && value > 0 {
		top = value
	}

	// ZZPROBE_DUMP writes one record per selected fixture, in corpus order, as
	// the walk produces it. Writing incrementally is what makes a killed walk
	// still report the fixtures it reached.
	var dump *bufio.Writer
	if path := os.Getenv("ZZPROBE_DUMP"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("ZZPROBE_DUMP=%q: %v", path, err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				t.Errorf("close ZZPROBE_DUMP: %v", err)
			}
		}()
		dump = bufio.NewWriter(file)
	}

	samples := make([]zzProbeSolverSample, 0, len(selected))
	for _, project := range selected {
		sample := zzProbeSolverSampleOne(t, project)
		samples = append(samples, sample)
		if dump == nil {
			continue
		}
		if _, err := dump.WriteString(zzProbeDumpLine(sample) + "\n"); err != nil {
			t.Fatalf("write ZZPROBE_DUMP: %v", err)
		}
		if err := dump.Flush(); err != nil {
			t.Fatalf("flush ZZPROBE_DUMP: %v", err)
		}
	}

	var totals zzProbeSolverSample
	failures := 0
	for _, sample := range samples {
		if sample.err {
			failures++
			continue
		}
		totals.solve += sample.solve
		totals.compile += sample.compile
		totals.passes += sample.passes
		totals.refreshes += sample.refreshes
		totals.evaluates += sample.evaluates
		totals.folds += sample.folds
		totals.rhs += sample.rhs
		totals.restarts += sample.restarts
		totals.publications += sample.publications
		totals.bumps += sample.bumps
		totals.ifaceRefresh += sample.ifaceRefresh
		totals.dbgFoldTerms += sample.dbgFoldTerms
		totals.reuseAdmit += sample.reuseAdmit
		totals.reuseRefuse += sample.reuseRefuse
		totals.reuseTerms += sample.reuseTerms
		totals.rebuildTerm += sample.rebuildTerm
		totals.mergeMany += sample.mergeMany
		totals.cells += sample.cells
		totals.cellPairs += sample.cellPairs
		totals.regionsTotal += sample.regionsTotal
		totals.regionsWidenFactorFree += sample.regionsWidenFactorFree
		totals.regionsPureTransport += sample.regionsPureTransport
		totals.regionsLinearCandidate += sample.regionsLinearCandidate
		totals.backEnvTerms += sample.backEnvTerms
		totals.backFactorTerms += sample.backFactorTerms
		totals.backGroupTerms += sample.backGroupTerms
		totals.regionInteriorPointsTotal += sample.regionInteriorPointsTotal
		if sample.regionInteriorPointsMax > totals.regionInteriorPointsMax {
			totals.regionInteriorPointsMax = sample.regionInteriorPointsMax
		}
	}

	sort.SliceStable(samples, func(left, right int) bool { return samples[left].solve > samples[right].solve })
	t.Logf("ZZPROBE walk shard=%q fixtures=%d solved=%d failed=%d", prefix, len(samples), len(samples)-failures, failures)
	for index, sample := range samples {
		if index >= top {
			break
		}
		t.Logf("ZZPROBE row %s", zzProbeSolverLine(sample))
	}
	t.Logf("ZZPROBE totals solve=%s compile=%s passes=%d refreshes=%d evaluates=%d folds=%d rhs=%d restarts=%d pubs=%d bumps=%d iface=%d foldTerms=%d reuse=%d/%d reuseTerms=%d rebuildTerms=%d mergeMany=%d cells=%d cellPairs=%d "+
		"regions=%d widenFree=%d pureTransport=%d linearCandidate=%d back{env=%d factor=%d group=%d} interior{max=%d total=%d}",
		totals.solve.Round(time.Millisecond), totals.compile.Round(time.Millisecond), totals.passes, totals.refreshes, totals.evaluates,
		totals.folds, totals.rhs, totals.restarts, totals.publications, totals.bumps, totals.ifaceRefresh, totals.dbgFoldTerms,
		totals.reuseAdmit, totals.reuseRefuse, totals.reuseTerms, totals.rebuildTerm, totals.mergeMany, totals.cells, totals.cellPairs,
		totals.regionsTotal, totals.regionsWidenFactorFree, totals.regionsPureTransport, totals.regionsLinearCandidate,
		totals.backEnvTerms, totals.backFactorTerms, totals.backGroupTerms,
		totals.regionInteriorPointsMax, totals.regionInteriorPointsTotal)
}

// TestZZProbeSolverLadderFixture runs exactly one named fixture so the global
// counters, including the typprobe cell-pair distinctness map printed by the
// oracle TestMain, are attributable to it. Set ZZPROBE_FIXTURE.
func TestZZProbeSolverLadderFixture(t *testing.T) {
	name := os.Getenv("ZZPROBE_FIXTURE")
	if name == "" {
		t.Skip("set ZZPROBE_FIXTURE to a canonical fixture name")
	}
	sample := zzProbeSolverSampleOne(t, corpusHarnessFixture(t, name))
	t.Logf("ZZPROBE fixture %s", zzProbeSolverLine(sample))
	t.Logf("ZZPROBE dump %s", zzProbeDumpLine(sample))
}
