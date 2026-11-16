package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	jobapp "github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// JobService is the job temple surface plus the Home job mastery record.
type JobService interface {
	ListDefinitions() []corejob.Definition
	TempleStatus(ctx context.Context, characterID string) (jobapp.TempleStatus, error)
	ChangeJob(ctx context.Context, characterID string, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	ExchangeJob(ctx context.Context, characterID string, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	RestoreJobMemory(ctx context.Context, characterID string) (corecharacter.Character, corejob.CharacterJob, error)
	RecallFutureMemory(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error)
	GetJobMastery(ctx context.Context, characterID string) (jobapp.CharacterJobMastery, error)
}

type jobChangeRequest struct {
	JobID string `json:"job_id"`
}

type jobRecallRequest struct {
	JobID    string `json:"job_id"`
	OldJobID string `json:"old_job_id"`
}

type jobFutureRecallRequest struct {
	MemoryID string `json:"memory_id"`
}

// JobOption is one job the temple offers.
type JobOption struct {
	JobID string `json:"job_id"`
	Name  string `json:"name"`
}

// JobFutureMemory is one ＠よびおこす snapshot as the legacy table lists it.
type JobFutureMemory struct {
	ID         string `json:"id"`
	JobName    string `json:"job_name"`
	OldJobName string `json:"old_job_name"`
	Level      int    `json:"level"`
	MaxHP      int    `json:"max_hp"`
	MaxMP      int    `json:"max_mp"`
	Attack     int    `json:"attack"`
	Defense    int    `json:"defense"`
	Agility    int    `json:"agility"`
}

// JobTempleSceneData lists the choices each job temple command accepts now.
type JobTempleSceneData struct {
	Parent         string                      `json:"parent"`
	Jobs           []JobOption                 `json:"jobs"`
	RecallJobs     []JobOption                 `json:"recall_jobs"`
	RecallItem     string                      `json:"recall_item,omitempty"`
	JobMemory      corecharacter.JobMemoryKind `json:"job_memory,omitempty"`
	FutureMemories []JobFutureMemory           `json:"future_memories"`
}

// JobTempleResult is the character after a temple command and the priest's reply.
type JobTempleResult struct {
	Character characterResponse    `json:"character"`
	Job       corejob.CharacterJob `json:"job"`
	Message   string               `json:"message"`
}

// WithJob configures the job temple scene, its commands and the Home job mastery record.
func WithJob(jobs JobService) Option {
	return func(h *Handler) {
		h.jobs = jobs
		if jobs == nil {
			withActionCommand[jobChangeRequest]("job_change", nil, nil)(h)
			withActionCommand[jobRecallRequest]("job_recall", nil, nil)(h)
			withActionCommand[struct{}]("job_restore", nil, nil)(h)
			withActionCommand[jobFutureRecallRequest]("job_future_recall", nil, nil)(h)
			return
		}
		// job_change.cgi $npc_com replies; formats index their arguments so
		// one that omits the job name reports no extra argument.
		reply := func(format string) func(corecharacter.Character, corejob.CharacterJob, error) (any, error) {
			return func(char corecharacter.Character, job corejob.CharacterJob, err error) (any, error) {
				if err != nil {
					return nil, err
				}
				return JobTempleResult{Character: toCharacterResponse(char), Job: job, Message: fmt.Sprintf(format, char.Name, h.jobName(char.JobID))}, nil
			}
		}
		// job_change reads the pre-change jobs to name the consumed item or armor.
		def, _ := actionDefinition("job_change")
		if h.actionCommands == nil {
			h.actionCommands = make(map[string]actionCommand)
		}
		h.actionCommands["job_change"] = actionCommand{reject: jobActionRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
			var p jobChangeRequest
			if err := decodeActionParams(raw, def.RequiredParams, &p); err != nil {
				return nil, err
			}
			return func(ctx context.Context, _ string, actor string, current playercontext.Result) (any, error) {
				before := current.Snapshot.Character
				used := ""
				if corejob.IsItemConsumed(p.JobID, before.JobID, before.OldJobID) {
					used = itemName(corejob.Definition{ID: p.JobID}.RequiredItem()) + "を使いました！ "
				} else if corejob.IsArmorConsumed(p.JobID, before.JobID, before.OldJobID) {
					used = itemName("armor-29") + "を使いました！ "
				}
				result, err := reply("%[1]sよ！%[2]sとなり新たな道を歩むが良い")(jobs.ChangeJob(ctx, actor, p.JobID))
				if err != nil {
					return nil, err
				}
				r := result.(JobTempleResult)
				r.Message = used + r.Message
				return r, nil
			}, nil
		}}
		withActionCommand("job_recall", func(ctx context.Context, actor string, p jobRecallRequest) (any, error) {
			return reply("%[1]sの記憶を呼び起こしたぞ！")(jobs.ExchangeJob(ctx, actor, p.JobID, p.OldJobID))
		}, jobActionRejection)(h)
		withActionCommand("job_restore", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			return reply("%[1]sを元の職業に戻したぞ！")(jobs.RestoreJobMemory(ctx, actor))
		}, jobActionRejection)(h)
		withActionCommand("job_future_recall", func(ctx context.Context, actor string, p jobFutureRecallRequest) (any, error) {
			return reply("%[1]sの記憶を呼び起こしたぞ")(jobs.RecallFutureMemory(ctx, actor, p.MemoryID))
		}, jobActionRejection)(h)
	}
}

// jobName is the display name of jobID, or "" without a job service.
func (h *Handler) jobName(jobID string) string {
	if h.jobs != nil {
		for _, job := range h.jobs.ListDefinitions() {
			if job.ID == jobID {
				return job.Name
			}
		}
	}
	return ""
}

func (h *Handler) jobTempleSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.jobs == nil {
		return nil, errSceneNotConfigured
	}
	status, err := h.jobs.TempleStatus(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	data := JobTempleSceneData{Parent: "town", Jobs: jobOptions(status.Changeable), RecallJobs: jobOptions(status.Recallable),
		RecallItem: status.RecallItem, FutureMemories: make([]JobFutureMemory, 0, len(status.FutureMemories))}
	if memory := r.Snapshot.Character.JobMemory; memory != nil {
		data.JobMemory = corecharacter.JobMemoryKindPersistent
		if memory.IsTemporary() {
			data.JobMemory = corecharacter.JobMemoryKindTemporary
		}
	}
	for _, m := range status.FutureMemories {
		data.FutureMemories = append(data.FutureMemories, JobFutureMemory{ID: m.ID, JobName: h.jobName(m.JobID), OldJobName: h.jobName(m.OldJobID),
			Level: m.Level, MaxHP: m.MaxHP, MaxMP: m.MaxMP, Attack: m.Attack, Defense: m.Defense, Agility: m.Agility})
	}
	return data, nil
}

func jobOptions(defs []corejob.Definition) []JobOption {
	options := make([]JobOption, 0, len(defs))
	for _, def := range defs {
		options = append(options, JobOption{JobID: def.ID, Name: def.Name})
	}
	return options
}

func jobActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, corejob.ErrDefinitionNotFound), errors.Is(err, corejob.ErrJobUnavailable):
		return http.StatusConflict, ErrorDetail{Code: "JOB_UNAVAILABLE", Message: "Choose one of the observed job temple choices."}
	case errors.Is(err, jobapp.ErrFutureMemoryNotFound):
		return http.StatusConflict, ErrorDetail{Code: "JOB_FUTURE_MEMORY_NOT_FOUND", Message: "Choose one of the observed future memories."}
	case errors.Is(err, jobapp.ErrRequiredItem):
		return http.StatusConflict, ErrorDetail{Code: "JOB_REQUIRED_ITEM", Message: "The required item is not held."}
	case errors.Is(err, jobapp.ErrRequiredArmor):
		return http.StatusConflict, ErrorDetail{Code: "JOB_REQUIRED_ARMOR", Message: "The required armor is not equipped."}
	default:
		return 0, ErrorDetail{}
	}
}

// The legacy temple and its name are franchise-specific; both use generic
// names (rule 00 §3). The priest's lines are job_change.cgi @words.
var jobTempleFacility = facility{
	title: "転職の神殿", commands: []string{"job_change", "job_recall", "job_restore", "job_future_recall"},
	read: (*Handler).jobTempleSceneData, placeLog: true,
	present: func(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
		// job_change.cgi:118-127 offers only the memory commands that apply.
		d := data.(JobTempleSceneData)
		var hide []string
		if d.JobMemory != "" || d.RecallItem == "" || len(d.RecallJobs) < 2 {
			hide = append(hide, "job_recall")
		}
		if d.JobMemory != corecharacter.JobMemoryKindPersistent {
			hide = append(hide, "job_restore")
		}
		if d.JobMemory != "" || len(d.FutureMemories) == 0 {
			hide = append(hide, "job_future_recall")
		}
		return scenePresentation{hide: hide}
	},
	npc: &npcProfile{name: "@神官", wordsOf: priestWords,
		inspect: "モンスター撃退数 {monster_kills}回\nプレイヤー撃退数 {pvp_wins}回\n封印解いた回数 {mao}回\n封印した回数 {hero}回\nカジノ勝利数 {casino_wins}回"},
}

func priestWords(h *Handler, _ context.Context, char corecharacter.Character) ([]string, error) {
	var names []string
	if h.jobs != nil {
		for _, def := range h.jobs.ListDefinitions() {
			if def.ID != "starter" {
				names = append(names, def.Name)
			}
		}
	}
	someJob := func() string {
		if len(names) == 0 {
			return h.jobName(char.JobID)
		}
		return names[random.Intn(len(names))]
	}
	n := char.Name
	return []string{
		"よくきた！この神殿ではお主の職業を変えることができるぞ",
		"ふむふむ。どの職業にしようかまよっているのじゃな",
		"転職をすると今のステータスが半分になってしまうぞ",
		"職業は重要じゃからよーく考えるのじゃよ",
		"転職アイテムを持っていれば、特別な職業に転職することができるぞ",
		n + "は男前だから剣士なんかどうじゃろ？",
		n + "はお金が欲しいと思っているな？それなら商人になりなさい",
		n + "はモンスターと仲良くなりたいと思っているな？それなら魔物使いになりなさい",
		n + "は癒し系になりたいと思っているな？それなら僧侶になりなさい",
		n + "は相手やお宝が気になっているな？それなら盗賊になりなさい",
		n + "は誰かにイタズラしたいと思っているな？それなら遊び人になりなさい",
		n + "はモコモコしたものが好きじゃな？それなら羊飼いになりなさい",
		n + "は最終的に" + someJob() + "を目指すと良いじゃろぉ",
		n + "に一番しっくりくるのは" + someJob() + "じゃな",
		"転職条件が厳しいからといって強いとは限らんぞ",
		"どんなにスキルを覚えても使いこなせなきゃ意味がないぞ",
		"スキルを早く覚えたい場合は早期転職をオススメしておる",
		"ステータスを上げたい場合は、成長率の高い職業を選び、なるべく遅く転職するのがコツじゃ",
		"今の職業のスキルを全てマスターしてから転職しても、おそくはないはずじゃ",
		fmt.Sprintf("%sの今の転職回数は…%d回。ふむ、なかなかじゃのぉ", n, char.JobLevel),
		"転職回数は冒険者の熟練度でもある。３回転職をすると初心者卒業レベルかのぉ",
		"転職回数は冒険者の熟練度でもある。10回以上の転職者は、この世界を熟知しているベテランじゃのぉ",
	}, nil
}
