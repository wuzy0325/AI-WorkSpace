Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default ""
## !define INFO_COMPANYNAME    "My Company" # Default "My Company"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "My Product"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© now, My Company"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####

# Product version must match projects/probe-interpolator/VERSION.
# 所有 INFO_* 变量必须在 !include "wails_tools.nsh" 之前显式定义，
# 因为本地 makensis 调用不经过 wails build 流程，wails_tools.nsh 不会从 wails.json 回填项目信息，
# 默认值是空字符串会导致 installer 文件名变成 "-<version>-amd64-installer.exe"。
!define INFO_PROJECTNAME "probe-interpolator"
!define INFO_COMPANYNAME "Wind-DAQ Team"
!define INFO_PRODUCTNAME "Probe Interpolator"
!define INFO_PRODUCTVERSION "0.3.0"
!define INFO_COPYRIGHT "© 2026, Wind-DAQ Team"

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
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

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

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

# Languages (first = default)
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

# ------ Installer UI Strings ------
# Chinese
LangString INST_WELCOME_TITLE  ${LANG_SIMPCHINESE} "欢迎使用探针插值器"
LangString INST_WELCOME_TEXT   ${LANG_SIMPCHINESE} "探针插值器 是风洞多孔探针测量数据后处理的一体化工具。$\r$\n$\r$\n主要功能：$\r$\n  * 支持三孔 / 五孔 / 七孔探针插值计算$\r$\n  * 反算气流方向角（α、β）、总压、静压、速度与马赫数$\r$\n  * 内置用户说明书与在线帮助$\r$\n  * 批量计算与结果导出$\r$\n$\r$\n安装程序将引导您完成安装。"
LangString INST_DIRECTORY_TEXT ${LANG_SIMPCHINESE} "选择安装目录。$\r$\n$\r$\n建议安装在非系统盘，并确保有足够的磁盘空间用于数据存储。"
LangString INST_FINISH_TITLE   ${LANG_SIMPCHINESE} "安装完成"
LangString INST_FINISH_TEXT    ${LANG_SIMPCHINESE} "探针插值器 已成功安装。$\r$\n$\r$\n点击完成退出安装程序。"
LangString INST_RUN_TEXT       ${LANG_SIMPCHINESE} "启动探针插值器"
# English
LangString INST_WELCOME_TITLE  ${LANG_ENGLISH} "Welcome to Probe Interpolator"
LangString INST_WELCOME_TEXT   ${LANG_ENGLISH} "Probe Interpolator is an all-in-one post-processing tool for wind tunnel multi-hole probe measurements.$\r$\n$\r$\nKey features:$\r$\n  * Supports 3-hole / 5-hole / 7-hole probe interpolation$\r$\n  * Computes flow angles (α, β), total pressure, static pressure, velocity and Mach number$\r$\n  * Built-in user manuals and online help$\r$\n  * Batch calculation with result export$\r$\n$\r$\nThe wizard will guide you through the installation."
LangString INST_DIRECTORY_TEXT ${LANG_ENGLISH} "Select the installation folder.$\r$\n$\r$\nA non-system drive with sufficient free space for data storage is recommended."
LangString INST_FINISH_TITLE   ${LANG_ENGLISH} "Installation Complete"
LangString INST_FINISH_TEXT    ${LANG_ENGLISH} "Probe Interpolator has been successfully installed.$\r$\n$\r$\nClick Finish to exit the setup wizard."
LangString INST_RUN_TEXT       ${LANG_ENGLISH} "Launch Probe Interpolator"

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
# Output to build/bin/ with version in filename, matches archive-release script convention.
# Path is relative to windows/nsis/ (where makensis runs): ../../build/bin/ -> apps/desktop-wails/build/bin/
OutFile "..\..\build\bin\${INFO_PROJECTNAME}-${INFO_PRODUCTVERSION}-${ARCH}-installer.exe" # Name of the installer's file.
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro MUI_LANGDLL_DISPLAY
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # 用户说明书 HTML（5/3/7 孔各一份）必须随安装包发布，否则 OpenHelpDocByPath
    # 在安装目录下找不到 docs/<fileName> 会报"未找到用户说明书文件"。
    # 源路径相对 windows/nsis/：..\..\docs -> apps/desktop-wails/docs
    SetOutPath $INSTDIR\docs
    File /r "..\..\docs\*.*"
    SetOutPath $INSTDIR

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r "$INSTDIR\docs" # 卸载时清理用户说明书 HTML
    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
