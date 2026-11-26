package runhost

import (
	"context"
	"slices"
)

// RoomStore lists the live runs of one kind for the public quest list.
type RoomStore interface {
	ListRuns(ctx context.Context, kind string) ([]State, error)
}

// QuestRoom is a live run's public quest-list row (quest.cgi quest_html): no
// battle, earnings or recipient facts.
type QuestRoom struct {
	Name       string `json:"name"`
	Speed      int    `json:"speed"`
	StageID    string `json:"stage_id"`
	Mode       string `json:"mode,omitempty"`
	NeedJoin   string `json:"need_join,omitempty"`
	Round      int    `json:"round"`
	LeaderName string `json:"leader_name"`
	// MemberNames follows the roster: the leader first (admission order;
	// exit promotes Roster[0]), then join order.
	MemberNames []string `json:"member_names"`
	MaxMembers  int      `json:"max_members"`
}

// ListQuestRooms lists this kind's live runs, newest first. System rooms (vs_king)
// are listed by their own owner, not here.
func (s *Service[R, P]) ListQuestRooms(ctx context.Context) ([]QuestRoom, error) {
	runs, err := s.cfg.Store.ListRuns(ctx, s.cfg.Kind)
	if err != nil {
		return nil, err
	}
	runs = slices.DeleteFunc(runs, func(r State) bool { return r.Retained || len(r.Roster) == 0 })
	slices.SortStableFunc(runs, func(a, b State) int { return b.StartedAt.Compare(a.StartedAt) })
	rooms := make([]QuestRoom, 0, len(runs))
	for _, r := range runs {
		names := make(map[string]string, len(r.Battle.Actors))
		for _, a := range r.Battle.Actors {
			names[a.ID] = a.Name
		}
		members := make([]string, 0, len(r.Roster))
		for _, id := range r.Roster {
			members = append(members, names[id])
		}
		a := r.Admission
		rooms = append(rooms, QuestRoom{
			Name: a.Name, Speed: a.Speed, StageID: a.StageID, Mode: a.Mode, NeedJoin: a.NeedJoin, Round: r.Round,
			LeaderName: names[r.LeaderID], MemberNames: members, MaxMembers: a.MaxMembers,
		})
	}
	return rooms, nil
}
