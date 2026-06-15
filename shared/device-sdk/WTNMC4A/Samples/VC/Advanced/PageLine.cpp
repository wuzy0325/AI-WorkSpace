// PageLine.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "math.h"
#include "PageLine.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageLine property page

IMPLEMENT_DYNCREATE(CPageLine, CPropertyPage)

CPageLine::CPageLine() : CPropertyPage(CPageLine::IDD)
{
	//{{AFX_DATA_INIT(CPageLine)
	m_nCurrentAxis = 0; // X轴
	m_nFunction = 0;
	//}}AFX_DATA_INIT
}

CPageLine::~CPageLine()
{
}

void CPageLine::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageLine)
	DDX_Control(pDX, IDC_BUTTON_HomeSearchSet, m_Button_HomeSearchSet);
	DDX_Control(pDX, IDC_OutStartAll, m_Button_OutStartAll);
	DDX_Control(pDX, IDC_STOP_InstStopAll, m_Button_InstStopAll);
	DDX_Control(pDX, IDC_STOP_DecStopAll, m_Button_DecStopAll);
	DDX_Control(pDX, IDC_START_LINEALL, m_Button_StartLineAll);
	DDX_Control(pDX, IDC_STATIC_DVImpulseNum, m_Static_DVImpulseNum);
	DDX_Control(pDX, IDC_EDIT_DVImpulseNum, m_Edit_DVImpulseNum);
	DDX_Control(pDX, IDC_EDIT_SCurveHandDecNum, m_Edit_HandDecNum);
	DDX_Control(pDX, IDC_COMBO_LineDecType, m_Combo_DecType);
	DDX_Control(pDX, IDC_STATIC_AccIncRate, m_Static_AccIncRate);
	DDX_Control(pDX, IDC_STATIC_DecelerationK, m_Static_DecelerationK);
	DDX_Control(pDX, IDC_EDIT_DECACCK, m_Edit_DecelerationK);
	DDX_Control(pDX, IDC_EDIT_AccIncRate, m_Edit_AccIncRate);
	DDX_Control(pDX, IDC_COMBO_ImpulseType, m_Combo_ImpulseType);
	DDX_Control(pDX, IDC_COMBO_LineMode, m_Combo_LineMode);
	DDX_Control(pDX, IDC_OutStartX, m_Button_OutStartX);
	DDX_Control(pDX, IDC_OutStartY, m_Button_OutStartY);
	DDX_Control(pDX, IDC_OutStartZ, m_Button_OutStartZ);
	DDX_Control(pDX, IDC_OutStartU, m_Button_OutStartU);
	DDX_Control(pDX, IDC_START_LINEX, m_Button_StartLineX);
	DDX_Control(pDX, IDC_START_LINEY, m_Button_StartLineY);
	DDX_Control(pDX, IDC_START_LINEZ, m_Button_StartLineZ);
	DDX_Control(pDX, IDC_START_LINEU, m_Button_StartLineU);
	DDX_Control(pDX, IDC_STOP_InstStopX, m_Button_InstStopX);
	DDX_Control(pDX, IDC_STOP_InstStopY, m_Button_InstStopY);
	DDX_Control(pDX, IDC_STOP_InstStopZ, m_Button_InstStopZ);
	DDX_Control(pDX, IDC_STOP_InstStopU, m_Button_InstStopU);
	DDX_Control(pDX, IDC_STOP_DecStopX, m_Button_DecStopX);
	DDX_Control(pDX, IDC_STOP_DecStopY, m_Button_DecStopY);
	DDX_Control(pDX, IDC_STOP_DecStopZ, m_Button_DecStopZ);
	DDX_Control(pDX, IDC_STOP_DecStopU, m_Button_DecStopU);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageLine, CPropertyPage)
	//{{AFX_MSG_MAP(CPageLine)
	ON_BN_CLICKED(IDC_RADIO_Forward, OnRADIOForward)
	ON_BN_CLICKED(IDC_RADIO_Reverse, OnRADIOReverse)
	ON_CBN_SELCHANGE(IDC_COMBO_LineMode, OnSelchangeCOMBOLineMode)
	ON_CBN_SELCHANGE(IDC_COMBO_ImpulseType, OnSelchangeCOMBOImpulseType)
	ON_EN_KILLFOCUS(IDC_EDIT_DECACCK, OnKillfocusEditDecacck)
	ON_EN_KILLFOCUS(IDC_EDIT_AccIncRate, OnKillfocusEDITAccIncRate)
	ON_CBN_SELCHANGE(IDC_COMBO_LineDecType, OnSelchangeCOMBOLineDecType)
	ON_EN_CHANGE(IDC_EDIT_SCurveHandDecNum, OnChangeEDITSCurveHandDecNum)
	ON_EN_CHANGE(IDC_EDIT_DVImpulseNum, OnChangeEDITDVImpulseNum)
	ON_BN_CLICKED(IDC_START_LINEX, OnStartLineX)
	ON_BN_CLICKED(IDC_START_LINEY, OnStartLineY)
	ON_BN_CLICKED(IDC_START_LINEZ, OnStartLineZ)
	ON_BN_CLICKED(IDC_START_LINEU, OnStartLineU)
	ON_BN_CLICKED(IDC_OutStartX, OnOutStartX)
	ON_BN_CLICKED(IDC_OutStartY, OnOutStartY)
	ON_BN_CLICKED(IDC_OutStartZ, OnOutStartZ)
	ON_BN_CLICKED(IDC_OutStartU, OnOutStartU)
	ON_BN_CLICKED(IDC_STOP_DecStopX, OnSTOPDecStopX)
	ON_BN_CLICKED(IDC_STOP_DecStopY, OnSTOPDecStopY)
	ON_BN_CLICKED(IDC_STOP_DecStopZ, OnSTOPDecStopZ)
	ON_BN_CLICKED(IDC_STOP_DecStopU, OnSTOPDecStopU)
	ON_BN_CLICKED(IDC_STOP_InstStopX, OnSTOPInstStopX)
	ON_BN_CLICKED(IDC_STOP_InstStopY, OnSTOPInstStopY)
	ON_BN_CLICKED(IDC_STOP_InstStopZ, OnSTOPInstStopZ)
	ON_BN_CLICKED(IDC_STOP_InstStopU, OnSTOPInstStopU)
	ON_BN_CLICKED(IDC_START_LINEALL, OnStartLineall)
	ON_BN_CLICKED(IDC_OutStartAll, OnOutStartAll)
	ON_BN_CLICKED(IDC_STOP_DecStopAll, OnSTOPDecStopAll)
	ON_BN_CLICKED(IDC_STOP_InstStopAll, OnSTOPInstStopAll)
	ON_BN_CLICKED(IDC_BUTTON_HomeSearchSet, OnBUTTONHomeSearchSet)
	
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageLine message handlers
void CPageLine::EnableWindows(BOOL bEnable)
{	
	CButton* pButton1 = (CButton*)GetDlgItem(IDC_RADIO_Forward);
	CButton* pButton2 = (CButton*)GetDlgItem(IDC_RADIO_Reverse);
	pButton1->EnableWindow(bEnable);
	pButton2->EnableWindow(bEnable);
	m_Combo_ImpulseType.EnableWindow(bEnable);
	m_Combo_LineMode.EnableWindow(bEnable);
	if(m_Combo_LineMode.GetCurSel()==0) // 直线运动
	{

// 		m_Static_AccIncRate.EnableWindow(FALSE);
// 		m_Static_DecelerationK.EnableWindow(FALSE);
// 		m_Edit_AccIncRate.EnableWindow(FALSE);
// 		m_Edit_DecelerationK.EnableWindow(FALSE);
// 		m_Combo_DecType.SetCurSel(WTNMC4A_AUTO); // 只能是自动减速
// 		gl_pADView->m_LCData[m_nCurrentAxis].DecMode = WTNMC4A_AUTO;
// 		m_Combo_DecType.EnableWindow(FALSE);
// 		m_Edit_HandDecNum.EnableWindow(FALSE);

		m_Edit_AccIncRate.EnableWindow(FALSE);
		m_Edit_DecelerationK.EnableWindow(FALSE);
		if(m_Combo_DecType.GetCurSel() == 1) // 如果是手动减速
			m_Edit_HandDecNum.EnableWindow(TRUE);
		else // 自动减速
			m_Edit_HandDecNum.EnableWindow(FALSE);
	}
	else // S曲线运动
	{
// 		m_Static_AccIncRate.EnableWindow(TRUE);
// 		m_Static_DecelerationK.EnableWindow(TRUE);
// 		m_Edit_AccIncRate.EnableWindow(TRUE);
// 		m_Edit_DecelerationK.EnableWindow(TRUE);
// 		m_Combo_DecType.EnableWindow(TRUE);  // 可以选择手动减速
// 		if(m_Combo_DecType.GetCurSel() == 1) // 如果是手动减速
// 			m_Edit_HandDecNum.EnableWindow(TRUE);
// 		else // 自动减速
// 			m_Edit_HandDecNum.EnableWindow(FALSE);

		m_Edit_AccIncRate.EnableWindow(bEnable);
		m_Edit_DecelerationK.EnableWindow(bEnable);
		if(m_Combo_DecType.GetCurSel() == 1) // 如果是手动减速
			m_Edit_HandDecNum.EnableWindow(TRUE);
		else // 自动减速
			m_Edit_HandDecNum.EnableWindow(FALSE);
		
	}
//	m_Combo_Type.EnableWindow(bEnable);
	m_Button_StartLineX.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_XAXIS]);
	m_Button_StartLineY.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_YAXIS]);
	m_Button_StartLineZ.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS]);
	m_Button_StartLineU.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_UAXIS]);
	m_Button_StartLineAll.EnableWindow(!gl_pADView->m_bAllAxisRun);
	m_Button_OutStartX.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_XAXIS]);
	m_Button_OutStartY.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_YAXIS]);
	m_Button_OutStartZ.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS]);
	m_Button_OutStartU.EnableWindow(!gl_pADView->m_bAxisRun[WTNMC4A_UAXIS]);
	m_Button_OutStartAll.EnableWindow(!gl_pADView->m_bAllAxisRun);
	m_Button_DecStopX.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_XAXIS]);
	m_Button_DecStopY.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_YAXIS]);
	m_Button_DecStopZ.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS]);
	m_Button_DecStopU.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_UAXIS]);
	m_Button_DecStopAll.EnableWindow(gl_pADView->m_bAllAxisRun);
	m_Button_InstStopX.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_XAXIS]);
	m_Button_InstStopY.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_YAXIS]);
	m_Button_InstStopZ.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS]);
	m_Button_InstStopU.EnableWindow(gl_pADView->m_bAxisRun[WTNMC4A_UAXIS]);
	m_Button_InstStopAll.EnableWindow(gl_pADView->m_bAllAxisRun);
}

//轴号选择(X、Y、Z、U轴)
void CPageLine::OnSelchangeComboAxis() 
{
	// TODO: Add your control notification handler code here
//	m_nCurrentAxis = m_Combo_Axis.GetCurSel(); // 当前选择的轴号
	gl_pADView->m_LCData[m_nCurrentAxis].AxisNum = m_nCurrentAxis;
	
}


void CPageLine::OnRADIOForward() // 正方向运动
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_LCData[m_nCurrentAxis].Direction = 0x1; // 正方向
}

void CPageLine::OnRADIOReverse() // 反方向运动
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_LCData[m_nCurrentAxis].Direction = 0x0; // 反方向
}

// 运动方式(直线|S曲线)
void CPageLine::OnSelchangeCOMBOLineMode() 
{
	// TODO: Add your control notification handler code here
	int nLineMode = m_Combo_LineMode.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].Line_Curve = nLineMode;
	if(nLineMode == WTNMC4A_LINE) // 直线运动
	{
		m_Static_AccIncRate.EnableWindow(FALSE);
		m_Static_DecelerationK.EnableWindow(FALSE);
		m_Edit_AccIncRate.EnableWindow(FALSE);
		m_Edit_DecelerationK.EnableWindow(FALSE);
		m_Combo_DecType.SetCurSel(WTNMC4A_AUTO); // 只能是自动减速
		gl_pADView->m_LCData[m_nCurrentAxis].DecMode = WTNMC4A_AUTO;
		m_Combo_DecType.EnableWindow(FALSE);
		m_Edit_HandDecNum.EnableWindow(FALSE);
	}
	else // 曲线运动
	{
		m_Static_AccIncRate.EnableWindow(TRUE);
		m_Static_DecelerationK.EnableWindow(TRUE);
		m_Edit_AccIncRate.EnableWindow(TRUE);
		m_Edit_DecelerationK.EnableWindow(TRUE);
		m_Combo_DecType.EnableWindow(TRUE);  // 可以选择手动减速
		if(m_Combo_DecType.GetCurSel() == 1) // 如果是手动减速
			m_Edit_HandDecNum.EnableWindow(TRUE);
		else // 自动减速
			m_Edit_HandDecNum.EnableWindow(FALSE);
	} 
}

// 驱动方式(连续驱动|定长驱动)
void CPageLine::OnSelchangeCOMBOImpulseType() 
{
	// TODO: Add your control notification handler code here
	int nIndex = m_Combo_ImpulseType.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].LV_DV = nIndex;
 	if(nIndex == 0) // 定长方式
	{
		m_Static_DVImpulseNum.EnableWindow(TRUE);
		m_Edit_DVImpulseNum.EnableWindow(TRUE);
	}
	else
	{
		m_Static_DVImpulseNum.EnableWindow(FALSE);
		m_Edit_DVImpulseNum.EnableWindow(FALSE);
	}
}

BOOL CPageLine::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	CRect rect;
	m_PageSynchronSet.Create(IDD_Page_SynchronSet, this);
	GetDlgItem(IDC_STATIC_LineCom)->GetWindowRect(rect);
	rect.DeflateRect(3, 6);
	rect.OffsetRect(0, 4);
	ScreenToClient(rect);
	m_PageSynchronSet.MoveWindow(rect);
	
	// TODO: Add extra initialization here
	InitCfg(m_nCurrentAxis); // 初始化X轴的参数
	EnableSynchronWindows(FALSE);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CPageLine::InitCfg(int nCurrentAxis)
{
	m_nCurrentAxis = nCurrentAxis; // 初始化当前轴号
	CString str = gl_pADView->GetAxisString(nCurrentAxis);
	str += "轴 ";
	m_Button_HomeSearchSet.SetWindowText(str+"原点搜寻设置...");

	if(gl_pADView->m_LCData[m_nCurrentAxis].Direction == 1) // 正转
	{
		CButton* pButton1 = (CButton*)GetDlgItem(IDC_RADIO_Forward);
		CButton* pButton2 = (CButton*)GetDlgItem(IDC_RADIO_Reverse);
		pButton1->SetCheck(1);
		pButton2->SetCheck(0);
	}
	else // 反转
	{
		CButton* pButton1 = (CButton*)GetDlgItem(IDC_RADIO_Forward);
		CButton* pButton2 = (CButton*)GetDlgItem(IDC_RADIO_Reverse);
		pButton1->SetCheck(0);
		pButton2->SetCheck(1);
	}
	m_Combo_LineMode.SetCurSel(gl_pADView->m_LCData[m_nCurrentAxis].Line_Curve); 
	OnSelchangeCOMBOLineMode();   // 运动方式(直线|S曲线)
	m_Combo_ImpulseType.SetCurSel(gl_pADView->m_LCData[m_nCurrentAxis].LV_DV); 
	OnSelchangeCOMBOImpulseType();// 驱动方式(连续|定长)
	
	int nValue;
	nValue = gl_pADView->m_DataList[m_nCurrentAxis].AccIncRate;
	str.Format(L"%d", nValue);
	m_Edit_AccIncRate.SetWindowText(str); // 加速度变化率
	nValue = gl_pADView->m_DataList[m_nCurrentAxis].DecIncRate; 
	str.Format(L"%d", nValue);
	m_Edit_DecelerationK.SetWindowText(str); // 减速度变化率
	gl_pADView->m_LCData[m_nCurrentAxis].DecMode = 0;	// *************************
	nValue = gl_pADView->m_LCData[m_nCurrentAxis].DecMode;   // 减速方式(自动|手动)
	m_Combo_DecType.SetCurSel(nValue);
	nValue = gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum; // 定长脉冲长度
	str.Format(L"%d", nValue);
	m_Edit_DVImpulseNum.SetWindowText(str);

	if(m_Combo_DecType.GetCurSel() == 1) // 如果是手动减速
	{		
		nValue = gl_pADView->m_OtherPara[m_nCurrentAxis].HandDecPulse;//HandDecNum;
		str.Format(L"%d", nValue);
		m_Edit_HandDecNum.SetWindowText(str); // 手动减速点
	}

	nValue = gl_pADView->m_LCData[m_nCurrentAxis].PulseMode; // 输出脉冲方式
	if(gl_pADView->m_LCData[m_nCurrentAxis].LV_DV == WTNMC4A_LV) // 连续脉冲驱动方式
	{
		m_Edit_DVImpulseNum.EnableWindow(FALSE);
		m_Static_DVImpulseNum.EnableWindow(FALSE);
	}
	else
	{
		m_Edit_DVImpulseNum.EnableWindow(TRUE);
		m_Static_DVImpulseNum.EnableWindow(TRUE);
	}
		
	EnableWindows(TRUE);

}

// 改变减速度变化率
void CPageLine::OnKillfocusEditDecacck() 
{
	// TODO: Add your control notification handler code here
	int nDecelerationK;
	CString str;
	m_Edit_DecelerationK.GetWindowText(str);
	nDecelerationK = wcstol(str, NULL, 10);
	gl_pADView->m_DataList[m_nCurrentAxis].DecIncRate = nDecelerationK;	
}

// 改变加速度变化率
void CPageLine::OnKillfocusEDITAccIncRate() 
{
	// TODO: Add your control notification handler code here
	int nAccIncRate;
	CString str;
	m_Edit_AccIncRate.GetWindowText(str);
	if(str.Left(2) != "0x") // 十进制数
		nAccIncRate = wcstol(str, NULL, 10);
	else // 十六进制数
		nAccIncRate = wcstol(str, NULL, 16);
	gl_pADView->m_DataList[m_nCurrentAxis].AccIncRate = nAccIncRate;	
}

 
// 选择减速方式(自动|手动)
void CPageLine::OnSelchangeCOMBOLineDecType() 
{
	// TODO: Add your control notification handler code here
	CString str;
	int PulseNum, DriverSpeed, StartSpeed, DecIncRate, HandDecNum,Acceleration,Deceleration,Multiple;
	int nIndex = m_Combo_DecType.GetCurSel();
	gl_pADView->m_LCData[m_nCurrentAxis].DecMode = nIndex;           
	if(nIndex == WTNMC4A_HAND) // 手动
	{
		PulseNum = gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum;       // 定长脉冲数
		DriverSpeed = gl_pADView->m_DataList[m_nCurrentAxis].DriveSpeed; // 驱动速度
		StartSpeed = gl_pADView->m_DataList[m_nCurrentAxis].StartSpeed;  // 初始速度
		Acceleration = gl_pADView->m_DataList[m_nCurrentAxis].Acceleration;//加速度
		Deceleration = gl_pADView->m_DataList[m_nCurrentAxis].Deceleration;//减速度
		Multiple = gl_pADView->m_DataList[m_nCurrentAxis].Multiple;			//倍率

		DecIncRate = gl_pADView->m_DataList[m_nCurrentAxis].DecIncRate;  // 减速度变化率
		double dSqrt = sqrt((DriverSpeed-StartSpeed)/(DecIncRate*1.0)); 
		HandDecNum = (int)(PulseNum - (DriverSpeed+StartSpeed)*dSqrt);   // 手动减速点
		gl_pADView->m_OtherPara[m_nCurrentAxis].HandDecPulse = HandDecNum; // 设置手动减速点
		str.Format(L"%d", HandDecNum);
		m_Edit_HandDecNum.SetWindowText(str);
		WTNMC4A_HanDec(gl_pADView->m_hDevice, m_nCurrentAxis, HandDecNum);
		m_Edit_HandDecNum.EnableWindow(TRUE);
	}
	else // 自动减速
	{
		WTNMC4A_AutoDec(gl_pADView->m_hDevice, m_nCurrentAxis);
		m_Edit_HandDecNum.EnableWindow(FALSE);
	}
	::SendMessage(gl_pADView->m_PageComSet.m_hWnd, WM_LINEDECTYPE_CHANGE, nIndex, NULL);

}

void CPageLine::OnChangeEDITSCurveHandDecNum() 
{
	// TODO: Add your control notification handler code here
	int nHandDecNum;
	CString str;
	m_Edit_HandDecNum.GetWindowText(str);
	nHandDecNum = wcstol(str, NULL, 10);
	gl_pADView->m_OtherPara[m_nCurrentAxis].HandDecPulse = nHandDecNum;
	WTNMC4A_HanDec(gl_pADView->m_hDevice, m_nCurrentAxis, nHandDecNum);
	//TRACE("轴号 = %d,手动减速点 = %d",m_nCurrentAxis,nHandDecNum);
}

//DEL void CPageLine::OnBUTTONSynchronSet() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	gl_pADView->ShowSynchronSetPage();
//DEL }

//DEL void CPageLine::OnCHECKSynchronEnable() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	int nCheck = m_Check_SynchronEnable.GetCheck();
//DEL 	gl_pADView->m_bSynchronEnable = nCheck;
//DEL 	if(nCheck)
//DEL 	{
//DEL 		EnableSynchronWindows(TRUE);
//DEL 	}
//DEL 	else
//DEL 	{
//DEL 		EnableSynchronWindows(FALSE);
//DEL 	}
//DEL }

void CPageLine::OnChangeEDITDVImpulseNum() 
{
	// TODO: Add your control notification handler code here
	int nImpulseNum;
	CString str;
	m_Edit_DVImpulseNum.GetWindowText(str);
	nImpulseNum = wcstol(str, NULL, 10);
	gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum = nImpulseNum;
	if(gl_pADView->m_hDevice)
	{
	WTNMC4A_SetP( // 设置定长脉冲数
					gl_pADView->m_hDevice, // 设备句柄
					gl_pADView->m_LCData[m_nCurrentAxis].AxisNum, // 轴号(WTNMC4A_XAXIS:X轴; WTNMC4A_YAXIS:Y轴) 
					nImpulseNum);	
	}	
	
}

// 启动X轴直线(S曲线)运动或同步运动或原点搜寻
void CPageLine::OnStartLineX() 
{
	// TODO: Add your control notification handler code here
	StartFuncMovement(WTNMC4A_XAXIS);
	EnableWindows(FALSE);
}

void CPageLine::OnStartLineY() 
{
	// TODO: Add your control notification handler code here
	StartFuncMovement(WTNMC4A_YAXIS);
	EnableWindows(FALSE);
}

void CPageLine::OnStartLineZ() 
{
	// TODO: Add your control notification handler code here
	StartFuncMovement(WTNMC4A_ZAXIS);
	EnableWindows(FALSE);
}

void CPageLine::OnStartLineU() 
{
	// TODO: Add your control notification handler code here
	StartFuncMovement(WTNMC4A_UAXIS);
	EnableWindows(FALSE);
}

// 外部点动X轴
void CPageLine::OnOutStartX() 
{
	m_Button_StartLineX.EnableWindow(FALSE);
	m_Button_OutStartX.EnableWindow(FALSE);
	gl_pADView->OutStart(WTNMC4A_XAXIS);
}

void CPageLine::OnOutStartY()
{
	m_Button_StartLineY.EnableWindow(FALSE);
	m_Button_OutStartY.EnableWindow(FALSE);
	gl_pADView->OutStart(WTNMC4A_YAXIS);
}

void CPageLine::OnOutStartZ()
{
	m_Button_StartLineZ.EnableWindow(FALSE);
	m_Button_OutStartZ.EnableWindow(FALSE);
	gl_pADView->OutStart(WTNMC4A_ZAXIS);
}

void CPageLine::OnOutStartU()
{
	m_Button_StartLineU.EnableWindow(FALSE);
	m_Button_OutStartU.EnableWindow(FALSE);
	gl_pADView->OutStart(WTNMC4A_UAXIS);
}

// 减速停止X轴
void CPageLine::OnSTOPDecStopX() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->DecStop(WTNMC4A_XAXIS);
	if(gl_pADView->m_bAxisRun[WTNMC4A_XAXIS])
		m_Button_DecStopX.EnableWindow(FALSE);
}

void CPageLine::OnSTOPDecStopY()
{
	gl_pADView->DecStop(WTNMC4A_YAXIS);
	if(gl_pADView->m_bAxisRun[WTNMC4A_YAXIS])
		m_Button_DecStopY.EnableWindow(FALSE);
}

void CPageLine::OnSTOPDecStopZ()
{
	gl_pADView->DecStop(WTNMC4A_ZAXIS);
	if(gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS])
		m_Button_DecStopZ.EnableWindow(FALSE);
}

void CPageLine::OnSTOPDecStopU()
{
	gl_pADView->DecStop(WTNMC4A_UAXIS);
	if(gl_pADView->m_bAxisRun[WTNMC4A_UAXIS])
		m_Button_DecStopU.EnableWindow(FALSE);
}

// 立即停止
void CPageLine::OnSTOPInstStopX() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->ImmediateStop(WTNMC4A_XAXIS);
	gl_pADView->m_bAxisRun[WTNMC4A_XAXIS] = FALSE;
	m_Button_StartLineX.EnableWindow(TRUE);
	m_Button_OutStartX.EnableWindow(TRUE);
	m_Button_DecStopX.EnableWindow(FALSE);
	m_Button_InstStopX.EnableWindow(FALSE);
}

void CPageLine::OnSTOPInstStopY()
{
	gl_pADView->ImmediateStop(WTNMC4A_YAXIS);
	gl_pADView->m_bAxisRun[WTNMC4A_YAXIS] = FALSE;
	m_Button_StartLineY.EnableWindow(TRUE);
	m_Button_OutStartY.EnableWindow(TRUE);
	m_Button_DecStopY.EnableWindow(FALSE);
	m_Button_InstStopY.EnableWindow(FALSE);
}

void CPageLine::OnSTOPInstStopZ()
{
	gl_pADView->ImmediateStop(WTNMC4A_ZAXIS);
	gl_pADView->m_bAxisRun[WTNMC4A_ZAXIS] = FALSE;
	m_Button_StartLineZ.EnableWindow(TRUE);
	m_Button_OutStartZ.EnableWindow(TRUE);
	m_Button_DecStopZ.EnableWindow(FALSE);
	m_Button_InstStopZ.EnableWindow(FALSE);
}
void CPageLine::OnSTOPInstStopU()
{
	gl_pADView->ImmediateStop(WTNMC4A_UAXIS);
	gl_pADView->m_bAxisRun[WTNMC4A_UAXIS] = FALSE;
	m_Button_StartLineU.EnableWindow(TRUE);
	m_Button_OutStartU.EnableWindow(TRUE);
	m_Button_DecStopU.EnableWindow(FALSE);
	m_Button_InstStopU.EnableWindow(FALSE);
}



// 写同步操作命令
//DEL void CPageLine::OnBUTTONStartSynchronActionX() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	WTNMC4A_StartSynchronAction(gl_pADView->m_hDevice, WTNMC4A_XAXIS);	 	
//DEL }

//DEL void CPageLine::OnBUTTONStartSynchronActionY() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	WTNMC4A_StartSynchronAction(gl_pADView->m_hDevice, WTNMC4A_YAXIS);
//DEL }

//DEL void CPageLine::OnBUTTONStartSynchronActionZ() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	WTNMC4A_StartSynchronAction(gl_pADView->m_hDevice, WTNMC4A_ZAXIS);
//DEL }

//DEL void CPageLine::OnBUTTONStartSynchronActionU() 
//DEL {
//DEL 	// TODO: Add your control notification handler code here
//DEL 	WTNMC4A_StartSynchronAction(gl_pADView->m_hDevice, WTNMC4A_UAXIS);
//DEL }


void CPageLine::EnableSynchronWindows(BOOL bEnable)
{
//	m_Combo_Type.EnableWindow(!bEnable);
//	m_Combo_Type.SetCurSel(0);
//	OnSelchangeComboType();
	
//	m_Button_SynchronSet.EnableWindow(bEnable);
//	m_Button_StartSynchronActionX.EnableWindow(bEnable);
//	m_Button_StartSynchronActionY.EnableWindow(bEnable);
//	m_Button_StartSynchronActionZ.EnableWindow(bEnable);
//	m_Button_StartSynchronActionU.EnableWindow(bEnable);
//	m_Edit_COMPP.EnableWindow(bEnable);
//	m_Edit_COMPN.EnableWindow(bEnable);
	m_Button_StartLineAll.EnableWindow(!bEnable);
	m_Button_OutStartAll.EnableWindow(!bEnable);

}

// 同时启动四轴
void CPageLine::OnStartLineall() 
{
	gl_pADView->StartLineMovement(WTNMC4A_ALLAXIS);
}

// 同时外部点动四轴
void CPageLine::OnOutStartAll() 
{
	gl_pADView->OutStart(WTNMC4A_ALLAXIS);
	
}

// 减速停止四轴
void CPageLine::OnSTOPDecStopAll() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->DecStop(WTNMC4A_ALLAXIS);
}

// 立即停止四轴
void CPageLine::OnSTOPInstStopAll() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->ImmediateStop(WTNMC4A_ALLAXIS);
}

void CPageLine::OnBUTTONHomeSearchSet() 
{
	// TODO: Add your control notification handler code here
	m_HomeSearchDlg.DoModal();

	//Invalidate();
}

void CPageLine::ShowSynchronSetWnd(BOOL bShow)
{
	if(bShow)
		m_PageSynchronSet.ShowWindow(SW_SHOW);
	else
		m_PageSynchronSet.ShowWindow(SW_HIDE);
}

void CPageLine::SetCurrentAxisNum(int nCurrentAxis)
{

}

void CPageLine::SetFunction(int nFunction)
{
	m_nFunction = nFunction;
	switch(nFunction)
	{
	case LINECURVE_FUNC:
		m_Button_HomeSearchSet.ShowWindow(SW_HIDE);
		m_PageSynchronSet.ShowWindow(SW_HIDE);
		break;
	case SYNCHRON_FUNC:
		m_Button_HomeSearchSet.ShowWindow(SW_HIDE);
		m_PageSynchronSet.ShowWindow(SW_SHOW);
		break;
	case HOMESEARCH_FUNC:
		m_Button_HomeSearchSet.ShowWindow(SW_SHOW);
		m_PageSynchronSet.ShowWindow(SW_HIDE);
		break;
	default:
		break;
	}
}

void CPageLine::StartFuncMovement(int nAxisNum)
{
	switch(m_nFunction)
	{
	case LINECURVE_FUNC: // 直线|S曲线运动
		gl_pADView->StartLineMovement(nAxisNum);
		break;
	case SYNCHRON_FUNC:  // 同步运动
		gl_pADView->StartSynchronMovement(nAxisNum);
		break;
	case HOMESEARCH_FUNC: // 原点搜寻
		gl_pADView->StartAutoHomeSearch(nAxisNum);
		break;
	default:
		break;
	}
}

void CPageLine::OnButton1() 
{
	// TODO: Add your control notification handler code here
	CString str;
	LONG nFv = WTNMC4A_ReadEP(gl_pADView->m_hDevice, WTNMC4A_ZAXIS);  
	str.Format(L"%d", nFv);
	AfxMessageBox(str);

}
