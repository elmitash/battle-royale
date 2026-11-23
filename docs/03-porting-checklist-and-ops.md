# 03. 포팅 로드맵, 테스트 및 오라클 VPS 배포 가이드

---

## 1. 단계별 개발 로드맵

### Phase 1: 프로젝트 초기화 및 데이터 변환
- [ ] `go mod init` 및 디렉토리 구조 생성
- [ ] 실운영 에셋 복사 및 UTF-8 변환
  - `/home/elmilin/다운로드/763759eb50e844bcba8/img/*` → `static/img/`
  - `/home/elmilin/다운로드/763759eb50e844bcba8/dat/*` → `iconv -f cp949 -t utf-8` 변환 후 `data/`에 저장
  - `Br1.15Ruria/br.css`, `index.htm`, `rule.htm` 등도 UTF-8 변환 참고
- [ ] `internal/config`: `br.pl`의 지역 22개, 반/정원, 경험치, 스태미나, 아이콘 목록 등 상수 정의
- [ ] `internal/db`: SQLite(`modernc.org/sqlite`) 연결, WAL 모드 및 스키마 마이그레이션, `data/` 파싱 로더 구현

### Phase 2: 인증, 등록 및 코어 탐색 루프
- [ ] 회원가입(`regist.cgi` 대응): 랜덤 스탯, 동아리(`CLUBMAKE`), 초기 무기(`wepfile`)/소지품(`stitemfile`) 지급, `bcrypt` 비밀번호 저장
- [ ] 로그인/세션 미들웨어(`login.cgi` 대응): `HttpOnly` 세션 쿠키 발급 및 검증
- [ ] 메인 화면(`battle.cgi`, `lib2.cgi/STS` 대응): 유저 상태, 소지품, 현재 지역 정보, 금지구역 상태 렌더링
- [ ] 이동(`MOVE`) 및 탐색(`SEARCH`): 지역 이동, 아이템 습득(`ITEMGET`), 함정 발동, 랜덤 이벤트(`event.cgi`)

### Phase 3: 아이템, 전투, 특수 시스템 및 엔딩
- [ ] 아이템 시스템(`item.cgi`, `itemgou.cgi`, `itemsei.cgi`): 장착/해제, 회복, 탄약 장전, 내구도, **조합식(`gousei_tbl.cgi` 10KB 전체 파싱)**, 정리
- [ ] 특수 시스템: 독약/독미(`poison.cgi`), 레이더(`reader.cgi`), 확성기(`speaker.cgi`), 분교 해킹(`hack.cgi`)
- [ ] 전투 시스템(`attack.cgi`): 선제/반격 판정, 무기별 숙련도/명중/대미지 공식, 도주(`RUNAWAY`), 전리품 획득(`DEATHGET`), 레벨업(`LVUPCHK`), 킬/사망 뉴스 로그
- [ ] 금지구역 스케줄러 & 엔딩(`ending.cgi`): 백그라운드 `time.Ticker` 금지구역 확장, 최후 1인/해킹/해제키 엔딩 및 `winners` 기록

### Phase 4: 관리자 기능, 보안 및 테스트
- [ ] 관리자 페이지(`admin.cgi` 대응): 라운드 초기화/리셋(`DATARESET`), NPC 리스폰(`base.cgi`), 유저 목록/추방, DB 백업 다운로드
- [ ] 보안 강화: 모든 입력값 검증, `html/template` 자동 XSS 이스케이프 확인, `BR_ADMIN_PASS` 환경변수 분리
- [ ] 테스트 작성:
  - `attack.cgi` 대미지/숙련도/레벨업 공식 단위 테스트 (Perl 원본과 결과 비교)
  - `gousei_tbl.cgi` 전체 조합식 파싱 테스트
  - `go test -race ./...` 동시성 테스트 (동시 아이템 습득/전투 경합 시뮬레이션)

### Phase 5: 오라클 프리티어 1GB VPS 배포 (선택: 스페셜 모드)
- [ ] (선택) 1인/소수 유저용 **스피드 라운드 / AI 봇** 옵션 추가 고려
- [ ] `linux/amd64` (또는 `arm64`) 크로스 컴파일 및 VPS 배포

---

## 2. 오라클 클라우드 프리티어 (1GB RAM) 배포 체크리스트

1. **로컬 크로스 컴파일 (서버에서 빌드 금지)**
   * 1GB 램 서버에서 `go build`를 직접 실행하면 메모리 부족으로 멈출 수 있으므로, 로컬에서 빌드 후 단일 바이너리만 업로드합니다.
   ```bash
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o battle-royale ./cmd/server
   ```
2. **Swap 파일 1~2GB 설정**
   * OS 업데이트나 로그 작업 시 OOM(Out of Memory) 방지를 위해 VPS에 1~2GB 스왑을 미리 잡아둡니다.
3. **방화벽 2중 오픈**
   * 오라클 웹 콘솔 **VCN → Security List (Ingress Rule)** 에서 80/443 포트 허용
   * 인스턴스 내부 OS 방화벽(`iptables` 또는 `firewalld`/`ufw`)에서도 80/443 포트 허용
4. **리버스 프록시 + 자동 HTTPS**
   * **Caddy** 또는 **Nginx**를 앞단에 두고 Go 서버(`127.0.0.1:8080`)로 프록시 연결 (Caddy는 Let's Encrypt HTTPS 자동 설정이 매우 간단함).
5. **systemd 서비스 등록 & Graceful Shutdown**
   * `Restart=always` 설정으로 프로세스 재시작 보장, `SIGINT`/`SIGTERM` 수신 시 진행 중인 HTTP 요청과 SQLite 체크포인트(`PRAGMA wal_checkpoint(TRUNCATE)`)를 안전하게 마무리.
6. **백업 & 유휴 인스턴스 주의**
   * SQLite DB 파일(`.db`)을 주기적으로 외부(내 PC, 오브젝트 스토리지 등)에 백업.
   * 오라클 무료 인스턴스는 장기간 부하가 극도로 낮으면 유휴 회수 대상이 될 수 있으므로 주의.
