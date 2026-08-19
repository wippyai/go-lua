package engine

import "testing"

func TestOccurrenceFootprintCoversEveryExactWriteTarget(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 8, nil, nil)
	for index, producer := range fixture.solver.runtime.producers {
		if producer.span.count() == 0 {
			t.Fatalf("producer %d has no sealed member span", index)
		}
		if len(producer.footprint) == 0 {
			t.Fatalf("producer %d has no occurrence footprint", index)
		}
		for factorIndex, occurrence := range producer.footprint {
			if !occurrence.key.Available() || len(occurrence.targets) == 0 {
				t.Fatalf("producer %d footprint %d has no exact target", index, factorIndex)
			}
			for targetIndex, target := range occurrence.targets {
				for prior := 0; prior < targetIndex; prior++ {
					if occurrence.targets[prior].Same(target) {
						t.Fatalf("producer %d footprint %d repeats target %d", index, factorIndex, targetIndex)
					}
				}
			}
		}
	}
}

func TestOccurrenceFootprintCoversWritesOfACarryClosureOnlyMember(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 16, nil, nil)
	for index, producer := range fixture.solver.runtime.producers {
		if len(producer.footprint) != 1 {
			t.Fatalf("producer %d footprint factors=%d, want one sealed factor", index, len(producer.footprint))
		}
		// The current row model stores the union that a carry closure reaches in
		// one occurrence row. A route/factor universe must not be duplicated per
		// member or per Region.
		if len(producer.footprint[0].targets) == 0 || len(producer.footprint[0].targets) > producer.span.count()+1 {
			t.Fatalf("producer %d footprint targets=%d span=%d", index, len(producer.footprint[0].targets), producer.span.count())
		}
	}
}
