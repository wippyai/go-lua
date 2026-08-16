package analysis

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func artifactResultLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func TestArtifactResultSharesOneValueAxisAcrossBodies(t *testing.T) {
	if _, retained := reflect.TypeOf(resultBody{}).FieldByName("values"); retained {
		t.Fatal("resultBody still owns a duplicate Value axis")
	}
	if axis, published := reflect.TypeOf(Result{}).FieldByName("values"); !published || axis.Type != reflect.TypeOf([]identity.ContentID(nil)) {
		t.Fatal("Result does not own the canonical Value ContentID axis")
	}
	values := make([]identity.ContentID, 130)
	for index := range values {
		values[index] = artifactResultLawID(byte(index + 1))
	}
	bodies := make([]resultBody, 3)
	for index := range bodies {
		bodies[index] = resultBody{id: artifactResultLawID(byte(201 + index)), valuePresence: make([]uint64, resultValueWordCount(len(values)))}
		if got, want := cap(bodies[index].valuePresence), resultValueWordCount(len(values)); got != want {
			t.Fatalf("body %d presence capacity = %d, want %d", index, got, want)
		}
	}
	if !setResultValuePresent(bodies[0].valuePresence, 0) || !setResultValuePresent(bodies[1].valuePresence, 64) || !setResultValuePresent(bodies[2].valuePresence, 129) {
		t.Fatal("presence bitmap did not admit an in-range value")
	}
	result := &Result{source: artifactResultLawID(250), content: artifactResultLawID(251), values: values, bodies: bodies, sealed: true}
	for bodyIndex := range bodies {
		body, ok := result.BodyAt(bodyIndex)
		if !ok || body.ValueCount() != len(values) {
			t.Fatalf("BodyAt(%d) ValueCount = %d/%t, want %d/true", bodyIndex, body.ValueCount(), ok, len(values))
		}
		for _, valueIndex := range []int{0, 64, 129} {
			id, present, readable := body.ValueAt(valueIndex)
			wantPresent := bodyIndex == 0 && valueIndex == 0 || bodyIndex == 1 && valueIndex == 64 || bodyIndex == 2 && valueIndex == 129
			if !readable || id != values[valueIndex] || present != wantPresent {
				t.Fatalf("body %d ValueAt(%d) = %v/%t/%t, want %v/%t/true", bodyIndex, valueIndex, id, present, readable, values[valueIndex], wantPresent)
			}
		}
	}
	if got, want := len(bodies[0].valuePresence)+len(bodies[1].valuePresence)+len(bodies[2].valuePresence), 3*resultValueWordCount(len(values)); got != want {
		t.Fatalf("presence storage words = %d, want %d", got, want)
	}
}

func TestArtifactResultAxisHashIsSequentiallyStable(t *testing.T) {
	values := []identity.ContentID{artifactResultLawID(1), artifactResultLawID(2)}
	bodies := []resultBody{{id: artifactResultLawID(3), valuePresence: []uint64{1}}}
	first, firstOK := analysisResultID(artifactResultLawID(4), values, bodies)
	second, secondOK := analysisResultID(artifactResultLawID(4), append([]identity.ContentID(nil), values...), append([]resultBody(nil), bodies...))
	if !firstOK || !secondOK || first != second {
		t.Fatalf("equivalent axis result IDs = %v/%t and %v/%t, want same available ID", first, firstOK, second, secondOK)
	}
	bodies[0].valuePresence[0] = 2
	changed, changedOK := analysisResultID(artifactResultLawID(4), values, bodies)
	if !changedOK || changed == first {
		t.Fatal("presence bitmap did not participate in Result content identity")
	}
}
