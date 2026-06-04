package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalFactsPathValueAtUsesStructuralMemberKeys(t *testing.T) {
	const point cfg.Point = 7
	const sym cfg.SymbolID = 11

	nested := product.WithMember(
		product.FromType(typ.NewRecord().Build()),
		value.MemberStringIndex(""),
		product.FromType(typ.String),
	)
	root := product.WithMember(
		product.FromType(typ.NewRecord().Build()),
		value.MemberField("headers"),
		nested,
	)
	root = product.WithMember(root, value.MemberIntIndex(1), product.FromType(typ.Number))

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): root,
				},
			},
		},
	}
	facts := &canonicalFacts{
		state: fs,
		paths: newPathProjector(fs, nil, callableProjector{}),
	}

	cases := []struct {
		name string
		path constraint.Path
		want typ.Type
	}{
		{
			name: "empty string index below field",
			path: constraint.NewPath(sym, "root").
				Field("headers").
				IndexStr(""),
			want: typ.String,
		},
		{
			name: "integer index",
			path: constraint.NewPath(sym, "root").
				IndexInt(1),
			want: typ.Number,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := facts.RefinedPathValueAt(point, tc.path)
			if got.State != flow.StateResolved {
				t.Fatalf("RefinedPathValueAt(%v) did not resolve", tc.path)
			}
			if !typ.TypeEquals(got.Value.ProjectValue(), tc.want) {
				t.Fatalf("RefinedPathValueAt(%v) = %v, want %v", tc.path, got.Value.ProjectValue(), tc.want)
			}
		})
	}
}

func TestCanonicalFactsRefinedPathAtAppliesLengthProofForStaticIndex(t *testing.T) {
	const point cfg.Point = 9
	const sym cfg.SymbolID = 13

	num := numeric.NewState()
	num.ApplyLenGeConst(constraint.PathKey(flow.SymbolValueKey(sym)), 1)

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): product.FromType(typ.NewArray(typ.Number)),
				},
				Num: num,
			},
		},
	}
	facts := &canonicalFacts{
		state: fs,
		paths: newPathProjector(fs, nil, callableProjector{}),
	}

	path := constraint.NewPath(sym, "arr").IndexInt(1)
	raw := facts.RefinedPathValueAt(point, path)
	if raw.State != flow.StateResolved || !typ.TypeEquals(raw.Value.ProjectValue(), typ.NewOptional(typ.Number)) {
		t.Fatalf("raw RefinedPathValueAt(%v) = %v/%v, want number?", path, raw.Value.ProjectValue(), raw.State)
	}

	got := facts.RefinedPathAt(point, path)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.Number) {
		t.Fatalf("RefinedPathAt(%v) = %v/%v, want number/resolved", path, got.Type, got.State)
	}
}

func TestCanonicalFactsIndexWriteAdmissionReadsPostState(t *testing.T) {
	const point cfg.Point = 10
	const sym cfg.SymbolID = 14
	target := constraint.NewPath(sym, "m")

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
					Target: flow.IndexWriteAdmissionPathKey(target),
					Key:    product.FromType(typ.String),
					Value:  product.FromType(typ.Number),
				}),
			},
		},
		Points: map[cfg.Point]flow.PointState{
			point: {
				IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
					Target: flow.IndexWriteAdmissionPathKey(target),
					Key:    product.FromType(typ.String),
					Value:  product.FromType(typ.Boolean),
				}),
			},
		},
	}
	facts := &canonicalFacts{state: fs}

	got, ok := facts.IndexWriteAdmission(flow.IndexWriteQuery{
		Point:   point,
		Target:  target,
		KeyType: typ.String,
	})
	if !ok || !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("IndexWriteAdmission = %v/%v, want post-state boolean/true", got, ok)
	}
}

func TestCanonicalFactsConditionTypeAtProjectsDiscriminatedChildWithoutSolution(t *testing.T) {
	const point cfg.Point = 11
	const sym cfg.SymbolID = 15

	root := constraint.NewPath(sym, "result")
	okVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("ok")).
		Field("value", typ.String).
		Build()
	errVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("err")).
		Field("value", typ.Number).
		Build()
	declared := typ.NewUnion(okVariant, errVariant)

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Cond: constraint.FromConstraints(constraint.FieldEquals{
					Target: root,
					Field:  "kind",
					Value:  typ.LiteralString("ok"),
				}),
			},
		},
	}
	facts := &canonicalFacts{
		state:    fs,
		declared: map[cfg.SymbolID]typ.Type{sym: declared},
		annotate: map[cfg.SymbolID]bool{sym: true},
		paths:    newPathProjector(fs, nil, callableProjector{}),
	}

	got := facts.ConditionTypeAt(point, root.Field("value"))
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionTypeAt(result.value) = %v, want string", got)
	}
}

func TestCanonicalFactsConditionedSeedTypeAtProjectsLocalCondition(t *testing.T) {
	const point cfg.Point = 12
	const sym cfg.SymbolID = 16

	root := constraint.NewPath(sym, "entry")
	aVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("payload", typ.String).
		Build()
	bVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("payload", typ.Number).
		Build()
	seed := typ.NewUnion(aVariant, bVariant)
	facts := &canonicalFacts{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Cond: constraint.TrueCondition(),
				},
			},
		},
	}

	got := facts.ConditionedSeedTypeAt(point, root, seed, root.Field("payload"), constraint.FromConstraints(constraint.FieldEquals{
		Target: root,
		Field:  "kind",
		Value:  typ.LiteralString("a"),
	}))
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionedSeedTypeAt(entry.payload) = %v, want string", got)
	}
}

func TestCanonicalFactsProvesTypeAtUsesConditionProofFacts(t *testing.T) {
	const point cfg.Point = 13
	const sym cfg.SymbolID = 17

	path := constraint.NewPath(sym, "value")
	facts := &canonicalFacts{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Cond: constraint.FromConstraints(constraint.HasType{
						Path: path,
						Type: narrow.BuiltinTypeKey("string"),
					}),
				},
			},
		},
	}

	if !facts.ProvesTypeAt(point, path, typ.String) {
		t.Fatalf("ProvesTypeAt(value, string) = false, want true")
	}
}

func TestCanonicalFactsObservePathUsesRootHasTypeConditionProofWithoutDeclaredSeed(t *testing.T) {
	const point cfg.Point = 18
	const sym cfg.SymbolID = 22

	path := constraint.NewPath(sym, "value")
	facts := &canonicalFacts{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Cond: constraint.FromConstraints(constraint.HasType{
						Path: path,
						Type: narrow.BuiltinTypeKey("string"),
					}),
				},
			},
		},
	}

	proof := facts.ConditionTypeAt(point, path)
	if !typ.TypeEquals(proof, typ.String) {
		t.Fatalf("ConditionTypeAt(value) = %v, want string", proof)
	}

	obs := facts.ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		AllowConditionProof: true,
		PreserveProof:       true,
	})
	if !obs.Resolved() || obs.Source != flow.PathObservationConditionProof || !typ.TypeEquals(obs.Type, typ.String) {
		t.Fatalf("ObservePath condition proof = %#v, want condition-proof string", obs)
	}
}

func TestCanonicalFactsObservePathUsesDirectPathProjection(t *testing.T) {
	const point cfg.Point = 14
	const sym cfg.SymbolID = 18

	root := product.WithMember(
		product.FromType(typ.NewRecord().Build()),
		value.MemberField("name"),
		product.FromType(typ.String),
	)
	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): root,
				},
			},
		},
	}
	facts := &canonicalFacts{
		state: fs,
		paths: newPathProjector(fs, nil, callableProjector{}),
	}

	obs := facts.ObservePath(flow.PathObservationQuery{
		Point: point,
		Path:  constraint.NewPath(sym, "user").Field("name"),
	})
	if !obs.Resolved() || obs.Source != flow.PathObservationDirectPath || !typ.TypeEquals(obs.Type, typ.String) {
		t.Fatalf("ObservePath direct = %#v, want direct string", obs)
	}
}

func TestCanonicalFactsObservePathHonorsStrictPrePhase(t *testing.T) {
	const point cfg.Point = 15
	const sym cfg.SymbolID = 19

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): product.FromType(typ.String),
				},
			},
		},
		Points: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): product.FromType(typ.Number),
				},
			},
		},
	}
	facts := &canonicalFacts{
		state: fs,
		paths: newPathProjector(fs, nil, callableProjector{}),
	}
	path := constraint.NewPath(sym, "value")

	pre := facts.ObservePath(flow.PathObservationQuery{
		Point:      point,
		Path:       path,
		View:       flow.PathReadPre,
		StrictView: true,
	})
	if !pre.Resolved() || !typ.TypeEquals(pre.Type, typ.String) {
		t.Fatalf("ObservePath strict pre = %#v, want string", pre)
	}

	post := facts.ObservePath(flow.PathObservationQuery{
		Point: point,
		Path:  path,
		View:  flow.PathReadPost,
	})
	if !post.Resolved() || !typ.TypeEquals(post.Type, typ.Number) {
		t.Fatalf("ObservePath post = %#v, want number", post)
	}
}

func TestCanonicalFactsObservePathKeepsNilRefinementOfOptionalDeclaredRead(t *testing.T) {
	const point cfg.Point = 17
	const sym cfg.SymbolID = 21

	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				Env: map[flow.ValueKey]product.AbstractValue{
					flow.SymbolValueKey(sym): product.FromType(typ.Nil),
				},
			},
		},
	}
	facts := &canonicalFacts{
		state:    fs,
		declared: map[cfg.SymbolID]typ.Type{sym: typ.NewOptional(typ.String)},
		annotate: map[cfg.SymbolID]bool{sym: true},
		paths:    newPathProjector(fs, nil, callableProjector{}),
	}

	obs := facts.ObservePath(flow.PathObservationQuery{
		Point: point,
		Path:  constraint.NewPath(sym, "value"),
	})
	if !obs.Resolved() || !typ.TypeEquals(obs.Type, typ.Nil) {
		t.Fatalf("ObservePath nil optional refinement = %#v, want nil", obs)
	}
}

func TestCanonicalFactsObservePathUsesLocalConditionProof(t *testing.T) {
	const point cfg.Point = 16
	const sym cfg.SymbolID = 20

	root := constraint.NewPath(sym, "entry")
	aVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("payload", typ.String).
		Build()
	bVariant := typ.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("payload", typ.Number).
		Build()
	facts := &canonicalFacts{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {Cond: constraint.TrueCondition()},
			},
		},
		declared: map[cfg.SymbolID]typ.Type{
			sym: typ.NewUnion(aVariant, bVariant),
		},
		annotate: map[cfg.SymbolID]bool{sym: true},
	}
	local := constraint.FromConstraints(constraint.FieldEquals{
		Target: root,
		Field:  "kind",
		Value:  typ.LiteralString("a"),
	})

	obs := facts.ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                root.Field("payload"),
		AllowConditionProof: true,
		LocalCondition:      &local,
		PreserveProof:       true,
	})
	if !obs.Resolved() || obs.Source != flow.PathObservationConditionProof || !typ.TypeEquals(obs.Type, typ.String) {
		t.Fatalf("ObservePath local condition = %#v, want condition-proof string", obs)
	}
}
