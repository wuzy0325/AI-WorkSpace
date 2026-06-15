#if !defined(AFX_FILEOPENDLG_H__E6711E6E_A9EE_4942_9AA3_827B6B9252D4__INCLUDED_)
#define AFX_FILEOPENDLG_H__E6711E6E_A9EE_4942_9AA3_827B6B9252D4__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// FileOpenDlg.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CFileOpenDlg dialog

class CFileOpenDlg : public CDialog
{
// Construction
public:
	CFileOpenDlg(CWnd* pParent = NULL);   // standard constructor

// Dialog Data
	//{{AFX_DATA(CFileOpenDlg)
	enum { IDD = CG_IDD_FILE_OPEN };
	CEdit	m_Edit_Path;
	//}}AFX_DATA


// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CFileOpenDlg)
	protected:
	virtual void DoDataExchange(CDataExchange* pDX);    // DDX/DDV support
	//}}AFX_VIRTUAL

// Implementation
protected:
	int m_nBatCode;
	void InintFilePath(CString strPath);
	
	BOOL ReadFileHead(CString strFilepath, int ChannelNum);
	CString SelFilePath();
	// Generated message map functions
	//{{AFX_MSG(CFileOpenDlg)
	virtual void OnOK();
	virtual BOOL OnInitDialog();
	afx_msg void OnBUTTONFileSel0();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_FILEOPENDLG_H__E6711E6E_A9EE_4942_9AA3_827B6B9252D4__INCLUDED_)
