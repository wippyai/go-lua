package analysis

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	typeauthority "github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/testfixture"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

// diagStaticFailure replicates the static plane construction in
// newProgramBinding with per-step reporting.
func diagStaticFailure(state *compiledState, source *link.Link) string {
	if state == nil || source == nil || state.artifacts == nil || len(state.artifacts.mounts) == 0 {
		return "input"
	}
	coldMounts, coldMountsOK := constructionMountedArtifacts(source, state.artifacts.mounts)
	if !coldMountsOK {
		return "cold-mounts"
	}
	artifactTypes := make([]*programartifact.Artifact, 0, len(state.artifacts.byProgram))
	for _, a := range state.artifacts.byProgram {
		if a == nil || !a.Available() {
			return "artifact-unavailable"
		}
		artifactTypes = append(artifactTypes, a)
	}
	types, typesErr := typeauthority.SealArtifactRows(state.sourceID, artifactTypes)
	if typesErr != nil {
		return "seal-artifact-rows: " + typesErr.Error()
	}
	if os.Getenv("ZZ_ROWS") != "" {
		artifactAuthority, artifactErr := typeauthority.SealArtifacts(artifactTypes)
		if artifactErr != nil {
			return "seal-artifacts: " + artifactErr.Error()
		}
		report := make([]string, 0)
		known := make(map[identity.ContentID]programartifact.StaticTypeNodeRow)
		for _, item := range artifactTypes {
			for i := 0; i < item.StaticTypeNodeCount(); i++ {
				if row, rowOK := item.StaticTypeNodeAt(i); rowOK {
					known[row.ID()] = row
				}
			}
		}
		for _, item := range artifactTypes {
			for i := 0; i < item.StaticTypeNodeCount(); i++ {
				row, rowOK := item.StaticTypeNodeAt(i)
				if !rowOK {
					continue
				}
				edges := make([]string, 0)
				for c := 0; c < row.ChildCount(); c++ {
					id, idOK := row.ChildAt(c)
					child, present := known[id]
					_, childRaw := artifactAuthority.Resolve(id)
					edges = append(edges, fmt.Sprintf("child%d[ok=%t present=%t kind=%v raw=%t]", c, idOK, present, child.Kind(), childRaw))
				}
				_ = edges
				ref, refOK := types.FindByReferenceID(row.ID())
				raw, rawOK := artifactAuthority.Resolve(row.ID())
				_, resolveOK := types.Resolve(ref)
				var validateErr error
				if rawOK {
					validateErr = typ.ValidateStaticGenericRecurrence(raw)
				}
				if !refOK || !rawOK || !resolveOK {
					report = append(report, fmt.Sprintf("row[%d] kind=%v name=%q res=%d ref=%t raw=%t resolve=%t validate=%v %s", i, row.Kind(), row.Name(), row.Resolution(), refOK, rawOK, resolveOK, validateErr, strings.Join(edges, " ")))
				}
			}
		}
		if len(report) != 0 {
			return "rows:\n  " + strings.Join(report, "\n  ")
		}
		return "rows: all resolve"
	}
	staticMounts := make([]staticdomain.MountedArtifact, len(coldMounts))
	staticValueIDs := make([]staticdomain.MountedValueID, 0)
	staticValues := source.Boundary().Values()
	seen := make(map[[2]identity.ContentID]struct{})
	for index, mounted := range coldMounts {
		published := mounted.published
		if published.artifact == nil || !published.artifact.Available() || !published.moduleKey.Available() || !published.programID.Available() {
			return "mount-unavailable"
		}
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: published.artifact, ModuleID: published.moduleKey, ProgramID: published.programID, NamespaceID: published.moduleKey}
		for rowIndex := 0; rowIndex < published.artifact.StaticTypeValueCount(); rowIndex++ {
			row, rowOK := published.artifact.StaticTypeValueAt(rowIndex)
			if !rowOK || !row.Available() {
				return "row-unavailable"
			}
			key := [2]identity.ContentID{published.moduleKey, row.ID()}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Sprintf("duplicate-row mount=%d row=%d/%d", index, rowIndex, published.artifact.StaticTypeValueCount())
			}
			value, valueOK := staticValues.ForMountedSemantic(published.moduleKey, row.ID())
			valueID, valueIDOK := staticValues.ID(value)
			if !valueOK {
				return fmt.Sprintf("no-mounted-semantic mount=%d row=%d/%d id=%v", index, rowIndex, published.artifact.StaticTypeValueCount(), row.ID())
			}
			if !valueIDOK || !valueID.Available() {
				return fmt.Sprintf("no-value-id mount=%d row=%d", index, rowIndex)
			}
			seen[key] = struct{}{}
			staticValueIDs = append(staticValueIDs, staticdomain.MountedValueID{ModuleID: published.moduleKey, SemanticID: row.ID(), ValueID: valueID})
		}
	}
	staticTarget, staticTargetOK := source.Boundary().Target()
	if !staticTargetOK {
		return "target"
	}
	static, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: state.sourceID, Target: staticTarget, ValueIDs: staticValueIDs}, types, staticMounts)
	if err != nil {
		return "seal: " + err.Error()
	}
	if static == nil {
		return "seal: nil authority"
	}
	return ""
}

func diagCompiledState(source *link.Link) (*compiledState, string) {
	receipt, receiptOK := grammar.Global()
	if !receiptOK || !receipt.Available() {
		return nil, "grammar"
	}
	artifacts, artifactsOK := compileProgramArtifacts(source, receipt)
	if !artifactsOK {
		return nil, "artifacts"
	}
	values, valuesOK := compileValueCoordinates(source)
	if !valuesOK {
		return nil, "values"
	}
	observations, observationsOK := compileDiagnosticObservations(source, artifacts, values)
	if !observationsOK {
		return nil, "observations"
	}
	resultReceipt, resultReceiptOK := compileArtifactResultReceipt(source.ContentID(), artifacts.mounts, values, observations)
	if !resultReceiptOK {
		return nil, "result-receipt"
	}
	state := &compiledState{artifacts: artifacts, resultReceipt: resultReceipt, receipt: receipt, sourceID: source.ContentID()}
	state.lifecycleCond = sync.NewCond(&state.lifecycleMu)
	if !state.admit() {
		state.release()
		return nil, "admit"
	}
	return state, ""
}

func TestZZClassBStaticDiag(t *testing.T) {
	prefix := os.Getenv("ZZ_PREFIX")
	contract := corpusHarnessContract(t)
	for _, project := range corpusHarnessProjects(t) {
		if prefix != "" && !strings.HasPrefix(project.name, prefix) {
			continue
		}
		linked, err := testfixture.SealCorpusProject(contract, project.source)
		if err != nil {
			t.Logf("STATIC-DIAG %s link-error %v", project.name, err)
			continue
		}
		plan, status, _ := CompileWithDiagnostics(linked)
		if status != CompileComplete || plan == nil {
			state, stateErr := diagCompiledState(linked)
			if stateErr != "" {
				t.Logf("STATIC-DIAG %s state=%s", project.name, stateErr)
				continue
			}
			t.Logf("STATIC-DIAG %s => %s", project.name, diagStaticFailure(state, linked))
			state.release()
			continue
		}
		plan.Close()
	}
}
