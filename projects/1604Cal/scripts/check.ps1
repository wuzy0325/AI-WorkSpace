$ErrorActionPreference = 'Stop'

# 1604Cal 不在根 go.work 的 use 列表（依赖通过 go.mod replace 引用 shared/device-sdk），
# 必须以单模块模式构建；workspace 模式下会报 "module not one of the workspace modules"
$env:GOWORK = 'off'

function Invoke-CheckedCommand {
    param(
        [string]$Command,
        [string[]]$Arguments
    )

    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "command failed: $Command $($Arguments -join ' ')"
    }
}

Write-Host '[check] go test ./cmd/... ./internal/...'
Invoke-CheckedCommand -Command 'go' -Arguments @('test', './cmd/...', './internal/...')

Write-Host '[check] go vet ./cmd/... ./internal/...'
Invoke-CheckedCommand -Command 'go' -Arguments @('vet', './cmd/...', './internal/...')

Write-Host '[check] npm --prefix web run typecheck'
Invoke-CheckedCommand -Command 'npm' -Arguments @('--prefix', 'web', 'run', 'typecheck')

Write-Host '[check] npm --prefix web run lint'
Invoke-CheckedCommand -Command 'npm' -Arguments @('--prefix', 'web', 'run', 'lint')

Write-Host '[check] npm --prefix web run test'
Invoke-CheckedCommand -Command 'npm' -Arguments @('--prefix', 'web', 'run', 'test')
