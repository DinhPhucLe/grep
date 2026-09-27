# Mock CSV → Mongo (one-shot, gitignored)

Streams `mock_dev_knowledge_base.csv` row-by-row into `knowledge_documents`.

**File:** 2000 rows, ~3.4MB. Columns: `id`, `doc_type`, `title`, `body`,
`accepted_answer`, `tags`, `primary_technology`, `category`, `author`,
`created_date`, `score`, `view_count`, `answer_count`, `is_answered`,
`has_code`, `content`.

Env (from `server/.env` is fine):

- `MONGODB_URI`, `MONGODB_DATABASE` (and `DB_USERNAME`/`DB_PASSWORD` if placeholders)
- optional: `MOCK_CSV_PATH`, `SO_IMPORT_LIMIT` (default 1500), `ORGANIZATION_ID`

```bash
cd scripts/oneoff/so_cloud_to_mongo
go mod tidy
go run .
```
