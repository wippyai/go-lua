package collector

// SourceRoot is the Source construction capability of one Collector. It is
// deliberately only a root selector: semantic operations live on the small
// Source-owned leaves returned by Literals, Order, Keys, and Faults.
type SourceRoot struct{ collector *Collector }

// SourceLiterals owns authored scalar literal construction.
type SourceLiterals struct{ collector *Collector }

// SourceOrder owns authored Body/source order and binding-order construction.
type SourceOrder struct{ collector *Collector }

// SourceKeys owns only Name/List source spellings and admits their raw exact
// candidates through typed operations; it exposes no generic atom sink.
type SourceKeys struct{ collector *Collector }

// FlowRoot, StaticRoot, and ModuleRoot are narrow owner roots. Their
// vertical operations are implemented by the corresponding owner files; the
// Collector exposes no cross-owner semantic methods through these selectors.
type FlowRoot struct{ collector *Collector }
type StaticRoot struct{ collector *Collector }
type ModuleRoot struct{ collector *Collector }

// Source selects Source's construction capability without exposing the
// Collector's mutable row storage.
func (c *Collector) Source() SourceRoot { return SourceRoot{collector: c} }

// Flow selects Flow's construction capability.
func (c *Collector) Flow() FlowRoot { return FlowRoot{collector: c} }

// Static selects Static's construction capability.
func (c *Collector) Static() StaticRoot { return StaticRoot{collector: c} }

// Module selects Module's construction capability.
func (c *Collector) Module() ModuleRoot { return ModuleRoot{collector: c} }

// Literals selects Source's scalar literal leaf.
func (r SourceRoot) Literals() SourceLiterals { return SourceLiterals{collector: r.collector} }

// Order selects Source's Body/source-order leaf.
func (r SourceRoot) Order() SourceOrder { return SourceOrder{collector: r.collector} }

// Keys selects Source's raw-key/fault leaf.
func (r SourceRoot) Keys() SourceKeys { return SourceKeys{collector: r.collector} }

// Flow leaf selectors keep operation families explicit at call sites and
// prevent the Flow root from becoming a second semantic owner.
func (r FlowRoot) Values() FlowValuesWriter       { return FlowValuesWriter{collector: r.collector} }
func (r FlowRoot) Access() FlowAccessWriter       { return FlowAccessWriter{collector: r.collector} }
func (r FlowRoot) Storage() FlowStorageWriter     { return FlowStorageWriter{collector: r.collector} }
func (r FlowRoot) Tables() FlowTablesWriter       { return FlowTablesWriter{collector: r.collector} }
func (r FlowRoot) Functions() FlowFunctionsWriter { return FlowFunctionsWriter{collector: r.collector} }
func (r FlowRoot) Calls() FlowCallsWriter         { return FlowCallsWriter{collector: r.collector} }
func (r FlowRoot) Control() FlowControlWriter     { return FlowControlWriter{collector: r.collector} }
func (r FlowRoot) Operators() FlowOperatorsWriter { return FlowOperatorsWriter{collector: r.collector} }
func (r FlowRoot) Operands() FlowOperandsWriter   { return FlowOperandsWriter{collector: r.collector} }
