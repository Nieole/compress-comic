@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion

:: 设置编译时环境变量
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

echo [32m开始构建...[0m

:: 构建命令
go build -ldflags="-s -w" -trimpath -o compress-comic.exe

if !ERRORLEVEL! EQU 0 (
    echo [32m构建成功！[0m
    echo 输出文件: compress-comic.exe
) else (
    echo [31m构建失败！[0m
)

endlocal 