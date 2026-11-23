# Battle Royale Web Game (Go + SQLite Porting)

2003년 한글판 Perl CGI 웹게임(`Br1.15Ruria`) 및 실운영 데이터/이미지를 기반으로, **Go + SQLite** 단일 바이너리 웹 애플리케이션으로 현대화 포팅하기 위한 개발 설계 문서 저장소입니다.

---

## 1. 원본 소스 및 실운영 에셋 위치

| 구분 | 경로 | 비고 |
| --- | --- | --- |
| **Perl CGI 원본 소스** | `/home/elmilin/다운로드/Br1.15Ruria/` | 총 6,219줄 (CP949/CRLF) |
| **실운영 이미지 에셋** | `/home/elmilin/다운로드/763759eb50e844bcba8/img/` | 총 58개 파일 (`0.jpg`~`56.jpg` 등 + `map.jpg`) |
| **실운영 커스텀 데이터** | `/home/elmilin/다운로드/763759eb50e844bcba8/dat/` | `gousei_tbl.cgi` 등 대폭 확장된 테이블 |

> **중요:** `Br1.15Ruria/dat/` 보다 `763759eb50e844bcba8/dat/`가 실제 운영 시 커스텀 추가된 데이터(특히 조합식 `gousei_tbl.cgi` 10KB)를 포함하고 있으므로, 포팅 시에는 **실운영 `dat/`와 `img/`를 기준**으로 UTF-8 변환 후 사용합니다.

---

## 2. 문서 구성

1. **[01-legacy-analysis.md](./docs/01-legacy-analysis.md)**
   * 기존 Perl 소스 파일별 역할 및 서브루틴 매핑
   * 플랫 파일 DB / CSV 스키마 / 파일 락 구조 분석
   * 실운영 데이터 차이점 및 레거시 코드의 한계점
2. **[02-go-architecture.md](./docs/02-go-architecture.md)**
   * Go + SQLite(WAL) 기반 대체 아키텍처 및 패키지 구조
   * SQLite 테이블 스키마 설계
   * 동시성 제어, 인증/보안, 금지구역 스케줄러, UI/정적 파일 서빙 개선안
3. **[03-porting-checklist-and-ops.md](./docs/03-porting-checklist-and-ops.md)**
   * 단계별 포팅 로드맵 (Phase 1 ~ 5)
   * 테스트 및 검증 전략 (전투 공식 대조, 동시성 `-race` 테스트)
   * 오라클 프리티어 1GB RAM VPS 배포 및 운영 체크리스트

---

## 3. 새 채팅창에서 개발 시작하기

새 채팅창을 열고 이 폴더(`/home/elmilin/dev/work/battle-royale/`)를 워크스페이스로 지정한 뒤, 아래와 같이 요청하면 바로 개발을 시작할 수 있습니다.

> "`README.md`와 `docs/` 문서를 읽고, Phase 1(Go 모듈 초기화, 실운영 `dat/`/`img/` UTF-8 변환 및 임포트, SQLite 스키마 및 기본 서버 뼈대 구축)부터 시작해 줘."
