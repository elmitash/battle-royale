package game

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/db"
	"battle-royale/internal/model"
)

func setupTestEngine(t *testing.T) (*Engine, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	master, err := db.LoadMasterData("../../data")
	if err != nil {
		_ = store.Close()
		t.Fatalf("load master: %v", err)
	}
	cfg := config.Load()
	if err := db.EnsureRoundInitialized(context.Background(), store.DB, master, cfg); err != nil {
		_ = store.Close()
		t.Fatalf("init round: %v", err)
	}
	eng := NewEngine(store.DB, master, cfg)
	return eng, func() { _ = store.Close() }
}

func seedTestPlayer(t *testing.T, eng *Engine, id string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := eng.DB.Exec(
		`INSERT INTO users (
			id, password_hash, first_name, last_name, sex, class_name, student_no, club, icon,
			status, tactics, area_id, att, def, hp, max_hp, level, exp, stamina, kills, last_action_at,
			weapon_name, weapon_kind, weapon_att, weapon_dur, proficiency_json
		) VALUES (?, 'hash', '홍', '길동', '남성', '3학년A반', 1, '검도부', 0,
		          'ALIVE', '보통', 1, 10, 10, 80, 100, 1, 16, 200, 0, ?,
		          '맨손', 'WN', 0, 0, '{}')`,
		id, now,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
}

func TestMoveSearchAndRecovery(t *testing.T) {
	eng, cleanup := setupTestEngine(t)
	defer cleanup()
	ctx := context.Background()

	seedTestPlayer(t, eng, "u1")

	// Move to area 0 ("분교") is forbidden by default (forbidden_count = 1, area_order[0] = 0).
	msg, err := eng.Move(ctx, "u1", 0)
	if err != nil {
		t.Fatalf("move to forbidden: %v", err)
	}
	if msg == "" {
		t.Fatal("expected forbidden message")
	}
	u, _ := eng.LoadUser(ctx, "u1")
	if u.AreaID != 1 {
		t.Fatalf("expected user to stay in area 1, got %d", u.AreaID)
	}

	// Move to area 2
	if _, err := eng.Move(ctx, "u1", 2); err != nil {
		t.Fatalf("move to area 2: %v", err)
	}
	u, _ = eng.LoadUser(ctx, "u1")
	if u.AreaID != 2 {
		t.Fatalf("expected area 2, got %d", u.AreaID)
	}
	if u.Stamina >= 200 {
		t.Fatalf("expected stamina to decrease, got %d", u.Stamina)
	}

	// Search in area 2
	if _, err := eng.Search(ctx, "u1"); err != nil {
		t.Fatalf("search: %v", err)
	}

	// FirstAid
	faMsg, err := eng.FirstAid(ctx, "u1")
	if err != nil {
		t.Fatalf("firstaid: %v", err)
	}
	if faMsg == "" {
		t.Fatal("expected firstaid message")
	}
}

func TestItemUseDropAndCombine(t *testing.T) {
	eng, cleanup := setupTestEngine(t)
	defer cleanup()
	ctx := context.Background()

	seedTestPlayer(t, eng, "u2")

	// Give 경유 + 비료 -> should craft 화약
	_, err := eng.DB.Exec(
		`INSERT INTO user_items (user_id, slot, name, kind, effect, durability) VALUES
		 ('u2', 0, '경유', 'Y', 1, 1),
		 ('u2', 1, '비료', 'Y', 1, 1),
		 ('u2', 2, '빵', 'SH', 30, 1)`,
	)
	if err != nil {
		t.Fatalf("seed items: %v", err)
	}

	// Combine slots 0 and 1
	msg, err := eng.CombineItems(ctx, "u2", 0, 1)
	if err != nil {
		t.Fatalf("combine: %v", err)
	}
	u, _ := eng.LoadUser(ctx, "u2")
	if u.Items[0].Name != "화약" {
		t.Fatalf("expected 화약 in slot 0, got %q (msg: %s)", u.Items[0].Name, msg)
	}

	// Use stamina item in slot 2
	if _, err := eng.UseItem(ctx, "u2", 2); err != nil {
		t.Fatalf("use item: %v", err)
	}
	u, _ = eng.LoadUser(ctx, "u2")
	if u.Stamina != 230 {
		t.Fatalf("expected stamina 230, got %d", u.Stamina)
	}
	if u.Items[2].Name != "없음" {
		t.Fatalf("expected slot 2 to be consumed, got %q", u.Items[2].Name)
	}

	// Drop 화약 from slot 0
	if _, err := eng.DropItem(ctx, "u2", 0); err != nil {
		t.Fatalf("drop item: %v", err)
	}
	u, _ = eng.LoadUser(ctx, "u2")
	if u.Items[0].Name != "없음" || u.Items[0].Durability == model.DurabilityInfinite {
		t.Fatalf("expected slot 0 to be empty, got %+v", u.Items[0])
	}
}

func TestAttackLootAndLevelUp(t *testing.T) {
	eng, cleanup := setupTestEngine(t)
	defer cleanup()
	ctx := context.Background()

	seedTestPlayer(t, eng, "attacker")
	seedTestPlayer(t, eng, "defender")

	// Give attacker strong weapon and low exp so attack/kill triggers level up
	_, err := eng.DB.Exec(
		`UPDATE users SET weapon_name = '일본도', weapon_kind = 'WNS', weapon_att = 50, weapon_dur = -1, exp = 1 WHERE id = 'attacker'`,
	)
	if err != nil {
		t.Fatalf("update attacker: %v", err)
	}
	_, err = eng.DB.Exec(
		`UPDATE users SET hp = 10, max_hp = 10, weapon_name = '식칼', weapon_kind = 'WNS', weapon_att = 15, weapon_dur = -1 WHERE id = 'defender'`,
	)
	if err != nil {
		t.Fatalf("update defender: %v", err)
	}

	// Retry up to 5 times in case of initial miss
	for i := 0; i < 5; i++ {
		if _, err := eng.Attack(ctx, "attacker", "defender"); err != nil {
			t.Fatalf("attack: %v", err)
		}
		def, _ := eng.LoadUser(ctx, "defender")
		if def.Status == "DEAD" {
			break
		}
	}

	def, _ := eng.LoadUser(ctx, "defender")
	if def.Status != "DEAD" {
		t.Fatalf("expected defender DEAD, got %s (hp=%d)", def.Status, def.HP)
	}

	atk, _ := eng.LoadUser(ctx, "attacker")
	if atk.Kills != 1 {
		t.Fatalf("expected attacker kills 1, got %d", atk.Kills)
	}
	if atk.Level < 2 {
		t.Fatalf("expected attacker level up to >=2, got %d", atk.Level)
	}

	// Loot defender's weapon (slot 6)
	if _, err := eng.LootCorpse(ctx, "attacker", "defender", 6); err != nil {
		t.Fatalf("loot: %v", err)
	}
	atk, _ = eng.LoadUser(ctx, "attacker")
	if atk.Items[0].Name != "식칼" {
		t.Fatalf("expected 식칼 in slot 0 after loot, got %q", atk.Items[0].Name)
	}
}

func TestForbiddenZoneExpansionAndHack(t *testing.T) {
	eng, cleanup := setupTestEngine(t)
	defer cleanup()
	ctx := context.Background()

	seedTestPlayer(t, eng, "victim")
	// Put victim in the next forbidden area (area_order[1]) and expire next_area_at
	gs, err := eng.GetGameState(ctx)
	if err != nil {
		t.Fatalf("get game state: %v", err)
	}
	_, _ = eng.DB.Exec(`UPDATE users SET area_id = ? WHERE id = 'victim'`, gs.AreaOrder[1])
	_, _ = eng.DB.Exec(`UPDATE game_state SET next_area_at = ? WHERE id = 1`, time.Now().Unix()-10)

	if err := eng.CheckForbiddenZones(ctx); err != nil {
		t.Fatalf("check forbidden zones: %v", err)
	}

	gsAfter, _ := eng.GetGameState(ctx)
	if gsAfter.ForbiddenCount != 4 {
		t.Fatalf("expected forbidden_count 4, got %d", gsAfter.ForbiddenCount)
	}
	v, _ := eng.LoadUser(ctx, "victim")
	if v.Status != "DEAD" {
		t.Fatalf("expected victim killed by forbidden zone, got %s", v.Status)
	}

	// Test HackProgram via 모바일PC
	seedTestPlayer(t, eng, "hacker")
	_, _ = eng.DB.Exec(`INSERT INTO user_items (user_id, slot, name, kind, effect, durability) VALUES ('hacker', 0, '모바일PC', 'Y', 1, 1)`)
	if _, err := eng.HackProgram(ctx, "hacker", 0); err != nil {
		t.Fatalf("hack: %v", err)
	}
	gsHack, _ := eng.GetGameState(ctx)
	if gsHack.HackFlag != 1 {
		t.Fatalf("expected hack_flag 1, got %d", gsHack.HackFlag)
	}
}

func TestConcurrentCombatSafety(t *testing.T) {
	eng, cleanup := setupTestEngine(t)
	defer cleanup()
	ctx := context.Background()

	seedTestPlayer(t, eng, "target")
	_, _ = eng.DB.Exec(`UPDATE users SET hp = 1, max_hp = 1 WHERE id = 'target'`)

	const workers = 10
	for i := 0; i < workers; i++ {
		id := fmt.Sprintf("p%d", i)
		seedTestPlayer(t, eng, id)
		_, _ = eng.DB.Exec(`UPDATE users SET weapon_name = '일본도', weapon_kind = 'WNS', weapon_att = 50, weapon_dur = -1 WHERE id = ?`, id)
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		id := fmt.Sprintf("p%d", i)
		go func(uid string) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				_, _ = eng.Attack(ctx, uid, "target")
			}
		}(id)
	}
	wg.Wait()

	var totalKills int
	_ = eng.DB.QueryRow(`SELECT COALESCE(SUM(kills), 0) FROM users WHERE id LIKE 'p%'`).Scan(&totalKills)
	if totalKills > 1 {
		t.Fatalf("duplicate kill detected under concurrency! totalKills=%d", totalKills)
	}
}
