# Kratos code generation (Windows). Equivalent to the "api" / "config" Makefile
# targets, rewritten in PowerShell so there is no make / git-bash dependency.
#
# NOTE: messages are intentionally ASCII-only. Windows PowerShell 5.1 reads
# .ps1 files as ANSI and mangles non-ASCII literals; run this with pwsh 7 if you
# want the Chinese comments in other files to render.
#
# Usage:
#   pwsh -File .\scripts\gen.ps1          # api + conf
#   pwsh -File .\scripts\gen.ps1 -Api     # api only
#   pwsh -File .\scripts\gen.ps1 -Conf    # conf only

param(
    [switch]$Api,
    [switch]$Conf
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not $Api -and -not $Conf) { $Api = $true; $Conf = $true }

# protoc
$protoc = 'D:\work\tool\protoc-35.1-win64\bin\protoc.exe'
if (-not (Test-Path $protoc)) {
    $c = Get-Command protoc -ErrorAction SilentlyContinue
    if (-not $c) { throw "protoc not found" }
    $protoc = $c.Source
}

# The three plugins must be resolvable. Do NOT pass --plugin=<abs path>:
# on Windows protoc fails to exec them ("filename, directory name, or volume
# label syntax is incorrect"). Relying on PATH works.
foreach ($p in 'protoc-gen-go', 'protoc-gen-go-http', 'protoc-gen-go-grpc') {
    if (-not (Get-Command $p -ErrorAction SilentlyContinue)) {
        throw "$p not found in PATH. Install with: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest ; go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest ; go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
    }
}

# well-known types ship with protoc; google/api comes from third_party
$wkt = Join-Path (Split-Path -Parent (Split-Path -Parent $protoc)) 'include'
$inc = @('-I.', '-I./api', '-I./third_party')
if (Test-Path $wkt) { $inc += "-I$wkt" }

function Invoke-Protoc([string[]]$targets) {
    $argv = $inc + @(
        '--go_out=paths=source_relative:.',
        '--go-http_out=paths=source_relative:.',
        '--go-grpc_out=paths=source_relative:.'
    ) + $targets
    Write-Host "==> protoc $($targets -join ' ')"
    & $protoc @argv
    if ($LASTEXITCODE -ne 0) { throw "protoc failed with exit code $LASTEXITCODE" }
}

if ($Api) {
    $files = Get-ChildItem -Path 'api' -Recurse -Filter *.proto | ForEach-Object {
        $_.FullName.Substring($root.Length + 1).Replace('\', '/')
    }
    if ($files) { Invoke-Protoc $files } else { Write-Host "no .proto under api/" }
}

if ($Conf) {
    Invoke-Protoc @('internal/conf/conf.proto')
}

Write-Host ""
Write-Host "Done. Generated:" -ForegroundColor Green
Get-ChildItem -Path 'api', 'internal/conf' -Recurse -Include *.pb.go | ForEach-Object {
    "  " + $_.FullName.Substring($root.Length + 1)
}
