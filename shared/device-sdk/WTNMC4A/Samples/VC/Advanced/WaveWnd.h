#if !defined(AFX_WAVE_H__24BE50A3_975B_11D8_8A5B_C53AB6349C2C__INCLUDED_)
#define AFX_WAVE_H__24BE50A3_975B_11D8_8A5B_C53AB6349C2C__INCLUDED_

#if _MSC_VER > 1000
#pragma once
#endif // _MSC_VER > 1000
// Wave.h : header file
//

/////////////////////////////////////////////////////////////////////////////
// CWave window

class CWaveWnd : public CWnd
{
// Construction
public:
	CWaveWnd();

// Attributes
public:
	BOOL m_bConstant;
	PWORD m_pWaveBuffer;
	PWORD m_pDigitBuffer;
// Operations
public:

// Overrides
	// ClassWizard generated virtual function overrides
	//{{AFX_VIRTUAL(CWave)
	public:
	virtual BOOL Create(DWORD dwStyle, const RECT& rect, CWnd* pParentWnd, UINT nID=NULL);
	//}}AFX_VIRTUAL

// Implementation
public:
	virtual ~CWaveWnd();

	// Generated message map functions
protected:
	//{{AFX_MSG(CWaveWnd)
	afx_msg void OnPaint();
	//}}AFX_MSG
	DECLARE_MESSAGE_MAP()
};

/////////////////////////////////////////////////////////////////////////////

//{{AFX_INSERT_LOCATION}}
// Microsoft Visual C++ will insert additional declarations immediately before the previous line.

#endif // !defined(AFX_WAVE_H__24BE50A3_975B_11D8_8A5B_C53AB6349C2C__INCLUDED_)
