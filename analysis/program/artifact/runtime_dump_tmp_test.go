package artifact_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func id3(id [32]byte) [3]byte { return [3]byte{id[0], id[1], id[2]} }

func TestRuntimeDumpTmp(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root, _ := testfixture.RepositoryRoot(filepath.Dir(current))
	corpus, _ := testfixture.LoadCorpus(root)
	project, err := corpus.Project("advice/redundant-guard")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}
	mounts := linked.Project().Mounts()
	grammar, _ := composite.Global()
	for i := 0; i < mounts.Count(); i++ {
		shard, _ := mounts.At(i)
		p, _ := mounts.Program(shard)
		module, _ := linked.Project().ModuleKey(shard)
		a, f := composite.CompileArtifactDetailed(p, grammar)
		if f.Available() {
			t.Fatal(f.Error())
		}
		fmt.Printf("MOUNT %x artifact=%x points=%d edges=%d calls=%d occ=%d rules=%d\n", id3(module), id3(a.ID()), a.PointCount(), a.EnvironmentEdgeCount(), a.CallCount(), a.OccurrenceCount(), a.RulePlacementCount())
		for j := 0; j < a.CallCount(); j++ {
			c, _ := a.CallAt(j)
			x, _ := c.DirectTargetBody()
			fmt.Printf("CALL id=%x form=%d span=%x args=%d target=%x\n", id3(c.ID()), c.Form(), id3(c.SpanID()), c.ArgumentCount(), id3(x))
		}
		for j := 0; j < a.EnvironmentEdgeCount(); j++ {
			e, _ := a.EnvironmentEdgeAt(j)
			cond, _ := e.ConditionValueSpanID()
			guard, _ := e.GuardID()
			tr, _ := e.Truth()
			fmt.Printf("EDGE id=%x from=%x to=%x route=%x cond=%x guard=%x truth=%t arm=%d\n", id3(e.ID()), id3(e.From()), id3(e.To()), id3(e.RouteID()), id3(cond), id3(guard), tr, e.Arm())
		}
		for j := 0; j < a.OccurrenceCount(); j++ {
			o, _ := a.OccurrenceAt(j)
			fmt.Printf("OCC kind=%d id=%x code=%d inputs=%d\n", o.Kind(), id3(o.ID()), o.Code(), o.InputCount())
		}
		for j := 0; j < a.OccurrenceCount(); j++ {
			o, _ := a.OccurrenceAt(j)
			if o.Kind() == programartifact.OccurrenceBinaryEquality {
				l, r, op, _ := o.BinaryEquality()
				fmt.Printf("EQ id=%x left=%x right=%x op=%d\n", id3(o.ID()), id3(l), id3(r), op)
			}
			if o.Kind() == programartifact.OccurrenceValueSource {
				s, _ := o.ValueSourceSpanID()
				fmt.Printf("SRC id=%x span=%x lit=%v\n", id3(o.ID()), id3(s), func() string {
					_, x, ok := o.Literal()
					if !ok {
						return ""
					}
					return fmt.Sprint(x)
				}())
			}
		}
		for j := 0; j < a.RulePlacementCount(); j++ {
			r, _ := a.RulePlacementAt(j)
			point, _ := r.PointAt(0)
			inp, _ := r.InputPoint()
			route, _ := r.PredecessorRouteID()
			fmt.Printf("RULE key=%s id=%x point=%x input=%x kind=%d stage=%d route=%x\n", r.Key(), id3(r.ID()), id3(point), id3(inp), r.InputKind(), r.Stage(), id3(route))
		}
	}
	_ = lower.Source{}
	_ = link.Spec{}
	_ = linkproject.Module{}
}
