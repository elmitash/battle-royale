package model

import (
	"fmt"
	"strings"
)

// DurabilityInfinite represents "∞" durability in the database.
const DurabilityInfinite = -1

// ItemSpec represents an item's name, kind code, effect value, and durability.
type ItemSpec struct {
	Name       string
	Kind       string
	Effect     int
	Durability int // DurabilityInfinite (-1) means "∞"
}

// FormatDurability returns "∞" for infinite durability or the numeric string.
func FormatDurability(dur int) string {
	if dur == DurabilityInfinite {
		return "∞"
	}
	return fmt.Sprintf("%d", dur)
}

// FormatItemKind converts internal item kind codes (HH, DH, Y, WGB, etc.) to Korean labels.
// Note: poisoned food (HD/SD) intentionally displays the same as safe food (HH/SH).
func FormatItemKind(kind string) string {
	switch {
	case kind == "":
		return "-"
	case strings.HasPrefix(kind, "HH"), strings.HasPrefix(kind, "HD"):
		return "체력회복"
	case strings.HasPrefix(kind, "SH"), strings.HasPrefix(kind, "SD"):
		return "스태미나"
	case strings.HasPrefix(kind, "WN"):
		return "무기(칼)"
	case strings.HasPrefix(kind, "WG"):
		return "무기(총)"
	case strings.HasPrefix(kind, "WA"):
		return "무기(활)"
	case strings.HasPrefix(kind, "WB"):
		return "무기(둔기)"
	case strings.HasPrefix(kind, "WC"):
		return "무기(투척)"
	case strings.HasPrefix(kind, "WD"):
		return "무기(폭탄)"
	case strings.HasPrefix(kind, "WS"):
		return "무기(창/검)"
	case strings.HasPrefix(kind, "W"):
		return "무기"
	case strings.HasPrefix(kind, "DB"):
		return "방어(몸)"
	case strings.HasPrefix(kind, "DH"):
		return "방어(머리)"
	case strings.HasPrefix(kind, "DF"):
		return "방어(다리)"
	case strings.HasPrefix(kind, "DA"):
		return "방어(팔)"
	case strings.HasPrefix(kind, "A"):
		return "장신구"
	case strings.HasPrefix(kind, "R"):
		return "레이더"
	case strings.HasPrefix(kind, "TN"), strings.HasPrefix(kind, "TO"):
		return "함정"
	case kind == "Y":
		return "도구/재료"
	default:
		return kind
	}
}

// ParseItemNameKind splits "이름<>종류" into (name, kind).
// If "<>" is absent, kind defaults to "".
func ParseItemNameKind(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "<>"); idx >= 0 {
		return raw[:idx], raw[idx+2:]
	}
	return raw, ""
}

// UserItem represents one of the 6 inventory slots (0..5) owned by a user.
type UserItem struct {
	UserID     string
	Slot       int
	Name       string
	Kind       string
	Effect     int
	Durability int
}

// AreaItem represents a dropped item or planted trap in an area.
type AreaItem struct {
	ID         int64
	AreaID     int
	SpawnGroup int
	Name       string
	Kind       string
	Effect     int
	Durability int
	OwnerID    string
}

// Recipe represents an item synthesis rule from gousei_tbl.cgi.
type Recipe struct {
	Ingredient1 string
	Ingredient2 string
	ResultName  string
	ResultKind  string
	Effect      int
	Durability  int
}
