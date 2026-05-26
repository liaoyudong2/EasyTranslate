param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$PackageArgs
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot
go run ./scripts/package.go @PackageArgs
