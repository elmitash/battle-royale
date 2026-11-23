package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/db"
	"battle-royale/internal/game"
	"battle-royale/internal/model"
	"battle-royale/internal/view"
)

const adminCookieName = "br_admin_session"

// AdminHandler manages the game master control panel.
type AdminHandler struct {
	DB     *sql.DB
	Master *db.MasterData
	Cfg    config.Config
	Engine *game.Engine
	View   *view.Renderer

	mu       sync.Mutex
	sessions map[string]int64
}

// NewAdminHandler creates an AdminHandler with an in-memory admin session store.
func NewAdminHandler(sqlDB *sql.DB, master *db.MasterData, cfg config.Config, engine *game.Engine, renderer *view.Renderer) *AdminHandler {
	return &AdminHandler{
		DB:       sqlDB,
		Master:   master,
		Cfg:      cfg,
		Engine:   engine,
		View:     renderer,
		sessions: make(map[string]int64),
	}
}

func (h *AdminHandler) isAdmin(r *http.Request) bool {
	c, err := r.Cookie(adminCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	exp, ok := h.sessions[c.Value]
	if !ok || exp <= time.Now().Unix() {
		delete(h.sessions, c.Value)
		return false
	}
	return true
}

// AdminPage renders either the admin login form or the admin dashboard.
func (h *AdminHandler) AdminPage(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		h.View.Render(w, "admin.html", map[string]any{
			"Title":      "정부통제실",
			"LoggedIn":   CurrentUserID(r) != "",
			"AdminAuthed": false,
		})
		return
	}

	gs, _ := h.Engine.GetGameState(r.Context())
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, first_name, last_name, class_name, status, area_id, hp, max_hp, level, kills
		 FROM users ORDER BY status ASC, id ASC`,
	)
	var users []model.User
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var u model.User
			if rows.Scan(&u.ID, &u.FirstName, &u.LastName, &u.ClassName, &u.Status, &u.AreaID, &u.HP, &u.MaxHP, &u.Level, &u.Kills) == nil {
				users = append(users, u)
			}
		}
	}

	h.View.Render(w, "admin.html", map[string]any{
		"Title":       "정부통제실",
		"LoggedIn":    CurrentUserID(r) != "",
		"AdminAuthed": true,
		"GameState":   gs,
		"Users":       users,
		"Flash":       popAdminFlash(w, r),
	})
}

// AdminLogin verifies BR_ADMIN_PASS and issues an admin session cookie.
func (h *AdminHandler) AdminLogin(w http.ResponseWriter, r *http.Request) {
	pass := r.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(pass), []byte(h.Cfg.AdminPass)) != 1 {
		h.View.Render(w, "admin.html", map[string]any{
			"Title":       "정부통제실",
			"LoggedIn":    CurrentUserID(r) != "",
			"AdminAuthed": false,
			"Error":       "관리자 비밀번호가 올바르지 않습니다.",
		})
		return
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(buf)
	exp := time.Now().Add(12 * time.Hour).Unix()

	h.mu.Lock()
	h.sessions[token] = exp
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    token,
		Path:     "/admin",
		Expires:  time.Unix(exp, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// ResetRound resets the entire round (game_state, area_items, users, NPCs, news_logs).
func (h *AdminHandler) ResetRound(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err := db.ResetRound(r.Context(), h.DB, h.Master, h.Cfg); err != nil {
		setAdminFlashAndRedirect(w, r, "라운드 초기화 실패: "+err.Error())
		return
	}
	setAdminFlashAndRedirect(w, r, "라운드를 초기화했습니다. (NPC 및 지역 아이템 재배치 완료)")
}

// AdvanceForbiddenZones immediately triggers the next forbidden zone expansion.
func (h *AdminHandler) AdvanceForbiddenZones(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `UPDATE game_state SET next_area_at = ? WHERE id = 1`, time.Now().Unix()-1); err != nil {
		setAdminFlashAndRedirect(w, r, "오류: "+err.Error())
		return
	}
	if err := h.Engine.CheckForbiddenZones(r.Context()); err != nil {
		setAdminFlashAndRedirect(w, r, "금지구역 확장 실패: "+err.Error())
		return
	}
	setAdminFlashAndRedirect(w, r, "금지구역을 즉시 확장했습니다.")
}

// DeleteUser removes a player or NPC from the database.
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	uid := r.FormValue("user_id")
	if uid == "" {
		setAdminFlashAndRedirect(w, r, "삭제할 유저 ID가 없습니다.")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM users WHERE id = ?`, uid); err != nil {
		setAdminFlashAndRedirect(w, r, "유저 삭제 실패: "+err.Error())
		return
	}
	setAdminFlashAndRedirect(w, r, fmt.Sprintf("유저(%s)를 삭제했습니다.", uid))
}

// DownloadBackup creates a consistent SQLite snapshot via VACUUM INTO and streams it.
func (h *AdminHandler) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("br_backup_%d.db", time.Now().UnixNano()))
	defer os.Remove(tmpPath)

	if _, err := h.DB.ExecContext(r.Context(), fmt.Sprintf("VACUUM INTO '%s'", tmpPath)); err != nil {
		http.Error(w, "백업 생성 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", `attachment; filename="battle_royale_backup.db"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, tmpPath)
}

func setAdminFlashAndRedirect(w http.ResponseWriter, r *http.Request, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "br_admin_flash",
		Value:    url.QueryEscape(msg),
		Path:     "/admin",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func popAdminFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie("br_admin_flash")
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "br_admin_flash",
		Value:    "",
		Path:     "/admin",
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
