package battle_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/battle"
)

func BenchmarkBattleSimulation(b *testing.B) {
	req := battle.Request{
		Participants: []battle.Participant{
			{ID: "hero", Name: "Hero", HP: 100, Attack: 25, Defense: 10},
			{ID: "monster", Name: "Slime", HP: 80, Attack: 18, Defense: 8},
		},
		VictoryReward: battle.Reward{Experience: 50, Currency: 30},
	}

	engine := battle.Engine{}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.Resolve(req)
		if err != nil {
			b.Fatalf("battle resolve failed: %v", err)
		}
	}
}

func BenchmarkBattleSimulationMultiTurn(b *testing.B) {
	req := battle.Request{
		Participants: []battle.Participant{
			{ID: "hero", Name: "Hero", HP: 1000, Attack: 50, Defense: 30},
			{ID: "boss", Name: "Demon King", HP: 1500, Attack: 45, Defense: 25},
		},
		VictoryReward: battle.Reward{Experience: 500, Currency: 1000},
	}

	engine := battle.Engine{}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := engine.Resolve(req)
		if err != nil {
			b.Fatalf("battle resolve failed: %v", err)
		}
	}
}
