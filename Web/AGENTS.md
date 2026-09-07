# ReleaseHub Frontend Rules

本文件補充repository root `AGENTS.md`，適用於`Web/`與其子目錄。

## Stack與結構

- 使用React、TypeScript、Vite、Ant Design與TanStack Query；加入dependency前先檢查既有package與bundle影響。
- 採feature-first結構；功能程式碼放在`src/features/<feature>/`，透過feature的`index.ts`公開必要介面。
- 應用組態、router與providers放在`src/app/`；跨feature共用能力放在`src/shared/`。
- OpenAPI產生碼放在`src/generated/`且不得手動修改。
- 使用`@/`path alias，禁止跨feature直接引用另一個feature的內部檔案。
- 保持strict type safety；禁止`any`、廣泛cast與忽略TypeScript錯誤。

## API與狀態

- API contract以root `API/openapi.yaml`為唯一來源，透過Orval產生TypeScript client與TanStack Query hooks。
- Server state使用TanStack Query；URL state使用React Router；表單使用Ant Design Form。
- 未經spec與設計確認，不引入Redux、Zustand或其他global state library。
- Frontend只負責呈現與輸入，不重新計算backend-owned status、permission或business result。
- 所有request畫面提供loading、empty、error與partial state，不得把失敗呈現成空資料或成功。

## UI、i18n與Theme

- 使用Ant Design components、Theme Tokens與CSS Modules；不得引入Material UI、Tailwind或Less。
- 支援English與繁體中文；專有名詞可保留英文，不維護zh-CN locale。
- 所有user-visible copy必須透過i18next resource，不得散落hard-coded copy。
- Theme支援system、light與dark，使用Ant Design algorithm與`prefers-color-scheme`。
- Form control必須具有label、validation、disabled與loading state。
- 維持keyboard操作、可見focus與WCAG 2.1 AA基本要求；dense view需處理長文字與小螢幕。

## Generated Code

- 執行`pnpm generate:api`從`../API/openapi.yaml`重新產生client。
- 不手動修改`src/generated/`；若產生結果錯誤，修正OpenAPI或Orval config。
- API變更需同步更新OpenAPI、Backend boundary、Frontend generated code、spec與tests。

## Verification

一般Frontend變更至少執行：

```bash
pnpm typecheck
pnpm test
pnpm build
```

OpenAPI變更另執行：

```bash
pnpm lint:api
pnpm generate:api
```

- 使用Vitest與React Testing Library測試component與hook，MSW模擬API，Playwright驗證關鍵流程。
- `git diff --check`必須通過；generated code需確認沒有非預期diff。
- 若檢查無法執行，需說明未驗證範圍與風險，不得宣稱完成。
