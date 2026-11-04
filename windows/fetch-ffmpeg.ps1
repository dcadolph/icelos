# fetch-ffmpeg.ps1 downloads the BtbN FFmpeg build pinned in ffmpeg-source.txt, refuses it unless
# its SHA-256 matches, and copies ffmpeg.exe, ffprobe.exe, and the build's LICENSE.txt into
# -Dest, printing each path. The pin is a month-end autobuild, which BtbN keeps for about two
# years, where daily builds are deleted after about two weeks.
param(
    [Parameter(Mandatory = $true)][string]$Dest
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$lines = @(Get-Content -LiteralPath (Join-Path $PSScriptRoot 'ffmpeg-source.txt') | Where-Object { $_.Trim() })
if ($lines.Count -ne 3) {
    [Console]::Error.WriteLine("ffmpeg-source.txt: want the tag, asset, and SHA-256 lines, got $($lines.Count) lines")
    exit 1
}
$tag, $asset, $want = $lines | ForEach-Object { $_.Trim() }
$url = "https://github.com/BtbN/FFmpeg-Builds/releases/download/$tag/$asset"

$work = Join-Path ([System.IO.Path]::GetTempPath()) ("icelos-ffmpeg-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    $zip = Join-Path $work $asset
    [Console]::Error.WriteLine("Downloading $url")
    # The progress bar slows Invoke-WebRequest down by an order of magnitude.
    $ProgressPreference = 'SilentlyContinue'
    Invoke-WebRequest -Uri $url -OutFile $zip
    $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant()
    if ($got -ne $want.ToLowerInvariant()) {
        [Console]::Error.WriteLine("SHA-256 mismatch for ${asset}: got $got, want $want")
        exit 1
    }
    Expand-Archive -LiteralPath $zip -DestinationPath $work
    $root = Join-Path $work ([System.IO.Path]::GetFileNameWithoutExtension($asset))
    New-Item -ItemType Directory -Force -Path $Dest | Out-Null
    foreach ($rel in 'bin/ffmpeg.exe', 'bin/ffprobe.exe', 'LICENSE.txt') {
        $to = Join-Path $Dest (Split-Path $rel -Leaf)
        Copy-Item -LiteralPath (Join-Path $root $rel) -Destination $to -Force
        Write-Output (Resolve-Path -LiteralPath $to).Path
    }
} finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
