// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/catalog/graph"
)

// graphRefKinds are the structural reference kinds used to build the analysis
// graph. Navigational kinds (layout/parameter-of-page/show_page) are excluded —
// they add UI coupling that blurs module/app clustering.
var graphRefKinds = []string{
	"call", "retrieve", "create", "change", "delete", "associate", "generalize",
	"parameter", "return",
	// A scheduled event is an entry point: the microflow it runs is reachable
	// even though nothing calls it. Without this kind, GRAPH_DEAD_ASSETS reports
	// every scheduled microflow as dead.
	"schedule",
}

// betweennessNodeCap bounds the O(V*E) betweenness computation. Above it,
// betweenness is skipped (PageRank/communities still run) to keep the pass fast.
const betweennessNodeCap = 6000

// effectiveResolution normalises the Leiden resolution. One function so the
// value recorded in the cache is the value the pass actually used — recording
// the caller's 0 would make a later re-run at "the same resolution" a different
// run.
func effectiveResolution(r float64) float64 {
	if r <= 0 {
		return 1.0
	}
	return r
}

// AddGraphAnalysis runs the graph-analysis pass (communities/cycles/layers/
// centrality) on an already-built catalog WITHOUT re-parsing. The catalog must
// already contain the refs table (built in full or source mode). This lets
// `refresh catalog communities` *augment* the existing catalog instead of
// rebuilding it — a rebuild downgrades a source-mode catalog to full and drops
// the source FTS data. The graph rows reuse the catalog's existing SnapshotId so
// the snapshot-framed views resolve.
func (c *Catalog) AddGraphAnalysis(resolution float64) error {
	resolution = effectiveResolution(resolution)
	var snapID string
	if err := c.db.QueryRow("SELECT SnapshotId FROM snapshots ORDER BY rowid DESC LIMIT 1").Scan(&snapID); err != nil {
		return fmt.Errorf("catalog has no snapshot (build full first): %w", err)
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	b := &Builder{
		catalog:         c,
		tx:              tx,
		snapshot:        &Snapshot{ID: snapID},
		communitiesMode: true,
		resolution:      resolution,
	}
	if err := b.buildGraphAnalysis(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Record that the pass ran. An empty graph_cycles_data means "no cycles"
	// only once this is set; before it, it means "never computed" — and the two
	// were indistinguishable (mendixlabs/mxcli#1060). Written after the commit
	// so the flag can never claim a pass that was rolled back.
	if err := c.SetMeta(MetaGraphAnalysis, time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	return c.SetMeta(MetaGraphResolution, strconv.FormatFloat(resolution, 'g', -1, 64))
}

// buildGraphAnalysis runs the pure-Go graph algorithms over the refs graph and
// writes communities/cycles/layers/centrality. Only runs in communities mode; it
// reads the refs table built earlier in the same transaction, so no re-parse.
func (b *Builder) buildGraphAnalysis() error {
	if !b.communitiesMode {
		return nil
	}
	edges, err := b.loadGraphEdges()
	if err != nil {
		return err
	}
	for _, tbl := range []string{"communities_data", "graph_cycles_data", "graph_layers_data", "graph_centrality_data"} {
		if _, err := b.tx.Exec("DELETE FROM " + tbl); err != nil {
			return err
		}
	}
	if len(edges) == 0 {
		return nil
	}

	g := graph.New(edges)
	projectID, snapshotID := b.snapshotMeta()
	resolution := effectiveResolution(b.resolution)
	// Communities.
	comm := g.Communities(resolution)
	commStmt, err := b.tx.Prepare(
		`INSERT INTO communities_data (AssetName, ModuleName, CommunityId, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	commSet := map[int]struct{}{}
	for id := 0; id < g.N(); id++ {
		name := g.Name(id)
		if _, err := commStmt.Exec(name, moduleOf(name), comm[id], projectID, snapshotID); err != nil {
			commStmt.Close()
			return err
		}
		commSet[comm[id]] = struct{}{}
	}
	commStmt.Close()

	// Cycles (SCCs of size > 1, or self-loops).
	cycStmt, err := b.tx.Prepare(
		`INSERT INTO graph_cycles_data (AssetName, ModuleName, CycleId, CycleSize, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for ci, comp := range g.Cycles() {
		for _, v := range comp {
			name := g.Name(v)
			if _, err := cycStmt.Exec(name, moduleOf(name), ci, len(comp), projectID, snapshotID); err != nil {
				cycStmt.Close()
				return err
			}
		}
	}
	cycStmt.Close()

	// Layers (topological sequence number).
	layers := g.Layers()
	layStmt, err := b.tx.Prepare(
		`INSERT INTO graph_layers_data (AssetName, ModuleName, Layer, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for id := 0; id < g.N(); id++ {
		name := g.Name(id)
		if _, err := layStmt.Exec(name, moduleOf(name), layers[id], projectID, snapshotID); err != nil {
			layStmt.Close()
			return err
		}
	}
	layStmt.Close()

	// Centrality: PageRank always; betweenness when the graph is small enough.
	pr := g.PageRank(0.85, 100)
	var bt []float64
	if g.N() <= betweennessNodeCap {
		bt = g.Betweenness()
	}
	cenStmt, err := b.tx.Prepare(
		`INSERT INTO graph_centrality_data (AssetName, PageRank, Betweenness, ProjectId, SnapshotId) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for id := 0; id < g.N(); id++ {
		betw := 0.0
		if bt != nil {
			betw = bt[id]
		}
		if _, err := cenStmt.Exec(g.Name(id), pr[id], betw, projectID, snapshotID); err != nil {
			cenStmt.Close()
			return err
		}
	}
	cenStmt.Close()

	b.report("Graph communities", len(commSet))
	return nil
}

// loadGraphEdges reads the structural edge set from the refs table.
func (b *Builder) loadGraphEdges() ([]graph.Edge, error) {
	quoted := make([]string, len(graphRefKinds))
	for i, k := range graphRefKinds {
		quoted[i] = "'" + k + "'"
	}
	stmt, err := b.tx.Prepare(
		`SELECT SourceName, TargetName FROM refs
		 WHERE SourceName != '' AND TargetName != '' AND RefKind IN (` + strings.Join(quoted, ", ") + `)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edges []graph.Edge
	for rows.Next() {
		var s, t string
		if err := rows.Scan(&s, &t); err != nil {
			return nil, err
		}
		edges = append(edges, graph.Edge{Source: s, Target: t})
	}
	return edges, rows.Err()
}
