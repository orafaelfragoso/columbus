## Summary

- 

## Change Type

- [ ] `feat` - user-visible feature
- [ ] `fix` - user-visible bug fix
- [ ] `perf` - performance improvement
- [ ] `refactor` - behavior-preserving code change
- [ ] `docs` - documentation only
- [ ] `test` - tests only
- [ ] `build` / `ci` / `chore` - tooling, release, or maintenance

## Scope Check

- [ ] This stays within Columbus' three responsibilities: index, search, memory.
- [ ] This does not add LLM calls, orchestration, hooks, guardrails, or agent policy.
- [ ] Any behavior change is covered by tests.

## Verification

- [ ] `make test`
- [ ] `make vet`
- [ ] `golangci-lint run ./...`
- [ ] `gofmt -l .` prints nothing

## Notes for Reviewers

- 
