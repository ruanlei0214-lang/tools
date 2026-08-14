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

go run ./tools/packportable
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# 产物名从 wails.json 读，别在这儿再写一份：它带版本号，改版本时漏改这里
# 只会让提示指向一个不存在的文件。
$exe = (Get-Content "$PSScriptRoot\wails.json" -Raw | ConvertFrom-Json).outputfilename
Write-Host "构建完成：build\bin\$exe\  (profile: $Profile)"
