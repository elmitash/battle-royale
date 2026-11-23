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

func areaTrait(gridPos int) config.AreaTrait {
	if named := config.NamedAreaByGridPos(gridPos); named != nil {
		return named.Trait
	}
	return ""
}

// findEncounterTx searches for an alive enemy or lootable corpse in the same grid cell.
// Returns (encounterTarget, isCorpse, ambushLog, error).
func (e *Engine) findEncounterTx(ctx context.Context, tx *sql.Tx, u *model.User, now int64, basePerSearch int) (*model.User, bool, string, error) {
	perSearch := basePerSearch
	if u.Tactics == "연속공격" {
		perSearch += (u.Level + u.Kills) / 2
		if perSearch > 90 {
			perSearch = 90
		}
	}
	if rand.IntN(100) > perSearch {
		return nil, false, "", nil
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM users WHERE area_id = ? AND id != ?`,
		u.AreaID, u.ID,
	)
	if err != nil {
		return nil, false, "", err
	}
	var candidates []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err == nil {
			candidates = append(candidates, cid)
		}
	}
	rows.Close()

	if len(candidates) == 0 {
		return nil, false, "", nil
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	for _, cid := range candidates {
		target, err := loadUserQ(ctx, tx, cid)
		if err != nil {
			continue
		}
		if u.Tactics != "연속공격" && target.LastTargetID == u.ID && target.HP > 0 {
			continue
		}

		if target.HP > 0 && target.Status != "DEAD" {
			// Found an alive opponent!
			// Preemptive check: ~60% chance player gets initiative, else enemy ambushes.
			if rand.IntN(10) <= 5 {
				u.LastTargetID = target.ID
				return target, false, fmt.Sprintf("%s (%s)와(과) 조우했다! 선제 공격의 기회다!", target.FullName(), target.ClassName), nil
			}
			// Enemy ambushes player once
			dmg, atkLog := resolveStrike(target, u, areaTrait(u.AreaID))
			u.LastTargetID = target.ID
			target.LastTargetID = u.ID
			_ = saveUserTx(ctx, tx, target)

			msg := fmt.Sprintf("%s (%s)에게 기습을 당했다!\n%s", target.FullName(), target.ClassName, atkLog)
			if u.HP <= 0 {
				u.HP = 0
				u.Status = "DEAD"
				target.Kills++
				_ = saveUserTx(ctx, tx, target)
				_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) %s에게 살해되었습니다.", u.FullName(), u.ClassName, target.FullName()))
				msg += fmt.Sprintf("\n%s은(는) 사망했다... (피해 %d)", u.FullName(), dmg)
				_, _ = checkLastSurvivorTx(ctx, tx, now)
			}
			return target, false, msg, nil
		}

		// Dead target: 20% chance to notice lootable corpse
		if target.HP <= 0 && rand.IntN(10) > 7 && hasLootableGear(target) {
			u.LastTargetID = target.ID
			return target, true, fmt.Sprintf("%s의 시체를 발견했다. 전리품을 회수할 수 있을 것 같다.", target.FullName()), nil
		}
	}
	return nil, false, "", nil
}

// Attack executes a player-initiated strike against targetID, with possible counterattack.
func (e *Engine) Attack(ctx context.Context, userID, targetID string) (string, error) {
	if targetID == "" {
		return "공격 대상이 없습니다.", nil
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
		return "사망한 상태에서는 공격할 수 없습니다.", nil
	}

	target, err := loadUserQ(ctx, tx, targetID)
	if err != nil {
		return "대상을 찾을 수 없습니다.", nil
	}
	if target.Status == "DEAD" || target.HP <= 0 {
		u.LastTargetID = ""
		_ = saveUserTx(ctx, tx, u)
		_ = tx.Commit()
		return fmt.Sprintf("%s은(는) 이미 사망했습니다.", target.FullName()), nil
	}
	if target.AreaID != u.AreaID {
		u.LastTargetID = ""
		_ = saveUserTx(ctx, tx, u)
		_ = tx.Commit()
		return "상대가 이미 다른 지역으로 이동했습니다.", nil
	}

	now := time.Now().Unix()
	applyRecovery(u, now)
	trait := areaTrait(u.AreaID)

	var logs []string
	dmg, strikeLog := resolveStrike(u, target, trait)
	logs = append(logs, fmt.Sprintf("%s에게 %s(으)로 공격! %s", target.FullName(), u.WeaponName, strikeLog))

	// Level difference EXP gain
	if dmg > 0 {
		lvDiff := target.Level - u.Level
		if lvDiff < 0 {
			lvDiff = 0
		}
		u.Exp -= (lvDiff/2 + 2)
		if lvMsg := checkLevelUp(u); lvMsg != "" {
			logs = append(logs, lvMsg)
		}
	}

	if target.HP <= 0 {
		target.HP = 0
		target.Status = "DEAD"
		u.Kills++
		u.LastTargetID = target.ID // Keep for optional looting
		logs = append(logs, fmt.Sprintf("%s을(를) 쓰러뜨렸다!", target.FullName()))
		if target.DeathMsg != "" {
			logs = append(logs, fmt.Sprintf("%s의 유언: \"%s\"", target.FullName(), target.DeathMsg))
		}
		_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) %s에게 살해되었습니다.", target.FullName(), target.ClassName, u.FullName()))
		if winMsg, err := checkLastSurvivorTx(ctx, tx, now); err == nil && winMsg != "" {
			logs = append(logs, winMsg)
		}
	} else {
		// 70% counterattack chance
		if rand.IntN(10) <= 7 {
			cdmg, cLog := resolveStrike(target, u, trait)
			logs = append(logs, fmt.Sprintf("%s의 반격! %s", target.FullName(), cLog))
			if cdmg > 0 {
				lvDiff := u.Level - target.Level
				if lvDiff < 0 {
					lvDiff = 0
				}
				target.Exp -= (lvDiff/2 + 1)
				_ = checkLevelUp(target)
			}
			if u.HP <= 0 {
				u.HP = 0
				u.Status = "DEAD"
				target.Kills++
				logs = append(logs, fmt.Sprintf("%s은(는) 반격을 맞고 사망했다...", u.FullName()))
				_ = addNewsLogTx(ctx, tx, now, "DEATH", fmt.Sprintf("%s(%s)이(가) %s의 반격으로 사망했습니다.", u.FullName(), u.ClassName, target.FullName()))
				_, _ = checkLastSurvivorTx(ctx, tx, now)
			}
		} else {
			logs = append(logs, fmt.Sprintf("%s은(는) 반격하지 못하고 물러났다.", target.FullName()))
		}
		u.LastTargetID = ""
		target.LastTargetID = u.ID
	}

	if err := saveUserTx(ctx, tx, target); err != nil {
		return "", err
	}
	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return strings.Join(logs, "\n"), nil
}

// Runaway clears the current combat target.
func (e *Engine) Runaway(ctx context.Context, userID string) (string, error) {
	if _, err := e.DB.ExecContext(ctx, `UPDATE users SET last_target_id = '' WHERE id = ?`, userID); err != nil {
		return "", err
	}
	return "전속력으로 도망쳤다...", nil
}

// LootCorpse takes an item or equipment from a dead user in the same area.
// sourceSlot: 0..5 = inventory slot, 6 = weapon, 7 = body, 8 = head, 9 = foot, 10 = arm.
func (e *Engine) LootCorpse(ctx context.Context, userID, targetID string, sourceSlot int) (string, error) {
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
	target, err := loadUserQ(ctx, tx, targetID)
	if err != nil {
		return "시체를 찾을 수 없습니다.", nil
	}
	if target.HP > 0 || target.AreaID != u.AreaID {
		return "전리품을 회수할 수 없는 상태입니다.", nil
	}

	destSlot := -1
	for i := 0; i < 5; i++ {
		if u.Items[i].Name == "없음" {
			destSlot = i
			break
		}
	}
	if destSlot == -1 {
		return "가방이 가득 차서 전리품을 챙길 수 없습니다.", nil
	}

	var loot model.UserItem
	switch sourceSlot {
	case 6:
		if target.WeaponName == "" || strings.Contains(target.WeaponName, "맨손") {
			return "가져갈 무기가 없습니다.", nil
		}
		loot = model.UserItem{Name: target.WeaponName, Kind: target.WeaponKind, Effect: target.WeaponAtt, Durability: target.WeaponDur}
		target.WeaponName, target.WeaponKind, target.WeaponAtt, target.WeaponDur = "맨손", "WN", 0, model.DurabilityInfinite
	case 7:
		if target.ArmorBody == "" || target.ArmorBody == "없음" || strings.Contains(target.ArmorBody, "속옷") {
			return "가져갈 몸 방어구가 없습니다.", nil
		}
		loot = model.UserItem{Name: target.ArmorBody, Kind: "DBN", Effect: target.ArmorBodyDef, Durability: target.ArmorBodyDur}
		target.ArmorBody, target.ArmorBodyDef, target.ArmorBodyDur = "속옷", 0, model.DurabilityInfinite
	case 8:
		if target.ArmorHead == "" || target.ArmorHead == "없음" {
			return "가져갈 머리 방어구가 없습니다.", nil
		}
		loot = model.UserItem{Name: target.ArmorHead, Kind: "DH", Effect: target.ArmorHeadDef, Durability: target.ArmorHeadDur}
		target.ArmorHead, target.ArmorHeadDef, target.ArmorHeadDur = "없음", 0, 0
	case 9:
		if target.ArmorFoot == "" || target.ArmorFoot == "없음" {
			return "가져갈 다리 방어구가 없습니다.", nil
		}
		loot = model.UserItem{Name: target.ArmorFoot, Kind: "DF", Effect: target.ArmorFootDef, Durability: target.ArmorFootDur}
		target.ArmorFoot, target.ArmorFootDef, target.ArmorFootDur = "없음", 0, 0
	case 10:
		if target.ArmorArm == "" || target.ArmorArm == "없음" {
			return "가져갈 팔 방어구가 없습니다.", nil
		}
		loot = model.UserItem{Name: target.ArmorArm, Kind: "DA", Effect: target.ArmorArmDef, Durability: target.ArmorArmDur}
		target.ArmorArm, target.ArmorArmDef, target.ArmorArmDur = "없음", 0, 0
	default:
		if sourceSlot < 0 || sourceSlot >= 6 || target.Items[sourceSlot].Name == "없음" {
			return "해당 위치에 아이템이 없습니다.", nil
		}
		loot = target.Items[sourceSlot]
		clearSlot(&target.Items[sourceSlot])
	}

	loot.UserID = u.ID
	loot.Slot = destSlot
	u.Items[destSlot] = loot
	u.LastTargetID = ""

	if err := saveUserTx(ctx, tx, target); err != nil {
		return "", err
	}
	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s의 시체에서 %s을(를) 회수했다.", target.FullName(), loot.Name), nil
}

func resolveStrike(atk, def *model.User, trait config.AreaTrait) (int, string) {
	// Accuracy check (base 80% + proficiency bonus)
	prof := getWeaponProf(atk)
	hitChance := 75 + prof/5
	if hitChance > 95 {
		hitChance = 95
	}
	if rand.IntN(100) >= hitChance {
		return 0, "그러나 간발의 차이로 피했다!"
	}

	attPower := float64(atk.Att + atk.WeaponAtt)
	// Weapon usable ammo check
	if (strings.HasPrefix(atk.WeaponKind, "WG") || strings.HasPrefix(atk.WeaponKind, "WA")) && atk.WeaponDur <= 0 {
		attPower = float64(atk.Att) // Out of ammo -> bash
	}
	attPower *= (1.0 + float64(prof)/200.0)
	if trait == config.TraitAttUp {
		attPower *= 1.2
	} else if trait == config.TraitAttDown {
		attPower *= 0.8
	}

	defPower := float64(def.Def + def.ArmorBodyDef + def.ArmorHeadDef + def.ArmorFootDef + def.ArmorArmDef)
	if trait == config.TraitDefUp {
		defPower *= 1.2
	} else if trait == config.TraitDefDown {
		defPower *= 0.8
	}

	base := attPower - defPower*0.6
	if base < 2 {
		base = 2
	}
	dmg := base/2.0 + rand.Float64()*(base/2.0)
	dmg *= armorAffinityMultiplier(atk.WeaponKind, def)
	finalDmg := int(dmg)
	if finalDmg < 1 {
		finalDmg = 1
	}

	def.HP -= finalDmg

	// 20% chance to degrade body armor durability
	if def.ArmorBodyDur > 0 && rand.IntN(5) == 0 {
		def.ArmorBodyDur--
		if def.ArmorBodyDur <= 0 {
			def.ArmorBody = "속옷"
			def.ArmorBodyDef = 0
			def.ArmorBodyDur = model.DurabilityInfinite
		}
	}

	// Weapon ammo / durability consumption & proficiency gain
	consumeWeaponOnStrike(atk)
	addWeaponProf(atk, 1)

	return finalDmg, fmt.Sprintf("%d 데미지!", finalDmg)
}

func armorAffinityMultiplier(wKind string, def *model.User) float64 {
	hasHelmet := def.ArmorHead != "" && def.ArmorHead != "없음"
	switch {
	case wKind == "WG" && hasHelmet:
		return 1.5
	case strings.HasPrefix(wKind, "WB") && hasHelmet:
		return 0.5
	default:
		return 1.0
	}
}

func consumeWeaponOnStrike(u *model.User) {
	switch {
	case strings.HasPrefix(u.WeaponKind, "WG") || strings.HasPrefix(u.WeaponKind, "WA"):
		if u.WeaponDur > 0 {
			u.WeaponDur--
		}
	case strings.HasPrefix(u.WeaponKind, "WC") || strings.HasPrefix(u.WeaponKind, "WD"):
		if u.WeaponDur > 0 {
			u.WeaponDur--
			if u.WeaponDur <= 0 {
				u.WeaponName, u.WeaponKind, u.WeaponAtt, u.WeaponDur = "맨손", "WN", 0, model.DurabilityInfinite
			}
		}
	case strings.HasPrefix(u.WeaponKind, "WN") && !strings.Contains(u.WeaponName, "맨손"):
		if rand.IntN(5) == 0 {
			u.WeaponAtt--
			if u.WeaponAtt <= 0 {
				u.WeaponName, u.WeaponKind, u.WeaponAtt, u.WeaponDur = "맨손", "WN", 0, model.DurabilityInfinite
			}
		}
	}
}

func getWeaponProf(u *model.User) int {
	switch {
	case strings.HasPrefix(u.WeaponKind, "WG"):
		return u.Proficiency.WG
	case strings.HasPrefix(u.WeaponKind, "WA"):
		return u.Proficiency.WA
	case strings.HasPrefix(u.WeaponKind, "WC"):
		return u.Proficiency.WC
	case strings.HasPrefix(u.WeaponKind, "WD"):
		return u.Proficiency.WD
	case strings.HasPrefix(u.WeaponKind, "WB"):
		return u.Proficiency.WB
	case strings.HasPrefix(u.WeaponKind, "WS"):
		return u.Proficiency.WS
	default:
		return u.Proficiency.WN
	}
}

func addWeaponProf(u *model.User, delta int) {
	switch {
	case strings.HasPrefix(u.WeaponKind, "WG"):
		u.Proficiency.WG += delta
	case strings.HasPrefix(u.WeaponKind, "WA"):
		u.Proficiency.WA += delta
	case strings.HasPrefix(u.WeaponKind, "WC"):
		u.Proficiency.WC += delta
	case strings.HasPrefix(u.WeaponKind, "WD"):
		u.Proficiency.WD += delta
	case strings.HasPrefix(u.WeaponKind, "WB"):
		u.Proficiency.WB += delta
	case strings.HasPrefix(u.WeaponKind, "WS"):
		u.Proficiency.WS += delta
	default:
		u.Proficiency.WN += delta
	}
}

func checkLevelUp(u *model.User) string {
	if u.Exp > 0 || u.HP <= 0 {
		return ""
	}
	u.MaxHP += rand.IntN(3) + 7
	u.Att += rand.IntN(3) + 2
	u.Def += rand.IntN(3) + 2
	u.Level++
	u.Exp = config.BaseExp + config.LvExpInc*u.Level
	return fmt.Sprintf("레벨이 %d(으)로 올랐다! (HP/공격/방어 상승)", u.Level)
}

func hasLootableGear(u *model.User) bool {
	if u.WeaponName != "" && !strings.Contains(u.WeaponName, "맨손") {
		return true
	}
	if u.ArmorBody != "" && u.ArmorBody != "없음" && !strings.Contains(u.ArmorBody, "속옷") {
		return true
	}
	if u.ArmorHead != "" && u.ArmorHead != "없음" {
		return true
	}
	if u.ArmorFoot != "" && u.ArmorFoot != "없음" {
		return true
	}
	if u.ArmorArm != "" && u.ArmorArm != "없음" {
		return true
	}
	for _, it := range u.Items {
		if it.Name != "" && it.Name != "없음" {
			return true
		}
	}
	return false
}
