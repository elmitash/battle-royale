package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
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

// Load reads runtime configuration from .env (if present) and environment variables.
func Load() Config {
	loadDotEnv(".env")
	return Config{
		Port:      getEnv("BR_PORT", "8080"),
		DBPath:    getEnv("BR_DB_PATH", "./battle_royale.db"),
		DataDir:   getEnv("BR_DATA_DIR", "./data"),
		StaticDir: getEnv("BR_STATIC_DIR", "./static"),
		AdminPass: getEnv("BR_ADMIN_PASS", "1234"),
		AdminID:   getEnv("BR_ADMIN_ID", "npc"),
	}
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
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
	{0, "분교", "D-6", TraitSearchUp, "붉은 벽돌의 낡은 분교 건물이다. 복도 끝 교무실에서는 정체 모를 기척이 느껴진다.<br>이곳은 가장 먼저 금지구역으로 지정되는 곳이다. 오래 머무르지 말고 서둘러 빠져나가자."},
	{1, "북쪽 곶", "A-2", TraitDefDown, "깎아지른 절벽 너머로 차가운 북쪽 바다가 펼쳐진 북쪽 곶이다.<br>거센 바닷바람이 불어와 몸을 숨길 곳이 마땅치 않으니 주변을 경계하자."},
	{2, "대포리", "B-4", TraitDefUp, "과거 섬 북쪽의 중심지였을 대포리 마을이다. 지금은 주민들이 떠나고 빈집만 남아 있다.<br>골목마다 버려진 생활용품이나 쓸 만한 도구가 남아 있을지 모른다."},
	{3, "면사무소", "C-3", TraitSearchUp, "대포리 서쪽에 자리한 낡은 면사무소다. 텅 빈 민원실 책상 위로 먼지가 수북이 쌓여 있다.<br>창고나 서류함 사이를 뒤져보면 의외의 물자를 찾을 수 있을 것 같다."},
	{4, "우체국", "C-4", TraitSearchDown, "녹슨 우체통이 기울어진 채 서 있는 작은 우체국이다. 흩어진 우편물 사이로 적막만 흐른다.<br>문을 닫고 숨어 있기엔 좁지만, 잠시 숨을 고르기에는 괜찮아 보인다."},
	{5, "소방서", "C-5", TraitSearchUp, "셔터가 반쯤 내려앉은 섬의 소방서다. 소방차는 이미 사라지고 텅 빈 차고만 남아 있다.<br>구조 장비나 비상용 물품이 남아 있을 법해 구석구석 살펴볼 가치가 있다."},
	{6, "관음당", "C-6", TraitAttUp, "작은 돌계단 위에 세워진 관음당이다. 낡은 법당 안에는 크고 작은 불상들이 고요히 자리하고 있다.<br>향 냄새가 옅게 감도는 어두운 실내에서는 작은 발소리도 크게 울린다."},
	{7, "샘솟는 연못", "D-4", TraitSearchUp, "산기슭 아래 맑은 물이 솟아나는 샘솟는 연못이다. 수면 위로 주변 숲과 하늘이 잔잔하게 비친다.<br>탁 트인 물가라 사방이 잘 보이지만, 그만큼 다른 사람의 눈에도 띄기 쉽다."},
	{8, "서광사", "E-2", TraitSearchDown, "서쪽 해안 언덕에 자리한 고즈넉한 사찰 서광사다. 오래된 목조 건물과 돌탑이 세월을 말해준다.<br>경내의 나무들이 시야를 가려 은신하기에는 좋지만 기습에 주의해야 한다."},
	{9, "호텔 자리", "E-4", TraitAttDown, "영업을 중단한 지 오래된 호텔 자리다. 깨진 유리창 사이로 눅눅한 바람이 드나든다.<br>어두운 복도와 수많은 객실 중 어딘가에 누군가 숨어 있을지도 모른다."},
	{10, "터널", "E-7", TraitDefDown, "산 허리를 관통하는 어둡고 긴 터널이다. 안쪽에서는 천장에서 물방울 떨어지는 소리만 메아리친다.<br>한 치 앞도 분간하기 어려운 좁은 통로여서 마주치면 피하기 어렵다."},
	{11, "서포리", "F-2", TraitDefUp, "서쪽 바닷가를 따라 집들이 모여 있던 서포리 마을이다. 녹슨 어구와 빈 상자들이 골목에 널려 있다.<br>담벼락이 촘촘히 이어져 있어 골목을 돌아다닐 때 시야를 잘 살펴야 한다."},
	{12, "산악지대", "F-5", TraitSearchUp, "섬 한가운데 우뚝 솟은 산악지대다. 정상 부근에 오르면 섬 전체의 윤곽이 한눈에 들어온다.<br>높은 지형 덕분에 주변을 살피기 좋지만, 능선 위에서는 내 모습도 쉽게 노출된다."},
	{13, "무연사", "F-8", TraitSearchDown, "인적이 끊겨 잡초만 무성한 폐사찰 무연사다. 무너져 내린 기와지붕 아래로 서늘한 공기가 감돈다.<br>음산한 분위기 속에서 낡은 불당 뒤편에 무엇이 숨겨져 있을지 알 수 없다."},
	{14, "학교", "G-3", TraitAttDown, "운동장에 잡초가 무성하게 자란 폐교다. 깨진 교실 창문 너머로 낡은 책걸상들이 보인다.<br>넓은 복도와 여러 교실이 얽혀 있어 언제 어디서 누가 튀어나올지 모른다."},
	{15, "남해암", "G-6", TraitSearchDown, "산 남쪽 중턱에 위치한 작은 암자 남해암이다. 낡은 처마 밑으로 바람 소리가 낮게 스쳐 간다.<br>조용히 몸을 숨기기에는 알맞지만, 빠져나갈 길이 좁아 방심할 수 없다."},
	{16, "삼림지대", "H-4", TraitSearchDown, "하늘이 보이지 않을 정도로 빽빽하게 나무가 우거진 삼림지대다. 발밑에는 마른 낙엽이 두껍게 깔려 있다.<br>수풀에 몸을 숨기기는 쉽지만, 덤불 너머의 기척을 미리 알아채기는 어렵다."},
	{17, "까치늪", "H-6", TraitSearchDown, "습하고 탁한 물이 고여 있는 까치늪이다. 늪 주변으로 질척한 진흙과 갈대가 무성하게 자라 있다.<br>발이 빠지기 쉬워 움직임이 둔해질 수 있으니 발밑을 조심하며 이동해야 한다."},
	{18, "남포리", "I-6", TraitAttUp, "남쪽 해안을 따라 상점가가 늘어서 있던 남포리다. 부서진 간판과 빈 가게들이 과거의 흔적을 남기고 있다.<br>가게가 많았던 곳답게 식료품이나 잡화 같은 쓸 만한 물건이 남아 있을 법하다."},
	{19, "요양소", "I-7", TraitSearchUp, "바다가 내려다보이는 언덕에 세워진 오래된 요양소다. 하얀 외벽은 빛바래고 복도는 쥐 죽은 듯 고요하다.<br>약제실이나 병실을 뒤져보면 회복에 도움이 되는 의약품을 구할 수도 있다."},
	{20, "등대", "I-10", TraitDefUp, "섬 동남쪽 끝에 우뚝 선 하얀 등대다. 좁은 나선형 계단 위로는 거친 파도 소리가 들려온다.<br>사방이 막힌 견고한 구조라 방어에 유리하지만, 한 번 몰리면 도망치기 어렵다."},
	{21, "남쪽 곶", "J-6", TraitSearchUp, "섬의 최남단에 위치한 남쪽 곶이다. 검은 바위 너머 수평선 위로 감시선으로 보이는 배 몇 척이 떠 있다.<br>탁 트인 바닷바람을 맞으며 섬의 남쪽 끝자락에서 다음 행동을 고민하게 된다."},
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
		"H-7":  "까치늪 동쪽 길",
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
