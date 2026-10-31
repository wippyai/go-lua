-- Source excerpt from app:pim_schema_api_test in spiralscout/pim/test.
local test = require("test")
local schema = require("schema")

local fam, ferr = schema.update_family("generic_product", {
    attributes = { "sku", "gtin", "brand", "title", "description", "main_image", "gallery", "weight", "price" },
    requirements = {
        { channel = "ecommerce", locale = "en_US", required_attributes = { "sku", "title", "price" } },
    },
}, "user:schema-admin")
test.is_nil(ferr)
test.eq(#fam.attributes, 9)
test.eq(#fam.requirements, 1)
test.eq(fam.requirements[1].channel_code, "ecommerce")
test.eq(fam.requirements[1].required_attributes[2], "title")
