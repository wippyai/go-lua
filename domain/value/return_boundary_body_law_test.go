package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func returnBoundaryLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0], id[31] = 0xb4, value
	return id
}

func TestReturnBoundaryBodyIndexPointsToCanonicalRows(t *testing.T) {
	module, body := returnBoundaryLawID(1), returnBoundaryLawID(2)
	firstKey := computationKey{module: module, occurrence: returnBoundaryLawID(3)}
	secondKey := computationKey{module: module, occurrence: returnBoundaryLawID(4)}
	schema := &Schema{
		coordinateCount:        3,
		returnBoundaries:       make(map[computationKey]ReturnBoundary),
		returnBoundariesByBody: make(map[computationKey][]computationKey),
		returnBoundaryMembers:  []returnBoundaryMember{{coordinate: Coordinate{index: 2}}, {coordinate: Coordinate{index: 3}}},
	}
	for index := range schema.returnBoundaryMembers {
		schema.returnBoundaryMembers[index].coordinate.schema = schema
	}
	first := ReturnBoundary{schema: schema, key: firstKey, body: body, content: returnBoundaryLawID(5), root: Coordinate{schema: schema, index: 1}, memberCount: 1}
	second := ReturnBoundary{schema: schema, key: secondKey, body: body, content: returnBoundaryLawID(6), root: Coordinate{schema: schema, index: 1}, memberOffset: 1, memberCount: 1}
	schema.returnBoundaries[firstKey], schema.returnBoundaries[secondKey] = first, second
	bodyKey := computationKey{module: module, occurrence: body}
	schema.returnBoundariesByBody[bodyKey] = []computationKey{firstKey, secondKey}

	rows, ok := schema.ReturnBoundariesForBody(module, body)
	if !ok || len(rows) != 2 || rows[0] != first || rows[1] != second {
		t.Fatalf("body return rows=%v ok=%t", rows, ok)
	}
	for index, row := range rows {
		owner, ownerOK := row.BodyID()
		member, memberOK := row.MemberAt(0)
		if !ownerOK || owner != body || !memberOK || member.schema != schema || member.index != uint32(index+2) {
			t.Fatalf("body return row %d lost owner/member", index)
		}
	}
	foreign := *schema
	if foreign.OwnsReturnBoundary(rows[0]) {
		t.Fatal("foreign equal-content Schema accepted a return boundary")
	}
}
