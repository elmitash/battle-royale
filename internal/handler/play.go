package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"battle-royale/internal/config"
	"battle-royale/internal/game"
	"battle-royale/internal/model"
	"battle-royale/internal/view"
)

const flashCookieName = "br_flash"

// PlayHandler handles the main game views, actions, map, news, rank, and winner pages.
type PlayHandler struct {
	DB     *sql.DB
	Engine *game.Engine
	View   *view.Renderer
}

// BattlePage renders the main player dashboard with passive recovery applied.
func (h *PlayHandler) BattlePage(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	u, err := h.Engine.ApplyPassiveRecovery(r.Context(), uid)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	gs, err := h.Engine.GetGameState(r.Context())
	if err != nil {
		http.Error(w, "게임 상태를 불러올 수 없습니다.", http.StatusInternalServerError)
		return
	}

	forbiddenSet := make(map[int]bool)
	warningSet := make(map[int]bool)
	if gs.HackFlag == 0 {
		for i, id := range gs.AreaOrder {
			if i < gs.ForbiddenCount {
				forbiddenSet[id] = true
			} else if i < gs.ForbiddenCount+3 {
				warningSet[id] = true
			}
		}
	}

	type mapCell struct {
		Name      string
		Code      string
		LeftPct   string
		TopPct    string
		SizePct   string
		Forbidden bool
		Warning   bool
		IsCurrent bool
	}
	var mapCells []mapCell
	const totalPx = 740.0
	const padPx = 30.0
	const cellPx = 70.0
	sizePct := fmt.Sprintf("%.3f", (cellPx/totalPx)*100.0)

	for _, a := range config.Areas {
		rIdx, cIdx := parseAreaGrid(a.Code)
		leftPct := fmt.Sprintf("%.3f", ((padPx+float64(cIdx)*cellPx)/totalPx)*100.0)
		topPct := fmt.Sprintf("%.3f", ((padPx+float64(rIdx)*cellPx)/totalPx)*100.0)
		mapCells = append(mapCells, mapCell{
			Name:      a.Name,
			Code:      a.Code,
			LeftPct:   leftPct,
			TopPct:    topPct,
			SizePct:   sizePct,
			Forbidden: forbiddenSet[a.ID],
			Warning:   warningSet[a.ID],
			IsCurrent: a.ID == u.AreaID,
		})
	}

	type moveOption struct {
		ID   int
		Name string
		Code string
	}
	var moveOptions []moveOption
	for _, a := range config.Areas {
		if !forbiddenSet[a.ID] && a.ID != u.AreaID {
			moveOptions = append(moveOptions, moveOption{ID: a.ID, Name: a.Name, Code: a.Code})
		}
	}

	areaName, areaCode, areaInfo := "-", "-", ""
	if u.AreaID >= 0 && u.AreaID < len(config.Areas) {
		a := config.Areas[u.AreaID]
		areaName, areaCode, areaInfo = a.Name, a.Code, a.Info
	}

	flash := popFlash(w, r)

	var target *model.User
	if u.LastTargetID != "" {
		if t, err := h.Engine.LoadUser(r.Context(), u.LastTargetID); err == nil && t.AreaID == u.AreaID {
			target = t
		}
	}

	h.View.Render(w, "battle.html", map[string]any{
		"Title":       "게임 화면",
		"LoggedIn":    true,
		"User":        u,
		"Target":      target,
		"GameStatus":  gs.Status,
		"HackFlag":    gs.HackFlag,
		"AreaName":    areaName,
		"AreaCode":    areaCode,
		"AreaInfo":    areaInfo,
		"MoveOptions": moveOptions,
		"MapCells":    mapCells,
		"Flash":       flash,
		"MaxStamina":  config.MaxStamina,
		"TacticsList": []string{"보통", "공격중시", "방어중시", "은밀행동", "탐색중시", "연속공격"},
	})
}

// AttackAction handles POST /battle/attack.
func (h *PlayHandler) AttackAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	msg, err := h.Engine.Attack(r.Context(), uid, r.FormValue("target_id"))
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// RunawayAction handles POST /battle/runaway.
func (h *PlayHandler) RunawayAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	msg, err := h.Engine.Runaway(r.Context(), uid)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// LootAction handles POST /battle/loot.
func (h *PlayHandler) LootAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	srcSlot, err := strconv.Atoi(r.FormValue("source_slot"))
	if err != nil {
		setFlashAndRedirect(w, r, "잘못된 전리품 슬롯입니다.")
		return
	}
	msg, err := h.Engine.LootCorpse(r.Context(), uid, r.FormValue("target_id"), srcSlot)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// SpeakAction handles POST /battle/speak.
func (h *PlayHandler) SpeakAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slot, _ := strconv.Atoi(r.FormValue("slot"))
	msg, err := h.Engine.Speak(r.Context(), uid, slot, r.FormValue("text"))
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// PoisonCheckAction handles POST /battle/poisoncheck.
func (h *PlayHandler) PoisonCheckAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slot, _ := strconv.Atoi(r.FormValue("slot"))
	msg, err := h.Engine.CheckPoison(r.Context(), uid, slot)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// MoveAction handles POST /battle/move.
func (h *PlayHandler) MoveAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	targetID, err := strconv.Atoi(r.FormValue("area_id"))
	if err != nil {
		setFlashAndRedirect(w, r, "이동할 지역을 선택해 주세요.")
		return
	}
	msg, err := h.Engine.Move(r.Context(), uid, targetID)
	if err != nil {
		msg = "이동 처리 중 오류가 발생했습니다: " + err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// SearchAction handles POST /battle/search.
func (h *PlayHandler) SearchAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	msg, err := h.Engine.Search(r.Context(), uid)
	if err != nil {
		msg = "탐색 처리 중 오류가 발생했습니다: " + err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// RestAction handles POST /battle/rest.
func (h *PlayHandler) RestAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	mode := r.FormValue("mode")
	msg, err := h.Engine.SetRestMode(r.Context(), uid, mode)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// FirstAidAction handles POST /battle/firstaid.
func (h *PlayHandler) FirstAidAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	msg, err := h.Engine.FirstAid(r.Context(), uid)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// TacticsAction handles POST /battle/tactics.
func (h *PlayHandler) TacticsAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	msg, err := h.Engine.UpdateTactics(r.Context(), uid, r.FormValue("tactics"))
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// UseItemAction handles POST /battle/item/use.
func (h *PlayHandler) UseItemAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slot, err := strconv.Atoi(r.FormValue("slot"))
	if err != nil {
		setFlashAndRedirect(w, r, "잘못된 슬롯입니다.")
		return
	}
	msg, err := h.Engine.UseItem(r.Context(), uid, slot)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// DropItemAction handles POST /battle/item/drop.
func (h *PlayHandler) DropItemAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slot, err := strconv.Atoi(r.FormValue("slot"))
	if err != nil {
		setFlashAndRedirect(w, r, "잘못된 슬롯입니다.")
		return
	}
	msg, err := h.Engine.DropItem(r.Context(), uid, slot)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

// CombineItemAction handles POST /battle/item/combine.
func (h *PlayHandler) CombineItemAction(w http.ResponseWriter, r *http.Request) {
	uid := CurrentUserID(r)
	if uid == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s1, err1 := strconv.Atoi(r.FormValue("slot1"))
	s2, err2 := strconv.Atoi(r.FormValue("slot2"))
	if err1 != nil || err2 != nil {
		setFlashAndRedirect(w, r, "조합할 슬롯을 선택해 주세요.")
		return
	}
	msg, err := h.Engine.CombineItems(r.Context(), uid, s1, s2)
	if err != nil {
		msg = err.Error()
	}
	setFlashAndRedirect(w, r, msg)
}

func setFlashAndRedirect(w http.ResponseWriter, r *http.Request, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    url.QueryEscape(msg),
		Path:     "/battle",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/battle", http.StatusSeeOther)
}

func popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/battle",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	decoded, err := url.QueryUnescape(c.Value)
	if err != nil {
		return c.Value
	}
	return decoded
}

func parseAreaGrid(code string) (rowIdx int, colIdx int) {
	parts := strings.Split(code, "-")
	if len(parts) != 2 || len(parts[0]) == 0 {
		return 0, 0
	}
	r := int(parts[0][0] - 'A')
	c, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0
	}
	return r, c - 1
}

// MapPage displays the map image and forbidden zone schedule.
func (h *PlayHandler) MapPage(w http.ResponseWriter, r *http.Request) {
	var nextAreaAt int64
	var forbiddenCount int
	var orderJSON string
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT next_area_at, forbidden_count, area_order FROM game_state WHERE id = 1`,
	).Scan(&nextAreaAt, &forbiddenCount, &orderJSON)

	var order []int
	_ = json.Unmarshal([]byte(orderJSON), &order)

	type mapRow struct {
		Order     int
		Name      string
		Code      string
		Trait     config.AreaTrait
		Forbidden bool
	}
	rows := make([]mapRow, 0, len(order))
	for i, areaID := range order {
		if areaID < 0 || areaID >= len(config.Areas) {
			continue
		}
		a := config.Areas[areaID]
		rows = append(rows, mapRow{
			Order:     i + 1,
			Name:      a.Name,
			Code:      a.Code,
			Trait:     a.Trait,
			Forbidden: i < forbiddenCount,
		})
	}

	h.View.Render(w, "map.html", map[string]any{
		"Title":          "회장 지도",
		"LoggedIn":       CurrentUserID(r) != "",
		"ForbiddenCount": forbiddenCount,
		"NextAreaAt":     nextAreaAt,
		"Areas":          rows,
	})
}

// NewsPage displays recent news logs.
func (h *PlayHandler) NewsPage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, created_at, kind, message FROM news_logs ORDER BY id DESC LIMIT 100`,
	)
	var logs []model.NewsLog
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var nl model.NewsLog
			if err := rows.Scan(&nl.ID, &nl.CreatedAt, &nl.Kind, &nl.Message); err == nil {
				logs = append(logs, nl)
			}
		}
	}

	h.View.Render(w, "news.html", map[string]any{
		"Title":    "진행상황",
		"LoggedIn": CurrentUserID(r) != "",
		"Logs":     logs,
	})
}

// RankPage displays registered students and their status.
func (h *PlayHandler) RankPage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT class_name, student_no, first_name, last_name, club, icon, level, kills, status
		 FROM users WHERE status NOT LIKE 'NPC%' ORDER BY status ASC, kills DESC, level DESC`,
	)
	var users []model.User
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var u model.User
			if err := rows.Scan(&u.ClassName, &u.StudentNo, &u.FirstName, &u.LastName, &u.Club, &u.Icon, &u.Level, &u.Kills, &u.Status); err == nil {
				users = append(users, u)
			}
		}
	}

	h.View.Render(w, "rank.html", map[string]any{
		"Title":    "생존자 리스트",
		"LoggedIn": CurrentUserID(r) != "",
		"Users":    users,
	})
}

// WinnerPage displays past round winners.
func (h *PlayHandler) WinnerPage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, round_ended_at, winner_name, ending_type, kills FROM winners ORDER BY id DESC LIMIT 50`,
	)
	var winners []model.Winner
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var win model.Winner
			if err := rows.Scan(&win.ID, &win.RoundEndedAt, &win.WinnerName, &win.EndingType, &win.Kills); err == nil {
				winners = append(winners, win)
			}
		}
	}

	h.View.Render(w, "winner.html", map[string]any{
		"Title":    "역대 우승자",
		"LoggedIn": CurrentUserID(r) != "",
		"Winners":  winners,
	})
}
