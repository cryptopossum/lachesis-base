package election

import (
	"errors"
	"sort"

	"github.com/Fantom-foundation/go-lachesis/src/hash"
)

type sortedRoot struct {
	stake Amount
	root  hash.Event
}
type sortedRoots []sortedRoot

func (s sortedRoots) Len() int {
	return len(s)
}
func (s sortedRoots) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// compare by stake amount, root hash
func (s sortedRoots) Less(i, j int) bool {
	if s[i].stake == s[j].stake {
		return s[i].root.Big().Cmp(s[j].root.Big()) < 0
	}
	return s[i].stake > s[j].stake
}

// Chooses the decided "yes" roots with the greatest stake amount.
// This root serves as a "checkpoint" within DAG, as it's guaranteed to be final and consistent unless more than 1/3n are Byzantine.
// Other members will come to the same SfWitness not later than current highest frame + 2.
func (el *Election) chooseSfWitness() (*ElectionRes, error) {
	finalRoots := make(sortedRoots, 0, len(el.members))
	// fill yesRoots
	for _, member := range el.members {
		vote, ok := el.decidedRoots[member.Addr]
		if !ok {
			el.Fatal("called before all the roots are decided")
		}
		if vote.yes {
			finalRoots = append(finalRoots, sortedRoot{
				root:  vote.seenRoot,
				stake: Amount(member.Stake),
			})
		}
	}
	if len(finalRoots) == 0 {
		return nil, errors.New("All the roots aren't decided as 'yes', which is possible only if more than 1/3n are Byzantine")
	}

	// sort by stake amount, root hash
	sort.Sort(finalRoots)

	// take root with greatest stake
	return &ElectionRes{
		DecidedFrame:     el.frameToDecide,
		DecidedSfWitness: finalRoots[0].root,
	}, nil
}
