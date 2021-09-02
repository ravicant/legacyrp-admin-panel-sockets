@echo off

echo Building linux...
set GOOS=linux
go build -o admin-panel-sockets

echo Building windows...
set GOOS=windows
go build -o admin-panel-sockets.exe