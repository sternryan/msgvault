# MBOX Import Script (Prototype)

I've prototyped an MBOX importer that injects messages directly into the `msgvault` SQLite database. This bypasses the need for an intermediate format and works with standard MBOX exports (e.g., Google Takeout).

### Features
*   **Direct SQLite Injection:** populates `sources`, `conversations`, `messages`, `participants`, `message_recipients`, and `message_bodies`.
*   **Threading:** Uses `X-GM-THRID` (Gmail) or falls back to Subject-based threading.
*   **Deduplication:** Uses `Message-ID`.
*   **Participants:** Extracts and normalizes names/emails for `participants` table.

### Usage
```bash
python3 import_mbox.py path/to/archive.mbox path/to/msgvault.db
```

### Script (`import_mbox.py`)

(See attached file `import_mbox.py` in the repo)
