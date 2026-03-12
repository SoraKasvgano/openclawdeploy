@echo off
setlocal enabledelayedexpansion

if not exist dist mkdir dist

call :build linux arm 7 openclaw-server-linux-armv7
call :build linux arm64 - openclaw-server-linux-arm64
call :build darwin amd64 - openclaw-server-darwin-amd64
call :build darwin arm64 - openclaw-server-darwin-arm64
goto :eof

:build
set GOOS=%1
set GOARCH=%2
set GOARM=%3
set OUTPUT=%4

echo Building %OUTPUT% ...
set CGO_ENABLED=0
if "%GOARM%"=="-" (
  set GOARM=
) else (
  set GOARM=%GOARM%
)

go build -trimpath -o dist\%OUTPUT% .
if errorlevel 1 exit /b 1
exit /b 0
