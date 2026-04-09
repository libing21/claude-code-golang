# claude-code-running-go

Go 版 claude-code 的可运行同构工程（当前只支持 `--print` 非交互模式）。

## 快速开始

### 1) 配置鉴权与网关

需要设置 Base URL 和鉴权（二选一：API Key 或 Auth Token）。

```bash
export ANTHROPIC_BASE_URL="https://api.anthropic.com"
export ANTHROPIC_API_KEY="..."
# 或者：
# export ANTHROPIC_AUTH_TOKEN="..."
```

可选：默认模型

```bash
export ANTHROPIC_MODEL="sonnet"
```

### 2) 运行（非交互）

prompt 通过命令行参数传入（`--print` 是布尔开关，prompt 是剩余 args 拼接的字符串）：

```bash
go run cmd/claude-haha/main.go --print "你好，介绍一下这个仓库"
```

也可以通过 stdin 传入 prompt（适合 pipe）：

```bash
echo "总结一下这段代码做了什么" | go run cmd/claude-haha/main.go --print
```

## 常用参数

### 输出与调试

- `--print` / `-p`：非交互模式，打印回答并退出（Go 端口目前只支持这种模式）
- `--debug`：打印调试日志（system prompt、tool schema、messages、权限决策、工具执行等）
- `--messages-dump <dir>`：把每个 step 的 request/response/messages/tool_results/progress dump 到目录
  - 传 `--messages-dump 1` 会自动生成到 `.claude-go/messages-dump/<timestamp>/`
- `--dump-system-prompt`：打印最终 system prompt 并退出

示例：

```bash
go run cmd/claude-haha/main.go --print --debug --messages-dump 1 "用 ToolSearch 找到合适工具，然后解释原因"
```

### 模型与 API

- `--model <id>`：覆盖 `ANTHROPIC_MODEL`（例如 `sonnet/haiku/opus/best`）
- `--base-url <url>`：覆盖 `ANTHROPIC_BASE_URL`
- `--api-key <key>`：覆盖 `ANTHROPIC_API_KEY`
- `--auth-token <token>`：覆盖 `ANTHROPIC_AUTH_TOKEN`

### 权限与工具白/黑名单

权限模式：

- `--permission-mode default`：默认行为（可能会对敏感工具要求确认）
- `--permission-mode ask`：更倾向于在工具执行前询问/拦截（Go 端口目前没有交互式确认 UI，通常会返回 permission error）
- `--permission-mode bypass`：跳过权限确认（危险，谨慎使用）

工具 allow/deny：

- `--allowed-tools "Read,Glob,Write"`：只允许这些工具（逗号分隔）
- `--disallowed-tools "Bash,Edit"`：禁止这些工具（逗号分隔）

提示：如果你看到类似错误：

`permission ask: default prompt required (rerun with --permission-mode bypass, or allow the tool via --allowed-tools Write)`

说明本次工具被权限层拦截，通常需要：

- 直接绕过：`--permission-mode bypass`
- 或者显式放行：`--allowed-tools Write,Bash`（按需要添加）

示例：允许写文件

```bash
go run cmd/claude-haha/main.go --print --allowed-tools "Write,Bash,Read,Glob,Grep" \
  "把介绍 Skill 的文档写到 skill_intro.md"
```

## MCP / Plugins / Skills

### MCP

- `--mcp-config <path>`：指定 MCP 配置文件（例如 `.mcp.json`），为空则走默认搜索路径

### Plugins

两种方式指定插件目录：

- `--plugin-dirs "/a:/b"`：用 `:` 分隔多个目录
- `--plugin-dir "/a"`：可重复传多次

插件目录会被扫描以加载插件内的 `.mcp.json` / `plugin.json` 中的 `mcpServers`（如果存在）。

### Skills

- `--skill-dir <path>`：可重复传多次

如果不传 `--skill-dir`，`DiscoverSkills` 可能会返回 `No skill directories configured.`。

示例：

```bash
go run cmd/claude-haha/main.go --print --skill-dir "./skills" "列出本地可用 skills"
```

## 输出文件/状态文件位置

- `.claude-go/transcript.jsonl`：session transcript（可用于 debug/replay）
- `.claude-go/discovered-tools.json`：defer_loading discovered tools 持久化集合
- `.claude-go/messages-dump/...`：`--messages-dump` 输出目录

## 现状说明

- 当前 Go 端口 **不支持** 交互式 TUI 模式；请使用 `--print`。
- 权限模式 `ask/default` 在没有 UI 的情况下，遇到需要确认的工具会直接返回 permission error；需要显式 `--allowed-tools ...` 或 `--permission-mode bypass`。

