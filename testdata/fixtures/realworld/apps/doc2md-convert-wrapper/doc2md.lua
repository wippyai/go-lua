local doc2md = {}

function doc2md.convert(input: {bytes: string}, options: {extract_images: boolean?}?): ({markdown: string, images: {any}}?, string?)
    if input.bytes == "" then
        return nil, "empty document"
    end
    local images = {}
    if options and options.extract_images then
        table.insert(images, { name = "image.png", data = input.bytes })
    end
    return { markdown = input.bytes, images = images }, nil
end

return doc2md
