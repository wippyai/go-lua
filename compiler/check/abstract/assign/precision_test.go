package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPreferPreciseDirectSourceType_PrefersNamedEquivalentAliasForSingleTarget(t *testing.T) {
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Integer).
		Build()
	alias := typ.NewAlias("Counter", record)

	got := preferPreciseDirectSourceType(
		record,
		&ast.IdentExpr{Value: "x"},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return alias },
		true,
	)
	if got != alias {
		t.Fatalf("expected direct named alias to win, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_DoesNotReplaceNamedAssignedType(t *testing.T) {
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Integer).
		Build()
	alias := typ.NewAlias("Counter", record)

	got := preferPreciseDirectSourceType(
		alias,
		&ast.IdentExpr{Value: "x"},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return record },
		true,
	)
	if got != alias {
		t.Fatalf("expected existing named assigned type to remain, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_RefinesUnknownRecordFieldFromSameExpression(t *testing.T) {
	assigned := typ.NewRecord().
		Field("headers", typ.NewMap(typ.String, typ.String)).
		Field("timeout", typ.Unknown).
		Build()
	precise := typ.NewRecord().
		Field("headers", typ.NewMap(typ.String, typ.String)).
		Field("timeout", typ.Number).
		Build()

	got := preferPreciseDirectSourceType(
		assigned,
		&ast.TableExpr{},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return precise },
		true,
	)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("expected same-expression concrete field evidence to win, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_PrefersSameExpressionExtraFieldEvidence(t *testing.T) {
	assigned := typ.NewRecord().
		Field("name", typ.String).
		Build()
	precise := typ.NewRecord().
		Field("name", typ.String).
		Field("ready", typ.Boolean).
		Build()

	got := preferPreciseDirectSourceType(
		assigned,
		&ast.TableExpr{},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return precise },
		true,
	)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("expected same-expression extra field evidence to win, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_DoesNotDropAssignedRecordEvidence(t *testing.T) {
	assigned := typ.NewRecord().
		Field("headers", typ.NewMap(typ.String, typ.String)).
		Field("timeout", typ.Unknown).
		Build()
	precise := typ.NewRecord().
		Field("timeout", typ.Number).
		Build()

	got := preferPreciseDirectSourceType(
		assigned,
		&ast.TableExpr{},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return precise },
		true,
	)
	if !typ.TypeEquals(got, assigned) {
		t.Fatalf("expected assigned evidence to remain when direct type drops fields, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_RecursiveSoftRefinementUsesValueDomain(t *testing.T) {
	assigned := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("payload", typ.NewRecord().
				Field("owner", self).
				Field("value", typ.Any).
				Build()).
			Build()
	})
	precise := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("payload", typ.NewRecord().
				Field("owner", self).
				Field("value", typ.String).
				Build()).
			Build()
	})

	got := preferPreciseDirectSourceType(
		assigned,
		&ast.TableExpr{},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return precise },
		true,
	)
	if got != precise {
		t.Fatalf("expected recursive soft refinement to win through value domain, got %s", typ.FormatShort(got))
	}
}

func TestMergeUnannotatedParamType_PreservesReceiverContractWithFieldOverlay(t *testing.T) {
	current := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build()
	inferred := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()
	want := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		Field("prefix", typ.String).
		Build()

	got := mergeUnannotatedParamType(current, inferred)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("merged parameter type = %v, want %v", got, want)
	}
}

func TestMergeUnannotatedParamType_PreservesReceiverAliasWithFieldOverlay(t *testing.T) {
	currentRecord := typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build()
	current := typ.NewAlias("Builder", currentRecord)
	inferred := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()

	got := mergeUnannotatedParamType(current, inferred)
	alias, ok := got.(*typ.Alias)
	if !ok || alias.Name != "Builder" {
		t.Fatalf("merged parameter type = %T %v, want Builder alias", got, got)
	}
	target, ok := alias.Target.(*typ.Record)
	if !ok || target.GetField("build") == nil {
		t.Fatalf("merged alias target lost declared fields: %v", alias.Target)
	}
	prefix := target.GetField("prefix")
	if prefix == nil || prefix.Optional {
		t.Fatalf("merged alias target should make prefix present, got %v", alias.Target)
	}
}

func TestMergeUnannotatedParamType_KeepsExplicitAnyContract(t *testing.T) {
	if got := mergeUnannotatedParamType(typ.Any, typ.String); !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("explicit any parameter contract should remain any, got %v", got)
	}
}

func TestPreferPreciseDirectSourceType_MultiTargetNonAnySkipsDirectSynthesis(t *testing.T) {
	called := false
	assigned := typ.NewRecord().Field("id", typ.String).Build()

	got := preferPreciseDirectSourceType(
		assigned,
		&ast.FuncCallExpr{},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type {
			called = true
			return typ.Any
		},
		false,
	)
	if got != assigned {
		t.Fatalf("expected assigned type to remain, got %s", typ.FormatShort(got))
	}
	if called {
		t.Fatal("multi-target non-any slot must not invoke direct synthesis")
	}
}
