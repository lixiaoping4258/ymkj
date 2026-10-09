# 解析原 PHP 项目的路由，生成 docs/ROUTES-AND-AUTH.md。
#
# ⚠️⚠️ 两个必须做对的地方（我都踩过）：
#
#  1. **要跟踪 Route::group 的 group 前缀**。各路由文件的分组各不相同
#     （account.php 是 '/v1/login'、common.php 是 'v1/common/'、Tao.php 是 'v1/tao'
#     、whitelist.php 甚至有 3 个不同的组），**用文件名当前缀会得到错误路径**。
#
#  2. **必须排除被注释掉的路由行**。article.php 里 6 条、whitelist.php 里 6 条
#     是 `// Route::get(...)` 形式的注释，不排除会让分母虚高 12 条
#     （140 vs 真实 131）。
#
# 用法：.\scripts\gen-routes-php.ps1
# 注意：含中文，必须存为 UTF-8 带 BOM。
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

# 原 PHP 项目路径（可用环境变量覆盖）
$phpProj = $env:XTRAVEL_PHP_DIR
if (-not $phpProj) { $phpProj = 'D:\work\phpstudy_pro\WWW\xTravel' }
$routeDir = Join-Path $phpProj 'app\api\route'
if (-not (Test-Path $routeDir)) {
    Write-Error "找不到原项目路由目录：$routeDir（可用 `$env:XTRAVEL_PHP_DIR 覆盖）"
}
$ctrlDir = Join-Path $phpProj 'app\api\controller'

# --- 1. 收集每个控制器的 $notNeedLogin ---
$notNeed = @{}
Get-ChildItem $ctrlDir -Recurse -Filter '*Controller.php' -File | ForEach-Object {
    $src = [System.IO.File]::ReadAllText($_.FullName, [System.Text.Encoding]::UTF8)
    if ($src -match 'notNeedLogin\s*=\s*\[([^\]]*)\]') {
        $notNeed[$_.BaseName] = (($Matches[1] -replace "'", '' -replace '"', '' -replace ' ', '') -split ',') |
            Where-Object { $_ }
    }
    else { $notNeed[$_.BaseName] = @() }
}

# --- 2. 解析路由（跟踪 group 栈 + 排除注释行） ---
$rows = @()
Get-ChildItem $routeDir -Filter '*.php' -File | ForEach-Object {
    $file = $_.Name
    $stack = New-Object System.Collections.Generic.List[string]
    foreach ($line in [System.IO.File]::ReadAllLines($_.FullName)) {
        $t = $line.Trim()

        # ⚠️ 排除注释：以 // 或 * 或 # 开头的一律不算
        $isComment = $t.StartsWith('//') -or $t.StartsWith('*') -or $t.StartsWith('#')

        if ($line -match "Route::group\(\s*'([^']+)'") {
            if (-not $isComment) { $stack.Add($Matches[1]) }
            continue
        }
        if ($t -match '^\}\);') {
            if ($stack.Count -gt 0) { $stack.RemoveAt($stack.Count - 1) }
            continue
        }
        if ($line -match "Route::(get|post|put|delete)\(\s*'([^']+)'\s*,\s*\[\s*(\w+)::class\s*,\s*'(\w+)'") {
            if ($isComment) { continue }   # ← 关键：跳过被注释的路由
            $prefix = if ($stack.Count -gt 0) { $stack[$stack.Count - 1] } else { '' }
            $full = '/' + (($prefix.Trim('/') + '/' + $Matches[2]).TrimStart('/'))
            $cls = $Matches[3]
            $act = $Matches[4]
            $isPub = if ($notNeed.ContainsKey($cls)) { $notNeed[$cls] -contains $act } else { $false }
            $rows += [pscustomobject]@{
                Method = $Matches[1].ToUpper()
                Path   = $full
                Ctrl   = $cls
                Act    = $act
                Pub    = $isPub
                File   = $file
            }
        }
    }
}
$rows = $rows | Sort-Object Path -Unique

# --- 3. 已迁移的路径（从 proto 提取，保持与实现一致） ---
$done = @()
Get-ChildItem (Join-Path $root 'api\xtravel\v1\*.proto') | ForEach-Object {
    foreach ($m in [regex]::Matches([System.IO.File]::ReadAllText($_.FullName), '(get|post):\s*"([^"]+)"')) {
        $done += $m.Groups[2].Value
    }
}
$doneSet = @{}; foreach ($d in $done) { $doneSet[$d] = $true }

# --- 4. 输出 ---
$out = New-Object System.Collections.Generic.List[string]
$out.Add('# C 端 `app/api` 路由 × 免登录 × 迁移状态（由 scripts/gen-routes-php.ps1 生成）')
$out.Add('')
$out.Add('> 静态解析原 PHP 项目：从路由文件按 **`Route::group` 分组** 解析 `Route::` 声明')
$out.Add('> （**必须跟踪 group 前缀**，且**必须排除注释行**），')
$out.Add('> 再从控制器抽 `$notNeedLogin` 交叉得出。')
$out.Add('')
$pubCount = ($rows | Where-Object { $_.Pub }).Count
$doneCount = ($rows | Where-Object { $doneSet[$_.Path] }).Count
$out.Add("> **共 $($rows.Count) 条有效路由，其中免登录 $pubCount 条，已迁 $doneCount 条。**")
$out.Add('')
$out.Add('> ⚠️ **这是迁移时最容易标错的地方**，两个方向都会出事：')
$out.Add('> 把该登录的标成公开 = 未授权访问；把该公开的标成需登录 = 前端匿名访问直接 403。')
$out.Add('> **不要凭接口名猜。**')
$out.Add('')
$out.Add('## 各路由文件的 group 前缀（路径由它决定）')
$out.Add('')
$out.Add('| 文件 | 有效路由 | 被注释 | group 前缀 |')
$out.Add('|---|---|---|---|')
Get-ChildItem $routeDir -Filter '*.php' -File | Sort-Object Name | ForEach-Object {
    $groups = @(); $act = 0; $com = 0
    foreach ($line in [System.IO.File]::ReadAllLines($_.FullName)) {
        $t = $line.Trim()
        if ($line -match "Route::group\(\s*'([^']+)'") { if (-not ($t.StartsWith('//'))) { $groups += $Matches[1] } }
        if ($line -match 'Route::(get|post|put|delete)\(') {
            if ($t.StartsWith('//')) { $com++ } else { $act++ }
        }
    }
    $out.Add("| ``$($_.Name)`` | $act | $com | ``$(($groups | Select-Object -Unique) -join '``, ``')`` |")
}
$out.Add('')
$out.Add('## 全量路由表')
$out.Add('')
$out.Add('| 方法 | 完整路径 | 控制器::方法 | 免登录 | 已迁 |')
$out.Add('|---|---|---|---|---|')
foreach ($r in $rows) {
    $pubCell = if ($r.Pub) { '**是**' } else { '否' }
    $doneCell = if ($doneSet[$r.Path]) { '✅' } else { '' }
    $out.Add("| $($r.Method) | ``$($r.Path)`` | $($r.Ctrl)::$($r.Act) | $pubCell | $doneCell |")
}
$out.Add('')
$out.Add('## 免登录接口清单')
$out.Add('')
$out.Add('```')
$rows | Where-Object { $_.Pub } | ForEach-Object { $out.Add("$($_.Method) $($_.Path)") }
$out.Add('```')
$out.Add('')

[System.IO.File]::WriteAllText(
    (Join-Path $root 'docs\ROUTES-AND-AUTH.md'),
    ($out -join "`n"),
    (New-Object System.Text.UTF8Encoding($false))
)
Write-Output "已生成 docs\ROUTES-AND-AUTH.md"
Write-Output "  有效路由 $($rows.Count) 条（免登录 $pubCount 条，已迁 $doneCount 条）"
