param(
    [Parameter(Mandatory=$true)][string]$ServerUrl,
    [string]$ConfigPath = ""
)
$ErrorActionPreference = "Stop"
if (-not $ConfigPath) {
    $ConfigPath = Join-Path $PSScriptRoot "commerce.json"
    if (-not (Test-Path -LiteralPath $ConfigPath -PathType Leaf)) {
        $ConfigPath = Join-Path $PSScriptRoot "..\assets\commerce.json"
    }
}
$url = $null
if (-not [Uri]::TryCreate($ServerUrl.Trim(), [UriKind]::Absolute, [ref]$url) -or $url.Scheme -ne "https" -or -not $url.Host -or $url.UserInfo -or $url.Query -or $url.Fragment -or $url.AbsolutePath -ne "/") {
    throw "Use a full HTTPS backend origin, without a path, credentials, query or fragment."
}
if (-not (Test-Path -LiteralPath $ConfigPath -PathType Leaf)) { throw "commerce.json was not found: $ConfigPath" }
$path = (Resolve-Path -LiteralPath $ConfigPath).Path
$cfg = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json
if ($null -eq $cfg -or $cfg -is [Array] -or $cfg -is [string] -or $cfg -is [ValueType]) { throw "commerce.json must contain a JSON object." }
$origin = $url.GetLeftPart([UriPartial]::Authority)
$cfg | Add-Member -NotePropertyName server_url -NotePropertyValue $origin -Force
$temp = Join-Path (Split-Path -Parent $path) ("commerce-" + [Guid]::NewGuid().ToString("N") + ".tmp")
$backup = Join-Path (Split-Path -Parent $path) ("commerce-backup-" + [Guid]::NewGuid().ToString("N") + ".bak")
$replaced = $false
try {
    [IO.File]::WriteAllText($temp, ($cfg | ConvertTo-Json -Depth 10) + "`n", [Text.UTF8Encoding]::new($false))
    [IO.File]::Replace($temp, $path, $backup)
    $replaced = $true
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force }
    if ($replaced -and (Test-Path -LiteralPath $backup)) { Remove-Item -LiteralPath $backup -Force }
}
Write-Output "Client backend configured: $origin"
Write-Output "Restart an existing client; rebuild before distributing the source configuration."
