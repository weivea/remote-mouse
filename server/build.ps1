# Build the RemoteMouse Windows server -> rmserver.exe
# Usage: powershell -ExecutionPolicy Bypass -File build.ps1
$ErrorActionPreference = "Stop"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags "-H windowsgui" -o rmserver.exe .
Write-Host "built rmserver.exe"
Write-Host "run:  .\rmserver.exe -pass 1234            # tray icon"
Write-Host "      .\rmserver.exe -pass 1234 -notray    # console"
