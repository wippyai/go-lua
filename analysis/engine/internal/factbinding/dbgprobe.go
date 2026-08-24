package factbinding

// dbgprobe.go carries temporary structural counters for the correlated
// summary group product. The solve loop is single threaded, so the counters
// are plain fields.

// DbgFactBindingCounters records the Boolean volume of the correlated summary
// fold: how many declared keys extend an existing group set, how many of
// those keys are constant over the observed region, how many prefix/piece
// pairs the product visits, and how many region conjunctions those pairs
// issue. Conjunctions per pair is the measured quantity: a key constant over
// the observed region refines no prefix and must issue none.
type DbgFactBindingCounters struct {
	SummaryExtendKeys   uint64
	SummaryConstantKeys uint64
	SummaryPairs        uint64
	SummaryConjunctions uint64
	SummaryMaxPartials  uint64
}

var dbgFactBinding DbgFactBindingCounters

// DbgFactBinding returns the accumulated correlated summary counters.
func DbgFactBinding() DbgFactBindingCounters { return dbgFactBinding }

// DbgFactBindingReset clears the accumulated correlated summary counters.
func DbgFactBindingReset() { dbgFactBinding = DbgFactBindingCounters{} }
