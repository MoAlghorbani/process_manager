@echo off
title Process Manager (Go Desktop)
cd /d "%~dp0"
if "%~1"=="" (
    pm.exe gui
    if errorlevel 1 (
        echo.
        echo [Process Manager] Application exited with error code %ERRORLEVEL%.
        pause
    )
) else (
    pm.exe %*
)
