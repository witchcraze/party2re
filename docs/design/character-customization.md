# Character Customization, Naming Hall & Profile Design (命名の館・キャラクターカスタマイズ)

## 1. Overview

The Character Customization & Naming Hall system (`internal/character`, legacy `name_change.cgi`, `profile.cgi`, `custom_image.cgi`, NPC `@マリナン`) allows players to change their character's name, update gender and appearance, edit a custom profile bio/comment, and upload or set an avatar image.

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
- **Flea Market Constraint**: The character must not have active listings in the flea market (`HasActiveListings == false`).

### 3.2 Name Format & Validation Rules
1. **Length**: $1 \le \text{Length} \le 32$ (UTF-8 rune count).
2. **Prohibited Characters**:
   - Punctuation & symbols: `,`, `;`, `"`, `'`, `&`, `<`, `>`, `\`, `/`, `@`, `＠`.
   - Whitespace: ASCII spaces, tabs, newline characters, and Japanese fullwidth space `\u3000`.
   - Control characters: Any Unicode control characters.
3. **Uniqueness**: The new name must not be identical to the character's current name, and must not already be taken by any other active character in the system.

### 3.3 Side Effects
- Deducts 500,000 Gold within the Unit of Work transaction.
- Broadcasts a public system news announcement: `"<oldName>が <newName> と名前を変更"`.

---

## 4. Gender & Appearance Change Rules (`ChangeGender`)

- **Fee**: 10,000 Gold (`GenderChangeCost = 10000`).
- **Allowed Values**: `m` (male), `f` (female), `unspecified` (other).
- **Rule**: Target gender must differ from current character gender, and character must have at least 10,000 Gold.

---

## 5. Profile Bio & Avatar Rules

### 5.1 Profile Attributes (`character_profiles`)
- **Self-Introduction Comment**: Up to 160 characters (`MaxCommentLength = 160`).
- **Avatar URL**: Up to 512 characters. May be a valid `http://` or `https://` URL, a relative path `/...`, or an inline data URI (`data:image/...;base64,...`).
- **Bio Data**: Optional structured key-value map (e.g. `hobby`, `dream`, `like_food`, `dislike_food`, `blood_type`, `birthday`) with keys $\le 32$ characters and values $\le 160$ characters.

### 5.2 Avatar Image Upload
- **Maximum File Size**: 2 MB (`MaxAvatarSizeBytes = 2097152`).
- **Supported Formats**: PNG, JPEG, GIF, WebP, SVG (`image/png`, `image/jpeg`, `image/gif`, `image/webp`, `image/svg+xml`).
- Stored as a data URI or avatar URL in the character's profile.

---

## 6. HTTP API Specification

| Action | HTTP Endpoint | Auth | Description |
| :--- | :--- | :--- | :--- |
| **Naming Hall Dialogue** | `GET /naming-hall/dialogue` | Public | Returns `@マリナン` dialogue and pricing. |
| **Change Name** | `POST /characters/{id}/name` | Session + Character | Renames character (500,000 G, validates guild & flea market). |
| **Change Gender** | `POST /characters/{id}/gender` | Session + Character | Changes gender/appearance (10,000 G). |
| **Get Profile** | `GET /characters/{id}/profile` | Public | Returns character stats and profile data. |
| **Update Profile** | `POST /characters/{id}/profile`, `PUT /characters/{id}/profile` | Session + Character | Updates comment, avatar URL, or bio data. |
| **Upload Avatar** | `POST /characters/{id}/avatar` | Session + Character | Uploads avatar file (multipart form or JSON). |

---

## 7. Concurrency & Security Guarantees

- **Atomicity & Locking**: Name changes and gender modifications lock the target character row with `SELECT ... FOR UPDATE` within a database transaction (`RunInTx`).
- **IDOR Protection**: All mutating `/characters/{id}/*` endpoints enforce session token authentication and verify that the character belongs to the authenticated player (`withAuthenticatedCharacter`).
- **Unique Name Integrity**: Character name uniqueness is validated and supported by an index on `characters(name)`.
