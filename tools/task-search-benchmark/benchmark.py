#!/usr/bin/env python3
"""Run Tasc search comparisons against a frozen, read-only snapshot."""
import argparse
from array import array
from contextlib import closing
import hashlib
import json
import math
from pathlib import Path
import re
import resource
import sqlite3
import statistics
import time
import urllib.request

MODELS = {"nomic-embed-text:latest": 768, "qwen3-embedding:0.6b": 1024}
INSTRUCTION = "Given a web search query, retrieve relevant passages that answer the query"
PROFILES = ("handoff", "evidence")


def readonly(path, immutable=False):
    options = "?mode=ro&immutable=1" if immutable else "?mode=ro"
    db = sqlite3.connect(path.resolve().as_uri() + options, uri=True)
    db.execute("PRAGMA query_only=ON")
    db.row_factory = sqlite3.Row
    return db


def snapshot(source, destination):
    # SQLite's backup API includes committed WAL data and yields a consistent copy.
    fd = destination.open("xb")
    fd.close()
    with closing(readonly(source)) as db, closing(sqlite3.connect(destination)) as copy:
        db.backup(copy)
        copy.execute("PRAGMA journal_mode=DELETE")


def tasks(db):
    # Match SQLiteStore.ListTasks for derived blocked status and archived projects.
    return {row["id"]: row for row in db.execute("""
        SELECT t.id, t.title, COALESCE(t.project, '') AS project,
               COALESCE(t.description, '') AS description,
               CASE WHEN t.status NOT IN ('done', 'deleted') AND EXISTS (
                   SELECT 1 FROM task_dependencies d JOIN tasks b ON b.id = d.blocker_id
                   WHERE d.blocked_id = t.id AND b.status NOT IN ('done', 'deleted')
               ) THEN 'blocked' ELSE t.status END AS status,
               COALESCE(p.status = 'archived', 0) AS archived
        FROM tasks t LEFT JOIN projects p ON p.name = NULLIF(t.project, '')
        WHERE t.status <> 'deleted'
        ORDER BY t.id
    """)}


def candidates(rows, case):
    statuses = case.get("statuses")
    projects = case.get("projects")
    return {i: row for i, row in rows.items()
            if (statuses is None or row["status"] in statuses)
            and (projects is None or row["project"] in projects)
            and (not case.get("exclude_archived") or not row["archived"])}


def bounded(text, byte_limit):
    return text.encode()[:byte_limit].decode(errors="ignore")


def passages(row):
    # Tasc handoffs use the opening nonblank paragraph, capped at 512 UTF-8 bytes.
    description = row["description"].strip()
    parts = re.split(r"\n[^\S\n]*\n", description, maxsplit=1)
    handoff = bounded(parts[0], 512)
    base = f'{bounded(row["title"], 512)}\nProject: {bounded(row["project"], 128)}'
    documents = [(f"{base}\n{handoff}", handoff or bounded(row["title"], 512))]
    remainder = "\n\n".join([parts[0][len(handoff):], *parts[1:]]).strip()
    if len(description.encode()) > 512 and remainder:
        # Limit extra passages to the first 768 bytes after the handoff.
        first = bounded(remainder, 384)
        second = bounded(remainder[len(first):].strip(), 384)
        documents.extend((f"{base}\n{part}", part) for part in (first, second) if part)
    return documents


class Ollama:
    def __init__(self, model, offline=False):
        self.model = model
        self.offline = offline
        self.url = "http://127.0.0.1:1" if offline else "http://127.0.0.1:11434"
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def api(self, path, body=None):
        request = urllib.request.Request(
            self.url + path, data=json.dumps(body).encode() if body is not None else None,
            headers={"Content-Type": "application/json"},
        )
        with self.http.open(request, timeout=120) as response:
            return json.load(response)

    def digest(self):
        for item in self.api("/api/tags")["models"]:
            if item["name"] == self.model:
                return item["digest"]
        raise ValueError("requested embedding model is absent")

    def embed(self, texts, query=False):
        if self.model.startswith("nomic-embed-text"):
            role = "search_query" if query else "search_document"
            texts = [f"{role}: {text}" for text in texts]
        elif query:
            texts = [f"Instruct: {INSTRUCTION}\nQuery:{text}" for text in texts]
        result = self.api("/api/embed", {
            "model": self.model, "input": texts, "truncate": False,
            "options": {"num_thread": 2}, "keep_alive": "5m",
        })
        vectors = result["embeddings"]
        if len(vectors) != len(texts):
            raise ValueError("wrong number of embeddings")
        normalized = []
        for vector in vectors:
            norm = math.hypot(*vector)
            if len(vector) != MODELS[self.model] or not math.isfinite(norm) or norm == 0:
                raise ValueError("invalid embedding")
            normalized.append(array("f", (v / norm for v in vector)))
        return normalized

    def memory(self):
        return [{k: model[k] for k in ("name", "size", "size_vram")}
                for model in self.api("/api/ps")["models"] if model["name"] == self.model]


class VectorCache:
    def __init__(self, db, client):
        self.db = db
        self.client = client
        self.db.execute("""
            CREATE TABLE IF NOT EXISTS vectors (
                profile TEXT, hash TEXT, data BLOB NOT NULL, PRIMARY KEY(profile, hash)
            )
        """)

    def refresh(self, documents, progress=False):
        digest = self.client.digest()
        profile = f"v1/{self.client.model}/{digest}/{MODELS[self.client.model]}"
        required = {key: text for task_docs in documents.values() for key, text, _ in task_docs}
        present = dict(self.db.execute("SELECT hash, data FROM vectors WHERE profile = ?", (profile,)))
        pending = sorted(required.keys() - present.keys())
        for start in range(0, len(pending), 16):
            keys = pending[start:start + 16]
            vectors = self.client.embed([required[key] for key in keys])
            if self.client.digest() != digest:
                raise ValueError("embedding model changed during refresh")
            data = {key: vector.tobytes() for key, vector in zip(keys, vectors, strict=True)}
            with self.db:
                self.db.executemany("INSERT INTO vectors VALUES (?, ?, ?)",
                                    ((profile, key, blob) for key, blob in data.items()))
            present.update(data)
            if progress and (start % 128 == 0 or start + len(keys) == len(pending)):
                print(f"Embedded {start + len(keys)}/{len(pending)} new inputs", flush=True)
        stale = present.keys() - required.keys()
        if self.client.digest() != digest:
            raise ValueError("embedding model changed during refresh")
        with self.db:
            self.db.execute("DELETE FROM vectors WHERE profile <> ?", (profile,))
            self.db.executemany("DELETE FROM vectors WHERE profile = ? AND hash = ?",
                                ((profile, key) for key in stale))
        stats = dict(new=len(pending), reused=len(required) - len(pending), removed=len(stale),
                     model=self.client.model, digest=digest)
        return {key: array("f", present[key]) for key in required}, stats


def describe(rows):
    return {i: [(hashlib.sha256(text.encode()).hexdigest(), text, excerpt)
                for text, excerpt in passages(row)] for i, row in rows.items()}


def lexical(db, query, allowed):
    # Same FTS5 MATCH expression and rank as SQLiteStore.SearchTasks. The
    # snapshot contains the original index, including its document statistics.
    ids = [r[0] for r in db.execute(
        "SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH ? ORDER BY rank", (query,))]
    return [i for i in ids if i in allowed]


def semantic(vector, allowed, documents, vectors, profile):
    scores, excerpts = {}, {}
    for i in allowed:
        docs = documents[i][:1] if profile == "handoff" else documents[i]
        score, _, excerpt = max((math.sumprod(vector, vectors[key]), -index, excerpt)
                                for index, (key, _, excerpt) in enumerate(docs))
        scores[i], excerpts[i] = score, excerpt
    return sorted(scores, key=lambda i: (-scores[i], i)), excerpts


def exact(query, allowed):
    if re.fullmatch(r"#?-?[0-9]+", query):
        i = int(query.lstrip("#"))
        return ([i] if i in allowed else []), True
    title = query.strip().casefold()
    matches = [i for i, task in allowed.items() if task["title"].strip().casefold() == title]
    return sorted(matches, key=lambda i: (allowed[i]["status"] == "done", allowed[i]["archived"], i)), False


def combined(keyword, meaning, exact_ids):
    scores = {}
    for ranks in (keyword, meaning):
        for rank, i in enumerate(ranks[:50], 1):
            scores[i] = scores.get(i, 0) + 1 / (60 + rank)
    result = sorted(scores, key=lambda i: (-scores[i], i))
    return list(dict.fromkeys(exact_ids + result))


def fts_excerpt(db, expression, task_id):
    for column in (0, 1, 2):
        row = db.execute("""
            SELECT highlight(tasks_fts, ?, char(1), char(2)) FROM tasks_fts
            WHERE rowid = ? AND tasks_fts MATCH ?
        """, (column, task_id, expression)).fetchone()
        if row and row[0]:
            offset = row[0].find("\x01")
            if offset >= 0:
                return row[0][max(0, offset - 65):offset + 110].replace("\x01", "[").replace("\x02", "]")
    return ""


def result_rows(db, expression, ids, allowed, mode, keyword, excerpts, exact_ids):
    rows = []
    for i in ids[:5]:
        task = allowed[i]
        source = mode
        if mode != "fts5" and i in exact_ids and not mode.startswith("semantic"):
            source = "exact"
        excerpt = excerpts.get(i, "")
        if (source in ("fts5", "keyword + exact", "keyword fallback") or source.startswith("hybrid")) and i in keyword:
            excerpt = fts_excerpt(db, expression, i) or excerpt
        rows.append(dict(id=i, title=task["title"], status=task["status"], mode=source,
                         excerpt=excerpt or task["title"]))
    return rows


def nearest_rank(samples, percentile):
    return sorted(samples)[math.ceil(len(samples) * percentile) - 1]


def evaluate(db, rows, documents, vectors, client, cases, repeats):
    results = []
    for case in cases:
        allowed = candidates(rows, case)
        if case["target"] not in allowed:
            raise ValueError(f"target excluded for case {case['name']}")
        if set(case.get("excluded", ())) & allowed.keys():
            raise ValueError(f"excluded control survived for case {case['name']}")
        query = case["query"]
        expression = case.get("fts", query)
        ranks, excerpts, durations = {}, {}, {}
        for _ in range(repeats):
            start = time.perf_counter()
            kw = lexical(db, expression, allowed)
            ranks["fts5"] = kw
            lexical_seconds = time.perf_counter() - start
            durations.setdefault("fts5", []).append(lexical_seconds)
            start = time.perf_counter()
            exact_ids, id_only = exact(query, allowed)
            ranks["keyword + exact"] = exact_ids if id_only else list(dict.fromkeys(exact_ids + kw))
            exact_seconds = time.perf_counter() - start
            durations.setdefault("keyword + exact", []).append(lexical_seconds + exact_seconds)
            start = time.perf_counter()
            try:
                q, = client.embed([query], query=True)
            except OSError:
                if not client.offline:
                    raise
                q = None
            else:
                if client.offline:
                    raise ValueError("offline endpoint unexpectedly responded")
            embedding_seconds = time.perf_counter() - start
            for profile in PROFILES:
                start = time.perf_counter()
                label = f"semantic {profile}"
                if not client.offline:
                    ranks[label], excerpts[label] = semantic(q, allowed, documents, vectors, profile)
                    semantic_seconds = embedding_seconds + time.perf_counter() - start
                    durations.setdefault(label, []).append(semantic_seconds)
                else:
                    semantic_seconds = embedding_seconds
                start = time.perf_counter()
                hybrid = f"hybrid {profile}"
                if id_only or client.offline:
                    ranks[hybrid] = ranks["keyword + exact"]
                else:
                    ranks[hybrid] = combined(kw, ranks[label], exact_ids)
                durations.setdefault(hybrid, []).append(
                    lexical_seconds + exact_seconds + (0 if id_only else semantic_seconds)
                    + time.perf_counter() - start)
        modes = {}
        for name, ids in ranks.items():
            label = "keyword fallback" if client.offline and name.startswith("hybrid") else name
            excerpts_for_mode = excerpts.get(name.replace("hybrid", "semantic"), {})
            modes[name] = dict(hit=case["target"] in ids[:5], first=bool(ids and ids[0] == case["target"]),
                               seconds=durations[name], results=result_rows(
                                   db, expression, ids, allowed, label, kw, excerpts_for_mode, exact_ids))
        results.append(dict(name=case["name"], group=case["group"],
                            candidate_count=len(allowed), modes=modes))
    return results


def summarize(results):
    summary = {}
    for name in results[0]["modes"]:
        timings = [s * 1000 for case in results for s in case["modes"][name]["seconds"]]
        groups = {}
        for group in sorted({case["group"] for case in results}):
            cases = [case["modes"][name] for case in results if case["group"] == group]
            groups[group] = dict(hits=sum(c["hit"] for c in cases),
                                 first=sum(c["first"] for c in cases), total=len(cases))
        summary[name] = dict(groups=groups, p50_ms=round(statistics.median(timings), 3),
                             p95_ms=round(nearest_rank(timings, 0.95), 3))
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    freeze = sub.add_parser("snapshot")
    freeze.add_argument("source", type=Path)
    freeze.add_argument("destination", type=Path)
    run = sub.add_parser("evaluate")
    run.add_argument("snapshot", type=Path)
    run.add_argument("cases", type=Path)
    run.add_argument("cache", type=Path)
    run.add_argument("results", type=Path)
    run.add_argument("--model", choices=MODELS, default="nomic-embed-text:latest")
    run.add_argument("--repeats", type=int, default=3)
    run.add_argument("--offline", action="store_true")
    args = parser.parse_args()
    if args.command == "snapshot":
        snapshot(args.source, args.destination)
        return
    if args.repeats < 1 or args.results.exists():
        parser.error("choose a positive repeat count and a new results file")
    if len({p.resolve() for p in (args.snapshot, args.cases, args.cache, args.results)}) != 4:
        parser.error("use distinct snapshot, cases, cache and results files")
    cases = json.loads(args.cases.read_text())
    client = Ollama(args.model, args.offline)
    with closing(readonly(args.snapshot, immutable=True)) as db:
        rows = tasks(db)
        # Fail if the snapshot lacks the shipping FTS5 index.
        db.execute("SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH 'probe'").fetchall()
        documents = describe(rows)
        vectors, stats = {}, {}
        if not args.offline:
            start = time.perf_counter()
            with closing(sqlite3.connect(args.cache)) as cache:
                vectors, stats = VectorCache(cache, client).refresh(documents, progress=True)
            stats.update(seconds=round(time.perf_counter() - start, 3),
                         bytes=args.cache.stat().st_size, ollama=client.memory())
            # Warm the query runner before recording latency.
            client.embed(["warm query"], query=True)
        results = evaluate(db, rows, documents, vectors, client, cases, args.repeats)
        if not args.offline:
            if client.digest() != stats["digest"]:
                raise ValueError("embedding model changed during evaluation")
            stats["ollama"] = client.memory()
    summary = dict(tasks=len(rows), refresh=stats, peak_rss_kib=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                   **{"modes": summarize(results)})
    with args.results.open("x") as output:
        json.dump(dict(summary=summary, cases=results), output, ensure_ascii=False, indent=2)
    print(json.dumps(summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
