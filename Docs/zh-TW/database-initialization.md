# 資料庫初始化

本文說明如何建立 ReleaseHub 使用的 PostgreSQL 資料庫、執行版本化 migration，並驗證初始化結果。適用對象為開發人員、DBA 與負責首次部署或版本升級的平台維運人員。

ReleaseHub 目前以 PostgreSQL 16 作為整合測試基準。Schema 由 `Server/migrations/` 中的版本化 SQL 管理，並由獨立的 `releasehub migrate` command 執行。API 與 Worker 啟動時不會執行 migration，也不使用 GORM `AutoMigrate`。

## 執行原則

- 首次部署及每次 Server 版本升級前，先執行所有尚未套用的 migration。
- Migration 必須在 API 與 Worker 新版本啟動前成功完成；失敗時停止部署。
- 不得修改已在任何環境套用的 migration。Schema 變更必須新增下一個 `.up.sql` 與對應 `.down.sql`。
- 正式環境執行 migration 前，必須建立備份或快照，並驗證其可正常還原。
- 不要手動修改 `schema_migrations`、業務資料表或 migration 的 dirty 狀態。
- 目前 Migrate、API 與 Worker 使用同一組資料庫帳號設定，但各自使用獨立的 connection pool。若要拆分 DDL 與 runtime 帳號，必須先變更設定與權限模型。

## 1. 前置條件

執行前確認：

- PostgreSQL 16 可由執行 Migrate 的主機或 Pod 連線。
- 已準備 database 名稱、群組角色、登入角色與 Secret；本文使用 database `releasehub`、群組角色 `releasehub_group` 與登入角色 `releasehub_user`。
- Migration 帳號可在目標 database 建立 extension、table、index、function 與 trigger。
- PostgreSQL 提供 `pgcrypto` extension。第一個 migration 會執行 `CREATE EXTENSION IF NOT EXISTS pgcrypto`。
- 已取得與即將部署版本相同的 ReleaseHub Server binary 或 container image。

ReleaseHub 不需要額外的 ULID extension 或 tablespace。本程序會建立 `releasehub` schema，並將它設為 database 層級的 `search_path`；目前 migration 使用未限定 schema 的物件名稱，因此會在此 schema 建立物件。

## 2. 建立角色與資料庫

以 DBA 或 PostgreSQL 管理帳號開啟 `psql`：

```bash
psql -d postgres
```

建立共用群組角色與登入角色，再以互動方式設定登入密碼。下列權限刻意允許登入角色建立資料庫與角色，並管理 `releasehub_group` 的成員資格；只有確實需要這些管理能力時，才應完整採用此權限模型。

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

使用 `\password` 可保留所需的角色模型，也不會把明文密碼寫進本文或 SQL 命令歷程。正式環境應由 Secret 管理系統產生並注入密碼。

確認資料庫擁有者、角色成員資格、schema 與設定的 `search_path`：

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

結果必須顯示資料庫擁有者為 `postgres`、`is_member = true`、存在 `releasehub` schema，且資料庫設定包含 `search_path=releasehub`。本初始化流程只執行一次；角色或資料庫已存在時，未加條件判斷的 `CREATE ROLE` 與 `CREATE DATABASE` 會依設計回報失敗。

## 3. 確認 `pgcrypto`

連線至新資料庫，確認 PostgreSQL 伺服器可提供 `pgcrypto`：

```bash
psql -U releasehub_user -d releasehub
```

```sql
SELECT name, default_version, installed_version
FROM pg_available_extensions
WHERE name = 'pgcrypto';
```

查詢必須回傳 `pgcrypto`。Migration 會負責安裝尚未安裝的 extension；若受管 PostgreSQL 限制 migration 帳號安裝 extension，請由 DBA 先執行：

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

若 extension 不存在或無權建立，請先停止初始化，不要跳過第一個 migration。

## 4. 設定 Migration 連線

Migrate、API 與 Worker 使用相同的 typed YAML 設定。不含機密資料的完整範例位於 [`Server/configs/config.example.yaml`](../../Server/configs/config.example.yaml)，設定來源的優先順序請參閱[設定文件](configuration.md)。

本機初始化可在 repository 外建立不含 Secret 的 `/tmp/releasehub-migrate.yaml`：

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

再以 environment variable 注入密碼，不要把密碼寫入 YAML 或 repository：

```bash
cd Server

printf 'Database password: '
IFS= read -r -s RELEASEHUB_DATABASE_PASSWORD
printf '\n'
export RELEASEHUB_DATABASE_PASSWORD
```

`RELEASEHUB_REDIS_ADDRESS` 目前是共用設定驗證的必要欄位；Migrate command 不會建立 Redis 連線。正式環境必須提供實際設定，不應依賴本機範例值。

依環境設定 `database.ssl_mode`：上述本機範例未啟用 TLS，因此使用 `disable`；正式環境預設及部署範本使用 `require`。

## 5. 執行 Versioned Migrations

在 `Server/` 目錄執行：

```bash
go run ./cmd/releasehub migrate --config /tmp/releasehub-migrate.yaml
```

若使用已建置 binary：

```bash
./bin/releasehub migrate --config /tmp/releasehub-migrate.yaml
```

Migrate 會依版本編號套用 `Server/migrations/*.up.sql` 中所有尚未執行的檔案。SQL 已嵌入 Server binary，不需要在 runtime image 額外掛載 migration 目錄。

成功時，Migrate process 以 exit code `0` 結束，並輸出 `database migrations completed`。沒有待執行 migration 時也視為成功。

完成後清除 shell 中的 Secret：

```bash
unset RELEASEHUB_DATABASE_PASSWORD
```

## 6. Kubernetes 初始化

使用 repository 內的部署範本時，不需要另外手動啟動 Migrate Pod：

- Helm 將 `releasehub-migrate` 建立為 `pre-install,pre-upgrade` hook。
- Kustomize／Argo CD 將 `releasehub-migrate` 建立為 `PreSync` hook，並以 sync wave `-10` 執行。
- Migrate Job 使用與 Server 相同的 image、`/app/configs/config.yaml` 與 `releasehub-secrets`。

Repository 內的部署範本目前使用 `database.user: releasehub`。採用本文件的角色模型時，部署前必須在私有 Helm values 或 Kustomize overlay 將該值覆寫為 `releasehub_user`。

部署流程必須等待 Job 完成，成功後才啟動或更新 API 與 Worker。確認狀態：

```bash
kubectl get jobs -n releasehub \
  -l app.kubernetes.io/component=migrate

kubectl logs -n releasehub job/releasehub-migrate
```

成功訊號為 Job 顯示 `Complete`，且 log 包含 `database migrations completed`。若 Job 為 `Failed`，不得跳過 hook 或先行啟動新版本。

## 7. 驗證 Schema 與 Seed

Migration 完成後，以 `releasehub_user` 帳號連線並執行：

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

驗證條件：

- `schema_migrations.dirty` 為 `false`。
- `version` 等於目前 `Server/migrations/` 中最大的 `.up.sql` 版本編號。
- `pgcrypto` 查詢回傳一筆資料。
- `to_regclass` 的五個欄位都不是 `NULL`。
- `permission_count` 與 `role_count` 均大於 `0`。

## 8. 升級與回復

升級順序固定為：

1. 備份 database，並驗證備份可還原。
2. 使用新版本 Server image 執行 Migrate。
3. 確認 Migrate exit code、Job 狀態與 `schema_migrations`。
4. 部署同版本 API 與 Worker。
5. 執行服務健康檢查與必要的功能 smoke test。

目前 CLI 只提供向前執行的 `migrate` command，沒有正式環境使用的 `down` 或 `force` command。升級後若需要回復應用程式：

- Schema 與舊版應用程式相容時，回復舊版 API／Worker image，但保留已套用的 schema。
- Schema 不相容或 migration 部分失敗時，停止 API 與 Worker，依已演練程序還原升級前的 database backup。
- 若 migration 已在 production 成功完成，優先新增修正 migration 向前處理，不要改寫既有 SQL 或手動回退版本表。

## 9. 故障排除

### 無法建立 `pgcrypto`

確認 extension 在 `pg_available_extensions` 中，並由 DBA 安裝或授予符合環境政策的權限。不要刪除 migration 中的 `CREATE EXTENSION`。

### 連線或 TLS 失敗

檢查 `RELEASEHUB_DATABASE_SERVER`、port、database、user 與 `database.ssl_mode`。不要把完整 DSN 或密碼寫入 log、issue 或聊天紀錄。

### Migration 顯示 dirty

停止部署並保留失敗 log。比對目前 Server 版本、失敗的 migration 與資料庫實際物件，再決定向前修正或從備份還原。ReleaseHub 不提供 `force` command；不要直接修改 `schema_migrations`。

### 重跑 Migration

已成功套用的版本不會重複執行。修正連線或基礎設施問題後，可以用相同 Server 版本重新執行 `migrate`；成功時 process 以 exit code `0` 結束。
