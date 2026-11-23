package config

import (
	"os"
	"strconv"
)

// AreaTrait represents the special modifier of an area.
type AreaTrait string

const (
	TraitSearchUp   AreaTrait = "SU" // 발견증가
	TraitSearchDown AreaTrait = "SD" // 발견감소
	TraitDefUp      AreaTrait = "DU" // 방어증가
	TraitDefDown    AreaTrait = "DD" // 방어감소
	TraitAttUp      AreaTrait = "AU" // 공격증가
	TraitAttDown    AreaTrait = "AD" // 공격감소
)

// AreaInfo defines static metadata for one of the 22 map areas (from br.pl).
type AreaInfo struct {
	ID    int
	Name  string
	Code  string
	Trait AreaTrait
	Info  string
}

// IconInfo defines a selectable character icon.
type IconInfo struct {
	ID   int
	File string
	Name string
	Sex  string // "남성" or "여성"
}

// Config holds runtime server configuration and game constants.
type Config struct {
	Port      string
	DBPath    string
	DataDir   string
	StaticDir string
	AdminPass string
	AdminID   string
}

// Load reads runtime configuration from environment variables with safe defaults.
func Load() Config {
	return Config{
		Port:      getEnv("BR_PORT", "8080"),
		DBPath:    getEnv("BR_DB_PATH", "./battle_royale.db"),
		DataDir:   getEnv("BR_DATA_DIR", "./data"),
		StaticDir: getEnv("BR_STATIC_DIR", "./static"),
		AdminPass: getEnv("BR_ADMIN_PASS", "1234"),
		AdminID:   getEnv("BR_ADMIN_ID", "npc"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Game constants migrated from br.pl.
const (
	GameVersion = "V01.15-Go"
	GameTitle   = "Battle Royale - Edited by Ruria"

	ClassCount     = 2  // $clmax
	MaxPerSex      = 21 // $manmax
	MaxMembers     = ClassCount * MaxPerSex * 2
	SaveLimitDays  = 7
	BaseExp        = 16
	LvExpInc       = 4
	BaseProf       = 20
	BattleMinDays  = 2
	RegLimitDays   = 4
	MaxStamina     = 300
	FirstAidSta    = 30
	PoisonCheckSta = 10
	RecoverTimeSec = 30 // 30초당 스태미나 +1
	RecoverHPRatio = 3  // 체력 회복 = 스태미나 회복량 / 3

	IconFemaleStart = 21 // $icon_check: 여자 아이콘 시작 인덱스
)

// Classes lists available school classes.
var Classes = []string{
	"3학년A반", "3학년B반", "3학년C반", "3학년D반", "3학년E반",
	"3학년F반", "3학년G반", "3학년H반", "3학년I반", "3학년J반",
}

// Areas defines all 22 regions from br.pl.
var Areas = []AreaInfo{
	{0, "분교", "D-6", TraitSearchUp, "분교다. 곧 여기는 금지지역이 되겠구나.<br>빨리 이동하지 않으면, 목걸이가 폭발하고 만다..."},
	{1, "북쪽 곶", "A-2", TraitDefDown, "바다에는 배가 보인다. 탈출하려는 학생들을 감시하고 있는 정부의 배인가..."},
	{2, "대포리", "B-4", TraitDefUp, "전에는 여기에서도 사람이 살고 있었겠지. 지금은 폐허로 변했구나."},
	{3, "면사무소", "C-3", TraitSearchUp, "여기가 마을의 중심인가. 지금으로서는 아무도 없는가..."},
	{4, "우체국", "C-4", TraitSearchDown, "여기서는, 눈에 띄는 것이 없는데..."},
	{5, "소방서", "C-5", TraitSearchUp, "소방서라곤 해도, 소방차조차 없구나."},
	{6, "관음당", "C-6", TraitAttUp, "크고 작은 여러가지 불상이 모셔져 있구나. 밤에는 으시시하겠네."},
	{7, "맑은 연못", "D-4", TraitSearchUp, "맑은 물이 가득 차 있구나."},
	{8, "서광사", "E-2", TraitSearchDown, "오래된 불상이 모셔져 있는 절이구나."},
	{9, "호텔 자리", "E-4", TraitAttDown, "이런 장소는 유령이 자주 나올까? 유령같은건 믿지 않지만."},
	{10, "터널", "E-7", TraitDefDown, "깜깜하구나. 이런 곳에서 협공당하면 국물도 없겠군."},
	{11, "서포리", "F-2", TraitDefUp, "여기도, 다른 마을과 같이, 모두 폐허로 변했구나..."},
	{12, "산악지대", "F-5", TraitSearchUp, "이 섬을 한눈에 볼 수 있는 고지대다. 당연히 이곳에 서 있으면, 다른 곳에서도 날 발견할 가능성이 높을 것이다."},
	{13, "무연사", "F-8", TraitSearchDown, "모두 아주 황폐해져 있구나."},
	{14, "학교", "G-3", TraitAttDown, "낮동안의 학교라는 건 좋은 것이지만, 밤의 학교라는 것은 좋은 것이 아니군."},
	{15, "남해암", "G-6", TraitSearchDown, "사찰 처마도 썩어서 형편없는 모습을 드러내고 있구나."},
	{16, "삼림지대", "H-4", TraitSearchDown, "나무들이 울창하게 자라 있구나. 풀숲에서 갑자기 습격하더라도 알아채지 못할거야..."},
	{17, "검은 연못", "H-6", TraitSearchDown, "여기는 연못이라기보다도, 웅덩이구나. 정말 기분 나쁜 곳이야..."},
	{18, "남포리", "I-6", TraitAttUp, "여기는, 다른 마을보다, 가게가 많구나. 상점가인가 뭔가였겠지."},
	{19, "요양소", "I-7", TraitSearchUp, "황폐해져 있지만, 찾아보면 아직 사용할 수 있는 약 같은 것이 있을 것 같다..."},
	{20, "등대", "I-10", TraitDefUp, "요새로서 점거하면, 견고한 요새가 될듯한데. 바닥에는 말라붙은 혈흔이 많이 있구나. 무슨 일이 있었던거지?"},
	{21, "남쪽 곶", "J-6", TraitSearchUp, "병사의 배와... 우승자가 탈 배인가? 몇척의 배가 떠 있다."},
}

// Icons returns the 42 selectable student icons (0..20 male, 21..41 female).
func Icons() []IconInfo {
	icons := make([]IconInfo, 42)
	for i := 0; i < 42; i++ {
		sex := "남성"
		label := "남자 " + strconv.Itoa(i+1) + "번"
		if i >= IconFemaleStart {
			sex = "여성"
			label = "여자 " + strconv.Itoa(i-IconFemaleStart+1) + "번"
		}
		icons[i] = IconInfo{
			ID:   i,
			File: strconv.Itoa(i) + ".jpg",
			Name: label,
			Sex:  sex,
		}
	}
	return icons
}

// GridCode returns "A-1".."J-10" for a 0..99 grid position.
func GridCode(pos int) string {
	if pos < 0 || pos >= 100 {
		return "-"
	}
	r, c := pos/10, pos%10
	return string(rune('A'+r)) + "-" + strconv.Itoa(c+1)
}

// GridPosFromCode converts "A-1".."J-10" to a 0..99 grid position.
func GridPosFromCode(code string) int {
	var r, c int
	if len(code) < 3 || code[1] != '-' {
		return 0
	}
	r = int(code[0] - 'A')
	v, err := strconv.Atoi(code[2:])
	if err != nil {
		return 0
	}
	c = v - 1
	if r < 0 || r >= 10 || c < 0 || c >= 10 {
		return 0
	}
	return r*10 + c
}

// NamedAreaByGridPos returns the AreaInfo if pos (0..99) has a named location, else nil.
func NamedAreaByGridPos(pos int) *AreaInfo {
	for i := range Areas {
		if GridPosFromCode(Areas[i].Code) == pos {
			return &Areas[i]
		}
	}
	return nil
}

// GridPosByAreaID converts a legacy 0..21 area ID into its 0..99 grid position.
func GridPosByAreaID(areaID int) int {
	if areaID >= 0 && areaID < len(Areas) {
		return GridPosFromCode(Areas[areaID].Code)
	}
	return 0
}

var unnamedAreaNames = func() map[int]string {
	raw := map[string]string{
		"B-2":  "북쪽 곶 남쪽 길",
		"B-3":  "대포리 서쪽 갈림길",
		"B-5":  "대포리 동쪽 해안길",
		"C-1":  "북서쪽 해안 절벽",
		"C-2":  "면사무소 서쪽 해안길",
		"C-7":  "관음당 동쪽 해안길",
		"D-1":  "서쪽 해안 절벽",
		"D-2":  "면사무소 남서쪽 길",
		"D-3":  "면사무소 남쪽 길",
		"D-5":  "분교 서쪽 길",
		"D-7":  "분교 동쪽 구불길",
		"D-8":  "북동쪽 해안 갈림길",
		"D-9":  "북동쪽 해안가",
		"E-1":  "서광사 서쪽 해안",
		"E-3":  "호텔 자리 서쪽 갈림길",
		"E-5":  "산악지대 북쪽 사면",
		"E-6":  "분교 남쪽 산길",
		"E-8":  "터널 동쪽 갈림길",
		"E-9":  "동쪽 해안 언덕",
		"F-1":  "서포리 서쪽 해안",
		"F-3":  "서포리 동쪽 갈림길",
		"F-4":  "산악지대 서쪽 사면",
		"F-6":  "산악지대 동쪽 사면",
		"F-7":  "무연사 서쪽 숲길",
		"F-9":  "무연사 동쪽 해안길",
		"G-1":  "남서쪽 해안 절벽",
		"G-2":  "학교 서쪽 해안길",
		"G-4":  "학교 동쪽 갈림길",
		"G-5":  "산악지대 남쪽 사면",
		"G-7":  "남해암 동쪽 삼거리",
		"G-8":  "무연사 남쪽 길",
		"G-9":  "동쪽 해안가 길",
		"H-2":  "남서쪽 해안 내리막길",
		"H-3":  "삼림지대 서쪽 갈림길",
		"H-5":  "삼림지대 동쪽 숲길",
		"H-7":  "검은 연못 동쪽 길",
		"H-8":  "남동쪽 내륙 길",
		"H-9":  "남동쪽 해안 갈림길",
		"H-10": "등대 북쪽 해안가",
		"I-3":  "남서쪽 해안 갯바위",
		"I-4":  "삼림지대 남쪽 해안길",
		"I-5":  "남포리 서쪽 갈림길",
		"I-8":  "요양소 동쪽 해안길",
		"I-9":  "등대 서쪽 길",
		"J-7":  "요양소 남쪽 해안",
	}
	m := make(map[int]string, len(raw))
	for code, name := range raw {
		m[GridPosFromCode(code)] = name
	}
	return m
}()

// AreaNameAndCode returns the display name and grid code for any 0..99 position.
func AreaNameAndCode(pos int) (string, string, string) {
	if a := NamedAreaByGridPos(pos); a != nil {
		return a.Name, a.Code, a.Info
	}
	if name, ok := unnamedAreaNames[pos]; ok {
		return name, GridCode(pos), "특별한 건물이 없는 섬의 길목이다. 주위를 살피며 이동하자."
	}
	return "섬 외곽 숲길", GridCode(pos), "특별한 건물이 없는 평범한 섬의 숲길이다."
}

var oceanSet = func() map[int]bool {
	codes := []string{
		"A-1", "A-3", "A-4", "A-5", "A-6", "A-7", "A-8", "A-9", "A-10",
		"B-1", "B-6", "B-7", "B-8", "B-9", "B-10",
		"C-8", "C-9", "C-10",
		"D-10", "E-10", "F-10", "G-10",
		"H-1",
		"I-1", "I-2",
		"J-1", "J-2", "J-3", "J-4", "J-5", "J-8", "J-9", "J-10",
	}
	m := make(map[int]bool, len(codes))
	for _, c := range codes {
		m[GridPosFromCode(c)] = true
	}
	return m
}()

// IsOcean reports whether the 0..99 grid position is impassable ocean.
func IsOcean(pos int) bool {
	return oceanSet[pos]
}
