-- kickside/converters/doc2md/src/app/env/doc2md_convert_test.lua: a local
-- wrapper forwards its arguments into the module's record parameters.
local doc2md = require("doc2md")

local fixtures = { docx = "docx-bytes", xlsx = "xlsx-bytes" }

local function convert(bytes, extract_images)
    return doc2md.convert({ bytes = bytes }, { extract_images = extract_images })
end

local plain, plain_err = convert(fixtures.docx, false)
local with_images, image_err = convert(fixtures.docx, true)
local sheet, sheet_err = convert(fixtures.xlsx, false)
return { plain, with_images, sheet, plain_err, image_err, sheet_err }
