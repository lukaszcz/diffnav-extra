# Repository Guidelines

DiffNav is a git diff pager based on delta with a GitHub-style file tree sidebar.

## Tech stack

- **Language:** Go 1.25.
- **TUI:** Charm `bubbletea` v2 / `lipgloss` v2 / `bubbles` v2 (from `charm.land`), with `bubblezone` for mouse hit-testing.
- **CLI:** `cobra` for command-line parsing, `fang` for styled help.
- **Diff rendering:** delegates to the external [`delta`](https://github.com/dandavison/delta) binary; diffnav parses its output and adds the file-tree sidebar.
- **Tooling:** `devbox` (Nix-backed) pins all dev tools; `go-task` is the task runner; `golangci-lint` v2 handles lint + fmt.

## Project Structure

- `main.go`, `cmd/root.go` — entrypoint and cobra root command.
- `pkg/ui/tui.go` — main bubbletea model; `pkg/ui/keys.go` — key bindings.
- `pkg/ui/panes/` — sub-views: `diffviewer` (delta output + selection), `filetree` (sidebar), `help`.
- `pkg/ui/common/` — shared styles, colors, zone IDs.
- `pkg/config/` — config loading; defaults in `cfg/`.
- `pkg/dirnode/`, `pkg/filenode/` — file-tree node types.
- `pkg/icons/` — nerd-font icon mapping per file type.
- `pkg/watch/` — file watcher used in dir-diff mode.
- `pkg/utils/`, `pkg/constants/`, `pkg/version/` — misc helpers, constants, build-stamped version.
- `examples/` — sample diffs used for manual testing; `demo.tape` — vhs demo script.

## Development Commands

Always invoke `task` through `devbox run` so the pinned `go`, `golangci-lint`, and other tools are on `$PATH`. Use `devbox shell` once if you want an interactive session that doesn't require the prefix.

- `devbox run task` — run diffnav (default task, with `DEBUG=true`).
- `devbox run task build` — compile all packages.
- `devbox run task test` — run tests.
- `devbox run task lint` — run golangci-lint.
- `devbox run task fmt` — apply formatters.
- `devbox run task check` — fmt + lint + test in sequence (the pre-merge gate).
- `devbox run task debug` — run with verbose logging; tail `./debug.log` with `devbox run task logs` in another terminal.

## Testing Guidelines

- **IMPORTANT**: Every new feature should include tests that verify its correctness at the appropriate levels (unit, integration, and possibly system level).
- **IMPORTANT**: Follow Test Driven Development (TDD). Write failing tests first, implement changes later to make the tests pass.
- **IMPORTANT**: For every bug found, add a regression test that fails because of the bug, then fix the bug and ensure the test passes.
- Avoid brittle tests. Test user workflows, not implementation details.
- Test only main app Go code, not build/install scripts, `Taskfile.yml` commands or config file content. Do not test exact help messages.
- Maintain 100% test coverage, no exemptions.

## Commit Guidelines

- Commit format: `type: subject` in imperative lowercase (e.g., `feat: add transfer flow`).
- Keep commits focused; avoid mixing unrelated changes.

## Instructions

- Avoid code duplication. Abstract common logic into parameterized functions.
- When finished, verify with `devbox run task check`.
