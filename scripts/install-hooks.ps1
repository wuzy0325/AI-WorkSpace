# install-hooks.ps1
# 把 .githooks/ 下的 hook 部署到 .git/hooks/，使本地 pre-commit 守卫生效。
#
# 设计说明：
#   - 不修改 git core.hooksPath（若修改会把所有 hook 查找路径切到 .githooks，
#     导致 .git/hooks/post-commit 等既有 hook 失效，例如 Qoder AI tracker）。
#   - 改用复制部署：把 .githooks/pre-commit 复制到 .git/hooks/pre-commit，
#     保留 .git/hooks 下其他既有 hook 不受影响。
#   - .githooks/ 作为被 git 跟踪的"源"，便于跨机器/克隆复用；新克隆后跑一次本脚本即可启用。
#
# 用法：在仓库根目录运行  powershell -ExecutionPolicy Bypass -File .\scripts\install-hooks.ps1

$ErrorActionPreference = 'Stop'

$hookSrc = Join-Path $PSScriptRoot '..\.githooks\pre-commit' | Resolve-Path -ErrorAction Stop
$hooksDir = Join-Path $PSScriptRoot '..\.git\hooks'
$hookDst = Join-Path $hooksDir 'pre-commit'

if (-not (Test-Path $hooksDir)) {
    New-Item -ItemType Directory -Path $hooksDir -Force | Out-Null
}

# 复制 hook 文件
Copy-Item -Path $hookSrc.Path -Destination $hookDst -Force

Write-Host "已部署 pre-commit hook 到 .git\hooks\pre-commit" -ForegroundColor Green
Write-Host "  提交前将自动运行 validate-import-direction.ps1 校验架构 import 方向。" -ForegroundColor Green
Write-Host "  若校验失败会阻止提交；修复违规后重试即可。" -ForegroundColor Green
