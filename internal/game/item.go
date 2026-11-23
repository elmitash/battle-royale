package game

import (
	"context"
	"fmt"
	"strings"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/model"
)

// UseItem consumes, equips, reloads, or plants the item in the given slot (0..5).
func (e *Engine) UseItem(ctx context.Context, userID string, slot int) (string, error) {
	if slot < 0 || slot >= 6 {
		return "잘못된 소지품 슬롯입니다.", nil
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
		return "사망한 상태에서는 아이템을 사용할 수 없습니다.", nil
	}

	it := u.Items[slot]
	if it.Name == "" || it.Name == "없음" {
		return "해당 슬롯이 비어 있습니다.", nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)

	var msg string

	switch {
	case strings.HasPrefix(it.Kind, "SH"):
		u.Stamina += it.Effect
		if u.Stamina > config.MaxStamina {
			u.Stamina = config.MaxStamina
		}
		consumeSlot(&u.Items[slot])
		msg = fmt.Sprintf("%s을(를) 사용했다. 스태미나가 회복됐다. (%d/%d)", it.Name, u.Stamina, config.MaxStamina)

	case strings.HasPrefix(it.Kind, "HH"):
		u.HP += it.Effect
		if u.HP > u.MaxHP {
			u.HP = u.MaxHP
		}
		consumeSlot(&u.Items[slot])
		msg = fmt.Sprintf("%s을(를) 사용했다. 체력이 회복됐다. (%d/%d)", it.Name, u.HP, u.MaxHP)

	case strings.HasPrefix(it.Kind, "SD") || strings.HasPrefix(it.Kind, "HD"):
		dmg := it.Effect
		if strings.HasPrefix(it.Kind, "SD2") || strings.HasPrefix(it.Kind, "HD2") {
			dmg = int(float64(it.Effect) * 1.5)
		}
		u.HP -= dmg
		consumeSlot(&u.Items[slot])
		msg = fmt.Sprintf("윽, 큰일났다! %s에 독이 들어가 있었던 것 같다! %d 데미지!", it.Name, dmg)
		if u.HP <= 0 {
			u.HP = 0
			u.Status = "DEAD"
			msg += fmt.Sprintf("\n%s은(는) 사망했다.", u.FullName())
			_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) 독이 든 %s을(를) 먹고 사망했습니다.", u.FullName(), u.ClassName, it.Name))
		}

	case strings.HasPrefix(it.Kind, "W") && !strings.HasPrefix(it.Kind, "WF"):
		old := model.UserItem{UserID: u.ID, Slot: slot, Name: u.WeaponName, Kind: u.WeaponKind, Effect: u.WeaponAtt, Durability: u.WeaponDur}
		u.WeaponName, u.WeaponKind, u.WeaponAtt, u.WeaponDur = it.Name, it.Kind, it.Effect, it.Durability
		if old.Name != "" && !strings.Contains(old.Name, "맨손") {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 무기로 장비했다.", it.Name)

	case strings.HasPrefix(it.Kind, "DB"):
		old := model.UserItem{UserID: u.ID, Slot: slot, Name: u.ArmorBody, Kind: "DBN", Effect: u.ArmorBodyDef, Durability: u.ArmorBodyDur}
		u.ArmorBody, u.ArmorBodyDef, u.ArmorBodyDur = it.Name, it.Effect, it.Durability
		if old.Name != "" && old.Name != "없음" && !strings.Contains(old.Name, "속옷") {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 몸에 장비했다.", it.Name)

	case strings.HasPrefix(it.Kind, "DH"):
		old := model.UserItem{UserID: u.ID, Slot: slot, Name: u.ArmorHead, Kind: "DH", Effect: u.ArmorHeadDef, Durability: u.ArmorHeadDur}
		u.ArmorHead, u.ArmorHeadDef, u.ArmorHeadDur = it.Name, it.Effect, it.Durability
		if old.Name != "" && old.Name != "없음" {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 머리에 장비했다.", it.Name)

	case strings.HasPrefix(it.Kind, "DF"):
		old := model.UserItem{UserID: u.ID, Slot: slot, Name: u.ArmorFoot, Kind: "DF", Effect: u.ArmorFootDef, Durability: u.ArmorFootDur}
		u.ArmorFoot, u.ArmorFootDef, u.ArmorFootDur = it.Name, it.Effect, it.Durability
		if old.Name != "" && old.Name != "없음" {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 다리에 장비했다.", it.Name)

	case strings.HasPrefix(it.Kind, "DA"):
		old := model.UserItem{UserID: u.ID, Slot: slot, Name: u.ArmorArm, Kind: "DA", Effect: u.ArmorArmDef, Durability: u.ArmorArmDur}
		u.ArmorArm, u.ArmorArmDef, u.ArmorArmDur = it.Name, it.Effect, it.Durability
		if old.Name != "" && old.Name != "없음" {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 팔에 장비했다.", it.Name)

	case strings.HasPrefix(it.Kind, "A"):
		old := u.Items[5]
		old.Slot = slot
		u.Items[5] = model.UserItem{UserID: u.ID, Slot: 5, Name: it.Name, Kind: it.Kind, Effect: it.Effect, Durability: it.Durability}
		if old.Name != "" && old.Name != "없음" {
			u.Items[slot] = old
		} else {
			clearSlot(&u.Items[slot])
		}
		msg = fmt.Sprintf("%s을(를) 장신구로 장착했다.", it.Name)

	case strings.HasPrefix(it.Kind, "TN"):
		trapKind := strings.Replace(it.Kind, "TN", "TO", 1)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO area_items (area_id, spawn_group, name, kind, effect, durability, owner_id) VALUES (?, 1, ?, ?, ?, ?, ?)`,
			areaItemKey(u.AreaID), it.Name, trapKind, it.Effect, it.Durability, u.ID,
		); err != nil {
			return "", err
		}
		consumeSlot(&u.Items[slot])
		msg = fmt.Sprintf("%s을(를) 덫으로 설치했다. 스스로도 조심하지 않으면...", it.Name)

	case (it.Name == "숫돌" || it.Name == "옷감") && strings.HasPrefix(u.WeaponKind, "WN"):
		u.WeaponAtt += it.Effect
		if u.WeaponAtt > 30 {
			u.WeaponAtt = 30
		}
		consumeSlot(&u.Items[slot])
		msg = fmt.Sprintf("%s을(를) 사용했다. %s의 공격력이 %d이(가) 되었다.", it.Name, u.WeaponName, u.WeaponAtt)

	case it.Name == "탄환" && strings.HasPrefix(u.WeaponKind, "WG"):
		up := it.Effect
		if u.WeaponDur+up > 6 {
			up = 6 - u.WeaponDur
		}
		if up < 0 {
			up = 0
		}
		u.WeaponDur += up
		u.Items[slot].Effect -= up
		if u.Items[slot].Effect <= 0 {
			clearSlot(&u.Items[slot])
		}
		if u.WeaponKind == "WGB" {
			u.WeaponKind = "WG"
		}
		msg = fmt.Sprintf("탄환을 %s에 장전했다. (사용횟수 +%d)", u.WeaponName, up)

	case strings.Contains(it.Name, "화살") && strings.HasPrefix(u.WeaponKind, "WA"):
		up := it.Effect
		if u.WeaponDur+up > 6 {
			up = 6 - u.WeaponDur
		}
		if up < 0 {
			up = 0
		}
		u.WeaponDur += up
		u.Items[slot].Effect -= up
		if u.Items[slot].Effect <= 0 {
			clearSlot(&u.Items[slot])
		}
		if u.WeaponKind == "WAB" {
			u.WeaponKind = "WA"
		}
		msg = fmt.Sprintf("%s을(를) %s에 보충했다. (사용횟수 +%d)", it.Name, u.WeaponName, up)

	case strings.HasPrefix(it.Kind, "R"):
		// Radar handled without consuming item
		rows, rerr := tx.QueryContext(ctx,
			`SELECT area_id, COUNT(*) FROM users WHERE status != 'DEAD' AND hp > 0 AND id != ? GROUP BY area_id`,
			u.ID,
		)
		if rerr != nil {
			return "", rerr
		}
		counts := make(map[int]int)
		for rows.Next() {
			var aid, cnt int
			if rows.Scan(&aid, &cnt) == nil {
				counts[aid] = cnt
			}
		}
		rows.Close()
		if it.Kind == "R1" {
			curName, curCode, _ := config.AreaNameAndCode(u.AreaID)
			msg = fmt.Sprintf("[간이 레이더] 현재 위치(%s %s)에 나 외의 생존 반응이 %d건 감지되었습니다.", curName, curCode, counts[u.AreaID])
		} else {
			var sb strings.Builder
			sb.WriteString("[광역 레이더 탐지 결과]\n")
			total := 0
			for aid, c := range counts {
				if c > 0 {
					aName, aCode, _ := config.AreaNameAndCode(aid)
					sb.WriteString(fmt.Sprintf("- %s (%s): %d명\n", aName, aCode, c))
					total += c
				}
			}
			if total == 0 {
				sb.WriteString("감지된 생존 반응이 없습니다.")
			}
			msg = strings.TrimSpace(sb.String())
		}

	case it.Name == "모바일PC":
		if it.Durability == 0 {
			msg = "모바일PC의 배터리가 방전되었습니다."
		} else {
			if it.Durability > 0 {
				u.Items[slot].Durability--
			}
			if _, err := tx.ExecContext(ctx, `UPDATE game_state SET hack_flag = 1 WHERE id = 1`); err != nil {
				return "", err
			}
			_ = addNewsLogTx(ctx, tx, now, "HACK", fmt.Sprintf("%s(%s)이(가) 정부 시스템 해킹에 성공하여 모든 금지구역이 해제되었습니다!", u.FullName(), u.ClassName))
			msg = "해킹 성공! 모든 금지구역이 해제되었습니다!"
		}

	case it.Name == "프로그램해제키":
		named := config.NamedAreaByGridPos(u.AreaID)
		if named != nil && named.ID == 0 {
			consumeSlot(&u.Items[slot])
			if _, err := tx.ExecContext(ctx, `UPDATE game_state SET status = 'ended' WHERE id = 1`); err != nil {
				return "", err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO winners (round_ended_at, winner_name, ending_type, kills, summary_json) VALUES (?, ?, '프로그램 해제', ?, '{}')`,
				now, u.FullName(), u.Kills,
			); err != nil {
				return "", err
			}
			_ = addNewsLogTx(ctx, tx, now, "EX_END", fmt.Sprintf("%s(%s)이(가) 분교에서 프로그램해제키를 사용해 게임을 종료시켰습니다!", u.FullName(), u.ClassName))
			msg = "해제키를 써서 프로그램을 정지시켰다. 목걸이를 벗었다! (게임 종료)"
		} else {
			msg = "여기에서 써도 의미가 없다... (분교에서만 사용 가능)"
		}

	default:
		msg = fmt.Sprintf("%s: 이건 어디에 쓰는 것일까...", it.Name)
	}

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return msg, nil
}

// DropItem drops an item from the user's inventory onto the current area.
func (e *Engine) DropItem(ctx context.Context, userID string, slot int) (string, error) {
	if slot < 0 || slot >= 6 {
		return "잘못된 소지품 슬롯입니다.", nil
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
		return "사망한 상태에서는 아이템을 버릴 수 없습니다.", nil
	}
	it := u.Items[slot]
	if it.Name == "" || it.Name == "없음" {
		return "해당 슬롯이 비어 있습니다.", nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO area_items (area_id, spawn_group, name, kind, effect, durability, owner_id) VALUES (?, 1, ?, ?, ?, ?, '')`,
		areaItemKey(u.AreaID), it.Name, it.Kind, it.Effect, it.Durability,
	); err != nil {
		return "", err
	}
	clearSlot(&u.Items[slot])

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s을(를) 버렸다.", it.Name), nil
}

// CombineItems attempts to synthesize two inventory slots using gousei_tbl recipes.
func (e *Engine) CombineItems(ctx context.Context, userID string, slot1, slot2 int) (string, error) {
	if slot1 < 0 || slot1 >= 5 || slot2 < 0 || slot2 >= 5 || slot1 == slot2 {
		return "조합할 서로 다른 소지품 2개를 선택해 주세요.", nil
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
		return "사망한 상태에서는 아이템을 조합할 수 없습니다.", nil
	}
	it1, it2 := u.Items[slot1], u.Items[slot2]
	if it1.Name == "없음" || it2.Name == "없음" {
		return "빈 슬롯은 조합할 수 없습니다.", nil
	}

	var matched *model.Recipe
	for i := range e.Master.Recipes {
		r := &e.Master.Recipes[i]
		if (r.Ingredient1 == it1.Name && r.Ingredient2 == it2.Name) ||
			(r.Ingredient1 == it2.Name && r.Ingredient2 == it1.Name) {
			matched = r
			break
		}
	}
	if matched == nil {
		return fmt.Sprintf("%s와(과) %s은(는) 조합할 수 없는 것 같다.", it1.Name, it2.Name), nil
	}

	consumeSlot(&u.Items[slot1])
	consumeSlot(&u.Items[slot2])

	// Put crafted item into slot1 if free, else first free slot 0..4
	dest := -1
	if u.Items[slot1].Name == "없음" {
		dest = slot1
	} else {
		for i := 0; i < 5; i++ {
			if u.Items[i].Name == "없음" {
				dest = i
				break
			}
		}
	}
	if dest == -1 {
		return "가방에 빈 공간이 없어 조합할 수 없습니다.", nil
	}

	u.Items[dest] = model.UserItem{
		UserID:     u.ID,
		Slot:       dest,
		Name:       matched.ResultName,
		Kind:       matched.ResultKind,
		Effect:     matched.Effect,
		Durability: matched.Durability,
	}

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s와(과) %s을(를) 조합하여 %s을(를) 만들었다!", it1.Name, it2.Name, matched.ResultName), nil
}

func consumeSlot(it *model.UserItem) {
	if it.Durability == model.DurabilityInfinite {
		return
	}
	it.Durability--
	if it.Durability <= 0 {
		clearSlot(it)
	}
}

func clearSlot(it *model.UserItem) {
	it.Name = "없음"
	it.Kind = ""
	it.Effect = 0
	it.Durability = 0
}
