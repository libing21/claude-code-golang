---
name: count-go-lines
description: Count source lines in the current repository and summarize the largest files.
model: inherit
permissionMode: dontAsk
tools:
  - Glob
  - Read
  - Grep
  - Bash
maxTurns: 3
---

Count source lines for common code files in the current repository.

Base expectations:
- Prefer using Bash for the final line count because `wc -l` is the fastest way to aggregate many files.
- You may use Glob/Grep/Read first to confirm the repository layout before running the command.
- Exclude common dependency directories such as `vendor` and `node_modules`.
- Return:
  1. a short summary,
  2. the total line count,
  3. the top 10 largest source files by line count.

Suggested Bash approach:

```bash
find . -type f \( -name "*.go" -o -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" \) \
  -not -path "*/vendor/*" -not -path "*/node_modules/*" | sort | xargs wc -l | sort -n
```

