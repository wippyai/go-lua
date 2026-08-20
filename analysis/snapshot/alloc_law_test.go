package snapshot

import "testing"

// Sinks keep the measured reads observable so a compiler cannot delete the
// work being measured. They are concrete types, so storing into them never
// allocates and never contaminates the measurement.
var (
	sinkInt    int
	sinkRecord record
	sinkStatus ReadStatus
)

// TestReadsAllocateNothing is the cost law of a point read. Every read
// outcome runs in zero allocations: the axis carries no storage, and the
// column pointer is recovered out of the erased slot without boxing the key
// or the value. A read that allocated would price the authoritative path above
// a local shortcut, which is the one thing the incentive law forbids.
func TestReadsAllocateNothing(t *testing.T) {
	sealed := newFixture(t)

	cases := []struct {
		name string
		want ReadStatus
		read func()
	}{
		{
			name: "read hit",
			want: ReadHit,
			read: func() { sinkInt, sinkStatus = Read(&sealed, totalAxis, "present") },
		},
		{
			name: "read proven absence",
			want: ReadProvenAbsent,
			read: func() { sinkInt, sinkStatus = Read(&sealed, totalAxis, "absent") },
		},
		{
			name: "read miss",
			want: ReadMiss,
			read: func() { sinkInt, sinkStatus = Read(&sealed, totalAxis, "unknown") },
		},
		{
			name: "read miss without denominator",
			want: ReadMiss,
			read: func() { sinkInt, sinkStatus = Read(&sealed, partialAxis, "absent") },
		},
		{
			name: "read rejection",
			want: ReadInvalid,
			read: func() {
				sinkInt, sinkStatus = Read(&sealed, Axis[string, int]{SchemaID: fixtureOtherSchema}, "present")
			},
		},
		{
			name: "read wide value hit",
			want: ReadHit,
			read: func() { sinkRecord, sinkStatus = Read(&sealed, recordAxis, 5) },
		},
		{
			name: "read wide value miss",
			want: ReadMiss,
			read: func() { sinkRecord, sinkStatus = Read(&sealed, recordAxis, 6) },
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(1000, testCase.read); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
			if sinkStatus != testCase.want {
				t.Fatalf("status = %v, want %v", sinkStatus, testCase.want)
			}
		})
	}

}

// BenchmarkRead reports the point-read cost the law above fixes at zero
// allocations, so a regression shows up as bytes per operation rather than
// only as a failing law.
func BenchmarkRead(b *testing.B) {
	sealed := newFixture(b)
	b.Run("hit", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Read(&sealed, totalAxis, "present")
		}
	})
	b.Run("miss", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Read(&sealed, totalAxis, "unknown")
		}
	})
	b.Run("proven-absent", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Read(&sealed, totalAxis, "absent")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		rejected := Axis[string, int]{SchemaID: fixtureOtherSchema, Slot: totalAxis.Slot}
		b.ReportAllocs()
		for range b.N {
			sinkInt, sinkStatus = Read(&sealed, rejected, "present")
		}
	})
}
