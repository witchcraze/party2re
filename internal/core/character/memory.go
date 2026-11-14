package character

import "context"

// MemoryKind classifies a milestone in a character's trajectory (legacy write_memory).
type MemoryKind string

const (
	MemoryBirth        MemoryKind = "birth"
	MemoryNameChange   MemoryKind = "name_change"
	MemoryGenderChange MemoryKind = "gender_change"
	MemoryJobChange    MemoryKind = "job_change"
	MemoryJobMaster    MemoryKind = "job_master"
	MemoryJobRecall    MemoryKind = "job_recall"
	MemoryJobRevert    MemoryKind = "job_revert"
	MemoryCompletion   MemoryKind = "completion"
	MemoryGuildFound   MemoryKind = "guild_found"
	MemoryGuildJoin    MemoryKind = "guild_join"
	MemoryGuildLeave   MemoryKind = "guild_leave"
	MemoryGuildDisband MemoryKind = "guild_disband"
	MemoryWish         MemoryKind = "wish"
	MemoryContestPrize MemoryKind = "contest_prize"
)

// MemoryRecorder appends a milestone to a character's trajectory inside the
// caller's transaction. The entry keeps the character's name, guild, level and
// job as stored at that moment, so callers choose the moment by where they call.
type MemoryRecorder func(ctx context.Context, characterID string, kind MemoryKind, content string) error

// Record calls the recorder; a nil recorder records nothing.
func (r MemoryRecorder) Record(ctx context.Context, characterID string, kind MemoryKind, content string) error {
	if r == nil {
		return nil
	}
	return r(ctx, characterID, kind, content)
}
