@echo off
set "PATH=C:\msys64\ucrt64\bin;%PATH%"
go build -v -o pm.exe .
if %ERRORLEVEL% NEQ 0 (
    echo BUILD FAILED with code %ERRORLEVEL%
    exit /b %ERRORLEVEL%
)
echo BUILD SUCCESSFUL
