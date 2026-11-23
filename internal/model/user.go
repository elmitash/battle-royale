package model

// Proficiency tracks the 10 weapon proficiency categories (from userdatfile.cgi).
type Proficiency struct {
	WN int `json:"wn"` // 날/단검 (Knife)
	WP int `json:"wp"` // 둔기/파쇄? (or Spear/etc.)
	WA int `json:"wa"` // 활/석궁 (Bow/Crossbow)
	WG int `json:"wg"` // 총기 (Gun)
	WE int `json:"we"` // 폭발물?
	WC int `json:"wc"` // 투척 (Throw)
	WD int `json:"wd"` // 폭탄/화기 (Bomb)
	WB int `json:"wb"` // 타격/둔기 (Blunt)
	WF int `json:"wf"` // ?
	WS int `json:"ws"` // 창/검 (Spear/Sword)
}

// User represents a player or NPC in the game.
type User struct {
	ID           string
	PasswordHash string
	FirstName    string
	LastName     string
	Sex          string
	ClassName    string
	StudentNo    int
	Club         string
	Icon         int
	Status       string // "ALIVE", "DEAD", "NPC0", "NPC1", etc.
	Tactics      string
	AreaID       int
	Att          int
	Def          int
	HP           int
	MaxHP        int
	Level        int
	Exp          int
	Stamina      int
	Kills        int
	LastActionAt int64

	// Equipped weapon
	WeaponName string
	WeaponKind string
	WeaponAtt  int
	WeaponDur  int

	// Equipped armor (Body, Head, Foot, Arm)
	ArmorBody    string
	ArmorBodyDef int
	ArmorBodyDur int
	ArmorHead    string
	ArmorHeadDef int
	ArmorHeadDur int
	ArmorFoot    string
	ArmorFootDef int
	ArmorFootDur int
	ArmorArm     string
	ArmorArmDef  int
	ArmorArmDur  int

	Proficiency  Proficiency
	BattleLog    string
	DeathMsg     string
	Message      string
	LastTargetID string
	PoisonFlag   string

	Items []UserItem
}

// FullName returns the formatted Korean full name.
func (u *User) FullName() string {
	return u.FirstName + u.LastName
}
