# Build the RemoteMouse Windows server -> rmserver.exe
# Usage: powershell -ExecutionPolicy Bypass -File build.ps1
$ErrorActionPreference = "Stop"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# Embed application manifest (Common Controls v6 + DPI) and the app icon.
# rsrc.syso is auto-linked by `go build` when present in the package dir.
if (-not (Test-Path rsrc.syso)) {
    Write-Host "generating rsrc.syso (manifest + icon)..."
    go run github.com/akavel/rsrc@latest -manifest assets\manifest.xml -ico assets\tray.ico -o rsrc.syso
}

go build -ldflags "-H windowsgui" -o rmserver.exe .
Write-Host "built rmserver.exe"
Write-Host "run:  .\rmserver.exe -pass 1234            # window + tray"
Write-Host "      .\rmserver.exe -pass 1234 -notray    # console"
