package game

import (
	"context"
	"fmt"
	"strings"
	"time"

	"battle-royale/internal/config"
)

// UseRadar scans areas for alive players/NPCs (R1 = current area, R2/R3 = all areas).
func (e *Engine) UseRadar(ctx context.Context, userID string, slot int) (string, error) {
	u, err := e.LoadUser(ctx, userID)
	if err != nil {
		return "", err
	}
	if slot < 0 || slot >= 6 || !strings.HasPrefix(u.Items[slot].Kind, "R") {
		return "레이더 아이템이 아닙니다.", nil
	}

	rows, err := e.DB.QueryContext(ctx,
		`SELECT area_id, COUNT(*) FROM users WHERE status != 'DEAD' AND hp > 0 AND id != ? GROUP BY area_id`,
		userID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	counts := make(map[int]int)
	for rows.Next() {
		var areaID, cnt int
		if err := rows.Scan(&areaID, &cnt); err == nil {
			counts[areaID] = cnt
		}
	}

	if u.Items[slot].Kind == "R1" {
		curName, curCode, _ := config.AreaNameAndCode(u.AreaID)
		c := counts[u.AreaID]
		return fmt.Sprintf("[간이 레이더] 현재 위치(%s %s)에 나 외의 생존 반응이 %d건 감지되었습니다.", curName, curCode, c), nil
	}

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
	return strings.TrimSpace(sb.String()), nil
}

// Speak broadcasts a message to news_logs using a megaphone item.
func (e *Engine) Speak(ctx context.Context, userID string, slot int, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "방송할 내용을 입력해 주세요.", nil
	}
	if len([]rune(text)) > 80 {
		text = string([]rune(text)[:80])
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
	if slot < 0 || slot >= 6 || !strings.Contains(u.Items[slot].Name, "확성기") {
		return "확성기가 필요합니다.", nil
	}

	now := time.Now().Unix()
	consumeSlot(&u.Items[slot])
	_ = addNewsLogTx(ctx, tx, now, "SPEAK", fmt.Sprintf("[%s] \"%s\"", u.FullName(), text))

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("확성기로 외쳤다: \"%s\"", text), nil
}

// PoisonFood mixes poison into a food/drink item (SH->SD, HH->HD).
func (e *Engine) PoisonFood(ctx context.Context, userID string, poisonSlot, foodSlot int) (string, error) {
	if poisonSlot < 0 || poisonSlot >= 5 || foodSlot < 0 || foodSlot >= 5 || poisonSlot == foodSlot {
		return "독약과 음식 슬롯을 올바르게 선택해 주세요.", nil
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
	if !strings.Contains(u.Items[poisonSlot].Name, "독약") {
		return "선택한 슬롯에 독약이 없습니다.", nil
	}
	food := &u.Items[foodSlot]
	if !strings.HasPrefix(food.Kind, "SH") && !strings.HasPrefix(food.Kind, "HH") {
		return "독을 탈 수 있는 음식이나 음료가 아닙니다.", nil
	}

	if strings.HasPrefix(food.Kind, "SH") {
		food.Kind = strings.Replace(food.Kind, "SH", "SD", 1)
	} else {
		food.Kind = strings.Replace(food.Kind, "HH", "HD", 1)
	}
	consumeSlot(&u.Items[poisonSlot])

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s에 독을 탔다...", food.Name), nil
}

// CheckPoison inspects a food item for poison at the cost of PoisonCheckSta stamina.
func (e *Engine) CheckPoison(ctx context.Context, userID string, foodSlot int) (string, error) {
	if foodSlot < 0 || foodSlot >= 5 {
		return "조사할 슬롯을 선택해 주세요.", nil
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
	if u.Stamina < config.PoisonCheckSta {
		return fmt.Sprintf("독 조사를 하려면 스태미나가 %d 이상 필요합니다.", config.PoisonCheckSta), nil
	}
	it := u.Items[foodSlot]
	if it.Name == "없음" {
		return "빈 슬롯입니다.", nil
	}
	u.Stamina -= config.PoisonCheckSta
	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}

	if strings.HasPrefix(it.Kind, "SD") || strings.HasPrefix(it.Kind, "HD") {
		return fmt.Sprintf("[독 조사] 위험! %s에는 독이 들어 있습니다!", it.Name), nil
	}
	return fmt.Sprintf("[독 조사] %s은(는) 안전한 것 같습니다.", it.Name), nil
}

// HackProgram disables all forbidden zones using 모바일PC.
func (e *Engine) HackProgram(ctx context.Context, userID string, slot int) (string, error) {
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
	if slot < 0 || slot >= 5 || !strings.Contains(u.Items[slot].Name, "모바일PC") {
		return "해킹에는 모바일PC가 필요합니다.", nil
	}
	if u.Items[slot].Durability == 0 {
		return "모바일PC의 배터리가 방전되었습니다.", nil
	}

	now := time.Now().Unix()
	if u.Items[slot].Durability > 0 {
		u.Items[slot].Durability--
	}
	if _, err := tx.ExecContext(ctx, `UPDATE game_state SET hack_flag = 1 WHERE id = 1`); err != nil {
		return "", err
	}
	_ = addNewsLogTx(ctx, tx, now, "HACK", fmt.Sprintf("%s(%s)이(가) 정부 시스템 해킹에 성공하여 모든 금지구역이 해제되었습니다!", u.FullName(), u.ClassName))

	if err := saveUserTx(ctx, tx, u); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return "해킹 성공! 모든 금지구역이 해제되었습니다!", nil
}
