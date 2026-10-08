# End-to-end smoke test against a running instance.
#
# Checks the things that actually break in this migration:
#   - the response envelope is {code,show,msg,data}, not Kratos' default
#   - code=1 means SUCCESS (flipped vs Kratos)
#   - HTTP status stays 200 even for auth failures
#   - auth failures carry code=-403 and show=0 exactly like the PHP side
#   - decimal fields (userMoney) come back as STRINGS, not numbers
#
# ASCII only: Windows PowerShell 5.1 reads .ps1 as ANSI.
#
# Usage:
#   pwsh -File .\scripts\smoke.ps1
#   pwsh -File .\scripts\smoke.ps1 -Base http://127.0.0.1:18000

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

Write-Host "smoke test against $Base"
Write-Host ""

# ---------------------------------------------------------------- liveness
$live = Get-Json "$Base/v1/common/config" @{}
Check "service is up" ($live.status -eq 200) "HTTP $($live.status)"

# ---------------------------------------------------------------- envelope
Check "envelope has code/show/msg/data" (Has-Envelope $live.body) $live.raw
Check "public endpoint succeeds with code=1 (NOT kratos' 0)" ($live.body.code -eq 1) "code=$($live.body.code)"
Check "success msg is empty string" ($live.body.msg -eq '') "msg='$($live.body.msg)'"

# no-zero-value-dropping: protojson omits zero values unless EmitUnpopulated is on,
# and PHP's json_encode always emits them
$data = $live.body.data
Check "tradeSwitch present (not dropped as zero)" ($null -ne $data.tradeSwitch) $live.raw
Check "outFee present" ($null -ne $data.outFee) $live.raw
Check "periods is an array" ($data.tradeSwitch.periods -is [array] -or $null -ne $data.tradeSwitch.periods) $live.raw

$tc = Get-Json "$Base/v1/common/trade/config" @{}
Check "trade/config code=1" ($tc.body.code -eq 1) "code=$($tc.body.code)"
Check "trade/config switch present" ($null -ne $tc.body.data.switch) $tc.raw

# ---------------------------------------------------------------- auth failures
# Expected Chinese messages, built from code points to keep this file ASCII.
# ([char]'a' + [char]'b' does INTEGER addition in PowerShell -- must use -join.)
$wantNoToken = -join @(0x8BF7, 0x6C42, 0x8BA4, 0x8BC1, 0x4FE1, 0x606F, 0x6709, 0x8BEF,
                       0xFF0C, 0x8BF7, 0x91CD, 0x65B0, 0x767B, 0x5F55 | ForEach-Object { [char]$_ })
$wantBadToken = -join @(0x767B, 0x5F55, 0x8D85, 0x65F6, 0xFF0C, 0x8BF7, 0x91CD, 0x65B0,
                        0x767B, 0x5F55 | ForEach-Object { [char]$_ })

$noTok = Get-Json "$Base/v1/user/info" @{}
Check "auth required without token" ($noTok.body.code -eq -403) "code=$($noTok.body.code)"
Check "auth failure keeps HTTP 200" ($noTok.status -eq 200) "HTTP $($noTok.status)"
Check "auth failure show=0 (frontend must not popup)" ($noTok.body.show -eq 0) "show=$($noTok.body.show)"
Check "auth failure msg matches PHP verbatim" ($noTok.body.msg -eq $wantNoToken) "msg='$($noTok.body.msg)'"

$badTok = Get-Json "$Base/v1/user/info" @{ token = 'deadbeefdeadbeefdeadbeefdeadbeef' }
Check "invalid token rejected" ($badTok.body.code -eq -403) "code=$($badTok.body.code)"
Check "invalid token show=0" ($badTok.body.show -eq 0) "show=$($badTok.body.show)"
Check "invalid token msg matches PHP verbatim" ($badTok.body.msg -eq $wantBadToken) "msg='$($badTok.body.msg)'"

# ---------------------------------------------------------------- real token
$mysql = Get-ChildItem 'D:\work\phpstudy_pro\Extensions' -Recurse -Filter mysql.exe -EA SilentlyContinue |
         Select-Object -First 1 -ExpandProperty FullName
$envPath = Join-Path $XTravelRoot '.env'
if ($mysql -and (Test-Path $envPath)) {
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

    if ($tok.Length -eq 32) {
        $me = Get-Json "$Base/v1/user/info" @{ token = $tok }
        Check "valid token accepted" ($me.body.code -eq 1) "code=$($me.body.code)"
        if ($me.body.code -eq 1) {
            Check "user id is a number" ($me.body.data.id -gt 0) "id=$($me.body.data.id)"
            # the important one: decimal(10,2) must stay a string like PHP
            $um = $me.body.data.userMoney
            Check "userMoney is a STRING (matches PHP decimal output)" ($um -is [string]) "type=$($um.GetType().Name) value=$um"
            Check "hasPassword is boolean" ($me.body.data.hasPassword -is [bool]) "type=$($me.body.data.hasPassword.GetType().Name)"
            Check "userCodeAuth present" ($null -ne $me.body.data.userCodeAuth) $me.raw
            Check "password field NOT leaked" ($me.body.data.PSObject.Properties.Name -notcontains 'password') $me.raw
        }
    } else {
        Write-Host "  SKIP  valid-token checks (no live session found in db)" -ForegroundColor Yellow
    }
} else {
    Write-Host "  SKIP  valid-token checks (mysql client or .env not found)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host ("result: {0} passed, {1} failed" -f $script:pass, $script:fail)
if ($script:fail -gt 0) { exit 1 }
