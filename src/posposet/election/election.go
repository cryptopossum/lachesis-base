package election

import (
	"github.com/Fantom-foundation/go-lachesis/src/hash"
	"github.com/Fantom-foundation/go-lachesis/src/logger"
	"github.com/Fantom-foundation/go-lachesis/src/posposet/members"
)

// TODO implement&test coinRound
//const coinRound = 10 // every 10th round is a round with pseudorandom votes

type IdxFrame uint64
type Amount uint64

type Election struct {
	// election params
	frameToDecide IdxFrame

	members       []members.Member
	totalStake    Amount // the sum of stakes (n)
	superMajority Amount // the quorum (should be 2/3n + 1)

	// election state
	decidedRoots map[hash.Peer]voteValue // decided roots at "frameToDecide"
	votes        map[voteId]voteValue

	// external world
	stronglySee RootStronglySeeRootFn

	logger.Instance
}

// specifies a slot {addr, frame}. Normal members can have only one root with this pair.
// Due to a fork, different roots may occupy the same slot
type RootSlot struct {
	Frame IdxFrame
	Addr  hash.Peer
}

// @return hash of root B, if root A strongly sees root B.
// Due to a fork, there may be many roots B with the same slot,
// but strongly seen may be only one of them (if no more than 1/3n are Byzantine), with a specific hash.
type RootStronglySeeRootFn func(a hash.Event, b RootSlot) *hash.Event

type voteId struct {
	fromRoot  hash.Event
	forMember hash.Peer
}
type voteValue struct {
	decided  bool
	yes      bool
	seenRoot hash.Event
}

type ElectionRes struct {
	DecidedFrame     IdxFrame
	DecidedSfWitness hash.Event
}

func NewElection(
	members members.Members,
	totalStake Amount,
	superMajority Amount,
	frameToDecide IdxFrame,
	stronglySeeFn RootStronglySeeRootFn,
) *Election {
	return &Election{
		members:       members,
		totalStake:    totalStake,
		superMajority: superMajority,
		frameToDecide: frameToDecide,
		decidedRoots:  make(map[hash.Peer]voteValue),
		votes:         make(map[voteId]voteValue),
		stronglySee:   stronglySeeFn,
		Instance:      logger.MakeInstance(),
	}
}

// erase the current election state, prepare for new election frame
func (el *Election) ResetElection(frameToDecide IdxFrame) {
	el.frameToDecide = frameToDecide
	el.votes = make(map[voteId]voteValue)
	el.decidedRoots = make(map[hash.Peer]voteValue)
}

// return root slots which are not within el.decidedRoots
func (el *Election) notDecidedRoots() []hash.Peer {
	notDecidedRoots := make([]hash.Peer, 0, len(el.members))

	for _, member := range el.members {
		if _, ok := el.decidedRoots[member.Addr]; !ok {
			notDecidedRoots = append(notDecidedRoots, member.Addr)
		}
	}
	if len(notDecidedRoots)+len(el.decidedRoots) != len(el.members) { // sanity check
		el.Fatal("Mismatch of roots")
	}
	return notDecidedRoots
}

type weightedRoot struct {
	root  hash.Event
	stake Amount
}

// @return all the roots which are strongly seen by the specified root at the specified frame
func (el *Election) stronglySeenRoots(root hash.Event, frame IdxFrame) []weightedRoot {
	seenRoots := make([]weightedRoot, 0, len(el.members))
	for _, member := range el.members {
		slot := RootSlot{
			Frame: frame,
			Addr:  member.Addr,
		}
		seenRoot := el.stronglySee(root, slot)
		if seenRoot != nil {
			seenRoots = append(seenRoots, weightedRoot{
				root:  *seenRoot,
				stake: Amount(member.Stake),
			})
		}
	}
	return seenRoots
}

type GetRootsFn func(slot RootSlot) hash.Events

// The function is similar to ProcessRoot, but it fully re-processes the current voting.
// This routine should be called after node startup, and after each decided frame.
func (el *Election) ProcessKnownRoots(maxKnownFrame IdxFrame, getRootsFn GetRootsFn) (*ElectionRes, error) {
	// iterate all the roots from lowest frame to highest, call ProcessRootVotes for each
	for frame := el.frameToDecide + 1; frame <= maxKnownFrame; frame++ {
		for _, member := range el.members {
			slot := RootSlot{
				Frame: frame,
				Addr:  member.Addr,
			}
			roots := getRootsFn(slot)
			// if there's more than 1 root, then all of them are forks. it's fine
			for root := range roots {
				decided, err := el.ProcessRoot(root, slot)
				if decided != nil || err != nil {
					return decided, err
				}
			}
		}
	}

	return nil, nil
}
