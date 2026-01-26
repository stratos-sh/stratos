---
name: document-feature
description: Document a feature or change that was just implemented. Use when the user says "document this", "add docs", "update the docs for this", or similar after completing implementation work. This skill uses conversation context to understand what was built and creates/updates appropriate documentation.
---

# Document Feature

Document the feature or change from the current conversation.

## Workflow

1. **Identify what to document** from conversation context:
   - What was just implemented/changed?
   - Which files were modified?
   - What's the user-facing impact?

2. **Determine documentation scope**:
   - New feature → May need new doc or new section
   - API change → Update `docs/reference/api/`
   - Config change → Update relevant guide
   - Bug fix → Usually no docs needed unless behavior changed

3. **Create/update documentation**:
   - Follow existing style in `docs/`
   - Use Docusaurus frontmatter
   - Add to `docs/sidebars.js` if creating new files
   - Pull examples from `config/samples/` when available

4. **Verify the build**:
   ```bash
   cd docs && npm install && npm start
   ```
   Fix any MDX errors (watch for `<`, `>`, `<=` outside code blocks).

5. **Report what was done** and ask if adjustments needed.

## Documentation Standards

- **Frontmatter**: Every doc needs `sidebar_position`, `title`, `description`
- **Code blocks**: Specify language, use `title="filename"` when helpful
- **Admonitions**: Use `:::note`, `:::warning`, `:::tip` for callouts
- **Cross-refs**: Use relative paths like `../concepts/node-lifecycle.md`

## Sources of Truth

- CRD definitions: `api/v1alpha1/*.go`
- Controller flags: `cmd/stratos/main.go`
- Sample configs: `config/samples/`
- Labels/annotations: `internal/controller/state.go`

## MDX Gotchas

Characters that break MDX outside code blocks:
- `<` `>` `<=` `>=` → Use `≤` `≥` or wrap in backticks
- Curly braces `{}` → Escape or use code blocks
