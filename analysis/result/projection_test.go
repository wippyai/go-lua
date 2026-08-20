package result

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
)

func artifactResultLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func artifactResultLawCodec(seed byte) identity.SemanticKey {
	var digest [32]byte
	digest[0] = seed
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("result law codec")
	}
	return key
}

func artifactResultLawContract(t testing.TB, family, codec byte) engine.CanonicalResultContract {
	t.Helper()
	contract, ok := engine.NewCanonicalResultContract(artifactResultLawID(family), artifactResultLawCodec(codec))
	if !ok {
		t.Fatalf("canonical result contract %d/%d", family, codec)
	}
	return contract
}

func artifactResultLawCell(t testing.TB, contract engine.CanonicalResultContract, payload string) engine.CanonicalResultCell {
	t.Helper()
	cell, ok := engine.NewCanonicalResultCell(contract, true, 1, []byte(payload))
	if !ok {
		t.Fatalf("canonical result cell %q", payload)
	}
	return cell
}

type artifactResultLawTables struct {
	source   identity.ContentID
	bodies   []resultBody
	points   []resultPoint
	families []resultFamily
}

func artifactResultLawBaseTables(t testing.TB) artifactResultLawTables {
	t.Helper()
	firstContract := artifactResultLawContract(t, 11, 21)
	secondContract := artifactResultLawContract(t, 12, 22)
	firstCell := artifactResultLawCell(t, firstContract, "first")
	secondCell := artifactResultLawCell(t, secondContract, "second")
	return artifactResultLawTables{
		source: artifactResultLawID(1),
		bodies: []resultBody{
			{id: artifactResultLawID(2)},
			{id: artifactResultLawID(3)},
		},
		points: []resultPoint{{
			mount:  artifactResultLawID(4),
			point:  artifactResultLawID(5),
			bodies: []uint32{1},
		}},
		families: []resultFamily{
			{
				ordinal:  1,
				key:      schema.Key("law/first"),
				contract: firstContract,
				queries: []resultQuery{{
					site: artifactResultLawID(6), key: artifactResultLawID(7), point: 1,
					status: QueryHit, cell: firstCell,
				}},
			},
			{
				ordinal:  2,
				key:      schema.Key("law/second"),
				contract: secondContract,
				queries: []resultQuery{{
					site: artifactResultLawID(8), key: artifactResultLawID(9), point: 1,
					status: QueryHit, cell: secondCell,
				}},
			},
		},
	}
}

func artifactResultLawCloneTables(tables artifactResultLawTables) artifactResultLawTables {
	clone := artifactResultLawTables{
		source:   tables.source,
		bodies:   append([]resultBody(nil), tables.bodies...),
		points:   make([]resultPoint, len(tables.points)),
		families: make([]resultFamily, len(tables.families)),
	}
	for index, point := range tables.points {
		clone.points[index] = point
		clone.points[index].bodies = append([]uint32(nil), point.bodies...)
	}
	for index, family := range tables.families {
		clone.families[index] = family
		clone.families[index].queries = append([]resultQuery(nil), family.queries...)
	}
	return clone
}

func artifactResultLawIDFor(t testing.TB, tables artifactResultLawTables) identity.ContentID {
	t.Helper()
	id, ok := analysisResultID(tables.source, tables.bodies, tables.points, tables.families, nil)
	if !ok {
		t.Fatal("normalized result tables were not hashable")
	}
	return id
}

func artifactResultLawResult(t testing.TB, tables artifactResultLawTables) *Result {
	t.Helper()
	content := artifactResultLawIDFor(t, tables)
	result := &Result{
		source: tables.source, content: content,
		bodies: tables.bodies, points: tables.points, families: tables.families,
		sealed: true,
	}
	if !result.valid() {
		t.Fatal("normalized result fixture did not satisfy Result.valid")
	}
	return result
}

func TestArtifactResultNormalizedTablesHaveNoFixedValueOrEffectLanes(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(Result{}), reflect.TypeOf(resultBody{})} {
		for _, field := range []string{"values", "valuePresence", "effects", "effectPresent", "effectTop"} {
			if _, found := typ.FieldByName(field); found {
				t.Fatalf("%s still owns deleted fixed-lane field %q", typ.Name(), field)
			}
		}
	}
}

func TestArtifactResultFamiliesShareOnePointAndBodyRow(t *testing.T) {
	tables := artifactResultLawBaseTables(t)
	result := artifactResultLawResult(t, tables)
	if len(result.points) != 1 || len(result.points[0].bodies) != 1 {
		t.Fatalf("normalized point table = %#v, want one point/body row", result.points)
	}

	if got, want := result.FamilyCount(), 2; got != want {
		t.Fatalf("FamilyCount = %d, want %d", got, want)
	}
	for familyIndex := 0; familyIndex < result.FamilyCount(); familyIndex++ {
		family, familyOK := result.FamilyAt(familyIndex)
		if !familyOK || family.QueryCount() != 1 {
			t.Fatalf("FamilyAt(%d) = valid %t/query count %d", familyIndex, familyOK, family.QueryCount())
		}
		query, queryOK := family.QueryAt(0)
		if !queryOK {
			t.Fatalf("FamilyAt(%d).QueryAt(0) unavailable", familyIndex)
		}
		if mount, ok := query.MountID(); !ok || mount != tables.points[0].mount {
			t.Fatalf("family %d mount = %v/%t, want %v/true", familyIndex, mount, ok, tables.points[0].mount)
		}
		if point, ok := query.PointID(); !ok || point != tables.points[0].point {
			t.Fatalf("family %d point = %v/%t, want %v/true", familyIndex, point, ok, tables.points[0].point)
		}
		if query.BodyCount() != 1 {
			t.Fatalf("family %d body count = %d, want 1", familyIndex, query.BodyCount())
		}
		body, bodyOK := query.BodyAt(0)
		bodyID, bodyIDOK := body.ID()
		if !bodyOK || !bodyIDOK || bodyID != tables.bodies[0].id {
			t.Fatalf("family %d body = %v/%t/%t, want %v/true/true", familyIndex, bodyID, bodyOK, bodyIDOK, tables.bodies[0].id)
		}
	}
}

func TestArtifactResultFamilyContractsAndCellsRemainDistinct(t *testing.T) {
	tables := artifactResultLawBaseTables(t)
	result := artifactResultLawResult(t, tables)
	firstFamily, firstFamilyOK := result.FamilyAt(0)
	secondFamily, secondFamilyOK := result.FamilyAt(1)
	if !firstFamilyOK || !secondFamilyOK {
		t.Fatal("normalized family fixture")
	}
	if firstFamily.ContractID() == secondFamily.ContractID() || firstFamily.ID() == secondFamily.ID() {
		t.Fatal("family contracts collapsed distinct registration identities")
	}
	firstQuery, firstQueryOK := firstFamily.QueryAt(0)
	secondQuery, secondQueryOK := secondFamily.QueryAt(0)
	if !firstQueryOK || !secondQueryOK {
		t.Fatal("normalized query fixture")
	}
	firstCell, firstCellOK := firstQuery.Cell()
	secondCell, secondCellOK := secondQuery.Cell()
	if !firstCellOK || !secondCellOK || firstCell.ContentID() == secondCell.ContentID() {
		t.Fatal("family cells collapsed distinct canonical payloads")
	}
	if firstCell.ContractID() != firstFamily.ContractID() || secondCell.ContractID() != secondFamily.ContractID() {
		t.Fatal("canonical cell escaped its family contract")
	}
}

func TestArtifactResultNormalizedIdentityIsStableAndComplete(t *testing.T) {
	base := artifactResultLawBaseTables(t)
	first := artifactResultLawIDFor(t, base)
	second := artifactResultLawIDFor(t, artifactResultLawCloneTables(base))
	if first != second {
		t.Fatal("equivalent normalized tables received different Result identities")
	}

	tests := []struct {
		name   string
		mutate func(artifactResultLawTables) artifactResultLawTables
	}{
		{
			name: "point membership",
			mutate: func(tables artifactResultLawTables) artifactResultLawTables {
				variant := artifactResultLawCloneTables(tables)
				variant.points[0].bodies = []uint32{2}
				return variant
			},
		},
		{
			name: "family contract",
			mutate: func(tables artifactResultLawTables) artifactResultLawTables {
				variant := artifactResultLawCloneTables(tables)
				contract := artifactResultLawContract(t, 31, 41)
				variant.families[0].contract = contract
				variant.families[0].queries[0].cell = artifactResultLawCell(t, contract, "replacement")
				return variant
			},
		},
		{
			name: "query status",
			mutate: func(tables artifactResultLawTables) artifactResultLawTables {
				variant := artifactResultLawCloneTables(tables)
				variant.families[0].queries[0].status = QueryProvenAbsent
				variant.families[0].queries[0].cell = engine.CanonicalResultCell{}
				return variant
			},
		},
		{
			name: "cell identity",
			mutate: func(tables artifactResultLawTables) artifactResultLawTables {
				variant := artifactResultLawCloneTables(tables)
				contract := variant.families[0].contract
				variant.families[0].queries[0].cell = artifactResultLawCell(t, contract, "changed")
				return variant
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if changed := artifactResultLawIDFor(t, test.mutate(base)); changed == first {
				t.Fatalf("%s did not participate in Result identity", test.name)
			}
		})
	}
}

func TestArtifactResultSyntheticFamilyUsesNormalizedGenericTables(t *testing.T) {
	tables := artifactResultLawBaseTables(t)
	thirdContract := artifactResultLawContract(t, 51, 61)
	tables.families = append(tables.families, resultFamily{
		ordinal:  3,
		key:      schema.Key("law/synthetic-third"),
		contract: thirdContract,
		queries: []resultQuery{{
			site: artifactResultLawID(10), key: artifactResultLawID(13), point: 1,
			status: QueryHit, cell: artifactResultLawCell(t, thirdContract, "third"),
		}},
	})
	result := artifactResultLawResult(t, tables)
	family, familyOK := result.FamilyAt(2)
	if !familyOK || family.Key() != schema.Key("law/synthetic-third") || family.ContractID() != thirdContract.ContentID() {
		t.Fatal("synthetic family did not resolve through generic family storage")
	}
	query, queryOK := family.QueryAt(0)
	cell, cellOK := query.Cell()
	if !queryOK || !cellOK || query.Status() != QueryHit || cell.ContractID() != thirdContract.ContentID() {
		t.Fatal("synthetic family query did not resolve through generic query storage")
	}
}

func TestArtifactResultRootRowsPreserveOrderAndSeparateDuplicateMounts(t *testing.T) {
	artifact := artifactResultLawID(1)
	localRoot := artifactResultLawID(2)
	firstMount, secondMount := artifactResultLawID(3), artifactResultLawID(4)
	firstID, firstOK := mountedResultID("root", firstMount, artifact, localRoot)
	secondID, secondOK := mountedResultID("root", secondMount, artifact, localRoot)
	singleID, singleOK := mountedResultID("root", firstMount, artifact, artifactResultLawID(10))
	if !firstOK || !secondOK || !singleOK || firstID == secondID {
		t.Fatal("duplicate artifact mounts did not receive distinct Result root identities")
	}
	result := &Result{
		source:  artifactResultLawID(5),
		content: artifactResultLawID(6),
		bodies: []resultBody{
			{id: artifactResultLawID(7)},
			{id: artifactResultLawID(8), roots: []resultRoot{{id: singleID, family: keyspace.FamilyBind}}},
			{id: artifactResultLawID(9), roots: []resultRoot{{id: firstID, family: keyspace.FamilyBind}, {id: secondID, family: keyspace.FamilyReturn}}},
		},
		sealed: true,
	}
	zero, zeroOK := result.BodyAt(0)
	if !zeroOK || zero.RootCount() != 0 {
		t.Fatalf("zero-root body = %d/%t, want 0/true", zero.RootCount(), zeroOK)
	}
	single, singleOK := result.BodyAt(1)
	if !singleOK || single.RootCount() != 1 {
		t.Fatalf("single-root body = %d/%t, want 1/true", single.RootCount(), singleOK)
	}
	body, bodyOK := result.BodyAt(2)
	if !bodyOK || body.RootCount() != 2 {
		t.Fatalf("root count = %d/%t, want 2/true", body.RootCount(), bodyOK)
	}
	for index, want := range []struct {
		id     identity.ContentID
		family keyspace.Family
	}{{firstID, keyspace.FamilyBind}, {secondID, keyspace.FamilyReturn}} {
		root, rootOK := body.RootAt(index)
		rootID, rootIDOK := root.ID()
		if !rootOK || !rootIDOK || rootID != want.id || root.Family() != want.family {
			t.Fatalf("RootAt(%d) lost exact ordered row", index)
		}
	}
	if _, ok := body.RootAt(2); ok {
		t.Fatal("out-of-range root was accepted")
	}
}
