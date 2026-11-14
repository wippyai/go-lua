package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestNestedTableWrite_SiblingFieldDoesNotCreateCycle(t *testing.T) {
	result := checkWithin(t, `
        type Image = {bytes: string}
        type Holder = {bytes: integer, clock: integer, entries: {[integer]: {image: Image, used: integer}}}
        local function read(source: {images: Holder}, page: integer, image: Image)
            local holder = source.images
            holder.clock = holder.clock + 1
            local cached = holder.entries[page]
            if cached then cached.used = holder.clock; return cached.image end
            local replaced = holder.entries[page]
            if replaced then
                holder.bytes = holder.bytes - #replaced.image.bytes
                holder.entries[page] = nil
            end
            while holder.bytes + #image.bytes > 100 do
                local oldest, used = nil, math.huge
                for number, entry in pairs(holder.entries) do
                    if entry.used < used then oldest, used = number, entry.used end
                end
                local removed = holder.entries[assert(oldest)]
                holder.bytes = holder.bytes - #removed.image.bytes
                holder.entries[oldest] = nil
            end
            holder.entries[page] = {image = image, used = holder.clock}
            holder.bytes = holder.bytes + #image.bytes
            return image
        end
        return {read = read}
    `)
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Errors))
	}
}
