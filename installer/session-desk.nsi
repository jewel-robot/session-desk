; Session Desk per-user installer (no administrator rights required).
; Built by build.ps1:  makensis /INPUTCHARSET UTF8 /DVERSION=x.y.z /DVERSION4=x.y.z.0 /DBIN=<bin dir> /DOUTFILE=<setup.exe> session-desk.nsi
Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef VERSION4
  !define VERSION4 "${VERSION}.0"
!endif
!ifndef BIN
  !define BIN "..\bin"
!endif
!ifndef OUTFILE
  !define OUTFILE "${BIN}\session-desk-${VERSION}-setup.exe"
!endif

!define APPNAME "Session Desk"
!define PUBLISHER "jewel-robot"
!ifndef WEBSITE
  !define WEBSITE ""
!endif
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\SessionDesk"
!define EXE "session-desk.exe"
; Microsoft Edge WebView2 Runtime (Evergreen): client GUID and the official bootstrapper link.
!define WV2_GUID "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
!define WV2_URL "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

Name "${APPNAME}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\Session Desk"
InstallDirRegKey HKCU "${UNINSTKEY}" "InstallLocation"
BrandingText "${APPNAME} ${VERSION}"

VIProductVersion "${VERSION4}"
VIFileVersion "${VERSION4}"
VIAddVersionKey /LANG=0 "ProductName" "${APPNAME}"
VIAddVersionKey /LANG=0 "CompanyName" "${PUBLISHER}"
VIAddVersionKey /LANG=0 "FileDescription" "${APPNAME} 安装程序"
VIAddVersionKey /LANG=0 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "LegalCopyright" "Copyright (c) 2026 ${PUBLISHER}"

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
!include "Sections.nsh"
!include "StrFunc.nsh"
${StrLoc}
${UnStrLoc}

!ifdef SIGNTOOL
  ; Sign the uninstaller while it is written (NSIS 3.08+).
  !uninstfinalize '"${SIGNTOOL}" sign /fd sha256 /tr ${SIGN_TSA} /td sha256 /sha1 ${SIGN_SHA1} "%1"' = 0
!endif

!define MUI_ICON "..\build\windows\icon.ico"
!define MUI_UNICON "..\build\windows\icon.ico"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "启动 ${APPNAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Section "${APPNAME}（必需）" SecMain
  SectionIn RO
  Call EnsureWebView2
  Call CloseRunning
  SetOutPath "$INSTDIR"
  SetOverwrite on
  File "${BIN}\${EXE}"
  File "${BIN}\session-desk-worker"
  File "${BIN}\LICENSE.txt"
  File "${BIN}\THIRD_PARTY_NOTICES.md"
  SetOutPath "$INSTDIR\licenses"
  File /r "${BIN}\licenses\*.*"
  SetOutPath "$INSTDIR"

  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortCut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0

  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  !if "${WEBSITE}" != ""
    WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "${WEBSITE}"
  !endif
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\${EXE},0"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"
SectionEnd

Section /o "桌面快捷方式" SecDesktop
  CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
SectionEnd

; An upgrade that unticks the desktop shortcut removes the old one.
Section "-DesktopCleanup"
  ${IfNot} ${SectionIsSelected} ${SecDesktop}
    Delete "$DESKTOP\${APPNAME}.lnk"
  ${EndIf}
SectionEnd

; Upgrades: the previous install location comes from the registry (InstallDirRegKey), an existing desktop
; shortcut stays selected, and a running Session Desk is closed before files are replaced.
Function CloseRunning
  nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${EXE}" /NH'
  Pop $0
  Pop $1
  ${StrLoc} $2 "$1" "${EXE}" ">"
  ${If} $2 == ""
    Return
  ${EndIf}
  ${IfNot} ${Silent}
    MessageBox MB_OKCANCEL|MB_ICONINFORMATION "${APPNAME} 正在运行，将先关闭它再继续。" IDOK +2
    Abort
  ${EndIf}
  ; Ask politely first (WM_CLOSE), then force after a short wait.
  nsExec::Exec '"$SYSDIR\taskkill.exe" /IM ${EXE}'
  Sleep 2000
  nsExec::Exec '"$SYSDIR\taskkill.exe" /F /IM ${EXE}'
  Sleep 500
FunctionEnd

; $R0 = 1 when a WebView2 Runtime is registered per machine (64- or 32-bit view) or per user.
Function HasWebView2
  StrCpy $R0 0
  ReadRegStr $R1 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\${WV2_GUID}" "pv"
  ${If} $R1 == ""
  ${OrIf} $R1 == "0.0.0.0"
    ReadRegStr $R1 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\${WV2_GUID}" "pv"
  ${EndIf}
  ${If} $R1 == ""
  ${OrIf} $R1 == "0.0.0.0"
    ReadRegStr $R1 HKCU "Software\Microsoft\EdgeUpdate\Clients\${WV2_GUID}" "pv"
  ${EndIf}
  ${If} $R1 != ""
  ${AndIf} $R1 != "0.0.0.0"
    StrCpy $R0 1
  ${EndIf}
FunctionEnd

; Without WebView2 the window cannot open. Offer Microsoft's bootstrapper; silent installs fetch it without asking
; and carry on if that fails.
Function EnsureWebView2
  Call HasWebView2
  ${If} $R0 == 1
    Return
  ${EndIf}
  ${IfNot} ${Silent}
    MessageBox MB_YESNO|MB_ICONQUESTION "${APPNAME} 需要 Microsoft Edge WebView2 Runtime。现在下载并安装？" IDYES +2
    Abort
  ${EndIf}
  InitPluginsDir
  StrCpy $R2 "$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe"
  DetailPrint "下载 WebView2 Runtime"
  nsExec::ExecToLog '"$SYSDIR\curl.exe" -fsSL --retry 2 -o "$R2" "${WV2_URL}"'
  Pop $R3
  ${If} $R3 != 0
    nsExec::ExecToLog `"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -Command "Invoke-WebRequest -UseBasicParsing -Uri '${WV2_URL}' -OutFile '$R2'"`
    Pop $R3
  ${EndIf}
  ${If} $R3 == 0
    DetailPrint "安装 WebView2 Runtime"
    ExecWait '"$R2" /silent /install' $R3
  ${EndIf}
  Call HasWebView2
  ${If} $R0 != 1
  ${AndIfNot} ${Silent}
    MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "WebView2 Runtime 未能安装，${APPNAME} 可能无法启动。继续安装？" IDOK +2
    Abort
  ${EndIf}
FunctionEnd

Function un.CloseRunning
  nsExec::ExecToStack '"$SYSDIR\tasklist.exe" /FI "IMAGENAME eq ${EXE}" /NH'
  Pop $0
  Pop $1
  ${UnStrLoc} $2 "$1" "${EXE}" ">"
  ${If} $2 == ""
    Return
  ${EndIf}
  ${IfNot} ${Silent}
    MessageBox MB_OKCANCEL|MB_ICONINFORMATION "${APPNAME} 正在运行，将先关闭它再继续。" IDOK +2
    Abort
  ${EndIf}
  ; Ask politely first (WM_CLOSE), then force after a short wait.
  nsExec::Exec '"$SYSDIR\taskkill.exe" /IM ${EXE}'
  Sleep 2000
  nsExec::Exec '"$SYSDIR\taskkill.exe" /F /IM ${EXE}'
  Sleep 500
FunctionEnd

Function .onInit
  SetShellVarContext current
  ${If} ${FileExists} "$DESKTOP\${APPNAME}.lnk"
    !insertmacro SelectSection ${SecDesktop}
  ${EndIf}
FunctionEnd

; Silent upgrades started by the app pass /RELAUNCH to start it again when done.
Function .onInstSuccess
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/RELAUNCH" $1
  ${IfNot} ${Errors}
    Exec '"$INSTDIR\${EXE}"'
  ${EndIf}
FunctionEnd

Function un.onInit
  SetShellVarContext current
FunctionEnd

; Removes only the files this installer created. Settings and caches in %APPDATA%\SessionDesk are kept.
Section "Uninstall"
  Call un.CloseRunning
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\session-desk-worker"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  RMDir /r "$INSTDIR\licenses"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APPNAME}.lnk"
  Delete "$DESKTOP\${APPNAME}.lnk"
  DeleteRegKey HKCU "${UNINSTKEY}"
SectionEnd
