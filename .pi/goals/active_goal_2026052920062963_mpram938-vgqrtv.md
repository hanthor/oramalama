{
  "version": 3,
  "id": "mpram938-vgqrtv",
  "objective": "Get every non-tui package to 90%+ test coverage: progress(85→90), api(82→90), model(81→90), runtime(77→90), server(73→90), cli(33→90). CLI launch refactor required.",
  "status": "active",
  "autoContinue": true,
  "usage": {
    "tokensUsed": 560377,
    "activeSeconds": 1923
  },
  "sisyphus": true,
  "createdAt": "2026-05-29T19:06:29.636Z",
  "updatedAt": "2026-05-29T19:39:12.492Z",
  "activePath": ".pi/goals/active_goal_2026052920062963_mpram938-vgqrtv.md",
  "taskList": {
    "tasks": [
      {
        "id": "progress-90",
        "title": "Progress: 85% → 90%+ (spinner start/render edge cases)",
        "status": "complete",
        "completedAt": "2026-05-29T19:31:07.306Z",
        "verificationContract": "go test -cover ./internal/progress/ shows 90%+"
      },
      {
        "id": "api-90",
        "title": "API: 82% → 90%+ (client error path + stream edge cases)",
        "status": "pending",
        "verificationContract": "go test -cover ./internal/api/ shows 90%+"
      },
      {
        "id": "model-90",
        "title": "Model: 81% → 90%+ (SizeCheck GPU edge cases, JSON error paths)",
        "status": "pending",
        "verificationContract": "go test -cover ./internal/model/ shows 90%+"
      },
      {
        "id": "runtime-90",
        "title": "Runtime: 77% → 90%+ (EnsureServer remaining branches, Configure* error paths)",
        "status": "pending",
        "verificationContract": "go test -cover ./internal/runtime/ shows 90%+"
      },
      {
        "id": "server-90",
        "title": "Server: 73% → 90%+ (stream goroutine completion, remaining handler branches)",
        "status": "pending",
        "verificationContract": "go test -cover ./internal/server/ shows 90%+"
      },
      {
        "id": "cli-refactor",
        "title": "CLI: Refactor launch command — extract TUI picker from business logic",
        "status": "pending",
        "verificationContract": "Launch logic testable without real TUI; existing tests still pass"
      },
      {
        "id": "cli-90",
        "title": "CLI: Test extracted launch logic + push to 90%+",
        "status": "pending",
        "verificationContract": "go test -cover ./internal/cli/ shows 90%+"
      },
      {
        "id": "verify",
        "title": "Verify final board — every non-tui package at 90%+",
        "status": "pending",
        "verificationContract": "All packages at 90%+: go test -cover ./internal/config/ ./internal/progress/ ./internal/api/ ./internal/model/ ./internal/runtime/ ./internal/server/ ./internal/cli/"
      }
    ],
    "blockCompletion": false,
    "proposedAt": "2026-05-29T19:08:12.081Z"
  }
}

# Goal Prompt

Get every non-tui package to 90%+ test coverage: progress(85→90), api(82→90), model(81→90), runtime(77→90), server(73→90), cli(33→90). CLI launch refactor required.

## Progress

- Status: sisyphus running
- Auto-continue: on
- Sisyphus mode: yes (prompt/criteria style)
- Time spent: 32m03s
- Tokens used: 560K (560,377) tokens
## Tasks

<!-- blockCompletion: false -->
- [x] progress-90: Progress: 85% → 90%+ (spinner start/render edge cases)
- [ ] api-90: API: 82% → 90%+ (client error path + stream edge cases) — contract: go test -cover ./internal/api/ shows 90%+
- [ ] model-90: Model: 81% → 90%+ (SizeCheck GPU edge cases, JSON error paths) — contract: go test -cover ./internal/model/ shows 90%+
- [ ] runtime-90: Runtime: 77% → 90%+ (EnsureServer remaining branches, Configure* error paths) — contract: go test -cover ./internal/runtime/ shows 90%+
- [ ] server-90: Server: 73% → 90%+ (stream goroutine completion, remaining handler branches) — contract: go test -cover ./internal/server/ shows 90%+
- [ ] cli-refactor: CLI: Refactor launch command — extract TUI picker from business logic — contract: Launch logic testable without real TUI; existing tests still pass
- [ ] cli-90: CLI: Test extracted launch logic + push to 90%+ — contract: go test -cover ./internal/cli/ shows 90%+
- [ ] verify: Verify final board — every non-tui package at 90%+ — contract: All packages at 90%+: go test -cover ./internal/config/ ./internal/progress/ ./internal/api/ ./internal/model/ ./internal/runtime/ ./internal/server/ ./internal/cli/

