package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func staticTestTerm(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func staticTestCoordinate() source.Coordinate {
	coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
	return coordinate
}

func TestStaticRowsValidateRawPayloadsAndDenseOrdinals(t *testing.T) {
	if _, err := rawString(""); err == nil {
		t.Fatal("rawString accepted an empty authored key")
	}
	if _, err := rawLiteral(keyspace.LiteralValue{}); err == nil {
		t.Fatal("rawLiteral accepted an invalid literal")
	}
	path, err := staticRawPath([]string{"pkg", "Type"})
	if err != nil || len(path) != 2 {
		t.Fatalf("staticRawPath = %#v, %v", path, err)
	}
	if err := requireFamily(staticTestTerm(keyspace.FamilyTypeAlias, 1), keyspace.FamilyTypeInterface); err == nil {
		t.Fatal("requireFamily accepted a foreign family")
	}
	if _, err := denseOrdinal(staticTestTerm(keyspace.FamilyTypeAlias, 1), keyspace.FamilyTypeAlias, 0); err == nil {
		t.Fatal("denseOrdinal accepted an absent row")
	}
}
