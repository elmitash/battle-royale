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

// Move relocates the user 1 step (N/S/E/W) to targetPos (0..99), deducts stamina, and searches.
func (e *Engine) Move(ctx context.Context, userID string, targetPos int) (string, error) {
	if targetPos < 0 || targetPos >= 100 {
		return "존재하지 않는 좌표입니다.", nil
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

	r1, c1 := u.AreaID/10, u.AreaID%10
	r2, c2 := targetPos/10, targetPos%10
	dr := r1 - r2
	if dr < 0 {
		dr = -dr
	}
	dc := c1 - c2
	if dc < 0 {
		dc = -dc
	}
	if dr+dc != 1 {
		return "현재 위치에서 동·서·남·북 인접한 칸으로만 이동할 수 있습니다.", nil
	}

	gs, err := getGameStateQ(ctx, tx)
	if err != nil {
		return "", err
	}

	areaName, areaCode, areaDesc := config.AreaNameAndCode(targetPos)
	var warning bool
	if named := config.NamedAreaByGridPos(targetPos); named != nil {
		var forbidden bool
		forbidden, warning = isAreaForbidden(gs, named.ID)
		if forbidden {
			return fmt.Sprintf("%s (%s)은(는) 금지구역입니다. 이동할 수 없습니다...", areaName, areaCode), nil
		}
	}

	now := time.Now().Unix()
	applyRecovery(u, now)
	u.Status = "ALIVE"

	var logs []string
	if warning {
		logs = append(logs, fmt.Sprintf("%s (%s)(으)로 이동했다. 곧 이곳은 금지구역이 되겠구나.", areaName, areaCode))
	} else {
		logs = append(logs, fmt.Sprintf("%s (%s)(으)로 이동했다.", areaName, areaCode))
	}
	logs = append(logs, strings.ReplaceAll(areaDesc, "<br>", " "))

	u.AreaID = targetPos

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

// Search explores the user's current grid cell for items, traps, or encounters.
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

func areaItemKey(gridPos int) int {
	if named := config.NamedAreaByGridPos(gridPos); named != nil {
		return named.ID
	}
	return 100 + gridPos
}

func (e *Engine) searchAreaTx(ctx context.Context, tx *sql.Tx, u *model.User, now int64) ([]string, error) {
	named := config.NamedAreaByGridPos(u.AreaID)

	// 1. Random area event (named areas only)
	if named != nil {
		if evLogs, triggered, err := triggerAreaEvent(ctx, tx, u, named.ID, now); err != nil {
			return nil, err
		} else if triggered {
			return evLogs, nil
		}
	}

	// 2. Enemy / Corpse encounter: 60% base in named areas, 20% base in unnamed wilderness
	encRate := 20
	if named != nil {
		encRate = 60
	}
	if target, _, encLog, err := e.findEncounterTx(ctx, tx, u, now, encRate); err != nil {
		return nil, err
	} else if target != nil {
		return []string{encLog}, nil
	}

	// 3. Discover item or trap from area_items
	itemKey := areaItemKey(u.AreaID)
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, kind, effect, durability FROM area_items WHERE area_id = ?`,
		itemKey,
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

	// Unnamed cells: if no dropped items/traps exist there, only 25% chance to scavenge a minor starter item.
	if named == nil && len(areaItems) == 0 {
		if rand.IntN(100) >= 25 || len(e.Master.StartItems) == 0 {
			return []string{"주변을 살폈지만 특별한 것을 찾지 못했다."}, nil
		}
		scav := e.Master.StartItems[rand.IntN(len(e.Master.StartItems))]
		return giveItemToUser(u, scav.Name, scav.Kind, scav.Effect, scav.Durability), nil
	}

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

	logs := giveItemToUser(u, picked.Name, picked.Kind, picked.Effect, picked.Durability)
	if !strings.Contains(logs[0], "포기했다") {
		if _, err := tx.ExecContext(ctx, `DELETE FROM area_items WHERE id = ?`, picked.ID); err != nil {
			return nil, err
		}
	}
	return logs, nil
}

func giveItemToUser(u *model.User, name, kind string, effect, durability int) []string {
	targetSlot := -1
	isStackable := strings.HasPrefix(kind, "WC") ||
		strings.HasPrefix(kind, "TN") ||
		strings.HasPrefix(kind, "NR") ||
		strings.Contains(name, "탄환") ||
		strings.Contains(name, "화살")

	for i := 0; i < 5; i++ {
		if u.Items[i].Name == "없음" {
			targetSlot = i
			break
		}
		if isStackable && u.Items[i].Name == name && u.Items[i].Kind == kind {
			targetSlot = i
			break
		}
	}

	if targetSlot == -1 {
		return []string{fmt.Sprintf("%s을(를) 발견했다. 그러나, 더 이상 가방에 들어가지 않아 포기했다...", name)}
	}

	if u.Items[targetSlot].Name == "없음" {
		u.Items[targetSlot] = model.UserItem{
			UserID:     u.ID,
			Slot:       targetSlot,
			Name:       name,
			Kind:       kind,
			Effect:     effect,
			Durability: durability,
		}
	} else if strings.Contains(name, "탄환") || strings.Contains(name, "화살") {
		u.Items[targetSlot].Effect += effect
	} else if durability > 0 && u.Items[targetSlot].Durability != model.DurabilityInfinite {
		u.Items[targetSlot].Durability += durability
	}

	return []string{fmt.Sprintf("%s을(를) 발견했다. %s", name, itemDiscoveryHint(kind))}
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

func triggerAreaEvent(ctx context.Context, tx *sql.Tx, u *model.User, namedAreaID int, now int64) ([]string, bool, error) {
	dice := rand.IntN(20)
	if dice < 17 {
		return nil, false, nil
	}
	dmg := rand.IntN(5) + 5

	switch namedAreaID {
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
