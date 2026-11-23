# 02. Go + SQLite 현대화 아키텍처 설계

---

## 1. 기술 스택 선정

* **언어 / 런타임**: Go (표준 라이브러리 `net/http`, `html/template` 중심, 필요 시 `chi` 등 경량 라우터 사용)
* **데이터베이스**: **SQLite** (`modernc.org/sqlite` — Pure Go 드라이버 권장)
  * `CGO_ENABLED=0` 크로스 컴파일이 가능해 로컬(macOS/Windows/Linux)에서 오라클 VPS(`linux/amd64` 또는 `linux/arm64`)용 단일 바이너리를 쉽게 빌드할 수 있음.
  * **WAL 모드 (`PRAGMA journal_mode=WAL;`)** 및 `busy_timeout` 설정으로 동시 읽기 성능 확보.
* **정적 에셋 임베딩**: Go `embed` 패키지로 HTML 템플릿과 `static/img/`(58개 이미지), CSS를 바이너리에 포함하거나 외부 `static/` 폴더로 서빙.

---

## 2. 권장 프로젝트 디렉토리 구조

```text
battle-royale/
├── cmd/
│   └── server/
│       └── main.go              # 서버 진입점, DB 초기화, 스케줄러, Graceful Shutdown
├── internal/
│   ├── config/
│   │   └── config.go            # 환경변수 및 게임 상수 (br.pl 설정 이관)
│   ├── db/
│   │   ├── sqlite.go            # SQLite 연결, WAL/PRAGMA 설정, 마이그레이션
│   │   └── seed.go              # dat/*.json(또는 UTF-8 CSV) 초기 데이터 로드
│   ├── model/
│   │   ├── user.go              # User 구조체 (70+ 필드 정규화)
│   │   ├── item.go              # Item, Recipe, AreaItem 구조체
│   │   └── game.go              # GameState, Area, NewsLog, Winner 구조체
│   ├── game/
│   │   ├── engine.go            # 게임 틱, 금지구역 추가, 스태미나/체력 회복 계산
│   │   ├── battle.go            # 전투, 선제/반격, 무기/방어구 내구도, 숙련도, 사망 처리
│   │   ├── explore.go           # 이동, 탐색, 아이템 습득, 이벤트, 함정 발동
│   │   ├── item.go              # 아이템 사용, 장착, 합성(조합), 독/해독, 레이더, 해킹
│   │   └── ending.go            # 최후 생존, 해킹/해제키 탈출, 우승자 기록
│   ├── handler/
│   │   ├── middleware.go        # 세션 인증, 패닉 복구, IP 차단, 로깅
│   │   ├── auth.go              # 로그인, 캐릭터 등록, 로그아웃
│   │   ├── play.go              # 메인 게임 화면, 행동 커맨드, 맵, 뉴스, 랭킹
│   │   └── admin.go             # 게임 리셋, 유저 관리, 백업/복구
│   └── view/
│       └── renderer.go          # html/template 렌더링 헬퍼
├── templates/                   # HTML 템플릿 (layout, battle, regist, map, news, admin 등)
├── static/
│   ├── css/br.css
│   └── img/                     # 763759eb50e844bcba8/img/ 복사본 (58개)
├── data/                        # UTF-8 변환된 마스터 데이터 (base, gousei_tbl, itemfile, stitemfile, wepfile)
└── docs/                        # 설계 문서
```

---

## 3. 레거시 대체 구조 매핑

| 레거시 (Perl CGI) | Go + SQLite 대체 구조 | 기대 효과 |
| --- | --- | --- |
| 매 요청 `perl` 프로세스 실행 | 상주 Go HTTP 서버 (Goroutine) | CPU/메모리 오버헤드 수십 배 감소 |
| `mkdir ./lock/lock` 파일 락 | SQLite 트랜잭션 + 앱 레벨 `sync.Mutex` (게임 턴/상태 임계영역) | 데드락·아이템 복사·데이터 손실 원천 차단 |
| `userdatfile.cgi` (70+ CSV) | `users` + `user_items` 테이블 | 행 단위 갱신, 인덱스 파싱 실수 제거 |
| `0~21itemfile.cgi` (22개 파일) | `area_items` 테이블 (`area_id` 인덱싱) | 파일 I/O 22개 → 단일 쿼리 |
| `areafile.cgi`, `timefile.cgi`, `e_flag.txt` | `game_state` + `areas` 테이블 | 원자적 상태 관리 |
| `pref.cgi` Lazy 금지구역 체크 | Go 백그라운드 `time.Ticker` + 요청 시 보정 | 정확한 스케줄링 및 동시성 안전 |
| 평문 비밀번호 / 쿠키 변조 위험 | `bcrypt` 해싱 + `HttpOnly` 서명 세션 쿠키 | 계정 탈취 및 폼 변조 방지 |
| `http://www.ruria.pe.kr/brimg/` | `/static/img/...` 로컬 서빙 | 깨진 이미지 복구 |

---

## 4. SQLite 스키마 초안

```sql
-- 게임 라운드 전역 상태 (단일 행 id=1)
CREATE TABLE IF NOT EXISTS game_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    status TEXT NOT NULL DEFAULT 'running', -- 'running', 'ended'
    started_at INTEGER NOT NULL,
    next_area_at INTEGER NOT NULL,
    forbidden_count INTEGER NOT NULL DEFAULT 1,
    hack_flag INTEGER NOT NULL DEFAULT 0,
    area_order TEXT NOT NULL                -- JSON 배열: 셔플된 22개 지역 ID 순서
);

-- 유저 및 NPC (기존 userdatfile.cgi 정규화)
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    sex TEXT NOT NULL,
    class_name TEXT NOT NULL,
    student_no INTEGER NOT NULL,
    club TEXT NOT NULL DEFAULT '',
    icon INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'ALIVE',   -- 'ALIVE', 'DEAD', 'NPC0'
    tactics TEXT NOT NULL DEFAULT '보통',
    area_id INTEGER NOT NULL DEFAULT 0,
    att INTEGER NOT NULL,
    def INTEGER NOT NULL,
    hp INTEGER NOT NULL,
    max_hp INTEGER NOT NULL,
    level INTEGER NOT NULL DEFAULT 1,
    exp INTEGER NOT NULL DEFAULT 0,
    stamina INTEGER NOT NULL DEFAULT 300,
    kills INTEGER NOT NULL DEFAULT 0,
    last_action_at INTEGER NOT NULL,        -- 스태미나/체력 회복 계산 기준 시각
    -- 장비 (무기, 몸, 머리, 발, 팔)
    weapon_name TEXT NOT NULL DEFAULT '맨손',
    weapon_kind TEXT NOT NULL DEFAULT 'WN',
    weapon_att INTEGER NOT NULL DEFAULT 0,
    weapon_dur INTEGER NOT NULL DEFAULT 0,
    armor_body TEXT NOT NULL DEFAULT '없음', armor_body_def INTEGER NOT NULL DEFAULT 0, armor_body_dur INTEGER NOT NULL DEFAULT 0,
    armor_head TEXT NOT NULL DEFAULT '없음', armor_head_def INTEGER NOT NULL DEFAULT 0, armor_head_dur INTEGER NOT NULL DEFAULT 0,
    armor_foot TEXT NOT NULL DEFAULT '없음', armor_foot_def INTEGER NOT NULL DEFAULT 0, armor_foot_dur INTEGER NOT NULL DEFAULT 0,
    armor_arm  TEXT NOT NULL DEFAULT '없음', armor_arm_def  INTEGER NOT NULL DEFAULT 0, armor_arm_dur  INTEGER NOT NULL DEFAULT 0,
    -- 숙련도 10종 (wn, wp, wa, wg, we, wc, wd, wb, wf, ws) JSON 또는 개별 컬럼
    proficiency_json TEXT NOT NULL,
    battle_log TEXT NOT NULL DEFAULT '',
    death_msg TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    last_target_id TEXT NOT NULL DEFAULT '',
    poison_flag TEXT NOT NULL DEFAULT ''
);

-- 유저 소지품 (최대 6슬롯)
CREATE TABLE IF NOT EXISTS user_items (
    user_id TEXT NOT NULL,
    slot INTEGER NOT NULL CHECK (slot BETWEEN 0 AND 5),
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    effect INTEGER NOT NULL,
    durability INTEGER NOT NULL,
    PRIMARY KEY (user_id, slot),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 지역별 드랍 아이템 / 설치 함정 (기존 0~21itemfile.cgi 통합)
CREATE TABLE IF NOT EXISTS area_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    area_id INTEGER NOT NULL,
    spawn_group INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    effect INTEGER NOT NULL,
    durability INTEGER NOT NULL,
    owner_id TEXT NOT NULL DEFAULT ''       -- 함정 설치자 ID (있을 경우)
);
CREATE INDEX IF NOT EXISTS idx_area_items_area ON area_items(area_id);

-- 뉴스 로그 / 총성 로그 / 역대 우승자
CREATE TABLE IF NOT EXISTS news_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER NOT NULL,
    kind TEXT NOT NULL,
    message TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS winners (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_ended_at INTEGER NOT NULL,
    winner_name TEXT NOT NULL,
    ending_type TEXT NOT NULL,
    kills INTEGER NOT NULL,
    summary_json TEXT NOT NULL
);
```

---

## 5. 추가로 챙겨야 할 설계 사항

1. **동시성 전략 (중요)**
   * SQLite는 단일 Writer 모델이므로, `DB.SetMaxOpenConns(1)` 또는 Write 트랜잭션을 직렬화하는 구조가 안전하며, 배틀로얄처럼 라운드당 동시 접속자가 수백 명 수준이면 처리 시간이 마이크로초~밀리초 단위라 병목이 전혀 발생하지 않습니다.
   * 두 유저가 동시에 같은 지역 아이템을 줍거나 동시에 서로를 공격할 때 **DB 트랜잭션(`BEGIN IMMEDIATE`)** 안에서 현재 상태(`hp > 0`, 아이템 존재 여부)를 재확인하도록 작성해 아이템 복사·중복 킬 버그를 방지합니다.
2. **스태미나 및 체력 회복 공식**
   * `br.pl` 기준: 30초마다 스태미나 +1 (`$kaifuku_time = 30`), 체력 회복은 스태미나 회복 비율(`$kaifuku_rate = 3`).
   * 매 요청 시 `last_action_at`과 현재 시각의 차이를 계산해 반영하고 `last_action_at`을 갱신합니다.
3. **세션 & 보안**
   * `regist.cgi`에서 유저 ID/PW 입력 시 `bcrypt`로 해싱 저장.
   * 로그인 성공 시 랜덤 세션 토큰을 생성해 `HttpOnly`, `SameSite=Lax` 쿠키로 발급 (인메모리 또는 `sessions` 테이블 저장).
   * 관리자 비밀번호(`$a_pass = "1234"`)는 절대 코드에 하드코딩하지 않고 **환경변수(`BR_ADMIN_PASS`)**로 주입받도록 변경.
4. **HTML 템플릿 현대화 (선택)**
   * 1차 포팅은 기존 Perl의 UI 레이아웃과 색상을 `html/template`로 최대한 그대로 재현해 호환성과 검증 속도를 높입니다.
   * 이후 CSS(`br.css`)에 모바일 반응형(`viewport`, `max-width`)만 추가해도 스마트폰 플레이가 가능해집니다.
