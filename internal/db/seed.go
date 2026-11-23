package db

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"battle-royale/internal/config"
	"battle-royale/internal/model"
)

// AreaItemSeed represents an entry from itemfile.cgi.
type AreaItemSeed struct {
	AreaID int // 99 means random area 1..21
	Item   model.ItemSpec
}

// NPCSeed represents an NPC template from base.cgi.
type NPCSeed struct {
	FirstName string
	LastName  string
	Sex       string
	ClassName string
	StudentNo int
	Weapon    model.ItemSpec
	ArmorBody model.ItemSpec
	ArmorHead model.ItemSpec
	ArmorFoot model.ItemSpec
	ArmorArm  model.ItemSpec
	Icon      int
	Items     []model.ItemSpec
	Comment   string
	Message   string
	DeathMsg  string
}

// MasterData holds all static game tables loaded from data/.
type MasterData struct {
	NPCs       []NPCSeed
	Weapons    []model.ItemSpec
	StartItems []model.ItemSpec
	AreaItems  []AreaItemSeed
	Recipes    []model.Recipe
}

// LoadMasterData reads and parses all UTF-8 converted data files from dataDir.
func LoadMasterData(dataDir string) (*MasterData, error) {
	var err error
	md := &MasterData{}

	if md.Weapons, err = loadItemSpecList(filepath.Join(dataDir, "wepfile.cgi")); err != nil {
		return nil, fmt.Errorf("load wepfile.cgi: %w", err)
	}
	if md.StartItems, err = loadItemSpecList(filepath.Join(dataDir, "stitemfile.cgi")); err != nil {
		return nil, fmt.Errorf("load stitemfile.cgi: %w", err)
	}
	if md.AreaItems, err = loadAreaItemSeeds(filepath.Join(dataDir, "itemfile.cgi")); err != nil {
		return nil, fmt.Errorf("load itemfile.cgi: %w", err)
	}
	if md.Recipes, err = loadRecipes(filepath.Join(dataDir, "gousei_tbl.cgi")); err != nil {
		return nil, fmt.Errorf("load gousei_tbl.cgi: %w", err)
	}
	if md.NPCs, err = loadNPCSeeds(filepath.Join(dataDir, "base.cgi")); err != nil {
		return nil, fmt.Errorf("load base.cgi: %w", err)
	}
	return md, nil
}

// EnsureRoundInitialized creates the initial game_state, area_items, and NPCs if no round exists.
func EnsureRoundInitialized(ctx context.Context, sqlDB *sql.DB, master *MasterData, cfg config.Config) error {
	var count int
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM game_state WHERE id = 1").Scan(&count); err != nil {
		return fmt.Errorf("check game_state: %w", err)
	}
	if count > 0 {
		return nil
	}
	return ResetRound(ctx, sqlDB, master, cfg)
}

// ResetRound resets the current round in a single transaction, seeding game_state, area_items, and NPCs.
func ResetRound(ctx context.Context, sqlDB *sql.DB, master *MasterData, cfg config.Config) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_items"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM area_items"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM news_logs"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM game_state"); err != nil {
		return err
	}

	now := time.Now().Unix()
	nextAreaAt := now + 24*60*60 // Default 1 day until next forbidden zone

	// Area 0 ("분교") is always first, remaining 1..21 are shuffled.
	order := make([]int, len(config.Areas))
	order[0] = 0
	rest := make([]int, len(config.Areas)-1)
	for i := range rest {
		rest[i] = i + 1
	}
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	copy(order[1:], rest)
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO game_state (id, status, started_at, next_area_at, forbidden_count, hack_flag, area_order)
		 VALUES (1, 'running', ?, ?, 1, 0, ?)`,
		now, nextAreaAt, string(orderJSON),
	); err != nil {
		return err
	}

	// Seed area_items
	stmtItem, err := tx.PrepareContext(ctx,
		`INSERT INTO area_items (area_id, spawn_group, name, kind, effect, durability, owner_id)
		 VALUES (?, 1, ?, ?, ?, ?, '')`,
	)
	if err != nil {
		return err
	}
	defer stmtItem.Close()

	for _, ai := range master.AreaItems {
		areaID := ai.AreaID
		if areaID == 99 {
			areaID = rand.Intn(len(config.Areas)-1) + 1
		}
		if _, err := stmtItem.ExecContext(ctx, areaID, ai.Item.Name, ai.Item.Kind, ai.Item.Effect, ai.Item.Durability); err != nil {
			return err
		}
	}

	// Seed NPCs from base.cgi
	adminHash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for i, npc := range master.NPCs {
		npcID := fmt.Sprintf("%s%d", cfg.AdminID, i)
		isBossOrSoldier := npc.ClassName == "담임" || npc.ClassName == "병사"

		att := rand.Intn(5) + 8
		def := rand.Intn(5) + 8
		maxHP := rand.Intn(20) + 80
		level := 1
		areaID := rand.Intn(len(config.Areas)-1) + 1
		status := "NPC"
		var prof model.Proficiency

		if isBossOrSoldier {
			att = rand.Intn(10) + 40
			def = rand.Intn(10) + 40
			maxHP = rand.Intn(30) + 120
			level = 10
			areaID = 0
			status = "NPC0"
			p := config.BaseProf * 5
			prof = model.Proficiency{WN: p, WP: p, WA: p, WG: p, WE: p, WC: p, WD: p, WB: p, WF: p, WS: p}
		}
		exp := config.BaseExp + config.LvExpInc*level
		profJSON, _ := json.Marshal(prof)

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (
				id, password_hash, first_name, last_name, sex, class_name, student_no, club, icon,
				status, tactics, area_id, att, def, hp, max_hp, level, exp, stamina, kills, last_action_at,
				weapon_name, weapon_kind, weapon_att, weapon_dur,
				armor_body, armor_body_def, armor_body_dur,
				armor_head, armor_head_def, armor_head_dur,
				armor_foot, armor_foot_def, armor_foot_dur,
				armor_arm, armor_arm_def, armor_arm_dur,
				proficiency_json, message, death_msg
			) VALUES (?, ?, ?, ?, ?, ?, ?, '정부', ?, ?, '보통', ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			npcID, string(adminHash), npc.FirstName, npc.LastName, npc.Sex, npc.ClassName, npc.StudentNo, npc.Icon,
			status, areaID, att, def, maxHP, maxHP, level, exp, config.MaxStamina, now,
			npc.Weapon.Name, npc.Weapon.Kind, npc.Weapon.Effect, npc.Weapon.Durability,
			npc.ArmorBody.Name, npc.ArmorBody.Effect, npc.ArmorBody.Durability,
			npc.ArmorHead.Name, npc.ArmorHead.Effect, npc.ArmorHead.Durability,
			npc.ArmorFoot.Name, npc.ArmorFoot.Effect, npc.ArmorFoot.Durability,
			npc.ArmorArm.Name, npc.ArmorArm.Effect, npc.ArmorArm.Durability,
			string(profJSON), npc.Message, npc.DeathMsg,
		); err != nil {
			return err
		}

		for slot, item := range npc.Items {
			if item.Name == "" || item.Name == "없음" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO user_items (user_id, slot, name, kind, effect, durability) VALUES (?, ?, ?, ?, ?, ?)`,
				npcID, slot, item.Name, item.Kind, item.Effect, item.Durability,
			); err != nil {
				return err
			}
		}
	}

	// Initial news log
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO news_logs (created_at, kind, message) VALUES (?, 'NEWGAME', '새로운 게임이 시작되었습니다.')`,
		now,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func parseDurability(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "∞" || raw == "" {
		return model.DurabilityInfinite
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return v
}

func parseIntDefault(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func loadItemSpecList(path string) ([]model.ItemSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []model.ItemSpec
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		name, kind := model.ParseItemNameKind(parts[0])
		items = append(items, model.ItemSpec{
			Name:       name,
			Kind:       kind,
			Effect:     parseIntDefault(parts[1], 0),
			Durability: parseDurability(parts[2]),
		})
	}
	return items, sc.Err()
}

func loadAreaItemSeeds(path string) ([]AreaItemSeed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var seeds []AreaItemSeed
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		areaID := parseIntDefault(parts[0], 1)
		name, kind := model.ParseItemNameKind(parts[1])
		seeds = append(seeds, AreaItemSeed{
			AreaID: areaID,
			Item: model.ItemSpec{
				Name:       name,
				Kind:       kind,
				Effect:     parseIntDefault(parts[2], 0),
				Durability: parseDurability(parts[3]),
			},
		})
	}
	return seeds, sc.Err()
}

var recipeRegex = regexp.MustCompile(`push\s*\(\s*@tmakeitem\s*,\s*"([^"]+)"\s*\)\s*;?`)

func loadRecipes(path string) ([]model.Recipe, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var recipes []model.Recipe
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || line == "1;" {
			continue
		}
		m := recipeRegex.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		parts := strings.Split(m[1], "<>")
		if len(parts) < 6 {
			continue
		}
		recipes = append(recipes, model.Recipe{
			Ingredient1: strings.TrimSpace(parts[0]),
			Ingredient2: strings.TrimSpace(parts[1]),
			ResultName:  strings.TrimSpace(parts[2]),
			ResultKind:  strings.TrimSpace(parts[3]),
			Effect:      parseIntDefault(parts[4], 0),
			Durability:  parseDurability(parts[5]),
		})
	}
	return recipes, sc.Err()
}

func loadNPCSeeds(path string) ([]NPCSeed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var npcs []NPCSeed
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, ",")
		if len(p) < 39 {
			continue
		}
		parseEquip := func(idx int) model.ItemSpec {
			name, kind := model.ParseItemNameKind(p[idx])
			return model.ItemSpec{
				Name:       name,
				Kind:       kind,
				Effect:     parseIntDefault(p[idx+1], 0),
				Durability: parseDurability(p[idx+2]),
			}
		}
		var items []model.ItemSpec
		for slot := 0; slot < 6; slot++ {
			items = append(items, parseEquip(21+slot*3))
		}
		getCol := func(idx int) string {
			if idx < len(p) {
				return strings.TrimSpace(p[idx])
			}
			return ""
		}
		npcs = append(npcs, NPCSeed{
			FirstName: strings.TrimSpace(p[0]),
			LastName:  strings.TrimSpace(p[1]),
			Sex:       strings.TrimSpace(p[2]),
			ClassName: strings.TrimSpace(p[3]),
			StudentNo: parseIntDefault(p[4], 1),
			Weapon:    parseEquip(5),
			ArmorBody: parseEquip(8),
			ArmorHead: parseEquip(11),
			ArmorFoot: parseEquip(14),
			ArmorArm:  parseEquip(17),
			Icon:      parseIntDefault(p[20], 0),
			Items:     items,
			Comment:   getCol(39),
			Message:   getCol(40),
			DeathMsg:  getCol(41),
		})
	}
	return npcs, sc.Err()
}
