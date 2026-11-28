# Oracle Shop Design

The Oracle shop (オラクル屋, `@ラクル`) reconstructs legacy `lib/goods.cgi`: it rents costumes and sells home wallpapers. The catalog, job-level gating and wallpaper prices live in [`internal/store/oracle.go`](../../internal/store/oracle.go) and [`models.go`](../../internal/store/models.go).

- Costumes requested by an active helper quest are excluded; read failures fail catalog reads and purchases closed.
- Home item use applies a costume ([Home dispatch contract](home.md#legacy-dispatch-and-consumption-contract)); sleep, cross-job change, and job/future-memory recall or restore reset to the ordinary job appearance.
- The hidden `＠やみいちば` action is the only entry into the black market ([access](black-market.md#access)).
