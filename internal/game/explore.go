package game

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/model"
)

// Move relocates the user to targetAreaID, deducts stamina, and triggers area search.
func (e *Engine) Move(ctx context.Context, userID string, targetAreaID int) (string, error) {
	if targetAreaID < 0 || targetAreaID >= len(config.Areas) {
		return "존재하지 않는 지역입니다.", nil
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
	if u.Status == "DEAD" || u.HP <= 0 {
		return "사망한 상태에서는 이동할 수 없습니다.", nil
	}

	gs, err := getGameStateQ(ctx, tx)
	if err != nil {
		return "", err
	}

	forbidden, warning := isAreaForbidden(gs, targetAreaID)
	area := config.Areas[targetAreaID]
	if forbidden {
		return fmt.Sprintf("%s은(는) 금지구역입니다. 이동할 수 없습니다...", area.Name), nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)
	u.Status = "ALIVE"

	var logs []string
	if warning {
		logs = append(logs, fmt.Sprintf("%s(으)로 이동했다. 곧 이곳은 금지구역이 되겠구나.", area.Name))
	} else {
		logs = append(logs, fmt.Sprintf("%s(으)로 이동했다.", area.Name))
	}
	logs = append(logs, strings.ReplaceAll(area.Info, "<br>", " "))

	u.AreaID = targetAreaID

	staCost := rand.IntN(5) + 8
	if strings.Contains(u.PoisonFlag, "足") {
		staCost = rand.IntN(5) + 13
	} else if u.Club == "육상부" {
		staCost = rand.IntN(5) + 5
	}
	u.Stamina -= staCost
	if u.Stamina <= 0 {
		u.Stamina = 0
		logs = append(logs, "스태미나가 바닥나 지쳐버렸다... (휴식 또는 음식 섭취 필요)")
	}

	searchLogs, err := e.searchAreaTx(ctx, tx, u, now)
	if err != nil {
		return "", err
	}
	logs = append(logs, searchLogs...)

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return strings.Join(logs, "\n"), nil
}

// Search explores the user's current area for items, traps, or random events.
func (e *Engine) Search(ctx context.Context, userID string) (string, error) {
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
		return "사망한 상태에서는 탐색할 수 없습니다.", nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)
	u.Status = "ALIVE"

	logs := []string{fmt.Sprintf("%s은(는) 주위를 탐색했다...", u.LastName)}

	staCost := rand.IntN(5) + 18
	if strings.Contains(u.PoisonFlag, "足") {
		staCost = rand.IntN(5) + 23
	} else if u.Club == "육상부" {
		staCost = rand.IntN(5) + 13
	}
	u.Stamina -= staCost
	if u.Stamina <= 0 {
		u.Stamina = 0
		logs = append(logs, "스태미나가 바닥나 지쳐버렸다...")
	}

	searchLogs, err := e.searchAreaTx(ctx, tx, u, now)
	if err != nil {
		return "", err
	}
	logs = append(logs, searchLogs...)

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return strings.Join(logs, "\n"), nil
}

func (e *Engine) searchAreaTx(ctx context.Context, tx *sql.Tx, u *model.User, now int64) ([]string, error) {
	// 1. Random area event (~15% chance, matching event.cgi dice >= 17 out of 0..19)
	if evLogs, triggered, err := triggerAreaEvent(ctx, tx, u, now); err != nil {
		return nil, err
	} else if triggered {
		return evLogs, nil
	}

	// 2. Enemy / Corpse encounter
	if target, _, encLog, err := e.findEncounterTx(ctx, tx, u, now); err != nil {
		return nil, err
	} else if target != nil {
		return []string{encLog}, nil
	}

	// 3. Discover item or trap from area_items
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, kind, effect, durability FROM area_items WHERE area_id = ?`,
		u.AreaID,
	)
	if err != nil {
		return nil, err
	}
	var areaItems []model.AreaItem
	for rows.Next() {
		var ai model.AreaItem
		if err := rows.Scan(&ai.ID, &ai.Name, &ai.Kind, &ai.Effect, &ai.Durability); err == nil {
			areaItems = append(areaItems, ai)
		}
	}
	rows.Close()

	if len(areaItems) == 0 {
		return []string{"이제, 이 지역에는 아무것도 없는건가...?"}, nil
	}

	picked := areaItems[rand.IntN(len(areaItems))]

	// Planted trap ("TO")
	if strings.HasPrefix(picked.Kind, "TO") {
		if _, err := tx.ExecContext(ctx, `DELETE FROM area_items WHERE id = ?`, picked.ID); err != nil {
			return nil, err
		}
		half := picked.Effect / 2
		if half < 1 {
			half = 1
		}
		dmg := rand.IntN(half) + half
		u.HP -= dmg
		logs := []string{fmt.Sprintf("덫이다! 설치되어 있던 %s에 상처를 입어 %d의 데미지를 입었다!", picked.Name, dmg)}
		if u.HP <= 0 {
			u.HP = 0
			u.Status = "DEAD"
			logs = append(logs, fmt.Sprintf("%s (%s %s%d번)은(는) 사망했다.", u.FullName(), u.ClassName, u.Sex, u.StudentNo))
			_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) 덫(%s)에 의해 사망했습니다.", u.FullName(), u.ClassName, picked.Name))
		}
		return logs, nil
	}

	// Find empty or stackable slot (0..4; slot 5 is accessory)
	targetSlot := -1
	isStackable := strings.HasPrefix(picked.Kind, "WC") ||
		strings.HasPrefix(picked.Kind, "TN") ||
		strings.HasPrefix(picked.Kind, "NR") ||
		strings.Contains(picked.Name, "탄환") ||
		strings.Contains(picked.Name, "화살")

	for i := 0; i < 5; i++ {
		if u.Items[i].Name == "없음" {
			targetSlot = i
			break
		}
		if isStackable && u.Items[i].Name == picked.Name && u.Items[i].Kind == picked.Kind {
			targetSlot = i
			break
		}
	}

	if targetSlot == -1 {
		return []string{fmt.Sprintf("%s을(를) 발견했다. 그러나, 더 이상 가방에 들어가지 않아 포기했다...", picked.Name)}, nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM area_items WHERE id = ?`, picked.ID); err != nil {
		return nil, err
	}

	if u.Items[targetSlot].Name == "없음" {
		u.Items[targetSlot] = model.UserItem{
			UserID:     u.ID,
			Slot:       targetSlot,
			Name:       picked.Name,
			Kind:       picked.Kind,
			Effect:     picked.Effect,
			Durability: picked.Durability,
		}
	} else if strings.Contains(picked.Name, "탄환") || strings.Contains(picked.Name, "화살") {
		u.Items[targetSlot].Effect += picked.Effect
	} else if picked.Durability > 0 && u.Items[targetSlot].Durability != model.DurabilityInfinite {
		u.Items[targetSlot].Durability += picked.Durability
	}

	hint := itemDiscoveryHint(picked.Kind)
	return []string{fmt.Sprintf("%s을(를) 발견했다. %s", picked.Name, hint)}, nil
}

func itemDiscoveryHint(kind string) string {
	switch {
	case strings.HasPrefix(kind, "HH"), strings.HasPrefix(kind, "HD"):
		return "먹으면 체력이 회복될 것 같다."
	case strings.HasPrefix(kind, "SH"), strings.HasPrefix(kind, "SD"):
		return "먹으면 스태미나가 회복될 것 같다."
	case strings.HasPrefix(kind, "W"):
		return "이건 무기가 될 듯하다."
	case strings.HasPrefix(kind, "D"):
		return "이건 방어구가 될 듯하다."
	case strings.HasPrefix(kind, "A"):
		return "이건 몸에 달 수 있을 것 같다."
	case strings.HasPrefix(kind, "TN"):
		return "이것으로 덫을 설치할 수 있을 것 같다."
	default:
		return "분명 뭔가에 쓸 수 있겠지."
	}
}

func triggerAreaEvent(ctx context.Context, tx *sql.Tx, u *model.User, now int64) ([]string, bool, error) {
	dice := rand.IntN(20)
	if dice < 17 {
		return nil, false, nil
	}
	dmg := rand.IntN(5) + 5

	switch u.AreaID {
	case 2, 12, 18:
		logs := []string{"문득 하늘을 보자 새떼들이다!"}
		if dice == 18 {
			if !strings.Contains(u.PoisonFlag, "頭") {
				u.PoisonFlag += "頭"
			}
			logs = append(logs, "새에게 습격받아 머리를 다쳤다!")
		} else if dice == 19 {
			u.HP -= dmg
			logs = append(logs, fmt.Sprintf("새에게 습격받아 %d 데미지를 입었다!", dmg))
			if u.HP <= 0 {
				u.HP = 0
				u.Status = "DEAD"
				logs = append(logs, fmt.Sprintf("%s은(는) 사망했다.", u.FullName()))
				_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) 새떼의 습격으로 사망했습니다.", u.FullName(), u.ClassName))
			}
		} else {
			logs = append(logs, "후우, 간신히 쫓아냈다...")
		}
		return logs, true, nil

	case 10:
		logs := []string{"이런, 토사가 쏟아졌다!"}
		if dice == 18 {
			if !strings.Contains(u.PoisonFlag, "足") {
				u.PoisonFlag += "足"
			}
			logs = append(logs, "간신히 피했지만 낙석에 발을 다쳤다!")
		} else if dice == 19 {
			u.HP -= dmg
			logs = append(logs, fmt.Sprintf("낙석에 의해 %d 데미지를 입었다!", dmg))
			if u.HP <= 0 {
				u.HP = 0
				u.Status = "DEAD"
				logs = append(logs, fmt.Sprintf("%s은(는) 사망했다.", u.FullName()))
				_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) 낙석 사고로 사망했습니다.", u.FullName(), u.ClassName))
			}
		} else {
			logs = append(logs, "후우, 간신히 피했다...")
		}
		return logs, true, nil

	case 16:
		logs := []string{"갑자기 들개가 습격해 왔다!"}
		if dice == 18 {
			if !strings.Contains(u.PoisonFlag, "腕") {
				u.PoisonFlag += "腕"
			}
			logs = append(logs, "팔을 물려 팔을 다쳤다!")
		} else if dice == 19 {
			u.HP -= dmg
			logs = append(logs, fmt.Sprintf("들개에게 습격받아 %d 데미지를 입었다!", dmg))
			if u.HP <= 0 {
				u.HP = 0
				u.Status = "DEAD"
				logs = append(logs, fmt.Sprintf("%s은(는) 사망했다.", u.FullName()))
				_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) 들개에게 습격당해 사망했습니다.", u.FullName(), u.ClassName))
			}
		} else {
			logs = append(logs, "후우, 간신히 쫓아냈다...")
		}
		return logs, true, nil

	case 17:
		logs := []string{"이런, 발이 미끄러진다!"}
		if dice <= 18 {
			cost := dmg + 10
			u.Stamina -= cost
			if u.Stamina < 0 {
				u.Stamina = 0
			}
			logs = append(logs, fmt.Sprintf("연못 안으로 떨어졌지만 간신히 기어 올라왔다! 스태미나를 %d 소비했다!", cost))
		} else {
			logs = append(logs, "후우, 간신히 미끄러지지 않았다...")
		}
		return logs, true, nil
	}

	return nil, false, nil
}
