# Contributing

Thanks for wanting to help. Issues and pull requests are welcome.

## Before you start

- For anything larger than a small fix, open an issue first and describe the problem, the
  behaviour you expect and how you would test it. Changes to the design go through a decision
  record (`DECISIONS.md`, `docs/adr/`).
- Read [STATUS.md](STATUS.md) for what is built and what is not verified yet, and
  [ARCHITECTURE.md](ARCHITECTURE.md) for how the program is put together.
- Coding agents follow [CLAUDE.md](CLAUDE.md) and [AGENTS.md](AGENTS.md); so do people.

## Building and testing

SessionKeep is one Go program without CGO. With Go installed:

    bash scripts/check.sh                 # formatting, vet, all tests; must print CHECK PASSED
    bash scripts/npm-pack.sh 1.2.3 all    # build the npm package for every platform

Every change needs tests: unit tests for the logic, and failure and security tests where it
touches capture, redaction, networking, encryption, permissions or process start-up. Say which
checks ran against real services and which against stand-ins.

## Rules that keep users safe

- Use made-up data in tests, fixtures and issues: never real conversations, source code,
  credentials or personal paths. The `_private/` folder is for local notes and is never committed.
- Capture must never block Claude Code, and nothing may be sent over the network unless the user
  turned that feature on. No telemetry.
- Do not weaken a test, a redaction rule or a consent step to make something pass.
- New dependencies need a reason, a pinned version and a compatible licence.
- Database migrations, once released, never change; add a new one instead.

## Licence

By contributing you agree that your contribution is licensed under the [MIT licence](LICENSE).
