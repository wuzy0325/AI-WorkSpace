Unicode true

; 项目显示名与公司名：从改名的 daq-p1604 统一为 wispa。
; 这些常量在 include wails_tools.nsh 之前定义，利用其 !ifndef 语义覆盖
; wails build 自动生成头文件里的旧项目名，保证安装包文件名、安装目录、
; 快捷方式和卸载显示名都与当前项目名一致。
!define INFO_PROJECTNAME    "wispa"
!define INFO_COMPANYNAME    "WindTuner Team"
!define INFO_PRODUCTNAME    "wispa"
; Project version. Keep in sync with VERSION.
!define INFO_PRODUCTVERSION "0.7.6"

!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"    "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

# Launch app after install (checked by default)
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_CHECKED
!define MUI_FINISHPAGE_RUN_TEXT "$(INST_RUN_TEXT)"

# Language selection dialog (zh/en)
!define MUI_LANGDLL_ALLLANGUAGES
!define MUI_LANGDLL_INFO "Please select the installation language:$\n请选择安装程序语言："

# Custom welcome/directory/finish text via LangString
!define MUI_WELCOMEPAGE_TITLE "$(INST_WELCOME_TITLE)"
!define MUI_WELCOMEPAGE_TEXT "$(INST_WELCOME_TEXT)"
!define MUI_DIRECTORYPAGE_TEXT_TOP "$(INST_DIRECTORY_TEXT)"
!define MUI_FINISHPAGE_TITLE "$(INST_FINISH_TITLE)"
!define MUI_FINISHPAGE_TEXT "$(INST_FINISH_TEXT)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

# Languages (first = default)
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

# ------ Installer UI Strings ------
# Chinese
LangString INST_WELCOME_TITLE  ${LANG_SIMPCHINESE} "欢迎使用 wispa 压力扫描系统"
LangString INST_WELCOME_TEXT   ${LANG_SIMPCHINESE} "wispa 用于 DAQ-P-1604 压力扫描采集与监控。$\r$\n$\r$\n主要功能：$\r$\n  * 设备发现与连接管理（TCP / UDP）$\r$\n  * 16 通道压力采集（含大气压力与温度）$\r$\n  * 实时数据显示与趋势监控$\r$\n  * CSV 数据录制与回放$\r$\n$\r$\n安装程序将引导您完成安装。"
LangString INST_DIRECTORY_TEXT ${LANG_SIMPCHINESE} "选择安装目录。$\r$\n$\r$\n建议安装在非系统盘，并确保有足够的磁盘空间用于数据存储。"
LangString INST_FINISH_TITLE   ${LANG_SIMPCHINESE} "安装完成"
LangString INST_FINISH_TEXT    ${LANG_SIMPCHINESE} "wispa 压力扫描系统 已成功安装。$\r$\n$\r$\n点击完成退出安装程序。"
LangString INST_RUN_TEXT       ${LANG_SIMPCHINESE} "启动 wispa"
# English
LangString INST_WELCOME_TITLE  ${LANG_ENGLISH} "Welcome to wispa Pressure Scanning"
LangString INST_WELCOME_TEXT   ${LANG_ENGLISH} "wispa is a data acquisition application for the DAQ-P-1604 pressure scanning device.$\r$\n$\r$\nKey features:$\r$\n  * Device discovery and connection management (TCP / UDP)$\r$\n  * 16-channel pressure acquisition (plus atmospheric pressure and temperature)$\r$\n  * Real-time data display and trend monitoring$\r$\n  * CSV data recording and playback$\r$\n$\r$\nThe wizard will guide you through the installation."
LangString INST_DIRECTORY_TEXT ${LANG_ENGLISH} "Select the installation folder.$\r$\n$\r$\nA non-system drive with sufficient free space for data storage is recommended."
LangString INST_FINISH_TITLE   ${LANG_ENGLISH} "Installation Complete"
LangString INST_FINISH_TEXT    ${LANG_ENGLISH} "wispa Pressure Scanning has been successfully installed.$\r$\n$\r$\nClick Finish to exit the setup wizard."
LangString INST_RUN_TEXT       ${LANG_ENGLISH} "Launch wispa"

!define PRODUCT_UNINST_ROOT_KEY "SHCTX"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${INFO_PRODUCTVERSION}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
    !insertmacro MUI_LANGDLL_DISPLAY
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    ; Skip WebView2 runtime download/install if already present (avoids hang on offline machines)
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        !insertmacro wails.webview2runtime
        ; Re-check: warn the user if download/silent install could not complete (e.g. offline).
        ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
        ${If} $0 == ""
            MessageBox MB_ICONEXCLAMATION "WebView2 Runtime is missing and automatic download/install could not complete (possibly no network). wispa requires WebView2 Runtime to run. Please install it (e.g. copy the offline installer from another PC) and restart the application."
        ${EndIf}
    ${EndIf}

    SetOutPath $INSTDIR

    !insertmacro wails.files

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd