package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"battle-royale/internal/config"
)

// StartScheduler launches a background goroutine that periodically checks forbidden zone expansion.
func (e *Engine) StartScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.CheckForbiddenZones(ctx); err != nil {
					log.Printf("forbidden zone check error: %v", err)
				}
			}
		}
	}()
}

// CheckForbiddenZones advances forbidden zones if next_area_at has passed.
func (e *Engine) CheckForbiddenZones(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	gs, err := getGameStateQ(ctx, tx)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if gs.Status != "running" || gs.HackFlag != 0 || now < gs.NextAreaAt {
		return nil
	}

	newCount := gs.ForbiddenCount + 3
	if newCount > len(gs.AreaOrder) {
		newCount = len(gs.AreaOrder)
	}
	nextAt := now + 24*60*60

	if _, err := tx.ExecContext(ctx,
		`UPDATE game_state SET forbidden_count = ?, next_area_at = ? WHERE id = 1`,
		newCount, nextAt,
	); err != nil {
		return err
	}

	var addedNames []string
	for i := gs.ForbiddenCount; i < newCount && i < len(gs.AreaOrder); i++ {
		aid := gs.AreaOrder[i]
		if aid >= 0 && aid < len(config.Areas) {
			addedNames = append(addedNames, config.Areas[aid].Name)
		}
	}
	if len(addedNames) > 0 {
		_ = addNewsLogTx(ctx, tx, now, "AREAADD", fmt.Sprintf("새로운 금지구역이 추가되었습니다: %v", addedNames))
	}

	// Kill any alive non-NPC0 users in forbidden areas
	for i := 0; i < newCount && i < len(gs.AreaOrder); i++ {
		aid := gs.AreaOrder[i]
		gridPos := config.GridPosByAreaID(aid)
		rows, err := tx.QueryContext(ctx,
			`SELECT id, first_name, last_name, class_name FROM users WHERE area_id = ? AND status != 'DEAD' AND status != 'NPC0' AND hp > 0`,
			gridPos,
		)
		if err != nil {
			continue
		}
		type victim struct{ id, name, cls string }
		var victims []victim
		for rows.Next() {
			var id, fn, ln, cls string
			if err := rows.Scan(&id, &fn, &ln, &cls); err == nil {
				victims = append(victims, victim{id, fn + ln, cls})
			}
		}
		rows.Close()

		for _, v := range victims {
			_, _ = tx.ExecContext(ctx, `UPDATE users SET hp = 0, status = 'DEAD' WHERE id = ?`, v.id)
			_ = addNewsLogTx(ctx, tx, now, "DEATHAREA", fmt.Sprintf("%s(%s)이(가) 금지구역(%s) 폭발로 사망했습니다.", v.name, v.cls, config.Areas[aid].Name))
		}
	}

	if newCount >= len(gs.AreaOrder) {
		var alive int
		_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE status NOT LIKE 'NPC%' AND status != 'DEAD' AND hp > 0`).Scan(&alive)
		if alive == 0 {
			_, _ = tx.ExecContext(ctx, `UPDATE game_state SET status = 'ended' WHERE id = 1`)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO winners (round_ended_at, winner_name, ending_type, kills, summary_json) VALUES (?, '우승자없음', '전원사망', 0, '{}')`,
				now,
			)
			_ = addNewsLogTx(ctx, tx, now, "NOWINNER", "모든 구역이 금지구역이 되어 전원 사망했습니다.")
		}
	}

	_, _ = checkLastSurvivorTx(ctx, tx, now)
	return tx.Commit()
}

func checkLastSurvivorTx(ctx context.Context, tx *sql.Tx, now int64) (string, error) {
	gs, err := getGameStateQ(ctx, tx)
	if err != nil || gs.Status != "running" {
		return "", err
	}

	// Respect minimum battle duration (BattleMinDays) unless all players are dead.
	if now-gs.StartedAt < int64(config.BattleMinDays*24*60*60) {
		return "", nil
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, first_name, last_name, class_name, kills FROM users WHERE status NOT LIKE 'NPC%' AND status != 'DEAD' AND hp > 0`,
	)
	if err != nil {
		return "", err
	}
	type survivor struct {
		id, name, cls string
		kills         int
	}
	var survivors []survivor
	for rows.Next() {
		var id, fn, ln, cls string
		var k int
		if err := rows.Scan(&id, &fn, &ln, &cls, &k); err == nil {
			survivors = append(survivors, survivor{id, fn + ln, cls, k})
		}
	}
	rows.Close()

	if len(survivors) != 1 {
		return "", nil
	}

	w := survivors[0]
	summary, _ := json.Marshal(map[string]any{"user_id": w.id, "class": w.cls})
	if _, err := tx.ExecContext(ctx, `UPDATE game_state SET status = 'ended' WHERE id = 1`); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO winners (round_ended_at, winner_name, ending_type, kills, summary_json) VALUES (?, ?, '최후 생존', ?, ?)`,
		now, w.name, w.kills, string(summary),
	); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("최후의 생존자 %s(%s)이(가) 우승했습니다! (킬 수: %d)", w.name, w.cls, w.kills)
	_ = addNewsLogTx(ctx, tx, now, "WINEND", msg)
	return msg, nil
}
