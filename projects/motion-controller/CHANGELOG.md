# Changelog

## [0.3.2] - 2026-07-22

### Changed

- 缂栫爜鍣ㄨˉ鍋块厤缃」鏀逛负浠呭湪 B140 鎺у埗鍣ㄤ笖璇ヨ酱銆屼綅缃潵婧愩€嶉€変簡缂栫爜鍣ㄦ椂鏄剧ず銆俉TNMC4A / 妯℃嫙鎺у埗鍣ㄤ笉鏄剧ず閰嶇疆椤癸紝閬垮厤璇厤缃笉鏀寔鐨勫姛鑳姐€?- 缂栫爜鍣ㄨˉ鍋块厤缃尯涓夌被绌烘€佸垎鍒粰鍑烘槑纭彁绀猴細闈?B140 鎺у埗鍣ㄣ€丅140 浣嗘棤缂栫爜鍣ㄨ酱銆丅140 涓旀湭鍚敤浠讳綍琛ュ伩銆?
### Fixed

- 淇鐢ㄦ埛浠?B140 鍒囨崲鍒板叾浠栨帶鍒跺櫒绫诲瀷鏃讹紝閬楃暀鐨勭紪鐮佸櫒琛ュ伩閰嶇疆浠嶈Е鍙戜繚瀛橀樆鏂殑闂锛歚compensationErrors` 澧炲姞 B140 绫诲瀷瀹堝崼锛岄潪 B140 鏃惰烦杩囨牎楠屻€?
### Verification

- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues

- 鏆傛棤銆?
## [0.3.1] - 2026-07-06

### Changed

- 杞撮厤缃崱鐗囧竷灞€鏀硅繘锛氱洿绾胯酱鏄剧ず"瀵肩▼ mm"銆佹棆杞酱鏄剧ず"浼犲姩姣?锛屽彇浠ｅ浐瀹氬弻瀛楁锛屽噺灏戞贩娣嗐€?- 杞撮厤缃爣绛炬枃妗堜紭鍖栵細姝ヨ窛瑙掑姞鍗曚綅"掳"銆?鍙嶈浆"鈫?鏂瑰悜鍙嶈浆"銆?浣嶇疆婧?鈫?浣嶇疆鏉ユ簮"銆?鏈€澶ч€熷害"鑷€傚簲鏄剧ず涓?鏈€澶ц浆閫?锛堟棆杞酱锛夈€?- 杞撮厤缃剦鍐插綋閲忔敼涓?computed 缂撳瓨锛岄伩鍏嶆ā鏉挎瘡娆℃覆鏌撻噸澶嶈绠椼€?- 鐐瑰姩闈欐榛樿鍊间粠 1 鏀逛负 0.1锛岄檷浣庤皟璇曟椂鎰忓纰版挒椋庨櫓銆?
### Internal

- 灏?motion 閰嶇疆宸ュ叿鍑芥暟锛坈reateDefaultAxis銆乧omputePulsesPerUnit銆乿alidateEncoderCompensation 绛夛級鎻愬彇鍒?workspace 绾?`shared/frontend/motion-utils`锛宮otion-controller 椤圭洰閫氳繃 re-export 浣跨敤锛屼繚鐣欓」鐩骇 maxSpeed 榛樿鍊硷紙浣庨€?10锛夈€?
### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 鏋勫缓瀹夎鍖?
### Known Issues

- 鏆傛棤銆?
## [0.3.0] - 2026-07-02

### Added
- B140 缂栫爜鍣ㄨˉ鍋垮叏閾捐矾鏀寔锛氳ˉ鍋垮弬鏁扮紪杈戝櫒 UI銆佷袱闃舵鐘舵€佹満锛坵aitingStop鈫抯ettling鈫抍hecking鈫攃ompensating锛夈€佷笁灞傜簿搴︾害鏉熸牎楠岄摼锛堣剦鍐插綋閲?鈮?encoderScale 鈮?tolerance > minStep锛夈€?- 琛ュ伩鐘舵€佹満鏂板 compensating 鍒嗘敮閲嶈 TP 澶辫触鍛婅锛屼笉鍐嶉潤榛樺悶閿欍€?- 鍏夋爡灏哄垎杈ㄧ巼鍙紪杈戣緭鍏ュ叆鍙ｏ紝鏀寔棰勮/鑷畾涔夊弬鏁板垏鎹€?
### Changed
- encoderScale 褰掍竴鍖栨樉绀猴紝閰嶇疆涓嶅悎鐞嗘椂鍛婅鍗囩骇涓?error 绾ч樆鏂繚瀛樸€?- Status() 缂栫爜鍣ㄨ鍙栧け璐ユ椂鏀堕泦鍒?LastError锛屼娇鏁呴殰瀵规搷浣滃憳鍙銆?
### Internal
- shared/device-sdk: ValidateCompensationConfig / ResolveEncoderCompensation 鐗╃悊鍚堢悊鎬ф牎楠屽嚱鏁般€?- shared/device-sdk: B140 琛ュ伩鐘舵€佹満瀹炵幇锛坆140_motion.go锛夈€?- shared/motion-control: UpsertProfile 杈圭晫鍏滃簳锛岄樆鏂墿鐞嗕笉鍙兘鐨勮ˉ鍋块厤缃€?- 娴嬭瘯瑕嗙洊锛歝onversions_test (6)銆乵otion_manager_compensation_test (6)銆乥140_compensation_test锛堢姸鎬佹満鍚勫満鏅?+ 缂栫爜鍣ㄨ澶辫触鏆撮湶锛夈€?
### Verification
- `go test ./...` (with GOWORK=off): passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.6] - 2026-06-29

### Fixed
- 淇绐楀彛鏍囬鏍忓乏涓婅鍥炬爣涓嶆樉绀虹殑闂锛歮ain.go 缂哄皯 `application.Options.Icon` 璁剧疆锛屽鑷?Windows 绐楀彛鏍囬鏍忓浘鏍囩己澶憋紙浠诲姟鏍忓浘鏍囧洜 `rsrc.syso` 宓屽叆 EXE 璧勬簮鑰屾甯革紝浣嗙獥鍙ｅ浘鏍囬渶瑕佹樉寮忔彁渚?PNG 鏁版嵁锛夈€?
### Internal
- 鍦?main.go 涓坊鍔?`//go:embed build/appicon.png` 鍜?`var appIcon []byte`锛屽苟鍦?`application.Options` 涓缃?`Icon: appIcon`锛屼笌 wind-daq銆乨aq-t1603銆乨aq-p1604 绛夊叾浠栭」鐩繚鎸佷竴鑷淬€?
### Verification
- `go build -tags production`: passed
- 鍐掔儫鍚姩娴嬭瘯: passed锛堢獥鍙ｅ乏涓婅鍥炬爣宸叉甯告樉绀猴級
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.5] - 2026-06-29

### Fixed
- 淇绐楀彛缂╂斁鏃惰繍鍔ㄦ帶鍒朵富闈㈡澘涓嬫柟鍑虹幇澶х墖绌虹櫧鐨勯棶棰橈細MotionControlPanel 鏍瑰鍣ㄤ笌涓婚潰鏉?section 鏈缓绔嬪畬鏁撮珮搴︾户鎵块摼锛岃酱鍗＄墖缃戞牸鏈崰婊″彲鐢ㄩ珮搴︺€?
### Internal
- MotionControlPanel 鏍瑰鍣ㄨˉ鍏?`h-full min-h-0`锛屼富闈㈡澘 section 绉婚櫎 `h-fit` 鏀逛负 `min-h-0`銆?- `.axis-content` 鏀逛负 flex 鍒楀竷灞€锛宍.axis-grid` 澧炲姞 `flex: 1; min-height: 0`锛宍.axis-card` 鏀逛负 `height: 100%`锛屼娇杞村崱鐗囬殢绐楀彛楂樺害鍚堢悊濉厖銆?
### Verification
- `npm run typecheck`: passed
- `npm run build`: passed
- `validate-frontend-structure.ps1`: passed
- `task release`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.4] - 2026-06-29

### Internal
- 瑙勮寖鍖栫敓浜ф瀯寤烘爣绛撅細Taskfile.yml `build-go` 澧炲姞 `-tags production -trimpath` 涓?`-w -s`銆?
### Verification
- `go test ./...`: passed锛堟棤娴嬭瘯鏂囦欢锛?- `npm run typecheck`: passed
- `npm run build`: passed - vite build
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.3] - 2026-06-29

### Changed
- 绉婚櫎杞村惎鐢?绂佺敤寮€鍏筹紝鎵€鏈夎酱濮嬬粓鍚敤锛岀畝鍖栭厤缃祦绋嬨€?
### Fixed
- 璁剧疆 Windows GUI subsystem锛屾寮忔瀯寤虹▼搴忎笉鍐嶆樉绀烘帶鍒跺彴绐楀彛銆?
### Internal
- 鏃ч厤缃枃浠惰鍏ユ椂 enabled 瀛楁鑷姩褰掍竴鍖栦负 true锛屾棤闇€鐢ㄦ埛鎵嬪姩杩佺Щ銆?
### Verification
- `go test ./...`: passed锛堟棤娴嬭瘯鏂囦欢锛?- `npm run typecheck`: passed
- `npm run build`: passed - vite build, 1788 modules
- `go build -buildvcs=false`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed - 7.45 MB installer

### Known Issues
- 鏆傛棤銆?
## [0.2.2] - 2026-06-26

### Fixed
- 淇 0.2.1 瀹夎鍚?Windows 鎶モ€滃簲鐢ㄧ▼搴忕殑骞惰閰嶇疆涓嶆纭€濆鑷存棤娉曞惎鍔ㄧ殑闂銆?
### Internal
- Windows 璧勬簮鐢熸垚鏀逛负浠呭祵鍏ュ簲鐢ㄥ浘鏍囷紝閬垮厤灏嗘湭娓叉煋鐨?Wails manifest 妯℃澘宓屽叆 `.exe`銆?
### Verification
- `rsrc -ico + go build`: passed
- `go test ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails3 build`: passed
- `makensis /DARG_WAILS_AMD64_BINARY=..\\..\\bin\\motion-controller.exe project.nsi`: passed
- 鏈湴鍚姩 `build/bin/motion-controller.exe` 鍚庤繘绋嬩繚鎸佽繍琛岋紝SideBySide 鏃ュ織鏃犳柊澧?manifest 閿欒
- `npm run test`: not applicable锛屽綋鍓嶆棤 `src/**/*.test.ts` 娴嬭瘯鏂囦欢锛孷itest 杩斿洖 code 1

### Known Issues
- 鏆傛棤銆?
## [0.2.1] - 2026-06-26

### Fixed
- 淇瀹夎鍖呮棤搴旂敤鍥炬爣鐨勯棶棰橈細icon.ico 鏂囦欢鎹熷潖瀵艰嚧 .exe 鏈祵鍏ュ浘鏍囪祫婧愶紝妗岄潰蹇嵎鏂瑰紡鏄剧ず绌虹櫧鍥炬爣
- 鏋勫缓娴佺▼琛ュ厖 .syso 璧勬簮鏂囦欢鐢熸垚姝ラ锛岀‘淇濆浘鏍囨纭祵鍏ュ彲鎵ц鏂囦欢

### Internal
- icon.ico 鐢?wails3 generate icons 浠?512脳512 appicon.png 閲嶆柊鐢熸垚锛? 绉嶅垎杈ㄧ巼锛?- Taskfile.yml 鏂板 generate-syso 浠诲姟锛屽湪 build-go 鍓嶈嚜鍔ㄧ敓鎴愬苟娓呯悊 .syso

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `rsrc -ico + go build + NSIS` 鏋勫缓瀹夎鍖?
### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-06-26

### Added
- Wails v3 杩佺Щ锛氶噸鏂扮敓鎴?bindings锛岀Щ闄ゆ棫 wailsjs 妗ユ帴
- B140 鎺у埗鍣ㄥ疄鏃朵綅缃鍙栨敮鎸?- 浼樺厛绾у懡浠ら€氶亾锛坧riorityCmdCh锛夛紝Stop/Jog/MoveTo 绛夊姩浣滃懡浠や笉鍐嶈 Status 鏌ヨ闃诲
- Dll 鏌ユ壘璺緞澧炲己锛氫紭鍏堜粠鍙墽琛屾枃浠舵墍鍦ㄧ洰褰曟煡鎵?WTNMC4A_64.dll

### Changed
- 杩愬姩鎺у埗鍣ㄥ叏閾捐矾鏀归€狅細鐘舵€佹帹閫侀噸鏋勩€乁I 浜や簰澧炲己
- ApplyConfig 鎺ュ彛閫昏緫鍗囩骇锛氶厤缃繚瀛樹笉鍐嶆柇杩?- 杩愬姩鎺у埗涓荤敾闈㈠拰閰嶇疆鐢婚潰甯冨眬浼樺寲涓庝唬鐮佽川閲忔彁鍗?- go.mod/go.sum 渚濊禆鏇存柊閫傞厤 Wails v3

### Fixed
- Stop/EmergencyStop 瀹屽叏缁曡繃閿佽皟鐢?DLL锛堢撼绉掔骇鎸侀攣锛?- 鎻愰珮鍋滄/鎬ュ仠浼樺厛绾э紝涓嶄笌 Status 杞浜夋姠鍐欓攣
- WTNMC4A 椹卞姩绋冲畾鎬у姞鍥猴細DLL 宕╂簝闃叉姢銆佽緭鍏ユ牎楠屻€侀攣浼樺寲
- Stop 鏀圭敤 instStop 绔嬪嵆鍋滄 + atomic 鏃犻攣璇绘秷闄ら攣绔炰簤
- UI spatial rules compliance + token system fixes

### Internal
- Wails v2 鈫?v3 鍏ㄥ钩鍙拌縼绉伙紙daq-p1604, five-hole-interpolator, motion-controller, three-hole-interpolator锛?- motion-controller 椤圭洰鑴氭墜鏋跺垵濮嬪寲鍜岀粨鏋勫畬鍠?- docs: README銆丼PEC銆丳LAN銆丄GENTS 鏂囨。琛ュ厖

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `wails build`

### Known Issues
- 鏆傛棤銆?

## [0.3.1-win7.1] - 2026-07-24

### Added

- Windows 7 鍏煎鐗堟敼閫狅細鏂板 `apps/desktop-electron/` Electron 22.3.27 澹筹紝鎵胯浇 Vue 3 鍓嶇 + Go HTTP 鍚庣鐨?Win7 妗岄潰搴旂敤銆?
### Changed

- Go 鍚庣 (`apps/desktop-wails/`) 绉婚櫎 Wails v3 渚濊禆锛?  - `go.mod` 闄嶇骇鍒?Go 1.20锛圵ails v3 鍐呴儴鐢?log/slog + maps + slices锛岄渶 Go 1.21+锛學in7 蹇呴』鐢?Go 1.20.14锛夈€?  - `main.go` 鏀逛负 `net/http` server锛岀洃鍚?`127.0.0.1:16888`锛宔mbed `frontend/dist` 闈欐€佽祫婧愩€?  - `backend/app.go` 绉婚櫎 `application.Service` 鎺ュ彛渚濊禆锛屾柊澧?`RegisterRoutes(mux)` 鏂规硶娉ㄥ唽 motion HTTP 璺敱 + CORS 涓棿浠讹紝HTTP server 鐢熷懡鍛ㄦ湡鐢?`main.go` 缁熶竴绠＄悊銆?  - 绉婚櫎 Wails 缁戝畾鏂规硶锛坄MotionUpsertProfile` / `MotionConnect` / `MotionGetStatus` 绛夛級锛宮otion 鍏ㄩ儴璧?HTTP銆?  - `log/slog` 鏀逛负 `shared.local/device-sdk/go/pkg/slog` polyfill銆?- 鍓嶇 (`apps/desktop-wails/frontend/`) 绉婚櫎 Wails runtime 渚濊禆锛?  - `package.json` 绉婚櫎 `@wailsio/runtime`銆?  - 鍒犻櫎 `src/api/wails-adapter.ts` 鍜?`src/api/http-client.ts`锛堜笉鍐嶈寮曠敤锛夈€?  - 閲嶅啓 `src/api/motionApi.ts`锛氭墍鏈夎皟鐢ㄧ粺涓€璧?`MOTION_HTTP_BASE = http://127.0.0.1:16888`锛岀Щ闄?`isWailsAvailable()` 鍒嗘敮銆?  - `vite.config.ts` proxy 鎸囧悜 `127.0.0.1:16888`锛堝紑鍙戞€佺敤锛夛紝鐢熶骇鎬?Electron 涓庡悗绔悓婧愩€?
### Internal

- 澶嶇敤 `shared.local/motion-control/go/httpapi.RegisterMotionRoutes` 娉ㄥ唽瀹屾暣 motion HTTP 璺敱锛坧rofiles / status / connect / disconnect / home / moveTo / moveBy / jog / stop / emergencyStop / resetEmergencyStop / definePosition锛夈€?- 绔彛 16888 涓?wind-daq锛?900/8901锛? daq-t1603锛?8181锛? daq-p1604锛?8182锛? probe-interpolator锛?8183锛夊尯鍒嗭紝閬垮厤鍚屾満澶氬紑鍐茬獊銆?- 浠?wind-daq 澶嶅埗 `appicon.ico` / `appicon.png` 浣滀负鍗犱綅鍥炬爣锛堝緟鏇挎崲涓?motion-controller 涓撳睘鍥炬爣锛夈€?
### Verification

- `go vet ./...` (GOWORK=off, Go 1.20.14): passed
- `go test ./...` (GOWORK=off, Go 1.20.14): passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `npm run build:backend`: passed
- `npm run dist:win7`: passed锛岀敓鎴?`apps/desktop-electron/dist/Motion-Controller-Win7-Setup-0.3.1-win7.1-x64.exe`

### Known Issues

- 搴旂敤鍥炬爣鏆傜敤 wind-daq 鍥炬爣鍗犱綅锛屽悗缁浛鎹负 motion-controller 涓撳睘鍥炬爣銆?
## [0.3.1] - 2026-07-06

### Changed

- 杞撮厤缃崱鐗囧竷灞€鏀硅繘锛氱洿绾胯酱鏄剧ず"瀵肩▼ mm"銆佹棆杞酱鏄剧ず"浼犲姩姣?锛屽彇浠ｅ浐瀹氬弻瀛楁锛屽噺灏戞贩娣嗐€?- 杞撮厤缃爣绛炬枃妗堜紭鍖栵細姝ヨ窛瑙掑姞鍗曚綅"掳"銆?鍙嶈浆"鈫?鏂瑰悜鍙嶈浆"銆?浣嶇疆婧?鈫?浣嶇疆鏉ユ簮"銆?鏈€澶ч€熷害"鑷€傚簲鏄剧ず涓?鏈€澶ц浆閫?锛堟棆杞酱锛夈€?- 杞撮厤缃剦鍐插綋閲忔敼涓?computed 缂撳瓨锛岄伩鍏嶆ā鏉挎瘡娆℃覆鏌撻噸澶嶈绠椼€?- 鐐瑰姩闈欐榛樿鍊间粠 1 鏀逛负 0.1锛岄檷浣庤皟璇曟椂鎰忓纰版挒椋庨櫓銆?
### Internal

- 灏?motion 閰嶇疆宸ュ叿鍑芥暟锛坈reateDefaultAxis銆乧omputePulsesPerUnit銆乿alidateEncoderCompensation 绛夛級鎻愬彇鍒?workspace 绾?`shared/frontend/motion-utils`锛宮otion-controller 椤圭洰閫氳繃 re-export 浣跨敤锛屼繚鐣欓」鐩骇 maxSpeed 榛樿鍊硷紙浣庨€?10锛夈€?
### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 鏋勫缓瀹夎鍖?
### Known Issues

- 鏆傛棤銆?
## [0.3.0] - 2026-07-02

### Added
- B140 缂栫爜鍣ㄨˉ鍋垮叏閾捐矾鏀寔锛氳ˉ鍋垮弬鏁扮紪杈戝櫒 UI銆佷袱闃舵鐘舵€佹満锛坵aitingStop鈫抯ettling鈫抍hecking鈫攃ompensating锛夈€佷笁灞傜簿搴︾害鏉熸牎楠岄摼锛堣剦鍐插綋閲?鈮?encoderScale 鈮?tolerance > minStep锛夈€?- 琛ュ伩鐘舵€佹満鏂板 compensating 鍒嗘敮閲嶈 TP 澶辫触鍛婅锛屼笉鍐嶉潤榛樺悶閿欍€?- 鍏夋爡灏哄垎杈ㄧ巼鍙紪杈戣緭鍏ュ叆鍙ｏ紝鏀寔棰勮/鑷畾涔夊弬鏁板垏鎹€?
### Changed
- encoderScale 褰掍竴鍖栨樉绀猴紝閰嶇疆涓嶅悎鐞嗘椂鍛婅鍗囩骇涓?error 绾ч樆鏂繚瀛樸€?- Status() 缂栫爜鍣ㄨ鍙栧け璐ユ椂鏀堕泦鍒?LastError锛屼娇鏁呴殰瀵规搷浣滃憳鍙銆?
### Internal
- shared/device-sdk: ValidateCompensationConfig / ResolveEncoderCompensation 鐗╃悊鍚堢悊鎬ф牎楠屽嚱鏁般€?- shared/device-sdk: B140 琛ュ伩鐘舵€佹満瀹炵幇锛坆140_motion.go锛夈€?- shared/motion-control: UpsertProfile 杈圭晫鍏滃簳锛岄樆鏂墿鐞嗕笉鍙兘鐨勮ˉ鍋块厤缃€?- 娴嬭瘯瑕嗙洊锛歝onversions_test (6)銆乵otion_manager_compensation_test (6)銆乥140_compensation_test锛堢姸鎬佹満鍚勫満鏅?+ 缂栫爜鍣ㄨ澶辫触鏆撮湶锛夈€?
### Verification
- `go test ./...` (with GOWORK=off): passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.6] - 2026-06-29

### Fixed
- 淇绐楀彛鏍囬鏍忓乏涓婅鍥炬爣涓嶆樉绀虹殑闂锛歮ain.go 缂哄皯 `application.Options.Icon` 璁剧疆锛屽鑷?Windows 绐楀彛鏍囬鏍忓浘鏍囩己澶憋紙浠诲姟鏍忓浘鏍囧洜 `rsrc.syso` 宓屽叆 EXE 璧勬簮鑰屾甯革紝浣嗙獥鍙ｅ浘鏍囬渶瑕佹樉寮忔彁渚?PNG 鏁版嵁锛夈€?
### Internal
- 鍦?main.go 涓坊鍔?`//go:embed build/appicon.png` 鍜?`var appIcon []byte`锛屽苟鍦?`application.Options` 涓缃?`Icon: appIcon`锛屼笌 wind-daq銆乨aq-t1603銆乨aq-p1604 绛夊叾浠栭」鐩繚鎸佷竴鑷淬€?
### Verification
- `go build -tags production`: passed
- 鍐掔儫鍚姩娴嬭瘯: passed锛堢獥鍙ｅ乏涓婅鍥炬爣宸叉甯告樉绀猴級
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.5] - 2026-06-29

### Fixed
- 淇绐楀彛缂╂斁鏃惰繍鍔ㄦ帶鍒朵富闈㈡澘涓嬫柟鍑虹幇澶х墖绌虹櫧鐨勯棶棰橈細MotionControlPanel 鏍瑰鍣ㄤ笌涓婚潰鏉?section 鏈缓绔嬪畬鏁撮珮搴︾户鎵块摼锛岃酱鍗＄墖缃戞牸鏈崰婊″彲鐢ㄩ珮搴︺€?
### Internal
- MotionControlPanel 鏍瑰鍣ㄨˉ鍏?`h-full min-h-0`锛屼富闈㈡澘 section 绉婚櫎 `h-fit` 鏀逛负 `min-h-0`銆?- `.axis-content` 鏀逛负 flex 鍒楀竷灞€锛宍.axis-grid` 澧炲姞 `flex: 1; min-height: 0`锛宍.axis-card` 鏀逛负 `height: 100%`锛屼娇杞村崱鐗囬殢绐楀彛楂樺害鍚堢悊濉厖銆?
### Verification
- `npm run typecheck`: passed
- `npm run build`: passed
- `validate-frontend-structure.ps1`: passed
- `task release`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.4] - 2026-06-29

### Internal
- 瑙勮寖鍖栫敓浜ф瀯寤烘爣绛撅細Taskfile.yml `build-go` 澧炲姞 `-tags production -trimpath` 涓?`-w -s`銆?
### Verification
- `go test ./...`: passed锛堟棤娴嬭瘯鏂囦欢锛?- `npm run typecheck`: passed
- `npm run build`: passed - vite build
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.2.3] - 2026-06-29

### Changed
- 绉婚櫎杞村惎鐢?绂佺敤寮€鍏筹紝鎵€鏈夎酱濮嬬粓鍚敤锛岀畝鍖栭厤缃祦绋嬨€?
### Fixed
- 璁剧疆 Windows GUI subsystem锛屾寮忔瀯寤虹▼搴忎笉鍐嶆樉绀烘帶鍒跺彴绐楀彛銆?
### Internal
- 鏃ч厤缃枃浠惰鍏ユ椂 enabled 瀛楁鑷姩褰掍竴鍖栦负 true锛屾棤闇€鐢ㄦ埛鎵嬪姩杩佺Щ銆?
### Verification
- `go test ./...`: passed锛堟棤娴嬭瘯鏂囦欢锛?- `npm run typecheck`: passed
- `npm run build`: passed - vite build, 1788 modules
- `go build -buildvcs=false`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed - 7.45 MB installer

### Known Issues
- 鏆傛棤銆?
## [0.2.2] - 2026-06-26

### Fixed
- 淇 0.2.1 瀹夎鍚?Windows 鎶モ€滃簲鐢ㄧ▼搴忕殑骞惰閰嶇疆涓嶆纭€濆鑷存棤娉曞惎鍔ㄧ殑闂銆?
### Internal
- Windows 璧勬簮鐢熸垚鏀逛负浠呭祵鍏ュ簲鐢ㄥ浘鏍囷紝閬垮厤灏嗘湭娓叉煋鐨?Wails manifest 妯℃澘宓屽叆 `.exe`銆?
### Verification
- `rsrc -ico + go build`: passed
- `go test ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails3 build`: passed
- `makensis /DARG_WAILS_AMD64_BINARY=..\\..\\bin\\motion-controller.exe project.nsi`: passed
- 鏈湴鍚姩 `build/bin/motion-controller.exe` 鍚庤繘绋嬩繚鎸佽繍琛岋紝SideBySide 鏃ュ織鏃犳柊澧?manifest 閿欒
- `npm run test`: not applicable锛屽綋鍓嶆棤 `src/**/*.test.ts` 娴嬭瘯鏂囦欢锛孷itest 杩斿洖 code 1

### Known Issues
- 鏆傛棤銆?
## [0.2.1] - 2026-06-26

### Fixed
- 淇瀹夎鍖呮棤搴旂敤鍥炬爣鐨勯棶棰橈細icon.ico 鏂囦欢鎹熷潖瀵艰嚧 .exe 鏈祵鍏ュ浘鏍囪祫婧愶紝妗岄潰蹇嵎鏂瑰紡鏄剧ず绌虹櫧鍥炬爣
- 鏋勫缓娴佺▼琛ュ厖 .syso 璧勬簮鏂囦欢鐢熸垚姝ラ锛岀‘淇濆浘鏍囨纭祵鍏ュ彲鎵ц鏂囦欢

### Internal
- icon.ico 鐢?wails3 generate icons 浠?512脳512 appicon.png 閲嶆柊鐢熸垚锛? 绉嶅垎杈ㄧ巼锛?- Taskfile.yml 鏂板 generate-syso 浠诲姟锛屽湪 build-go 鍓嶈嚜鍔ㄧ敓鎴愬苟娓呯悊 .syso

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `rsrc -ico + go build + NSIS` 鏋勫缓瀹夎鍖?
### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-06-26

### Added
- Wails v3 杩佺Щ锛氶噸鏂扮敓鎴?bindings锛岀Щ闄ゆ棫 wailsjs 妗ユ帴
- B140 鎺у埗鍣ㄥ疄鏃朵綅缃鍙栨敮鎸?- 浼樺厛绾у懡浠ら€氶亾锛坧riorityCmdCh锛夛紝Stop/Jog/MoveTo 绛夊姩浣滃懡浠や笉鍐嶈 Status 鏌ヨ闃诲
- Dll 鏌ユ壘璺緞澧炲己锛氫紭鍏堜粠鍙墽琛屾枃浠舵墍鍦ㄧ洰褰曟煡鎵?WTNMC4A_64.dll

### Changed
- 杩愬姩鎺у埗鍣ㄥ叏閾捐矾鏀归€狅細鐘舵€佹帹閫侀噸鏋勩€乁I 浜や簰澧炲己
- ApplyConfig 鎺ュ彛閫昏緫鍗囩骇锛氶厤缃繚瀛樹笉鍐嶆柇杩?- 杩愬姩鎺у埗涓荤敾闈㈠拰閰嶇疆鐢婚潰甯冨眬浼樺寲涓庝唬鐮佽川閲忔彁鍗?- go.mod/go.sum 渚濊禆鏇存柊閫傞厤 Wails v3

### Fixed
- Stop/EmergencyStop 瀹屽叏缁曡繃閿佽皟鐢?DLL锛堢撼绉掔骇鎸侀攣锛?- 鎻愰珮鍋滄/鎬ュ仠浼樺厛绾э紝涓嶄笌 Status 杞浜夋姠鍐欓攣
- WTNMC4A 椹卞姩绋冲畾鎬у姞鍥猴細DLL 宕╂簝闃叉姢銆佽緭鍏ユ牎楠屻€侀攣浼樺寲
- Stop 鏀圭敤 instStop 绔嬪嵆鍋滄 + atomic 鏃犻攣璇绘秷闄ら攣绔炰簤
- UI spatial rules compliance + token system fixes

### Internal
- Wails v2 鈫?v3 鍏ㄥ钩鍙拌縼绉伙紙daq-p1604, five-hole-interpolator, motion-controller, three-hole-interpolator锛?- motion-controller 椤圭洰鑴氭墜鏋跺垵濮嬪寲鍜岀粨鏋勫畬鍠?- docs: README銆丼PEC銆丳LAN銆丄GENTS 鏂囨。琛ュ厖

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `wails build`

### Known Issues
- 鏆傛棤銆?
