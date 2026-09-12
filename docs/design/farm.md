# Monster Ranch Design (旧 farm.cgi / モンスター牧場)

> [!NOTE]
> **Fictional Crop Farm Purged (Issue #488)**:
> The fictional 4-plot crop cultivation system previously located in `internal/farm` has been completely purged from the codebase.
> In original Party2, `farm.cgi` is the **Monster Ranch (モンスターじいさん @モンジィ)**.
>
> - **Monster Ranch & Home Pet Companion Design**: See [`monster.md`](monster.md) (`internal/monster`, legacy `farm.cgi`).
> - **Plantation Seed Cultivation Design**: See `docs/design/plantation.md` (`internal/plantation`, legacy `plantation.cgi` @ロータス, Issue #489).

---

## 1. Role in Legacy Party2 (`farm.cgi`)

`farm.cgi` serves as the monster stabling and ranch facility overseen by NPC `@モンジィ` (モンスターじいさん).
Its responsibilities are:
1. **Monster Stabling**: Storing captured and befriended monsters in the character's ranch box (`character_monsters`).
   - Base capacity: 50 monsters (100 monsters at character Level $\ge$ 100).
   - Expandable up to 300 monsters via underworld celestial wishes (`OverMonster`, +50 per tier up to 5 tiers).
2. **Home Pet Companions**:
   - Transferring monsters to player private home estates as active pets (`つれてく`, `BringToHome`, up to 8 pets).
   - Depositing active pets back to ranch storage (`あずける`, `DepositToBox`).
   - Duplicate name restriction: No two pets in the same home may share identical custom names.
3. **Monster Renaming (`なづける`, `Rename`)**:
   - Custom nicknames up to 8 UTF-8 characters.
   - Rejection of invalid whitespace or special characters (`,`, `;`, `"`, `'`, `&`, `<`, `>`, `@`).
4. **P2P Monster Gifting (`おくる`, `SendMonster`)**:
   - Direct transfer of a monster instance to another player's ranch box with deterministic two-party row locking.
5. **Wild Release (`わかれる`, `ReleaseMonster`)**:
   - Permanently releasing a monster back to the wild.
