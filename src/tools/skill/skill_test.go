package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSkillMarkdown(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: demo
description: Demo skill
model: inherit
permissionMode: dontAsk
tools:
  - Read
maxTurns: 3
---

Hello from ${CLAUDE_SKILL_DIR}`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := ResolveSkill([]string{root}, "demo")
	if !ok {
		t.Fatalf("expected skill to resolve")
	}
	if def.Kind != "markdown" || def.Description != "Demo skill" || len(def.Tools) != 1 {
		t.Fatalf("unexpected definition: %#v", def)
	}
}

func TestSkillToolMarkdownRunsForked(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: demo
description: Demo skill
tools:
  - Read
disallowedTools:
  - Write
maxTurns: 2
---

Use ${CLAUDE_SKILL_DIR}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var got RunOptions
	tool := NewWithConfig(Config{
		Dirs:             []string{root},
		BaseSystemPrompt: []string{"base"},
		ParentModel:      "sonnet",
		ParentMode:       "default",
		Run: func(_ context.Context, opts RunOptions) (string, error) {
			got = opts
			return "done", nil
		},
	})
	res, err := tool.Call(context.Background(), map[string]any{"name": "demo", "args": "run it"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError || res.Content != "done" {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got.UserPrompt != "run it" || got.MaxTurns != 2 || len(got.AllowedTools) != 1 {
		t.Fatalf("unexpected run options: %#v", got)
	}
	if len(got.SystemPrompt) < 2 || !strings.Contains(got.SystemPrompt[len(got.SystemPrompt)-1], "Base directory for this skill:") {
		t.Fatalf("expected injected skill prompt, got: %#v", got.SystemPrompt)
	}
}

