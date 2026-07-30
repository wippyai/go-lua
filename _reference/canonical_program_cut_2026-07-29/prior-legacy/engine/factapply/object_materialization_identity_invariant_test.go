package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func materializationIdentityTestID(index uint64) identity.ID {
	return identity.ID{Kind: "table", Site: "materialization-invariant", Index: index}
}

func TestMaterializationPlannersRejectIdentitylessLiteralGraphs(t *testing.T) {
	point := cfg.Point(811)
	rootRef, nestedRef := factflow.ExprRef(812), factflow.ExprRef(813)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: rootRef, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: nestedRef, HasExpr: true}
	rootLiteral := factflow.NewObjectLiteral([]factflow.ObjectEntry{
		factflow.NewObjectEntryWithMetadata(fieldSuffix("child"), nestedSource, factflow.SourceSpan{}, ""),
	}).WithIdentity(materializationIdentityTestID(uint64(rootRef)))
	identitylessNested := factflow.NewObjectLiteral(nil)
	objects := map[factflow.ExprRef]factflow.ObjectLiteral{
		rootRef:   rootLiteral,
		nestedRef: identitylessNested,
	}

	t.Run("root assignment", func(t *testing.T) {
		target := symbol.ID(814)
		facts := factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), rootSource),
			},
			ObjectLiterals: objects,
		})
		if _, ok := PlanRootAssignmentTransaction(facts, point); ok {
			t.Fatal("identityless nested literal admitted to root-assignment materialization")
		}
	})

	t.Run("path assignment", func(t *testing.T) {
		target := symbol.ID(815)
		facts := factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(pathdom.NewPath(target, "value").Field("child"), rootSource),
			},
			ObjectLiterals: objects,
		})
		if _, ok := PlanPathStoreTransaction(facts, point); ok {
			t.Fatal("identityless nested literal admitted to path-assignment materialization")
		}
	})

	t.Run("static write", func(t *testing.T) {
		target := symbol.ID(816)
		facts := factflow.NewFacts(factflow.FactsInput{
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(pathdom.NewPath(target, "value").Field("child"), rootSource),
			},
			ObjectLiterals: objects,
		})
		if _, ok := PlanPathStoreTransaction(facts, point); ok {
			t.Fatal("identityless nested literal admitted from static-write source")
		}
	})

	t.Run("return", func(t *testing.T) {
		facts := factflow.NewFacts(factflow.FactsInput{
			Returns:        map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{rootSource})},
			ObjectLiterals: objects,
		})
		if _, ok := PlanReturnTransaction(facts, point); ok {
			t.Fatal("identityless nested literal admitted to return materialization")
		}
	})
}

func TestMaterializationPlannersRejectCyclesAndAcceptSharedDAGs(t *testing.T) {
	point := cfg.Point(821)
	rootRef, nestedRef := factflow.ExprRef(822), factflow.ExprRef(823)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: rootRef, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: nestedRef, HasExpr: true}
	rootID := materializationIdentityTestID(uint64(rootRef))
	nestedID := materializationIdentityTestID(uint64(nestedRef))
	sharedDAG := map[factflow.ExprRef]factflow.ObjectLiteral{
		rootRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
			factflow.NewObjectEntryWithMetadata(fieldSuffix("left"), nestedSource, factflow.SourceSpan{}, ""),
			factflow.NewObjectEntryWithMetadata(fieldSuffix("right"), nestedSource, factflow.SourceSpan{}, ""),
		}).WithIdentity(rootID),
		nestedRef: factflow.NewObjectLiteral(nil).WithIdentity(nestedID),
	}
	cycle := map[factflow.ExprRef]factflow.ObjectLiteral{
		rootRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
			factflow.NewObjectEntryWithMetadata(fieldSuffix("self"), rootSource, factflow.SourceSpan{}, ""),
		}).WithIdentity(rootID),
	}

	planners := []struct {
		name string
		run  func(map[factflow.ExprRef]factflow.ObjectLiteral) bool
	}{
		{
			name: "root assignment",
			run: func(objects map[factflow.ExprRef]factflow.ObjectLiteral) bool {
				target := symbol.ID(824)
				facts := factflow.NewFacts(factflow.FactsInput{
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "value"), rootSource),
					},
					ObjectLiterals: objects,
				})
				_, ok := PlanRootAssignmentTransaction(facts, point)
				return ok
			},
		},
		{
			name: "path assignment",
			run: func(objects map[factflow.ExprRef]factflow.ObjectLiteral) bool {
				target := symbol.ID(825)
				facts := factflow.NewFacts(factflow.FactsInput{
					PathAssignments: map[cfg.Point]factflow.PathAssignment{
						point: factflow.NewPathAssignment(pathdom.NewPath(target, "value").Field("child"), rootSource),
					},
					ObjectLiterals: objects,
				})
				_, ok := PlanPathStoreTransaction(facts, point)
				return ok
			},
		},
		{
			name: "return",
			run: func(objects map[factflow.ExprRef]factflow.ObjectLiteral) bool {
				facts := factflow.NewFacts(factflow.FactsInput{
					Returns:        map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{rootSource})},
					ObjectLiterals: objects,
				})
				_, ok := PlanReturnTransaction(facts, point)
				return ok
			},
		},
	}
	for _, planner := range planners {
		t.Run(planner.name, func(t *testing.T) {
			if !planner.run(sharedDAG) {
				t.Fatal("shared acyclic object graph was rejected")
			}
			if planner.run(cycle) {
				t.Fatal("cyclic object graph was admitted")
			}
		})
	}
}
