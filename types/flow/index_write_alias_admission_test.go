package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexWriteAdmissionWithKeyAliasesRetriesAliasKeys(t *testing.T) {
	table := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(401), "table"))
	key := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(402), "key"))
	alias := testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(403), "alias"))
	q := IndexWriteReadQuery{
		Point: cfg.Point(1),
		Admission: IndexWriteAddressQuery{
			Target:     table,
			KeyPath:    key,
			HasKeyPath: true,
		},
	}

	var seen []StableAddress
	got, ok := IndexWriteAdmissionWithKeyAliases(
		q,
		func(next IndexWriteReadQuery) (typ.Type, bool) {
			seen = append(seen, next.Admission.KeyPath)
			if next.Admission.KeyPath.Equal(alias) {
				return typ.String, true
			}
			return nil, false
		},
		func(point cfg.Point, root StableAddress) []StableAddress {
			if point != q.Point || !root.Equal(key) {
				t.Fatalf("alias query = (%v, %s), want (%v, %s)", point, root.Key(), q.Point, key.Key())
			}
			return []StableAddress{key, alias}
		},
	)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("IndexWriteAdmissionWithKeyAliases = %v/%v, want string/true", got, ok)
	}
	if len(seen) != 2 || !seen[0].Equal(key) || !seen[1].Equal(alias) {
		t.Fatalf("admission keys = %v, want original then alias", seen)
	}
}
