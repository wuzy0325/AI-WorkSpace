#if !defined(AFX_PAGESYNCHRONX_H__148AD17D_0835_4697_9C1E_1EEC1D83EC0B__INCLUDED_)
#define AFX_PAGESYNCHRONX_H__148AD17D_0835_4697_9C1E_1EEC1D83EC0B__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// PageSynchronX.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CPageSynchron dialog

class CPageSynchron : public CPropertyPage
{
	DECLARE_DYNCREATE(CPageSynchron)

// Construction
public:
	void SetAxisNum(int nAxisNum);
	CPageSynchron();
	~CPageSynchron();

// Dialog Data
	//{{AFX_DATA(CPageSynchron)
	enum { IDD = IDD_Page_SynchronX };
		// NOTE - ClassWizard will add data members here.
		//    DO NOT EDIT what you see in these blocks of generated code !
	//}}AFX_DATA
protected:
	int m_nAxisNum;

// Overrides
	// ClassWizard generate virtual function overrides
	//{{AFX_VIRTUAL(CPageSynchron)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	// Generated message map functions
	//{{AFX_MSG(CPageSynchron)
	afx_msg void OnCheckAxis1();
	afx_msg void OnCheckAxis2();
	afx_msg void OnCheckAxis3();
	afx_msg void OnCheckCmdX();
	afx_msg void OnCheckLprdX();
	afx_msg void OnCheckDstaX();
	afx_msg void OnCheckDendX();
	afx_msg void OnRadioLpx();
	afx_msg void OnRadioEpx();
	afx_msg void OnCheckPbcpX();
	afx_msg void OnCheckPscpX();
	afx_msg void OnCheckPbcmX();
	afx_msg void OnCheckPscmX();
	afx_msg void OnCheckIn3lhX();
	afx_msg void OnCheckIn3hlX();
	virtual BOOL OnInitDialog();
	afx_msg void OnCheckFdrvpX();
	afx_msg void OnCheckFdrvmX();
	afx_msg void OnCheckCdrvpX();
	afx_msg void OnCheckCdrvmX();
	afx_msg void OnCheckSstopX();
	afx_msg void OnCheckIstopX();
	afx_msg void OnCheckLpsavX();
	afx_msg void OnCheckEpsavX();
	afx_msg void OnCheckLpsetX();
	afx_msg void OnCheckOpsetX();
	afx_msg void OnCheckVlsetX();
	afx_msg void OnCheckOutnX();
	afx_msg void OnCheckIntnX();
	afx_msg void OnCheckEpsetX();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()

};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_PAGESYNCHRONX_H__148AD17D_0835_4697_9C1E_1EEC1D83EC0B__INCLUDED_)
