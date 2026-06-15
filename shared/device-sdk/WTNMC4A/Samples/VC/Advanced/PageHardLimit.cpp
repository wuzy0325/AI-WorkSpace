// PageHardLimit.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageHardLimit.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

extern LONG gl_LogLever;
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageHardLimit property page

IMPLEMENT_DYNCREATE(CPageHardLimit, CPropertyPage)

CPageHardLimit::CPageHardLimit() : CPropertyPage(CPageHardLimit::IDD)
{
	m_nStopNum = 0;
	for(int i=0; i< 4; i++)
	{
		m_nStopNumSts[i] = FALSE;
	}
	//{{AFX_DATA_INIT(CPageHardLimit)
	//}}AFX_DATA_INIT
}

CPageHardLimit::~CPageHardLimit()
{
}

void CPageHardLimit::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageHardLimit)
	DDX_Control(pDX, IDC_ClearInPos, m_Button_ClearInPos);
	DDX_Control(pDX, IDC_SetInPos, m_Button_SetInPos);
	DDX_Control(pDX, IDC_ClearALARM, m_Button_ClearAlarm);
	DDX_Control(pDX, IDC_SetALARM, m_Button_Alarm);
	DDX_Control(pDX, IDC_Set_HLimit, m_Button_SetHLimit);
	DDX_Control(pDX, IDC_Clear_StopNum, m_Button_ClearStopNum);
	DDX_Control(pDX, IDC_SetStopNum, m_Button_SetStopNum);
	DDX_Control(pDX, IDC_BUTTON_FilterSet, m_Button_FilterSet);
	DDX_Control(pDX, IDC_CHECK_FilterEnable, m_Button_FilterEnable);
	DDX_Control(pDX, IDC_COMBO_StopNum, m_Combo_StopNum);
	DDX_Control(pDX, IDC_COMBO_StopType, m_Combo_StopType);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageHardLimit, CPropertyPage)
	//{{AFX_MSG_MAP(CPageHardLimit)
	ON_CBN_SELCHANGE(IDC_COMBO_StopType, OnSelchangeCOMBOStopType)
	ON_BN_CLICKED(IDC_Set_HLimit, OnSetHLimit)
	ON_BN_CLICKED(IDC_SetALARM, OnSetALARM)
	ON_BN_CLICKED(IDC_ClearALARM, OnClearALARM)
	ON_BN_CLICKED(IDC_SetStopNum, OnSetStopNum)
	ON_BN_CLICKED(IDC_Clear_StopNum, OnClearStopNum)
	ON_BN_CLICKED(IDC_SetInPos, OnSetInPos)
	ON_BN_CLICKED(IDC_ClearInPos, OnClearInPos)
	ON_CBN_SELCHANGE(IDC_COMBO_StopNum, OnSelchangeCOMBOStopNum)
	ON_BN_CLICKED(IDC_CHECK_FilterEnable, OnCHECKFilterEnable)
	ON_BN_CLICKED(IDC_BUTTON_FilterSet, OnBUTTONFilterSet)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageHardLimit message handlers

void CPageHardLimit::OnSelchangeCOMBOStopType() 
{
	// TODO: Add your control notification handler code here
//	gl_pADView->m_OtherPara[gl_pADView->m_nCurrentAxis].
}

// 设置硬件限位
void CPageHardLimit::OnSetHLimit() 
{
	// TODO: Add your control notification handler code here
	CComboBox* pStopType = (CComboBox*)GetDlgItem(IDC_COMBO_StopType);
	USHORT nStopMode;
	if(pStopType->GetCurSel() == 0)
	{
		nStopMode = WTNMC4A_SUDDENSTOP; // 立即停止
	}
	else
	{
		nStopMode = WTNMC4A_DECSTOP;    // 减速停止
	}
	if(!WTNMC4A_SetPDirLMTEnable(gl_pADView->m_hDevice,
		gl_pADView->m_nCurrentAxis,
		nStopMode, 
		gl_LogLever))
	{
		AfxMessageBox(L"设置正向硬件限位失败！");
		return;
	}	
	if(!WTNMC4A_SetMDirLMTEnable(gl_pADView->m_hDevice,
		gl_pADView->m_nCurrentAxis,
		nStopMode, 
		gl_LogLever))
	{
		AfxMessageBox(L"设置正向硬件限位失败！");
		return;
	}	
}

// 设置报警信号
void CPageHardLimit::OnSetALARM() 
{
	// TODO: Add your control notification handler code here
	if(!WTNMC4A_SetALARMEnable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis, gl_LogLever))
	{
		AfxMessageBox(L"设置报警信号有效失败！");
		return;
	}
	
	CButton* pSetAlarm = (CButton*)GetDlgItem(IDC_SetALARM);
	pSetAlarm->EnableWindow(FALSE);
	
	CButton* pClearAlarm = (CButton*)GetDlgItem(IDC_ClearALARM);
	pClearAlarm->EnableWindow(TRUE);
	
	gl_pADView->m_bAlarm[gl_pADView->m_nCurrentAxis] = TRUE;	
}

// 清除报警信号
void CPageHardLimit::OnClearALARM() 
{
	// TODO: Add your control notification handler code here
	if(!WTNMC4A_SetALARMDisable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis))
	{
		AfxMessageBox(L"清除报警信号有效失败！");
		return;
	}
	CButton* pSetAlarm = (CButton*)GetDlgItem(IDC_SetALARM);
	pSetAlarm->EnableWindow(TRUE);
	
	CButton* pClearAlarm = (CButton*)GetDlgItem(IDC_ClearALARM);
	pClearAlarm->EnableWindow(FALSE);
	
	gl_pADView->m_bAlarm[gl_pADView->m_nCurrentAxis] = FALSE;
}

// 设置停止号
void CPageHardLimit::OnSetStopNum() 
{
	// TODO: Add your control notification handler code here
	m_nStopNum = m_Combo_StopNum.GetCurSel(); // 停止号
	if(!WTNMC4A_SetStopEnable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis, m_nStopNum, gl_LogLever))
	{
		AfxMessageBox(L"设置外部停止号有效失败！");
		return;
	}
	
	CButton* pSetStopNum = (CButton*)GetDlgItem(IDC_SetStopNum);
	pSetStopNum->EnableWindow(FALSE);
	
	CButton* pClearStopNum = (CButton*)GetDlgItem(IDC_Clear_StopNum);
	pClearStopNum->EnableWindow(TRUE);
	m_nStopNumSts[m_nStopNum] = TRUE;
	gl_pADView->m_bStopNum[gl_pADView->m_nCurrentAxis][m_nStopNum] = TRUE;
}

// 取消外部停止信号
void CPageHardLimit::OnClearStopNum() 
{
	// TODO: Add your control notification handler code here
	m_nStopNum = m_Combo_StopNum.GetCurSel();
	if(!WTNMC4A_SetStopDisable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis, m_nStopNum))
	{
		AfxMessageBox(L"清除外部停止号有效失败！");
		return;
	}
	
	CButton* pSetStopNum = (CButton*)GetDlgItem(IDC_SetStopNum);
	pSetStopNum->EnableWindow(TRUE);
	
	CButton* pClearStopNum = (CButton*)GetDlgItem(IDC_Clear_StopNum);
	pClearStopNum->EnableWindow(FALSE);
	
	m_nStopNumSts[m_nStopNum] = FALSE;
	gl_pADView->m_bStopNum[gl_pADView->m_nCurrentAxis][m_nStopNum] = FALSE;
}

// 设置伺服马达定位完毕输入信号有效 
void CPageHardLimit::OnSetInPos() 
{
	// TODO: Add your control notification handler code here
	if(!WTNMC4A_SetINPOSEnable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis, gl_LogLever))
	{
		AfxMessageBox(L"设置伺服电机定位完毕输入信号失败！");
		return;
	}
	
	CButton* pSetInPos = (CButton*)GetDlgItem(IDC_SetInPos);
	pSetInPos->EnableWindow(FALSE);
	
	CButton* pClearInPos = (CButton*)GetDlgItem(IDC_ClearInPos);
	pClearInPos->EnableWindow(TRUE);
	gl_pADView->m_bInPos[gl_pADView->m_nCurrentAxis] = TRUE;
}

// 清除马达定位完毕输入信号有效
void CPageHardLimit::OnClearInPos() 
{
	// TODO: Add your control notification handler code here
    if(!WTNMC4A_SetINPOSDisable(gl_pADView->m_hDevice, gl_pADView->m_nCurrentAxis))
	{
		AfxMessageBox(L"清除马达定位完毕输入信号有效失败！");
		return;
	}
	CButton* pSetInPos = (CButton*)GetDlgItem(IDC_SetInPos);
	pSetInPos->EnableWindow(TRUE);
	
	CButton* pClearInPos = (CButton*)GetDlgItem(IDC_ClearInPos);
	pClearInPos->EnableWindow(FALSE);

	gl_pADView->m_bInPos[gl_pADView->m_nCurrentAxis] = FALSE;
}

void CPageHardLimit::OnSelchangeCOMBOStopNum() 
{
	CComboBox* pStopNum = (CComboBox*)GetDlgItem(IDC_COMBO_StopNum);
	CButton* pSetStopNum = (CButton*)GetDlgItem(IDC_SetStopNum);	
	CButton* pClearStopNum = (CButton*)GetDlgItem(IDC_Clear_StopNum);
	int nStopNum = pStopNum->GetCurSel();
	if (m_nStopNumSts[nStopNum] && gl_pADView->m_bStopNum[gl_pADView->m_nCurrentAxis][nStopNum])
	{
		pSetStopNum->EnableWindow(FALSE);
		pClearStopNum->EnableWindow(TRUE);
	}
	else
	{
		pSetStopNum->EnableWindow(TRUE);
		pClearStopNum->EnableWindow(FALSE);
	}
	
}

void CPageHardLimit::InitCfg(int nAxisNum)
{
	m_Combo_StopType.SetCurSel(0);
	m_Combo_StopNum.SetCurSel(0);
	m_Button_FilterSet.EnableWindow(m_Button_FilterEnable.GetCheck());
	CComboBox* pStopNum = (CComboBox*)GetDlgItem(IDC_COMBO_StopNum);
	CButton* pSetStopNum = (CButton*)GetDlgItem(IDC_SetStopNum);
	CButton* pClearStopNum = (CButton*)GetDlgItem(IDC_Clear_StopNum);
	int nStopNum = pStopNum->GetCurSel();
	if (m_nStopNumSts[nStopNum] && gl_pADView->m_bStopNum[gl_pADView->m_nCurrentAxis][m_nStopNum])
	{
		pSetStopNum->EnableWindow(FALSE);
		pClearStopNum->EnableWindow(TRUE);
	}
	else
	{
		pSetStopNum->EnableWindow(TRUE);
		pClearStopNum->EnableWindow(FALSE);
	}
}


BOOL CPageHardLimit::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	InitCfg(gl_pADView->m_nCurrentAxis);
	
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

 
// 选择滤波器有效
void CPageHardLimit::OnCHECKFilterEnable() 
{
	// TODO: Add your control notification handler code here
	WTNMC4A_PARA_ExpMode filterPara;
	int nCurrentAxis = gl_pADView->m_nCurrentAxis;
	int nCheck = m_Button_FilterEnable.GetCheck();
	if(nCheck)
	{
		m_Button_FilterSet.EnableWindow(TRUE);
		filterPara = gl_pADView->m_FilterPara[nCurrentAxis];
	}
	else
	{
		m_Button_FilterSet.EnableWindow(FALSE);
		PUINT pPara = (PUINT)&filterPara;
		for(int i=0; i<8; i++)
		{
			pPara[i] = 0;
		}
	}
	WTNMC4A_ExtMode(			
		gl_pADView->m_hDevice,			// 设备句柄
		gl_pADView->m_nCurrentAxis,			// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)
		&filterPara);// 滤波器参数结构体指针	
}

void CPageHardLimit::OnBUTTONFilterSet() 
{
	// TODO: Add your control notification handler code here
	m_FilterSetDlg.DoModal();
}

void CPageHardLimit::SetCurrentAxisNum(int nCurrentAxis)
{

}
