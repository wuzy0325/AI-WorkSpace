# Changelog

## [0.2.0] - 2026-07-31

### Added

- **涓冨瓟鎺㈤拡鏍″噯妯″潡**锛?  - 鏂板 `LoadSevenHoleCalibrationCsvFiles` API锛屾敮鎸?7 涓?GBK 缂栫爜 CSV 鏂囦欢
    锛堝唴鍖?1 + 澶栧尯 6锛夊鍏ワ紝閫€鍖栬竟缂樿嚜鍔?dither锛屽鐢?PRB 鎶樼嚎娓叉煋鍣ㄥ睍绀烘牎鍑嗘洸绾匡紝
    骞惰繑鍥?warnings 鐢ㄤ簬閫€鍖栬竟缂樺彲瑙傛祴鎬с€?  - 鏂板 `PickSevenHoleFiles` / `GetSevenHoleDataSource` API锛?    鏀寔鍓嶇妲戒綅 UI 鏂囦欢閫夋嫨涓?PRB/CSV 鏍″噯鏁版嵁婧愬垏鎹€?  - 鍓嶇鏂板 `SevenHoleSlotRow.vue` 缁勪欢锛屼笌 wind-daq traversal-test 妲戒綅 UI 妯″紡涓€鑷达紝
    姣忎釜妲戒綅鐙珛閫夋嫨 PRB 鎴?CSV 鏍″噯鏂囦欢銆?  - 鍓嶇鏂板 `useSevenHoleCalibration.ts` composable锛?    灏佽鏍″噯娴佺▼鐘舵€佺鐞嗕笌 CSV 瀵煎叆浜や簰閫昏緫銆?  - 鏂板 `seven-hole-helpers.ts` 閫傞厤灞傝緟鍔╁嚱鏁颁笌 `seven-hole-slot-row.css` /
    `seven-hole-workspace.css` 鏍峰紡鏂囦欢銆?
### Changed

- **涓冨瓟 PRB 鍔犺浇娴佺▼閲嶆瀯**锛歚LoadSevenHolePrbFiles` 鎺ュ彈鍓嶇棰勫垎閰嶇殑
  inner + outer[6] 璺緞鏁扮粍锛屽彇浠ｅ悗绔閫夊璇濇 + 鍚庣 basename 璺敱鐨勬棫瀹炵幇锛?  涓?wind-daq traversal-test slot-UI 妯″紡瀵归綈銆?- **CSV 瀵煎叆閲嶆瀯**锛歚ImportSevenHoleCsvData` 鏀逛负 `csvFieldSetter` 鏁版嵁椹卞姩妯″紡锛?  鍙栦唬 9 涓嫭绔嬭В鏋愯皟鐢?+ 9 璺敊璇仈鍚堬紝闄嶄綆缁存姢鎴愭湰銆?- **涓冨瓟宸ヤ綔鍖洪噸鍐?*锛歚SevenHoleWorkspace.vue` 浠?947 琛岀簿绠€鑷崇害 440 琛岋紝
  鏍″噯閫昏緫杩佺Щ鑷?`useSevenHoleCalibration.ts` composable銆?- **`loadSevenHoleCalibrationCsvFiles` 绛惧悕**锛氫粠 `[6]string` 鏀逛负 `[]string`锛?  绉婚櫎璋冪敤鏂瑰啑浣欑殑鏁扮粍杞崲銆?- **build/config.yml `info.version`**锛氫粠 wails 妯℃澘榛樿鍊?`0.0.1` 淇涓?`0.2.0`锛?  瑙ｅ喅鍘嗗彶鐗堟湰涓嶅悓姝ラ棶棰橈紙鍚庣画 release 灏嗛殢 VERSION 涓€骞舵洿鏂帮級銆?
### Fixed

- **鏂囦欢瀵硅瘽妗嗚繃婊ゅ櫒涓嶅啀姹℃煋 CSV 瀵煎叆**锛?0ed7c8锛夛細
  `openFileDialog` 棰勮 PRB 杩囨护鍣紝CSV 瀵煎叆閾惧紡 `AddFilter` 鍙槸杩藉姞鑰岄潪鏇挎崲锛?  瀵艰嚧 CSV 瀵硅瘽妗嗗悓鏃舵樉绀?PRB 鏂囦欢绫诲瀷銆傛敼涓?`openFileDialog` 浠?`SetTitle`锛?  鍚勮皟鐢ㄦ柟鎸夊満鏅樉寮?`AddFilter`锛圥RB 鍔犺浇鍔?`.prb`锛孋SV 瀵煎叆鍔?`.csv/.txt/.dat`锛夈€?- **GetHelpDocPath 鍦?Windows dev 妯″紡涓嬫壘涓嶅埌 docs/**锛?0ed7c8锛夛細
  琛ュ厖 cwd 鍏滃簳鍙婁笂 4 绾х洰褰曟煡鎵撅紝骞舵牎楠屽懡涓槸鏂囦欢鑰岄潪鐩綍銆?- **搴旂敤鍥炬爣宓屽叆閿欒**锛坈162dcc锛夛細
  `main.go` 鐨?`//go:embed` 鐢?`appicon.png` 鏀逛负 `app_icon.png`锛?  瀵瑰簲鍥炬爣鏂囦欢閲嶅懡鍚嶏紝閬垮厤缂栬瘧鏃舵壘涓嶅埌 embed 鏂囦欢銆?- **NSIS installer 鏂囦欢鍚嶇己鍓嶇紑**锛坆dbed99锛夛細
  鏈湴 makensis 涓嶇粡杩?wails build锛宍wails_tools.nsh` 涓嶄細浠?`wails.json` 鍥炲～
  `INFO_PROJECTNAME` 绛夊彉閲忥紝榛樿绌哄瓧绗︿覆瀵艰嚧 installer 鏂囦欢鍚嶅彉鎴?  `-<version>-amd64-installer.exe`銆傛樉寮?`!define` 鎵€鏈?INFO_* 鍙橀噺淇銆?
### Internal

- `Taskfile.yml` 鏇存柊 build/release 娴佺▼锛岀Щ闄ゆ棫 `config.yml`锛?9 琛?wails 妯℃澘鍐椾綑閰嶇疆锛夈€?- `app_icon.png` 鏇挎崲涓洪珮鍒嗚鲸鐜囩増鏈紙135KB 鈫?1.19MB锛夛紝鍚屾 `windows/icon.ico`锛?1KB 鈫?107KB锛夈€?- `seven_hole_service_test.go` 鎵╁睍娴嬭瘯瑕嗙洊锛堟柊澧炴牎鍑?CSV 瀵煎叆涓庢Ы浣?UI 鐩稿叧娴嬭瘯锛夈€?- 涓変唤鐢ㄦ埛璇存槑涔?HTML 鍚屾鏇存柊锛?/3/7 瀛旓級銆?
### Verification

- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`: passed
- `$env:GOWORK="off"; go vet ./...`: passed
- `npm install --no-audit --no-fund`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis -DARG_WAILS_AMD64_BINARY=<exe> project.nsi`: passed锛圲TF-8 BOM 宸叉仮澶嶏級
- `task archive-release`: passed

### Known Issues

- 鏆傛棤銆?
## [0.1.0] - 2026-07-20

### Added

- 棣栨鍙戝竷锛氬皢 5 瀛?/ 3 瀛?/ 7 瀛旀帰閽堟彃鍊兼暣鍚堜负鍗曚竴妗岄潰绋嬪簭銆?- 鍚姩閫夋嫨椤碉細3 涓崱鐗囨寜閽紝session-locked锛堥噸鍚悗鍒囨崲鎺㈤拡绫诲瀷锛夈€?- 涓変釜鐙珛宸ヤ綔鍖虹粍浠讹細`FiveHoleWorkspace.vue` / `ThreeHoleWorkspace.vue` / `SevenHoleWorkspace.vue`锛?  閫氳繃 `App.vue` 鍔ㄦ€?`import()` 鎳掑姞杞斤紝閬垮厤鍚姩鏃朵竴娆℃€у姞杞藉叏閮ㄣ€?- 鍏变韩绠楁硶鍖呭紩鐢細
  - `shared/algorithms/go/fivehole/interpolation`
  - `shared/algorithms/go/threehole/interpolation`
  - `shared/algorithms/go/sevenhole/interpolation`
- 涓夊鐢ㄦ埛璇存槑涔︼紙HTML锛夛細`five-hole-鐢ㄦ埛璇存槑涔?html` / `three-hole-鐢ㄦ埛璇存槑涔?html` / `seven-hole-鐢ㄦ埛璇存槑涔?html`銆?- 鎺㈤拡閫夋嫨鍣紙`probe_selector.go`锛夛細鍩轰簬 `sync.RWMutex` 鐨勫苟鍙戝畨鍏ㄥ疄鐜帮紝
  姣忎釜 probe 绫诲瀷鐙珛 state锛堝惈鑷繁鐨?`sync.RWMutex`锛夛紝閬垮厤閿佹贩鐢ㄣ€?- 7 瀛斾笓灞?API锛歚LoadSevenHolePrbFiles` / `CalculateSevenHole` / `BatchCalculateSevenHole` 绛?8 涓柟娉曪紝
  鎵€鏈夌被鍨嬪姞 `SevenHole` 鍓嶇紑閬垮厤 Wails binding 鐢熸垚鍐茬獊銆?- 7 瀛斿悗绔祴璇曞浠讹細11 涓祴璇曞嚱鏁拌鐩栧唴鍖?澶栧尯鎻掑€笺€佹壒閲忚绠椼€佸苟鍙戝畨鍏ㄣ€丳RB 鍔犺浇绛夊満鏅紝
  浣跨敤绠楁硶鍖?golden test data锛坄boundary.json`锛変綔涓烘潈濞佽緭鍏ヨ緭鍑哄銆?
### Behavior

- 5 瀛?/ 3 瀛斿伐浣滃尯涓庢棫鐙珛绋嬪簭鍔熻兘绛変环锛歅RB 鏂囦欢鏍煎紡銆丆SV 杈撳叆/杈撳嚭鏍煎紡銆佸帇鍔涙ā寮忓紑鍏冲畬鍏ㄥ吋瀹广€?- 7 瀛斿伐浣滃尯閬靛惊 spec 搂1.1锛氬己鍒惰〃鍘嬭緭鍏ワ紝涓嶆彁渚?PressureMode 鍒囨崲寮€鍏筹紱
  CSV 蹇呭惈 P1-P7 + Patm + Tatm 鍏?9 鍒楋紝鍏ㄩ儴蹇呴渶銆?- 7 瀛旂粨鏋?伪/尾 璇箟涓?5 瀛斿弽杞紙spec 搂2.2锛夛細伪=渚ф粦瑙掋€佄?杩庤锛孋SV 瀵煎嚭琛ㄥご鏄庣‘鏍囨敞鐗╃悊鍚箟銆?- 7 瀛?PRB 鍔犺浇瑕佹眰鏂囦欢鍚?basename 涓?1~7 鐨勭函鏁板瓧锛?.prb=鍐呭尯锛?~6.prb=澶栧尯鎵囧尯 n锛夈€?
### Deprecation

- 鏃х嫭绔嬬▼搴?`projects/five-hole-interpolator` 涓?`projects/three-hole-interpolator` 鏍囪涓?deprecated銆?- 鏃ч」鐩粨搴撲唬鐮佷笌鍘嗗彶 release 鍒跺搧淇濈暀锛屼笉鍐嶅彂甯冩柊鐗堟湰鎴栦慨澶嶇己闄枫€?- 鐢ㄦ埛搴旇縼绉昏嚦鏈」鐩紙`probe-interpolator`锛夎繘琛屽悗缁紑鍙戜笌浣跨敤銆?
### Verification

- `GOWORK=off; go build ./...`: passed
- `GOWORK=off; go vet ./...`: passed
- `GOWORK=off; go test -race ./backend/...`: passed锛堝惈 7 瀛?11 涓祴璇?+ 5 瀛?/ 3 瀛旀棦鏈夋祴璇曪級
- `npm --prefix frontend run typecheck`: passed
- `npm --prefix frontend run build`: passed

### Known Issues

- 鏆傛棤銆?

## [0.1.0] - 2026-07-20

### Added

- 棣栨鍙戝竷锛氬皢 5 瀛?/ 3 瀛?/ 7 瀛旀帰閽堟彃鍊兼暣鍚堜负鍗曚竴妗岄潰绋嬪簭銆?- 鍚姩閫夋嫨椤碉細3 涓崱鐗囨寜閽紝session-locked锛堥噸鍚悗鍒囨崲鎺㈤拡绫诲瀷锛夈€?- 涓変釜鐙珛宸ヤ綔鍖虹粍浠讹細`FiveHoleWorkspace.vue` / `ThreeHoleWorkspace.vue` / `SevenHoleWorkspace.vue`锛?  閫氳繃 `App.vue` 鍔ㄦ€?`import()` 鎳掑姞杞斤紝閬垮厤鍚姩鏃朵竴娆℃€у姞杞藉叏閮ㄣ€?- 鍏变韩绠楁硶鍖呭紩鐢細
  - `shared/algorithms/go/fivehole/interpolation`
  - `shared/algorithms/go/threehole/interpolation`
  - `shared/algorithms/go/sevenhole/interpolation`
- 涓夊鐢ㄦ埛璇存槑涔︼紙HTML锛夛細`five-hole-鐢ㄦ埛璇存槑涔?html` / `three-hole-鐢ㄦ埛璇存槑涔?html` / `seven-hole-鐢ㄦ埛璇存槑涔?html`銆?- 鎺㈤拡閫夋嫨鍣紙`probe_selector.go`锛夛細鍩轰簬 `sync.RWMutex` 鐨勫苟鍙戝畨鍏ㄥ疄鐜帮紝
  姣忎釜 probe 绫诲瀷鐙珛 state锛堝惈鑷繁鐨?`sync.RWMutex`锛夛紝閬垮厤閿佹贩鐢ㄣ€?- 7 瀛斾笓灞?API锛歚LoadSevenHolePrbFiles` / `CalculateSevenHole` / `BatchCalculateSevenHole` 绛?8 涓柟娉曪紝
  鎵€鏈夌被鍨嬪姞 `SevenHole` 鍓嶇紑閬垮厤 Wails binding 鐢熸垚鍐茬獊銆?- 7 瀛斿悗绔祴璇曞浠讹細11 涓祴璇曞嚱鏁拌鐩栧唴鍖?澶栧尯鎻掑€笺€佹壒閲忚绠椼€佸苟鍙戝畨鍏ㄣ€丳RB 鍔犺浇绛夊満鏅紝
  浣跨敤绠楁硶鍖?golden test data锛坄boundary.json`锛変綔涓烘潈濞佽緭鍏ヨ緭鍑哄銆?
### Behavior

- 5 瀛?/ 3 瀛斿伐浣滃尯涓庢棫鐙珛绋嬪簭鍔熻兘绛変环锛歅RB 鏂囦欢鏍煎紡銆丆SV 杈撳叆/杈撳嚭鏍煎紡銆佸帇鍔涙ā寮忓紑鍏冲畬鍏ㄥ吋瀹广€?- 7 瀛斿伐浣滃尯閬靛惊 spec 搂1.1锛氬己鍒惰〃鍘嬭緭鍏ワ紝涓嶆彁渚?PressureMode 鍒囨崲寮€鍏筹紱
  CSV 蹇呭惈 P1-P7 + Patm + Tatm 鍏?9 鍒楋紝鍏ㄩ儴蹇呴渶銆?- 7 瀛旂粨鏋?伪/尾 璇箟涓?5 瀛斿弽杞紙spec 搂2.2锛夛細伪=渚ф粦瑙掋€佄?杩庤锛孋SV 瀵煎嚭琛ㄥご鏄庣‘鏍囨敞鐗╃悊鍚箟銆?- 7 瀛?PRB 鍔犺浇瑕佹眰鏂囦欢鍚?basename 涓?1~7 鐨勭函鏁板瓧锛?.prb=鍐呭尯锛?~6.prb=澶栧尯鎵囧尯 n锛夈€?
### Deprecation

- 鏃х嫭绔嬬▼搴?`projects/five-hole-interpolator` 涓?`projects/three-hole-interpolator` 鏍囪涓?deprecated銆?- 鏃ч」鐩粨搴撲唬鐮佷笌鍘嗗彶 release 鍒跺搧淇濈暀锛屼笉鍐嶅彂甯冩柊鐗堟湰鎴栦慨澶嶇己闄枫€?- 鐢ㄦ埛搴旇縼绉昏嚦鏈」鐩紙`probe-interpolator`锛夎繘琛屽悗缁紑鍙戜笌浣跨敤銆?
### Verification

- `GOWORK=off; go build ./...`: passed
- `GOWORK=off; go vet ./...`: passed
- `GOWORK=off; go test -race ./backend/...`: passed锛堝惈 7 瀛?11 涓祴璇?+ 5 瀛?/ 3 瀛旀棦鏈夋祴璇曪級
- `npm --prefix frontend run typecheck`: passed
- `npm --prefix frontend run build`: passed

### Known Issues

- 鏆傛棤銆?
