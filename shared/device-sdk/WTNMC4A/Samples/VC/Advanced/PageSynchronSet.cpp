// PageSynchronSet.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageSynchronSet.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageSynchronSet property page

IMPLEMENT_DYNCREATE(CPageSynchronSet, CPropertyPage)

CPageSynchronSet::CPageSynchronSet() : CPropertyPage(CPageSynchronSet::IDD)
{
	//{{AFX_DATA_INIT(CPageSynchronSet)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT
}

CPageSynchronSet::~CPageSynchronSet()
{
}

void CPageSynchronSet::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageSynchronSet)
	DDX_Control(pDX, IDC_EDIT_COMPN, m_Edit_COMPN);
	DDX_Control(pDX, IDC_EDIT_COMPP, m_Edit_COMPP);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageSynchronSet, CPropertyPage)
	//{{AFX_MSG_MAP(CPageSynchronSet)
	ON_BN_CLICKED(IDC_BUTTON_SynchronSet, OnBUTTONSynchronSet)
	ON_EN_CHANGE(IDC_EDIT_COMPP, OnChangeEditCompp)
	ON_EN_CHANGE(IDC_EDIT_COMPN, OnChangeEditCompn)
	ON_BN_CLICKED(IDC_BUTTON_StartSynchronActionX, OnBUTTONStartSynchronActionX)
	ON_BN_CLICKED(IDC_BUTTON_StartSynchronActionY, OnBUTTONStartSynchronActionY)
	ON_BN_CLICKED(IDC_BUTTON_StartSynchronActionU, OnBUTTONStartSynchronActionU)
	ON_BN_CLICKED(IDC_BUTTON_StartSynchronActionZ, OnBUTTONStartSynchronActionZ)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageSynchronSet message handlers

void CPageSynchronSet::OnBUTTONSynchronSet() 
{
	// TODO: Add your control notification handler code here
	m_SynchronSheet.ShowWindow(SW_SHOW);
}

BOOL CPageSynchronSet::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	// TODO: Add extra initialization here
	m_SynchronSheet.Create(IDD_Page_SynchronSheet, this);
	m_SynchronSheet.ShowWindow(SW_HIDE);
	SetCurrentAxisNum(gl_pADView->m_nCurrentAxis);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CPageSynchronSet::OnChangeEditCompp() 
{
	// TODO: Add your control notification handler code here
 	CString str;
 	m_Edit_COMPP.GetWindowText(str);  
	for (int i=0; i<4; i++)
	{
 		gl_pADView->m_nSynchronCOMPPValue[i] = wcstol(str, NULL, 10);
	}
}

void CPageSynchronSet::OnChangeEditCompn() 
{
	// TODO: Add your control notification handler code here
 	CString str;
 	m_Edit_COMPN.GetWindowText(str);  
 	int nValue = wcstol(str, NULL, 10);
 //	if(nValue > 0) nValue *= -1;      // 保证永远是负值
	for (int i=0; i<4; i++)
	{
 		gl_pADView->m_nSynchronCOMPNValue[i] = nValue;	
	}
}

void CPageSynchronSet::SetCurrentAxisNum(int nCurrentAxis)
{
	m_nCurrentAxis = nCurrentAxis;
	CString str;
    m_nCurrentAxis = gl_pADView->m_nCurrentAxis;
	str.Format(L"%d", gl_pADView->m_nSynchronCOMPPValue[m_nCurrentAxis]); // COMP+的值
	m_Edit_COMPP.SetWindowText(str);
	str.Format(L"%d", gl_pADView->m_nSynchronCOMPNValue[m_nCurrentAxis]); // COMP-的值
	m_Edit_COMPN.SetWindowText(str);
}

void CPageSynchronSet::OnBUTTONStartSynchronActionX() 
{
	WTNMC4A_WriteSynchronActionCom(gl_pADView->m_hDevice, WTNMC4A_XAXIS);
	
}

void CPageSynchronSet::OnBUTTONStartSynchronActionY() 
{
	WTNMC4A_WriteSynchronActionCom(gl_pADView->m_hDevice, WTNMC4A_YAXIS);
	
}

void CPageSynchronSet::OnBUTTONStartSynchronActionU() 
{
	WTNMC4A_WriteSynchronActionCom(gl_pADView->m_hDevice, WTNMC4A_UAXIS);
	
}

void CPageSynchronSet::OnBUTTONStartSynchronActionZ() 
{
	WTNMC4A_WriteSynchronActionCom(gl_pADView->m_hDevice, WTNMC4A_ZAXIS);
	
}
