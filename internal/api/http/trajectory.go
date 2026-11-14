package http

import (
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type trajectoryMemory struct {
	Kind      corecharacter.MemoryKind `json:"kind"`
	Content   string                   `json:"content"`
	Name      string                   `json:"name"`
	GuildName string                   `json:"guild_name"`
	Color     string                   `json:"color"`
	Level     int                      `json:"level"`
	JobID     string                   `json:"job_id"`
	Gender    string                   `json:"gender"`
	SP        int                      `json:"sp"`
	CreatedAt time.Time                `json:"created_at"`
}

type trajectoryResponse struct {
	Name            string `json:"name"`
	CompletionRates struct {
		Weapon  int `json:"weapon"`
		Armor   int `json:"armor"`
		Item    int `json:"item"`
		Alchemy int `json:"alchemy"`
	} `json:"completion_rates"`
	Memories []trajectoryMemory `json:"memories"`
}

// handleGetCharacterTrajectory returns the public milestone log and collection
// completion rates of a character (legacy player.cgi).
func (h *Handler) handleGetCharacterTrajectory(w http.ResponseWriter, r *http.Request) {
	if h.collections == nil || h.alchemy == nil {
		writeError(w, http.StatusNotImplemented, errors.New("trajectory is not configured"))
		return
	}
	ctx, charID := r.Context(), r.PathValue("id")
	char, err := h.characters.Get(ctx, charID)
	if errors.Is(err, corecharacter.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
		return
	}
	var res trajectoryResponse
	res.Name = char.Name
	if err == nil {
		err = h.fillTrajectory(r, charID, &res)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get character trajectory"})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) fillTrajectory(r *http.Request, charID string, res *trajectoryResponse) error {
	ctx := r.Context()
	memories, err := h.characters.Trajectory(ctx, charID)
	if err != nil {
		return err
	}
	res.Memories = make([]trajectoryMemory, len(memories))
	for i, m := range memories {
		res.Memories[i] = trajectoryMemory(m)
	}
	_, weapon, err := h.collections.GetWeaponCollection(ctx, charID)
	if err != nil {
		return err
	}
	_, armor, err := h.collections.GetArmorCollection(ctx, charID)
	if err != nil {
		return err
	}
	_, item, err := h.collections.GetItemCollection(ctx, charID, "item")
	if err != nil {
		return err
	}
	compendium, err := h.alchemy.GetCompendium(ctx, charID)
	if err != nil {
		return err
	}
	// Whole percent, rounded down and capped at 100 as in legacy.
	res.CompletionRates.Weapon = min(int(weapon.CompletionPercentage), 100)
	res.CompletionRates.Armor = min(int(armor.CompletionPercentage), 100)
	res.CompletionRates.Item = min(int(item.CompletionPercentage), 100)
	res.CompletionRates.Alchemy = min(compendium.CompletionPercentage, 100)
	return nil
}
