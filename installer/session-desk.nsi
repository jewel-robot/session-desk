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
