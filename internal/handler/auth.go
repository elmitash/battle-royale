package handler

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"battle-royale/internal/config"
	"battle-royale/internal/db"
	"battle-royale/internal/model"
	"battle-royale/internal/view"
)

// AuthHandler handles login, logout, and character registration.
type AuthHandler struct {
	DB     *sql.DB
	Master *db.MasterData
	View   *view.Renderer
}

// LoginPage renders the login screen or redirects to /battle if already logged in.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if CurrentUserID(r) != "" {
		http.Redirect(w, r, "/battle", http.StatusSeeOther)
		return
	}
	h.View.Render(w, "login.html", map[string]any{
		"Title":    "로그인",
		"LoggedIn": false,
	})
}

// LoginSubmit authenticates a user and sets an HttpOnly session cookie.
func (h *AuthHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	pw := r.FormValue("password")

	var pwHash string
	err := h.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id = ?", id).Scan(&pwHash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(pw)) != nil {
		h.View.Render(w, "login.html", map[string]any{
			"Title":    "로그인",
			"LoggedIn": false,
			"Error":    "ID 또는 비밀번호가 올바르지 않습니다.",
		})
		return
	}

	if err := h.issueSession(w, r, id); err != nil {
		http.Error(w, "세션 생성 실패", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/battle", http.StatusSeeOther)
}

// Logout clears the session from DB and cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		_, _ = h.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE token = ?", c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// RegistPage renders the new character registration form.
func (h *AuthHandler) RegistPage(w http.ResponseWriter, r *http.Request) {
	h.View.Render(w, "regist.html", map[string]any{
		"Title":    "전학수속",
		"LoggedIn": CurrentUserID(r) != "",
		"Icons":    config.Icons(),
	})
}

// RegistSubmit validates input, creates a new student with starter gear, and logs them in.
func (h *AuthHandler) RegistSubmit(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	pw := r.FormValue("password")
	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))
	sex := strings.TrimSpace(r.FormValue("sex"))
	icon, _ := strconv.Atoi(r.FormValue("icon"))

	renderErr := func(msg string) {
		h.View.Render(w, "regist.html", map[string]any{
			"Title":    "전학수속",
			"LoggedIn": false,
			"Icons":    config.Icons(),
			"Error":    msg,
		})
	}

	if id == "" || pw == "" || firstName == "" || lastName == "" {
		renderErr("모든 필수 항목을 입력해 주세요.")
		return
	}
	if sex != "남성" && sex != "여성" {
		renderErr("성별을 올바르게 선택해 주세요.")
		return
	}
	if icon < 0 || icon >= 42 {
		icon = 0
	}

	var exists int
	if err := h.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists); err != nil {
		renderErr("데이터베이스 오류가 발생했습니다.")
		return
	}
	if exists > 0 {
		renderErr("이미 사용 중인 ID입니다.")
		return
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		renderErr("비밀번호 처리 중 오류가 발생했습니다.")
		return
	}

	now := time.Now().Unix()
	att := rand.IntN(5) + 8
	def := rand.IntN(5) + 8
	maxHP := rand.IntN(20) + 80
	className := config.Classes[rand.IntN(config.ClassCount)]
	studentNo := rand.IntN(config.MaxPerSex) + 1
	clubs := []string{"검도부", "궁도부", "야구부", "육상부", "사격부", "요리부", "컴퓨터부", "귀가부"}
	club := clubs[rand.IntN(len(clubs))]

	wep := model.ItemSpec{Name: "맨손", Kind: "WN", Effect: 0, Durability: 0}
	if len(h.Master.Weapons) > 0 {
		wep = h.Master.Weapons[rand.IntN(len(h.Master.Weapons))]
	}
	profJSON, _ := json.Marshal(model.Proficiency{})

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		renderErr("트랜잭션 시작 실패")
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO users (
			id, password_hash, first_name, last_name, sex, class_name, student_no, club, icon,
			status, tactics, area_id, att, def, hp, max_hp, level, exp, stamina, kills, last_action_at,
			weapon_name, weapon_kind, weapon_att, weapon_dur, proficiency_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'ALIVE', '보통', 0, ?, ?, ?, ?, 1, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		id, string(pwHash), firstName, lastName, sex, className, studentNo, club, icon,
		att, def, maxHP, maxHP, config.BaseExp, config.MaxStamina, now,
		wep.Name, wep.Kind, wep.Effect, wep.Durability, string(profJSON),
	); err != nil {
		renderErr("캐릭터 생성에 실패했습니다.")
		return
	}

	// Give 2 random starter items + standard bread/water
	starterPool := h.Master.StartItems
	if len(starterPool) > 0 {
		for slot := 0; slot < 2; slot++ {
			it := starterPool[rand.IntN(len(starterPool))]
			if _, err := tx.ExecContext(r.Context(),
				`INSERT INTO user_items (user_id, slot, name, kind, effect, durability) VALUES (?, ?, ?, ?, ?, ?)`,
				id, slot, it.Name, it.Kind, it.Effect, it.Durability,
			); err != nil {
				renderErr("초기 소지품 지급 실패")
				return
			}
		}
	}

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO news_logs (created_at, kind, message) VALUES (?, 'REGIST', ?)`,
		now, firstName+lastName+" ("+className+") 학생이 프로그램에 참가했습니다.",
	); err != nil {
		renderErr("로그 기록 실패")
		return
	}

	if err := tx.Commit(); err != nil {
		renderErr("저장 실패")
		return
	}

	if err := h.issueSession(w, r, id); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/battle", http.StatusSeeOther)
}

func (h *AuthHandler) issueSession(w http.ResponseWriter, r *http.Request, userID string) error {
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(buf); err != nil {
		return err
	}
	token := hex.EncodeToString(buf)
	now := time.Now().Unix()
	expires := now + int64(config.SaveLimitDays*24*60*60)

	if _, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now, expires,
	); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expires, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}
