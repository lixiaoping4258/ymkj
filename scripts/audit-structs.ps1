# audit-structs.ps1 —— 检查 service 结构体的字段是否都在构造函数里赋了值。
#
# 为什么需要它：
#   我在 MarketService 上加字段时，**三次漏掉构造函数的赋值**（saleOn / saleDetail /
#   goodsLook）。前两次编译就报错；**第三次编译、启动、其它接口全部正常**，
#   只有打那个接口时 panic —— 而 Kratos 的 recovery 把 panic 吞成了
#   `{"code":0,"show":1,"msg":"unknown request error"}`，看不到任何线索。
#
#   参见 README 第十一节第 15.5 / 15.6 条。
#
# 原理：
#   1. 扫每个 service 文件的 `type XxxService struct { ... }`，取出字段名
#   2. 在同一个文件里找 `字段名:` 形式的赋值
#   3. 字段名从未以 `name:` 出现过 -> 报"漏赋值"
#
# ⚠️ 这是**启发式**检查：
#   - 它只能发现"完全没赋值"，发现不了"赋错了值"
#   - 若字段名恰好出现在注释/参数里，可能误判为"已赋值"（**漏报**，不会误报）
#   - 反过来说，它报出来的**一定是真的漏了**
#
# **本脚本做过负向对照**：手动删掉 goodsLook 的赋值后，它正确报出漏赋值（不是"看起来能跑"）。

[CmdletBinding()]
param()

$ErrorActionPreference = 'Continue'
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

$files = Get-ChildItem 'internal\service' -Filter '*.go' -File |
    Where-Object { $_.Name -notmatch '_test' }

$problems = @()
$checked = 0

foreach ($f in $files) {
    $lines = [System.IO.File]::ReadAllLines($f.FullName)
    $structName = $null
    $fields = @()
    $inStruct = $false

    for ($i = 0; $i -lt $lines.Count; $i++) {
        $l = $lines[$i]
        if ($l -match '^type (\w+Service) struct \{') {
            $structName = $Matches[1]; $inStruct = $true; continue
        }
        if ($inStruct) {
            if ($l -match '^\}') { $inStruct = $false; continue }
            # 字段形如：`\tname Type` 或 `\tname *Type`；跳过注释
            if ($l -match '^\t(\w+)\s+[\*\[]?[\w\.]+' -and $l -notmatch '^\s*//') {
                $fields += $Matches[1]
            }
        }
    }
    if (-not $structName) { continue }

    $txt = [System.IO.File]::ReadAllText($f.FullName)
    $missing = @()
    foreach ($fl in $fields) {
        # 赋值形如 `field:`（可能在同一行的多项赋值里）
        if ($txt -notmatch "[\s{]$fl\s*:") { $missing += $fl }
    }

    $checked++
    if ($missing.Count -gt 0) {
        Write-Host ("  {0,-20} {1,-18} 漏赋值: {2}" -f $f.Name, $structName, ($missing -join ', ')) -ForegroundColor Red
        $problems += "$($f.Name) [$structName] -> $($missing -join ', ')"
    } else {
        Write-Host ("  {0,-20} {1,-18} ok ({2} 字段)" -f $f.Name, $structName, $fields.Count) -ForegroundColor Green
    }
}

Write-Host ""
if ($problems.Count -gt 0) {
    Write-Host "AUDIT FAIL: 有 service 结构体字段没有在构造函数里赋值 —— 打该接口时会 nil panic。" -ForegroundColor Red
    $problems | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    Write-Host "（这个 panic 会被 recovery 吞成 `"unknown request error`"，很难查 —— 所以在这里拦。）" -ForegroundColor Yellow
    exit 1
}
Write-Host "AUDIT PASS: $checked 个 service 结构体的字段均已赋值" -ForegroundColor Green
exit 0
