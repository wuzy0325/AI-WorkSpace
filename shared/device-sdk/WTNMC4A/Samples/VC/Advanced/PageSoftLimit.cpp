// PageSoftLimit.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageSoftLimit.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageSoftLimit property page

IMPLEMENT_DYNCREATE(CPageSoftLimit, CPropertyPage)

CPageSoftLimit::CPageSoftLimit() : CPropertyPage(CPageSoftLimit::IDD)
{
	//{{AFX_DATA_INIT(CPageSoftLimit)
	//}}AFX_DATA_INIT
}

CPageSoftLimit::~CPageSoftLimit()
{
}

void CPageSoftLimit::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageSoftLimit)
	DDX_Control(pDX, IDC_SET_SLIMIT, m_Button_Slimit);
	DDX_Control(pDX, IDC_CLEARLIMIT, m_Button_ClearLimit);
	DDX_Control(pDX, IDC_EDIT_UPDERLIMIT, m_Edit_UpperLimit);
	DDX_Control(pDX, IDC_EDIT_LOWERLIMIT, m_Edit_LowerLimit);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageSoftLimit, CPropertyPage)
	//{{AFX_MSG_MAP(CPageSoftLimit)
	ON_BN_CLICKED(IDC_SET_SLIMIT, OnSetSlimit)
	ON_BN_CLICKED(IDC_CLEARLIMIT, OnClearlimit)
	ON_BN_CLICKED(IDC_RADIO_LOGIC, OnRadioLogic)
	ON_BN_CLICKED(IDC_RADIO_FACT, OnRadioFact)
	ON_EN_CHANGE(IDC_EDIT_UPDERLIMIT, OnChangeEditUpderlimit)
	ON_EN_CHANGE(IDC_EDIT_LOWERLIMIT, OnChangeEditLowerlimit)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageSoftLimit message handlers

BOOL CPageSoftLimit::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	// TODO: Add extra initialization here
	CString str;
	m_nCurrentAxis = gl_pADView->m_nCurrentAxis;
	str.Format(L"%d", gl_pADView->m_OtherPara[m_nCurrentAxis].UpperLimit);
	m_Edit_UpperLimit.SetWindowText(str);
	str.Format(L"%d", gl_pADView->m_OtherPara[m_nCurrentAxis].LowerLimit);
	m_Edit_LowerLimit.SetWindowText(str);
	CButton *pBtnLogic = (CButton*)GetDlgItem(IDC_RADIO_LOGIC);
	pBtnLogic->SetCheck(1);
	
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

// 设置限位
void CPageSoftLimit::OnSetSlimit() 
{
	// TODO: Add your control notification handler code her
	if (m_bSetSoftLimit)
	{
		
	} 
	else
	{
	}
	
	if(!WTNMC4A_SetPDirSoftwareLimit(gl_pADView->m_hDevice,      // 正向限位
		gl_pADView->m_nCurrentAxis, 
		gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LogicFact, 
		gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].UpperLimit))
	{
		AfxMessageBox(L"设置正方向软件限位失败！");
		return;
	}
	
	if(!WTNMC4A_SetMDirSoftwareLimit(gl_pADView->m_hDevice,      // 负向限位
		gl_pADView->m_nCurrentAxis,
		gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LogicFact,
		gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LowerLimit))
	{
		AfxMessageBox(L"设置反方向软件限位失败！");
		return;
	}
	
    gl_pADView->m_bSLimit[gl_pADView->m_nCurrentAxis] = TRUE;
	CButton* pSetLimit = (CButton*)GetDlgItem(IDC_SET_SLIMIT);
	pSetLimit->EnableWindow(FALSE);
	
	CButton* pClearLimit = (CButton*)GetDlgItem(IDC_CLEARLIMIT);
	pClearLimit->EnableWindow(TRUE);
		
}

// 取消限位
void CPageSoftLimit::OnClearlimit() 
{
	// TODO: Add your control notification handler code here	
	if(!WTNMC4A_ClearSoftwareLimit(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis))
	{
		AfxMessageBox(L"清除软件限位失败！");
		return;
	}
	CButton* pSetLimit = (CButton*)GetDlgItem(IDC_SET_SLIMIT);
	pSetLimit->EnableWindow(TRUE);
	
	CButton* pClearLimit = (CButton*)GetDlgItem(IDC_CLEARLIMIT);
	pClearLimit->EnableWindow(FALSE);
	gl_pADView->m_bSLimit[gl_pADView->m_nCurrentAxis] = FALSE;
}

// 逻辑
void CPageSoftLimit::OnRadioLogic() 
{
	gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LogicFact = WTNMC4A_LOGIC;
}

// 实际
void CPageSoftLimit::OnRadioFact() 
{
	gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LogicFact = WTNMC4A_FACT;
}

// 上限限位
void CPageSoftLimit::OnChangeEditUpderlimit() 
{
	CString str;
	m_Edit_UpperLimit.GetWindowText(str);
	gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].UpperLimit = wcstol(str, NULL, 10);
//	TRACE("AXISNUM = %d, UpperLimit = %d\n", gl_pADView->m_nCurrentAxis, gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].UpperLimit);
}
// 下限限位
void CPageSoftLimit::OnChangeEditLowerlimit() 
{
	CString str;
	m_Edit_LowerLimit.GetWindowText(str);
	gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].LowerLimit = wcstol(str, NULL, 10);	
}

void CPageSoftLimit::SetCurrentAxisNum(int nCurrentAxis)
{

}
