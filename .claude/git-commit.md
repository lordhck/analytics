# Git Commit Rules

These rules define how commits MUST be written and structured.
They are optimized for both humans and automated agents.

---

## 1. Commit Format (MUST)

All commits MUST follow the Conventional Commits specification:

`<type>(<scope>): <short description>`

### Examples
- `feat(cli): add pipx installation support`
- `fix(docker): resolve permission issue`
- `chore: update dependencies`

---

## 2. Allowed Types (MUST)

Only the following types are allowed:

- feat     → new feature
- fix      → bug fix
- docs     → documentation changes
- style    → formatting, no code change
- refactor → code change without behavior change
- test     → adding or updating tests
- chore    → maintenance tasks

---

## 3. Scope (SHOULD)

- Scope is OPTIONAL but STRONGLY encouraged
- Scope SHOULD describe the affected area

### Examples
- `feat(cli): ...`
- `fix(api): ...`
- `refactor(docker): ...`

---

## 4. Title Rules (MUST)

- MUST be ≤ 50 characters
- MUST be lowercase
- MUST NOT end with a period
- MUST be concise and descriptive

### Good
`feat(cli): add pipx support`

### Bad
`feat(cli): Add pipx support.`
`update stuff`

---

## 5. Commit Body (SHOULD)

A commit body SHOULD be included when:

- The change is not obvious
- The change is complex
- Context or reasoning is important

### Format

`<short description>`

Optional extended explanation of WHY the change was made.

---

## 6. Breaking Changes

Breaking changes MUST be indicated using `!`:

`<type>(<scope>)!: <description>`

### Example
`feat(api)!: change authentication flow`

---

## 7. Commit Granularity (MUST)

- ONE commit MUST represent ONE logical change
- Commits MUST be atomic and focused
- Large mixed changes are NOT allowed

---

## 8. DO NOT

The following are strictly forbidden:

- vague messages (e.g. "update stuff", "fix bug")
- multiple logical changes in one commit
- committing broken or non-working code
- meaningless commit history
- skipping commit types
- using uppercase in titles
- adding trailing periods

---

## 9. Agent Rules

- Agents MUST follow ALL rules above
- Agents MUST generate deterministic commit messages
- Agents MUST NOT push commits
- Agents MAY create commits locally only

---

## 10. History Rewriting

- Commits MAY be amended BEFORE push
- Rewriting history AFTER push is NOT allowed

---
- Agents NEVER push commits, Humans do.