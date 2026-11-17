# Character Customization, Naming Hall & Profile Design (命名の館・キャラクターカスタマイズ)

## 1. Overview

The Character Customization & Naming Hall system (`internal/character`, legacy `name_change.cgi`, `profile.cgi`, `custom_image.cgi`, NPC `@マリナン`) allows players to change their character's name and gender, edit the legacy profile and set the image (aura and avatar).

---

## 2. Facilities & NPC

- **Facility Title**: 命名の館 (Naming Hall)
- **NPC Name**: `@マリナン`
- **Location Context**: Legacy `lib/name_change.cgi`, `lib/profile.cgi`, `lib/custom_image.cgi`

---

## 3. Name Change Rules (`ChangeName`)

### 3.1 Cost & Preconditions
- **Fee**: 500,000 Gold (`NameChangeCost = 500000`).
- **Guild Constraint**: The character must not be a member of any guild (`IsInGuild == false`). Characters must leave their guild prior to changing their name.
- **Exile Vote Constraint**: The character must not be an applicant, a target or a voter on an open [exile petition](exile.md#rules) (`name_change.cgi:112`). This check runs after the guild check.
- **Flea Market Constraint**: The character must not have active listings in the flea market (`HasActiveListings == false`).

### 3.2 Name Format & Validation Rules
1. **Length**: $1 \le \text{Length} \le 8$ (UTF-8 rune count; legacy `new_entry.cgi:114`, `name_change.cgi:125`).
2. **Prohibited Characters**:
   - Punctuation & symbols: `,`, `;`, `"`, `'`, `&`, `<`, `>`, `\`, `/`, `@`, `＠` (legacy `new_entry.cgi:111–112`, `name_change.cgi:122–123`).
   - Whitespace: ASCII spaces, tabs, newline characters, and Japanese fullwidth space `\u3000` anywhere in the name (surrounding or internal) are strictly rejected without trimming (legacy `new_entry.cgi:113`, `name_change.cgi:124`).
   - Control & Invisible characters: Any Unicode control characters, zero-width characters, bidirectional overrides, and excessive combining diacritical marks (Zalgo text > 2 combining marks).
3. **Normalization**: Unicode NFC normalized.
4. **Uniqueness**: The new name must not be identical to the character's current name, and must not already be taken by any other active character in the system.

### 3.3 Side Effects
- Deducts 500,000 Gold within the Unit of Work transaction.
- Broadcasts a public system news announcement: `"<oldName>が <newName> と名前を変更"`.

---

## 4. Gender & Appearance Change Rules (`ChangeGender`)

- **Fee**: 10,000 Gold (`GenderChangeCost = 10000`).
- **Allowed Values**: `m` (male), `f` (female).
- **Preconditions**:
  1. **Gender Difference**: Target gender must differ from current character gender (`char.Gender != newGender`, otherwise `ErrSameGender`).
  2. **Funds**: Character must possess at least 10,000 Gold (`char.Money >= GenderChangeCost`, otherwise `ErrInsufficientGold`).
  3. **Current Job Compatibility**: The character's current job must allow the target gender. If the job definition specifies a `required_gender` (e.g. `job-13` 吟遊詩人 / `job-15` 黒魔術師 requiring `m`, `job-14` 踊り子 / `job-16` 白魔術師 requiring `f`), changing to an incompatible gender is rejected with `ErrGenderIncompatibleWithJob`.
- **Rejection Guarantee**: On rejection (incompatible job, same gender, insufficient funds, or invalid gender), no fee is deducted; character gold and gender remain intact.
- **Legacy Parity & Translation**: Translates legacy `name_change.cgi` sprite-existence check (`$icondir/job/$job_$gender.gif`) into the Go job catalog's `RequiredGender` constraint, avoiding asset copying or runtime filesystem checks.
- **Concurrency & Locking**: Validation is performed on the row-locked character record (`findForUpdate`) within the Unit of Work database transaction (`runInTx`), ensuring deterministic serialization with concurrent job changes (`ChangeJob`).

---

## 5. Profile & Image Rules

### 5.1 Profile Attributes (`character_profiles`)
- **Profile fields**: the legacy profile form's fields (`lib/profile.cgi:6–30`), edited only by the owner in the [Home profile scene](home.md#observation-visibility-and-navigation); an edit replaces every field because the legacy form submits them all.
- **Avatar URL**: Empty (no avatar) or exactly `https://i.imgur.com/<ID>.gif`, where `<ID>` is 5–10 ASCII letters/digits. Legacy `lib/custom_image.cgi:65–70` accepts only that imgur ID and stores this URL; there is no image upload, data URI or other host. Other external services are deferred to [#1923](https://github.com/witchcraze/party2re/issues/1923). Known gap: the God new-world wish still writes `chr/052.gif` here although legacy changes the icon, not the avatar ([#1924](https://github.com/witchcraze/party2re/issues/1924)).
- **Aura Effect (`aura_effect`)**: legacy `effect1` (`custom_image.cgi`, `@effects` in `_data.cgi`), `0` (none) to `8`.
- Legacy sets aura and, after the avatar unlock, the avatar in own-Home `いめーじ`; there is no setter yet ([#2332](https://github.com/witchcraze/party2re/issues/2332)).
- **Comment**: no legacy counterpart; its removal is [#2333](https://github.com/witchcraze/party2re/issues/2333).

---

Transport schemas: [OpenAPI](../api/openapi.json). Storage key lifetimes: [Valkey keyspace](../architecture/valkey-keyspace.md).

## 7. Concurrency & Security Guarantees

- **Atomicity & Locking**: Name changes, gender modifications, and profile updates lock the target character row with `SELECT ... FOR UPDATE` within a database transaction (`RunInTx`).
- **IDOR Protection**: All mutating `/characters/{id}/*` endpoints enforce session token authentication and verify that the character belongs to the authenticated player (`withAuthenticatedCharacter`).
- **Unique Name Integrity**: Character name uniqueness is validated and supported by an index on `characters(name)`.
- **Public Profile Boundary**: Anonymous profile reads expose the character name and player-written profile only; account identifiers, balances and internal character state remain private. Legacy `lib/profile.cgi:58–78` uses the name as its heading and reads the bio entries separately; its editable `job` and `sex` entries are not the character's actual job or gender. [OpenAPI](../api/openapi.json) owns the response fields.
