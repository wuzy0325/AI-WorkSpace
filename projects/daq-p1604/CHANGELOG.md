# Changelog

## [0.6.0] - 2026-07-27

### Added
- 鏂板 DAQ-P-1604 鍏ㄥ帇鍔涢€氶亾鏍￠浂鍔熻兘锛氶《鏍忓彸渚т互鏍￠浂鎸夐挳鏇挎崲閰嶇疆鎸夐挳锛屽綋鍓嶉€変腑璁惧澶勪簬宸茶繛鎺ユ垨閲囬泦涓椂鍧囧彲鍙戦€佽澶囧師鐢?`h` 鏍￠浂鍛戒护銆?- 鏂板鏍￠浂鎿嶄綔閿併€佽繛鎺ョ姸鎬佹鏌ャ€?2 绉掑墠绔秴鏃舵彁绀哄強涓嫳鏂囩晫闈㈡枃妗堬紝闃叉閲嶅瑙﹀彂鍜屾湭杩炴帴璇搷浣溿€?
### Changed
- 椤舵爮绉婚櫎閰嶇疆鍥炬爣锛涜澶囪鎯呭尯鍘熸湁閰嶇疆鎸夐挳缁х画淇濈暀锛岃澶囬厤缃兘鍔涗笉鍙楀奖鍝嶃€?
### Internal
- 鏂板 `DevicePort`銆佽澶囩敤渚嬨€乄ails 鍚庣鍜?TypeScript bridge 鐨勬牎闆惰皟鐢ㄩ摼锛屽苟鍚屾鐢熸垚 Wails TypeScript bindings銆?- 妯℃嫙璁惧閫傞厤鍣ㄦ敮鎸侀噰闆嗕腑鏍￠浂璋冪敤锛涙柊澧炵‖浠堕€傞厤鍣ㄥ洖褰掓祴璇曪紝楠岃瘉閲囬泦涓彂閫?`h` 鍚庨噰闆嗙姸鎬佷繚鎸佷笉鍙樸€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.6.0锛歏ERSION銆乤pps/desktop-wails/wails.json銆乤pps/desktop-wails/frontend/package.json銆乤pps/desktop-wails/frontend/package-lock.json銆乤pps/desktop-wails/build/config.yml銆乤pps/desktop-wails/build/windows/installer/project.nsi銆?
### Verification
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run test`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 鏍￠浂鍛戒护鍦ㄩ噰闆嗘暟鎹祦涓紓姝ヨ繑鍥炴柊鐨勯浂浣嶇郴鏁帮紱褰撳墠鐗堟湰纭鍛戒护宸叉垚鍔熷啓鍏ヨ澶囷紝浣嗙晫闈笉瑙ｆ瀽骞跺睍绀鸿绯绘暟鍝嶅簲銆?- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?
## [0.5.0] - 2026-07-25

### Internal
- 涓?daq-t1603 鐗堟湰鍙峰悓姝ュ彂甯冦€傛湰鐗堟湰 daq-p1604 鏃犵嫭绔嬪姛鑳芥敼鍔細鏈€杩戝叡浜?SDK 鏀瑰姩锛坄shared/device-sdk/go/daq/hardware/daq_t1603.go` 鐨?drainConnection 杩炵画闈欓粯绐楀彛璇箟锛変粎褰卞搷 T1603 璁惧锛孭1604 浣跨敤 w1601 闀垮害鍓嶇紑鍗忚涓?ReadLoop 璺緞涓嶅悓锛屼笉鍙楀奖鍝嶃€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.5.0锛歏ERSION銆乤pps/desktop-wails/wails.json銆乤pps/desktop-wails/frontend/package.json銆乤pps/desktop-wails/frontend/package-lock.json銆乤pps/desktop-wails/build/config.yml銆乤pps/desktop-wails/build/windows/installer/project.nsi銆?
### Verification
- `task release`锛坈lean + 鍓嶇 build + 鐢熶骇 Go 鏋勫缓锛夛細閫氳繃銆傚墠绔?`npm run build`锛坴ue-tsc + vite锛夋垚鍔燂紝`go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"` 浜у嚭 `build/bin/daq-p1604.exe`銆?- `go vet ./...`锛圙OWORK=off锛夛細passed銆?- `go test ./...`锛圙OWORK=off锛夛細passed銆?- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-p1604.exe project.nsi`锛氫骇鍑?`daq-p1604-0.5.0-amd64-installer.exe`锛屽綊妗ｈ嚦 `releases/bin/`銆?- 宸茬煡闄愬埗锛歟xe 鑷韩 Windows 鐗堟湰璧勬簮鍥哄畾涓?`0.0.0.0`锛坵ails v3 alpha `generate syso` 闄愬埗锛屼笌鍘嗗彶 0.3.x/0.4.x 涓€鑷达級锛涘畨瑁呭寘 VIProductVersion 宸叉纭爣娉?0.5.0銆侴UI 鍐掔儫娴嬭瘯寤鸿鍦ㄧ洰鏍囨満鎵嬪姩楠岃瘉銆?
### Known Issues
- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?
## [0.4.0] - 2026-07-24

### Added
- 鏂板绌洪棽杩炴帴 TCP keepalive 澶辨晥妫€娴嬶紙CONN-008锛夛細connected & idle 鐘舵€佷笅 idleReadLoop 鎺㈡祴鍒板绔?keepalive 澶辫触鍗充富鍔ㄥ垽瀹氭柇杩烇紝浣滀负鍛戒护璋冪敤涔嬪鐨勮ˉ鍏呭揩閫熼€氶亾銆?- 鏂板绂佺敤閫氶亾绌?CSV 鍒楄緭鍑猴紙REC-006锛夛細绂佺敤閫氶亾鍦?CSV 涓緭鍑虹┖鍒楋紝淇濇寔鍒楅『搴忎笌琛ㄥご涓€鑷达紝渚夸簬鍚庣画鎸夊垪瑙ｆ瀽銆?- 鏂板鏃ュ織闈㈡澘鎼滅储涓庢棩蹇楁枃浠惰疆杞紙LOG-010/015锛夛細鍓嶇鏃ュ織鍙叧閿瓧鎼滅储锛屽悗绔棩蹇楁寜澶у皬杞浆锛屼究浜庣幇鍦烘帓鏌ャ€?- 鏂板寮傛璁惧鐘舵€佷簨浠舵帹閫侊紙ACQ-010/STB-003锛夛細OnReadLoopExit 鈫?hub 鈫?service 寮傛鎺ㄩ€佺姸鎬侊紝UI 鐘舵€佹洿鏂版洿鍙婃椂锛岄伩鍏嶉樆濉為噰闆嗙儹璺緞銆?
### Changed
- 閰嶇疆鑴忕姸鎬佹牎姝ｏ紙CFG-017锛夛細纭欢閰嶇疆涓?profile 涓嶄竴鑷存椂鑴忔爣璁版洿鍑嗙‘銆?- 鍓嶇 ChannelCard 鏀圭敤 computed 娲剧敓鏁板€笺€侀鑹插缁堣窡闅忛€氶亾鑹诧紝绉婚櫎閲囬泦鏈熸暟鍊煎彉鍖栭棯鐑佸姩鐢伙紙瑙嗚鍣煶锛夛紱MonitorView 鏂囨銆?8 閫氶亾骞惰銆嶁啋銆屽閫氶亾骞惰銆嶃€?
### Fixed
- 淇 ApplyConfig 涓?idleReadLoop 绔炰簤鍚屼竴 conn 璇诲彇瀵艰嚧 v01101 鍝嶅簲琚薄鏌撲负涔辩爜锛氭搷浣?conn 鍓嶆樉寮?stopIdleLoop 骞?join锛涚┖闂茶鍙栧惊鐜湪鏈熬 defer 浠ユ柊 stop 閫氶亾閲嶅惎锛屽洓閲嶅畧鍗槻姝㈠弻鍚垨澶辫仈鍚庨噸鍚紱StopAcquisition 涓嶅啀娲剧敓閲嶅 idle 寰幆锛坓uard on driver.idleStopCh == nil锛夈€?- 淇鎷旂綉绾块噸杩炴椂 u01101/v01101 闃舵瀵圭 FIN 琚綋浣滆蒋閿欒鍚炴帀銆佸鑷村崐姝昏繛鎺ュ悗缁?StartAcquisition 瑙﹀彂 Windows WSAECONNABORTED锛氭柊澧?sharedproto.IsConnResetByPeer 鍒ゅ畾瀵圭 FIN/RST 纭瘉鎹紙io.EOF / connection reset / broken pipe / WSAECONNABORTED锛夛紝涓?IsConnectionFault锛堟棩蹇楅檷鍣敤锛夎涔夊垎绂伙紱Connect 妫€娴嬪埌 unitErr 鏃跺叧闂?conn 骞惰繑鍥?error 寮哄埗閲嶈繛锛汚pplyConfig v01101 澶辫触涓斿懡涓椂澶嶇敤 handleConnectionLost 娓呯悊 driver + conn + status銆?- 淇杩炴帴璁惧鏃剁‖浠跺崟浣嶄笌 profile 涓嶄竴鑷村鑷村墠绔€氶亾鍗＄墖鏄剧ず闄堟棫鍗曚綅锛坧si锛夎€岄《閮?閰嶇疆鏄剧ず鏂板崟浣嶏紙Pa锛夛細鏂板 syncChannelsUnit helper锛岃鍒欎笌鍓嶇 getChannelUnit 涓€鑷达紙CH1-CH16 璺熼殢鍏ㄥ眬鍘嬪姏鍗曚綅锛孋H17 澶ф皵鍘嬪姏閿佸畾 Pa锛孋H18 澶ф皵娓╁害閿佸畾 鈩冿級銆?
### Internal
- 鏂板 e2e 娴嬭瘯濂椾欢锛坋2e_*.py / cases.json / gen_e2e_report.py锛変笌鏂囨。 e2e-testing-guide.md銆?- 鏂板 idle-stop 鍥炲綊娴嬭瘯锛圓pplyConfig & StopAcquisition锛夈€両sConnResetByPeer 鍗曞厓娴嬭瘯锛?3 鏉★級銆乻yncUnitFromHardware EOF/瓒呮椂娴嬭瘯銆丄pplyConfig v01101 EOF/杞敊璇祴璇曘€乻yncChannelsUnit 4 鏉″崟鍏冩祴璇曘€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.4.0锛歏ERSION銆乤pps/desktop-wails/wails.json銆乤pps/desktop-wails/frontend/package.json銆乤pps/desktop-wails/frontend/package-lock.json銆乤pps/desktop-wails/build/config.yml銆乤pps/desktop-wails/build/windows/installer/project.nsi銆?
### Verification
- `task release`锛坈lean + 鍓嶇 build + 鐢熶骇 Go 鏋勫缓锛夛細閫氳繃銆傚墠绔?`npm run build`锛坴ue-tsc + vite锛夋垚鍔燂紝`go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"` 浜у嚭 `build/bin/daq-p1604.exe`銆?- `go vet ./...`锛圙OWORK=off锛夛細passed銆?- `go test ./...`锛圙OWORK=off锛夛細passed锛坅dapters/hardware銆乤dapters/recording銆乥ackend 鍧?ok锛夈€?- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-p1604.exe project.nsi`锛氫骇鍑?`daq-p1604-0.4.0-amd64-installer.exe`锛屽綊妗ｈ嚦 `releases/bin/`銆?- SHA-256锛歚24669f125a2e28dd4c55e7e74639c5f3d0f331958dc9a02cb06bcd476d169bda`銆?- 宸茬煡闄愬埗锛歟xe 鑷韩 Windows 鐗堟湰璧勬簮鍥哄畾涓?`0.0.0.0`锛坵ails v3 alpha `generate syso` 闄愬埗锛屼笌鍘嗗彶 0.3.x 涓€鑷达級锛涘畨瑁呭寘 VIProductVersion 宸叉纭爣娉?0.4.0銆侴UI 鍐掔儫娴嬭瘯寤鸿鍦ㄧ洰鏍囨満鎵嬪姩楠岃瘉銆?
### Known Issues
- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?
## [0.3.0] - 2026-07-03

### Changed

- **CSV 褰曞埗鏀逛负姣忚澶囦竴涓枃浠?*锛氬師鍗曟枃浠惰璁″湪澶氳澶囧悓鏃跺綍鍒舵椂锛屼袱鍙拌澶囩殑纭欢鏃堕棿鎴充氦鏇胯烦璺冨啓鍏ュ悓涓€ CSV 鏂囦欢锛屾椂闂存埑鍒楀湪涓や釜鍊间箣闂存潵鍥炶烦鍙橈紝鏁版嵁鍒椾篃娣锋潅銆傜幇鎸?deviceId 璺敱鍒扮嫭绔嬫枃浠讹紝涓?wind-daq 璁捐瀵归綈銆?- 鏂囦欢鍚嶆牸寮忓彉鏇达紙涓嶅吋瀹规棫鐗堟湰锛夛細
  - 鏃э細`<prefix>_YYYYMMDD-HHMMSS.csv`
  - 鏂帮細`<prefix>-<deviceSlug>-YYYYMMDD-HHMMSS-NNN.csv`
  - `deviceSlug` 浼樺厛鐢ㄨ澶囧悕锛坰anitize 鍚庯級锛屽悓鍚嶅啿绐佹椂杩藉姞 deviceId 鍓?6 浣?- 鏂囦欢婊氬姩鏉′欢锛圡axSize/MaxRecordCount/MaxDuration锛夋敼涓?*鎸夎澶囩嫭绔嬭瘎浼?*锛屽仠姝㈡潯浠讹紙璺ㄨ澶囨眹鎬伙級淇濇寔涓嶅彉銆?- 褰曞埗鍚姩鏃朵笉鍐嶉鍒涘缓绌烘枃浠讹紝鏀逛负绗竴涓?payload 鍒拌揪鏃舵寜 deviceId 鎳掑垱寤猴紝閬垮厤澶氳澶囧満鏅笅鏈姇閫掓暟鎹殑璁惧浜х敓绌?CSV銆?
### Internal

- `CSVRecorder` 閲嶆瀯涓哄 writer 鏋舵瀯锛歚map[deviceId]*perDeviceWriter`锛屾瘡璁惧鐙珛鎸佹湁鏂囦欢/缂撳啿/缁熻锛屽崟 writer goroutine 涓茶娑堣垂 channel 娑堥櫎澶氳澶囬攣浜夌敤銆?- `core.RecordingConfig` 鏂板 `DeviceNames map[string]string` 瀛楁锛岀敱 backend 鍦?StartRecording 鏃朵粠 profiles 涓€娆℃€у～鍏?deviceId鈫抧ame 鏄犲皠锛宺ecorder 鐢ㄤ簬鐢熸垚浜虹被鍙鐨勬枃浠跺悕 slug銆?- `backend/app.go` 鐨?`StartRecordingWithConfig` 璋冩暣涓猴細鍏堝彇 profiles 鈫?鑱氬悎閫氶亾绮惧害 鈫?鏋勫缓 deviceNames map 鈫?娉ㄥ叆 RecordingConfig銆?- 娓呯悊 dead code锛氱Щ闄ゆ湭娑堣垂鐨?`autoDone`/`autoDoneOnce`/`signalAutoDone` 淇″彿鏈哄埗锛堥潬 `started.CompareAndSwap` + writerLoop 涓茶 I/O 宸蹭繚璇佸苟鍙戝畨鍏級锛屼互鍙?`perDeviceWriter` 鐨?`deviceID`/`headerWritten`/`totalRecords` 涓変釜鏈瀛楁銆?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.3.0锛歚VERSION`銆乣apps/desktop-wails/wails.json`銆乣apps/desktop-wails/frontend/package.json`銆乣apps/desktop-wails/frontend/package-lock.json`銆乣apps/desktop-wails/build/config.yml`銆乣apps/desktop-wails/build/windows/installer/project.nsi`銆?
### Verification

- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`

### Known Issues

- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?- 澶?writer 鏍稿績璺敱閫昏緫锛坄getOrCreateWriter` / `sanitizeFileSegment` / `uniqueFileSlugLocked` / `shouldRotate(deviceID)`锛夋殏鏃犲崟鍏冩祴璇曡鐩栵紝渚濊禆瀹炴満澶氳澶囬獙璇併€?
## [0.2.4] - 2026-07-03

### Added

- 鏂板搴旂敤灞傝繛缁秴鏃舵柇杩炴娴嬶細`readLoop` 缁存姢 `consecutiveTimeouts` 璁℃暟鍣紝杩炵画 25 娆★紙5s锛塕eadFrame 瓒呮椂鍗宠皟鐢?`handleConnectionLost` 涓诲姩鍒ゅ畾鏂繛锛屼綔涓?TCP keepalive 涔嬪鐨勫揩閫熼€氶亾銆?
### Changed

- `p1604KeepAlivePeriod` 浠?`10 * time.Second` 璋冩暣涓?`3 * time.Second`锛學indows ~33s / Linux ~12s 鍏滃簳锛屾瘮鍘?~110s 蹇?3 鍊嶄互涓娿€?- 褰㈡垚鍙屼繚闄╂娴嬫灦鏋勶細閲囬泦鏈?readLoop 娲昏穬鏃剁敱杩炵画瓒呮椂璁℃暟鍣ㄤ富妫€娴嬶紙5s锛夛紝闈為噰闆嗘湡 readLoop 绌洪棽鏃剁敱 keepalive 鍏滃簳锛垀33s/12s锛夈€?
### Fixed

- 淇閫氶亾閫夋嫨鍣ㄧ粍浠舵枃鏈孩鍑烘埅鏂殑闂锛坴0.2.3 閬楁紡鏈彂甯冿級銆?- 淇 CH17/CH18 澶ф皵閫氶亾榛樿琚嬀閫夎繘瀹炴椂鍥捐〃鐨勯棶棰樸€?
### Internal

- 鍚屾鏇存柊 `enableTCPKeepalive` 璁捐娉ㄩ噴銆乣Connect` keepalive 鍚敤鍧楁敞閲娿€乣p1604ConsecutiveTimeoutThreshold` 鍙屼繚闄╄鏄庯紝淇涓庢柊鐗?keepalive 鏁板€硷紙3s/33s锛夌煕鐩剧殑杩囨湡鎻忚堪锛堝師 10s/100s/110s锛夈€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.2.4锛歚VERSION`銆乣apps/desktop-wails/wails.json`銆乣apps/desktop-wails/frontend/package.json`銆乣apps/desktop-wails/frontend/package-lock.json`銆乣apps/desktop-wails/build/config.yml`銆乣apps/desktop-wails/build/windows/installer/project.nsi`銆?- 閫氳繃 `npm install --package-lock-only` 鍚屾 package-lock.json 涓?package.json 鐗堟湰鍙枫€?
### Verification

- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`

### Known Issues

- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?- 闈為噰闆嗘湡闂存棤鍛ㄦ湡鎬?I/O 瑙﹀彂 keepalive 澶辫触涓婃姤锛屾柇杩炴娴嬩粛渚濊禆涓嬩竴娆″懡浠よ皟鐢ㄣ€?
## [0.2.3] - 2026-07-03

### Fixed

- 淇鍋滄閲囬泦鍚庣珛鍗抽厤缃崟浣嶆椂杩斿洖 "unexpected v01101 response" 閿欒鐨勯棶棰樸€傚仠姝㈤噰闆嗗悗 TCP 缂撳啿鍖烘畫鐣欓噰闆嗘暟鎹抚锛寁01101 鍛戒护鐨?ReadFrame 鎶婃畫鐣欏綋浣滃搷搴旇鍑恒€傚湪 ApplyConfig 鍙戦€?v01101 鍓嶅鍔?frameReader.Reset + DrainConnection 鎺掔┖娈嬬暀鏁版嵁銆?
### Internal

- 瀵归綈 wind-daq 宸叉湁鐨?SetUnit 缂撳啿鍖烘帓绌轰慨澶嶆柟妗堛€?
### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go vet ./...`
- `task release`

### Known Issues

- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?
## [0.2.2] - 2026-07-03

### Fixed
- 淇 CSV Timestamp 鍒楁椂闂存埑閿欒锛欴AQ-P-1604 璁惧纭欢鏃堕棿鎴冲瓨鍦ㄥ浐浠?bug锛坒ractional 瀛楁浠?~4348Hz 閫熺巼閫掑锛屾瘡绱Н绾?232ms 璺宠穬鏍℃锛夛紝瀵艰嚧 1000Hz 閲囬泦涓?1 姣鍐呭嚭鐜板甯ф椂闂存埑锛涚郴缁熸绉掓椂闂存埑鍦?1000Hz 涓嬬簿搴︿篃涓嶈冻銆傜粺涓€鎴柇鍒扮绾э紝閬垮厤灞曠ず閿欒鐨勬椂闂寸粏鍒嗐€?- 淇 `CSVRecorder.Stop()` 缂哄皯 `return nil` 瀵艰嚧鐨勭紪璇戦敊璇紙棰勫瓨闂锛岄樆濉為獙璇侊級銆?- 淇 `csv_recorder.go` 琛ㄥご娉ㄩ噴閿欒锛氫粠銆屽井绉掔簿搴︺€嶆洿姝ｄ负銆岀绾х簿搴︺€嶏紝涓庡疄闄呮牸寮忎覆涓€鑷淬€?
### Internal
- Taskfile `generate-icon` 浠诲姟鏀圭敤 `build/windows/info.json` 鍜?`build/windows/wails.exe.manifest` 妯℃澘鏂囦欢锛宍wails3 generate syso` 鍐呴儴鐢?`wails.json` 鐨?info 瀛楁娓叉煋妯℃澘銆傚垹闄ゅ啑浣欑殑鍏蜂綋鍊肩増鏈?`build/info.json` 鍜?`build/windows.manifest`锛岀増鏈彿婧愪粠 7 涓敹鏁涘埌 6 涓€?- 閲嶆柊鐢熸垚 `wails_windows_amd64.syso` 璧勬簮娈点€?- 鏂板椤圭洰 `README.md` 鍜?`CLAUDE.md` 鏂囨。锛屽榻?daq-t1603 / wind-daq 椤圭洰銆?
### Verification
- `go build ./...`: passed
- `go vet ./adapters/recording/...`: passed
- `go test ./adapters/recording/...`: passed (no test files)

### Known Issues
- 璁惧纭欢鏃堕棿鎴冲浐浠?bug 鏈慨澶嶏紝闇€鑱旂郴纭欢宸ョ▼甯堜慨澶嶅浐浠跺悗鎵嶈兘鎭㈠姣绮惧害鏃堕棿鎴炽€?
## [0.2.1] - 2026-07-02

### Added
- 鏂板鎵弿寮圭獥澶氶€?+ 鍐呰仈鏀瑰悕 + 鎵归噺娣诲姞璁惧鍔熻兘锛屾敮鎸侀娆¤鏈哄満鏅竴娆″嬀閫夊鍙拌澶囦竴閿惤搴撱€?- 鏂板纭欢閫氫俊 hardware-send/hardware-recv 鍒嗙被鏃ュ織锛屽墠绔€氫俊鍒嗙粍鍙瀹屾暣鍛戒护浜や簰娴佺▼銆?
### Changed
- 鎵弿寮圭獥鏀惧ぇ鑷?44rem x 80vh锛屽凡娣诲姞璁惧缃伆涓嶅彲閲嶅姞锛屾湭娣诲姞璁惧榛樿棰勫嬀閫夈€?- 娣诲姞鍚庣珛鍗宠Е鍙戞柊璁惧骞跺彂杩炴帴锛屼笉鍐嶉渶瑕侀噸鍚簲鐢ㄣ€?
### Internal
- deviceStoreHelpers 鎶藉嚭 6 涓函 TS 宸ュ叿鍑芥暟锛屾柊澧?18 鏉?vitest 鍗曞厓娴嬭瘯銆?- 琛ラ綈 build/config.yml 鍜?build/info.json 鐗堟湰鍙峰埌涓庨」鐩竴鑷淬€?
### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `npm test`: 18/18 passed
- `go build -tags production`: passed
- `makensis`: passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)

### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-07-01

### Added
- 鏂板 FileRotation 鏂囦欢婊氬姩閰嶇疆锛堟寜澶у皬/鏃堕暱/璁板綍鏁拌嚜鍔ㄥ垏鏂囦欢锛夈€?- 鏂板 StopConditions 褰曞埗鑷姩鍋滄鏉′欢銆?- 鏂板 RecordingStopping 鐘舵€佸拰 DroppedCount 涓㈠抚璁℃暟锛屽墠绔彲鏄剧ず鏁版嵁瀹屾暣鎬ф寚鏍囥€?- 鏂板 Taskfile.yml 鏋勫缓浠诲姟瀹氫箟銆?
### Changed
- RecordingPort.Start 鎺ユ敹 RecordingConfig 缁撴瀯浣擄紝鏇夸唬绂绘暎鍙傛暟銆?- RecordingSession 鏂板 Format/DroppedCount/FileCount/CurrentFile/LastError 瀛楁銆?- CSV 褰曞埗鍣ㄩ噸鏋勪负寮傛 writer 鏋舵瀯锛屾敮鎸佸璁惧骞跺彂鍐欏叆鍜屾枃浠舵粴鍔ㄣ€?- CSV Timestamp 鍒楁敼涓哄甫姣鐨勫崟鍒楁牸寮忥紝鍓嶇紑鍗曞紩鍙峰己鍒?Excel 鏂囨湰妯″紡銆?- 纭欢閫傞厤鍣?p1604_adapter 閲嶆瀯锛屾彁鍗囪繛鎺ョǔ瀹氭€с€?- 鍓嶇缁戝畾鍜?stores 灞傞€傞厤鏂扮殑褰曞埗閰嶇疆鍜岀姸鎬佹ā鍨嬨€?
### Removed
- 绉婚櫎 v0.1.x 瀹為獙鎬?Binary 褰曞埗鏍煎紡锛氭棤璇荤銆佸鍎挎牸寮忥紝缁存姢鎴愭湰楂樸€?  CSV 宸茶兘婊¤冻 1000Hz 閲囬泦闇€姹傘€傚師 v0.1.x 褰曞埗鐨?Binary 鏂囦欢鏃犳硶鍦ㄦ湰鐗堟湰璇诲彇銆?
### Internal
- AGENTS.md 澧炲姞 ADR-004 绱㈠紩銆?- 璋冩暣 appicon 鍥炬爣銆?
### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 鍥?go.sum 缂哄け涓嶅彲鐢紝鏀圭敤 go build 鐩村嚭)
- `makensis` 鏋勫缓瀹夎鍖? passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)

### Known Issues
- 鏆傛棤銆?
## [0.1.1] - 2026-06-29

### Internal
- AGENTS.md 鏂板銆屽澶栦氦浠樻墦鍖呫€嶈妭锛屼娇鐢?wails3 build锛堝唴閮ㄨ嚜鍔ㄥ惎鐢?-tags production锛夈€?- 鍒涘缓 CHANGELOG.md 鍜屽彂甯冨熀纭€璁炬柦銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed

### Known Issues
- 鏆傛棤銆?

## [0.5.0-win7.1] - 2026-07-25
### Changed
- 鐗堟湰鍙峰悓姝ヨ嚦 `0.5.0-win7.1`锛氫笌 master 0.5.0 涓荤増鏈彿瀵归綈锛屼繚鐣?`-win7.1` 鍚庣紑浠ユ爣璇?Win7 LTS 鍏煎鐗堟湰銆俙apps/desktop-electron/package.json` 涓?`frontend/package.json` 鍚屾鏇存柊锛汵SIS 瀹夎鍖呮枃浠跺悕鏍煎紡 `DAQ-P-1604-Win7-Setup-0.5.0-win7.1-x64.exe`銆?
### Internal
- 鏈鏈紩鍏ユ柊鐨勪笟鍔″姛鑳芥敼鍔紝浠呬负鐗堟湰鍙峰悓姝ヤ笌 master 0.5.0 瀵归綈銆俶aster 0.5.0 涓?daq-p1604 涔熸棤鐙珛鍔熻兘鏀瑰姩锛堜粎鍚屾鍙戝竷锛夛紝鍏变韩 SDK 鐨?drainConnection 鏀瑰姩浠呭奖鍝?T1603 璁惧锛孭1604 浣跨敤 w1601 闀垮害鍓嶇紑鍗忚涓?ReadLoop 璺緞涓嶅悓锛屼笉鍙楀奖鍝嶃€?- master 0.5.0 娑夊強鐨?`wails.json` / `wails_windows_amd64.syso` / `frontend/package-lock.json` / `build/config.yml` / `build/windows/installer/project.nsi` 绛?master 鐗堟湰鍙锋枃浠跺湪 lts/win7 涓婁笉瀛樺湪锛堝凡鐢?`apps/desktop-electron/package.json` 鏇夸唬锛夛紝鏃犻渶鍚屾銆?
### Verification
- `go build ./...`锛圙OWORK=off锛孏o 1.20.14锛夛細passed
- `go vet ./...`锛歱assed
- `go test ./...`锛歱assed
- `npm run typecheck`锛歱assed
- `npm run build`锛歱assed
- `npm run build:backend`锛歱assed
- `npm run dist:win7`锛歱assed锛堜骇鐗?NSIS x64 瀹夎鍖咃級
- 瀹夎鍖?SHA-256 瑙?`releases/0.5.0-win7.1.md`
### Known Issues
- 涓?0.4.0-win7.1 涓€鑷达細Electron 22.3.27 涓嶆敮鎸?`color-mix()` CSS 鍑芥暟锛堝凡鐢?rgba fallback 瑙勯伩锛夛紱360 涓诲姩闃插尽鍙兘閿佸畾 `app.asar`锛堝缓璁坊鍔犱俊浠诲尯鎴栨敼鐢?`--config.directories.output=dist2` 缁曡繃锛夈€?
## [0.4.0-win7.1] - 2026-07-25
### Added
- **IsConnResetByPeer 鍗曞厓娴嬭瘯**锛坈herry-pick acd21c0锛夛細琛ラ綈 `shared/device-sdk/go/protocol/conn_helpers_test.go` 鐨?`TestIsConnResetByPeer` 13 鏉＄敤渚嬨€傚嚱鏁版湰浣撲笌 `p1604_adapter.go` 鐨勭‖/杞敊璇鐞嗗凡鍦?0.3.0-win7.1锛坉586dec锛夊紩鍏ワ紝鏈浠呭悓姝?master 涓婄殑娴嬭瘯瑕嗙洊銆?  - 瑕嗙洊纭瘉鎹細io.EOF / 鍖呰 EOF / connection reset by peer / broken pipe / wsasend aborted / wsarecv aborted / connection abort銆?  - 杞敊璇笉鍖归厤锛歩/o timeout / timeout net error / device error N05 / parse error銆?
### Changed
- 鐗堟湰鍙峰悓姝ヨ嚦 `0.4.0-win7.1`锛氫笌 master 0.4.0 涓荤増鏈彿瀵归綈锛屼繚鐣?`-win7.1` 鍚庣紑浠ユ爣璇?Win7 LTS 鍏煎鐗堟湰銆?- `apps/desktop-electron/package.json` 鐗堟湰鍙峰瓧娈靛悓姝ユ洿鏂帮紱NSIS 瀹夎鍖呮枃浠跺悕鏍煎紡 `DAQ-P-1604-Win7-Setup-0.4.0-win7.1-x64.exe`銆?
### Internal
- 鏈鏈紩鍏ユ柊鐨勪笟鍔″姛鑳芥敼鍔紝浠呬负鐗堟湰鍙峰悓姝ヤ笌 master 0.4.0 瀵归綈 + 娴嬭瘯鐢ㄤ緥琛ュ厖銆?- master 0.4.0 娑夊強鐨?wails.json / wails_windows_amd64.syso / frontend/package.json 绛夌増鏈彿鏂囦欢鍦?lts/win7 涓婁笉瀛樺湪锛堝凡鐢?`apps/desktop-electron/package.json` 鏇夸唬锛夛紝鏃犻渶鍚屾銆?
### Verification
- `go build ./...`锛圙OWORK=off锛孏o 1.20.14锛夛細passed
- `go vet ./...`锛歱assed
- `go test ./...`锛歱assed锛堝惈 `TestIsConnResetByPeer` 13 鐢ㄤ緥锛?- `npm run typecheck`锛歱assed
- `npm run build`锛歱assed
- `npm run build:backend`锛歱assed
- `npm run dist:win7`锛歱assed锛堜骇鐗?NSIS x64 瀹夎鍖咃級
- 瀹夎鍖?SHA-256 瑙?`releases/0.4.0-win7.1.md`
### Known Issues
- 涓?0.3.0-win7.1 涓€鑷达細Electron 22.3.27 涓嶆敮鎸?`color-mix()` CSS 鍑芥暟锛堝凡鐢?rgba fallback 瑙勯伩锛夛紱360 涓诲姩闃插尽鍙兘閿佸畾 `app.asar`锛堝缓璁坊鍔犱俊浠诲尯鎴栨敼鐢?`--config.directories.output=dist2` 缁曡繃锛夈€?
## [0.3.0-win7.1] - 2026-07-23
### Changed
- **Win7 LTS 鏀归€?*锛氬皢妗岄潰澹充粠 Wails v3 + WebView2 鏇挎崲涓?**Go 1.20.14 + Electron 22.3.27 + net/http**锛屼笟鍔″眰锛坈ore/ports/usecase/adapters锛夐浂鏀瑰姩銆傝瑙?`docs/runbooks/win7-migration-guide.md`銆?- 鐩戝惉绔彛鏀逛负 `127.0.0.1:18182`锛堜笌 daq-t1603 鐨?18181 鍖哄垎锛岄伩鍏嶅悓鏈哄弻寮€鍐茬獊锛夈€?- `apps/desktop-wails/main.go` 鏀逛负 net/http server + `//go:embed all:frontend/dist` + 浼橀泤鍏抽棴锛岀Щ闄?Wails 渚濊禆銆?- `apps/desktop-wails/backend/app.go` 鏀逛负 hub 妯″紡锛岀Щ闄?`application.App` 渚濊禆锛屼緷璧?`core.EventBus` 鎺ュ彛鑰岄潪鍏蜂綋浼犺緭銆?- Go 1.20 鏃?`log/slog` 鍖咃紝缁熶竴鏇挎崲涓?`shared.local/device-sdk/go/pkg/slog` polyfill锛? 涓枃浠讹級銆?- 鍓嶇 `bridge/` 鏀逛负 fetch + WebSocket锛堢Щ闄?`@wailsio/runtime` 渚濊禆锛夛細`httpClient.ts`锛堢粺涓€鍝嶅簲淇″皝瑙ｅ寘锛夈€乣wsClient.ts`锛圵ebSocket 鍗曚緥 + 鎸囨暟閫€閬块噸杩烇級銆乣deviceBridge.ts` / `logBridge.ts` / `recordingBridge.ts`锛圚TTP RPC + WebSocket 浜嬩欢璁㈤槄锛夈€?- 鍓嶇 `package.json` 绉婚櫎 `@wailsio/runtime` 渚濊禆锛沗tsconfig.json` 绉婚櫎 `bindings/**` 閬垮厤寮曠敤 Wails 缁戝畾瀵艰嚧 vue-tsc 鎶ラ敊銆?- `go.mod` 鏀逛负 `go 1.20` + `nhooyr.io/websocket v1.8.17` + `shared.local/device-sdk/go`锛岀Щ闄?Wails v3 渚濊禆銆?- 宸ヤ綔绌洪棿 `go.work` 娉ㄩ噴鎺?`projects/daq-p1604/apps/desktop-wails` 琛岋紙Go 1.20 + Wails v3 alpha 涓庡伐浣滅┖闂?Go 1.26 涓嶅吋瀹癸級銆?
### Added
- **鏂板 `apps/desktop-wails/httpserver/` 鍖?*锛欻TTP handler + WebSocket hub锛坄register.go` / `device_handler.go` / `recording_handler.go` / `log_handler.go` / `ws_hub.go` / `helpers.go`锛夛紝瀹炵幇 `core.EventBus` 鎺ュ彛锛屽崟 goroutine 涓茶澶勭悊 register/unregister/broadcast锛屾瘡瀹㈡埛绔嫭绔?send channel锛坆uffered 32锛? writePump goroutine銆?- **鏂板 `apps/desktop-wails/core/eventbus.go`**锛歚EventBus` 鎺ュ彛 + 4 涓簨浠跺父閲忥紙`daq:log` / `daq:recording-status` / `daq:recording-warning` / `daq:device-state`锛夛紝瑙ｈ€︿簨浠舵帹閫佷笌浼犺緭灞傘€?- **鏂板 `apps/desktop-wails/core/hub.go`**锛欻ub 鐘舵€佸鍣紝闆嗕腑绠＄悊 ctx銆乺elay 鍗忕▼鏄犲皠銆丩ogEmitter銆丒ventBus锛岄伩鍏?Service 闂村惊鐜緷璧栥€?- **鏂板 `apps/desktop-electron/` 鐩綍**锛欵lectron 22.3.27 妗岄潰澹筹紝鍖呭惈 `main.cjs`锛堜富杩涚▼锛歴pawn Go 鍚庣 + 鍒涘缓 BrowserWindow + IPC 妗ワ級銆乣preload.cjs`锛坈ontextBridge 鏆撮湶 `showOpenDialog`锛夈€乣package.json`锛坋lectron-builder NSIS 鎵撳寘閰嶇疆锛夈€乣scripts/build-backend.ps1`锛圙o 1.20.14 璺緞纭紪鐮?+ GOWORK=off + CGO_ENABLED=0锛夈€乣scripts/generate-ico.ps1`锛堜粠 appicon.png 鐢熸垚澶氬昂瀵?ICO锛夈€乣.gitignore`銆?- **鏂板 `frontend/src/bridge/httpClient.ts`**锛氱粺涓€鍝嶅簲淇″皝瑙ｅ寘锛坄{ok:true, data}` / `{ok:false, error}`锛夛紝瀵煎嚭 `post<T>` / `get<T>` / `del<T>` 渚挎嵎灏佽銆?- **鏂板 `frontend/src/bridge/wsClient.ts`**锛歐ebSocket 鍗曚緥 + 鎸囨暟閫€閬块噸杩烇紙1s 鈫?2s 鈫?4s 鈫?... 涓婇檺 10s锛夛紝鑷姩閲嶈繛鍚庨噸鏂拌闃呬簨浠躲€?- **閲嶆柊鐢熸垚 `appicon.ico`**锛氶噰鐢?`tools/ico/wave_green_512.png`锛?12x512 32bpp ARGB锛屾尝娴豢涓婚锛変綔涓哄浘鏍囨簮锛岄€氳繃 `scripts/generate-ico.ps1` 鐢熸垚 6 灏哄锛?56/128/64/48/32/16锛夊鍒嗚鲸鐜?ICO锛?3074 bytes锛夛紝婊¤冻 electron-builder 鑷冲皯 256x256 瑕佹眰銆傚師 ico 浠?192 bytes 鍗曞昂瀵镐笉杈炬爣銆俙appicon.png` 鍚屾鏇挎崲涓?wave_green_512.png銆?
### Internal
- **澶氬弬鏁颁簨浠?wire 鏍煎紡**锛歚daq:device-state` 鏄弻鍙傛暟浜嬩欢 `[id, state]`锛學SHub.Emit 褰?data 闀垮害 > 1 鏃舵墦鍖呬负鏁扮粍鎺ㄩ€侊紝鍓嶇 onmessage 瑙ｆ瀯鏁扮粍銆?- **缁熶竴鍝嶅簲淇″皝**锛歚{ok:true, data}` / `{ok:false, error}` 渚夸簬鍓嶇 fetch 缁熶竴澶勭悊锛宍apiOK` / `apiErr` 杈呭姪鍑芥暟闆嗕腑鍦?`httpserver/helpers.go`銆?- 涓存椂澶嶇敤 daq-t1603 鐨勫灏哄 `appicon.ico` 瑕嗙洊 daq-p1604 鍘?ico锛堝師 ico 浠?192 bytes 鍗曞昂瀵革紝涓嶆弧瓒?electron-builder 鑷冲皯 256x256 瑕佹眰锛夆€斺€?*宸插湪鏈閲嶆柊鐢熸垚涓撳睘 ICO 鍚庣Щ闄よ涓存椂鎺柦**銆?- `frontend/dist/` 鐩綍鐢?`.gitignore` 绗?5 琛?`dist/` 瑙勫垯蹇界暐锛宑lone 鍚庝笉瀛樺湪锛沗build-backend.ps1` 閫氳繃"鍏?`npm run build` 鍐?`go build`"淇濊瘉 `//go:embed all:frontend/dist` 鍦ㄧ紪璇戞椂鐩綍蹇呯劧瀛樺湪锛堜笌 daq-t1603 涓€鑷达紝涓嶄繚鐣?`.gitkeep` 鍗犱綅鏂囦欢锛夈€?- 鍚屾 3 涓増鏈彿鏂囦欢鍒?`0.3.0-win7.1`锛歚VERSION`銆乣apps/desktop-wails/frontend/package.json`銆乣apps/desktop-electron/package.json`銆?- 鏇存柊 `AGENTS.md`锛堟柊澧?Win7 LTS 鍒嗘敮鏋勫缓"鑺傦級銆乣CLAUDE.md`锛堟柊澧?Win7 LTS Branch"鑺傦紝鍚灦鏋勫彉鍖栧浘銆佸叧閿璁°€佺鍙ｄ笌浜嬩欢娓呭崟銆佷笌 daq-t1603 Win7 鐗堝樊寮傦級銆?
### Verification
- `$env:GOWORK="off"; go build -buildvcs=false ./...`锛圙o 1.20.14锛?- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`锛坅dapters/hardware銆乥ackend銆乭ttpserver 涓夊寘娴嬭瘯鍏ㄧ豢锛?- `npm run typecheck`锛坴ue-tsc锛?- `npm run build`锛圴ite锛?153 modules锛?- `npm run build:backend`锛堢敓鎴?`backend/daq-p1604-backend.exe` 6.6MB锛?- `npm run dist:win7`锛堢敓鎴?`dist/DAQ-P-1604-Win7-Setup-0.3.0-win7.1-x64.exe` 67.6MB锛?
### Known Issues
- 鏃犻噸澶у凡鐭ラ棶棰樸€?- `frontend/dist/` 琚?`.gitignore` 蹇界暐锛宑lone 鍚庡崟鐙?`go build` 浼氬洜 embed 鐩綍缂哄け澶辫触锛涘紑鍙戣€呴』鍏堟墽琛?`npm run build` 鎴栫洿鎺ヤ娇鐢?`npm run build:backend`锛堣剼鏈唴宸蹭覆琛屾墽琛屼袱姝ワ級銆傛琛屼负涓?daq-t1603 Win7 鐗堜竴鑷淬€?
## [0.3.0] - 2026-07-03
### Changed
- **CSV 褰曞埗鏀逛负姣忚澶囦竴涓枃浠?*锛氬師鍗曟枃浠惰璁″湪澶氳澶囧悓鏃跺綍鍒舵椂锛屼袱鍙拌澶囩殑纭欢鏃堕棿鎴充氦鏇胯烦璺冨啓鍏ュ悓涓€ CSV 鏂囦欢锛屾椂闂存埑鍒楀湪涓や釜鍊间箣闂存潵鍥炶烦鍙橈紝鏁版嵁鍒椾篃娣锋潅銆傜幇鎸?deviceId 璺敱鍒扮嫭绔嬫枃浠讹紝涓?wind-daq 璁捐瀵归綈銆?- 鏂囦欢鍚嶆牸寮忓彉鏇达紙涓嶅吋瀹规棫鐗堟湰锛夛細
  - 鏃э細`<prefix>_YYYYMMDD-HHMMSS.csv`
  - 鏂帮細`<prefix>-<deviceSlug>-YYYYMMDD-HHMMSS-NNN.csv`
  - `deviceSlug` 浼樺厛鐢ㄨ澶囧悕锛坰anitize 鍚庯級锛屽悓鍚嶅啿绐佹椂杩藉姞 deviceId 鍓?6 浣?- 鏂囦欢婊氬姩鏉′欢锛圡axSize/MaxRecordCount/MaxDuration锛夋敼涓?*鎸夎澶囩嫭绔嬭瘎浼?*锛屽仠姝㈡潯浠讹紙璺ㄨ澶囨眹鎬伙級淇濇寔涓嶅彉銆?- 褰曞埗鍚姩鏃朵笉鍐嶉鍒涘缓绌烘枃浠讹紝鏀逛负绗竴涓?payload 鍒拌揪鏃舵寜 deviceId 鎳掑垱寤猴紝閬垮厤澶氳澶囧満鏅笅鏈姇閫掓暟鎹殑璁惧浜х敓绌?CSV銆?
### Internal
- `CSVRecorder` 閲嶆瀯涓哄 writer 鏋舵瀯锛歚map[deviceId]*perDeviceWriter`锛屾瘡璁惧鐙珛鎸佹湁鏂囦欢/缂撳啿/缁熻锛屽崟 writer goroutine 涓茶娑堣垂 channel 娑堥櫎澶氳澶囬攣浜夌敤銆?- `core.RecordingConfig` 鏂板 `DeviceNames map[string]string` 瀛楁锛岀敱 backend 鍦?StartRecording 鏃朵粠 profiles 涓€娆℃€у～鍏?deviceId鈫抧ame 鏄犲皠锛宺ecorder 鐢ㄤ簬鐢熸垚浜虹被鍙鐨勬枃浠跺悕 slug銆?- `backend/app.go` 鐨?`StartRecordingWithConfig` 璋冩暣涓猴細鍏堝彇 profiles 鈫?鑱氬悎閫氶亾绮惧害 鈫?鏋勫缓 deviceNames map 鈫?娉ㄥ叆 RecordingConfig銆?- 娓呯悊 dead code锛氱Щ闄ゆ湭娑堣垂鐨?`autoDone`/`autoDoneOnce`/`signalAutoDone` 淇″彿鏈哄埗锛堥潬 `started.CompareAndSwap` + writerLoop 涓茶 I/O 宸蹭繚璇佸苟鍙戝畨鍏級锛屼互鍙?`perDeviceWriter` 鐨?`deviceID`/`headerWritten`/`totalRecords` 涓変釜鏈瀛楁銆?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.3.0锛歚VERSION`銆乣apps/desktop-wails/wails.json`銆乣apps/desktop-wails/frontend/package.json`銆乣apps/desktop-wails/frontend/package-lock.json`銆乣apps/desktop-wails/build/config.yml`銆乣apps/desktop-wails/build/windows/installer/project.nsi`銆?
### Verification
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`
### Known Issues
- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?- 澶?writer 鏍稿績璺敱閫昏緫锛坄getOrCreateWriter` / `sanitizeFileSegment` / `uniqueFileSlugLocked` / `shouldRotate(deviceID)`锛夋殏鏃犲崟鍏冩祴璇曡鐩栵紝渚濊禆瀹炴満澶氳澶囬獙璇併€?
## [0.2.4] - 2026-07-03
### Added
- 鏂板搴旂敤灞傝繛缁秴鏃舵柇杩炴娴嬶細`readLoop` 缁存姢 `consecutiveTimeouts` 璁℃暟鍣紝杩炵画 25 娆★紙5s锛塕eadFrame 瓒呮椂鍗宠皟鐢?`handleConnectionLost` 涓诲姩鍒ゅ畾鏂繛锛屼綔涓?TCP keepalive 涔嬪鐨勫揩閫熼€氶亾銆?
### Changed
- `p1604KeepAlivePeriod` 浠?`10 * time.Second` 璋冩暣涓?`3 * time.Second`锛學indows ~33s / Linux ~12s 鍏滃簳锛屾瘮鍘?~110s 蹇?3 鍊嶄互涓娿€?- 褰㈡垚鍙屼繚闄╂娴嬫灦鏋勶細閲囬泦鏈?readLoop 娲昏穬鏃剁敱杩炵画瓒呮椂璁℃暟鍣ㄤ富妫€娴嬶紙5s锛夛紝闈為噰闆嗘湡 readLoop 绌洪棽鏃剁敱 keepalive 鍏滃簳锛垀33s/12s锛夈€?
### Fixed
- 淇閫氶亾閫夋嫨鍣ㄧ粍浠舵枃鏈孩鍑烘埅鏂殑闂锛坴0.2.3 閬楁紡鏈彂甯冿級銆?- 淇 CH17/CH18 澶ф皵閫氶亾榛樿琚嬀閫夎繘瀹炴椂鍥捐〃鐨勯棶棰樸€?
### Internal
- 鍚屾鏇存柊 `enableTCPKeepalive` 璁捐娉ㄩ噴銆乣Connect` keepalive 鍚敤鍧楁敞閲娿€乣p1604ConsecutiveTimeoutThreshold` 鍙屼繚闄╄鏄庯紝淇涓庢柊鐗?keepalive 鏁板€硷紙3s/33s锛夌煕鐩剧殑杩囨湡鎻忚堪锛堝師 10s/100s/110s锛夈€?- 鍚屾 6 涓増鏈彿鏂囦欢鍒?0.2.4锛歚VERSION`銆乣apps/desktop-wails/wails.json`銆乣apps/desktop-wails/frontend/package.json`銆乣apps/desktop-wails/frontend/package-lock.json`銆乣apps/desktop-wails/build/config.yml`銆乣apps/desktop-wails/build/windows/installer/project.nsi`銆?- 閫氳繃 `npm install --package-lock-only` 鍚屾 package-lock.json 涓?package.json 鐗堟湰鍙枫€?
### Verification
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`
### Known Issues
- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?- 闈為噰闆嗘湡闂存棤鍛ㄦ湡鎬?I/O 瑙﹀彂 keepalive 澶辫触涓婃姤锛屾柇杩炴娴嬩粛渚濊禆涓嬩竴娆″懡浠よ皟鐢ㄣ€?
## [0.2.3] - 2026-07-03
### Fixed
- 淇鍋滄閲囬泦鍚庣珛鍗抽厤缃崟浣嶆椂杩斿洖 "unexpected v01101 response" 閿欒鐨勯棶棰樸€傚仠姝㈤噰闆嗗悗 TCP 缂撳啿鍖烘畫鐣欓噰闆嗘暟鎹抚锛寁01101 鍛戒护鐨?ReadFrame 鎶婃畫鐣欏綋浣滃搷搴旇鍑恒€傚湪 ApplyConfig 鍙戦€?v01101 鍓嶅鍔?frameReader.Reset + DrainConnection 鎺掔┖娈嬬暀鏁版嵁銆?
### Internal
- 瀵归綈 wind-daq 宸叉湁鐨?SetUnit 缂撳啿鍖烘帓绌轰慨澶嶆柟妗堛€?
### Verification
- `go test ./...`
- `go build -buildvcs=false ./...`
- `go vet ./...`
- `task release`
### Known Issues
- DAQ-P-1604 璁惧鍥轰欢鏃堕棿鎴?bug 浠嶅瓨鍦紝CSV 鏃堕棿鎴冲凡缁熶竴鎴柇鍒扮绾ц閬裤€?
## [0.2.2] - 2026-07-03
### Fixed
- 淇 CSV Timestamp 鍒楁椂闂存埑閿欒锛欴AQ-P-1604 璁惧纭欢鏃堕棿鎴冲瓨鍦ㄥ浐浠?bug锛坒ractional 瀛楁浠?~4348Hz 閫熺巼閫掑锛屾瘡绱Н绾?232ms 璺宠穬鏍℃锛夛紝瀵艰嚧 1000Hz 閲囬泦涓?1 姣鍐呭嚭鐜板甯ф椂闂存埑锛涚郴缁熸绉掓椂闂存埑鍦?1000Hz 涓嬬簿搴︿篃涓嶈冻銆傜粺涓€鎴柇鍒扮绾э紝閬垮厤灞曠ず閿欒鐨勬椂闂寸粏鍒嗐€?- 淇 `CSVRecorder.Stop()` 缂哄皯 `return nil` 瀵艰嚧鐨勭紪璇戦敊璇紙棰勫瓨闂锛岄樆濉為獙璇侊級銆?- 淇 `csv_recorder.go` 琛ㄥご娉ㄩ噴閿欒锛氫粠銆屽井绉掔簿搴︺€嶆洿姝ｄ负銆岀绾х簿搴︺€嶏紝涓庡疄闄呮牸寮忎覆涓€鑷淬€?
### Internal
- Taskfile `generate-icon` 浠诲姟鏀圭敤 `build/windows/info.json` 鍜?`build/windows/wails.exe.manifest` 妯℃澘鏂囦欢锛宍wails3 generate syso` 鍐呴儴鐢?`wails.json` 鐨?info 瀛楁娓叉煋妯℃澘銆傚垹闄ゅ啑浣欑殑鍏蜂綋鍊肩増鏈?`build/info.json` 鍜?`build/windows.manifest`锛岀増鏈彿婧愪粠 7 涓敹鏁涘埌 6 涓€?- 閲嶆柊鐢熸垚 `wails_windows_amd64.syso` 璧勬簮娈点€?- 鏂板椤圭洰 `README.md` 鍜?`CLAUDE.md` 鏂囨。锛屽榻?daq-t1603 / wind-daq 椤圭洰銆?
### Verification
- `go build ./...`: passed
- `go vet ./adapters/recording/...`: passed
- `go test ./adapters/recording/...`: passed (no test files)
### Known Issues
- 璁惧纭欢鏃堕棿鎴冲浐浠?bug 鏈慨澶嶏紝闇€鑱旂郴纭欢宸ョ▼甯堜慨澶嶅浐浠跺悗鎵嶈兘鎭㈠姣绮惧害鏃堕棿鎴炽€?
## [0.2.1] - 2026-07-02
### Added
- 鏂板鎵弿寮圭獥澶氶€?+ 鍐呰仈鏀瑰悕 + 鎵归噺娣诲姞璁惧鍔熻兘锛屾敮鎸侀娆¤鏈哄満鏅竴娆″嬀閫夊鍙拌澶囦竴閿惤搴撱€?- 鏂板纭欢閫氫俊 hardware-send/hardware-recv 鍒嗙被鏃ュ織锛屽墠绔€氫俊鍒嗙粍鍙瀹屾暣鍛戒护浜や簰娴佺▼銆?
### Changed
- 鎵弿寮圭獥鏀惧ぇ鑷?44rem x 80vh锛屽凡娣诲姞璁惧缃伆涓嶅彲閲嶅姞锛屾湭娣诲姞璁惧榛樿棰勫嬀閫夈€?- 娣诲姞鍚庣珛鍗宠Е鍙戞柊璁惧骞跺彂杩炴帴锛屼笉鍐嶉渶瑕侀噸鍚簲鐢ㄣ€?
### Internal
- deviceStoreHelpers 鎶藉嚭 6 涓函 TS 宸ュ叿鍑芥暟锛屾柊澧?18 鏉?vitest 鍗曞厓娴嬭瘯銆?- 琛ラ綈 build/config.yml 鍜?build/info.json 鐗堟湰鍙峰埌涓庨」鐩竴鑷淬€?
### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `npm test`: 18/18 passed
- `go build -tags production`: passed
- `makensis`: passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)
### Known Issues
- 鏆傛棤銆?
## [0.2.0] - 2026-07-01
### Added
- 鏂板 FileRotation 鏂囦欢婊氬姩閰嶇疆锛堟寜澶у皬/鏃堕暱/璁板綍鏁拌嚜鍔ㄥ垏鏂囦欢锛夈€?- 鏂板 StopConditions 褰曞埗鑷姩鍋滄鏉′欢銆?- 鏂板 RecordingStopping 鐘舵€佸拰 DroppedCount 涓㈠抚璁℃暟锛屽墠绔彲鏄剧ず鏁版嵁瀹屾暣鎬ф寚鏍囥€?- 鏂板 Taskfile.yml 鏋勫缓浠诲姟瀹氫箟銆?
### Changed
- RecordingPort.Start 鎺ユ敹 RecordingConfig 缁撴瀯浣擄紝鏇夸唬绂绘暎鍙傛暟銆?- RecordingSession 鏂板 Format/DroppedCount/FileCount/CurrentFile/LastError 瀛楁銆?- CSV 褰曞埗鍣ㄩ噸鏋勪负寮傛 writer 鏋舵瀯锛屾敮鎸佸璁惧骞跺彂鍐欏叆鍜屾枃浠舵粴鍔ㄣ€?- CSV Timestamp 鍒楁敼涓哄甫姣鐨勫崟鍒楁牸寮忥紝鍓嶇紑鍗曞紩鍙峰己鍒?Excel 鏂囨湰妯″紡銆?- 纭欢閫傞厤鍣?p1604_adapter 閲嶆瀯锛屾彁鍗囪繛鎺ョǔ瀹氭€с€?- 鍓嶇缁戝畾鍜?stores 灞傞€傞厤鏂扮殑褰曞埗閰嶇疆鍜岀姸鎬佹ā鍨嬨€?
### Removed
- 绉婚櫎 v0.1.x 瀹為獙鎬?Binary 褰曞埗鏍煎紡锛氭棤璇荤銆佸鍎挎牸寮忥紝缁存姢鎴愭湰楂樸€?  CSV 宸茶兘婊¤冻 1000Hz 閲囬泦闇€姹傘€傚師 v0.1.x 褰曞埗鐨?Binary 鏂囦欢鏃犳硶鍦ㄦ湰鐗堟湰璇诲彇銆?
### Internal
- AGENTS.md 澧炲姞 ADR-004 绱㈠紩銆?- 璋冩暣 appicon 鍥炬爣銆?
### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 鍥?go.sum 缂哄け涓嶅彲鐢紝鏀圭敤 go build 鐩村嚭)
- `makensis` 鏋勫缓瀹夎鍖? passed
- 鍐掔儫娴嬭瘯: passed (GUI 鍚姩姝ｅ父锛屾棤"correct build tags"閿欒)
### Known Issues
- 鏆傛棤銆?
## [0.1.1] - 2026-06-29
### Internal
- AGENTS.md 鏂板銆屽澶栦氦浠樻墦鍖呫€嶈妭锛屼娇鐢?wails3 build锛堝唴閮ㄨ嚜鍔ㄥ惎鐢?-tags production锛夈€?- 鍒涘缓 CHANGELOG.md 鍜屽彂甯冨熀纭€璁炬柦銆?
### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed
### Known Issues
- 鏆傛棤銆?
