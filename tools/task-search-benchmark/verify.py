#!/usr/bin/env python3
"""Exercise the benchmark lifecycle with a disposable real Tasc database."""
import argparse
from contextlib import closing
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import tempfile

from benchmark import Ollama, VectorCache, candidates, describe, evaluate, lexical, readonly, snapshot, tasks


def verify(binary, workspace):
    source = workspace / "fixture.sqlite"
    env = dict(os.environ, HOME=str(workspace), TASC_DB_PATH=str(source), TASC_REMOTE="")

    def cli(*arguments):
        result = subprocess.run([binary, *arguments], env=env, capture_output=True, text=True)
        if result.returncode:
            raise RuntimeError(f"Tasc {arguments} failed: {result.stdout}{result.stderr}")
        return result.stdout

    def add(title, *arguments):
        return int(re.search(r"Created task (\d+)", cli("add", title, *arguments)).group(1))

    cli("project", "create", "Archive")
    archived = add("Archive control", "--project", "Archive")
    cli("project", "archive", "Archive")
    done = add("Review calibration")
    cli("done", str(done))
    blocker = add("Order reference materials")
    active = add("Review calibration", "--project", "Lab", "--depends-on", str(blocker),
                 "--description", "Compare standards.\n\n" + "readings " * 100 + "outside")
    case = dict(name="filtered target", group="fixture", target=active, query="Review calibration",
                statuses=["blocked"], projects=["Lab"], exclude_archived=True, excluded=[archived, done])
    cache = workspace / "vectors.sqlite"
    nomic = Ollama("nomic-embed-text:latest")
    qwen = Ollama("qwen3-embedding:0.6b")

    def capture(label):
        frozen = workspace / (label + ".sqlite")
        snapshot(source, frozen)
        return readonly(frozen, immutable=True)

    with closing(sqlite3.connect(cache)) as cache_db:
        index = VectorCache(cache_db, nomic)
        with closing(capture("base")) as db:
            rows = tasks(db)
            assert rows[active]["status"] == "blocked" and rows[archived]["archived"]
            assert set(candidates(rows, dict(exclude_archived=True))) == {active, blocker, done}
            assert set(candidates(rows, case)) == {active}
            assert set(lexical(db, case["query"], rows)) == {done, active}
            docs = describe(rows)
            vectors, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"], stats["removed"]) == (6, 0, 0), stats
            assert len(vectors[docs[active][0][0]]) == 768
            _, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"], stats["removed"]) == (0, 6, 0), stats
            normal, = evaluate(db, rows, docs, vectors, nomic, [case], 1)
            fallback, = evaluate(db, rows, docs, {}, Ollama(nomic.model, offline=True), [case], 1)
            for profile in ("handoff", "evidence"):
                mode = "hybrid " + profile
                assert normal["modes"][mode]["first"]
                result, = fallback["modes"][mode]["results"]
                keyword, = fallback["modes"]["keyword + exact"]["results"]
                assert result == keyword and result["id"] == active

        # A changed handoff replaces only its vector. A changed task title
        # replaces every passage referencing the title. Status isn't vector text.
        cli("modify", str(active), "--yes", "--description", "Measure standards.\n\n" + "readings " * 100 + "outside")
        with closing(capture("edited")) as db:
            rows = tasks(db)
            docs = describe(rows)
            vectors, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"], stats["removed"]) == (1, 5, 1), stats
            assert lexical(db, "Measure", rows) == [active]
            assert lexical(db, "Compare", rows) == []
        cli("modify", str(active), "--yes", "Compare new calibration")
        with closing(capture("renamed")) as db:
            docs = describe(tasks(db))
            _, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"], stats["removed"]) == (3, 3, 3), stats
        cli("done", str(blocker))
        with closing(capture("unblocked")) as db:
            rows = tasks(db)
            assert rows[active]["status"] == "backlog" and not candidates(rows, case)
            _, stats = index.refresh(describe(rows))
            assert (stats["new"], stats["reused"], stats["removed"]) == (0, 6, 0), stats
        cli("delete", str(active), "--yes")
        with closing(capture("deleted")) as db:
            rows = tasks(db)
            assert active not in rows and not lexical(db, "Measure", rows)
            docs = describe(rows)
            _, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"], stats["removed"]) == (0, 3, 3), stats
            qwen_vectors, stats = VectorCache(cache_db, qwen).refresh(docs)
            assert (stats["new"], stats["reused"]) == (3, 0), stats
            assert len(next(iter(qwen_vectors.values()))) == 1024
            assert len(qwen.embed(["Which calibration was completed?"], query=True)[0]) == 1024
            _, stats = index.refresh(docs)
            assert (stats["new"], stats["reused"]) == (3, 0), stats
    print("PASS: filters, fallback, handoff and title edits, dependency freshness, deletion, Nomic → Qwen → Nomic")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tasc", default="tasc", help="installed or locally built FTS5 binary")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="tasc-search-verify-") as directory:
        verify(args.tasc, Path(directory))
