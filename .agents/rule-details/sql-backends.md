# SQL backends

Read `docs/backends.md`, `docs/generation.md` and the selected backend's README before changing persistence.

Each SQLite, MySQL and PostgreSQL backend owns a `db` subpackage. Plain models mirror its physical schema; embedded SQL owns schema changes; GORM Gen repositories provide runtime access. Use generated accessors and clauses for ordinary CRUD, counts, upserts and activity locks. Use custom queriers only for joins, correlated readiness and other SQL that needs them. Bind runtime values and scope every event, lease and deletion to the correct instance/execution pair.

Bind generated queries to the backend's own connection. Never introduce a global default query handle. Preserve worker ownership checks and transactional claims/checkpoints. PostgreSQL and MySQL row claims use read-committed transactions and skip-locked selection. SQLite needs immediate writer serialization.

Use common-go connection and migration helpers. Preserve caller-owned pools, existing migration filenames, SQL bytes and `schema_migrations`. Never call `AutoMigrate` or replay initial migrations over existing history. A new schema change needs a new paired migration, not an edit to published SQL.

Test real SQLite files and isolated PostgreSQL/MySQL services through the public backend API. Cover migration retries, pool ownership, legacy rows, cursor ties, rollback, forged leases, overlapping execution IDs and restart recovery. Binary rollback for the GORM transition uses unchanged schema, not destructive SQL DOWN scripts.
