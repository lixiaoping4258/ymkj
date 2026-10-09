# 从 proto 提取所有路由，生成统一的 routes 文件。
#
# 为什么需要它：Kratos 的路由分散在各 proto 的 `option (google.api.http)` 里，
# 没有像原 PHP 项目 app/api/route/*.php 那样的单一来源。
# 这个脚本把两处真相合并成一份文档，并且**从代码生成**，不会与实际实现脱节：
#
#   1. api/xtravel/v1/*.proto  -> 方法、路径、rpc 名
#   2. internal/server/middleware.go 的 defaultPublicOps -> 是否免登录
#   3. docs/ROUTES-AND-AUTH.md -> 原项目全部 131 条有效路由（含未迁的）
#
# 输出：docs/ROUTES.md
#
# 用法：.\scripts\gen-routes.ps1
# 注意：本文件含中文，必须存为 UTF-8 **带 BOM**（PS 5.1 否则按 GBK 读会报
#       "Missing closing ')'" 之类的假错误，见 README 10.1）。
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# --- 1. 从 proto 提取已实现路由 ---
$impl = @()
Get-ChildItem 'api\xtravel\v1\*.proto' | ForEach-Object {
    $svc = ''; $rpc = ''
    foreach ($line in [System.IO.File]::ReadAllLines($_.FullName)) {
        if ($line -match '^\s*service\s+(\w+)') { $svc = $Matches[1] }
        if ($line -match '^\s*rpc\s+(\w+)\s*\(') { $rpc = $Matches[1] }
        if ($line -match '(get|post|put|delete):\s*"([^"]+)"') {
            $impl += [pscustomobject]@{
                Method = $Matches[1].ToUpper()
                Path   = $Matches[2]
                Svc    = $svc
                Rpc    = $rpc
                File   = $_.Name
            }
        }
    }
}
$impl = $impl | Sort-Object Path -Unique

# --- 2. 从 middleware.go 提取免登录白名单 ---
$pub = @()
$mw = [System.IO.File]::ReadAllText('internal\server\middleware.go')
foreach ($m in [regex]::Matches($mw, '"(/xtravel\.v1\.[^"]+)"\s*:\s*\{\}')) {
    $pub += $m.Groups[1].Value
}
$pubSet = @{}
foreach ($p in $pub) { $pubSet[$p] = $true }

# --- 3. 从 ROUTES-AND-AUTH.md 读原项目全量路由（用于算进度） ---
$total = 0; $publicTotal = 0
$ra = 'docs\ROUTES-AND-AUTH.md'
if (Test-Path $ra) {
    $txt = [System.IO.File]::ReadAllText($ra)
    foreach ($m in [regex]::Matches($txt, '(?m)^\|\s*(GET|POST|PUT|DELETE)\s*\|\s*`([^`]+)`\s*\|[^|]*\|\s*(\*\*是\*\*|否)\s*\|\s*(✅?)\s*\|')) {
        $total++
        if ($m.Groups[3].Value -eq '**是**') { $publicTotal++ }
    }
}

# --- 输出 ---
$out = New-Object System.Collections.Generic.List[string]
$out.Add('# 统一路由表（本文件由 scripts/gen-routes.ps1 自动生成，请勿手改）')
$out.Add('')
$out.Add('> **为什么有这份文件**：Kratos 的路由分散在各 `api/xtravel/v1/*.proto` 的')
$out.Add('> `option (google.api.http)` 里，没有原 PHP 项目 `app/api/route/*.php` 那样的单一来源。')
$out.Add('> 这份表把 proto、免登录白名单两处真相合并，且**从代码生成**，不会脱节。')
$out.Add('')
$out.Add('> 改路由后重新生成：`.\scripts\gen-routes.ps1`')
$out.Add('')
$pct = if ($total -gt 0) { [Math]::Round($impl.Count / $total * 100, 1) } else { 0 }
$out.Add("**已实现 $($impl.Count) / $total 条（$pct%）；其中免登录 $(($impl | Where-Object { $pubSet["/xtravel.v1.$($_.Svc)/$($_.Rpc)"] }).Count) 条。**")
$out.Add('')

$out.Add('## 免登录（无需 token）')
$out.Add('')
$out.Add('| 方法 | 路径 | 服务·方法 | proto |')
$out.Add('|---|---|---|---|')
foreach ($r in $impl) {
    if ($pubSet["/xtravel.v1.$($r.Svc)/$($r.Rpc)"]) {
        $out.Add("| $($r.Method) | ``$($r.Path)`` | $($r.Svc).$($r.Rpc) | ``$($r.File)`` |")
    }
}
$out.Add('')

$out.Add('## 需要 token')
$out.Add('')
$out.Add('| 方法 | 路径 | 服务·方法 | proto |')
$out.Add('|---|---|---|---|')
foreach ($r in $impl) {
    if (-not $pubSet["/xtravel.v1.$($r.Svc)/$($r.Rpc)"]) {
        $out.Add("| $($r.Method) | ``$($r.Path)`` | $($r.Svc).$($r.Rpc) | ``$($r.File)`` |")
    }
}
$out.Add('')

$out.Add('## 接口约定（三条，改动会直接打挂前端）')
$out.Add('')
$out.Add('1. **响应信封**：`{"code":1,"show":0,"msg":"","data":{...}}` —— **`code=1` 才是成功**')
$out.Add('   （Kratos 默认 `code=0` 成功，是反的，所以替换了 `ResponseEncoder`）')
$out.Add('2. **HTTP 状态码恒为 200**，业务码只在 body 里；失败是 `code:0, show:1`')
$out.Add('3. **鉴权**：token 放在名为 **`token`** 的请求头里（不是 `Authorization`）')
$out.Add('')
$out.Add('## 免登录白名单的实现位置')
$out.Add('')
$out.Add('`internal/server/middleware.go` 的 `defaultPublicOps`（map 的 key 是完整的')
$out.Add('operation 名 `/<package>.<Service>/<Method>`）。')
$out.Add('')
$out.Add('> ⚠️ 原项目是在控制器上声明 `$notNeedLogin`，Kratos 没有"控制器对象"，')
$out.Add('> 只能展开成 operation 全路径。**改名时要同步改这里**，')
$out.Add('> 否则免登录判定会静默失效（两个方向都会出事：该公开的变需登录 -> 前端 403；')
$out.Add('> 该需登录的变公开 -> 未授权访问）。')
$out.Add('')

[System.IO.File]::WriteAllText(
    (Join-Path $root 'docs\ROUTES.md'),
    ($out -join "`n"),
    (New-Object System.Text.UTF8Encoding($false))
)
Write-Output "已生成 docs\ROUTES.md"
Write-Output "  已实现 $($impl.Count) 条（免登录 $(($impl | Where-Object { $pubSet["/xtravel.v1.$($_.Svc)/$($_.Rpc)"] }).Count) 条）"
Write-Output "  原项目全量 $total 条（免登录 $publicTotal 条）"
