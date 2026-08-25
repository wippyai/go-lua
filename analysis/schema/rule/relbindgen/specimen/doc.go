// Package specimen holds the binding specimens that prove the semantic ABI
// against real owner mathematics.
//
// Each specimen is what a generator will emit for one family and nothing more:
// a typed decoder, a typed encoder, one owner operation, and the owner column
// codecs the two need. There is no relational read, join, route, schedule,
// ticket, outcome settlement, form selection, or publication choreography
// here, and none is generable from these shapes.
//
// The four semantic classes named by the ABI are proven by four declarations
// over one binding constructor, not by four runtime forms:
//
//	scalar judgment    module-load value result, one scalar frame, one row
//	finite expansion   receiver route observation, owner-named rows, bounded
//	grouped reduction  value summary, one complete span, one folded row
//	cell update        heap ascent, read cell and proposal, same row
package specimen
