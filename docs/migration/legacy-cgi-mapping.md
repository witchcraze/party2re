# Complete Legacy CGI to Go Architecture Mapping Catalog

This catalog is the Single Source of Truth (SSOT) mapping every single Perl CGI script from the legacy Party2 codebase (`/home/witchcraze/dev/party2/party2/`) to its corresponding Go module, HTTP endpoint, database table, and design document.

---

## 1. Critical Misnomer & Pitfall Warnings (取り違え厳禁)

| Misnomer Trap | Authentic Legacy Specification | Go Implementation Guideline |
| :--- | :--- | :--- |
| **`farm.cgi` (Monster Ranch)** | **モンスター牧場 (@モンジィ)**: 仲間モンスター保管（30/32匹）、自宅ペット連携（8枠）、改名（8文字）、P2P譲渡、野生への解放。**作物・畑の要素は一切存在しない**。 | Dedicated to `internal/monsterranch` (or clean `internal/farm`). Crop logic must be purged. |
| **`plantation.cgi` (Seed Cultivation)** | **種菜園 (@ロータス)**: 6種の種（赤/青/黄/緑/銀/金）、14種の特殊肥料、枯れ率計算、翌朝タイマー、預かり所（depot）への収穫物直送。 | Dedicated to `internal/plantation` (Issue #489). |
| **`reborn.cgi` / `altar.cgi` (No Rebirth)** | **転生の祭壇**: レベル1リセット（転生）は存在しない。**Lv99→150の限界突破（OverLevel）**および裏天界解放のみ。 | Purge fictional Rebirth system; restore OverLevel cap (Issue #470, #471). |
| **`guild.cgi` (No Donation Leveling)** | **ギルド拠点**: ゴールド寄付によるギルドLv1〜10上げは存在しない。**活動による動的GP**、自由役職命名（6文字）、申請承認制、HEXカラー。 | Purge fictional donation levels; restore activity GP & custom roles (Issue #490). |
| **`sleep.cgi` / `home.cgi` (No Paid Inn)** | **自宅での睡眠**: 有料の宿屋は存在しない。**自宅（または他人の家）で寝る**ことで無料全快・日次フラグリセット。 | Decommission `internal/inn`; integrate into `internal/home` (Issue #459). |
| **`free.cgi` (Flea Market)** | **フリーマーケット**: 旧ファイル名は `fleamarket.cgi` ではなく `lib/free.cgi`。預かり所から直接出品・引出。 | Implemented in `internal/fleamarket` (Issue #477). |
| **`secret.cgi` (Secret Shop)** | **秘密の店**: 旧ファイル名は `secretshop.cgi` ではなく `lib/secret.cgi`。合言葉入力と時間帯別出現。 | Implemented in `internal/secretshop` (Issue #462). |

---

## 2. Complete File-by-File Catalog (全100%ファイル対照)

### A. Root Meta & System Scripts (`party2/*.cgi`)

| Legacy Script | Facility / Purpose | Actions / Features | Go Package / Endpoint | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `index.cgi`, `_side_menu.cgi` | Top Portal & Navigation | Main menu, announcements, dynamic stats | Client Presentation / Static HTML | Planned |
| `login.cgi` | User Authentication | Session cookie generation, IP access control | `internal/player` (`POST /player/login`) | Compliant |
| `new_entry.cgi` | Character Registration | Initial race/gender/name creation | `internal/player` (`POST /player/register`) | Compliant |
| `my.cgi`, `my2.cgi` | Character Dashboard | Overview of stats, equipment, inventory | `internal/character` (`GET /characters/{id}`) | Compliant |
| `status.cgi` | Detailed Status | In-depth parameters, masteries, titles | `internal/character` (`GET /characters/{id}`) | Compliant |
| `profile.cgi` | Public Profile | Self-introduction, favorite food, battle image | `internal/character` (`GET/PATCH /characters/{id}/profile`) | Compliant |
| `delete.cgi` | Character/Account Deletion | Permanent account & subresource cleanup | `internal/player` (`DELETE /player/account`) | Compliant |
| `rescue.cgi` | Stuck Character Recovery | Resets character location to Town on error | `internal/api/http/rescue.go` | Compliant |
| `admin.cgi`, `maintenance.cgi` | Administration & Maintenance | Maintenance toggle, ban, broadcast | `internal/api/http/admin.go` | Compliant |
| `news.cgi` | Town Newspaper / News Log | Hall of fame, contest winners, events | `internal/news` (`GET /news`) | Compliant |
| `ranking.cgi`, `week_ranking.cgi`, `job_ranking.cgi`, `legend.cgi` | Server Rankings | Top levels, weekly stats, job distribution | `internal/ranking` (`GET /rankings/*`) | Compliant |
| `contest.cgi` | Photo Contest Portal | 10-day cycles, voting, past rounds, legends | `internal/contest` (`GET /contest/*`) | Compliant |
| `screen_shot.cgi` | Screenshot Viewer | Viewing saved battle/event screenshots | `internal/contest` (`GET /characters/{id}/photos`) | Compliant |
| `guild_list.cgi` | Guild Influence Ranks | Guild point rankings, member rosters | `internal/guild` (`GET /guilds`) | Reconciling (#490) |
| `challenge.cgi` | Continuous Challenge Ranks | Top endurance records and milestones | `internal/challenge` (`GET /challenge/records`) | Compliant |
| `party.cgi` | Multiplayer Party Loop | Co-op lobby, ready check, expedition | `internal/party` (`/parties/*`) | Compliant |
| `store.cgi` | Player Store Directory | Browsing player bazaars in Town | `internal/store` (`GET /stores`) | Reconciling (#466) |
| `view_monster.cgi` | Monster Compendium Viewer | Monster stats, drops, defeat counts | `internal/collection` (`GET /collection/monsters`) | Compliant |
| `replay.cgi` | Battle Replay Viewer | Structured turn-by-turn replay log playback | `internal/replay` (`GET /replays/{id}`) | Compliant |
| `link.cgi` | External Links | Community BBS and manual links | Client Presentation / Static HTML | N/A |

---

### B. Facility Scripts (`party2/lib/*.cgi`)

#### 1. Living, Housing & Towns
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/home.cgi` | 自宅 (Home) | `つかう`, `からー`, `ことばをおしえる`, `ねる` | `internal/home` | Reconciling (#459, #461) |
| `lib/sleep.cgi` | 睡眠 (Resting) | Free recovery, fatigue reset, daily flags reset | `internal/home` | Reconciling (#459) |
| `lib/park.cgi` | 公園 (Park) / @メリーヌ | `やすむ` (10G HP/MP partial heal, chat log) | `internal/park` | Compliant |
| `lib/depot.cgi` | 預かり所 (Depot) / @モリー | `あずける`, `ひきだす`, `すべてあずける` | `internal/depot` | Reconciling (#460) |
| `lib/town1.cgi` .. `town4.cgi`, `_town.cgi` | 町1〜町4 (Towns) | `たてる` (500G house), `みせ` (10000G store), `ちぇっく` | `internal/town` | Reconciling (#461, #466) |

#### 2. Commercial Shops & Economy
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/weapon.cgi` | 武器屋 (Weapon Shop) | `かう`, `うる` (Level bounds, 50% sellback) | `internal/shop` | Reconciling (#465) |
| `lib/armor.cgi` | 防具屋 (Armor Shop) | `かう`, `うる` (Level bounds, 50% sellback) | `internal/shop` | Reconciling (#465) |
| `lib/item.cgi`, `goods.cgi` | 道具屋 (Item Shop) | `かう`, `うる` (MasterCard 10% discount) | `internal/shop` | Reconciling (#465) |
| `lib/accessory.cgi` | 装飾品屋 (Accessory Shop) | `かう`, `うる` (Rare stat accessories) | `internal/shop` | Reconciling (#465) |
| `lib/blacksmith.cgi` | 鍛冶屋 / @コスタ | `きたえる` (+1..+10), `こくいん`, `なづける` | `internal/blacksmith` | Reconciling (#458) |
| `lib/secret.cgi` | 秘密の店 / @ゲルダ | `かう` (Passphrase required, timed rotation) | `internal/secretshop` | Reconciling (#462) |
| `lib/black_market.cgi` | 闇市 / @ヤンガス | `かう` (Entrance fee, non-standard items, bust) | `internal/blackmarket` | Reconciling (#463) |
| `lib/gem_store.cgi` | 宝石店 / @ルル | `かう` (Gem purchases & skill slot infusion) | `internal/gemstore` | Reconciling (#464) |
| `lib/store.cgi` | プレイヤー店舗 | `うる`, `かう`, `かんばん`, `きちょう` | `internal/store` | Reconciling (#466) |
| `lib/bar.cgi` | ルイーダの酒場 / @ルイーダ | `ちゅうもん` (Meal buff, +2GP), `でりばりー` | `internal/store` (`tavern`) | Reconciling (#475) |
| `lib/bank.cgi` | 銀行 / @ゴールド銀行 | `あずける`, `ひきだす`, `そうきん` (Daily interest) | `internal/bank` | Reconciling (#476) |
| `lib/auction.cgi` | オークション | `しゅっぴん`, `にゅうさつ`, `らつさつ` (Depot delivery) | `internal/auction` | Reconciling (#474) |
| `lib/free.cgi` | フリーマーケット | `しゅっぴん`, `かう` (Direct Depot withdrawal) | `internal/fleamarket` | Reconciling (#477) |

#### 3. Character Growth, Faith & Limits
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/job_change.cgi` | 転職所 / @ダーマ神官 | `てんしょく` (Level 30+ requirement, stat scaling) | `internal/job` | Reconciling (#467) |
| `lib/job_master.cgi` | 職業極め所 | `きわめる` (Mastery bonuses, stat passives) | `internal/job` | Reconciling (#467) |
| `lib/sp_change.cgi` | 願いの泉 / @女神 | `たいりょく`, `まりょく`, `こうげき`, `ぼうぎょ`, `すばやさ` (SP stat growth) | `internal/wishingwell` | Compliant (#468) |
| `lib/custom_skill.cgi` | 特技設定 / @マニャ | `つくる` (Incantations, custom cost/rates) | `internal/custom_skill` | Reconciling (#469) |
| `lib/name_change.cgi` | 命名の館 / @アストロン | `なまえをかえる` (Gold fee, unique name validation) | `internal/character` | Compliant |
| `lib/custom_image.cgi`, `upload_image.cgi` | 画像設定所 | `がぞうをかえる` (Custom avatar URL/icon) | `internal/character` | Compliant |
| `lib/altar.cgi`, `reborn.cgi` | 転生の祭壇 / @精霊ルビス | `げんかいとっぱ` (OverLevel 99→150 limit break) | `internal/core/progression` | Reconciling (#470, #471) |
| `lib/chapel.cgi` | 教会 / @神父 | `いのる`, `きふ`, `どくのちりょう`, `のろいをとく` | `internal/chapel` | Reconciling (#472) |
| `lib/god.cgi` | 天界 / @神 | `おいのり` (Stat seeds grant, celestial blessings) | `internal/god` | Reconciling (#473) |
| `lib/u_god.cgi` | 裏天界 / @裏神 | `うらおいのり` (Celestial ranch expansion +2) | `internal/god` | Reconciling (#473) |
| `lib/medal.cgi` | メダル王 / @メダル王 | `こうかん` (Small medal prize exchanges to Depot) | `internal/medal` | Reconciling (#473) |
| `lib/exile.cgi` | 荒らし追放騎士団 / @追放騎士 | `つほうしんせい`, `とうひょう` (Community moderation) | `internal/moderation` (future) | Backlog |

#### 4. Production, Cultivation & Monsters
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/alchemy.cgi`, `_alchemy_recipe.cgi` | 錬金堂 / @トロデ | `れんきん` (Free, Depot ingredients, overnight, Depot product) | `internal/alchemy` | Reconciling (#487) |
| `lib/farm.cgi` | モンスター牧場 / @モンジィ | `つれてく`, `あずける`, `なづける`, `おくる`, `わかれる` | `internal/monsterranch` | Reconciling (#488) |
| `lib/plantation.cgi` | 種菜園 / @ロータス | `たねをまく`, `しゅうかく` (6 seeds, 14 fertilizers, Depot delivery) | `internal/plantation` | Reconciling (#489) |

#### 5. Combat, Dungeons & Raids
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/vs_monster.cgi`, `lib/adventure.cgi` | 冒険 (Adventure) | 10-floor dungeon crawl, flee penalty, floor boss | `internal/adventure` | Reconciling (#478) |
| `lib/vs_king.cgi`, `_win_vs_king.cgi` | 封印の魔王 (Boss) | Proof of Kingship, single-run revives, Leaf of World Tree | `internal/boss` | Reconciling (#479) |
| `lib/_battle.cgi` | コア戦闘エンジン | Turn calculations, damage, hit rate, critical, status | `internal/core/battle` | Reconciled (#480) |
| `lib/_skill.cgi` | スキル発動エンジン | Skill triggers, MP/HP costs, elemental damage | `internal/core/skill` | Reconciling (#469) |
| `lib/vs_player.cgi` | 闘技場 (PvP Arena) | Real-time live betting, spectator logs, Elo rating | `internal/pvp` | Reconciling (#481) |
| `lib/vs_guild.cgi` | ギルド戦 (GvG Arena) | Multi-round tournament brackets, color defense | `internal/gvg` | Reconciling (#482) |
| `lib/vs_dungeon.cgi`, `dungeon.cgi` | ダンジョン探索 | Co-op multiplayer party crawling, trap chests | `internal/dungeon` | Reconciling (#483) |
| `lib/vs_challenge.cgi` | 連戦チャレンジ | Multi-round survival ladder, interim rewards | `internal/challenge` | Reconciling (#483) |
| `lib/quest.cgi` | パーティ共闘クエスト | Multi-player synchronized battles, synergy bonuses | `internal/party` | Compliant |
| `lib/helper.cgi` | 手助けクエスト | Player-to-player item delivery request board | `internal/adventure` (`helper`) | Backlog |

#### 6. Social, Gambling & Minigames
| Legacy Script | Facility / NPC | Key Actions (`@actions`) | Go Module | Migration Status |
| :--- | :--- | :--- | :--- | :---: |
| `lib/guild.cgi`, `join_guild.cgi` | ギルド (Guild) | `さんか`, `つくる`, `まーく`, `かべがみ`, `よびかける`, `からー`, `あたえる` | `internal/guild` | Reconciling (#490) |
| `lib/photo.cgi` | 写真館 / @ワコール | `みる`, `けす`, `とうひょう`, `えんとりー` | `internal/contest` | Compliant |
| `lib/event.cgi` | イベント広場 / @旅の商人 | `かう` (Real-time concurrency trigger, 3x markup) | `internal/eventplaza` | Reconciling (#491) |
| `lib/takarakuzi.cgi` | 宝くじ / @案内人 | `かう` (20-cap ticket limit, rollover jackpot) | `internal/lottery` | Reconciling (#484) |
| `lib/lot.cgi` | 福引 / @案内人 | `まわす` (Stat seeds, divine orbs, guaranteed tiers) | `internal/lottery` | Reconciling (#485) |
| `lib/casino.cgi`, `_casino.cgi` | カジノラウンジ | 8-player shared room, coin exchange | `internal/casino` | Reconciling (#486) |
| `lib/casino_slot.cgi` | スロットマシン | 3-reel, progressive jackpot | `internal/casino` | Reconciling (#486) |
| `lib/casino_highlow.cgi` | ハイローゲーム | High/Low card guessing, streak multipliers | `internal/casino` | Compliant |
| `lib/casino_indian.cgi` | インディアンポーカー | Bluffing, call/fold against NPC | `internal/casino` | Compliant |
| `lib/casino_doppel.cgi` | ドッペルゲンガー | 8-symbol matching game | `internal/casino` | Compliant |
| `lib/collection.cgi`, `_add_*.cgi` | 図鑑コレクション | Monster and item compendium completion tracking | `internal/collection` | Compliant |

#### 7. Core Shared System Subroutines
| Legacy Script | Purpose / Contents | Corresponding Go Layer |
| :--- | :--- | :--- |
| `lib/_data.cgi` | Master definitions: weapons, armor, items, monsters, jobs, dialogue tables | `internal/core/{item,job,skill}`, JSON catalogs |
| `lib/system.cgi` | Global mechanics: user file I/O, member presence, HTML renderers, mail dispatch | `internal/database`, `internal/api/http` |
| `lib/_npc_action.cgi`| Shared NPC dialogue and reaction engine | `internal/api/http` handlers |

---

### C. Data & Stage Scripts (`party2/stage/*.cgi`, `challenge/*.cgi`)

- `party2/stage/0.cgi` .. `27.cgi` (28 adventure stages) $\to$ `internal/adventure/data/stages.json`
- `party2/stage/king1.cgi` .. `king99.cgi` (10 King boss stages) $\to$ `internal/boss/data/kings.json`
- `party2/challenge/0.cgi` .. `8.cgi` (9 challenge stages) $\to$ `internal/challenge/data/challenge_tiers.json`
