# Database Initialization

This guide explains how to create the PostgreSQL database used by ReleaseHub, apply versioned migrations, and verify the result. It is intended for developers, DBAs, and platform operators responsible for initial deployment or upgrades.

ReleaseHub currently uses PostgreSQL 16 as its integration-test baseline. Versioned SQL files under `Server/migrations/` own the schema and are applied by the independent `releasehub migrate` command. API and Worker startup never runs migrations and does not use GORM `AutoMigrate`.

## Execution Rules

- Apply every pending migration before the first deployment and before each Server upgrade.
- Migrations must complete before the new API and Worker version starts. Stop the deployment on failure.
- Never edit a migration already applied in any environment. Add the next `.up.sql` and matching `.down.sql` for every schema change.
- Create a tested database backup or snapshot before migrating a production environment.
- Do not manually change `schema_migrations`, application tables, or the migration dirty state.
- Migrate, API, and Worker currently use the same database account configuration with separate connection pools. Splitting DDL and runtime accounts requires a configuration and permission-model change first.

## 1. Prerequisites

Confirm the following before starting:

- PostgreSQL 16 is reachable from the host or Pod that runs Migrate.
- A database name, group role, login role, and Secret are available. This guide uses database `releasehub`, group role `releasehub_group`, and login role `releasehub_user`.
- The migration account can create extensions, tables, indexes, functions, and triggers in the target database.
- PostgreSQL provides the `pgcrypto` extension. The first migration runs `CREATE EXTENSION IF NOT EXISTS pgcrypto`.
- The ReleaseHub Server binary or container image matches the version being deployed.

ReleaseHub does not require a separate ULID extension or tablespace. This procedure creates schema `releasehub` and sets it as the database-level `search_path`; current migrations use unqualified object names and therefore create their objects in that schema.

## 2. Create the Role and Database

Open `psql` with a DBA or PostgreSQL administrative account:

```bash
psql -d postgres
```

Create the shared group role and login role, then set the login password interactively. The privileges below intentionally allow the login role to create databases and roles and to administer membership in `releasehub_group`; use this exact model only when those administrative capabilities are required.

```sql
CREATE ROLE releasehub_group
    WITH NOLOGIN NOSUPERUSER INHERIT CREATEDB CREATEROLE
    NOREPLICATION VALID UNTIL 'infinity';

CREATE USER releasehub_user
    WITH NOSUPERUSER INHERIT CREATEDB CREATEROLE
    NOREPLICATION VALID UNTIL 'infinity';

\password releasehub_user

GRANT releasehub_group TO releasehub_user WITH ADMIN OPTION;

CREATE DATABASE releasehub
    WITH
    OWNER = postgres
    TEMPLATE = template0
    ENCODING = 'UTF8'
    LC_COLLATE = 'C'
    LC_CTYPE = 'C'
    CONNECTION LIMIT = -1
    IS_TEMPLATE = false;

GRANT ALL ON DATABASE releasehub TO releasehub_group WITH GRANT OPTION;
GRANT ALL ON DATABASE releasehub TO postgres;

\connect releasehub

CREATE SCHEMA IF NOT EXISTS releasehub AUTHORIZATION pg_database_owner;
COMMENT ON SCHEMA releasehub IS 'standard releasehub schema';
SET search_path TO releasehub;

GRANT USAGE ON SCHEMA releasehub TO releasehub_group;
GRANT ALL ON SCHEMA releasehub TO releasehub_group WITH GRANT OPTION;
GRANT ALL ON SCHEMA releasehub TO pg_database_owner;

ALTER DATABASE releasehub SET search_path TO releasehub;

ALTER DEFAULT PRIVILEGES FOR ROLE releasehub_user IN SCHEMA releasehub
    GRANT INSERT, SELECT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER
    ON TABLES TO releasehub_group;
ALTER DEFAULT PRIVILEGES FOR ROLE releasehub_user IN SCHEMA releasehub
    GRANT ALL ON SEQUENCES TO releasehub_group;
ALTER DEFAULT PRIVILEGES FOR ROLE releasehub_user IN SCHEMA releasehub
    GRANT ALL ON FUNCTIONS TO releasehub_group;
```

Using `\password` preserves the requested role model without placing plaintext credentials in this guide or SQL history. Production environments should generate and inject the password through their Secret-management system.

Verify the database owner, role membership, schema, and configured `search_path`:

```sql
SELECT datname, pg_get_userbyid(datdba) AS owner
FROM pg_database
WHERE datname = 'releasehub';

SELECT pg_has_role('releasehub_user', 'releasehub_group', 'MEMBER') AS is_member;

SELECT nspname, pg_get_userbyid(nspowner) AS owner
FROM pg_namespace
WHERE nspname = 'releasehub';

SELECT unnest(setconfig) AS database_setting
FROM pg_db_role_setting
WHERE setdatabase = (SELECT oid FROM pg_database WHERE datname = 'releasehub')
  AND setrole = 0;
```

The results must show database owner `postgres`, `is_member = true`, schema `releasehub`, and a database setting containing `search_path=releasehub`. Run this initialization once; plain `CREATE ROLE` and `CREATE DATABASE` statements intentionally fail if those objects already exist.

## 3. Verify `pgcrypto`

Connect to the new database and confirm that the server provides `pgcrypto`:

```bash
psql -U releasehub_user -d releasehub
```

```sql
SELECT name, default_version, installed_version
FROM pg_available_extensions
WHERE name = 'pgcrypto';
```

The query must return `pgcrypto`. Migrate installs the extension when it is not already installed. If a managed PostgreSQL service prevents the migration account from installing extensions, have a DBA run:

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

Stop initialization if the extension is unavailable or cannot be created. Do not skip the first migration.

## 4. Configure the Migration Connection

Migrate, API, and Worker use the same typed YAML configuration. The complete non-secret example is [`Server/configs/config.example.yaml`](../../Server/configs/config.example.yaml); see [Configuration](configuration.md) for source precedence.

For local initialization, create `/tmp/releasehub-migrate.yaml` outside the repository without any Secret:

```yaml
database:
  server: 127.0.0.1
  port: 5432
  user: releasehub_user
  password: ""
  database: releasehub
  ssl_mode: disable
  timezone: UTC

redis:
  address: 127.0.0.1:6379
```

Inject the password through an environment variable instead of storing it in YAML or the repository:

```bash
cd Server

printf 'Database password: '
IFS= read -r -s RELEASEHUB_DATABASE_PASSWORD
printf '\n'
export RELEASEHUB_DATABASE_PASSWORD
```

`RELEASEHUB_REDIS_ADDRESS` is currently required by shared configuration validation; the Migrate command does not open a Redis connection. Production must provide its real setting instead of relying on this local example.

Set `database.ssl_mode` for the environment. The local example above uses `disable` because it assumes PostgreSQL without TLS; production defaults and deployment templates use `require`.

## 5. Apply Versioned Migrations

Run the following from `Server/`:

```bash
go run ./cmd/releasehub migrate --config /tmp/releasehub-migrate.yaml
```

When using a built binary:

```bash
./bin/releasehub migrate --config /tmp/releasehub-migrate.yaml
```

Migrate applies every pending `Server/migrations/*.up.sql` file in version order. The SQL files are embedded in the Server binary, so the runtime image does not need a mounted migration directory.

On success, Migrate exits with code `0` and logs `database migrations completed`. Having no pending migration is also successful.

Remove the Secret from the shell after completion:

```bash
unset RELEASEHUB_DATABASE_PASSWORD
```

## 6. Kubernetes Initialization

The repository deployment templates run Migrate without a separately started Pod:

- Helm creates `releasehub-migrate` as a `pre-install,pre-upgrade` hook.
- Kustomize with Argo CD creates `releasehub-migrate` as a `PreSync` hook at sync wave `-10`.
- The Migrate Job uses the same image as Server, `/app/configs/config.yaml`, and `releasehub-secrets`.

The checked-in deployment templates currently use `database.user: releasehub`. When adopting the role model in this guide, override that value with `releasehub_user` in the private Helm values or Kustomize overlay before deployment.

The deployment must wait for the Job to finish before starting or updating API and Worker. Check its state with:

```bash
kubectl get jobs -n releasehub \
  -l app.kubernetes.io/component=migrate

kubectl logs -n releasehub job/releasehub-migrate
```

Success requires a `Complete` Job and a `database migrations completed` log entry. If the Job is `Failed`, do not bypass the hook or start the new version first.

## 7. Verify Schema and Seed Data

Connect as `releasehub_user` after Migrate completes and run:

```sql
SELECT version, dirty
FROM schema_migrations;

SELECT extname
FROM pg_extension
WHERE extname = 'pgcrypto';

SELECT
    to_regclass('releasehub.audit_logs') AS audit_logs,
    to_regclass('releasehub.outbox_events') AS outbox_events,
    to_regclass('releasehub.users') AS users,
    to_regclass('releasehub.applications') AS applications,
    to_regclass('releasehub.deployment_requests') AS deployment_requests;

SELECT COUNT(*) AS permission_count
FROM authorization_permissions;

SELECT COUNT(*) AS role_count
FROM authorization_roles;
```

The result is valid when:

- `schema_migrations.dirty` is `false`.
- `version` equals the largest `.up.sql` version currently under `Server/migrations/`.
- The `pgcrypto` query returns one row.
- None of the five `to_regclass` columns is `NULL`.
- Both `permission_count` and `role_count` are greater than `0`.

## 8. Upgrade and Recovery

Use this fixed upgrade order:

1. Back up the database and verify that the backup can be restored.
2. Run Migrate from the new Server image.
3. Verify the Migrate exit code, Job state, and `schema_migrations`.
4. Deploy API and Worker from the same version.
5. Run service health checks and the required functional smoke tests.

The CLI currently provides only the forward `migrate` command; it has no production `down` or `force` command. If the application must be rolled back after an upgrade:

- When the schema remains compatible, roll back the API and Worker image while retaining the migrated schema.
- When the schema is incompatible or a migration partially fails, stop API and Worker and restore the pre-upgrade database backup using the rehearsed recovery procedure.
- After a successful production migration, prefer a new corrective migration. Do not rewrite existing SQL or manually roll back the version table.

## 9. Troubleshooting

### `pgcrypto` Cannot Be Created

Confirm that the extension appears in `pg_available_extensions`, then have a DBA install it or grant permissions allowed by the environment policy. Do not remove `CREATE EXTENSION` from the migration.

### Connection or TLS Failure

Check `RELEASEHUB_DATABASE_SERVER`, port, database, user, and `database.ssl_mode`. Never include a complete DSN or password in logs, issues, or chat messages.

### Migration Is Dirty

Stop the deployment and retain the failure logs. Compare the Server version, failed migration, and actual database objects before choosing a forward fix or backup restore. ReleaseHub exposes no `force` command; do not edit `schema_migrations` directly.

### Rerunning Migrate

Successfully applied versions are not executed again. After fixing a connection or infrastructure failure, rerun `migrate` with the same Server version; a successful process exits with code `0`.
