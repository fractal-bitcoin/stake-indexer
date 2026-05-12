# Contributing

## Development workflow

1. Create a feature branch from `main`.
2. Keep changes focused and include tests when behavior changes.
3. Run `go test ./...` before opening a PR.
4. Open a PR with context, risk notes, and test evidence.

## Code style

- Follow idiomatic Go style.
- Prefer small, composable functions.
- Keep protocol parsing strict and explicit.

## Commit messages

Use concise, scope-aware messages, for example:

- `feat(protocol): add commission_rate parser`
- `fix(manager): handle empty indexer id`

## Pull request checklist

- [ ] Tests pass locally
- [ ] No secrets or private infra data committed
- [ ] Docs/config examples updated when needed
