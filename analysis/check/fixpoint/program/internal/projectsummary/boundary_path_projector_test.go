package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBoundaryPathProjectorOwnsNormalReturnRebasing(t *testing.T) {
	ks := keyspace.New()
	paramSym := symbol.ID(101)
	localSym := symbol.ID(202)
	capturedSym := symbol.ID(303)

	params := []path.Path{path.NewPath(paramSym, "arg")}
	returns := []exitFactReturnPath{
		{
			source: path.Path{Root: "tmp", Symbol: localSym, Version: 3}.Field("node"),
			target: path.Path{Root: "ret[0]"}.Field("value"),
		},
	}
	projector := newBoundaryPathProjector(ks, params, returns, map[symbol.ID]struct{}{
		capturedSym: {},
	})

	tests := []struct {
		name string
		got  func() (path.Path, bool)
		want path.Path
	}{
		{
			name: "placeholder path key stays on its placeholder",
			got: func() (path.Path, bool) {
				return projector.PlaceholderPath(path.NewPlaceholder(0).Field("ready").Key())
			},
			want: path.NewPlaceholder(0).Field("ready"),
		},
		{
			name: "parameter state key rebases to the matching placeholder suffix",
			got: func() (path.Path, bool) {
				return projector.StatePath(stateKeyForPath(t, ks, versionedPath(paramSym, 1, "arg").Field("seed").Field("id")))
			},
			want: path.NewPlaceholder(0).Field("seed").Field("id"),
		},
		{
			name: "parameter keyspace key rebases to placeholder without formatting",
			got: func() (path.Path, bool) {
				key := ks.FromPath(versionedPath(paramSym, 1, "arg").Field("native"))
				return projector.KeyspacePlaceholderPath(key)
			},
			want: path.NewPlaceholder(0).Field("native"),
		},
		{
			name: "return-source state key rebases to the matching return slot suffix",
			got: func() (path.Path, bool) {
				local := path.Path{Root: "tmp", Symbol: localSym, Version: 3}.Field("node").Field("leaf")
				return projector.StatePath(stateKeyForPath(t, ks, local))
			},
			want: path.Path{Root: "ret[0]"}.Field("value").Field("leaf"),
		},
		{
			name: "captured persistent sink remains a concrete state path",
			got: func() (path.Path, bool) {
				captured := versionedPath(capturedSym, 1, "sink").Field("field")
				return projector.StatePath(stateKeyForPath(t, ks, captured))
			},
			want: versionedPath(capturedSym, 1, "sink").Field("field"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.got()
			if !ok {
				t.Fatalf("projection failed")
			}
			if !got.Equal(tc.want) {
				t.Fatalf("projected path = %q, want %q", got.Key(), tc.want.Key())
			}
		})
	}
}

func TestBoundaryPathProjectorProjectsRelConstraints(t *testing.T) {
	ks := keyspace.New()
	aSym := symbol.ID(401)
	bSym := symbol.ID(402)
	projector := newBoundaryPathProjector(ks, []path.Path{
		path.NewPath(aSym, "a"),
		path.NewPath(bSym, "b"),
	}, nil, nil)

	got, ok := projector.RelConstraintFact(state.RelConstraint{
		CoA: 1,
		A:   state.RelValueOperand(stateKeyForPath(t, ks, versionedPath(aSym, 1, "a").Field("i"))),
		CoB: -1,
		B:   state.RelLengthOperand(stateKeyForPath(t, ks, versionedPath(bSym, 1, "b").Field("items"))),
		C:   state.RelValueOperand(stateKeyForPath(t, ks, versionedPath(bSym, 1, "b").Field("limit"))),
		K:   1,
	})
	if !ok {
		t.Fatalf("rel constraint projection failed")
	}
	if !got.A.Path.Equal(path.NewPlaceholder(0).Field("i")) || got.A.IsLength {
		t.Fatalf("A = %#v, want $0.i path operand", got.A)
	}
	if !got.B.Path.Equal(path.NewPlaceholder(1).Field("items")) || !got.B.IsLength {
		t.Fatalf("B = %#v, want len($1.items)", got.B)
	}
	if !got.C.Path.Equal(path.NewPlaceholder(1).Field("limit")) || got.C.IsLength {
		t.Fatalf("C = %#v, want $1.limit path operand", got.C)
	}
}

func stateKeyForPath(t *testing.T, ks *keyspace.KeySpace, p path.Path) pathaddr.StateKey {
	t.Helper()
	stateKey, ok := pathaddr.StateKeyFromPathKey(ks.Format(ks.FromPath(p)))
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", p.Key())
	}
	return stateKey
}

func versionedPath(sym symbol.ID, version int, name string) path.Path {
	return path.Path{Root: name, Symbol: sym, Version: version}
}
