# 按指定 profile 生成模块接线代码并构建。
#
#   .\build.ps1                  # 默认 all
#   .\build.ps1 -Profile netcfg-only
#
# profile 定义在 modules.json 里。

param(
    [string]$Profile = "all"
)

$ErrorActionPreference = "Stop"

go run ./tools/genmodules -profile $Profile
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

wails build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "构建完成：build\bin\embedtools.exe (profile: $Profile)"
