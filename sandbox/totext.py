#!/usr/bin/python3

def text(path):
    ext = str(path).rsplit(".", 1)[-1].lower()
    out = []
    if ext == "pdf":
        raise ValueError("Use read_documents to read original PDFs")
    elif ext == "pptx":
        import pptx
        for i, slide in enumerate(pptx.Presentation(path).slides, 1):
            out.append(f"--- slide {i} ---")
            for shape in slide.shapes:
                if shape.has_text_frame:
                    out.append(shape.text_frame.text)
                elif shape.has_table:
                    out += [_row(cell.text for cell in row.cells) for row in shape.table.rows]
            notes = slide.notes_slide.notes_text_frame.text.strip() if slide.has_notes_slide else ""
            if notes:
                out.append("notes: " + notes)
    elif ext == "docx":
        import docx
        for block in docx.Document(path).iter_inner_content():
            if isinstance(block, docx.table.Table):
                out += [_row(cell.text for cell in row.cells) for row in block.rows]
            else:
                out.append(block.text)
    else:
        with open(path, errors="replace") as f:
            return f.read()
    return "\n".join(out) + "\n"


def _row(cells):
    out = []
    for c in cells:
        c = " ".join((c or "").split())
        if c and (not out or out[-1] != c):
            out.append(c)
    return " | ".join(out)
