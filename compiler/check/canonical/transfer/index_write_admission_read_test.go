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

	out.Env[flow.SymbolValueKey(cfg.SymbolID(702))] = product.FromType(typ.String)
	got, ok = tr.evalAttrGet(&out, read, nil)
	if !ok || !typ.TypeEquals(got.ProjectValue(), nodeType) {
		t.Fatalf("present-key index read = %v/%v, want node", got.ProjectValue(), ok)
	}
}
