# Columbus — Brand & Messaging Kit

> The map your coding agent has been missing.

A working source-of-truth for how Columbus talks about itself. Audience-first
for **AI-agent power users** (devs running Claude Code / Cursor / Codex in the
terminal). Positioned against three enemies: **agents fumbling in the dark**,
**context-file pollution**, and **inefficient context gathering**.

---

## 1. Positioning statement

**For** developers who pair with AI coding agents
**Who** watch their agent burn tokens grepping the repo, re-learning the
codebase every session, and leaning on a junk drawer of stale `.md` context
files,
**Columbus** is a local, deterministic code-context server the agent calls as a
tool.
**Unlike** naive grep-and-read exploration or fuzzy embedding search,
**Columbus** returns ranked, LLM-ready context with exact line ranges that are
*always* current — and owns the project's durable memory so the agent stops
re-discovering and starts remembering.

One sentence: **Columbus is the map and compass for your coding agent —
deterministic, always current, local-only.**

---

## 2. Brand foundation

### The name
Columbus is a navigator. He doesn't write the journey; he charts the territory
so the journey is possible. That's the exact relationship: **your agent is the
explorer; Columbus is the cartography.** The agent decides where to go.
Columbus makes sure the map is accurate.

The metaphor gives us a clean, ownable vocabulary — use it consistently:

- **Chart / map** — the index
- **Navigate / locate** — search
- **Logbook / ship's log** — durable memory (decisions, epics, tasks)
- **Compass** — deterministic ranking (always points the same way)
- **Dead reckoning vs. real coordinates** — guessing from stale context vs.
  reading the live working tree

### Brand values (these come *from the product*, not bolted on)
The rare gift here: Columbus's engineering invariants **are** its brand. We
don't have to invent a personality — we describe the software honestly.

1. **Deterministic.** Same query, same answer. No dice rolls. No LLM in the
   loop. This is also how we talk: precise, repeatable, no hand-waving.
2. **Honest about state.** The DB never stores code — snippets are rebuilt live
   from the working tree, so Columbus *cannot* show you something stale. The
   brand never oversells for the same reason.
3. **Local-only.** Your code never leaves the machine. No embeddings shipped to
   a cloud, no API bill, no telemetry.
4. **Small & sharp.** One Go binary. `git` is the only hard dependency. It does
   three things and refuses to do a fourth.

### Brand personality
The calm, precise navigator. Senior-engineer energy: confident because it's
correct, not because it's loud. Think `ripgrep` / `fzf` / `sqlite` — tools that
earn trust by being unglamorous and *right*, not by shouting.

---

## 3. Voice & tone

### Principles
- **Show the mechanism, not the magic.** Our differentiator is that there's no
  magic. "Snippets are reconstructed live by re-parsing the working tree" beats
  "AI-powered context." Always prefer the concrete mechanism.
- **Anti-hype by design.** No "revolutionary," "10x," "game-changing,"
  "supercharge," "unleash." The product's whole pitch is *trustworthiness*;
  hype actively undercuts it.
- **Peer-to-peer, terminal-native.** Write to an engineer who lives in a shell.
  Short sentences. Real commands. Copy-pasteable.
- **Earn every claim.** If we can't show it in a command or a diff, we don't
  say it. "Deterministic" is provable — `columbus search X` twice, diff the
  output. Lean on claims like that.
- **Let determinism be the joke and the flex.** Dry wit is on-brand; exclamation
  marks are not.

### Tone by surface
| Surface | Tone |
|---|---|
| README / docs | Precise, declarative, example-first |
| Landing page | Confident, benefit-led, still concrete |
| Social / launch posts | Dry, sharp, one idea per post |
| CLI output & errors | Terse, actionable, never cute |
| Changelog | Factual, Conventional-Commits style |

### Word bank
**Use:** deterministic · current · live · local-only · ranked · LLM-ready ·
exact line ranges · durable memory · re-parse the working tree · metadata + git
anchors · contract · projection · chart · navigate · logbook.

**Avoid:** AI-powered · smart · magic · revolutionary · seamless · supercharge ·
next-gen · effortless · blazing-fast (just say how fast) · "leverage synergies"
and every cousin of that.

### Voice in one diff
> ❌ "Columbus uses cutting-edge AI to intelligently surface the most relevant
> code, supercharging your agent's understanding of your codebase!"
>
> ✅ "Your agent asks `columbus search "parse config"` and gets ranked results
> with exact line ranges — reconstructed live from your working tree, so they're
> never stale. No embeddings. No cloud. Same query, same answer, every time."

---

## 4. The problem (the three enemies)

Frame every top-of-funnel message around one of these. They are *felt*, daily
pains for the target user.

### Enemy 1 — Agents fumbling in the dark
Without a map, the agent explores by guessing: `grep`, read a whole file,
`grep` again, read another. It's non-deterministic (different path every run),
slow, and it pollutes its own context window with files it didn't need. You
watch it spend a third of a session just *finding* the thing before it can
change it.

### Enemy 2 — Context-file pollution
To compensate, we litter the repo: `.cursorrules`, three competing `*.md`
"context" files, a `docs/` folder that drifted out of date six months ago. The
agent dutifully reads them — and half of what they say is now a lie. Stale
context is *worse* than no context, because it's confidently wrong.

### Enemy 3 — Inefficient context gathering
Even when it finds the right file, the agent slurps the whole thing to use ten
lines. Every new session re-discovers what the last one already learned.
Tokens, latency, and your money, spent re-reading instead of reasoning.

**The throughline:** the agent is brilliant at *reasoning* and bad at
*remembering and locating*. Columbus takes the second job off its plate.

---

## 5. What Columbus is (the clear explanation)

Columbus is a **local-only, deterministic code-context server** that a coding
agent calls as a tool. It does exactly three things:

1. **Index** — charts the codebase with embedded tree-sitter. Stores *metadata
   and git anchors only* — never your code.
2. **Search** — returns ranked, LLM-ready context with exact line ranges,
   optionally with the 1-hop dependency graph.
3. **Memory** — owns the project's durable record: decisions, plus structured
   epics & tasks with history, references, and drift checks.

What it deliberately is **not**: it doesn't call an LLM, doesn't orchestrate the
agent, doesn't gate or enforce anything. Ranking, "why relevant," and risk hints
are deterministic heuristics. Orchestration and guardrails live in the agent;
Columbus is the trustworthy data layer underneath.

**The one detail that sells it:** the database is a *cache of metadata and git
anchors* — never a content store. Every snippet and line range is reconstructed
**live** at query time by re-parsing the working tree. So Columbus's answers
always reflect the code as it is *right now*. It structurally cannot go stale.

---

## 6. Benefits — "better than X"

### vs. naive agent exploration (grep + read whole files)
| Without Columbus | With Columbus |
|---|---|
| Different exploration path every run | Same query → same ranked answer |
| Reads whole files to use ten lines | Exact line ranges, only what's relevant |
| Re-discovers the codebase each session | Index + logbook persist across sessions |
| Finding-cost eats the context window | Finding is one tool call |

### vs. context-file pollution (`.cursorrules`, stray `*.md`, stale docs)
| Junk-drawer context | Columbus memory |
|---|---|
| Drifts silently; lies confidently | `validate` flags evidence drift & broken links |
| Unstructured prose | Structured: decisions, epics, tasks, refs, history |
| Clutters the repo & git history | `.columbus.json` is git-excluded; memory is queryable, not committed noise |
| Agent must read it all to use any of it | Agent *searches* memory like it searches code |

### vs. embedding / RAG code search (the considered alternative)
| Fuzzy vector search | Columbus |
|---|---|
| Non-deterministic, similarity-ranked | Deterministic heuristic ranking |
| Index drifts from code until re-embedded | Snippets rebuilt live; never stale |
| Ships code/embeddings to a cloud | Local-only; nothing leaves the machine |
| Per-query LLM/embedding cost | Zero LLM calls, zero per-query cost |

### The five benefits, plain
1. **Determinism you can audit.** Run it twice, diff it. Reproducible context is
   debuggable context.
2. **Never stale.** Live reconstruction from the working tree — answers match
   reality by construction.
3. **Token-thrifty.** Exact ranges, not whole files; locate once, not every
   session.
4. **Private & free to run.** No cloud, no embeddings, no API bill, one binary.
5. **A memory that outlives the session.** The agent forgets; the logbook
   doesn't.

---

## 7. How it works (the 30-second mechanism)

```
your working tree ──(tree-sitter)──> index: metadata + git anchors  (the chart)
        │                                        │
        │  agent: columbus search "parse config" │
        ▼                                        ▼
  re-parse live ◄───────────────────────  rank deterministically
        │
        ▼
  ranked results + EXACT line ranges + "why relevant"  →  LLM-ready
```

The chart tells Columbus *where* things are. The working tree tells it *what
they currently say*. Because it never caches the "what," it can never lie about
it.

---

## 8. Messaging by length

### Taglines (pick per surface; first is the lead)
- **The map your coding agent has been missing.**
- Deterministic code context for AI agents. Local-only. Never stale.
- Stop letting your agent grep in the dark.
- Your agent explores. Columbus charts.
- The agent forgets. The logbook doesn't.

### Elevator pitch — 1 line
Columbus is a local, deterministic code-context server your AI agent calls as a
tool: ranked, always-current context plus a durable project memory — no
embeddings, no cloud, no guessing.

### Elevator pitch — 1 paragraph
Your coding agent is brilliant at reasoning and bad at locating and remembering.
So it greps in the dark, reads whole files to use ten lines, and leans on stale
`.md` context that quietly lies. Columbus is the fix: a local-only, deterministic
context server it calls as a tool. Ask it where something is and get ranked,
LLM-ready results with exact line ranges — reconstructed live from your working
tree, so they're never stale. It also owns the project's durable memory —
decisions, epics, and tasks — with drift checks, so knowledge survives across
sessions instead of being re-discovered every time. No LLM calls, no cloud, no
API bill. One Go binary; `git` is the only hard dependency.

### Feature → benefit table (for landing page sections)
| Feature | So what (benefit) |
|---|---|
| Metadata + git anchors only; live re-parse | Context always matches current code — can't go stale |
| Deterministic heuristic ranking | Reproducible, auditable, debuggable context |
| `--json` versioned contract / `--llm` markdown | Drop-in for any agent; output modes can't diverge |
| Durable memory + epics & tasks | Knowledge persists across sessions; no context-file sprawl |
| `validate` drift checks | Memory warns you when it's drifting instead of lying |
| Local-only, no LLM calls | Private, free to run, fast |
| Single binary, `git`-only dep | Trivial to install and trust |

---

## 9. Proof — "better with than without"

We never ship invented numbers. These are the **architectural claims (provable
now)** and a **measurement plan (run it, then quote real results)**.

### Provable today (no benchmark needed — show, don't tell)
- **Determinism:** `columbus search X --json | sha256sum` twice → identical.
  Compare to two agent grep-explorations of the same task → different paths.
- **Never stale:** edit a file, *don't* re-index, `columbus show symbol Foo` →
  it reflects the edit (live reconstruction). A cached/RAG index would not.
- **Local-only:** run it with the network off. It works.
- **Zero LLM cost:** there is no API key to configure. Nothing to bill.

### Measurement plan (the with/without study)
Pick ~5 representative tasks in a real repo (e.g. "add a flag to command X",
"find where config is parsed and change the default"). For each, run the same
agent twice: once with naive exploration, once with Columbus as a tool. Capture:

| Metric | How to measure | Why it matters |
|---|---|---|
| **Tokens to first correct location** | Agent token log up to the edit | Quantifies "fumbling in the dark" |
| **Total session tokens** | Agent token log | The dollar number |
| **Tool calls to locate** | Count grep/read vs. one `search` | Concreteness of the win |
| **Wall-clock to first edit** | Stopwatch | Felt speed |
| **Correct-on-first-try rate** | Did it edit the right place? | Determinism → fewer wrong turns |
| **Run-to-run variance** | Repeat each task 3× | Shows non-determinism cost without it |

Output: a small table + one before/after transcript GIF. The transcript is more
persuasive than the table — show the agent grepping five times vs. one
`columbus search`.

### The hero demo (60 seconds, asciinema)
```sh
columbus init
columbus index
columbus search "where do we parse config" --llm     # ranked, exact ranges, why-relevant
# edit a file...
columbus show symbol Engine --in internal/search      # live — reflects the edit
columbus memory add --kind decision --title "Use WAL" --body "readers never block writers"
columbus ui                                            # the dashboard money shot
```

---

## 10. How to use (the on-ramp story)

Three commands to value:
```sh
columbus init      # mint project id, write git-excluded .columbus.json
columbus index     # chart the codebase
columbus search "parse config"   # ranked, LLM-ready context
```
Then point your agent at it as a tool. The agent stops grepping and starts
asking. `columbus ui` gives *you* the dashboard — index freshness, memory,
epics, tasks, graph hubs — auto-refreshing as the agent works.

---

## 11. Launch checklist & channels

**Assets to build (in priority order):**
1. README hero rewrite — lead with the three enemies + the live-reconstruction
   detail. (Current README is excellent reference material but feature-led, not
   pain-led.)
2. The 60-second asciinema hero demo.
3. The with/without transcript GIF (one task, side by side).
4. Landing page using §8 feature→benefit table.
5. The measurement table (§9) once real numbers exist.

**Channels (audience = terminal-dwelling AI-agent users):**
- Show HN: "Columbus — deterministic, always-current code context for AI agents
  (no embeddings)". Lead with the mechanism; HN rewards anti-hype.
- r/LocalLLaMA, r/ClaudeAI, r/commandline.
- X/Bluesky dev-tool circles — one-idea-per-post, dry tone.
- A "how I stopped my agent grepping in the dark" blog post / dev.to.

**The single most important message to land everywhere:**
*It cannot go stale, because it never stores your code.* That one line carries
the determinism, the honesty, and the differentiation against RAG all at once.

---

## Appendix — claims we can and cannot make

**Can (architectural fact):** deterministic · local-only · no LLM calls · never
stores code · always reflects current working tree · single binary · git-only
hard dep · versioned JSON contract.

**Cannot yet (needs measurement — don't publish until real):** specific token
savings %, specific speedups, "Nx fewer tool calls." Run §9 first, then quote
the real figure.
