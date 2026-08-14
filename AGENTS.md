# Repository Rules

## Scope and Precedence

- Root rules apply to this repository. Nested `AGENTS.md` files add stack-specific rules; the nearest applicable file wins on conflicts.
- Keep implementation-specific rules in the relevant nested `AGENTS.md`. Do not duplicate stack-specific details in this root file.

## Language and Communication

- Use Traditional Chinese (Taiwan) for user communication, documentation, commit messages, and PR titles/descriptions.
- Keep code in its existing language. Write comments and docstrings in Traditional Chinese; explain why, not line-by-line behavior.
- English is allowed for technical names, code, paths, URLs, API fields, and established terms.
- Report status first, then root cause, validation performed, and follow-up impact.
- State unknowns and assumptions explicitly. Never mark unverified work as complete.

## Specification Traceability

- Before implementing a new feature, an out-of-scope bug fix, or a requirement change, create or update the matching requirements, design, and unfinished task in `<spec-workspace>/<spec>/`.
- For an in-scope bug, confirm an existing task first. If none exists, add an unfinished task and wait for user scheduling before implementation.
- Keep requirements, design, tasks, implementation, and validation evidence traceable. Code-only completion claims are invalid.
- Do not append new work to a completed task while keeping it complete. Add a new unfinished task instead.
- Mark a task complete only when all listed acceptance criteria have evidence. Tools, drafts, plans, and unrun checks are not completion.
- The specification workspace is not a short list of reference files. Locate and use the spec that owns the requested feature or bug.

## Private Specification Workspace

- Internal requirements, designs, tasks, ADRs, operational documents, and business rules are maintained outside this public repository.
- Resolve the private workspace from `PRIVATE_SPEC_WORKSPACE`; expected specification layout: `$PRIVATE_SPEC_WORKSPACE/opscenter/.specs/<space>/`.
- Do not commit, copy, quote, or expose confidential specification content, operational data, internal identifiers, or business decisions in this public repository.
- When the private workspace is available, locate and use the owning specification before planning or implementing scoped work.
- When it is unavailable, state that limitation. Do not claim private specification review, create replacement internal documents here, or infer confidential requirements.


## Spec-Driven Development

- For new features, out-of-scope bug fixes, requirement changes, or user requests to plan/spec first, use the `sdd-skill`.
- The skill defines the workflow; do not duplicate its full procedure here.
- Project convention overrides the skill’s default paths and names:
  - Specification workspace: `$PRIVATE_SPEC_WORKSPACE/opscenter/.specs/`; use an explicitly approved local `.kiro/specs/` exception only when necessary.
  - Required files: `requirements.md`, `design.md`, and `task.md`
  - Use the owning existing spec space when available. Create a new space only when the change has no appropriate owner.
- If the skill and project convention differ, preserve the project workspace and filename convention while following the skill’s requirements, design, task-boundary, and verification principles.

## Frontend and Backend Contracts

- For shared API changes, update backend behavior, frontend schemas/types, OpenAPI, and the owning specification together.
- Frontend code must not recalculate backend-owned business metrics. Reports, BI views, and exports use the backend metric definition.
- Even single-layer changes require checking cross-layer contract and compatibility impact.
- Do not change existing API semantics, data definitions, or authorization behavior without confirmation.

## Implementation and Validation

- Inspect nearby code, types, configuration, and tests before editing. Follow existing project patterns; do not assume dependencies exist.
- Keep changes within the user request and task boundary. Do not bundle unrelated refactors.
- Run existing checks proportional to the change. Cross-layer changes require validation on both sides.
- Run `git diff --check` before handoff. Clearly report unavailable or skipped validation and its scope.

## Git, PRs, and Merges

- Use feature branches for features and fixes; default prefix: `codex/`.
- Commit and PR text must accurately describe verified scope. Do not use “complete” for unimplemented or unverified work.
- Do not commit, push, create/edit a PR, rebase, force-push, merge, switch branches, or pull without explicit user authorization for the current action.
- Before merging multiple PRs, check target branches, dependency order, migration/file conflicts, test state, and deployment impact; merge in the lowest-risk order.
- Do not use destructive Git commands such as `git reset --hard` or overwrite work without explicit approval.
- After merge, synchronize local `main` and report merge commits, outstanding deployment steps, and validation results.

## Project-Specific Index

- Add the project’s specification workspace path and active specification locations here when needed.
- Keep this index short; it is an entry point, not a full list of project files.
- Private specification root: `$PRIVATE_SPEC_WORKSPACE/opscenter/.specs/`
