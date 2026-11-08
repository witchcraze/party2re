package playercontext

// Controls are connected by their individual mutation adapters.
// Their role/phase eligibility comes from public feature reads, not town gates.
var continuationCatalog = []ActionDefinition{
	{ID: "party_ready", Label: "準備する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id", "ready"}, ActivityKind: "party", WaitExempt: WaitActivityPacing},
	{ID: "party_start", Label: "冒険を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party", WaitExempt: WaitActivityPacing},
	{ID: "party_leave", Label: "パーティーを離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "pvp_team", Label: "チームを選ぶ", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "team_color"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_start", Label: "対戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_advance", Label: "対戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing},
	{ID: "pvp_leave", Label: "対戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "gvg_start", Label: "ギルド戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing},
	{ID: "gvg_advance", Label: "ギルド戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "expected_round"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing},
	{ID: "gvg_leave", Label: "ギルド戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "dungeon_move", Label: "探索を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"direction"}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing},
	{ID: "dungeon_fight", Label: "探索の戦闘を続ける", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing},
	{ID: "dungeon_escape", Label: "探索から脱出する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "challenge_advance", Label: "試練を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"session_id"}, ActivityKind: "challenge", WaitExempt: WaitActivityPacing},
	{ID: "challenge_retire", Label: "試練から退出する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"session_id", "expected_revision"}, ActivityKind: "challenge", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "casino_room_start", Label: "ゲームを始める", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
	{ID: "casino_room_leave", Label: "カジノ部屋を離れる", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing, Recovery: true},
	{ID: "casino_room_action", Label: "ゲームの行動を選ぶ", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "action"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
	{ID: "casino_room_kick", Label: "部屋のメンバーを外す", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "target_character_id"}, ActivityKind: "casino", WaitExempt: WaitActivityPacing},
}
