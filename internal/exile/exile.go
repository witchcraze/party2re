// Package exile implements the player-run exile vote board (legacy lib/exile.cgi).
package exile

import (
	"errors"
	"time"
)

// Legacy settings from lib/exile.cgi.
const (
	MinJobLevel       = 50
	RequiredVotes     = 10
	GuiltySleep       = time.Hour
	AcquittalSleep    = 5 * 24 * time.Hour
	MaxOpenPetitions  = 3 // compared after incrementing per existing row, so a 4th petition is admitted
	CancelLockWindow  = 3 * time.Hour
	MaxCommandLength  = 500
	petitionSyntaxLen = 17 // escaped "＠ついほう>" and "＠りゆう>" around the name and reason
)

var (
	ErrInvalidTarget      = errors.New("exile target name and reason are required")
	ErrInvalidReason      = errors.New("exile reason is invalid")
	ErrJobLevelTooLow     = errors.New("exile requires at least 50 job changes")
	ErrSelfPetition       = errors.New("cannot petition to exile yourself")
	ErrTargetNotFound     = errors.New("exile target does not exist")
	ErrAlreadyPetitioned  = errors.New("target already has an open exile petition")
	ErrPetitionLimit      = errors.New("too many open exile petitions")
	ErrCancelLocked       = errors.New("exile petitions cannot be withdrawn shortly after a vote")
	ErrAlreadyVoted       = errors.New("already voted on this exile petition")
	ErrTargetCannotVote   = errors.New("the petitioned character cannot vote")
	ErrPetitionNotFound   = errors.New("no exile petition matched")
	ErrInvalidCharacterID = errors.New("invalid character ID")
)

// Vote is one ballot. The applicant's own approval is recorded on submission.
type Vote struct {
	VoterID   string    `json:"voter_id"`
	VoterName string    `json:"voter_name"`
	Approve   bool      `json:"approve"`
	VotedAt   time.Time `json:"voted_at"`
}

// Petition names are immutable copies; renaming is refused while involved.
type Petition struct {
	ID            string    `json:"id"`
	ApplicantID   string    `json:"applicant_id"`
	ApplicantName string    `json:"applicant_name"`
	TargetID      string    `json:"target_id"`
	TargetName    string    `json:"target_name"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
	Votes         []Vote    `json:"votes"` // newest first
}

// Tally counts approvals and oppositions.
func (p Petition) Tally() (approvals, oppositions int) {
	for _, v := range p.Votes {
		if v.Approve {
			approvals++
		} else {
			oppositions++
		}
	}
	return approvals, oppositions
}

func (p Petition) hasVoter(characterID string) bool {
	for _, v := range p.Votes {
		if v.VoterID == characterID {
			return true
		}
	}
	return false
}

// Sentence is a committed verdict sleep deadline awaiting timer projection.
type Sentence struct {
	CharacterID string
	SleepUntil  time.Time
}

// Verdict is empty while voting continues.
type Verdict string

const (
	VerdictGuilty    Verdict = "guilty"
	VerdictAcquitted Verdict = "acquitted"
)

// VoteResult reports withdrawn own petitions and the voted petition, if any.
type VoteResult struct {
	Withdrawn   []Petition `json:"withdrawn"`
	Petition    *Petition  `json:"petition,omitempty"`
	Approvals   int        `json:"approvals"`
	Oppositions int        `json:"oppositions"`
	Verdict     Verdict    `json:"verdict,omitempty"`
}

// Board is the eligible actor's view; ineligible actors see no petitions.
type Board struct {
	Eligible  bool       `json:"eligible"`
	Petitions []Petition `json:"petitions"`
}
