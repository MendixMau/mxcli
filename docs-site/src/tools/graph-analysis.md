# Graph Analysis

mxcli models a Mendix project as a **dependency graph** — documents (entities,
microflows, pages, …) are nodes and their references (`CATALOG.REFS`) are edges —
and runs topological analyses on top of it: god nodes, module coupling, community
detection, dependency cycles, layering, and centrality.

Use it to **understand an unfamiliar app and decide where to intervene**: what's
central and risky to change, which modules are entangled, what naturally belongs
together, and what it would take to split the app.

All of this is pure-Go (no external dependencies) and reads the catalog, so it
needs no Studio Pro and no cloud connectivity. It requires a **full** catalog
(the graph lives in the `refs` table, built by `refresh catalog full`).

## `mxcli graph-report` — the architecture map

A one-shot Markdown/JSON report rendered from the `CATALOG.graph_*` views:

```bash
mxcli graph-report -p app.mpr                 # markdown to stdout
mxcli graph-report -p app.mpr --top 25        # more rows per section
mxcli graph-report -p app.mpr --format json -o graph.json
mxcli graph-report -p app.mpr --include-framework   # keep System/Atlas/connectors
```

Sections: **god nodes** (degree centrality), **module coupling** (cross-module
"surprise edges"), **module cohesion** (intra/inter ratio), **dead documents**
(no inbound reference), **reference kinds** (the edge vocabulary), and **entity
hotspots** (entities used by the most flows). Framework/marketplace modules are
excluded by default — they dominate the raw top-N but aren't actionable.

Every section is a thin `SELECT` over a view, so it is reproducible directly:

```sql
select * from CATALOG.graph_god_nodes order by Degree desc limit 20
```

## Community detection, cycles, layers, centrality

These need a graph algorithm, so they are computed on demand and stored:

```bash
mxcli -p app.mpr -c "refresh catalog communities"               # default resolution
mxcli -p app.mpr -c "refresh catalog communities resolution 0.6"  # coarser (fewer, larger)
mxcli -p app.mpr -c "refresh catalog communities resolution 2.0"  # finer (more, smaller)
```

This runs **Leiden** community detection, **Tarjan** strongly-connected components
(cycles), topological **layering**, **PageRank**, and **betweenness**, then
populates these catalog objects (and fills the `PageRank`/`Betweenness` columns of
`graph_god_nodes`):

| Object | Contents |
|--------|----------|
| `CATALOG.communities` | community id per asset |
| `CATALOG.community_summary` | per-community size, dominant-module label, members |
| `CATALOG.graph_cycles` | assets in a dependency cycle |
| `CATALOG.graph_layers` | topological layer sequence number per asset |
| `CATALOG.graph_centrality` | PageRank / betweenness per asset |
| `CATALOG.graph_module_dependencies` | directed module→module edges (kind + count) |
| `CATALOG.graph_integration_surface` | cross-community edges → integration mechanism |

The `resolution` knob selects granularity: high γ → fine **candidate modules**;
low γ → coarse **candidate apps**.

### If one of these is empty

Empty means one of two very different things, and they used to look identical.
The tables above are filled by the pass, not by a build mode — so a catalog built
with `refresh catalog full` has all of them empty while `graph_module_coupling`,
a plain view over `refs`, answers normally. That asymmetry was
[mendixlabs/mxcli#1060](https://github.com/mendixlabs/mxcli/issues/1060).

A query now says which case you are in:

```
Warning: CATALOG.GRAPH_CYCLES requires refresh catalog communities (not run for this catalog)
```

No warning and no rows means the pass ran and genuinely found nothing.
`show catalog status` reports the same thing up front:

```
Graph analysis: ✓ Available (resolution 1)
Graph analysis: ✗ Not run (use refresh catalog communities)
```

The pass is **not** a build mode — it augments whatever mode is cached, so
`Build mode: full` says nothing about it. A later `refresh catalog full` used to
drop these tables silently; it now re-runs the pass at the same resolution, so the
graph survives a rebuild. To drop back, delete `.mxcli/catalog.db` and refresh.

### SHOW commands

```sql
show communities                                   -- the community_summary listing
show community of Sales.Order                      -- which community an asset is in
show community members of Sales.Order              -- its co-clustered assets
```

## Two refactoring journeys

**Spaghetti → layered / modular app.**

```sql
select * from CATALOG.graph_cycles;                       -- the tangles to break
select Layer, AssetName from CATALOG.graph_layers
  order by Layer;                                          -- dependency depth
show communities;                                          -- cleaner module groupings
```

mxcli reports the *facts* (layer numbers, directed `graph_module_dependencies`);
your team decides what ordering is "correct" and enforces it with a Starlark rule.

**Monolith → multi-app (REST / OData / events).**

```bash
mxcli -p app.mpr -c "refresh catalog communities resolution 0.6"   # candidate apps
```

```sql
-- the contract list a split would require, classified by mechanism
select * from CATALOG.graph_integration_surface order by Edges desc;
```

Each crossing edge maps to its integration mechanism — `associate`→OData/shared
entity, `retrieve`→OData read, `call`→REST, `create/change`→event/REST write —
and `generalize` crossings are flagged as **blockers** (inheritance can't cross an
app boundary).

## Enforce your own architecture (Starlark)

mxcli ships the *facts*, not an opinion. Teams enforce their own layering /
allowed-dependency / no-cycle / coupling-budget policies via Starlark lint rules,
using these builtins (which read the graph tables):

`community_of`, `layer_of`, `cycles`, `module_dependencies`, `centrality`,
`god_nodes`, `integration_surface`, `refs_from`.

```python
RULE_ID = "ARCH900"
RULE_NAME = "No Payments→Reporting dependency"
DESCRIPTION = "Payments must not depend on Reporting"
CATEGORY = "architecture"
SEVERITY = "error"

def check():
    return [violation(message = "Payments must not depend on Reporting")
            for d in module_dependencies()
            if d.source_module == "Payments" and d.target_module == "Reporting"]
```

The graph builtins return empty when the community tables aren't built, so run
`refresh catalog communities` before `lint` in the same session. See
[Writing Custom Rules](custom-rules.md).

## Notes

- Quality depends on edge completeness. The `refs` graph captures control flow,
  CRUD, associations, generalization, widget datasources/actions, layout, and
  flow parameter/return types. References buried inside **expressions / XPath
  constraints** (and enum/constant usage) are not yet edges.
- Betweenness is O(V·E); it is skipped above ~6,000 nodes to keep the refresh
  fast (PageRank and communities still run).
- The Leiden implementation is deterministic and matches the reference
  `leidenalg` results.
