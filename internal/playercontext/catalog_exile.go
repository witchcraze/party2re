package playercontext

// Legacy lib/exile.cgi commands ＠ついほう, ＠さんせい and ＠はんたい.
var exileCatalog = []ActionDefinition{
	{ID: "exile_petition", Label: "追放を申請する", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"target_name", "reason"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "exile_approve", Label: "追放に賛成する", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"target_name"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
	{ID: "exile_oppose", Label: "追放に反対する", Category: "social", OperationID: "executeCharacterAction", RequiredParams: []string{"target_name"}, RequiredGates: GateSleepCheck | GateCooldownCheck | GateLocationCheck},
}
