# Oracle Shop Design

The Oracle shop (オラクル屋, `@ラクル`) reconstructs legacy `lib/goods.cgi`: it rents costumes and sells home wallpapers. The costume catalog, its job-level gating and the wallpaper prices live in [`internal/store/oracle.go`](../../internal/store/oracle.go) and [`models.go`](../../internal/store/models.go).

- Costumes requested by an active helper quest are not sold. If that request list cannot be read, catalog reads and purchases fail instead of selling under unknown exclusions.
- Home item use applies a costume; the [Home dispatch contract](home.md#legacy-dispatch-and-consumption-contract) owns selection, fallback and reset. Sleep and job change restore the ordinary job appearance. Current Go also expires the costume at midnight JST; that extra reset is an implementation gap, not an adopted legacy rule.
- The hidden `＠やみいちば` action is the only way into the black market ([access](black-market.md#access)).
