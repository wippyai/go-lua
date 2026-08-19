package value

// dbgprobe.go carries temporary structural counters for the Value algebra.
// The solve loop is single threaded, so the counters are plain fields.

// DbgValueCounters records how often the Value algebra re-derives its
// construction invariant and how much image it scans doing so.
type DbgValueCounters struct {
	Owns        uint64
	Valid       uint64
	ValidRows   uint64
	LessOrEq    uint64
	LessOrEqRow uint64
	Join        uint64
	JoinBuild   uint64
	Equal       uint64
	MaxRows     uint64
}

var dbgValue DbgValueCounters

// DbgValue returns the accumulated Value algebra counters.
func DbgValue() DbgValueCounters { return dbgValue }

// DbgValueReset clears the accumulated Value algebra counters.
func DbgValueReset() { dbgValue = DbgValueCounters{} }
