package zzverify

import (
	"fmt"
	"os"
	"strings"
	"testing"

	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	typeauthority "github.com/wippyai/go-lua/analysis/domain/type/authority"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/testfixture"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

func TestZZStaticSealProbe(t *testing.T) {
	prefix := os.Getenv("ZZ_PREFIX")
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	projects, err := testfixture.FrozenCorpusProjects()
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := grammar.Global()
	if !receiptOK {
		t.Fatal("grammar unavailable")
	}
	for _, project := range projects {
		if prefix != "" && !strings.HasPrefix(project.Name(), prefix) {
			continue
		}
		linked, sealErr := testfixture.SealCorpusProject(contract, project)
		if sealErr != nil {
			t.Logf("ZZSTATIC %s link %v", project.Name(), sealErr)
			continue
		}
		mounts := linked.Project().Mounts()
		artifacts := make([]*programartifact.Artifact, 0, mounts.Count())
		staticMounts := make([]staticdomain.MountedArtifact, 0, mounts.Count())
		valueIDs := make([]staticdomain.MountedValueID, 0)
		values := linked.Boundary().Values()
		failed := false
		for index := 0; index < mounts.Count(); index++ {
			shard, shardOK := mounts.At(index)
			mounted, mountedOK := mounts.Program(shard)
			module, moduleOK := linked.Project().ModuleKey(shard)
			if !shardOK || !mountedOK || !moduleOK {
				t.Logf("ZZSTATIC %s mount %d unavailable", project.Name(), index)
				failed = true
				break
			}
			artifact, compiled := schemaadapter.Compile(mounted.TransformerInput(), receipt)
			if !compiled || artifact == nil {
				t.Logf("ZZSTATIC %s artifact %d uncompiled", project.Name(), index)
				failed = true
				break
			}
			artifacts = append(artifacts, artifact)
			staticMounts = append(staticMounts, staticdomain.MountedArtifact{
				Artifact: artifact, ModuleID: module, ProgramID: mounted.TransformerInput().ContentID(), NamespaceID: module,
			})
			for rowIndex := 0; rowIndex < artifact.StaticTypeValueCount(); rowIndex++ {
				row, rowOK := artifact.StaticTypeValueAt(rowIndex)
				if !rowOK {
					continue
				}
				value, valueOK := values.ForMountedSemantic(module, row.ID())
				valueID, idOK := values.ID(value)
				if !valueOK || !idOK {
					t.Logf("ZZSTATIC %s value row %d unavailable", project.Name(), rowIndex)
					failed = true
					break
				}
				valueIDs = append(valueIDs, staticdomain.MountedValueID{ModuleID: module, SemanticID: row.ID(), ValueID: valueID})
			}
		}
		if failed {
			continue
		}
		types, typesErr := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
		if typesErr != nil {
			t.Logf("ZZSTATIC %s types %v", project.Name(), typesErr)
			continue
		}
		target, targetOK := linked.Boundary().Target()
		if !targetOK {
			t.Logf("ZZSTATIC %s target unavailable", project.Name())
			continue
		}
		_, _, sealError := staticdomain.SealMountedArtifacts(staticdomain.MountContext{
			LinkID: linked.ContentID(), Target: target, ValueIDs: valueIDs,
		}, types, staticMounts)
		if sealError != nil {
			t.Logf("ZZSTATIC %s seal %v", project.Name(), sealError)
		}
	}
	_ = fmt.Sprint(identity.ContentID{})
}
