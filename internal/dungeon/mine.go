package dungeon

import "context"

// Mine is map16's さいくつ: a living member digs at the party's active site
// while no living enemy or unopened chest remains. It spends no round and
// ignores the round limit (map/16/1.cgi:37–77). A depleted site accepts the
// command with no draw and no change.
func (s *Service) Mine(ctx context.Context, characterID string) (ExpeditionStepResult, error) {
	run, err := s.admitStep(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	exp := run.exp
	st, err := map16Run(*exp)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if run.floor.mine == nil || st.Site == "" {
		return ExpeditionStepResult{}, ErrNoMiningSite
	}
	if st.depleted(st.Site) {
		return ExpeditionStepResult{Expedition: *exp, EventType: EventMineDepleted}, nil
	}
	// Only opened chests can remain; the step drops them.
	clearing := exp.Encounter != nil
	exp.Encounter = nil
	n := nativeStep{
		exp: exp, leader: &run.owner, actor: &run.actor, dungeon: run.dungeon, floor: run.floor,
		cell: run.floor.Grid[exp.PosY][exp.PosX], rng: s.rng,
		step: StepParams{
			ExpectedExpeditionID: exp.ID,
			ExpectedRevision:     &exp.Revision,
			Combat:               clearing,
			NewX:                 exp.PosX,
			NewY:                 exp.PosY,
			StockDrain:           run.actor.ID,
			Now:                  s.now().UTC(),
		},
	}
	if err := run.floor.mine.apply(&n); err != nil {
		return ExpeditionStepResult{}, err
	}
	res, err := s.finishNative(ctx, &n, nil, exp.PosX, exp.PosY)
	if err == nil && res.EventType == EventMove {
		res.EventType = EventMineEmpty
	}
	return res, err
}

// canMine reports that actor may mine now: a living member of an exploring
// map16 run at an active site with no living enemy or unopened chest.
// Depletion is not disclosed.
func (exp ActiveExpedition) canMine(actor string) bool {
	if exp.Status != StatusExploring || exp.DungeonID != nativeMap16ID || !exp.isAlive(actor) || exp.Encounter.blocking() {
		return false
	}
	st, err := map16Run(exp)
	return err == nil && st.Site != ""
}
