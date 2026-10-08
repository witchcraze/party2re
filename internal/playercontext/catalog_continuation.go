package playercontext

// These controls are deliberately unconnected until their mutation migration.
// Their role/phase eligibility comes from public feature reads, not town gates.
var continuationCatalog = []ActionDefinition{
	{ID: "party_ready", Label: "準備する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id", "ready"}, ActivityKind: "party"},
	{ID: "party_start", Label: "冒険を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party"},
	{ID: "party_leave", Label: "パーティーを離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"party_id"}, ActivityKind: "party", Recovery: true},
	{ID: "pvp_team", Label: "チームを選ぶ", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "team_color"}, ActivityKind: "pvp"},
	{ID: "pvp_start", Label: "対戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp"},
	{ID: "pvp_advance", Label: "対戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp"},
	{ID: "pvp_leave", Label: "対戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "pvp", Recovery: true},
	{ID: "gvg_start", Label: "ギルド戦を始める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg"},
	{ID: "gvg_advance", Label: "ギルド戦を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg"},
	{ID: "gvg_leave", Label: "ギルド戦部屋を離れる", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "gvg", Recovery: true},
	{ID: "dungeon_move", Label: "探索を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"direction"}, ActivityKind: "dungeon"},
	{ID: "dungeon_escape", Label: "探索から脱出する", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{}, ActivityKind: "dungeon", Recovery: true},
	{ID: "challenge_advance", Label: "試練を進める", Category: "adventure", OperationID: "executeCharacterAction", RequiredParams: []string{"session_id"}, ActivityKind: "challenge"},
	{ID: "casino_room_start", Label: "ゲームを始める", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino"},
	{ID: "casino_room_leave", Label: "カジノ部屋を離れる", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, ActivityKind: "casino", Recovery: true},
	{ID: "casino_room_action", Label: "ゲームの行動を選ぶ", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id", "action"}, ActivityKind: "casino"},
}
