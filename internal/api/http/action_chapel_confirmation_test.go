package http

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type chapelNameReader struct {
	*gatewayFixture
	err   error
	reads int
}

func (r *chapelNameReader) Get(ctx context.Context, actor string) (corecharacter.Character, error) {
	r.checkContext(ctx)
	r.reads++
	if r.reads == 2 {
		return corecharacter.Character{}, r.err
	}
	return r.gatewayFixture.Get(ctx, actor)
}

func TestChapelGatewayNameReadFailureDoesNotPray(t *testing.T) {
	for _, failure := range []struct {
		err    error
		status int
		code   string
	}{
		{corecharacter.ErrNotFound, 404, "CHARACTER_NOT_FOUND"},
		{context.Canceled, 500, "EXECUTION_FAILED"},
		{errors.New("name storage unavailable"), 500, "EXECUTION_FAILED"},
	} {
		t.Run(failure.err.Error(), func(t *testing.T) {
			f := &chapelGatewayFixture{gatewayFixture: newGatewayFixture(t), blessing: chapel.CharacterBlessing{CharacterID: "hero", ActiveBlessing: chapel.BlessingNone}}
			f.expectedContext = context.WithValue(context.Background(), struct{}{}, "name read")
			reader := &chapelNameReader{gatewayFixture: f.gatewayFixture, err: failure.err}
			pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil))
			h, err := NewHandler(f.gatewayFixture, reader, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithChapel(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
			if err != nil {
				t.Fatal(err)
			}
			status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"chapel_pray","params":{"blessing":"GOLD"}}`, f.expectedContext)
			if status != failure.status || reader.reads != 2 || f.executions != 0 || f.blessing.ActiveBlessing != chapel.BlessingNone || !f.blessing.PrayedAt.IsZero() {
				t.Fatalf("failed name read consumed prayer: status=%d reads=%d calls=%d response=%s", status, reader.reads, f.executions, got)
			}
			assertGatewayError(t, got, failure.code)
		})
	}
}
