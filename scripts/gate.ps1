# gate.ps1 —— 提交前的完整质量门。任何一项失败就以非 0 退出。
#
# 为什么要这个脚本：
#   本会话**两次**在 go vet / go test 失败的情况下提交并推送了代码
#   （见 README 第十一节第 14 条）。两次原因相同：把 vet/test 与别的命令
#   串在同一条 pwsh 里，又用 Select-Object 截断输出，于是 [build failed]
#   被截掉、没看见就继续提交。
#
# 用法（提交前必跑）：
#   .\scripts\gate.ps1
#   if ($LASTEXITCODE -ne 0) { throw "质量门未通过，禁止提交" }
#
# 两个 PowerShell 陷阱（本脚本第一版都踩到了）：
#   1. -match 默认大小写不敏感 —— 'FAIL' 会匹配到 "auth failure" 这种 PASS 行。
#      判断日志里的 FAIL/ERROR 必须用 -cmatch（区分大小写）。
#   2. 把命令输出接进管道（cmd 2>&1 | ...）会重置 $LASTEXITCODE，
#      所以不能在管道之后再检查退出码。本脚本改为：先执行、立刻记下退出码、
#      再处理已捕获到变量里的输出。

[CmdletBinding()]
param(
    [switch]$SkipSmoke
)

$ErrorActionPreference = 'Continue'
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

$failed = @()

function Step {
    param([string]$Name, [scriptblock]$Body)
    Write-Host ""
    Write-Host "==> $Name" -ForegroundColor Cyan
    $global:LASTEXITCODE = 0
    # 先执行并把输出收进变量，不要在这里接管道（管道会重置 LASTEXITCODE）
    # ⚠️ 必须 try/catch：脚本块里的 throw 会**穿透** Step 直接终止整个脚本，
    #    那样后续的门不会跑、也不会打印 GATE FAIL 汇总。
    $out = @()
    $code = 0
    try {
        $out = & $Body
        $code = $LASTEXITCODE
    } catch {
        $out = @("$_")
        $code = 1
    }
    # 再打印（不截断，失败时要能看到原始错误）
    $out | ForEach-Object { "    $_" }
    if ($code -ne 0) {
        Write-Host "    x $Name 失败 (exit $code)" -ForegroundColor Red
        $script:failed += $Name
    } else {
        Write-Host "    ok $Name" -ForegroundColor Green
    }
}

Step 'gofmt -l（有未格式化文件即失败）' {
    $unformatted = gofmt -l . 2>&1
    if ($LASTEXITCODE -ne 0) { return $unformatted }
    if ($unformatted) {
        Write-Output "以下文件未格式化："
        $unformatted | ForEach-Object { Write-Output "  $_" }
        throw "gofmt: 有未格式化文件"
    }
    Write-Output "(无未格式化文件)"
}

Step 'go build ./...' { go build ./... }
Step 'go vet ./...' { go vet ./... }
Step 'go test ./... -count=1' { go test ./... -count=1 }

if (-not $SkipSmoke) {
    Step 'scripts/smoke.ps1' {
        # ⚠️ smoke.ps1 用 Write-Host（走 Information 流 stream 6），`2>&1` **抓不到**，
        # 必须用 `*>&1` 才能捕获它的全部输出。
        $r = & "$PSScriptRoot\smoke.ps1" *>&1
        # 只打印与本门相关的行；用 -cmatch 避免 "failure" 被当成 FAIL
        $r | Where-Object { $_ -cmatch 'FAIL|result:' } | ForEach-Object { Write-Output $_ }
        # 逐行找 result 行（比 Out-String + 正则稳）
        $line = $r | Where-Object { $_ -match 'result:' } | Select-Object -Last 1
        if (-not $line) { throw "smoke: 没找到 result 行（服务可能没起）" }
        $m = [regex]::Match([string]$line, '(\d+)\s+passed,\s*(\d+)\s+failed')
        if (-not $m.Success) { throw "smoke: result 行格式不认识: $line" }
        if ([int]$m.Groups[2].Value -ne 0) { throw "smoke: $($m.Groups[2].Value) 项失败" }
        Write-Output "(smoke: $($m.Groups[1].Value) 项全通过)"
    }
}

Write-Host ""
if ($failed.Count -gt 0) {
    Write-Host "GATE FAIL: $($failed -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host "GATE PASS: 全部门通过（gofmt / build / vet / test / smoke）" -ForegroundColor Green
exit 0