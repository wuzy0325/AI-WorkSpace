// Sys.h : main header file for the SYS application
//

#if !defined(AFX_SYS_H__366C9F68_5419_4B22_8BE0_56282ECDCEE7__INCLUDED_)
#define AFX_SYS_H__366C9F68_5419_4B22_8BE0_56282ECDCEE7__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000

#ifndef __AFXWIN_H__
	#error include 'stdafx.h' before including this file for PCH
#endif

#include "resource.h"       // main symbols
#include "ADDoc.h"
#include "ADFrm.h"
/////////////////////////////////////////////////////////////////////////////
// CSysApp:
// See Sys.cpp for the implementation of this class
//

class CSysApp : public CWinApp
{
public:
	CSysApp();
	int m_CurrentDeviceID;  // 记录当前设备ID号
	HANDLE m_hMutex;
	HANDLE m_hDeviceApp;
	CMultiDocTemplate* pADTemplate;
	CADDoc*            m_pADDoc;
	CADFrame*          m_pADFrm;

	CString m_IPAddr;
	ULONG m_nRSTimeout;
	BOOL m_bLinkSuccess;
// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CSysApp)
	public:
	virtual BOOL InitInstance();
	virtual int ExitInstance();
	//}}AFX_VIRTUAL

// Implementation
	//{{AFX_MSG(CSysApp)
	afx_msg void OnAppAbout();
	afx_msg void OnOpenAD();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
	afx_msg void OnNetCfg();
	afx_msg void OnListdeviceinfo();
};


/////////////////////////////////////////////////////////////////////////////

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_SYS_H__366C9F68_5419_4B22_8BE0_56282ECDCEE7__INCLUDED_)
