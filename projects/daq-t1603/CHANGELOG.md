# Changelog

## [0.6.0] - 2026-07-27

### Added
- 鐙珛搴旂敤鏂板涓嫳鏂囩晫闈㈠垏鎹紝骞朵繚鎸佽瑷€鍋忓ソ銆?
### Fixed
- 璁惧杩炴帴澶辫触鍐欏叆鏄庣‘閿欒鏃ュ織锛屼究浜庣幇鍦哄畾浣嶅湴鍧€銆佺綉缁滃拰鍗忚闂銆?- 澶嶇敤鍏变韩 SDK 鐨勮繛缁潤榛樼獥鍙ｆ帓绌洪€昏緫锛岄檷浣庡揩閫熷惎鍋滃悗鐨勬畫鐣欏抚姹℃煋椋庨櫓銆?
### Verification
- `$env:GOWORK="off"; go test ./...`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 鏆傛棤銆?
## [0.5.0] - 2026-07-25

### Added
- 鏂板鍚庣 UI 鍒锋柊鐜囧姩鎬佸彲璋冿紙SetUIRefreshRateHz锛夛細鍓嶇 MainTopBar 鐨勫埛鏂扮巼涓嬫媺椤瑰疄鏃跺悓姝ュ埌鍚庣 `uiPayloadRefreshInterval`锛孉pp.onMounted 鍚姩鏃朵篃鍚屾 localStorage 鍋忓ソ锛涘悗绔?relayStream 鐢?`Ticker.Reset` 鍔ㄦ€佽窡闅忥紝涓嶅啀纭紪鐮?100ms銆?
### Changed
- drainConnection 鏀逛负銆岃繛缁?2 涓潤榛樼獥鍙ｆ墠閫€鍑恒€嶈涔夛細鍘熷崟娆￠潤榛樺嵆杩斿洖浼氬湪蹇€熷惎鍋滃悗娈嬬暀甯у熬琚笅涓€娆″懡浠よ鍙栧綋浣滃搷搴斾贡鐮侊紱鐜拌姹傝繛缁袱娆?SetReadDeadline 瓒呮椂鎵嶇粨鏉?drain锛岄伩鍏嶈鍒ゆ畫鐣欏抚灏俱€?- 绉婚櫎 MonitorView 杩炴帴鎸夐挳鏈娇鐢ㄧ殑 Loader2 鍥炬爣涓?.spin 鍔ㄧ敾锛屼粎淇濈暀 Connecting 鎬?loading锛屽噺灏戣瑙夊櫔闊炽€?
### Fixed
- 淇 UI 鍒锋柊鐜囪缃笉鐢熸晥锛氬悗绔?`uiPayloadRefreshInterval` 纭紪鐮?100ms 瀵艰嚧 10Hz鈫?0Hz 鏃犲彉鍖栥€?0Hz鈫?Hz 浠呭浘琛ㄥ彉鎱㈣€屾暟鍊煎崱浠?10Hz 鏇存柊銆傛敼涓哄師瀛愬彉閲?+ Ticker.Reset 鍔ㄦ€佽窡闅忋€?
### Internal
- 鏂板 `drainConnection` 杩炵画闈欓粯绐楀彛鍥炲綊娴嬭瘯 `TestDAQT1603DrainConnectionWaitsForDelayedFrameTail`锛岄獙璇?150ms 寤惰繜鍒拌揪鐨勫抚灏句笉浼氳璇涓哄懡浠ゅ搷搴斻€?- 鏂板 Taskfile.yml锛屼笌 daq-p1604 瀵归綈锛氬畾涔?`clean / build-frontend / build-go / release / archive-release / check-bindings / generate-icon` 浠诲姟锛屼究浜庡悗缁?release 娴佺▼缁熶竴銆?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.5.0锛歏ERSION銆乤pps/desktop-wails/wails.json銆乤pps/desktop-wails/frontend/package.json銆乤pps/desktop-wails/frontend/package-lock.json銆乤pps/desktop-wails/build/config.yml銆乤pps/desktop-wails/build/windows/installer/project.nsi銆?
### Verification
- 鐢熶骇 Go 鏋勫缓 `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`锛氶€氳繃锛屼骇鍑?`build/bin/daq-t1603.exe`銆?- `go vet ./...`锛圙OWORK=off锛夛細passed銆?- `go test ./...`锛圙OWORK=off锛夛細passed锛坅dapters/config銆乤dapters/hardware銆乤dapters/recording銆乽secase 鍧?ok锛夈€?- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-t1603.exe project.nsi`锛氫骇鍑?`daq-t1603-0.5.0-amd64-installer.exe`锛屽綊妗ｈ嚦 `releases/bin/`銆?- 宸茬煡闄愬埗锛歟xe 鑷韩 Windows 鐗堟湰璧勬簮鍥哄畾涓?`0.0.0.0`锛坵ails v3 alpha `generate syso` 闄愬埗锛屼笌鍘嗗彶 0.3.x/0.4.x 涓€鑷达級锛涘畨瑁呭寘 VIProductVersion 宸叉纭爣娉?0.5.0銆侴UI 鍐掔儫娴嬭瘯寤鸿鍦ㄧ洰鏍囨満鎵嬪姩楠岃瘉銆?
### Known Issues
- 鏆傛棤銆?
## [0.4.0] - 2026-07-24

### Added
- 鏂板寮傛璁惧鐘舵€佷簨浠舵帹閫侊紙ACQ-010/STB-003锛夛細OnReadLoopExit 鈫?hub 鈫?service 寮傛鎺ㄩ€侊紝UI 鐘舵€佹洿鏂版洿鍙婃椂锛岄伩鍏嶉樆濉為噰闆嗙儹璺緞銆?- 鏂板绂佺敤閫氶亾绌?CSV 鍒楄緭鍑猴紙REC-006锛夛細绂佺敤閫氶亾鍦?CSV 涓緭鍑虹┖鍒楋紝淇濇寔鍒楅『搴忎笌琛ㄥご涓€鑷淬€?- 鏂板鏃ュ織闈㈡澘鎼滅储涓庢棩蹇楁枃浠惰疆杞紙LOG-010/015锛夛細鍓嶇鏃ュ織鍙叧閿瓧鎼滅储锛屽悗绔棩蹇楁寜澶у皬杞浆銆?- 閫傞厤鍣ㄦ墿灞曟敮鎸佹柊 SDK 鎺ュ彛锛堥厤鍚?shared/device-sdk/go/daq/hardware/daq_t1603.go 鐨勬帴鍙ｆ墿灞曪級锛屼负鍚庣画鑳藉姏鎵╁睍閾鸿矾銆?
### Changed
- 閰嶇疆鑴忕姸鎬佹牎姝ｏ紙CFG-017锛夛細纭欢閰嶇疆涓?profile 涓嶄竴鑷存椂鑴忔爣璁版洿鍑嗙‘銆?- 鍓嶇 ChannelCard 鍚屾绉婚櫎鏁板€煎彉鍖栭棯鐑佸姩鐢伙紙瑙嗚鍣煶锛夈€?- CSV 琛ㄥご鍒楃敱 20 鍒楋紙DeviceID,Timestamp,Millisecond,Unit,CH01..CH16锛夋敼涓?18 鍒楋紙Timestamp,Unit,CH01..CH16锛夈€?  DeviceID 鍒楃Щ闄わ紙鏂囦欢鍚嶅凡鍚澶?ID锛夛紱Timestamp 鍒椾粎淇濈暀绉掔骇绮惧害锛?YYYY-MM-DD HH:MM:SS锛夛紝
  涓?0.3.2 鍐崇瓥涓€鑷粹€斺€?000Hz 閲囬泦鏃跺悓涓€绉掑唴鐨勬牱鏈叡浜悓涓€鏃堕棿鎴筹紝涓嶅啀鍖哄垎姣銆?
### Fixed
- 淇搴旂敤閫€鍑洪樁娈?readLoop 鏀跺熬鏃?EmitDeviceState 鍦ㄥ凡鍏抽棴 app 涓?panic锛歞evice_service.ServiceShutdown 娓呯┖ s.app锛孍mitDeviceState 鍔?recover 淇濇姢銆?- 淇 CSV 褰曞埗 Stop鈫扴tart 浼氳瘽闂寸鐢ㄩ€氶亾鎺╃爜娉勬紡锛歝sv_recorder.Stop 娓呯悊 deviceProfiles锛岄伩鍏嶄笂娆′細璇濈殑绂佺敤閫氶亾鎺╃爜姹℃煋鏂颁細璇濄€?- 淇閿欒淇℃伅鍖归厤璇垽锛坈onnection pool exhausted / permission_token 绛夐潪鐩爣鍦烘櫙琚鍒や负杩炴帴閿欒锛夛細recordingStore / DaqT1603Config 鏀圭敤 `\b` 鍗曡瘝杈圭晫姝ｅ垯銆?
### Internal
- 鏂板 csv_recorder_rec006_test.go銆乨evice_usecase_validation_test.go銆?- 鍚屾 bindings锛圗mitDeviceState / SetDeviceProfile 瀵煎嚭锛夊埌 daq-t1603 .ts bindings锛坵ails3 杩愯鏃朵細閲嶆柊鐢熸垚涓?.js 骞朵涪寮冩彁浜ょ殑 .ts锛夈€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.4.0锛歏ERSION銆乤pps/desktop-wails/wails.json銆乤pps/desktop-wails/frontend/package.json銆乤pps/desktop-wails/frontend/package-lock.json銆乤pps/desktop-wails/build/config.yml銆乤pps/desktop-wails/build/windows/installer/project.nsi銆?
### Verification
- 鐢熶骇 Go 鏋勫缓 `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`锛氶€氳繃锛屼骇鍑?`build/bin/daq-t1603.exe`銆?- `go vet ./...`锛圙OWORK=off锛夛細passed銆?- `go test ./...`锛圙OWORK=off锛夛細passed锛坅dapters/config銆乤dapters/hardware銆乤dapters/recording銆乽secase 鍧?ok锛夈€?- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-t1603.exe project.nsi`锛氫骇鍑?`daq-t1603-0.4.0-amd64-installer.exe`锛屽綊妗ｈ嚦 `releases/bin/`銆?- SHA-256锛歚22e82689af05ca0ca06fb577a6cd0cb709481a98c9320db4ade86f86ea8a0803`銆?- 宸茬煡闄愬埗锛歟xe 鑷韩 Windows 鐗堟湰璧勬簮鍥哄畾涓?`0.0.0.0`锛坵ails v3 alpha `generate syso` 闄愬埗锛屼笌鍘嗗彶 0.3.x 涓€鑷达級锛涘畨瑁呭寘 VIProductVersion 宸叉纭爣娉?0.4.0銆侴UI 鍐掔儫娴嬭瘯寤鸿鍦ㄧ洰鏍囨満鎵嬪姩楠岃瘉銆?
### Known Issues
- 鏆傛棤銆?
## [0.3.3] - 2026-07-03

### Fixed

- 淇鍋滄閲囬泦鍚庣珛鍗抽厤缃弬鏁版椂鍛戒护鍝嶅簲涔辩爜鎴栧け璐ョ殑闂銆傚仠姝㈤噰闆嗗悗 TCP 缂撳啿鍖烘畫鐣欓噰闆嗘暟鎹抚锛孉pplyDaqT1603Config 鐨?sendCommand 鎶婃畫鐣欏綋浣滃懡浠ゅ搷搴旇鍑恒€傚湪 stopAcquisitionLocked 鍋滄鍛戒护鍚庡鍔?drainConnection 鎺掔┖娈嬬暀鏁版嵁锛涘湪 ApplyDaqT1603Config 璋冪敤 applyHardwareConfig 鍓嶅鍔?drainConnection銆?
### Internal

- 淇鐐逛綅浜?shared/device-sdk/go/daq/hardware/daq_t1603.go锛寃ind-daq 鐨?T1603 璁惧鍚屾牱鍙楃泭銆?
### Verification

- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `$env:GOWORK="off"; go vet ./...`
- `task release`锛堟墜鍔?fallback锛歚go build -tags production` + `makensis`锛宒aq-t1603 鏃?Taskfile锛?
### Known Issues

- 鏆傛棤銆?
## [0.3.2] - 2026-07-03

### Fixed
- 淇 CSV Timestamp 鍒楁椂闂存埑绮惧害闂锛氳法椤圭洰瀵归綈鏃堕棿鎴虫牸寮忓彉鏇达紝缁熶竴鎴柇鍒扮绾э紙`'YYYY-MM-DD HH:MM:SS`锛夛紝閬垮厤灞曠ず閿欒鐨勬椂闂寸粏鍒嗐€傚師鍥犺瑙?daq-p1604 v0.2.2 release note锛圖AQ-P-1604 璁惧纭欢鏃堕棿鎴冲浐浠?bug锛夈€?
### Verification
- `$env:GOWORK="off"; go build ./...`: passed锛坉aq-t1603 宸ヤ綔绌洪棿闅旂锛岃 ADR-006锛?- `$env:GOWORK="off"; go vet ./adapters/recording/...`: passed
- `$env:GOWORK="off"; go test ./adapters/recording/...`: passed

### Known Issues
- 鏆傛棤銆?
## [0.3.1] - 2026-07-02

### Internal
- 淇鏋勫缓閰嶇疆鏂囦欢鐗堟湰婊炲悗锛歜uild/config.yml銆乥uild/info.json銆乸roject.nsi 鍚屾鍒?0.3.1銆?- v0.3.0 鐨?NSIS 瀹夎鍖呮湭姝ｇ‘鐢熸垚锛屾湰娆¤ˉ鍏ㄣ€?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed
- `makensis`: passed
- 鍐掔儫娴嬭瘯: passed锛圙UI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒锛?
### Known Issues
- 鏆傛棤銆?
## [0.3.0] - 2026-07-02

### Added
- 鏂板纭欢閫氫俊鏃ュ織锛氶┍鍔ㄥ眰 hardware-send/hardware-recv 鐨?debug 鏃ュ織鎻愬崌涓?info锛屽墠绔€氫俊鍒嗙粍鍙瀹屾暣鍛戒护浜や簰娴佺▼锛圱CP 杩炴帴/鏂紑銆丂fd/@fe/@f0 绛夊懡浠ょ殑鍙戦€佷笌鍝嶅簲锛夈€傞噰闆嗘湡闂寸殑浜岃繘鍒舵暟鎹抚涓嶆墦鍗帮紝閬垮厤楂橀鍒峰睆銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- 鍐掔儫娴嬭瘯: passed锛圙UI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒锛?
### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-07-01

### Added
- 鏂板褰曞埗鑳屽帇澶勭悊绯荤粺锛欱ackpressureEvent 鍚槦鍒楅暱搴?瀹归噺/绱涓㈠抚鏁般€?- 鏂板 SetBackpressureHandler / SetFatalErrorHandler 鍥炶皟锛屽綍鍒堕槦鍒楅ケ鍜屾垨 I/O 閿欒鏃堕潪闃诲閫氱煡銆?- RecordingSession 鏂板 DroppedCount 瀛楁锛屽墠绔彲鐩戞帶鏁版嵁瀹屾暣鎬с€?- 鏂板 LogFileState 绫诲瀷锛屽墠绔彲鏌ヨ鏃ュ織鏂囦欢鍐欏叆鐘舵€併€?- RecordingService 鏂板鑳屽帇/fatal 浜嬩欢闄愰锛堟瘡绫?1Hz锛夛紝閬垮厤浜嬩欢鍒峰睆銆?
### Changed
- CSV 褰曞埗鍣ㄩ噸鍐欙細鏀寔澶氳澶囩嫭绔嬪啓鍏ャ€侀潪闃诲寮傛闃熷垪銆佹枃浠舵粴鍔ㄣ€?- 鍚庣閲嶆瀯锛氬垹闄?monolithic app.go锛屾媶鍒?relayStream 鍒?DeviceService銆?- RecordingService 娉ㄥ叆鑳屽帇鍜岃嚧鍛介敊璇洖璋冿紝Hub EmitLog 寮傛骞挎挱銆?- 鍓嶇 App.vue/stores 閫傞厤鏂扮殑褰曞埗鐘舵€佸拰鑳屽帇浜嬩欢銆?
### Internal
- freqprobe 璋冭瘯宸ュ叿灏忓箙璋冩暣銆?- AGENTS.md 鍜?README.md 琛ュ厖 Release Commands 娈点€?- go.mod 鏇存柊渚濊禆銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 鍥?go.sum 缂哄け涓嶅彲鐢紝鏀圭敤 go build 鐩村嚭)
- `makensis` 鏋勫缓瀹夎鍖? passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)

### Known Issues
- 鏆傛棤銆?
## [0.1.4] - 2026-06-29

### Added
- 鏂板 Wails 妗岄潰 App backend 瀹炵幇锛氳澶囩鐞嗐€佹棩蹇椼€佸綍鍒舵湇鍔℃暣鍚堝埌缁熶竴妗岄潰搴旂敤銆?
### Internal
- AGENTS.md 鍜?README.md 鍖哄垎寮€鍙戜笌浜や粯鍛戒护锛屾柊澧?Release Commands 娈点€?- AGENTS.md 澧炲姞 ADR-004 绱㈠紩銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.1.3] - 2026-06-23

### Fixed
- 淇閫傞厤鍣ㄦ閿侊細`StopAcquisition`/`Disconnect` 灏嗙‖浠?I/O 绉诲埌閿佸鎵ц锛岄伩鍏嶄笌 `OnReadLoopExit`/`OnConfigSynced` 鍥炶皟鐩镐簰姝婚攣銆?- 淇 readLoop 寮傚父閫€鍑哄悗椹卞姩鏈柇寮€鐨勯棶棰橈細寮傚父閫€鍑烘椂娓呯悊 drivers 琛ㄥ苟璋冪敤 `driver.Disconnect()`锛岄槻姝笅娆?StartAcquisition 鍦ㄥ潖杩炴帴涓婇噸璇曘€?- 淇 Status() 鐘舵€佸崱鍦?Acquiring 鐨勯棶棰橈細绉婚櫎鏈夌己闄风殑 `st.Status != StatusAcquiring` 瀹堝崼锛屾敼涓轰俊浠婚┍鍔ㄥ眰鐘舵€侊紙true source of truth锛夈€?- 淇 CSV 褰曞埗鏁版嵁涓㈠け锛歳elayStream 鏀逛负姣忔潯 snapshot 鍗虫椂鍐欏叆锛堟鍓嶆瘡绉掑彧鍐欎竴娆℃渶鏂板€硷級锛屽苟缁撳悎 `IsActive()` 鏃犻攣鐑矾寰勯伩鍏嶉攣绔炰簤銆?- 淇鍓嶇鎵弿鍒楄〃閲嶅娣诲姞闂锛歋canResultList 鏄剧ず"宸叉坊鍔?鐘舵€佹爣璁帮紝store 灞傚鍔犻噸澶嶆坊鍔犻槻寰°€?- 淇璁剧疆鍛戒护鍚庢鐜囬噰闆嗘暟鎹В鏋愪贡鐮併€?
### Changed
- CSV flush 琛岄槇鍊间粠 100 鈫?2000锛岃 1s 鏃堕棿闂撮殧涓诲 flush 棰戠巼锛屽噺灏戝璁惧楂橀鍦烘櫙涓嬬殑纾佺洏鍚屾娆℃暟銆?- CSV `FormatFloat` 鏇夸唬 `fmt.Sprintf`锛屾秷闄ゆ瘡绉?16000 娆℃牸寮忎覆瑙ｆ瀽寮€閿€銆?- CSV `sync.Mutex` 鏇夸唬 `sync.RWMutex`锛圫tatus 璋冪敤棰戠巼浣庯紝璇诲啓閿侀澶栧紑閿€涓嶅€煎緱锛夈€?- 閲囬泦 channel 缂撳啿鍖轰粠 8192 鈫?65536锛屽湪 1000Hz 涓嬫彁渚涚害 65 绉掔紦鍐诧紝闃叉 CSV flush 闃诲鍙嶅帇鍒扮‖浠?readLoop銆?- 鍓嶇纭欢鏃堕棿鎴虫枃妗堬細"鏄剧ず/闅愯棌" 鈫?"鍚敤/绂佺敤"銆?- E2E 娴嬭瘯 fixture 榛樿鍚敤 `showTimestamp: true`銆?- 鏂囨。鏂囦欢 `MANUAL_TEST.md`銆乣TEST_PLAN.md`銆乣test-cases.html` 绉昏嚦 `docs/` 鐩綍銆?
### Removed
- 绉婚櫎妯℃嫙妯″紡锛氬垹闄?`SimulatedAdapter`銆乣SimulatedScanner` 鍙婂叾娴嬭瘯锛坄simulated_adapter_test.go`銆乣app_test.go`銆乣simulated_flow_test.go`锛夈€?- 绉婚櫎 `DAQ_T1603_MODE` 鐜鍙橀噺鍒嗘敮锛坢ain.go 纭紪鐮佷娇鐢?T1603Adapter锛夈€?- 娓呯悊涓存椂鏂囦欢銆佹祴璇曚骇鐗╁拰搴熷純鐨?threehole 浠ｇ爜銆?
### Internal
- `RecordingPort` 鎺ュ彛鏂板 `IsActive() bool`锛宍CSVRecorder` 鍜?`RecordingUsecase` 鍒嗗埆瀹炵幇鏃犻攣鐑矾寰勩€?- `frameprobe` 璋冭瘯宸ュ叿閲嶅啓锛氭敮鎸佷簩杩涘埗甯цВ鏋愬拰褰掍竴鍖栭厤缃煡璇€?- `deviceStore` 鏂板 `isScanResultAdded` 鏂规硶锛屽熀浜?IP:Port 鍘婚噸銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?
## [0.1.2] - 2026-06-18

### Fixed
- 淇纭欢鍛戒护鍗忚闂锛氱Щ闄?`SendCommand` 绯诲垪鍑芥暟涓拷鍔犵殑 `\n`锛岃澶囩洿鎺ユ帴鏀跺師濮嬪懡浠ゅ瓧绗︿覆銆?- 淇鍓嶇鐘舵€侀棯鐑侊細MonitorView 鍜?DeviceSidebar 琛ュ厖 "Starting" 鐘舵€佺殑澶勭悊锛岃繛鎺?鏂紑鎸夐挳鍦ㄨ繃娓＄姸鎬佹樉绀哄姞杞藉姩鐢汇€?- 淇棰戠巼鏄剧ず閿欒锛歁onitorView 鏀逛负浠?`t1603Config.samplingRate` 璇诲彇閲囨牱棰戠巼锛堣€岄潪椤跺眰 `samplingRate` 瀛楁锛夈€?- 澧炲己甯цВ鏋愬仴澹€э細`parseSpaceSeparatedFrame` 鏀寔 17 token 鍏冩暟鎹牸寮忥紙TIME 鎴?HEAD 鍗曠嫭鍚敤锛夛紝鏂板 `Resync()` 鏂规硶鐢ㄤ簬甯у亸绉诲悗閲嶅悓姝ャ€?
### Changed
- 閲囨牱鐜囪緭鍏ヤ粠涓嬫媺閫夋嫨妗嗘敼涓鸿嚜鐢辨暟瀛楄緭鍏ユ锛?-1000Hz锛夛紝鏀寔浠绘剰鏁存暟鍊笺€?- 鏂板 `metadataMode` 鏀寔锛歚T1603FrameReader` 鍙牴鎹?TIME/HEAD 閰嶇疆鍒囨崲鍥哄畾甯?鍙橀暱甯фā寮忋€?- 娴嬭瘯鐢ㄤ緥鏂囨。鏇存柊锛氱鍙ｄ粠 5000鈫?000锛岀Щ闄ゆā鎷熸ā寮忓紩鐢紝鏀圭敤鍗＄墖寮忓竷灞€銆?
### Internal
- 鏂板 E2E 娴嬭瘯杈呭姪锛歮ock-bridge 鏀寔澶氳澶囧苟鍙戯紝AppPage Page Object Model 鍜岄€夋嫨鍣?data-testid 灞炴€с€?- 娓呯悊 DaqT1603Config.vue 涓湭浣跨敤鐨?`samplingRateOptions` 鍜?`samplingRateSelectOptions`銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?
## [0.1.1] - 2026-06-10

### Fixed
- 绉婚櫎鍓嶇 TC_RANGES 姝讳唬鐮佸拰鏈娇鐢ㄧ殑 updateT1603Config 鍑芥暟銆?- 灏?閲囬泦涓笉涓嬪彂纭欢閰嶇疆"鐨勫垽鏂€昏緫浠庡墠绔Щ鍒板悗绔?usecase锛屽墠绔笉鍐嶅寘鍚‖浠惰涓虹煡璇嗐€?- 淇鍓嶇瑙ｆ瀽璁惧鐘舵€佹椂瀵瑰悗绔暟鍊兼灇涓剧殑渚濊禆锛屾敼涓轰娇鐢ㄥ悗绔洿鎺ヨ繑鍥炵殑 statusText 瀛楃涓层€?- 淇閰嶇疆淇濆瓨鏃剁‖浠跺簲鐢ㄩ敊璇绌?catch 鍚炴病鐨勯棶棰樸€?
### Changed
- DeviceState 鏂板 StatusText 瀛楁鍜?SetStatus() 杈呭姪鏂规硶锛屾墍鏈夐€傞厤鍣ㄩ€氳繃璇ユ柟娉曠粺涓€璁剧疆鐘舵€侊紝閬垮厤 Status/StatusText 涓嶄竴鑷淬€?- 閰嶇疆淇濆瓨鎴愬姛娑堟伅鏍规嵁纭欢涓嬪彂缁撴灉鍔ㄦ€佹樉绀恒€?
### Internal
- 閲嶆瀯妯℃嫙閫傞厤鍣ㄥ拰 T1603 閫傞厤鍣ㄤ腑鍏ㄩ儴鐘舵€佽祴鍊兼搷浣滐紝缁熶竴浣跨敤 SetStatus()銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed (pre-existing freqprobe IPv6 warning unrelated)
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?

## [0.5.0-win7.1] - 2026-07-25

### Added

- **drainConnection 杩炵画闈欓粯绐楀彛鍥炲綊娴嬭瘯**锛堝悓姝?master 0.5.0锛夛細鏂板 `TestDAQT1603DrainConnectionWaitsForDelayedFrameTail`锛岄獙璇?150ms 寤惰繜鍒拌揪鐨勬畫鐣欏抚灏句笉浼氳璇涓哄懡浠ゅ搷搴斻€傚悓姝ュ皢鏃㈡湁 3 澶?`readWithTimeout(server, 200*time.Millisecond)` 鏇挎崲涓?`testReadTimeout = time.Second`锛屼笌 drainConnection 鎬昏€楁椂涓婇檺锛坢axIters=10 脳 100ms = 1s锛夊榻愶紝閬垮厤娴嬭瘯渚ц秴鏃剁煭浜庤娴嬩唬鐮佸鑷磋鍒ゃ€?
### Changed

- **drainConnection 鏀逛负杩炵画 2 涓潤榛樼獥鍙ｆ墠閫€鍑?*锛堝悓姝?master 0.5.0锛夛細鍘熷崟娆￠潤榛樺嵆杩斿洖浼氬湪蹇€熷惎鍋滃悗娈嬬暀甯у熬琚笅涓€娆″懡浠よ鍙栧綋浣滃搷搴斾贡鐮侊紱鐜拌姹傝繛缁袱娆?`SetReadDeadline` 瓒呮椂鎵嶇粨鏉?drain锛岀‘淇濆欢杩熷埌杈剧殑甯у熬琚惛鏀躲€傛敼鍔ㄤ綅浜?`shared/device-sdk/go/daq/hardware/daq_t1603.go`锛寃ind-daq 鐨?T1603 璁惧鍚屾牱鍙楃泭銆?- 鐗堟湰鍙峰悓姝ヨ嚦 `0.5.0-win7.1`锛氫笌 master 0.5.0 涓荤増鏈彿瀵归綈锛屼繚鐣?`-win7.1` 鍚庣紑浠ユ爣璇?Win7 LTS 鍏煎鐗堟湰銆傚悓姝?`apps/desktop-electron/package.json` 涓?`frontend/package.json`銆?
### Internal

- 涓?0.4.0-win7.1 鐨勫姛鑳藉樊寮傦細浠呭鍑?drainConnection 杩炵画闈欓粯绐楀彛鏀瑰姩锛坰hared SDK 涓€澶?+ 涓€澶勬柊澧炲洖褰掓祴璇曪級銆?- master 0.5.0 娑夊強鐨?`Taskfile.yml` 涓嶅紩鍏?lts/win7锛圵in7 鐢?npm scripts锛屾棤 Taskfile 渚濊禆锛夛紱`wails.json` / `wails_windows_amd64.syso` / `build/config.yml` / `build/windows/installer/project.nsi` 绛?master 鐗堟湰鍙锋枃浠跺湪 lts/win7 涓婁笉瀛樺湪锛堝凡鐢?`apps/desktop-electron/package.json` 鏇夸唬锛夈€?
### Verification

- `go build ./...`锛圙OWORK=off锛孏o 1.20.14锛夛細passed
- `go vet ./...`锛歱assed
- `go test ./...`锛歱assed锛堝惈 `TestDAQT1603DrainConnectionWaitsForDelayedFrameTail` 鏂扮敤渚嬨€乣TestIsConnResetByPeer` 13 鐢ㄤ緥銆乣csv_recorder_test` / `csv_recorder_rec006_test` 18 鍒楀洖褰掞級
- `npm run typecheck`锛歱assed
- `npm run build`锛歱assed
- `npm run build:backend`锛歱assed
- `npm run dist:win7`锛歱assed锛堜骇鐗?NSIS x64 瀹夎鍖咃級
- 瀹夎鍖?SHA-256 瑙?`releases/0.5.0-win7.1.md`

### Known Issues

- 涓?0.4.0-win7.1 涓€鑷达細Electron 22.3.27 涓嶆敮鎸?`color-mix()` CSS 鍑芥暟锛堝凡鐢?rgba fallback 瑙勯伩锛夛紱360 涓诲姩闃插尽鍙兘閿佸畾 `app.asar`锛堝缓璁坊鍔犱俊浠诲尯鎴栨敼鐢?`--config.directories.output=dist2` 缁曡繃锛夈€?
## [0.4.0-win7.1] - 2026-07-25

### Added

- **UI 鍒锋柊鐜囧姩鎬佺敓鏁?*锛坈herry-pick cc78450锛夛細鏂板 `SetUIRefreshRateHz` 鍚庣鏂规硶 + `/api/device/set-ui-refresh-rate` HTTP 璺敱锛屽墠绔?MainTopBar 鍒囨崲鍒锋柊鐜囨。浣嶆椂鍗虫椂鍚屾鍚庣 `relayStream` 鐨?`uiTicker`锛岀湡姝ｆ敼鍙樻暟鍊煎崱涓庡浘琛ㄧ殑鏇存柊鑺傚銆傚師 0.3.3-win7 鍚庣纭紪鐮?10Hz锛屽墠绔垏鎹?2/5/15/20/30Hz 鏃犳晥鏋溿€?- **IsConnResetByPeer 鍗曞厓娴嬭瘯**锛坈herry-pick acd21c0锛夛細琛ラ綈 `shared/device-sdk/go/protocol/conn_helpers_test.go` 鐨?`TestIsConnResetByPeer` 13 鏉＄敤渚嬶紝瑕嗙洊 io.EOF / connection reset / broken pipe / WSAECONNABORTED 纭瘉鎹笌 i/o timeout 杞敊璇殑杈圭晫銆?
### Changed

- **CSV 琛ㄥご绠€鍖栦负 18 鍒?*锛坈herry-pick 4879ba6锛夛細鍘?20 鍒?`DeviceID,Timestamp,Millisecond,Unit,CH01..CH16` 鏀逛负 18 鍒?`Timestamp,Unit,CH01..CH16`銆?  - 绉婚櫎 `DeviceID` 鍒楋細姣忚澶囩嫭绔嬫枃浠讹紝鏂囦欢鍚嶅凡鍚?deviceSlug锛屽垪鍐呴噸澶嶅啑浣欍€?  - 绉婚櫎 `Millisecond` 鍒楋細鏃堕棿鎴冲洖鍒扮绾?`'YYYY-MM-DD HH:MM:SS'`锛屼笌 0.3.2 鍐崇瓥涓€鑷达紱1000Hz 鍚岀鏍锋湰鍏变韩鍚屼竴鏃堕棿鎴筹紝闈犳枃浠跺悕姣鍚庣紑鍖哄垎鏂囦欢銆?  - 鍚屾鏇存柊 `csv_recorder_test.go` / `csv_recorder_rec006_test.go` 鍒楃储寮曚笌 `docs/test-cases.html` 鐢ㄤ緥鏂囨。銆?- **MonitorView 杩炴帴鎸夐挳绉婚櫎 Loading spinner**锛坈herry-pick bdbbd1a锛夛細浠呬繚鐣?Connecting 鎬?loading锛岀畝鍖栬瑙夊櫔闊炽€?- 鐗堟湰鍙峰悓姝ヨ嚦 `0.4.0-win7.1`锛氫笌 master 0.4.0 涓荤増鏈彿瀵归綈锛屼繚鐣?`-win7.1` 鍚庣紑浠ユ爣璇?Win7 LTS 鍏煎鐗堟湰銆?
### Internal

- HTTP 璺敱琛ㄦ洿鏂帮細`device_handler.go` 澶撮儴娉ㄩ噴鏂板 `POST /api/device/set-ui-refresh-rate`锛宍register.go` 娉ㄥ唽璺敱銆?- `deviceBridge.ts` 鏂板 `setUIRefreshRateHz(hz)` 鍖呰锛岃皟鐢?`POST /api/device/set-ui-refresh-rate`銆?- `App.vue` `onMounted` 鍚姩鍚庡悓姝?localStorage 淇濆瓨鐨勫埛鏂扮巼鍋忓ソ鍒板悗绔紙涓嶉樆濉?onPayload 璁㈤槄锛夈€?- `MainTopBar.vue` `selectRefreshRate` 鍒囨崲妗ｄ綅鏃剁珛鍗冲悓姝ュ悗绔紝澶辫触涓嶉樆濉?UI锛坉isplayStore 宸叉湰鍦版寔涔呭寲锛夈€?
### Verification

- `go build ./...`锛圙OWORK=off锛孏o 1.20.14锛夛細passed
- `go vet ./...`锛歱assed
- `go test ./...`锛歱assed锛堝惈 `TestIsConnResetByPeer` 13 鐢ㄤ緥銆乣csv_recorder_test` / `csv_recorder_rec006_test` 18 鍒楀洖褰掞級
- `npm run typecheck`锛歱assed
- `npm run build`锛歱assed
- `npm run build:backend`锛歱assed
- `npm run dist:win7`锛歱assed锛堜骇鐗?NSIS x64 瀹夎鍖咃級
- 瀹夎鍖?SHA-256 瑙?`releases/0.4.0-win7.1.md`

### Known Issues

- 涓?0.3.3-win7 涓€鑷达細Electron 22.3.27 涓嶆敮鎸?`color-mix()` CSS 鍑芥暟锛堝凡鐢?rgba fallback 瑙勯伩锛夛紱360 涓诲姩闃插尽鍙兘閿佸畾 `app.asar`锛堝缓璁坊鍔犱俊浠诲尯鎴栨敼鐢?`--config.directories.output=dist2` 缁曡繃锛夈€?- `frontend/bindings/` 鐩綍浠嶄繚鐣?master 涓婄殑 Wails v3 .ts binding 鏂囦欢锛屼絾 `frontend/src/` 宸叉棤浠讳綍寮曠敤锛坙ts/win7 鐢?fetch + WebSocket 鏇夸唬锛夈€傝繖浜涙枃浠朵綔涓哄巻鍙查仐鐣欎繚鐣欙紝涓嶅奖鍝嶆瀯寤恒€?
## [0.3.3] - 2026-07-03

### Fixed

- 淇鍋滄閲囬泦鍚庣珛鍗抽厤缃弬鏁版椂鍛戒护鍝嶅簲涔辩爜鎴栧け璐ョ殑闂銆傚仠姝㈤噰闆嗗悗 TCP 缂撳啿鍖烘畫鐣欓噰闆嗘暟鎹抚锛孉pplyDaqT1603Config 鐨?sendCommand 鎶婃畫鐣欏綋浣滃懡浠ゅ搷搴旇鍑恒€傚湪 stopAcquisitionLocked 鍋滄鍛戒护鍚庡鍔?drainConnection 鎺掔┖娈嬬暀鏁版嵁锛涘湪 ApplyDaqT1603Config 璋冪敤 applyHardwareConfig 鍓嶅鍔?drainConnection銆?
### Internal

- 淇鐐逛綅浜?shared/device-sdk/go/daq/hardware/daq_t1603.go锛寃ind-daq 鐨?T1603 璁惧鍚屾牱鍙楃泭銆?
### Verification

- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `$env:GOWORK="off"; go vet ./...`
- `task release`锛堟墜鍔?fallback锛歚go build -tags production` + `makensis`锛宒aq-t1603 鏃?Taskfile锛?
### Known Issues

- 鏆傛棤銆?
## [0.3.2] - 2026-07-03

### Fixed
- 淇 CSV Timestamp 鍒楁椂闂存埑绮惧害闂锛氳法椤圭洰瀵归綈鏃堕棿鎴虫牸寮忓彉鏇达紝缁熶竴鎴柇鍒扮绾э紙`'YYYY-MM-DD HH:MM:SS`锛夛紝閬垮厤灞曠ず閿欒鐨勬椂闂寸粏鍒嗐€傚師鍥犺瑙?daq-p1604 v0.2.2 release note锛圖AQ-P-1604 璁惧纭欢鏃堕棿鎴冲浐浠?bug锛夈€?
### Verification
- `$env:GOWORK="off"; go build ./...`: passed锛坉aq-t1603 宸ヤ綔绌洪棿闅旂锛岃 ADR-006锛?- `$env:GOWORK="off"; go vet ./adapters/recording/...`: passed
- `$env:GOWORK="off"; go test ./adapters/recording/...`: passed

### Known Issues
- 鏆傛棤銆?
## [0.3.1] - 2026-07-02

### Internal
- 淇鏋勫缓閰嶇疆鏂囦欢鐗堟湰婊炲悗锛歜uild/config.yml銆乥uild/info.json銆乸roject.nsi 鍚屾鍒?0.3.1銆?- v0.3.0 鐨?NSIS 瀹夎鍖呮湭姝ｇ‘鐢熸垚锛屾湰娆¤ˉ鍏ㄣ€?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed
- `makensis`: passed
- 鍐掔儫娴嬭瘯: passed锛圙UI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒锛?
### Known Issues
- 鏆傛棤銆?
## [0.3.0] - 2026-07-02

### Added
- 鏂板纭欢閫氫俊鏃ュ織锛氶┍鍔ㄥ眰 hardware-send/hardware-recv 鐨?debug 鏃ュ織鎻愬崌涓?info锛屽墠绔€氫俊鍒嗙粍鍙瀹屾暣鍛戒护浜や簰娴佺▼锛圱CP 杩炴帴/鏂紑銆丂fd/@fe/@f0 绛夊懡浠ょ殑鍙戦€佷笌鍝嶅簲锛夈€傞噰闆嗘湡闂寸殑浜岃繘鍒舵暟鎹抚涓嶆墦鍗帮紝閬垮厤楂橀鍒峰睆銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- 鍐掔儫娴嬭瘯: passed锛圙UI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒锛?
### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-07-01

### Added
- 鏂板褰曞埗鑳屽帇澶勭悊绯荤粺锛欱ackpressureEvent 鍚槦鍒楅暱搴?瀹归噺/绱涓㈠抚鏁般€?- 鏂板 SetBackpressureHandler / SetFatalErrorHandler 鍥炶皟锛屽綍鍒堕槦鍒楅ケ鍜屾垨 I/O 閿欒鏃堕潪闃诲閫氱煡銆?- RecordingSession 鏂板 DroppedCount 瀛楁锛屽墠绔彲鐩戞帶鏁版嵁瀹屾暣鎬с€?- 鏂板 LogFileState 绫诲瀷锛屽墠绔彲鏌ヨ鏃ュ織鏂囦欢鍐欏叆鐘舵€併€?- RecordingService 鏂板鑳屽帇/fatal 浜嬩欢闄愰锛堟瘡绫?1Hz锛夛紝閬垮厤浜嬩欢鍒峰睆銆?
### Changed
- CSV 褰曞埗鍣ㄩ噸鍐欙細鏀寔澶氳澶囩嫭绔嬪啓鍏ャ€侀潪闃诲寮傛闃熷垪銆佹枃浠舵粴鍔ㄣ€?- 鍚庣閲嶆瀯锛氬垹闄?monolithic app.go锛屾媶鍒?relayStream 鍒?DeviceService銆?- RecordingService 娉ㄥ叆鑳屽帇鍜岃嚧鍛介敊璇洖璋冿紝Hub EmitLog 寮傛骞挎挱銆?- 鍓嶇 App.vue/stores 閫傞厤鏂扮殑褰曞埗鐘舵€佸拰鑳屽帇浜嬩欢銆?
### Internal
- freqprobe 璋冭瘯宸ュ叿灏忓箙璋冩暣銆?- AGENTS.md 鍜?README.md 琛ュ厖 Release Commands 娈点€?- go.mod 鏇存柊渚濊禆銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 鍥?go.sum 缂哄け涓嶅彲鐢紝鏀圭敤 go build 鐩村嚭)
- `makensis` 鏋勫缓瀹夎鍖? passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)

### Known Issues
- 鏆傛棤銆?
## [0.1.4] - 2026-06-29

### Added
- 鏂板 Wails 妗岄潰 App backend 瀹炵幇锛氳澶囩鐞嗐€佹棩蹇椼€佸綍鍒舵湇鍔℃暣鍚堝埌缁熶竴妗岄潰搴旂敤銆?
### Internal
- AGENTS.md 鍜?README.md 鍖哄垎寮€鍙戜笌浜や粯鍛戒护锛屾柊澧?Release Commands 娈点€?- AGENTS.md 澧炲姞 ADR-004 绱㈠紩銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed
- `makensis` 鏋勫缓瀹夎鍖? passed

### Known Issues
- 鏆傛棤銆?
## [0.1.3] - 2026-06-23

### Fixed
- 淇閫傞厤鍣ㄦ閿侊細`StopAcquisition`/`Disconnect` 灏嗙‖浠?I/O 绉诲埌閿佸鎵ц锛岄伩鍏嶄笌 `OnReadLoopExit`/`OnConfigSynced` 鍥炶皟鐩镐簰姝婚攣銆?- 淇 readLoop 寮傚父閫€鍑哄悗椹卞姩鏈柇寮€鐨勯棶棰橈細寮傚父閫€鍑烘椂娓呯悊 drivers 琛ㄥ苟璋冪敤 `driver.Disconnect()`锛岄槻姝笅娆?StartAcquisition 鍦ㄥ潖杩炴帴涓婇噸璇曘€?- 淇 Status() 鐘舵€佸崱鍦?Acquiring 鐨勯棶棰橈細绉婚櫎鏈夌己闄风殑 `st.Status != StatusAcquiring` 瀹堝崼锛屾敼涓轰俊浠婚┍鍔ㄥ眰鐘舵€侊紙true source of truth锛夈€?- 淇 CSV 褰曞埗鏁版嵁涓㈠け锛歳elayStream 鏀逛负姣忔潯 snapshot 鍗虫椂鍐欏叆锛堟鍓嶆瘡绉掑彧鍐欎竴娆℃渶鏂板€硷級锛屽苟缁撳悎 `IsActive()` 鏃犻攣鐑矾寰勯伩鍏嶉攣绔炰簤銆?- 淇鍓嶇鎵弿鍒楄〃閲嶅娣诲姞闂锛歋canResultList 鏄剧ず"宸叉坊鍔?鐘舵€佹爣璁帮紝store 灞傚鍔犻噸澶嶆坊鍔犻槻寰°€?- 淇璁剧疆鍛戒护鍚庢鐜囬噰闆嗘暟鎹В鏋愪贡鐮併€?
### Changed
- CSV flush 琛岄槇鍊间粠 100 鈫?2000锛岃 1s 鏃堕棿闂撮殧涓诲 flush 棰戠巼锛屽噺灏戝璁惧楂橀鍦烘櫙涓嬬殑纾佺洏鍚屾娆℃暟銆?- CSV `FormatFloat` 鏇夸唬 `fmt.Sprintf`锛屾秷闄ゆ瘡绉?16000 娆℃牸寮忎覆瑙ｆ瀽寮€閿€銆?- CSV `sync.Mutex` 鏇夸唬 `sync.RWMutex`锛圫tatus 璋冪敤棰戠巼浣庯紝璇诲啓閿侀澶栧紑閿€涓嶅€煎緱锛夈€?- 閲囬泦 channel 缂撳啿鍖轰粠 8192 鈫?65536锛屽湪 1000Hz 涓嬫彁渚涚害 65 绉掔紦鍐诧紝闃叉 CSV flush 闃诲鍙嶅帇鍒扮‖浠?readLoop銆?- 鍓嶇纭欢鏃堕棿鎴虫枃妗堬細"鏄剧ず/闅愯棌" 鈫?"鍚敤/绂佺敤"銆?- E2E 娴嬭瘯 fixture 榛樿鍚敤 `showTimestamp: true`銆?- 鏂囨。鏂囦欢 `MANUAL_TEST.md`銆乣TEST_PLAN.md`銆乣test-cases.html` 绉昏嚦 `docs/` 鐩綍銆?
### Removed
- 绉婚櫎妯℃嫙妯″紡锛氬垹闄?`SimulatedAdapter`銆乣SimulatedScanner` 鍙婂叾娴嬭瘯锛坄simulated_adapter_test.go`銆乣app_test.go`銆乣simulated_flow_test.go`锛夈€?- 绉婚櫎 `DAQ_T1603_MODE` 鐜鍙橀噺鍒嗘敮锛坢ain.go 纭紪鐮佷娇鐢?T1603Adapter锛夈€?- 娓呯悊涓存椂鏂囦欢銆佹祴璇曚骇鐗╁拰搴熷純鐨?threehole 浠ｇ爜銆?
### Internal
- `RecordingPort` 鎺ュ彛鏂板 `IsActive() bool`锛宍CSVRecorder` 鍜?`RecordingUsecase` 鍒嗗埆瀹炵幇鏃犻攣鐑矾寰勩€?- `frameprobe` 璋冭瘯宸ュ叿閲嶅啓锛氭敮鎸佷簩杩涘埗甯цВ鏋愬拰褰掍竴鍖栭厤缃煡璇€?- `deviceStore` 鏂板 `isScanResultAdded` 鏂规硶锛屽熀浜?IP:Port 鍘婚噸銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?
## [0.1.2] - 2026-06-18

### Fixed
- 淇纭欢鍛戒护鍗忚闂锛氱Щ闄?`SendCommand` 绯诲垪鍑芥暟涓拷鍔犵殑 `\n`锛岃澶囩洿鎺ユ帴鏀跺師濮嬪懡浠ゅ瓧绗︿覆銆?- 淇鍓嶇鐘舵€侀棯鐑侊細MonitorView 鍜?DeviceSidebar 琛ュ厖 "Starting" 鐘舵€佺殑澶勭悊锛岃繛鎺?鏂紑鎸夐挳鍦ㄨ繃娓＄姸鎬佹樉绀哄姞杞藉姩鐢汇€?- 淇棰戠巼鏄剧ず閿欒锛歁onitorView 鏀逛负浠?`t1603Config.samplingRate` 璇诲彇閲囨牱棰戠巼锛堣€岄潪椤跺眰 `samplingRate` 瀛楁锛夈€?- 澧炲己甯цВ鏋愬仴澹€э細`parseSpaceSeparatedFrame` 鏀寔 17 token 鍏冩暟鎹牸寮忥紙TIME 鎴?HEAD 鍗曠嫭鍚敤锛夛紝鏂板 `Resync()` 鏂规硶鐢ㄤ簬甯у亸绉诲悗閲嶅悓姝ャ€?
### Changed
- 閲囨牱鐜囪緭鍏ヤ粠涓嬫媺閫夋嫨妗嗘敼涓鸿嚜鐢辨暟瀛楄緭鍏ユ锛?-1000Hz锛夛紝鏀寔浠绘剰鏁存暟鍊笺€?- 鏂板 `metadataMode` 鏀寔锛歚T1603FrameReader` 鍙牴鎹?TIME/HEAD 閰嶇疆鍒囨崲鍥哄畾甯?鍙橀暱甯фā寮忋€?- 娴嬭瘯鐢ㄤ緥鏂囨。鏇存柊锛氱鍙ｄ粠 5000鈫?000锛岀Щ闄ゆā鎷熸ā寮忓紩鐢紝鏀圭敤鍗＄墖寮忓竷灞€銆?
### Internal
- 鏂板 E2E 娴嬭瘯杈呭姪锛歮ock-bridge 鏀寔澶氳澶囧苟鍙戯紝AppPage Page Object Model 鍜岄€夋嫨鍣?data-testid 灞炴€с€?- 娓呯悊 DaqT1603Config.vue 涓湭浣跨敤鐨?`samplingRateOptions` 鍜?`samplingRateSelectOptions`銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?
## [0.1.1] - 2026-06-10

### Fixed
- 绉婚櫎鍓嶇 TC_RANGES 姝讳唬鐮佸拰鏈娇鐢ㄧ殑 updateT1603Config 鍑芥暟銆?- 灏?閲囬泦涓笉涓嬪彂纭欢閰嶇疆"鐨勫垽鏂€昏緫浠庡墠绔Щ鍒板悗绔?usecase锛屽墠绔笉鍐嶅寘鍚‖浠惰涓虹煡璇嗐€?- 淇鍓嶇瑙ｆ瀽璁惧鐘舵€佹椂瀵瑰悗绔暟鍊兼灇涓剧殑渚濊禆锛屾敼涓轰娇鐢ㄥ悗绔洿鎺ヨ繑鍥炵殑 statusText 瀛楃涓层€?- 淇閰嶇疆淇濆瓨鏃剁‖浠跺簲鐢ㄩ敊璇绌?catch 鍚炴病鐨勯棶棰樸€?
### Changed
- DeviceState 鏂板 StatusText 瀛楁鍜?SetStatus() 杈呭姪鏂规硶锛屾墍鏈夐€傞厤鍣ㄩ€氳繃璇ユ柟娉曠粺涓€璁剧疆鐘舵€侊紝閬垮厤 Status/StatusText 涓嶄竴鑷淬€?- 閰嶇疆淇濆瓨鎴愬姛娑堟伅鏍规嵁纭欢涓嬪彂缁撴灉鍔ㄦ€佹樉绀恒€?
### Internal
- 閲嶆瀯妯℃嫙閫傞厤鍣ㄥ拰 T1603 閫傞厤鍣ㄤ腑鍏ㄩ儴鐘舵€佽祴鍊兼搷浣滐紝缁熶竴浣跨敤 SetStatus()銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed (pre-existing freqprobe IPv6 warning unrelated)
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 鏆傛棤銆?
