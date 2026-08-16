package equation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// relationLawTopology seals one topology carrying a single admissible
// activation receipt, so its publications can be advanced.
func relationLawTopology(t testing.TB) (*Topology, AcceptedMember) {
	t.Helper()
	fixture := newTemplateMaterializationFixture(t)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("relation law materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Points:           []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
	})
	if !sealed || topology == nil {
		t.Fatal("relation law topology seal")
	}
	key := boundaryKey(91)
	receipt := activationReceipt{key: key, family: boundaryKey(92), trigger: boundaryKey(93), application: boundaryKey(94), target: boundaryKey(95), endpoint: boundaryKey(96)}
	topology.receipts = []activationReceipt{receipt}
	topology.receiptAt = map[composition.Key]int{key: 0}
	topology.receiptByTrigger = map[composition.Key][]int{receipt.trigger: {0}}
	member, memberOK := topology.SelectReceiptMember(receipt.trigger, PairLocator{Application: receipt.application, Target: receipt.target, Endpoint: receipt.endpoint})
	if !memberOK {
		t.Fatal("relation law receipt member")
	}
	accepted, acceptedOK := topology.Accept(member, TrueExpr())
	if !acceptedOK {
		t.Fatal("relation law accepted member")
	}
	return topology, accepted
}

// TestPublicationStampsAdvanceMonotonicallyAndOrderTotally proves the stamp
// discipline of one store: the first publication is available, every Publish
// advances by exactly one Generation, and Precedes is a strict total order on
// the publications of that one Topology.
func TestPublicationStampsAdvanceMonotonicallyAndOrderTotally(t *testing.T) {
	topology, accepted := relationLawTopology(t)
	base, baseOK := topology.InitialRelation()
	if !baseOK || !base.Available() || !base.Generation().Available() {
		t.Fatal("sealed topology lacked its first publication")
	}
	next, nextOK := topology.Publish(base, []AcceptedMember{accepted})
	if !nextOK || next.Generation() != base.Generation().Next() {
		t.Fatalf("publish did not advance exactly one generation: base=%d next=%d", base.Generation(), next.Generation())
	}
	third, thirdOK := topology.Publish(next, nil)
	if !thirdOK || third.Generation() != next.Generation().Next() {
		t.Fatalf("second publish did not advance exactly one generation: next=%d third=%d", next.Generation(), third.Generation())
	}
	stamps := []Relation{base, next, third}
	for leftIndex, left := range stamps {
		if left.Precedes(left) {
			t.Fatal("a publication preceded itself")
		}
		for rightIndex, right := range stamps {
			if leftIndex == rightIndex {
				continue
			}
			if left.Precedes(right) == right.Precedes(left) {
				t.Fatalf("publications %d and %d were not totally ordered", leftIndex, rightIndex)
			}
			if (leftIndex < rightIndex) != left.Precedes(right) {
				t.Fatalf("publication order disagreed with issue order at %d,%d", leftIndex, rightIndex)
			}
		}
	}
}

// TestPublicationRejectsForeignAndUnstampedRelations proves the stale fence at
// the publication boundary: a Relation of one Topology is never a predecessor,
// graph anchor, or ownership witness for another, and an unstamped Relation is
// rejected everywhere.
func TestPublicationRejectsForeignAndUnstampedRelations(t *testing.T) {
	topology, accepted := relationLawTopology(t)
	foreign, _ := relationLawTopology(t)
	base, baseOK := topology.InitialRelation()
	foreignBase, foreignBaseOK := foreign.InitialRelation()
	if !baseOK || !foreignBaseOK {
		t.Fatal("relation law publications")
	}
	if base.OwnedBy(foreign) || foreignBase.OwnedBy(topology) {
		t.Fatal("a publication claimed a foreign topology as its owner")
	}
	if _, published := topology.Publish(foreignBase, []AcceptedMember{accepted}); published {
		t.Fatal("a foreign predecessor advanced this topology")
	}
	if _, issued := topology.Graph(foreignBase); issued {
		t.Fatal("a foreign publication anchored a graph")
	}
	unstamped := Relation{}
	if unstamped.Available() || unstamped.Generation().Available() || unstamped.Digest().Available() || unstamped.Rows() != nil {
		t.Fatal("the zero relation named a publication")
	}
	if _, published := topology.Publish(unstamped, nil); published {
		t.Fatal("an unstamped predecessor advanced this topology")
	}
	if _, issued := topology.Graph(unstamped); issued {
		t.Fatal("an unstamped publication anchored a graph")
	}
	// A superseded publication remains a valid anchor: retained generations stay
	// addressable, they only stop being the latest.
	next, nextOK := topology.Publish(base, []AcceptedMember{accepted})
	if !nextOK {
		t.Fatal("relation law successor")
	}
	if _, issued := topology.Graph(base); !issued {
		t.Fatal("a retained publication lost its graph anchor")
	}
	if !base.Precedes(next) || next.Precedes(base) {
		t.Fatal("a superseded publication did not precede its successor")
	}
}

// TestPublishedDigestIsDerivedOnceAndCarried proves the split between stamp and
// content identity: the digest travels with the publication, and every reader
// (including graph issuance) compares the stored value instead of deriving one.
func TestPublishedDigestIsDerivedOnceAndCarried(t *testing.T) {
	topology, accepted := relationLawTopology(t)
	base, baseOK := topology.InitialRelation()
	if !baseOK {
		t.Fatal("relation law publication")
	}
	published, publishedOK := topology.Publish(base, []AcceptedMember{accepted})
	if !publishedOK || !published.Digest().Available() || published.Digest() == base.Digest() {
		t.Fatal("publishing an accepted member did not change content identity")
	}
	first, firstOK := topology.Graph(published)
	second, secondOK := topology.Graph(published)
	if !firstOK || !secondOK || first == nil || second == nil {
		t.Fatal("published graph")
	}
	if first.relation.Digest() != published.Digest() || second.relation.Digest() != published.Digest() {
		t.Fatal("graph issuance did not carry the publication digest")
	}
	if first.relation.Generation() != published.Generation() || second.relation.Generation() != published.Generation() {
		t.Fatal("graph issuance did not carry the publication stamp")
	}
	// Publishing the same rows from the same predecessor is deterministic in
	// both parts: same content identity, same stamp.
	repeat, repeatOK := topology.Publish(base, []AcceptedMember{accepted})
	if !repeatOK || repeat.Digest() != published.Digest() || repeat.Generation() != published.Generation() {
		t.Fatal("publication was not a deterministic function of predecessor and rows")
	}
}

// TestRelationDigestDerivationIsUnrepresentableOutsidePublication makes the
// derived-once rule structural rather than observational: the digest deriver is
// unexported, and the only calls to it in this package are the two publication
// points. No exported surface derives structural identity for caller-supplied
// rows, so a reader physically cannot re-derive what a Relation already holds.
func TestRelationDigestDerivationIsUnrepresentableOutsidePublication(t *testing.T) {
	set := token.NewFileSet()
	packages, err := parser.ParseDir(set, ".", func(entry fs.FileInfo) bool {
		return !strings.HasSuffix(entry.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse equation package: %v", err)
	}
	parsed, present := packages["equation"]
	if !present {
		t.Fatal("equation package source")
	}
	callers := map[string]int{}
	for _, file := range parsed.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, isFunc := node.(*ast.FuncDecl)
			if !isFunc {
				return true
			}
			ast.Inspect(declaration.Body, func(inner ast.Node) bool {
				call, isCall := inner.(*ast.CallExpr)
				if !isCall {
					return true
				}
				selector, isSelector := call.Fun.(*ast.SelectorExpr)
				if isSelector && selector.Sel.Name == "deriveRelationDigest" {
					callers[declaration.Name.Name]++
				}
				return true
			})
			return false
		})
	}
	if len(callers) != 2 || callers["Publish"] != 1 || callers["sealInitialRelation"] != 1 {
		t.Fatalf("structural digest is derived outside the publication points: %v", callers)
	}
	for _, file := range parsed.Files {
		for _, declaration := range file.Decls {
			function, isFunc := declaration.(*ast.FuncDecl)
			if !isFunc || !function.Name.IsExported() || function.Name.Name == "Publish" {
				continue
			}
			for _, parameter := range function.Type.Params.List {
				array, isArray := parameter.Type.(*ast.ArrayType)
				if !isArray {
					continue
				}
				name, isName := array.Elt.(*ast.Ident)
				if !isName || name.Name != "AcceptedMember" {
					continue
				}
				for _, result := range function.Type.Results.List {
					if selector, isSelector := result.Type.(*ast.SelectorExpr); isSelector && selector.Sel.Name == "Key" {
						t.Fatalf("exported %s derives a content key from caller-supplied rows", function.Name.Name)
					}
				}
			}
		}
	}
}

// TestGenerationStampsFenceOnlyTheirOwnStore records the one comparison rule
// the whole engine relies on: an unset stamp names no revision, and stamps are
// only meaningful beside the store identity that issued them.
func TestGenerationStampsFenceOnlyTheirOwnStore(t *testing.T) {
	var unset identity.Generation
	if unset.Available() || unset.Precedes(unset.Next()) || unset.Next().Precedes(unset) {
		t.Fatal("an unset generation participated in the revision order")
	}
	first := unset.Next()
	if !first.Available() || !first.Precedes(first.Next()) {
		t.Fatal("the first generation did not precede its successor")
	}
	saturated := ^identity.Generation(0)
	if saturated.Next().Available() {
		t.Fatal("a saturated store reused a live revision number")
	}
}
