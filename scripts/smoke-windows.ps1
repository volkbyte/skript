# Smoke run for skript on Windows.
#
# Demonstrates one exec-skript and one command-skript end to end against a
# throwaway `~\.volkbyte\skript\config.toml` layout, using a fake USERPROFILE.
#
# Usage: scripts\smoke-windows.ps1 [-Skript <path-to-exe>]
# Builds the binary with `go build` when no path is given.

param(
    [string]$Skript = ""
)

$ErrorActionPreference = "Stop"

Set-Location -Path (Join-Path $PSScriptRoot "..")
$Repo = (Get-Location).Path

if ($Skript -eq "") {
    Write-Host "== building skript with go"
    New-Item -ItemType Directory -Force -Path ".smoke" | Out-Null
    go build -o ".smoke\skript.exe" ./cmd/skript
    if ($LASTEXITCODE -ne 0) { exit 1 }
    $Skript = Join-Path $Repo ".smoke\skript.exe"
}

$FakeHome = Join-Path ([System.IO.Path]::GetTempPath()) ("skript-smoke-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $FakeHome -Force | Out-Null
$SmokeDir = Join-Path $Repo ".smoke"

function Finish {
    Remove-Item -Recurse -Force $FakeHome -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $SmokeDir -ErrorAction SilentlyContinue
}

$configDir = Join-Path $FakeHome ".volkbyte\skript"
New-Item -ItemType Directory -Path $configDir -Force | Out-Null

$Root1 = Join-Path $FakeHome "roots\exec-root"
$Root2 = Join-Path $FakeHome "roots\command-root"
New-Item -ItemType Directory -Path $Root1, $Root2 -Force | Out-Null

@"
[skripts]
demo = "$($Root1 -replace '\\', '/')"
"@ | Set-Content -Path (Join-Path $configDir "config.toml") -Encoding ascii

@"
[meta]
version = "1"

[executor]
type = "exec"
executable = "cmd"
args = ["/C", "echo exec-skript: entrypoint=vargs-list"]
entrypoint = "entry.dummy"
"@ | Set-Content -Path (Join-Path $Root1 ".skriptrc") -Encoding ascii

Set-Content -Path (Join-Path $Root1 "entry.dummy") -Value "entrypoint file for the exec skript" -Encoding ascii

@"
[meta]
version = "1"

[executor]
type = "command"
command = "echo command-skript:"
"@ | Set-Content -Path (Join-Path $Root2 ".skriptrc") -Encoding ascii

$fail = $false

function Check {
    param($Name, $Actual, $Expected)
    Write-Host "$Name"
    Write-Host "   output: $Actual"
    if ($Actual -ne $Expected) {
        Write-Host "   !! FAIL: expected: $Expected" -ForegroundColor Red
        $script:fail = $true
        return
    }
    Write-Host "   OK"
}

# The exec executor resolves its entrypoint against SKRIPT_ROOT and reports
# the vargs through echoargs-style argv listing (cmd echoes them line-wise).
Write-Host "=== 1. exec executor: directory target"
$env:USERPROFILE = $FakeHome

# The rc entrypoint arrives at the child resolved against SKRIPT_ROOT.
$entrypointArg = Join-Path $Root1 "entry.dummy"
$out = & $Skript $Root1 A B 2>&1
Check "   exec-skript:" "$out" "exec-skript: entrypoint=vargs-list $entrypointArg A B"

Write-Host "=== 2. exec executor: file target inside the root (walk-up)"
$out = & $Skript (Join-Path $Root1 "entry.dummy") A B 2>&1
Check "   walk-up:" "$out" "exec-skript: entrypoint=vargs-list $entrypointArg A B"

Write-Host "=== 3. alias from ~\.volkbyte\skript\config.toml"
$out = & $Skript demo A B 2>&1
Check "   demo:" "$out" "exec-skript: entrypoint=vargs-list $entrypointArg A B"

Write-Host "=== 4. command executor: string form through cmd /C"
$out = & $Skript $Root2 A B 2>&1
Check "   command-skript:" "$out" "command-skript: A B"

Write-Host "=== 5. metacharacters stay inert (no second command runs)"
$evil = Join-Path $Root2 "evil.txt"
if (Test-Path $evil) { Remove-Item $evil -Force }
# echo is a cmd builtin: it keeps the CRT-quoted varg's quotes visible in its
# output; the important assertions are that the whole varg stays ONE argument
# of `echo` and that `A&B` never split the line into a second command.
$out = & $Skript $Root2 'A&B echo EVIL' 2>&1
$evilExists = Test-Path $evil
if ($out -ne 'command-skript: "A&B echo EVIL"' -or $evilExists) {
    Write-Host "   output: $out  evil.txt-exists: $evilExists" -ForegroundColor Red
    Write-Host "   !! FAIL: vargs must stay inert arguments" -ForegroundColor Red
    $script:fail = $true
} else {
    Write-Host "   output: $out"
    Write-Host "   OK (one literal argument; evil.txt never created)"
}

Finish

if (-not $script:fail) {
    Write-Host ""
    Write-Host "SMOKE OK" -ForegroundColor Green
} else {
    Write-Host ""
    Write-Host "SMOKE FAILED" -ForegroundColor Red
    exit 1
}
