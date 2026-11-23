# 01. 레거시 Perl CGI 소스 및 데이터 분석

원본 소스 경로: `/home/elmilin/다운로드/Br1.15Ruria/` (총 6,219줄, CP949 / CRLF)

---

## 1. 파일별 구조 및 서브루틴 매핑

### 1.1 루트 설정 및 진입점 (`*.cgi`, `br.pl`)

| 파일 | 줄 수 | 주요 역할 및 서브루틴 |
| --- | ---: | --- |
| `br.pl` | 264 | 전역 설정(관리자 ID/PW, 파일 경로, 22개 지역명/좌표/특성/설명, 반/정원, 레벨업/숙련도/회복 상수, NPC 설정, 아이콘 배열) |
| `pref.cgi` | 102 | 공통 전처리: IP 차단(`@kick`/`@oklist`), `areafile.cgi` 및 `e_flag.txt` 로드, **현재 시각 비교 후 금지구역 추가 및 해당 지역 생존자 사망 처리** |
| `battle.cgi` | 622 | 메인 게임 루프: `MAIN`, `COM`(커맨드 분기), `MOVE`, `SEARCH`, `SEARCH2`, `HEAL`, `INN`, `WINGET`, `WINCHG`, `KOUDOU`, `DEATHGET`, `WEPPNT`, `DEFCHK`, `OUKYU` |
| `attack.cgi` | 861 | 전투 시스템: `ATTACK`, `ATTACK1`, `ATTACK2`, `WEPTREAT`, `DEATH`, `DEATH2`, `BATTLE`, `RUNAWAY`, `DEFTREAT`, `LVUPCHK`, `BLOG_CK` |
| `regist.cgi` | 555 | 캐릭터 등록 및 초기 세팅: `MAIN`, `REGIST`, `INFO`, `CLUBMAKE` |
| `map.cgi` | 375 | 맵 현황 및 이동 UI |
| `admin.cgi` | 361 | 관리자 모드: `MAIN`, `MENU`, `BACKSAVE`, `BACKREAD`, `DATARESET`, `ADMDECODE`, `USERLIST`, `USERDEL` |
| `winner.cgi` | 192 | 역대 우승자/생존자 목록 |
| `news.cgi` | 131 | 진행 뉴스/사망자 로그 조회 |
| `login.cgi` | 62 | 로그인 폼 처리 |
| `icon.cgi` | 44 | 캐릭터 아이콘 미리보기 |
| `rank.cgi` | 37 | 랭킹/생존자 통계 |

### 1.2 핵심 라이브러리 (`lib/*.cgi`)

| 파일 | 줄 수 | 주요 역할 및 서브루틴 |
| --- | ---: | --- |
| `lib/lib2.cgi` | 979 | UI HTML 렌더링 및 핵심 상태 저장: `IDCHK`, `STS`, `DECODE`, `COMMAND`, `SAVE`, `SAVE2`, `CREAD`, `CSAVE`, `TACTGET`, `TACTGET2`, `DRAIN`, `u_save` |
| `lib/lib.cgi` | 313 | 공통 유틸/락/로그: `GetHostName`, `HEADER`, `FOOTER`, `LOGSAVE`, `LOCK`, `UNLOCK`, `COLOR`, `CLOCK` |
| `lib/item.cgi` | 356 | 아이템 사용/장착/폐기: `ITEMGET`, `ITEM`, `ITEMDEL`, `WEPDEL`, `WEPDEL2` |
| `lib/ending.cgi` | 298 | 게임 종료 처리(최후 1인/해킹/해제키): `ENDING`, `END_FOOT`, `ENDING2`, `EX_ENDING`, `EX_ENDING2`, `SURVIVORSAVE` |
| `lib/reader.cgi` | 242 | 레이더 기능: `READER` |
| `lib/event.cgi` | 112 | 랜덤 이벤트 처리: `EVENT` |
| `lib/itemgou.cgi` | 111 | 아이템 합성(조합): `ITEMGOUSEI`, `ITEMCOUNT` |
| `lib/hack.cgi` | 61 | 분교 해킹(프로그램 강제 종료): `HACKING` |
| `lib/itemsei.cgi` | 61 | 아이템 정리(합치기 등): `ITEMSEIRI` |
| `lib/poison.cgi` | 58 | 독약/독미 처리: `POISON`, `PSCHECK` |
| `lib/speaker.cgi` | 22 | 확성기 메시지: `SPEAKER` |

---

## 2. 데이터 저장 구조 (플랫 파일 DB)

### 2.1 런타임 상태 파일 (`log/`, `e_flag.txt`)
* **`log/userdatfile.cgi`**: 유저/NPC 1명당 1줄의 쉼표(`,`) 구분 CSV 레코드. 총 70개 이상의 필드를 순서대로 `split(/,/)`으로 파싱합니다.
  * 기본: `id, password, f_name, l_name, sex, cl, no, endtime`
  * 능력치: `att, def, hit, mhit, level, exp, sta`
  * 장비(이름<>종류, 효과, 내구): `wep, watt, wtai`, `bou, bdef, btai`, `bou_h, bdef_h, btai_h`, `bou_f, bdef_f, btai_f`, `bou_a, bdef_a, btai_a`
  * 상태: `tactics, death, msg, sts, pls, kill, icon`
  * 소지품 6슬롯: `item[0..5], eff[0..5], itai[0..5]`
  * 기타/숙련도: `log, dmes, bid, club`, 숙련도 10종(`wn, wp, wa, wg, we, wc, wd, wb, wf, ws`), `com, inf`
* **`log/0itemfile.cgi` ~ `log/21itemfile.cgi`**: 22개 지역별로 바닥에 드랍된 아이템 및 설치된 함정 목록 (`1,쥐덫<>TN,10,1,`).
* **`log/areafile.cgi`**: 5줄 구조 (1행: 다음 금지구역 추가 시각 `YYYY,M,D,HH,MM`, 2행: 현재 금지구역 수 및 해킹 플래그, 3~5행: 셔플된 지역명/좌표/인덱스).
* **`log/timefile.cgi`**: 게임 시작/진행 타임스탬프.
* **`log/newsfile.cgi` / `log/gunlog.cgi` / `log/survivor.cgi` / `log/memberfile.cgi`**: 뉴스, 총성, 역대 우승자, 접속/멤버 기록.
* **`e_flag.txt`**: 게임 종료 상태 플래그 (`종료` 문자열 포함 여부로 체크).

### 2.2 마스터 데이터 (`dat/`) — 실운영 에셋 비교

실제 운영에 사용된 `/home/elmilin/다운로드/763759eb50e844bcba8/dat/`는 원본(`Br1.15Ruria/dat/`) 대비 **커스텀 확장**되어 있습니다.

| 파일 | 원본 크기 | 실운영 크기 | 내용 |
| --- | ---: | ---: | --- |
| `base.cgi` | 379 B | 378 B | NPC(`담임`, `병사`) 초기 데이터 |
| `gousei_tbl.cgi` | 1,276 B | **10,344 B** | 아이템 합성 테이블 (`재료1<>재료2<>결과<>종류<>효과<>개수/내구`) |
| `itemfile.cgi` | 23,428 B | **26,005 B** | 지역별 초기 배치 아이템 테이블 |
| `stitemfile.cgi` | 404 B | **618 B** | 캐릭터 등록 시 랜덤 지급 소지품 |
| `wepfile.cgi` | 1,026 B | **1,218 B** | 캐릭터 등록 시 랜덤 지급 무기/방어구 |

### 2.3 이미지 에셋 (`img/`)
* `/home/elmilin/다운로드/763759eb50e844bcba8/img/`에 총 58개 파일 존재 (`map.jpg` + 캐릭터/NPC 아이콘 `.jpg` 57개).
* 기존 `br.pl`에서는 `http://www.ruria.pe.kr/brimg/` 외부 URL을 참조하고 있어 로컬 정적 파일 서빙으로 전환이 필요합니다.

---

## 3. 레거시 구조의 핵심 문제점 (Go 포팅 시 반드시 개선할 부분)

1. **매 요청 CGI 프로세스 생성**: 버튼 클릭마다 `perl` 프로세스가 새로 뜨고 종료됨.
2. **`mkdir` 기반 파일 락 (`$lkey = 2`)**: `./lock/lock` 디렉토리 생성/삭제로 동시성을 제어하여, 프로세스 비정상 종료 시 데드락 위험이 있으며 경합 시 대기/에러가 잦음.
3. **CSV 전체 덮어쓰기 I/O**: 유저 1명의 HP/스태미나만 바뀌어도 `userdatfile.cgi` 전체를 읽고 다시 씀.
4. **CP949(EUC-KR) 인코딩**: 리눅스/모던 웹 환경(UTF-8)과 호환을 위해 소스 및 마스터 데이터 전부 UTF-8 변환 필요.
5. **보안 취약점**: 평문 비밀번호 저장, 쿠키/폼 hidden input 기반 인증 변조 가능성, HTML 이스케이프 미흡(XSS 위험).
6. **Lazy 금지구역 체크**: `pref.cgi`가 유저 HTTP 요청을 받을 때만 시간을 비교해 금지구역을 추가하는 구조.
