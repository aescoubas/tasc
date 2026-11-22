# Hybrid task search evaluation

This harness reads a consistent SQLite snapshot from Tasc and sends bounded text
to local Ollama. It uses a disposable vector cache alongside the snapshot. Tasc
supplies IDs, titles, statuses, project memberships, dependency-derived blocked
status and project archive state. The harness never opens the Zettelkasten
index or writes to Tasc's source database.

## Run locally

Use Python 3.12 or later with SQLite FTS5. Ollama must have
`nomic-embed-text:latest` installed. The lifecycle check also uses
`qwen3-embedding:0.6b`. It creates and edits only its disposable Tasc fixture.

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/task-search-benchmark/verify.py
```

To benchmark private tasks, use a private scratch directory. Save the cases
there before running retrieval. Each case has a query, a known target task ID
and a group for the report. Optional filters take effect for every mode.
`excluded` lists control IDs that must be absent from the candidate set. The
optional `fts` value supplies FTS5 syntax when the text itself contains FTS
operators or punctuation.

```json
[
  {
    "name": "active calibration",
    "group": "filtered paraphrase",
    "query": "Which instrument needs another reference measurement",
    "target": 42,
    "statuses": ["backlog", "ongoing", "blocked"],
    "projects": ["Lab"],
    "exclude_archived": true,
    "excluded": [39]
  }
]
```

```sh
umask 077
scratch=$(mktemp -d)
python3 tools/task-search-benchmark/benchmark.py snapshot ~/.tasc.db "$scratch/tasc.sqlite"
# Save cases.json in "$scratch" before evaluating.
python3 tools/task-search-benchmark/benchmark.py evaluate \
  "$scratch/tasc.sqlite" "$scratch/cases.json" "$scratch/vectors.sqlite" "$scratch/results.json"
python3 tools/task-search-benchmark/benchmark.py evaluate \
  "$scratch/tasc.sqlite" "$scratch/cases.json" "$scratch/unused.sqlite" "$scratch/offline.json" --offline
```

The stdout summary contains aggregate metrics. Results files contain task IDs,
titles, effective statuses, retrieval modes and excerpts. Keep them private
along with the cases, database and vectors.

## Comparison

FTS5 uses the shipping `tasks_fts MATCH ? ORDER BY rank` query and original
index statistics. Every mode admits the same filtered set of Tasc rows.
The keyword control adds exact ID routing and promotes full-title matches.
Exact-title ties prefer open tasks over done tasks, then nonarchived projects
and ascending IDs. Semantic retrieval uses normalized cosine similarity.
Hybrid combines each ranker's first 50 results with reciprocal rank fusion
using a constant of 60, and applies the same exact routing as the keyword
control.

The handoff profile embeds title, project and the opening paragraph, capped
at 512 UTF-8 bytes for the handoff. The evidence profile adds up to two
384-byte passages immediately after the handoff for long descriptions. Each
task's best passage contributes its score. This bounds the text and creates
a chance to find evidence past the opening paragraph. It limits coverage of
later passages in very long descriptions.

Nomic uses the Zettelkasten `search_document:` and `search_query:` encodings.
Qwen uses plain documents and the Zettelkasten benchmark's retrieval
instruction on queries. The cache keys input bytes by SHA-256 plus the input
encoding profile, Ollama model digest and vector dimension. Refreshes embed
missing inputs, reuse matching vectors and remove obsolete inputs after
success. A change of model rebuilds that namespace. A metadata-only change
takes effect from the new snapshot without re-embedding unchanged text. The
offline run forces a loopback connection failure and exercises exact plus
FTS5 fallback with an empty vector cache.

## ThinkPad results, 2026-11-22

The snapshot held 1,833 nondeleted tasks. The 25 known-target cases reused
some targets across categories and repeated a duplicate query with and without
filters. They were
written before ranking and exercised exact names and IDs, free-form English
and French, evidence beyond the handoff, completed duplicates and filters.
Three warm runs per query timed retrieval before excerpt formatting. The
benchmark used installed Tasc 0.6.1's FTS5 index, Python 3.13.17, SQLite 3.53.3
and Ollama 0.40.3 on the ThinkPad's Ryzen 5 PRO 7540U. Ranking used Nomic,
digest `0a109f422b47`, with two CPU threads.

| Retrieval | Exact title | ID | English paraphrase | French | Legacy evidence | Duplicates & filters | Total hits@5 | p50 / p95 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Existing FTS5 | 3/3 | 0/3 | 0/8 | 0/4 | 3/3 | 2/4 | 8/25 | 0.6 / 1.2 ms |
| FTS5 + exact routing | 3/3 | 3/3 | 0/8 | 0/4 | 3/3 | 2/4 | 11/25 | 1.2 / 2.8 ms |
| Semantic, handoff | 3/3 | 0/3 | 5/8 | 1/4 | 2/3 | 4/4 | 15/25 | 89 / 106 ms |
| Hybrid, handoff | 3/3 | 3/3 | 5/8 | 1/4 | 3/3 | 4/4 | 19/25 | 91 / 108 ms |
| Semantic, with evidence | 3/3 | 0/3 | 5/8 | 2/4 | 3/3 | 4/4 | 17/25 | 107 / 129 ms |
| Hybrid, with evidence | 3/3 | 3/3 | 5/8 | 2/4 | 3/3 | 4/4 | 20/25 | 108 / 132 ms |

Both hybrid profiles and the keyword control put all six ID and exact-title
targets first. The control also put the active copy of the exact duplicate
first. Bounded passages restored one legacy-evidence hit in semantic search,
added one French hit to hybrid, and moved one English paraphrase from first
place to lower in the top five. Every legacy-evidence query already reached the
top five in FTS5 using its supplied keyword expression. The raw free-form
English and French FTS5 queries had zero hits. This comparison measures the
shipping parser and the specified expressions. A tuned lexical query-rewrite
baseline and a larger blinded query set are needed to estimate the gain beyond
this sample. Qwen was exercised for cache replacement and query encoding only.

The initial refresh embedded 2,761 distinct inputs in 439 seconds. A second
run reused all 2,761 in 60 ms. The disposable SQLite cache occupied 11.3 MiB.
Peak harness RSS was approximately 60 MiB. Ollama reported 376 MB allocated
for the loaded Nomic runner on CPU and zero VRAM. The offline run matched
the keyword control's result IDs, titles, statuses and excerpts in all 25
cases for both profiles. Its p95 was 1.9 ms.

The lifecycle fixture used the installed Tasc CLI. It verified archive,
status and project filters, completed duplicates, dependency and FTS5
freshness after writes, one-vector handoff replacement, three-vector title
replacement, deletion and Nomic → Qwen → Nomic replacement. The private
snapshot and 25-case manifest were frozen separately from this checkout.
Their SHA-256 digests are
`6694053b05b69feb5dc7074e4410bd6e0f5e639ec28cbcff9d6232eb7f455f51`
and `8e3ed8cb41ee486f93ddfab00bd10203011786dd858a15064be22776cda2d6e5`.

**Recommendation.** Keep keyword and exact lookup as the primary path. The
hybrid result merits further opt-in evaluation for free-form queries. Defer
production integration until a blinded set with a stronger lexical baseline
confirms enough additional value for the latency, index refresh cost and
limited French recall observed here.
