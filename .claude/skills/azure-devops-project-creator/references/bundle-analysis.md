# Bundle Analysis — Utility Functions

Functions used during Phases 1 and 2. Requires `pathlib` and the optional dependencies
listed in SKILL.md.

---

## scan_directory

Recursively lists all files under a root directory. Returns entries sorted by path.

```python
import pathlib

def scan_directory(root: str) -> list[dict]:
    return [
        {
            "path": str(p),
            "relative": str(p.relative_to(root)),
            "ext": p.suffix.lower(),
            "size_kb": round(p.stat().st_size / 1024, 1),
        }
        for p in sorted(pathlib.Path(root).rglob("*")) if p.is_file()
    ]
```

---

## read_file

Reads a file's text content based on its extension. Returns `None` for images and
unrecognised binary files. Returns an error string (not an exception) on read failure
so analysis can continue across the remaining files.

```python
def read_file(path: str) -> str | None:
    ext = pathlib.Path(path).suffix.lower()
    if ext in (".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico"):
        return None  # images: note existence only, no content
    try:
        if ext == ".md":
            return pathlib.Path(path).read_text(encoding="utf-8", errors="replace")
        elif ext == ".docx":
            import docx
            return "\n".join(p.text for p in docx.Document(path).paragraphs)
        elif ext == ".pdf":
            import fitz
            return "\n".join(page.get_text() for page in fitz.open(path))
        elif ext in (".xlsx", ".xls"):
            import openpyxl
            wb = openpyxl.load_workbook(path, data_only=True)
            return "\n".join(
                "\t".join(str(c) if c is not None else "" for c in row)
                for ws in wb.worksheets for row in ws.iter_rows(values_only=True)
            )
        else:
            return pathlib.Path(path).read_text(encoding="utf-8", errors="replace")
    except Exception as e:
        return f"[Could not read: {e}]"
```

---

## project_model

The working data structure assembled after reading all documents. Populated incrementally
during Phase 2 and completed during Phase 3.

```python
project_model = {
    "project_name": str,              # extracted from docs
    "process_template": str,          # default: "agile"
    "visibility": str,                # default: "private"
    "architecture_doc_source": str,   # path of the file that defines the repo list
    "repos": [
        {
            "name": str,              # exact name from architecture doc
            "description": str,
            "order": int,             # creation order per architecture doc
            "is_docs_repo": bool,     # True for the governance/docs repo
            "srs_file": str | None,   # local path of matched SRS/implementation plan
        }
    ],
    "project_docs": [
        {
            "local_path": str,        # source file on disk
            "repo_path": str,         # target path inside the docs repo
        }
    ],
    "landing_page_readme": str,        # generated README.md for the ADO default repo
    "unresolved": [str],              # descriptions of ambiguities requiring user input
}
```