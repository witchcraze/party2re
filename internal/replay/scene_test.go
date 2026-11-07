package replay_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/replay"
)

const (
	ally  = "#FFCC00"
	enemy = "#FF0000"
)

func baseCapture() replay.SceneCapture {
	return replay.SceneCapture{Title: "地下1階", Floor: 1, Background: "cave", Overlays: []string{"mist"}, ActorColor: ally}
}

func TestProjectMasksHPIndependentlyAndOmitsHiddenNumbers(t *testing.T) {
	c := baseCapture()
	c.Members = []replay.MemberCapture{
		{Name: "勇者", Color: ally, HP: 500, MaxHP: 1500, Appearance: "hero", Effects: []string{"glow"}},
		{Name: "竜", Color: enemy, HP: 1234, MaxHP: 5678},
		{Name: "猫", Color: ally, HP: 999, MaxHP: 999},
	}
	s, err := replay.Project(c)
	if err != nil {
		t.Fatal(err)
	}
	if m := s.Members[0]; m.HP == nil || *m.HP != 500 || m.MaxHP != nil || m.HPPercent != 33 {
		t.Fatalf("partially masked member = %+v", m)
	}
	if m := s.Members[1]; m.HP != nil || m.MaxHP != nil || m.HPPercent != 21 {
		t.Fatalf("fully masked member = %+v", m)
	}
	if m := s.Members[2]; m.HP == nil || *m.HP != 999 || m.MaxHP == nil || *m.MaxHP != 999 {
		t.Fatalf("boundary 999 must stay visible: %+v", m)
	}
	encoded, err := replay.EncodeScene(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"1500", "1234", "5678"} {
		if strings.Contains(encoded, hidden) {
			t.Fatalf("hidden value %s persisted: %s", hidden, encoded)
		}
	}
}

func TestProjectAppliesActorViewToDefeatedMembers(t *testing.T) {
	c := baseCapture()
	c.Members = []replay.MemberCapture{
		{Name: "倒れた仲間", Color: ally, HP: 0, MaxHP: 100, Appearance: "hero"},
		{Name: "倒れた敵", Color: enemy, HP: 0, MaxHP: 100},
		{Name: "@身代わり", Color: ally, HP: 0, MaxHP: 100, Substitute: true},
		{Name: "敵", Color: enemy, HP: 10, MaxHP: 100},
	}
	s, err := replay.Project(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Members) != 2 || s.Members[0].Name != "倒れた仲間" || !s.Members[0].Fallen || s.Members[0].Appearance != "" || s.Members[1].Name != "敵" {
		t.Fatalf("members = %+v", s.Members)
	}
	// The opponent's own perspective keeps its fallen ally and hides ours.
	c.ActorColor = enemy
	s, err = replay.Project(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Members) != 2 || s.Members[0].Name != "倒れた敵" || s.Members[1].Name != "敵" {
		t.Fatalf("opponent view = %+v", s.Members)
	}
}

func TestProjectStateSupersedesTension(t *testing.T) {
	c := baseCapture()
	c.Members = []replay.MemberCapture{
		{Name: "a", Color: ally, HP: 1, MaxHP: 1, State: "毒", Tension: "ハイ"},
		{Name: "b", Color: ally, HP: 1, MaxHP: 1, Temporary: "盾", ExtendedTurns: 3},
		{Name: "c", Color: ally, HP: 1, MaxHP: 1, Tension: "ハイ", ExtendedTurns: 3},
	}
	s, err := replay.Project(c)
	if err != nil {
		t.Fatal(err)
	}
	if m := s.Members[0]; m.State != "毒" || m.Tension != "" {
		t.Fatalf("state member = %+v", m)
	}
	if m := s.Members[1]; m.Temporary != "盾" || m.ExtendedTurns != 3 {
		t.Fatalf("temporary member = %+v", m)
	}
	if m := s.Members[2]; m.Tension != "ハイ" || m.ExtendedTurns != 0 {
		t.Fatalf("tension member = %+v", m)
	}
}

func TestProjectKeepsThreeNewestNonWhisperComments(t *testing.T) {
	c := baseCapture()
	c.Log = []replay.LogEntry{
		{Speaker: "a", Color: ally, Text: "4", Date: "d4"},
		{Speaker: "b", Color: ally, Text: "secret", Date: "d3", Whisper: true},
		{Speaker: "c", Color: ally, Text: "3", Date: "d2"},
		{Speaker: "d", Color: ally, Text: "2 <3 a > b", Date: "d1"},
		{Speaker: "e", Color: ally, Text: "1", Date: "d0"},
	}
	s, err := replay.Project(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Comments) != 3 || s.Comments[0].Text != "4" || s.Comments[1].Text != "3" || s.Comments[2].Text != "2 <3 a > b" {
		t.Fatalf("comments = %+v", s.Comments)
	}
	encoded, err := replay.EncodeScene(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded, "secret") || !strings.Contains(encoded, "<3") {
		t.Fatalf("encoding = %s", encoded)
	}
}

func TestProjectRejectsMarkupAndRawPresentation(t *testing.T) {
	member := replay.MemberCapture{Name: "a", Color: ally, HP: 1, MaxHP: 1}
	for name, mutate := range map[string]func(*replay.SceneCapture){
		"title tag":   func(c *replay.SceneCapture) { c.Title = `<script>x</script>` },
		"asset path":  func(c *replay.SceneCapture) { c.Background = "bgimg/field.gif" },
		"url overlay": func(c *replay.SceneCapture) { c.Overlays = []string{"http://x"} },
		"css color":   func(c *replay.SceneCapture) { c.ActorColor = "red;background:url(x)" },
		"member markup": func(c *replay.SceneCapture) {
			m := member
			m.Name = `<img src=x>`
			c.Members = []replay.MemberCapture{m}
		},
		"zero max hp":     func(c *replay.SceneCapture) { m := member; m.MaxHP = 0; c.Members = []replay.MemberCapture{m} },
		"comment control": func(c *replay.SceneCapture) { c.Log = []replay.LogEntry{{Speaker: "a", Color: ally, Text: "x\x00"}} },
	} {
		t.Run(name, func(t *testing.T) {
			c := baseCapture()
			mutate(&c)
			if _, err := replay.Project(c); !errors.Is(err, replay.ErrInvalidScene) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestEncodeSceneIsStableUTF8(t *testing.T) {
	s, err := replay.Project(baseCapture())
	if err != nil {
		t.Fatal(err)
	}
	a, err := replay.EncodeScene(s)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := replay.EncodeScene(s)
	want := `{"v":1,"title":"地下1階","floor":1,"background":"cave","overlays":["mist"],"members":[],"comments":[]}`
	if a != b || a != want {
		t.Fatalf("encoding = %s", a)
	}
}
