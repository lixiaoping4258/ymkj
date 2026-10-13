# safe-commit.ps1 —— 先跑质量门，**只有通过才提交**。
#
# 为什么需要它：
#   2026-10-13 01:2x，我跑完 gate 看到 `GATE FAIL: scripts/smoke.ps1`，
#   但同一条命令里紧接着就 `git add -A && git commit` —— **没检查退出码**，
#   把 smoke 失败的提交推了出去。
#
#   这已经是同类流程问题第 3 次（前两次是 vet/test 失败仍提交）。
#   gate.ps1 解决了"看不见失败"，但没有解决"看见了还继续提交"。
#   所以把「跑门 + 提交」合成一件事，让它们无法分开。
#
# 用法：
#   .\scripts\safe-commit.ps1 -MessageFile D:\work\harness\commitmsgN.txt
#   .\scripts\safe-commit.ps1 -Message "一句话说明"
#   .\scripts\safe-commit.ps1 -MessageFile ... -SkipSmoke      # 服务没起时
#   .\scripts\safe-commit.ps1 -MessageFile ... -NoPush         # 只提交不推

[CmdletBinding()]
param(
    [string]$MessageFile,
    [string]$Message,
    [switch]$SkipSmoke,
    [switch]$NoPush
)

$ErrorActionPreference = 'Continue'
$repo = Split-Path -Parent $PSScriptRoot

if (-not $MessageFile -and -not $Message) {
    Write-Host "必须提供 -MessageFile 或 -Message" -ForegroundColor Red
    exit 2
}

# ---------- 1. 质量门 ----------
Write-Host "=== 步骤 1/3：质量门 ===" -ForegroundColor Cyan
$gateArgs = @{}
if ($SkipSmoke) { $gateArgs['SkipSmoke'] = $true }
& "$PSScriptRoot\gate.ps1" @gateArgs
$gateExit = $LASTEXITCODE
Write-Host ""
if ($gateExit -ne 0) {
    Write-Host "质量门未通过（exit $gateExit）—— **拒绝提交**。" -ForegroundColor Red
    Write-Host "先把门修绿，再重新运行本脚本。" -ForegroundColor Yellow
    exit 1
}

# ---------- 2. 提交 ----------
Write-Host "=== 步骤 2/3：提交 ===" -ForegroundColor Cyan
Push-Location $repo
git add -A
if ($MessageFile) {
    git -c i18n.commitEncoding=utf-8 -c user.name="kratos-refactor" -c user.email="noreply@local" commit -q -F $MessageFile
} else {
    git -c i18n.commitEncoding=utf-8 -c user.name="kratos-refactor" -c user.email="noreply@local" commit -q -m $Message
}
$commitExit = $LASTEXITCODE
Pop-Location
if ($commitExit -ne 0) {
    Write-Host "提交失败（exit $commitExit）—— 可能是没有改动，或钩子拒绝。" -ForegroundColor Red
    exit 1
}
Write-Host "  已提交 $(git -C $repo rev-parse --short HEAD)"

# ---------- 3. 推送（先确认远端是本地祖先，绝不强推）----------
if ($NoPush) {
    Write-Host "=== 步骤 3/3：跳过推送（-NoPush）===" -ForegroundColor Cyan
    exit 0
}
Write-Host "=== 步骤 3/3：推送 ===" -ForegroundColor Cyan
$remote = (git -C $repo ls-remote origin refs/heads/develop 2>&1 | ForEach-Object { $_.Split()[0] })
if (-not $remote) { Write-Host "  读不到远端 —— 不推，请人工确认" -ForegroundColor Red; exit 1 }
git -C $repo merge-base --is-ancestor $remote HEAD 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ⚠️ 远端不在本地历史里 —— **不推**，请人工确认（绝不强推）" -ForegroundColor Red
    exit 1
}
git -C $repo push origin develop 2>&1 | Select-Object -First 2 | ForEach-Object { Write-Host "  $_" }
Write-Host "  远端 $((git -C $repo ls-remote origin refs/heads/develop 2>&1 | ForEach-Object { $_.Split()[0].Substring(0,7) }))  本地 $(git -C $repo rev-parse --short HEAD)" -ForegroundColor Green
exit 0
