package control

import "testing"

func TestControlScheduleRejectsAbsentReturnBeforeQueueing(t *testing.T) {
	var writer Writer
	if err := writer.ScheduleReturn(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("ScheduleReturn accepted an absent statement")
	}
}
