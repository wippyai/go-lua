package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexWriteReadAdmissionFollowsAssignmentAlias(t *testing.T) {
	nodes := &ast.IdentExpr{Value: "nodes"}
	id := &ast.IdentExpr{Value: "id"}
	last := &ast.IdentExpr{Value: "last"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		nodes: cfg.SymbolID(501),
		id:    cfg.SymbolID(502),
		last:  cfg.SymbolID(503),
	})
	tr := New(in, Config{})
	nodesPath := constraint.NewPath(cfg.SymbolID(501), "nodes")
	idPath := constraint.NewPath(cfg.SymbolID(502), "id")
	lastPath := constraint.NewPath(cfg.SymbolID(503), "last")
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(501)): product.FromType(typ.NewMap(typ.String, typ.NewOptional(typ.Number))),
			flow.SymbolValueKey(cfg.SymbolID(503)): product.FromType(typ.String),
		},
		ValueOrigins: flow.ValueOriginFacts{}.WithPaths(lastPath, idPath, flow.ValueOriginAssignmentAlias, 0),
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target:  flow.IndexWriteAdmissionPathKey(nodesPath),
			KeyPath: flow.IndexWriteAdmissionPathKey(idPath),
			Key:     product.FromType(typ.String),
			Value:   product.FromType(typ.Number),
		}),
	}

	got, ok := tr.evalAttrGet(&out, &ast.AttrGetExpr{
		Object:    nodes,
		Key:       last,
		KeySyntax: ast.AttrKeyIndex,
	}, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("alias-backed index read = %v/%v, want number/true", got.ProjectValue(), ok)
	}
}

func TestSymbolicDynamicIndexWriteSeedsKeyPresenceAndReadback(t *testing.T) {
	nodesSym := cfg.SymbolID(511)
	id := &ast.IdentExpr{Value: "id"}
	idSym := cfg.SymbolID(512)
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		id: idSym,
	})
	tr := New(in, Config{})
	nodesPath := constraint.NewPath(nodesSym, "nodes")
	idPath := constraint.NewPath(idSym, "id")
	nodeType := typ.NewRecord().
		Field("node_id", typ.String).
		Field("status", typ.String).
		Build()
	out := flow.PointState{}

	changed := tr.applySymbolicDynamicIndexWriteProof(&out, cfg.AssignTarget{
		Kind:       cfg.TargetIndex,
		BaseName:   "nodes",
		BaseSymbol: nodesSym,
		Key:        id,
	}, nil, product.FromType(nodeType))

	if !changed {
		t.Fatal("symbolic dynamic-index write did not report a fact change")
	}
	if !out.KeyPresence.HasPaths(nodesPath, idPath) {
		t.Fatalf("symbolic dynamic-index write did not seed key presence: %s", out.KeyPresence.Format())
	}
	got, ok := out.IndexWrites.Admission(flow.IndexWriteQuery{
		Target:  nodesPath,
		KeyPath: idPath,
		KeyType: typ.String,
	})
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("symbolic dynamic-index readback = %v/%v, want node record/true", got.ProjectValue(), ok)
	}
}

func TestDynamicIndexWriteProofBuilderAllowsOpaqueExactKeyPathReadback(t *testing.T) {
	nodesSym := cfg.SymbolID(521)
	id := &ast.IdentExpr{Value: "id"}
	idSym := cfg.SymbolID(522)
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		id: idSym,
	})
	tr := New(in, Config{})
	nodesPath := constraint.NewPath(nodesSym, "nodes")
	idPath := constraint.NewPath(idSym, "id")
	out := flow.PointState{}
	payload := product.FromType(typ.NewRecord().Field("kind", typ.String).Build())

	proof, ok := tr.dynamicIndexWriteProofEffect(WriteEffect{
		Place: Place{
			Root:     nodesSym,
			RootName: "nodes",
			Steps: []PlaceStep{{
				Kind: PlaceStepDynamicIndex,
				Key:  product.FromType(typ.Any),
			}},
		},
		IndexTarget: cfg.AssignTarget{
			Kind:       cfg.TargetIndex,
			BaseName:   "nodes",
			BaseSymbol: nodesSym,
			Key:        id,
		},
		Value: payload,
	}, product.FromType(typ.Any), payload)
	if !ok {
		t.Fatal("dynamic index write proof was not constructed")
	}
	tr.applyDynamicIndexWriteProofEffect(&out, proof)

	got, ok := out.IndexWrites.Admission(flow.IndexWriteQuery{
		Target:  nodesPath,
		KeyPath: idPath,
		KeyType: typ.Any,
	})
	if !ok || !product.Domain.Equal(got, payload) {
		t.Fatalf("exact opaque key-path readback = %v/%v, want payload/true", got.ProjectValue(), ok)
	}
	if _, ok := out.IndexWrites.Admission(flow.IndexWriteQuery{
		Target:  nodesPath,
		KeyType: typ.LiteralString("other"),
	}); ok {
		t.Fatal("opaque key-path proof matched a pathless key-value query")
	}
}

func TestAssignmentProvenanceCopiesKeyPresence(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	tablePath := constraint.NewPath(cfg.SymbolID(621), "nodes")
	sourcePath := constraint.NewPath(cfg.SymbolID(622), "node_id")
	targetPath := constraint.NewPath(cfg.SymbolID(623), "last_id")
	valuePath := constraint.NewPath(cfg.SymbolID(624), "node")
	out := flow.PointState{
		KeyPresence: flow.KeyPresenceFacts{}.WithValuePaths(tablePath, sourcePath, valuePath),
	}

	changed := tr.applyAssignmentProvenanceEffect(&out, AssignmentProvenanceEffect{
		TargetPath: targetPath,
		SourcePath: sourcePath,
		Value:      product.FromType(typ.String),
	})

	if !changed {
		t.Fatal("assignment provenance did not report copied facts")
	}
	if !out.KeyPresence.HasValuePaths(tablePath, targetPath, valuePath) {
		t.Fatalf("assignment provenance did not copy key-presence value fact: %s", out.KeyPresence.Format())
	}
	origins := out.ValueOrigins.OriginsOfPath(targetPath)
	if len(origins) != 1 || origins[0].Kind != flow.ValueOriginAssignmentAlias ||
		origins[0].Source != flow.KeyPresencePathKey(sourcePath) {
		t.Fatalf("assignment provenance origins = %v, want alias from %s", origins, flow.KeyPresencePathKey(sourcePath))
	}
}

func TestAssignmentProvenanceCopiesIndexWriteAdmissionKeyPath(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	tablePath := constraint.NewPath(cfg.SymbolID(625), "nodes")
	sourcePath := constraint.NewPath(cfg.SymbolID(626), "node_id")
	targetPath := constraint.NewPath(cfg.SymbolID(627), "last_id")
	nodeValue := product.FromType(typ.NewRecord().Field("config", typ.NewRecord().Build()).Build())
	out := flow.PointState{
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target:  flow.IndexWriteAdmissionPathKey(tablePath),
			KeyPath: flow.IndexWriteAdmissionPathKey(sourcePath),
			Key:     product.FromType(typ.Any),
			Value:   nodeValue,
		}),
	}

	changed := tr.applyAssignmentProvenanceEffect(&out, AssignmentProvenanceEffect{
		TargetPath: targetPath,
		SourcePath: sourcePath,
		Value:      product.FromType(typ.Any),
	})

	if !changed {
		t.Fatal("assignment provenance did not report copied index-write admission")
	}
	got, ok := out.IndexWrites.Admission(flow.IndexWriteQuery{
		Target:  tablePath,
		KeyPath: targetPath,
		KeyType: typ.Any,
	})
	if !ok || !product.Domain.Equal(got, nodeValue) {
		t.Fatalf("rebased index-write admission = %v/%v, want node/true; facts=%s", got.ProjectValue(), ok, out.IndexWrites.Format())
	}
	killed := out.IndexWrites.KillAffectedByWrite(flow.IndexWriteAdmissionPathKey(sourcePath))
	if got, ok := killed.Admission(flow.IndexWriteQuery{
		Target:  tablePath,
		KeyPath: targetPath,
		KeyType: typ.Any,
	}); !ok || !product.Domain.Equal(got, nodeValue) {
		t.Fatalf("source overwrite killed rebased target admission: %v/%v in %s", got.ProjectValue(), ok, killed.Format())
	}
}

func TestAssignmentProvenanceReconstructsIndexWriteAdmissionFromKeyPresenceReadback(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	tablePath := constraint.NewPath(cfg.SymbolID(628), "nodes")
	sourcePath := constraint.NewPath(cfg.SymbolID(629), "node_id")
	targetPath := constraint.NewPath(cfg.SymbolID(630), "last_id")
	nodeType := typ.NewRecord().Field("config", typ.NewRecord().Build()).Build()
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(628)): product.FromType(typ.NewMap(typ.String, typ.NewOptional(nodeType))),
			flow.SymbolValueKey(cfg.SymbolID(629)): product.FromType(typ.String),
		},
		KeyPresence: flow.KeyPresenceFacts{}.WithPaths(tablePath, sourcePath),
	}

	changed := tr.applyAssignmentProvenanceEffect(&out, AssignmentProvenanceEffect{
		TargetPath: targetPath,
		SourcePath: sourcePath,
		Value:      product.FromType(typ.String),
	})

	if !changed {
		t.Fatal("assignment provenance did not report reconstructed index-write admission")
	}
	got, ok := out.IndexWrites.Admission(flow.IndexWriteQuery{
		Target:  tablePath,
		KeyPath: targetPath,
		KeyType: typ.String,
	})
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("key-presence readback admission = %v/%v, want node/true; facts=%s", got.ProjectValue(), ok, out.IndexWrites.Format())
	}
}

func TestAssignmentProvenanceRecordsStaticMemberAlias(t *testing.T) {
	s := &ast.IdentExpr{Value: "s"}
	box := &ast.IdentExpr{Value: "box"}
	cur := &ast.AttrGetExpr{
		Object:    box,
		Key:       &ast.StringExpr{Value: "cur"},
		KeySyntax: ast.AttrKeyDot,
	}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		s:   cfg.SymbolID(631),
		box: cfg.SymbolID(632),
	})
	tr := New(in, Config{})
	out := flow.PointState{}

	provenance, ok := tr.assignmentProvenanceEffect(cfg.AssignTarget{
		Kind: cfg.TargetField,
		Expr: cur,
	}, s, product.FromType(typ.NewRecord().Build()))
	if !ok {
		t.Fatal("static member assignment provenance effect was not constructed")
	}
	tr.applyAssignmentProvenanceEffect(&out, provenance)

	targetPath := constraint.NewPath(cfg.SymbolID(632), "box").Field("cur")
	sourcePath := constraint.NewPath(cfg.SymbolID(631), "s")
	origins := out.ValueOrigins.OriginsOfPath(targetPath)
	if len(origins) != 1 || origins[0].Kind != flow.ValueOriginAssignmentAlias ||
		origins[0].Source != flow.KeyPresencePathKey(sourcePath) {
		t.Fatalf("static member assignment origins = %v, want alias from %s", origins, flow.KeyPresencePathKey(sourcePath))
	}
}

func TestAssignmentValueOriginRejectsGradualAnyAlias(t *testing.T) {
	src := &ast.IdentExpr{Value: "id"}
	in := input.BuildFromFunction(&ast.FunctionExpr{ParList: &ast.ParList{}}, nil, nil)
	in.Graph.Bindings().Bind(src, cfg.SymbolID(612))
	in.Graph.Bindings().SetName(cfg.SymbolID(612), "id")
	tr := New(in, Config{})
	out := flow.PointState{}

	provenance, ok := tr.assignmentProvenanceEffect(cfg.AssignTarget{
		Kind:   cfg.TargetIdent,
		Symbol: cfg.SymbolID(611),
		Name:   "alias",
	}, src, product.FromType(typ.Any))
	if !ok {
		t.Fatal("assignment provenance effect was not constructed")
	}
	tr.applyAssignmentProvenanceEffect(&out, provenance)

	if len(out.ValueOrigins.Entries()) != 0 {
		t.Fatalf("gradual any assignment seeded alias origin: %s", out.ValueOrigins.Format())
	}
	targetPath := constraint.NewPath(cfg.SymbolID(611), "alias")
	if aliases := out.PathAliases.AliasesOfPath(targetPath); len(aliases) != 1 ||
		aliases[0].Source != flow.KeyPresencePathKey(constraint.NewPath(cfg.SymbolID(612), "id")) {
		t.Fatalf("strict any assignment path aliases = %v in %s, want alias<-id", aliases, out.PathAliases.Format())
	}
}

func TestIndexWriteReadAdmissionFollowsStrictAnyPathAlias(t *testing.T) {
	nodes := &ast.IdentExpr{Value: "nodes"}
	id := &ast.IdentExpr{Value: "id"}
	last := &ast.IdentExpr{Value: "last"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		nodes: cfg.SymbolID(721),
		id:    cfg.SymbolID(722),
		last:  cfg.SymbolID(723),
	})
	tr := New(in, Config{})
	nodesPath := constraint.NewPath(cfg.SymbolID(721), "nodes")
	idPath := constraint.NewPath(cfg.SymbolID(722), "id")
	lastPath := constraint.NewPath(cfg.SymbolID(723), "last")
	nodeType := typ.NewRecord().
		Field("config", typ.NewRecord().Field("data_targets", typ.NewArray(typ.String)).Build()).
		Build()
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(721)): product.FromType(typ.NewMap(typ.String, typ.NewOptional(nodeType))),
			flow.SymbolValueKey(cfg.SymbolID(723)): product.FromType(typ.Any),
		},
		PathAliases: flow.PathAliasFacts{}.WithPaths(lastPath, idPath),
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target:  flow.IndexWriteAdmissionPathKey(nodesPath),
			KeyPath: flow.IndexWriteAdmissionPathKey(idPath),
			Key:     product.FromType(typ.Any),
			Value:   product.FromType(nodeType),
		}),
	}

	got, ok := tr.evalAttrGet(&out, &ast.AttrGetExpr{
		Object:    nodes,
		Key:       last,
		KeySyntax: ast.AttrKeyIndex,
	}, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("path-alias-backed index read = %v/%v, want node/true", got.ProjectValue(), ok)
	}
}

func TestKeyPresenceIndexReadRequiresPresentKey(t *testing.T) {
	nodes := &ast.IdentExpr{Value: "nodes"}
	last := &ast.IdentExpr{Value: "last"}
	in := valueOriginInput(t, map[*ast.IdentExpr]cfg.SymbolID{
		nodes: cfg.SymbolID(701),
		last:  cfg.SymbolID(702),
	})
	tr := New(in, Config{})
	nodesPath := constraint.NewPath(cfg.SymbolID(701), "nodes")
	lastPath := constraint.NewPath(cfg.SymbolID(702), "last")
	nodeType := typ.NewRecord().
		Field("config", typ.NewRecord().Build()).
		Build()
	read := &ast.AttrGetExpr{
		Object:    nodes,
		Key:       last,
		KeySyntax: ast.AttrKeyIndex,
	}
	out := flow.PointState{
		Env: map[flow.ValueKey]product.AbstractValue{
			flow.SymbolValueKey(cfg.SymbolID(701)): product.FromType(typ.NewMap(typ.String, typ.NewOptional(nodeType))),
			flow.SymbolValueKey(cfg.SymbolID(702)): product.FromType(typ.Nil),
		},
		KeyPresence: flow.KeyPresenceFacts{}.WithPaths(nodesPath, lastPath),
	}

	got, ok := tr.evalAttrGet(&out, read, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Nil) {
		t.Fatalf("nil-key index read = %v/%v, want nil", got.ProjectValue(), ok)
	}

	out.Env[flow.SymbolValueKey(cfg.SymbolID(702))] = product.FromType(typ.Any)
	got, ok = tr.evalAttrGet(&out, read, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("opaque proven-key index read = %v/%v, want node", got.ProjectValue(), ok)
	}

	out.Env[flow.SymbolValueKey(cfg.SymbolID(702))] = product.FromType(typ.String)
	got, ok = tr.evalAttrGet(&out, read, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("present-key index read = %v/%v, want node", got.ProjectValue(), ok)
	}
}
