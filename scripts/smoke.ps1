# End-to-end smoke test against a running instance.
#
# Checks the things that actually break in this migration:
#   - the response envelope is {code,show,msg,data}, not Kratos' default
#   - code=1 means SUCCESS (flipped vs Kratos)
#   - HTTP status stays 200 even for auth failures
#   - auth failures carry code=-403 and show=0 exactly like the PHP side
#   - json keys match the PHP side verbatim (snake_case vs camelCase differs
#     per endpoint -- this project has BOTH)
#   - 64-bit ints come back as NUMBERS, not proto3's int64-as-string
#   - decimal fields (user_money) come back as STRINGS
#   - nullable columns come back as null, not ""
#   - list endpoints put an ARRAY in `data`, not {"items":[...]}
#
# ============================================================================
# THIS FILE MUST STAY ASCII-ONLY.
# Windows PowerShell 5.1 reads .ps1 as ANSI, not UTF-8. A UTF-8 Chinese
# comment gets mis-decoded into byte pairs that can swallow the newline and
# shift every subsequent line number -- which produces bewildering parse
# errors far away from the real cause. Chinese text is built from code points
# with -join instead (see wantNoToken below).
# ============================================================================

param(
    [string]$Base = 'http://127.0.0.1:18000',
    [string]$XTravelRoot = 'D:\work\phpstudy_pro\WWW\xTravel'
)

$ErrorActionPreference = 'Stop'
$script:pass = 0
$script:fail = 0

function Check($name, $cond, $detail) {
    if ($cond) {
        $script:pass++
        Write-Host ("  PASS  {0}" -f $name) -ForegroundColor Green
    } else {
        $script:fail++
        Write-Host ("  FAIL  {0}  {1}" -f $name, $detail) -ForegroundColor Red
    }
}

# Invoke-WebRequest throws on 4xx/5xx; we want the body anyway.
function Get-Json($url, $headers) {
    try {
        $r = Invoke-WebRequest $url -Headers $headers -UseBasicParsing -TimeoutSec 20
        return @{ status = $r.StatusCode; body = ($r.Content | ConvertFrom-Json); raw = $r.Content }
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $sr = New-Object IO.StreamReader($resp.GetResponseStream())
            $txt = $sr.ReadToEnd()
            return @{ status = [int]$resp.StatusCode; body = ($txt | ConvertFrom-Json); raw = $txt }
        }
        throw
    }
}

function Has-Envelope($body) {
    return ($null -ne $body.code) -and ($null -ne $body.show) -and
           ($null -ne $body.msg) -and ($body.PSObject.Properties.Name -contains 'data')
}

# Build a string from code points (keeps this file ASCII).
function FromCodePoints([int[]]$cps) {
    return -join @($cps | ForEach-Object { [char]$_ })
}

$wantNoToken  = FromCodePoints @(0x8BF7,0x6C42,0x8BA4,0x8BC1,0x4FE1,0x606F,0x6709,0x8BEF,
                                 0xFF0C,0x8BF7,0x91CD,0x65B0,0x767B,0x5F55)
$wantBadToken = FromCodePoints @(0x767B,0x5F55,0x8D85,0x65F6,0xFF0C,0x8BF7,0x91CD,0x65B0,
                                 0x767B,0x5F55)
$wantTeaJiao  = FromCodePoints @(0x8336,0x4EA4,0x6240)   # "cha jiao suo"

Write-Host "smoke test against $Base"
Write-Host ""

# ---------------------------------------------------------------- liveness
$live = Get-Json "$Base/v1/common/config" @{}
Check "service is up" ($live.status -eq 200) "HTTP $($live.status)"

# ---------------------------------------------------------------- envelope
Check "envelope has code/show/msg/data" (Has-Envelope $live.body) $live.raw
Check "public endpoint succeeds with code=1 (NOT kratos' 0)" ($live.body.code -eq 1) "code=$($live.body.code)"
Check "success msg is empty string" ($live.body.msg -eq '') "msg='$($live.body.msg)'"

# protojson omits zero values unless EmitUnpopulated is on,
# while PHP's json_encode always emits them.
$data = $live.body.data
Check "tradeSwitch present (not dropped as zero)" ($null -ne $data.tradeSwitch) $live.raw
Check "outFee present" ($null -ne $data.outFee) $live.raw
Check "periods present" ($null -ne $data.tradeSwitch.periods) $live.raw

# common.proto keys are camelCase because the PHP source uses camelCase here
$ck = $data.PSObject.Properties.Name
Check "common/config key 'outFee' camelCase" ($ck -contains 'outFee') "keys=$($ck -join ',')"
Check "common/config key 'buyOrSaleConfig' camelCase" ($ck -contains 'buyOrSaleConfig') "keys=$($ck -join ',')"
$tsk = $data.tradeSwitch.PSObject.Properties.Name
Check "tradeSwitch key 'isOpen' camelCase" ($tsk -contains 'isOpen') "keys=$($tsk -join ',')"
Check "tradeSwitch key 'todayTypeStr' camelCase" ($tsk -contains 'todayTypeStr') "keys=$($tsk -join ',')"

# trade/config is NOT in IndexController::$notNeedLogin (['test','index','config',
# 'policy','decorate']) so it REQUIRES login. Stage 1 wrongly marked it public and
# this assertion used to encode that mistake -- never derive the expectation from
# our own implementation, read the PHP.
$tcNoTok = Get-Json "$Base/v1/common/trade/config" @{}
Check "trade/config requires login (no token -> -403)" ($tcNoTok.body.code -eq -403) "code=$($tcNoTok.body.code)"

# ---------------------------------------------------------------- auth failures
$noTok = Get-Json "$Base/v1/user/info" @{}
Check "auth required without token" ($noTok.body.code -eq -403) "code=$($noTok.body.code)"
Check "auth failure keeps HTTP 200" ($noTok.status -eq 200) "HTTP $($noTok.status)"
Check "auth failure show=0 (frontend must not popup)" ($noTok.body.show -eq 0) "show=$($noTok.body.show)"
Check "auth failure msg matches PHP verbatim" ($noTok.body.msg -eq $wantNoToken) "msg='$($noTok.body.msg)'"

$badTok = Get-Json "$Base/v1/user/info" @{ token = 'deadbeefdeadbeefdeadbeefdeadbeef' }
Check "invalid token rejected" ($badTok.body.code -eq -403) "code=$($badTok.body.code)"
Check "invalid token show=0" ($badTok.body.show -eq 0) "show=$($badTok.body.show)"
Check "invalid token msg matches PHP verbatim" ($badTok.body.msg -eq $wantBadToken) "msg='$($badTok.body.msg)'"

# market endpoints also require login
$pwNoTok = Get-Json "$Base/v1/market/pay_way" @{}
Check "market/pay_way requires login" ($pwNoTok.body.code -eq -403) "code=$($pwNoTok.body.code)"

# BUT PurchaseController::$notNeedLogin = ['index'], so this one is PUBLIC.
# Getting this backwards is exactly the bug Stage 1 had with trade/config.
$piNoTok = Get-Json "$Base/v1/market/purchase?page_no=1&page_size=2" @{}
Check "market/purchase is PUBLIC (no token -> code=1)" ($piNoTok.body.code -eq 1) "code=$($piNoTok.body.code) raw=$($piNoTok.raw)"
$pkeys = $piNoTok.body.data.PSObject.Properties.Name
foreach ($k in @('lists', 'count', 'page_no', 'page_size', 'extend')) {
    Check "purchase data key '$k'" ($pkeys -contains $k) "keys=$($pkeys -join ',')"
}
Check "purchase has NO camelCase leak (pageNo/pageSize)" `
      (($pkeys -notcontains 'pageNo') -and ($pkeys -notcontains 'pageSize')) "keys=$($pkeys -join ',')"
# outside trading hours the controller returns an empty list with a HARDCODED page_size=10
if ($piNoTok.body.data.count -eq 0) {
    Check "closed-trading empty list uses hardcoded page_size=10" ($piNoTok.body.data.page_size -eq 10) "page_size=$($piNoTok.body.data.page_size)"
    Check "closed-trading lists is an ARRAY" ($piNoTok.body.data.lists -is [array]) "type=$($piNoTok.body.data.lists.GetType().Name)"
    Check "closed-trading extend is an ARRAY" ($piNoTok.body.data.extend -is [array]) "type=$($piNoTok.body.data.extend.GetType().Name)"
}

# SaleController::$notNeedLogin = ['categories'] -> categories is PUBLIC, index is NOT
$scNoTok = Get-Json "$Base/v1/market/sales/categories" @{}
Check "sales/categories is PUBLIC" ($scNoTok.body.code -eq 1) "code=$($scNoTok.body.code) raw=$($scNoTok.raw)"
Check "sales/categories data is a top-level ARRAY" ($scNoTok.body.data -is [array]) "type=$($scNoTok.body.data.GetType().Name)"
if ($scNoTok.body.data -is [array] -and $scNoTok.body.data.Count -ge 1) {
    $c0 = $scNoTok.body.data[0]
    Check "sales/categories item has key 'key'" ($c0.PSObject.Properties.Name -contains 'key') "raw=$($scNoTok.raw)"
    Check "sales/categories item has key 'title'" ($c0.PSObject.Properties.Name -contains 'title') "raw=$($scNoTok.raw)"
    Check "sales/categories key is 'all'" ($c0.key -eq 'all') "key=$($c0.key)"
}

$siNoTok = Get-Json "$Base/v1/market/sales" @{}
Check "market/sales requires login" ($siNoTok.body.code -eq -403) "code=$($siNoTok.body.code)"

# ---------------------------------------------------------------- real token
$mysql = Get-ChildItem 'D:\work\phpstudy_pro\Extensions' -Recurse -Filter mysql.exe -EA SilentlyContinue |
         Select-Object -First 1 -ExpandProperty FullName
$envPath = Join-Path $XTravelRoot '.env'
if (-not ($mysql -and (Test-Path $envPath))) {
    Write-Host "  SKIP  valid-token checks (mysql client or .env not found)" -ForegroundColor Yellow
    Write-Host ""
    Write-Host ("result: {0} passed, {1} failed" -f $script:pass, $script:fail)
    if ($script:fail -gt 0) { exit 1 }
    exit 0
}

$cfg = @{}
$sec = ''
foreach ($raw in Get-Content $envPath) {
    $l = $raw.Trim()
    if ($l -match '^\[(.+)\]$') { $sec = $Matches[1].ToLower(); continue }
    if ($l -match '^([A-Za-z0-9_]+)\s*=\s*(.*)$' -and $l -notmatch '^#') {
        $cfg["$sec.$($Matches[1].ToLower())"] = $Matches[2].Trim().Trim('"')
    }
}
$env:MYSQL_PWD = $cfg['database.password']
$tok = (& $mysql -h $cfg['database.hostname'] -P $cfg['database.hostport'] -u $cfg['database.username'] `
        --default-character-set=utf8mb4 -D $cfg['database.database'] --batch --skip-column-names `
        -e "SELECT s.token FROM x_user_session s JOIN x_user u ON u.id=s.user_id WHERE s.expire_time > UNIX_TIMESTAMP() AND u.delete_time IS NULL ORDER BY s.expire_time DESC LIMIT 1;" 2>&1 |
        Select-Object -First 1)
$env:MYSQL_PWD = $null
$tok = "$tok".Trim()

if ($tok.Length -ne 32) {
    Write-Host "  SKIP  valid-token checks (no live session found in db)" -ForegroundColor Yellow
} else {
    $hdr = @{ token = $tok }

    # trade/config needs a token (see the note where it is checked without one)
    $tc = Get-Json "$Base/v1/common/trade/config" $hdr
    Check "trade/config code=1 (with token)" ($tc.body.code -eq 1) "code=$($tc.body.code)"
    Check "trade/config key 'limitTime' camelCase" ($tc.body.data.PSObject.Properties.Name -contains 'limitTime') $tc.raw

    # ---------------------------------------------------- query params take effect
    #
    # These assertions exist because of a real bug: list endpoints ignored the
    # query string entirely (ctx.(khttp.Context) never matched once a middleware
    # had wrapped the context), so page_size was always the default 25.
    #
    # The old assertions all passed while that bug was live, because they only
    # checked "key exists" / "type is right" -- never "the value I passed in
    # actually changed anything". Always assert the INPUT round-trips.
    #
    # /v1/market/sales is the carrier: it echoes page_no/page_size and needs no
    # rows to exist (count is 0 in this database).
    $pg = Get-Json "$Base/v1/market/sales?page_no=3&page_size=7" $hdr
    Check "sales echoes page_size=7 (params are not ignored)" ($pg.body.data.page_size -eq 7) `
          "page_size=$($pg.body.data.page_size) (25 == default, means params were dropped)"
    Check "sales echoes page_no=3" ($pg.body.data.page_no -eq 3) "page_no=$($pg.body.data.page_no)"

    # page_size must be a NUMBER, not a string
    Check "sales page_size is a number" ($pg.body.data.page_size -is [int] -or $pg.body.data.page_size -is [long]) `
          "type=$($pg.body.data.page_size.GetType().Name)"

    # and a different page_size must give a different echo (guards against a
    # hardcoded value looking "correct" by coincidence)
    $pg2 = Get-Json "$Base/v1/market/sales?page_size=1" $hdr
    Check "sales page_size=1 differs from page_size=7" ($pg2.body.data.page_size -eq 1) `
          "page_size=$($pg2.body.data.page_size)"

    # page_type=0 means "not paginated"; the original forces page_size to a huge
    # value. Assert it does NOT come back as the default 25.
    $pg3 = Get-Json "$Base/v1/market/sales?page_type=0" $hdr
    Check "sales page_type=0 is not treated as absent" ($pg3.body.data.page_size -ne 25) `
          "page_size=$($pg3.body.data.page_size)"

    # ------------------------------------------- 其余已迁接口也要有断言
    #
    # 之前这 6 条接口迁完并验证过，但**一条冒烟断言都没有** ——
    # 它们坏了不会有任何东西发现。这里补上。
    #
    # 断言优先挑**与数据无关**的部分（错误分支、字段类型、键集合），
    # 只有确实需要真实数据的才用已知存在的 id，并注明来源，
    # 避免测试因为数据变化而碎掉。

    # --- purchase/showTotalAmount：纯计算，不需要任何数据 ---
    $ta = Get-Json "$Base/v1/market/purchase/showTotalAmount?app_id=1&amount=1&unit_price=100" $hdr
    Check "showTotalAmount code=1" ($ta.body.code -eq 1) "code=$($ta.body.code) raw=$($ta.raw)"
    # 已用真机 PHP 对照过：feeRate=outFee(6.66)*0.01=0.0666 时 (1,100) -> "93.34"
    Check "showTotalAmount(1,100) = 93.34 (matches real PHP)" ($ta.body.data.totalAmount -eq "93.34") `
          "totalAmount=$($ta.body.data.totalAmount)"
    # ⚠️ totalAmount 是**字符串**不是数字（原实现返回 bcsub 的结果）
    Check "showTotalAmount totalAmount is a STRING" ($ta.body.data.totalAmount -is [string]) `
          "type=$($ta.body.data.totalAmount.GetType().Name)"
    # 参数不合法 -> 字符串 "0"（没有小数位），不是 "0.00"
    $ta0 = Get-Json "$Base/v1/market/purchase/showTotalAmount?app_id=1&amount=0&unit_price=100" $hdr
    Check "showTotalAmount(0,...) = '0' not '0.00'" ($ta0.body.data.totalAmount -eq "0") `
          "totalAmount=$($ta0.body.data.totalAmount)"

    # --- stock/lookall：不需要特定数据 ---
    $sl = Get-Json "$Base/v1/market/stock/lookall" $hdr
    Check "stock/lookall code=1" ($sl.body.code -eq 1) "code=$($sl.body.code)"
    $slk = $sl.body.data.PSObject.Properties.Name
    Check "stock/lookall has keys state+num" (($slk -contains 'state') -and ($slk -contains 'num')) `
          "keys=$($slk -join ',')"
    Check "stock/lookall num is a NUMBER" ($sl.body.data.num -is [int] -or $sl.body.data.num -is [long]) `
          "type=$($sl.body.data.num.GetType().Name)"

    # --- purchase/info：错误分支与数据无关 ---
    $piNoId = Get-Json "$Base/v1/market/purchase/info" $hdr
    Check "purchase/info missing id -> msg 档案ID不能为空" ($piNoId.body.msg -eq '档案ID不能为空') `
          "msg=$($piNoId.body.msg)"
    $piBad = Get-Json "$Base/v1/market/purchase/info?id=99999999" $hdr
    Check "purchase/info unknown id -> msg 未找到档案信息" ($piBad.body.msg -eq '未找到档案信息') `
          "msg=$($piBad.body.msg)"

    # --- sales/info：两条错误信息必须可区分（原实现分两步的意义） ---
    $siBad = Get-Json "$Base/v1/market/sales/info?id=999999999" $hdr
    Check "sales/info unknown id -> msg 未找到艺术品信息" ($siBad.body.msg -eq '未找到艺术品信息') `
          "msg=$($siBad.body.msg)"

    # --- purchase/out 与 purchase/on：同一个 id 应给出不同 count（证明 state 分支独立生效） ---
    $lpid = '178056289901818'
    $po = Get-Json "$Base/v1/market/purchase/out?id=$lpid" $hdr
    $pn = Get-Json "$Base/v1/market/purchase/on?id=$lpid" $hdr
    Check "purchase/out id 不存在 -> msg 记录不存在" `
          ((Get-Json "$Base/v1/market/purchase/out?id=99999999" $hdr).body.msg -eq '记录不存在') ""
    Check "purchase/out code=1 (list id $lpid)" ($po.body.code -eq 1) "code=$($po.body.code) raw=$($po.raw)"
    Check "purchase/on  code=1 (list id $lpid)" ($pn.body.code -eq 1) "code=$($pn.body.code) raw=$($pn.raw)"
    # 这两个接口必须各自有 count，且可以不同 —— 相同也不报错，但至少形状要对
    # 注意：PowerShell 对跨行的 -and 表达式解析很挑剔（会报 Missing closing ')'），
    # 所以先把键列表取到变量里，再写成单行判断。
    $poKeys = $po.body.data.PSObject.Properties.Name
    $poKeysOk = ($poKeys -contains 'count') -and ($poKeys -contains 'lists') -and ($poKeys -contains 'page_size')
    Check "purchase/out has count+lists+page_size" $poKeysOk "keys=$($poKeys -join ',')"
    # purchase/out 的行里 available_amount 必须是**字符串**（bcsub 的结果，不是数字）
    if ($po.body.data.lists.Count -gt 0) {
        $row = $po.body.data.lists[0]
        $aa = $row.available_amount
        Check "purchase/out available_amount is a STRING" ($aa -is [string]) "type=$($aa.GetType().Name) val=$aa"
        $rowKeys = $row.PSObject.Properties.Name
        $noLeak = ($rowKeys -notcontains 'receiveAmount') -and ($rowKeys -notcontains 'archiveId')
        Check "purchase/out row has no camelCase leak" $noLeak "keys=$($rowKeys -join ',')"
    }

    # ------------------------------------------------------------ user/info
    $me = Get-Json "$Base/v1/user/info" $hdr
    Check "valid token accepted" ($me.body.code -eq 1) "code=$($me.body.code)"
    if ($me.body.code -eq 1) {
        $d = $me.body.data
        $keys = $d.PSObject.Properties.Name

        # keys must be snake_case here: the original json_encodes $user->toArray(),
        # so keys are raw DB column names. protojson would emit camelCase unless
        # every field carries an explicit json_name in the .proto.
        foreach ($want in @('real_name', 'create_time', 'user_money', 'has_password',
                            'has_opt_pwd', 'is_real', 'user_code', 'user_code_auth')) {
            Check "user/info key '$want' snake_case" ($keys -contains $want) "keys=$($keys -join ',')"
        }
        Check "user/info has NO camelCase leak" `
              (($keys -notcontains 'realName') -and ($keys -notcontains 'userMoney')) "keys=$($keys -join ',')"

        # proto3 JSON maps int64/uint64 to STRINGS (to dodge JS 2^53 loss),
        # but PHP's json_encode emits numbers. httpx.unquoteInt64 converts back.
        Check "id is a NUMBER (not a proto3 int64 string)" ($d.id -isnot [string]) "type=$($d.id.GetType().Name) val=$($d.id)"
        Check "sn is a NUMBER" ($d.sn -isnot [string]) "type=$($d.sn.GetType().Name)"
        Check "create_time is a NUMBER" ($d.create_time -isnot [string]) "type=$($d.create_time.GetType().Name)"

        # decimal(10,2) -- PHP emits the STRING "0.00", verified against real PHP
        Check "user_money is a STRING (PHP decimal output)" ($d.user_money -is [string]) "type=$($d.user_money.GetType().Name)"

        Check "has_password is boolean" ($d.has_password -is [bool]) "type=$($d.has_password.GetType().Name)"
        Check "user_code_auth nested key tea_user_code" ($d.user_code_auth.PSObject.Properties.Name -contains 'tea_user_code') $me.raw
        Check "password field NOT leaked" ($keys -notcontains 'password') $me.raw
    }

    # ------------------------------------------------------------ market
    $pw = Get-Json "$Base/v1/market/pay_way" $hdr
    Check "pay_way code=1" ($pw.body.code -eq 1) "code=$($pw.body.code)"
    # `data` must be a top-level ARRAY, not {"items":[...]}
    Check "pay_way data is an ARRAY (not an object)" ($pw.body.data -is [array]) "type=$($pw.body.data.GetType().Name) raw=$($pw.raw)"
    if (($pw.body.data -is [array]) -and ($pw.body.data.Count -ge 2)) {
        $fkeys = $pw.body.data[0].PSObject.Properties.Name
        Check "pay_way item key 'pay_way' snake_case" ($fkeys -contains 'pay_way') "keys=$($fkeys -join ',')"
        Check "pay_way item key 'pay_name' snake_case" ($fkeys -contains 'pay_name') "keys=$($fkeys -join ',')"
        Check "pay_way value is 1" ($pw.body.data[0].pay_way -eq 1) "value=$($pw.body.data[0].pay_way)"
        Check "pay_name is the tea exchange name" ($pw.body.data[0].pay_name -eq $wantTeaJiao) "value=$($pw.body.data[0].pay_name)"
    } else {
        Check "pay_way returned 2 items" $false "raw=$($pw.raw)"
    }

    $ce = Get-Json "$Base/v1/market/check/exchange" $hdr
    Check "check/exchange responds with an envelope" (Has-Envelope $ce.body) $ce.raw
    Check "check/exchange keeps HTTP 200" ($ce.status -eq 200) "HTTP $($ce.status)"
    # two legal outcomes: code=1 (allowed) or code=0 + show=1 (blocked, msg shown to user)
    Check "check/exchange is ok(1) or biz-fail(0,show=1,msg non-empty)" `
          (($ce.body.code -eq 1) -or (($ce.body.code -eq 0) -and ($ce.body.show -eq 1) -and ($ce.body.msg -ne ''))) `
          "code=$($ce.body.code) show=$($ce.body.show) msg='$($ce.body.msg)'"
    if ($ce.body.code -eq 1) {
        Check "check/exchange success data is an empty ARRAY" ($ce.body.data -is [array]) "type=$($ce.body.data.GetType().Name)"
    }
}

Write-Host ""
Write-Host ("result: {0} passed, {1} failed" -f $script:pass, $script:fail)
if ($script:fail -gt 0) { exit 1 }
