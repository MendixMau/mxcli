// SPDX-License-Identifier: Apache-2.0

package executor

// Mode 2 of PROPOSAL_structured_microflow_description.md: describe a graph whose
// branches CROSS faithfully, instead of flattening it into MDL that means
// something else.
//
// Phase E made an error path's rejoin sayable (`merge`/`join`). This says the
// other half with the same vocabulary: an inner split's branch landing where an
// outer split's branch lands — the shape no nesting of `if` reproduces, and the
// one #923 was actually reported for.
//
// # Why a crossed merge is handled the opposite way from a rejoin merge
//
// Both get a label, but they are emitted differently, and getting this backwards
// produces MDL that does not parse:
//
//   - An ERROR-REJOIN merge is declared INLINE where the traversal meets it
//     (traverseFlowUntilMerge does this), because the normal path owns it and
//     only the handler needs to name it.
//   - A CROSSED merge is reached by two or more BRANCHES of the same split. It
//     can be declared only once, so each branch ends in `join <label>` and stops,
//     and the declaration is emitted at the split's own level once both branches
//     are closed.
//
// That is why crossedMerges is a separate set rather than more entries in the
// same map: the label alone does not say which of the two emissions applies.

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// labelCrossedMerges labels the shared-suffix entry of every RECOMBINABLE split,
// which is exactly the node a nested rendering cannot place.
//
// It deliberately ignores Interleaved findings. Those overlap at more than one
// entry, and per Böhm-Jacopini nesting them needs either a duplicated activity
// or a boolean variable the user never wrote; `merge`/`join` can express the
// graph, but the describer's branch structure cannot be built from one entry, so
// they keep MDL-FLOW01 and are left alone rather than half-described.
//
// Labels are assigned in position order, for the same reason labelRejoinMerges
// does it: map iteration order would make DESCRIBE unstable and turn every
// re-describe into a spurious diff.
func labelCrossedMerges(col *microflows.MicroflowObjectCollection) mergeLabels {
	if col == nil {
		return mergeLabels{}
	}

	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range col.Objects {
		if o != nil {
			objects[o.GetID()] = o
		}
	}

	// BOTH the shared entry and the split's post-dominator are labelled, and the
	// second is not optional. mxcli's `merge` permits fall-through (an ordinary
	// path entering a following `merge` joins it), which is friendly for a
	// hand-author and ambiguous for a generated description: split2's false arm
	// is EMPTY here and belongs at the post-dominator, but an empty arm followed
	// by `merge shared1` falls into shared1 instead — silently describing a
	// different graph, which is the whole failure Mode 2 exists to end.
	//
	// Labelling the join too lets every branch end in an explicit `join`, which
	// is what the proposal's Mode 2 sketch does and why it wanted fall-through
	// banned outright.
	needsLabel := map[model.ID]bool{}
	for _, f := range microflowgraph.Analyze(col.Objects, col.Flows) {
		if f.Class != microflowgraph.Recombinable || len(f.Entries) != 1 {
			continue
		}
		// When the shared entry IS the split's own post-dominator, nothing is
		// crossed: `end if` / `end split` already places it, and the nested
		// description is faithful. Analyze reports these because some branches
		// reach the tail and others return early — measured on
		// Administration.ManageMyAccount, whose description round-trips with its
		// merge $ID intact and which therefore never needed naming. Labelling it
		// would put a `join` where the structure is already right, and (worse)
		// would retire MDL-FLOW01 on the strength of a label nothing used.
		if f.Entries[0] == f.JoinID {
			continue
		}
		for _, id := range []model.ID{f.Entries[0], f.JoinID} {
			if _, isMerge := objects[id].(*microflows.ExclusiveMerge); isMerge {
				needsLabel[id] = true
			}
		}
	}
	if len(needsLabel) == 0 {
		return mergeLabels{}
	}

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

	out := mergeLabels{
		byID:    make(map[model.ID]string, len(ids)),
		crossed: make(map[model.ID]bool, len(ids)),
	}
	for i, id := range ids {
		out.byID[id] = fmt.Sprintf("shared%d", i+1)
		out.crossed[id] = true
	}
	return out
}

// mergeAllLabels combines the rejoin labels with the crossed ones.
//
// A merge that is BOTH an error rejoin and a crossed entry keeps its rejoin
// label — Phase E's output is already in the wild and its label names appear in
// scripts people have saved — but it joins the crossed set, because how it is
// emitted is decided by the branch structure, not by which pass named it.
func mergeAllLabels(rejoin, crossed mergeLabels) mergeLabels {
	if rejoin.len() == 0 {
		return crossed
	}
	if crossed.len() == 0 {
		return rejoin
	}
	out := mergeLabels{
		byID:    make(map[model.ID]string, rejoin.len()+crossed.len()),
		crossed: crossed.crossed,
	}
	for id, l := range crossed.byID {
		out.byID[id] = l
	}
	for id, l := range rejoin.byID {
		out.byID[id] = l
	}
	return out
}

// emitCrossedMergeSections writes the declaration and body of every crossed
// merge, after the main traversal has finished and each branch has emitted its
// `join`.
//
// Order is label order, which is position order, so the same graph always
// describes the same way.
//
// Each section traverses from the merge's SUCCESSOR rather than from the merge
// itself. That is what keeps the two emissions from colliding: the merge node is
// never "arrived at" during its own section, so the join-and-stop rule in
// traverseFlow cannot fire here and turn the declaration into a `join` to
// itself. The body then runs on until it reaches the NEXT crossed merge, where
// it emits a `join` and stops — which is how a chain of shared suffixes comes
// out as a flat sequence of sections instead of a nesting that does not exist.
func emitCrossedMergeSections(
	ctx *ExecContext,
	col *microflows.MicroflowObjectCollection,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) map[model.ID]bool {
	declared := map[model.ID]bool{}
	if len(labels.crossed) == 0 {
		return declared
	}

	ids := make([]model.ID, 0, len(labels.crossed))
	for id := range labels.crossed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		li, _ := labels.of(ids[i])
		lj, _ := labels.of(ids[j])
		if li != lj {
			return li < lj
		}
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		if visited[id] {
			continue
		}
		label, ok := labels.of(id)
		if !ok {
			continue
		}
		visited[id] = true
		declared[id] = true
		*lines = append(*lines, mergeDeclarationLines(0, label, activityMap[id])...)
		for _, flow := range flowsByOrigin[id] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest,
				splitMergeMap, visited, entityNames, microflowNames, lines, 0,
				sourceMap, headerLineCount, annotationsByTarget, labels)
		}
	}
	return declared
}
