Unicode true

!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
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
LangString INST_WELCOME_TITLE  ${LANG_SIMPCHINESE} "欢迎使用三孔探针插值器"
LangString INST_WELCOME_TEXT   ${LANG_SIMPCHINESE} "三孔探针插值器 专为风洞三孔探针测量数据的后处理设计。$\r$\n$\r$\n主要功能：$\r$\n  * 导入三孔探针压力测量数据（CSV）$\r$\n  * 基于 PRB 标定数据插值反算气流方向角（α、β）$\r$\n  * 计算总压、静压、速度与马赫数$\r$\n  * 批量计算与结果导出$\r$\n$\r$\n安装程序将引导您完成安装。"
LangString INST_DIRECTORY_TEXT ${LANG_SIMPCHINESE} "选择安装目录。$\r$\n$\r$\n建议安装在非系统盘，并确保有足够的磁盘空间用于数据存储。"
LangString INST_FINISH_TITLE   ${LANG_SIMPCHINESE} "安装完成"
LangString INST_FINISH_TEXT    ${LANG_SIMPCHINESE} "三孔探针插值器 已成功安装。$\r$\n$\r$\n点击完成退出安装程序。"
LangString INST_RUN_TEXT       ${LANG_SIMPCHINESE} "启动三孔探针插值器"
# English
LangString INST_WELCOME_TITLE  ${LANG_ENGLISH} "Welcome to Three-Hole Probe Interpolator"
LangString INST_WELCOME_TEXT   ${LANG_ENGLISH} "Three-Hole Probe Interpolator is designed for post-processing wind tunnel three-hole probe measurements.$\r$\n$\r$\nKey features:$\r$\n  * Import three-hole probe pressure data (CSV)$\r$\n  * Interpolate flow angles (α, β) from PRB calibration data$\r$\n  * Compute total pressure, static pressure, velocity and Mach number$\r$\n  * Batch calculation with result export$\r$\n$\r$\nThe wizard will guide you through the installation."
LangString INST_DIRECTORY_TEXT ${LANG_ENGLISH} "Select the installation folder.$\r$\n$\r$\nA non-system drive with sufficient free space for data storage is recommended."
LangString INST_FINISH_TITLE   ${LANG_ENGLISH} "Installation Complete"
LangString INST_FINISH_TEXT    ${LANG_ENGLISH} "Three-Hole Probe Interpolator has been successfully installed.$\r$\n$\r$\nClick Finish to exit the setup wizard."
LangString INST_RUN_TEXT       ${LANG_ENGLISH} "Launch Three-Hole Probe Interpolator"

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
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

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

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
