// SPDX-License-Identifier: Apache-2.0

package executor

// Labelling merges for DESCRIBE, so a rejoin can be written down.
//
// A Mendix ExclusiveMerge has no name. MDL's `merge <label>` / `join <label>`
// invents one for the length of a description, which is what lets DESCRIBE emit
// a graph whose paths do not nest.
//
// The case this exists for is an ERROR path that rejoins the normal one. Before
// it, `collectErrorHandlerStatements` stopped dead at the merge and emitted an
// empty `on error … { }` block — MDL that re-executes to a DIFFERENT graph, with
// no warning. Measured: repointing SUB_Feedback_SendToServer's error edge from
// the tail merge to an upstream one produced byte-identical MDL, and executing
// it reproduced the tail-merge graph.
//
// Scope is deliberately narrow. Only merges an error handler rejoins are
// labelled — not every irreducible split, which still gets the #923 warning.
// A label emitted where the nested description already reproduces the graph
// would be noise, and worse, would churn the describe output of every project.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mergeLabels maps an ExclusiveMerge to the label DESCRIBE will give it.
// Nil is the ordinary case and every lookup on it is a miss, so callers need no
// nil check.
type mergeLabels map[model.ID]string

func (m mergeLabels) of(id model.ID) (string, bool) {
	if m == nil {
		return "", false
	}
	l, ok := m[id]
	return l, ok
}

// labelRejoinMerges finds the merges that an error handler reaches and that the
// normal path also reaches — the ones a nested description cannot spell — and
// gives each a stable label.
//
// "The normal path also reaches it" is the discriminating half. An error handler
// whose own path happens to end at a merge of its own is ordinary and already
// describes correctly; it is the SHARED merge that makes the graph irreducible,
// because the handler has to say "carry on where the main path is".
func labelRejoinMerges(col *microflows.MicroflowObjectCollection) mergeLabels {
	if col == nil {
		return nil
	}

	objects := map[model.ID]microflows.MicroflowObject{}
	var startID model.ID
	for _, o := range col.Objects {
		if o == nil {
			continue
		}
		objects[o.GetID()] = o
		if _, ok := o.(*microflows.StartEvent); ok {
			startID = o.GetID()
		}
	}

	normalSucc := map[model.ID][]model.ID{}
	var errorFlows []*microflows.SequenceFlow
	for _, f := range col.Flows {
		if f == nil {
			continue
		}
		if f.IsErrorHandler {
			errorFlows = append(errorFlows, f)
			continue
		}
		normalSucc[f.OriginID] = append(normalSucc[f.OriginID], f.DestinationID)
	}
	if len(errorFlows) == 0 {
		return nil
	}

	reachable := map[model.ID]bool{}
	var walk func(model.ID)
	walk = func(id model.ID) {
		if id == "" || reachable[id] {
			return
		}
		reachable[id] = true
		for _, s := range normalSucc[id] {
			walk(s)
		}
	}
	walk(startID)

	// The merge each handler settles on: the first one reachable from where the
	// error edge lands, following the handler's own path.
	needsLabel := map[model.ID]bool{}
	for _, ef := range errorFlows {
		m := firstMergeFrom(ef.DestinationID, objects, normalSucc)
		if m != "" && reachable[m] {
			needsLabel[m] = true
		}
	}
	if len(needsLabel) == 0 {
		return nil
	}

	// Label in position order so the same graph always describes the same way —
	// map iteration order would make DESCRIBE output unstable, which turns every
	// re-describe into a spurious diff.
	ids := make([]model.ID, 0, len(needsLabel))
	for id := range needsLabel {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, pj := objects[ids[i]].GetPosition(), objects[ids[j]].GetPosition()
		if pi.X != pj.X {
			return pi.X < pj.X
		}
		if pi.Y != pj.Y {
			return pi.Y < pj.Y
		}
		return ids[i] < ids[j]
	})

	out := make(mergeLabels, len(ids))
	for i, id := range ids {
		out[id] = fmt.Sprintf("rejoin%d", i+1)
	}
	return out
}

// firstMergeFrom walks forward from a node over normal edges and returns the
// first ExclusiveMerge it meets, or "" if the path terminates without one.
//
// Breadth-first, so "first" means nearest rather than whichever branch the walk
// happened to take.
func firstMergeFrom(
	start model.ID,
	objects map[model.ID]microflows.MicroflowObject,
	succ map[model.ID][]model.ID,
) model.ID {
	if start == "" {
		return ""
	}
	if _, ok := objects[start].(*microflows.ExclusiveMerge); ok {
		return start
	}
	seen := map[model.ID]bool{start: true}
	queue := []model.ID{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, next := range succ[id] {
			if seen[next] {
				continue
			}
			seen[next] = true
			if _, ok := objects[next].(*microflows.ExclusiveMerge); ok {
				return next
			}
			queue = append(queue, next)
		}
	}
	return ""
}

// mergeDeclarationLines renders `merge <label>;` with the merge's stored
// position, so describe → exec → describe is a fixed point.
//
// Without the @position the rebuild places the merge wherever the layout cursor
// happens to be, and the SECOND description reports a different coordinate —
// a diff on every re-describe of an unchanged microflow, which is exactly what
// ADR-0008's idempotence is for.
func mergeDeclarationLines(indent int, label string, obj microflows.MicroflowObject) []string {
	pad := strings.Repeat("  ", indent)
	if obj == nil {
		return []string{pad + "merge " + label + ";"}
	}
	p := obj.GetPosition()
	return []string{
		fmt.Sprintf("%s@position(%d, %d)", pad, p.X, p.Y),
		pad + "merge " + label + ";",
	}
}
