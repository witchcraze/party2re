package playercontext

// Entry candidates remain unconnected until the Casino mutation migration.
var casinoCatalog = []ActionDefinition{
	{ID: "casino_exchange", Label: "コインに両替する", Category: "entertainment", OperationID: "executeCharacterAction", RequiredParams: []string{"coins"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "casino_prize_exchange", Label: "賞品を交換する", Category: "entertainment", OperationID: "executeCharacterAction", RequiredParams: []string{"cost_coins", "count"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "casino_room_create", Label: "カジノ部屋を作る", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"name", "game_type", "speed", "max_players", "rate", "allow_spectators"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "casino_room_join", Label: "部屋に参加する", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, RequiredGates: GateFatigueCheck | GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "casino_room_spectate", Label: "部屋を観戦する", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"room_id"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
}
