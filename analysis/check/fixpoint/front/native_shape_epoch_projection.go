package front

import (
	"fmt"
	"strconv"
)

// shapeEpochRevocations is the exhaustive deopt class set of a receiver whose
// proven physical layout a field read establishes. A store to one of the
// receiver's fields ends the epoch (write.field); a field addition mints a new
// identity (shape.transition); a metatable install turns a direct slot read
// into a dispatch (meta.set); and an opaque callee can reach and reshape the
// receiver (call.opaque).
const shapeEpochRevocations = "write.field,shape.transition,meta.set,call.opaque"

// shapeEpochNativeFacts publishes the receiver-bound shape_identity rows front
// left to the engine: a formal record receiver that a field read observes and a
// store to one of its fields invalidates. The subject is bound to the receiver
// term, which the anchor scan cannot recover across a body boundary, so the row
// is derived here where the receiver name is directly available. The module-wide
// layout contract for the same layout is withheld by the front shape walk, so
// one physical layout is never counted twice.
func shapeEpochNativeFacts(root Compilation) []NativeProjection {
	var rows []NativeProjection
	forEachNativeBody(root, func(compilation Compilation) {
		for _, receiver := range ShapeEpochReceivers(compilation) {
			value := fmt.Sprintf("epoch=field_read field_offsets=identical interned=true shape_id=%016x stable=true", uint64(receiver.Shape))
			for read := 0; read < receiver.Reads; read++ {
				rows = append(rows, NativeProjection{
					Key: "shape_identity/" + fmt.Sprintf("%x", compilation.Body) + "/epoch/" +
						receiver.Display + "/" + strconv.Itoa(read) + "/contract-revocation/" + shapeEpochRevocations,
					Value: value, Subject: receiver.Display,
				})
			}
		}
	})
	return rows
}
