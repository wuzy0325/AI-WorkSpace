Unicode true

####
## Wind-DAQ NSIS installer script (custom version).
## Adds: language selection dialog, custom welcome/finish text,
##       installer language persisted to registry for first-run i18n.
## wails_tools.nsh is the Git-tracked copy of Wails NSIS helpers.
## AppData\wind-daq.exe is cleaned on uninstall (includes crash logs).
####

; Version must match projects/wind-daq/VERSION.
!define INFO_PRODUCTVERSION "0.11.4"

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"    "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

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
LangString INST_WELCOME_TITLE  ${LANG_SIMPCHINESE} "欢迎使用 Wind-DAQ 风洞数据采集系统"
LangString INST_WELCOME_TEXT   ${LANG_SIMPCHINESE} "本系统专为风洞测试与测量设计，集数据采集、运动控制、探针标定于一体。$\r$\n$\r$\n主要功能：$\r$\n  * 多通道压力与温度数据采集（DSA3217 / DAQ-P-1604 / DAQ-T-1603）$\r$\n  * 运动控制器管理（B140 / WTNMC4A）$\r$\n  * 五孔 / 三孔 / 总压 / 总温探针标定$\r$\n  * 风洞流场遍历测试与实时插值计算$\r$\n  * 数据记录、存储与报告生成$\r$\n$\r$\n安装程序将引导您完成安装。"
LangString INST_DIRECTORY_TEXT ${LANG_SIMPCHINESE} "选择安装目录。$\r$\n$\r$\n建议安装在非系统盘，并确保有足够的磁盘空间用于数据存储。"
LangString INST_FINISH_TITLE   ${LANG_SIMPCHINESE} "安装完成"
LangString INST_FINISH_TEXT    ${LANG_SIMPCHINESE} "Wind-DAQ 风洞数据采集系统已成功安装。$\r$\n$\r$\n点击完成退出安装程序。"
# English
LangString INST_WELCOME_TITLE  ${LANG_ENGLISH} "Welcome to Wind-DAQ Wind Tunnel DAQ System"
LangString INST_WELCOME_TEXT   ${LANG_ENGLISH} "Wind-DAQ is a comprehensive wind tunnel measurement platform for data acquisition, motion control, and probe calibration.$\r$\n$\r$\nKey features:$\r$\n  * Multi-channel pressure & temperature acquisition (DSA3217 / DAQ-P-1604 / DAQ-T-1603)$\r$\n  * Motion controller management (B140 / WTNMC4A)$\r$\n  * Five-hole / Three-hole / Total pressure / Total temperature calibration$\r$\n  * Traversal testing with real-time interpolation$\r$\n  * Data recording, storage and report generation$\r$\n$\r$\nThe wizard will guide you through the installation."
LangString INST_DIRECTORY_TEXT ${LANG_ENGLISH} "Select the installation folder.$\r$\n$\r$\nA non-system drive with sufficient free space for data storage is recommended."
LangString INST_FINISH_TITLE   ${LANG_ENGLISH} "Installation Complete"
LangString INST_FINISH_TEXT    ${LANG_ENGLISH} "Wind-DAQ has been successfully installed.$\r$\n$\r$\nClick Finish to exit the setup wizard."

# Registry key for installer language (must match backend/app.go::GetInstallerLanguage)
!define INSTALLER_LANG_REGKEY "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"

# WebView2 offline installer filename (alongside installer.exe)
!define WEBVIEW2_OFFLINE_INSTALLER "MicrosoftEdgeWebView2RuntimeInstallerX64.exe"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${INFO_PRODUCTVERSION}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

# Init: language selection -> architecture check -> offline webview2
Function .onInit
    !insertmacro MUI_LANGDLL_DISPLAY
    !insertmacro wails.checkArchitecture

    # Check for offline WebView2 installer alongside installer.exe
    StrCpy $0 "$EXEDIR\${WEBVIEW2_OFFLINE_INSTALLER}"
    ${If} ${FileExists} "$0"
        DetailPrint "Found WebView2 offline installer: $0"
        DetailPrint "Installing WebView2 Runtime (offline)..."
        ExecWait '"$0" /silent /install' $1
        ${If} $1 == 0
            DetailPrint "WebView2 Runtime installed successfully (offline)"
        ${Else}
            DetailPrint "WebView2 offline installer returned: $1 (may already be installed, continuing)"
        ${EndIf}
    ${Else}
        DetailPrint "No offline WebView2 installer found at $0"
        DetailPrint "Will use online download as fallback"
    ${EndIf}
FunctionEnd

Function IsWebView2Installed
    StrCpy $R0 "0"
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 != ""
    ${AndIf} $0 != "0.0.0.0"
        StrCpy $R0 "1"
        Goto check_done
    ${EndIf}
    ReadRegStr $0 HKCU "SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 != ""
    ${AndIf} $0 != "0.0.0.0"
        StrCpy $R0 "1"
    ${EndIf}
    check_done:
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # WebView2: check if already installed before downloading
    Call IsWebView2Installed
    ${If} $R0 == "0"
        !insertmacro wails.webview2runtime
    ${Else}
        DetailPrint "WebView2 Runtime already installed, skipping download"
    ${EndIf}

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # Device driver DLLs
    !ifdef ARG_WAILS_AMD64_BINARY
        ${if} ${IsNativeAMD64}
            File "WTNMC4A_64.dll"
        ${EndIf}
    !endif

    !ifdef ARG_WAILS_AMD64_BINARY
        ${if} ${IsNativeAMD64}
            File "..\..\bin\WTNDAQ16H_64.dll"
        ${EndIf}
    !endif

    ; Persist installer language to registry for app first-run
    ${If} $LANGUAGE == 2052
        WriteRegStr HKCU "${INSTALLER_LANG_REGKEY}" "InstallerLanguage" "zh"
    ${Else}
        WriteRegStr HKCU "${INSTALLER_LANG_REGKEY}" "InstallerLanguage" "en"
    ${EndIf}

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    DeleteRegKey HKCU "${INSTALLER_LANG_REGKEY}"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
