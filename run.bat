@echo off
title Process Manager (Rust Edition)
cd /d "%~dp0"
if "%~1"=="" (
    if exist "pm_rs.exe" (
        start "" "pm_rs.exe"
    ) else if exist "rust\pm_rs.exe" (
        start "" "rust\pm_rs.exe"
    ) else (
        echo [ERROR] pm_rs.exe not found in %~dp0 or %~dp0rust\
        pause
    )
) else (
    if exist "pm_rs.exe" (
        "pm_rs.exe" %*
    ) else if exist "rust\pm_rs.exe" (
        "rust\pm_rs.exe" %*
    ) else (
        echo [ERROR] pm_rs.exe not found in %~dp0 or %~dp0rust\
    )
)
