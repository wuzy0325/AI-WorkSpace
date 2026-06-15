# Microsoft Developer Studio Project File - Name="Sys" - Package Owner=<4>
# Microsoft Developer Studio Generated Build File, Format Version 6.00
# ** DO NOT EDIT **

# TARGTYPE "Win32 (x86) Application" 0x0101

CFG=Sys - Win32 Debug
!MESSAGE This is not a valid makefile. To build this project using NMAKE,
!MESSAGE use the Export Makefile command and run
!MESSAGE 
!MESSAGE NMAKE /f "Sys.mak".
!MESSAGE 
!MESSAGE You can specify a configuration when running NMAKE
!MESSAGE by defining the macro CFG on the command line. For example:
!MESSAGE 
!MESSAGE NMAKE /f "Sys.mak" CFG="Sys - Win32 Debug"
!MESSAGE 
!MESSAGE Possible choices for configuration are:
!MESSAGE 
!MESSAGE "Sys - Win32 Release" (based on "Win32 (x86) Application")
!MESSAGE "Sys - Win32 Debug" (based on "Win32 (x86) Application")
!MESSAGE 

# Begin Project
# PROP AllowPerConfigDependencies 0
# PROP Scc_ProjName ""
# PROP Scc_LocalPath ""
CPP=cl.exe
MTL=midl.exe
RSC=rc.exe

!IF  "$(CFG)" == "Sys - Win32 Release"

# PROP BASE Use_MFC 6
# PROP BASE Use_Debug_Libraries 0
# PROP BASE Output_Dir "Release"
# PROP BASE Intermediate_Dir "Release"
# PROP BASE Target_Dir ""
# PROP Use_MFC 6
# PROP Use_Debug_Libraries 0
# PROP Output_Dir ""
# PROP Intermediate_Dir "Release"
# PROP Ignore_Export_Lib 0
# PROP Target_Dir ""
# ADD BASE CPP /nologo /MD /W3 /GX /O2 /D "WIN32" /D "NDEBUG" /D "_WINDOWS" /D "_AFXDLL" /Yu"stdafx.h" /FD /c
# ADD CPP /nologo /MD /W3 /GX /O2 /D "WIN32" /D "NDEBUG" /D "_WINDOWS" /D "_AFXDLL" /D "_MBCS" /FR /Yu"stdafx.h" /FD /c
# ADD BASE MTL /nologo /D "NDEBUG" /mktyplib203 /win32
# ADD MTL /nologo /D "NDEBUG" /mktyplib203 /win32
# ADD BASE RSC /l 0x804 /d "NDEBUG" /d "_AFXDLL"
# ADD RSC /l 0x804 /d "NDEBUG" /d "_AFXDLL"
BSC32=bscmake.exe
# ADD BASE BSC32 /nologo
# ADD BSC32 /nologo
LINK32=link.exe
# ADD BASE LINK32 /nologo /subsystem:windows /machine:I386
# ADD LINK32 /nologo /subsystem:windows /machine:I386

!ELSEIF  "$(CFG)" == "Sys - Win32 Debug"

# PROP BASE Use_MFC 6
# PROP BASE Use_Debug_Libraries 1
# PROP BASE Output_Dir "Debug"
# PROP BASE Intermediate_Dir "Debug"
# PROP BASE Target_Dir ""
# PROP Use_MFC 6
# PROP Use_Debug_Libraries 1
# PROP Output_Dir ""
# PROP Intermediate_Dir "Debug"
# PROP Ignore_Export_Lib 0
# PROP Target_Dir ""
# ADD BASE CPP /nologo /MDd /W3 /Gm /GX /ZI /Od /D "WIN32" /D "_DEBUG" /D "_WINDOWS" /D "_AFXDLL" /Yu"stdafx.h" /FD /GZ /c
# ADD CPP /nologo /MDd /W3 /Gm /GX /ZI /Od /D "WIN32" /D "_DEBUG" /D "_WINDOWS" /D "_AFXDLL" /D "_MBCS" /FR /Yu"stdafx.h" /FD /GZ /c
# ADD BASE MTL /nologo /D "_DEBUG" /mktyplib203 /win32
# ADD MTL /nologo /D "_DEBUG" /mktyplib203 /win32
# ADD BASE RSC /l 0x804 /d "_DEBUG" /d "_AFXDLL"
# ADD RSC /l 0x804 /d "_DEBUG" /d "_AFXDLL"
BSC32=bscmake.exe
# ADD BASE BSC32 /nologo
# ADD BSC32 /nologo
LINK32=link.exe
# ADD BASE LINK32 /nologo /subsystem:windows /debug /machine:I386 /pdbtype:sept
# ADD LINK32 /nologo /subsystem:windows /debug /machine:I386 /pdbtype:sept

!ENDIF 

# Begin Target

# Name "Sys - Win32 Release"
# Name "Sys - Win32 Debug"
# Begin Group "Source Files"

# PROP Default_Filter "cpp;c;cxx;rc;def;r;odl;idl;hpj;bat"
# Begin Source File

SOURCE=.\ADDoc.cpp
# End Source File
# Begin Source File

SOURCE=.\ADFrm.cpp
# End Source File
# Begin Source File

SOURCE=.\ADView.cpp
# End Source File
# Begin Source File

SOURCE=.\AutoHomeSearchDlg.cpp
# End Source File
# Begin Source File

SOURCE=.\ComSetPage.cpp
# End Source File
# Begin Source File

SOURCE=.\FilterSetDlg.cpp
# End Source File
# Begin Source File

SOURCE=.\InterruptSetDlg.cpp
# End Source File
# Begin Source File

SOURCE=.\MainFrm.cpp
# End Source File
# Begin Source File

SOURCE=.\PageDIO.cpp
# End Source File
# Begin Source File

SOURCE=.\PageHardLimit.cpp
# End Source File
# Begin Source File

SOURCE=.\PageInterApp.cpp
# End Source File
# Begin Source File

SOURCE=.\PageInterpolation.cpp
# End Source File
# Begin Source File

SOURCE=.\PageLine.cpp
# End Source File
# Begin Source File

SOURCE=.\PageSoftLimit.cpp
# End Source File
# Begin Source File

SOURCE=.\PageSynchron.cpp
# End Source File
# Begin Source File

SOURCE=.\PageSynchronSet.cpp
# End Source File
# Begin Source File

SOURCE=.\StdAfx.cpp
# ADD CPP /Yc"stdafx.h"
# End Source File
# Begin Source File

SOURCE=.\SynchronPageSheet.cpp
# End Source File
# Begin Source File

SOURCE=.\Sys.cpp
# End Source File
# Begin Source File

SOURCE=.\Sys.rc
# End Source File
# End Group
# Begin Group "Header Files"

# PROP Default_Filter "h;hpp;hxx;hm;inl"
# Begin Source File

SOURCE=.\ADDoc.h
# End Source File
# Begin Source File

SOURCE=.\ADFrm.h
# End Source File
# Begin Source File

SOURCE=.\ADView.h
# End Source File
# Begin Source File

SOURCE=.\AutoHomeSearchDlg.h
# End Source File
# Begin Source File

SOURCE=.\ComSetPage.h
# End Source File
# Begin Source File

SOURCE=.\FilterSetDlg.h
# End Source File
# Begin Source File

SOURCE=.\InterruptSetDlg.h
# End Source File
# Begin Source File

SOURCE=.\MainFrm.h
# End Source File
# Begin Source File

SOURCE=.\PageDIO.h
# End Source File
# Begin Source File

SOURCE=.\PageHardLimit.h
# End Source File
# Begin Source File

SOURCE=.\PageInterApp.h
# End Source File
# Begin Source File

SOURCE=.\PageInterpolation.h
# End Source File
# Begin Source File

SOURCE=.\PageLine.h
# End Source File
# Begin Source File

SOURCE=.\PageSoftLimit.h
# End Source File
# Begin Source File

SOURCE=.\PageSynchron.h
# End Source File
# Begin Source File

SOURCE=.\PageSynchronSet.h
# End Source File
# Begin Source File

SOURCE=.\Resource.h
# End Source File
# Begin Source File

SOURCE=.\StdAfx.h
# End Source File
# Begin Source File

SOURCE=.\SynchronPageSheet.h
# End Source File
# Begin Source File

SOURCE=.\Sys.h
# End Source File
# Begin Source File

SOURCE=.\USB1020.h
# End Source File
# Begin Source File

SOURCE=.\UserDef.h
# End Source File
# End Group
# Begin Group "Resource Files"

# PROP Default_Filter "ico;cur;bmp;dlg;rc2;rct;bin;rgs;gif;jpg;jpeg;jpe"
# Begin Source File

SOURCE=.\res\ADDoc.ico
# End Source File
# Begin Source File

SOURCE=.\RES\bitmap1.bmp
# End Source File
# Begin Source File

SOURCE=.\RES\bmp00001.bmp
# End Source File
# Begin Source File

SOURCE=.\RES\bmp00002.bmp
# End Source File
# Begin Source File

SOURCE=.\res\BmpGray.bmp
# End Source File
# Begin Source File

SOURCE=.\res\BmpHigher.bmp
# End Source File
# Begin Source File

SOURCE=.\res\BmpLower.bmp
# End Source File
# Begin Source File

SOURCE=.\Res\BmpRed.bmp
# End Source File
# Begin Source File

SOURCE=.\RES\heartsha.bmp
# End Source File
# Begin Source File

SOURCE=.\res\Sys.ico
# End Source File
# Begin Source File

SOURCE=.\res\Sys.rc2
# End Source File
# Begin Source File

SOURCE=.\res\Toolbar.bmp
# End Source File
# End Group
# Begin Source File

SOURCE=.\ReadMe.txt
# End Source File
# End Target
# End Project
