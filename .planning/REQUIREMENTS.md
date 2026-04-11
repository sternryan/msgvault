# Requirements: msgvault

**Defined:** 2026-04-11
**Core Value:** Users can safely archive their entire Gmail history offline and search it instantly, with confidence that nothing is lost before deletion.

## v1.2 Requirements

Requirements for AI Archive Intelligence milestone. Each maps to roadmap phases.

### Embeddings & Vector Search

- [ ] **EMBED-01**: User can batch-embed all messages via Azure OpenAI text-embedding-3-small
- [ ] **EMBED-02**: Embeddings stored in sqlite-vec table alongside message IDs
- [ ] **EMBED-03**: User can run semantic search queries from CLI (`msgvault search --semantic "query"`)
- [ ] **EMBED-04**: User can run semantic search from web UI with results ranked by similarity
- [ ] **EMBED-05**: Hybrid search combines FTS5 keyword matches with vector similarity, re-ranked

### AI Enrichment

- [ ] **ENRICH-01**: User can auto-categorize all messages (finance, travel, legal, health, shopping, newsletters, personal, work)
- [ ] **ENRICH-02**: Categories stored as AI-generated labels in existing label system
- [ ] **ENRICH-03**: User can filter by AI categories in TUI and web UI
- [ ] **ENRICH-04**: User can extract life events (jobs, moves, purchases, travel, milestones) from messages
- [ ] **ENRICH-05**: Life events exported in LifeVault-compatible format (JSON with date, type, description, source_message_id)
- [ ] **ENRICH-06**: User can extract entities (people, companies, dates, amounts) from message content
- [ ] **ENRICH-07**: Entities stored in searchable table with back-references to source messages

### Batch Pipeline

- [ ] **PIPE-01**: Azure OpenAI config section in config.toml (endpoint, API key reference, deployment names)
- [ ] **PIPE-02**: Batch processing with checkpoint-based resumability (resume after interruption)
- [ ] **PIPE-03**: Progress display with message count, cost estimate, rate, and ETA
- [ ] **PIPE-04**: Rate limiting respects Azure OpenAI TPM/RPM quotas

## v1.3 Requirements (Future)

### Relationship Intelligence

- **REL-01**: Relationship graph extraction (who emails whom, frequency, topics)
- **REL-02**: Cross-account entity deduplication (same person across Gmail, Outlook, iMessage)
- **REL-03**: D3.js or similar visualization of relationship graph in web UI

### Local Processing

- **LOCAL-01**: Tesseract OCR integration for PDF/image attachments
- **LOCAL-02**: Extracted text indexed in FTS5 for full-text search across attachment content

### Training Pipeline

- **TRAIN-01**: DPO training pair generation via GPT-4o for vaulttrain-stern
- **TRAIN-02**: Writing style analysis across accounts and time periods

## Out of Scope

| Feature | Reason |
|---------|--------|
| Azure AI Search | Monthly billing eats credits; FTS5 + sqlite-vec is sufficient |
| Speech-to-text on voicemails | Only 6 audio attachments; not worth integration effort |
| Azure-hosted inference | All processing local except API calls; no cloud deployment |
| Real-time enrichment during sync | Batch post-processing is simpler and cheaper |
| App-level encryption | Deferred from v1.0; orthogonal to AI features |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| EMBED-01 | — | Pending |
| EMBED-02 | — | Pending |
| EMBED-03 | — | Pending |
| EMBED-04 | — | Pending |
| EMBED-05 | — | Pending |
| ENRICH-01 | — | Pending |
| ENRICH-02 | — | Pending |
| ENRICH-03 | — | Pending |
| ENRICH-04 | — | Pending |
| ENRICH-05 | — | Pending |
| ENRICH-06 | — | Pending |
| ENRICH-07 | — | Pending |
| PIPE-01 | — | Pending |
| PIPE-02 | — | Pending |
| PIPE-03 | — | Pending |
| PIPE-04 | — | Pending |

**Coverage:**
- v1.2 requirements: 16 total
- Mapped to phases: 0
- Unmapped: 16 ⚠️

---
*Requirements defined: 2026-04-11*
*Last updated: 2026-04-11 after initial definition*
