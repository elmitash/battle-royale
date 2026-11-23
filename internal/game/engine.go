package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/db"
	"battle-royale/internal/model"
)

// Engine coordinates all game state mutations safely.
type Engine struct {
	DB     *sql.DB
	Master *db.MasterData
	Cfg    config.Config
	mu     sync.Mutex
}

// NewEngine creates a new game Engine.
func NewEngine(sqlDB *sql.DB, master *db.MasterData, cfg config.Config) *Engine {
	return &Engine{
		DB:     sqlDB,
		Master: master,
		Cfg:    cfg,
	}
}

// LoadUser loads a user and their inventory items (slots 0..5).
func (e *Engine) LoadUser(ctx context.Context, userID string) (*model.User, error) {
	return loadUserQ(ctx, e.DB, userID)
}

// GetGameState loads the global round state.
func (e *Engine) GetGameState(ctx context.Context) (*model.GameState, error) {
	return getGameStateQ(ctx, e.DB)
}

// ApplyPassiveRecovery updates stamina and HP based on elapsed time since LastActionAt.
func (e *Engine) ApplyPassiveRecovery(ctx context.Context, userID string) (*model.User, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	u, err := loadUserQ(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	if applyRecovery(u, now) {
		if err := saveUserTx(ctx, tx, u); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}

// SetRestMode switches the user between ALIVE, 치료, and 수면.
func (e *Engine) SetRestMode(ctx context.Context, userID, mode string) (string, error) {
	if mode != "ALIVE" && mode != "치료" && mode != "수면" {
		return "", errors.New("잘못된 상태입니다")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	u, err := loadUserQ(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	if u.Status == "DEAD" {
		return "이미 사망한 상태입니다.", nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)
	u.Status = mode
	u.LastActionAt = now
	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}

	switch mode {
	case "치료":
		return "상처를 치료하며 휴식을 취하기 시작했습니다.", nil
	case "수면":
		return "잠을 자며 스태미나를 회복하기 시작했습니다.", nil
	default:
		return "다시 행동을 시작합니다.", nil
	}
}

// FirstAid performs emergency first aid, spending FirstAidSta stamina to heal HP.
func (e *Engine) FirstAid(ctx context.Context, userID string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	u, err := loadUserQ(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	if u.Status == "DEAD" || u.HP <= 0 {
		return "사망한 상태에서는 행동할 수 없습니다.", nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)

	if u.Stamina < config.FirstAidSta {
		return fmt.Sprintf("응급처치를 하려면 스태미나가 %d 이상 필요합니다.", config.FirstAidSta), nil
	}
	u.Stamina -= config.FirstAidSta
	heal := 30
	u.HP += heal
	if u.HP > u.MaxHP {
		u.HP = u.MaxHP
	}
	u.PoisonFlag = ""
	u.Status = "ALIVE"

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("응급처치를 했다! 체력이 회복되고 상처가 아물었다. (HP %d/%d)", u.HP, u.MaxHP), nil
}

// UpdateTactics updates the user's combat stance.
func (e *Engine) UpdateTactics(ctx context.Context, userID, tactics string) (string, error) {
	allowed := map[string]bool{
		"보통": true, "공격중시": true, "방어중시": true, "은밀행동": true, "탐색중시": true, "연속공격": true,
	}
	if !allowed[tactics] {
		return "올바르지 않은 기본방침입니다.", nil
	}
	_, err := e.DB.ExecContext(ctx, `UPDATE users SET tactics = ? WHERE id = ?`, tactics, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("기본방침을 '%s'(으)로 변경했습니다.", tactics), nil
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadUserQ(ctx context.Context, q querier, userID string) (*model.User, error) {
	var u model.User
	var profJSON string
	err := q.QueryRowContext(ctx,
		`SELECT id, first_name, last_name, sex, class_name, student_no, club, icon,
		        status, tactics, area_id, att, def, hp, max_hp, level, exp, stamina, kills, last_action_at,
		        weapon_name, weapon_kind, weapon_att, weapon_dur,
		        armor_body, armor_body_def, armor_body_dur,
		        armor_head, armor_head_def, armor_head_dur,
		        armor_foot, armor_foot_def, armor_foot_dur,
		        armor_arm, armor_arm_def, armor_arm_dur,
		        proficiency_json, battle_log, death_msg, message, last_target_id, poison_flag
		 FROM users WHERE id = ?`, userID,
	).Scan(
		&u.ID, &u.FirstName, &u.LastName, &u.Sex, &u.ClassName, &u.StudentNo, &u.Club, &u.Icon,
		&u.Status, &u.Tactics, &u.AreaID, &u.Att, &u.Def, &u.HP, &u.MaxHP, &u.Level, &u.Exp, &u.Stamina, &u.Kills, &u.LastActionAt,
		&u.WeaponName, &u.WeaponKind, &u.WeaponAtt, &u.WeaponDur,
		&u.ArmorBody, &u.ArmorBodyDef, &u.ArmorBodyDur,
		&u.ArmorHead, &u.ArmorHeadDef, &u.ArmorHeadDur,
		&u.ArmorFoot, &u.ArmorFootDef, &u.ArmorFootDur,
		&u.ArmorArm, &u.ArmorArmDef, &u.ArmorArmDur,
		&profJSON, &u.BattleLog, &u.DeathMsg, &u.Message, &u.LastTargetID, &u.PoisonFlag,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(profJSON), &u.Proficiency)

	rows, err := q.QueryContext(ctx,
		`SELECT slot, name, kind, effect, durability FROM user_items WHERE user_id = ? ORDER BY slot ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	u.Items = make([]model.UserItem, 6)
	for i := range u.Items {
		u.Items[i] = model.UserItem{UserID: userID, Slot: i, Name: "없음"}
	}
	for rows.Next() {
		var it model.UserItem
		if err := rows.Scan(&it.Slot, &it.Name, &it.Kind, &it.Effect, &it.Durability); err == nil && it.Slot >= 0 && it.Slot < 6 {
			it.UserID = userID
			u.Items[it.Slot] = it
		}
	}
	return &u, nil
}

func saveUserTx(ctx context.Context, tx *sql.Tx, u *model.User) error {
	profJSON, _ := json.Marshal(u.Proficiency)
	_, err := tx.ExecContext(ctx,
		`UPDATE users SET
			status = ?, tactics = ?, area_id = ?, att = ?, def = ?, hp = ?, max_hp = ?,
			level = ?, exp = ?, stamina = ?, kills = ?, last_action_at = ?,
			weapon_name = ?, weapon_kind = ?, weapon_att = ?, weapon_dur = ?,
			armor_body = ?, armor_body_def = ?, armor_body_dur = ?,
			armor_head = ?, armor_head_def = ?, armor_head_dur = ?,
			armor_foot = ?, armor_foot_def = ?, armor_foot_dur = ?,
			armor_arm = ?, armor_arm_def = ?, armor_arm_dur = ?,
			proficiency_json = ?, battle_log = ?, death_msg = ?, message = ?, last_target_id = ?, poison_flag = ?
		 WHERE id = ?`,
		u.Status, u.Tactics, u.AreaID, u.Att, u.Def, u.HP, u.MaxHP,
		u.Level, u.Exp, u.Stamina, u.Kills, u.LastActionAt,
		u.WeaponName, u.WeaponKind, u.WeaponAtt, u.WeaponDur,
		u.ArmorBody, u.ArmorBodyDef, u.ArmorBodyDur,
		u.ArmorHead, u.ArmorHeadDef, u.ArmorHeadDur,
		u.ArmorFoot, u.ArmorFootDef, u.ArmorFootDur,
		u.ArmorArm, u.ArmorArmDef, u.ArmorArmDur,
		string(profJSON), u.BattleLog, u.DeathMsg, u.Message, u.LastTargetID, u.PoisonFlag,
		u.ID,
	)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_items WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	for _, it := range u.Items {
		if it.Name == "" || it.Name == "없음" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_items (user_id, slot, name, kind, effect, durability) VALUES (?, ?, ?, ?, ?, ?)`,
			u.ID, it.Slot, it.Name, it.Kind, it.Effect, it.Durability,
		); err != nil {
			return err
		}
	}
	return nil
}

func getGameStateQ(ctx context.Context, q querier) (*model.GameState, error) {
	var gs model.GameState
	var orderJSON string
	err := q.QueryRowContext(ctx,
		`SELECT id, status, started_at, next_area_at, forbidden_count, hack_flag, area_order FROM game_state WHERE id = 1`,
	).Scan(&gs.ID, &gs.Status, &gs.StartedAt, &gs.NextAreaAt, &gs.ForbiddenCount, &gs.HackFlag, &orderJSON)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(orderJSON), &gs.AreaOrder)
	return &gs, nil
}

func isAreaForbidden(gs *model.GameState, areaID int) (forbidden bool, warning bool) {
	if gs.HackFlag != 0 {
		return false, false
	}
	for i, id := range gs.AreaOrder {
		if id == areaID {
			if i < gs.ForbiddenCount {
				return true, false
			}
			if i < gs.ForbiddenCount+3 {
				return false, true
			}
			return false, false
		}
	}
	return false, false
}

func applyRecovery(u *model.User, now int64) bool {
	if u.Status == "DEAD" || u.HP <= 0 || u.LastActionAt <= 0 {
		return false
	}
	elapsed := now - u.LastActionAt
	if elapsed < config.RecoverTimeSec {
		return false
	}
	ticks := int(elapsed / config.RecoverTimeSec)
	staGain := ticks
	hpGain := ticks / config.RecoverHPRatio

	switch u.Status {
	case "치료":
		hpGain = ticks * 2
	case "수면":
		staGain = ticks * 3
		hpGain = ticks
	}

	u.Stamina += staGain
	if u.Stamina > config.MaxStamina {
		u.Stamina = config.MaxStamina
	}
	u.HP += hpGain
	if u.HP > u.MaxHP {
		u.HP = u.MaxHP
	}
	u.LastActionAt += int64(ticks * config.RecoverTimeSec)
	return true
}

func addNewsLogTx(ctx context.Context, tx *sql.Tx, now int64, kind, msg string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO news_logs (created_at, kind, message) VALUES (?, ?, ?)`,
		now, kind, strings.TrimSpace(msg),
	)
	return err
}
