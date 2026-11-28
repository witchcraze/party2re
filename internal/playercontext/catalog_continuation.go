package playercontext

// Controls are connected by their individual mutation adapters.
// Their role/phase eligibility comes from public feature reads, not town gates.
var continuationCatalog = []ActionDefinition{
	{ID: "party_start", Label: "冒険を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party", WaitExempt: WaitActivityPacing},
	{ID: "party_leave", Label: "パーティーを離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "party_kick", Label: "パーティーのメンバーを外す", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id", "target_character_id"}, ActivityKind: "party", WaitExempt: WaitActivityPacing},
	{ID: "pvp_team", Label: "チームを選ぶ", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "team_color"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_start", Label: "対戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_kick", Label: "部屋のメンバーを外す", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "target_character_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	// ＠さそう whispers into the quest place log with act_time = 0 (_skill.cgi:769-779).
	{ID: "pvp_invite", Label: "冒険者をさそう", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitWhisper, LegacyVerb: "さそう"},
	{ID: "pvp_advance", Label: "対戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_leave", Label: "対戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "gvg_start", Label: "ギルド戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing},
	{ID: "gvg_advance", Label: "ギルド戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "expected_round"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing},
	{ID: "gvg_leave", Label: "ギルド戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "dungeon_move", Label: "探索を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"direction"}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing},
	{ID: "dungeon_mine", Label: "採掘する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing},
	{ID: "dungeon_fight", Label: "探索の戦闘を続ける", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing},
	{ID: "dungeon_escape", Label: "探索から脱出する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "challenge_advance", Label: "試練を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"session_id"}, ActivityKind: "challenge", WaitExempt: WaitActivityPacing},
	{ID: "challenge_retire", Label: "試練から退出する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"session_id", "expected_revision"}, ActivityKind: "challenge", WaitExempt: WaitActivityPacing, Recovery: true},
	// Run commands are mode-neutral; their eligibility comes from the run's owned observation.
	{ID: "run_operation", Label: "行動する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"run_id", "version", "command_id", "operation"}, ActivityKind: "adventure", WaitExempt: WaitActivityPacing},
	{ID: "run_chest", Label: "宝箱をしらべる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"run_id", "version", "command_id", "box_id"}, ActivityKind: "adventure", WaitExempt: WaitActivityPacing},
	{ID: "run_exit", Label: "にげる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"run_id", "version", "command_id"}, ActivityKind: "adventure", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "run_recover", Label: "放置された冒険を片付ける", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"run_id", "version", "command_id"}, ActivityKind: "adventure", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "casino_room_start", Label: "ゲームを始める", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
	{ID: "casino_room_leave", Label: "カジノ部屋を離れる", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "casino_room_action", Label: "ゲームの行動を選ぶ", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "action"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
	// ＠さそう whispers into the casino lobby log with act_time = 0 (_casino.cgi:122-133).
	{ID: "casino_room_invite", Label: "ロビーの人をさそう", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", WaitExempt: WaitWhisper, LegacyVerb: "さそう"},
	{ID: "casino_room_kick", Label: "部屋のメンバーを外す", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "target_character_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
}
