.PHONY: check desktop-dev desktop-build desktop-build-installer

check:
	pwsh ./scripts/check.ps1

desktop-dev:
	wails dev

desktop-build:
	wails build -clean
	powershell -ExecutionPolicy Bypass -File scripts/copy-dll.ps1

# 打包安装包：先 build（-clean 会清空 bin，DLL 需在 NSIS 打包前放入 bin），
# 复制 WTNDAQ16H_64.dll 后再 wails build --nsis（不带 -clean，避免再清 bin）。
desktop-build-installer:
	wails build -clean
	powershell -ExecutionPolicy Bypass -File scripts/copy-dll.ps1
	wails build --nsis
