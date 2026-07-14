package service

import (
	"context"
	"sync"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Imported manifests are decoded once and intentionally shared by concurrent
// solves. Recursive type nodes in that immutable graph must therefore publish
// their derived caches without racing.
func TestRecursiveImportedManifestConcurrentSolve(t *testing.T) {
	m := manifest.New("example/recursive")
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return &typ.Record{Fields: []typ.Field{{Name: "next", Type: self}}}
	})
	m.DefineGlobalType("imported_value", node)
	m.DefineFunctionSignature("transform", signature.Function{
		Type: typ.Func().Param("value", node).Returns(node).Build(),
		OperationalEffects: &signature.OperationalEffects{
			SuspensionKnown: true,
			MaySuspend:      true,
			ReturnPresenceRelations: []signature.ReturnPresenceRelation{{
				TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Present(),
			}},
			NormalReturnTypeRefinements: []signature.PathTypeRefinement{{
				Path: pathdom.NewPlaceholder(0).Field("next"), Type: node, Assertion: assertion.Runtime(),
			}},
			EscapeEvents: []signature.EscapeEvent{{
				Target: pathdom.NewPlaceholder(0), Kind: signature.EscapeSend, Recursive: true,
			}},
		},
	})
	input := NewUnitInputFromFiles("recursive-manifest-race", "example/recursive-manifest-race", "main.lua", map[string][]byte{
		"main.lua": []byte("return imported_value\n"),
	})
	input.ExternalManifests = map[string]*manifest.Manifest{"recursive": m}
	session := NewBatchSession()
	if _, err := session.UpsertUnit(context.Background(), input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := session.EnsureSolved(context.Background(), SolveRequest{
				UnitID: input.ID, Freshness: FreshnessRequireNew,
			}); err != nil {
				t.Errorf("EnsureSolved: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
}
