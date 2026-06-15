#if !defined(AFX_PAGEINTERAPP_H__A55B2D63_AF93_46F7_B5A2_BF09DA260E25__INCLUDED_)
#define AFX_PAGEINTERAPP_H__A55B2D63_AF93_46F7_B5A2_BF09DA260E25__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageInterApp.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CPageInterApp dialog

class CPageInterApp : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageInterApp)

// Construction
public:
	CPageInterApp();
	~CPageInterApp();

// Dialog Data
	//{{AFX_DATA(CPageInterApp)
	enum { IDD = IDD_Page_InterpApp };
	CButton	m_IntStartBit;
	CButton	m_IntStopBit;
	CButton	m_StopSequence;
	CButton	m_StartSequence;
	CButton	m_StopBit;
	CButton	m_StartBit;
	//}}AFX_DATA


// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageInterApp)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	// Generated message map functions
	//{{AFX_MSG(CPageInterApp)
	virtual BOOL OnInitDialog();
	afx_msg void OnStartBit();
	afx_msg void OnStartSequence();
	afx_msg void OnIntStartBit();
	afx_msg void OnStopBit();
	afx_msg void OnStopSequence();
	afx_msg void OnIntSttopBit();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGEINTERAPP_H__A55B2D63_AF93_46F7_B5A2_BF09DA260E25__INCLUDED_)
