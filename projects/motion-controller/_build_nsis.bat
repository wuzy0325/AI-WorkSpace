@echo off
cd /d "c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\motion-controller\apps\desktop-wails\build\windows\installer"
"C:\Program Files (x86)\NSIS\makensis.exe" /DARG_WAILS_AMD64_BINARY=..\..\bin\motion-controller.exe project.nsi
