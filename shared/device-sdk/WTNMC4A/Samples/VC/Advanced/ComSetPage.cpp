// ComSetPage.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "ComSetPage.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageComSet property page

IMPLEMENT_DYNCREATE(CPageComSet, CPropertyPage)

CPageComSet::CPageComSet() : CPropertyPage(CPageComSet::IDD)
{
	//{{AFX_DATA_INIT(CPageComSet)
	m_nCurrentAxis = 0;
	//}}AFX_DATA_INIT
}

CPageComSet::~CPageComSet()
{
}

void CPageComSet::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageComSet)
	DDX_Control(pDX, IDC_SPIN_Multiple, m_Spin_Multiple);
	DDX_Control(pDX, IDC_EDIT_Multiple, m_Edit_Multiple);
	DDX_Control(pDX, IDC_COMBO_ImpulseModeIN, m_Combo_ImpulseModeIN);
	DDX_Control(pDX, IDC_COMBO_DirLogLever, m_Combo_DirLogLever);
	DDX_Control(pDX, IDC_COMBO_PulseLogLever, m_Combo_PulseLogLever);
	DDX_Control(pDX, IDC_EDIT_AccOffset, m_Edit_AccOffset);
	DDX_Control(pDX, IDC_COMBO_ImpulseMode, m_Combo_ImpulseMode);
	DDX_Control(pDX, IDC_EDIT_StartRate, m_Edit_StartRate);
	DDX_Control(pDX, IDC_EDIT_DriveRate, m_Edit_DriveRate);
	DDX_Control(pDX, IDC_EDIT_DECACC, m_Edit_DecAcc);
	DDX_Control(pDX, IDC_EDIT_Acceleration, m_Edit_Acceleration);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageComSet, CPropertyPage)
	//{{AFX_MSG_MAP(CPageComSet)
	ON_EN_CHANGE(IDC_EDIT_StartRate, OnChangeEDITStartRate)
	ON_EN_CHANGE(IDC_EDIT_DriveRate, OnChangeEDITDriveRate)
	ON_CBN_SELCHANGE(IDC_COMBO_ImpulseMode, OnSelchangeCOMBOImpulseMode)
	ON_EN_CHANGE(IDC_EDIT_AccOffset, OnChangeEDITAccOffset)
	ON_BN_CLICKED(IDC_BUTTON_InterrruptSet, OnBUTTONInterrruptSet)
	ON_EN_CHANGE(IDC_EDIT_Acceleration, OnChangeEDITAcceleration)
	ON_EN_CHANGE(IDC_EDIT_DECACC, OnChangeEditDecacc)
	ON_CBN_SELCHANGE(IDC_COMBO_PulseLogLever, OnSelchangeCOMBOPulseLogLever)
	ON_CBN_SELCHANGE(IDC_COMBO_DirLogLever, OnSelchangeCOMBODirLogLever)
	ON_CBN_EDITCHANGE(IDC_COMBO_ImpulseModeIN, OnEditchangeCOMBOImpulseModeIN)
	ON_EN_CHANGE(IDC_EDIT_Multiple, OnChangeEDITMultiple)
	ON_MESSAGE(WM_LINEDECTYPE_CHANGE, SCurveDecTypeChange)
	ON_CBN_SELCHANGE(IDC_COMBO_ImpulseModeIN, OnSelchangeCOMBOImpulseModeIN)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageComSet message handlers
// 初始化当前轴的配置
void CPageComSet::InitCfg(int nAxisNum)
{
	LONG nValue;
	CString str;
	nValue = gl_pADView->m_DataList[nAxisNum].Multiple;     // 倍率
	str.Format(L"%d", nValue);
	m_Edit_Multiple.SetWindowText(str);

	nValue = gl_pADView->m_LCData[nAxisNum].PulseMode;      // 脉冲方式
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Combo_ImpulseMode.SetCurSel(nValue);  
	}  
	nValue = gl_pADView->m_lPulseModeIN[nAxisNum];     
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Combo_ImpulseModeIN.SetCurSel(nValue);  
 	}  
	WTNMC4A_PulseInputMode( gl_pADView->m_hDevice,	
							nAxisNum,	
							gl_pADView->m_lPulseModeIN[nAxisNum] ); 

	nValue = gl_pADView->m_LCData[nAxisNum].PLSLogLever;      // 脉冲方向
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Combo_PulseLogLever.SetCurSel(nValue);  
	}  
	nValue = gl_pADView->m_LCData[nAxisNum].DIRLogLever;      // 方向信号逻辑电平
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Combo_DirLogLever.SetCurSel(nValue);  
	}  
	nValue = gl_pADView->m_DataList[nAxisNum].StartSpeed;   // 初始速度
	str.Format(L"%d", nValue);
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Edit_StartRate.SetWindowText(str);
	}
	nValue = gl_pADView->m_DataList[nAxisNum].DriveSpeed;   // 驱动速度
	str.Format(L"%d", nValue);
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Edit_DriveRate.SetWindowText(str); 
	}	
	nValue = gl_pADView->m_DataList[nAxisNum].Acceleration; // 加速度
	str.Format(L"%d", nValue);
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Edit_Acceleration.SetWindowText(str);  
	}		
	nValue = gl_pADView->m_DataList[nAxisNum].Deceleration; // 减速度
	str.Format(L"%d", nValue);
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Edit_DecAcc.SetWindowText(str); 
	}

	
// 	nValue = gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum; // 定长脉冲数
// 	str.Format(L"%d", nValue);
// 	if (nAxisNum == m_nCurrentAxis)
// 	{
// 		m_Edit_DVImpulseNum.SetWindowText(str);
// 	}
	
	nValue = gl_pADView->m_OtherPara[m_nCurrentAxis].AccOffset; // 加速计数偏移点
	str.Format(L"%d", nValue);
	if (nAxisNum == m_nCurrentAxis)
	{
		m_Edit_AccOffset.SetWindowText(str);
	}
	
	gl_pADView->m_PageSoftLimit.m_Button_ClearLimit.EnableWindow(gl_pADView->m_bSLimit[m_nCurrentAxis]);
	gl_pADView->m_PageSoftLimit.m_Button_Slimit.EnableWindow(!gl_pADView->m_bSLimit[m_nCurrentAxis]);
	gl_pADView->m_PageHardLimit.m_Button_SetStopNum.EnableWindow(!gl_pADView->m_bStopNum[m_nCurrentAxis][gl_pADView->m_nStopNum]);
	gl_pADView->m_PageHardLimit.m_Button_ClearStopNum.EnableWindow(gl_pADView->m_bStopNum[m_nCurrentAxis][gl_pADView->m_nStopNum]);
	gl_pADView->m_PageHardLimit.m_Button_Alarm.EnableWindow(!gl_pADView->m_bAlarm[m_nCurrentAxis]);
	gl_pADView->m_PageHardLimit.m_Button_ClearAlarm.EnableWindow(gl_pADView->m_bAlarm[m_nCurrentAxis]);
	gl_pADView->m_PageHardLimit.m_Button_SetInPos.EnableWindow(!gl_pADView->m_bInPos[m_nCurrentAxis]);
	gl_pADView->m_PageHardLimit.m_Button_ClearInPos.EnableWindow(gl_pADView->m_bInPos[m_nCurrentAxis]);	
}

BOOL CPageComSet::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	// TODO: Add extra initialization here
//	m_Combo_AxisNum.SetCurSel(0);
	InitCfg(m_nCurrentAxis);
	m_Spin_Multiple.SetBuddy(&m_Edit_Multiple);
	m_Spin_Multiple.SetRange(1, 500);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

// 初始速度
void CPageComSet::OnChangeEDITStartRate() 
{
	// TODO: Add your control notification handler code here
	CString str;
	LONG nValue;
	m_Edit_StartRate.GetWindowText(str);
	nValue = wcstol(str, NULL, 10);
// 	if(nValue < 1) 
// 	{
// 		AfxMessageBox(L"初始速度只能在1～8000之间");
// 		m_Edit_StartRate.SetWindowText(L"1");
// 		nValue = 1;
// 	}
// 	if(nValue > 8000) 
// 	{
// 		AfxMessageBox(L"初始速度只能在1～8000之间");
// 		m_Edit_StartRate.SetWindowText(L"8000");
// 		nValue = 8000;
// 	}
	gl_pADView->m_DataList[m_nCurrentAxis].StartSpeed = nValue;
}

// 驱动速度
void CPageComSet::OnChangeEDITDriveRate() 
{
	// TODO: Add your control notification handler code here
	CString str;
	LONG nValue;
	m_Edit_DriveRate.GetWindowText(str);
	nValue = wcstol(str, NULL, 10);
	if(nValue < 1) 
	{
// 		AfxMessageBox(L"驱动速度只能在1～8000之间");
// 		m_Edit_DriveRate.SetWindowText(L"1");
// 		nValue = 1;
	}
// 	if(nValue > 8000) 
// 	{
// 		AfxMessageBox(L"驱动速度只能在1～8000之间");
// 		m_Edit_DriveRate.SetWindowText(L"8000");
// 		nValue = 8000;
// 	}
	gl_pADView->m_DataList[m_nCurrentAxis].DriveSpeed= nValue;
	WTNMC4A_SetV(gl_pADView->m_hDevice, m_nCurrentAxis, nValue);
}


// 改变当前的轴号
void CPageComSet::SetCurrentAxisNum(int nAxisNum)
{
	m_nCurrentAxis = nAxisNum;
}


void CPageComSet::EnableWindows(BOOL bEnable)
{
	m_Edit_Multiple.EnableWindow(bEnable);
	m_Edit_StartRate.EnableWindow(bEnable);
	m_Combo_ImpulseMode.EnableWindow(bEnable);
	m_Combo_ImpulseModeIN.EnableWindow(bEnable);
	m_Combo_PulseLogLever.EnableWindow(bEnable);
	m_Combo_DirLogLever.EnableWindow(bEnable);
// 	if(gl_pADView->m_LCData[m_nCurrentAxis].DecMode == WTNMC4A_AUTO) // 自动减速
// 	{
// 		m_Edit_Acceleration.EnableWindow(FALSE);
// 		m_Edit_DecAcc.EnableWindow(FALSE);
// 	}
// 	else
//	{
		m_Edit_DecAcc.EnableWindow(bEnable);
		m_Edit_Acceleration.EnableWindow(bEnable);
		
//	}
}

// 选择脉冲输出模式
void CPageComSet::OnSelchangeCOMBOImpulseMode() 
{
	// TODO: Add your control notification handler code here
	int nMode = m_Combo_ImpulseMode.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].PulseMode = nMode;	
	WTNMC4A_PulseOutMode(gl_pADView->m_hDevice, 
						m_nCurrentAxis, 
						gl_pADView->m_LCData[m_nCurrentAxis].PulseMode, 
						gl_pADView->m_LCData[m_nCurrentAxis].PLSLogLever, 
						gl_pADView->m_LCData[m_nCurrentAxis].DIRLogLever);
}

// 设置加速计数器偏移
void CPageComSet::OnChangeEDITAccOffset() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_AccOffset.GetWindowText(str);
	LONG nOffset = wcstol(str, NULL, 10);
	gl_pADView->m_OtherPara[m_nCurrentAxis].AccOffset = nOffset;
	WTNMC4A_SetAccofst(			 // 设置加速计数器偏移
		gl_pADView->m_hDevice,	 // 设备句柄
		m_nCurrentAxis,			 // 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		nOffset);				 // 偏移范围(0-65535)		
		
}

// S曲线运动时减速方式改变时的消息函数
LRESULT CPageComSet::SCurveDecTypeChange(WPARAM wParam, LPARAM lParam)
{
	CString str;
	LONG nValue;
//	if(wParam == WTNMC4A_AUTO) // 自动减速
//	{
		nValue = gl_pADView->m_DataList[m_nCurrentAxis].Acceleration; // 加速度
		str.Format(L"%d", nValue);
		m_Edit_Acceleration.SetWindowText(str); 
		nValue = gl_pADView->m_DataList[m_nCurrentAxis].Deceleration; // 减速度
		str.Format(L"%d", nValue);
		m_Edit_DecAcc.SetWindowText(str); 	
		m_Edit_Acceleration.EnableWindow(TRUE);
		m_Edit_DecAcc.EnableWindow(TRUE);
		return 1;
//	}
//	else // 手动减速
// 	{	
// 		m_Edit_Acceleration.SetWindowText(L"8000"); 	
// 		gl_pADView->m_DataList[m_nCurrentAxis].Acceleration = 8000; // 加速度
// 		m_Edit_DecAcc.SetWindowText(L"8000"); 
// 		gl_pADView->m_DataList[m_nCurrentAxis].Deceleration = 8000; // 减速度
// 		m_Edit_Acceleration.EnableWindow(FALSE);
// 		m_Edit_DecAcc.EnableWindow(FALSE);
// 	}
}

 

void CPageComSet::ShowDVImpulseNumWindow(BOOL bShow)
{
//	CStatic* pStatic = (CStatic*)GetDlgItem(IDC_STATIC_DVImpulseNum);
//	pStatic->ShowWindow(bShow);
//	pStatic = (CStatic*)GetDlgItem(IDC_STATIC_DVImpulseNumber);
//	pStatic->ShowWindow(bShow);
//	m_Edit_DVImpulseNum.ShowWindow(bShow);
}

void CPageComSet::OnBUTTONInterrruptSet() 
{
	// TODO: Add your control notification handler code here
	m_InterruptSetDlg.DoModal();
}

// 改变当前轴的加速度
void CPageComSet::OnChangeEDITAcceleration() 
{	
	// TODO: Add your control notification handler code here
	CString str;
	LONG nValue;
	m_Edit_Acceleration.GetWindowText(str);
	nValue = wcstol(str, NULL, 10);
	if(nValue < 125) 
	{
	//	AfxMessageBox(L"加速度只能在125～1000000之间");
	//	m_Edit_Acceleration.SetWindowText(L"125");
	//	nValue = 125;
	}
	if(nValue > 1000000) 
	{
	//	AfxMessageBox(L"加速度只能在125～1000000之间");
	//	m_Edit_Acceleration.SetWindowText(L"1000000");
	//	nValue = 1000000;
	}
	gl_pADView->m_DataList[m_nCurrentAxis].Acceleration = nValue;
}

// 改变减速度
void CPageComSet::OnChangeEditDecacc() 
{
	// TODO: Add your control notification handler code here
	CString str;
	LONG nValue;
	m_Edit_DecAcc.GetWindowText(str);
	nValue = wcstol(str, NULL, 10);
	if(nValue < 125) 
	{
//		AfxMessageBox(L"减速度只能在125～1000000之间");
//		m_Edit_DecAcc.SetWindowText(L"125");
//		nValue = 125;
	}
	if(nValue > 1000000) 
	{
//		AfxMessageBox(L"减速度只能在125～1000000之间");
//		m_Edit_DecAcc.SetWindowText(L"1000000");
//		nValue = 1000000;
	}
	gl_pADView->m_DataList[m_nCurrentAxis].Deceleration = nValue;		
}

void CPageComSet::OnSelchangeCOMBOPulseLogLever() 
{
	int iPulseLogLever = m_Combo_PulseLogLever.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].PLSLogLever = iPulseLogLever;	
	WTNMC4A_PulseOutMode(gl_pADView->m_hDevice, 
						m_nCurrentAxis, 
						gl_pADView->m_LCData[m_nCurrentAxis].PulseMode, 
						gl_pADView->m_LCData[m_nCurrentAxis].PLSLogLever, 
						gl_pADView->m_LCData[m_nCurrentAxis].DIRLogLever);
	WTNMC4A_PulseInputMode( gl_pADView->m_hDevice,	
						m_nCurrentAxis,	
						gl_pADView->m_lPulseModeIN[m_nCurrentAxis]  );   
					


	
}

void CPageComSet::OnSelchangeCOMBODirLogLever() 
{
	int iDirLogLever = m_Combo_DirLogLever.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].DIRLogLever = iDirLogLever;	
	WTNMC4A_PulseOutMode(gl_pADView->m_hDevice, 
						m_nCurrentAxis, 
						gl_pADView->m_LCData[m_nCurrentAxis].PulseMode, 
						gl_pADView->m_LCData[m_nCurrentAxis].PLSLogLever, 
						gl_pADView->m_LCData[m_nCurrentAxis].DIRLogLever);
	WTNMC4A_PulseInputMode( gl_pADView->m_hDevice,	
						m_nCurrentAxis,	
						gl_pADView->m_lPulseModeIN[m_nCurrentAxis] );  
	
}
//选择脉冲输入模式
void CPageComSet::OnEditchangeCOMBOImpulseModeIN() 
{
	// TODO: Add your control notification handler code here
// 	int nModeIN = m_Combo_ImpulseMode.GetCurSel();
// 	gl_pADView->m_LCData[m_nCurrentAxis].PulseMode = nModeIN;	
// 	WTNMC4A_PulseInputMode( gl_pADView->m_hDevice,	
// 							m_nCurrentAxis,	
// 							gl_pADView->m_LCData[m_nCurrentAxis].PulseMode );   // 设置脉冲输入模式
	
}
// 倍率
void CPageComSet::OnChangeEDITMultiple() 
{
	// TODO: If this is a RICHEDIT control, the control will not
	// send this notification unless you override the CPropertyPage::OnInitDialog()
	// function and call CRichEditCtrl().SetEventMask()
	// with the ENM_CHANGE flag ORed into the mask.
	
	// TODO: Add your control notification handler code here
	CString str;
	int nValue;
	
	if(IsWindow(m_Edit_Multiple))
	{
		m_Edit_Multiple.GetWindowText(str);
		nValue = wcstol(str, NULL, 10);
		if(nValue < 1) 
		{
			AfxMessageBox(L"倍率只能在1～500之间");
			m_Edit_Multiple.SetWindowText(L"1");
			nValue = 1;
		}
		if(nValue > 500) 
		{
			AfxMessageBox(L"倍率只能在1～500之间");
			m_Edit_Multiple.SetWindowText(L"500");
			nValue = 500;
		}
		gl_pADView->m_DataList[m_nCurrentAxis].Multiple = nValue;
	}
	
}

void CPageComSet::OnSelchangeCOMBOImpulseModeIN() 
{
	// TODO: Add your control notification handler code here
	int nModeIN = m_Combo_ImpulseModeIN.GetCurSel();
	gl_pADView->m_lPulseModeIN[m_nCurrentAxis] = nModeIN;	
	WTNMC4A_PulseInputMode( gl_pADView->m_hDevice,	
							m_nCurrentAxis,	
							gl_pADView->m_lPulseModeIN[m_nCurrentAxis] );  
	
}
