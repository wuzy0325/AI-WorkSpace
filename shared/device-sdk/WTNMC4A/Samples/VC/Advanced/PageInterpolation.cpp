// PageInterpolation.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageInterpolation.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;

BOOL gl_bInstStop = FALSE;
/////////////////////////////////////////////////////////////////////////////
// CPageInterpolation property page

IMPLEMENT_DYNCREATE(CPageInterpolation, CPropertyPage)

CPageInterpolation::CPageInterpolation() : CPropertyPage(CPageInterpolation::IDD)
{
	//{{AFX_DATA_INIT(CPageInterpolation)
	m_bSingleStep = FALSE;
	m_FirstAxisNum = -1;
	//}}AFX_DATA_INIT
}

CPageInterpolation::~CPageInterpolation()
{
}

void CPageInterpolation::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageInterpolation)
	DDX_Control(pDX, IDC_BUTTON_StartSingleStepInterp, m_Button_StartSingleStepInterp);
	DDX_Control(pDX, IDC_CHECK_SingleStepInterp, m_Button_SingleSetpInterp);
	DDX_Control(pDX, IDC_STOP_InstStop, m_Button_InstStop);
	DDX_Control(pDX, IDC_STOP_DecStop, m_Button_DecStop);
	DDX_Control(pDX, IDC_START_Interpolation, m_Button_StartInterpolation);
	DDX_Control(pDX, IDC_EDIT_HandDecNum, m_Edit_HandDecNum);
	DDX_Control(pDX, IDC_STATIC_HandDecNum, m_Static_HandDecNum);
	DDX_Control(pDX, IDC_STATIC_AccK, m_Static_AccK);
	DDX_Control(pDX, IDC_CHECK_Constand, m_Check_Constand);
	DDX_Control(pDX, IDC_Edit_AccK, m_Edit_AccK);
	DDX_Control(pDX, IDC_STATIC_ThirdPulseNum, m_Static_ThirdPulseNum);
	DDX_Control(pDX, IDC_ThirdAxisPulseNum, m_Edit_ThirdAxisPulseNum);
	DDX_Control(pDX, IDC_STATIC_ShortAxisCenter, m_Static_ShortAxisCenter);
	DDX_Control(pDX, IDC_STATIC_LongAxisCenter, m_Static_LongAxisCenter);
	DDX_Control(pDX, IDC_EDIT_YCenter, m_Edit_ShortAxisCenter);
	DDX_Control(pDX, IDC_EDIT_XCenter, m_Edit_LongAxisCenter);
	DDX_Control(pDX, IDC_ShortAxisPulseNum, m_Edit_ShortAxisPulseNUm);
	DDX_Control(pDX, IDC_LongAxisPulseNum, m_Edit_LongAxisPulseNum);
	DDX_Control(pDX, IDC_STATIC_ThirdAxis, m_Static_ThirdAxis);
	DDX_Control(pDX, IDC_COMBO_ThirdAxisNum, m_Combo_ThirdAxisNum);
	DDX_Control(pDX, IDC_COMBO_SecondAxisNum, m_Combo_SecondAxisNum);
	DDX_Control(pDX, IDC_COMBO_FirstAxisNum, m_Combo_FirstAxisNum);
	DDX_Control(pDX, IDC_DECTYPE, m_Combo_DecType);
	DDX_Control(pDX, IDC_CHECK_LineCurve, m_Check_LineCurve);
	DDX_Control(pDX, IDC_COMBO_InterMode, m_Combo_InterMode);
	DDX_CBIndex(pDX, IDC_COMBO_FirstAxisNum, m_FirstAxisNum);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageInterpolation, CPropertyPage)
	//{{AFX_MSG_MAP(CPageInterpolation)
	ON_CBN_SELCHANGE(IDC_COMBO_InterMode, OnSelchangeCOMBOInterMode)
	ON_BN_CLICKED(IDC_START_Interpolation, OnSTARTInterpolation)
	ON_BN_CLICKED(IDC_CHECK_LineCurve, OnCHECKLineCurve)
	ON_CBN_SELCHANGE(IDC_COMBO_FirstAxisNum, OnSelchangeCOMBOFirstAxisNum)
	ON_CBN_SELCHANGE(IDC_COMBO_SecondAxisNum, OnSelchangeCOMBOSecondAxisNum)
	ON_CBN_SELCHANGE(IDC_COMBO_ThirdAxisNum, OnSelchangeCOMBOThirdAxisNum)
	ON_EN_CHANGE(IDC_LongAxisPulseNum, OnChangeLongAxisPulseNum)
	ON_EN_CHANGE(IDC_ShortAxisPulseNum, OnChangeShortAxisPulseNum)
	ON_EN_CHANGE(IDC_EDIT_XCenter, OnChangeEDITXCenter)
	ON_EN_CHANGE(IDC_EDIT_YCenter, OnChangeEDITYCenter)
	ON_EN_CHANGE(IDC_ThirdAxisPulseNum, OnChangeThirdAxisPulseNum)
	ON_BN_CLICKED(IDC_STOP_DecStop, OnSTOPDecStop)
	ON_BN_CLICKED(IDC_STOP_InstStop, OnSTOPInstStop)
	ON_BN_CLICKED(IDC_CHECK_Constand, OnCHECKConstand)
	ON_CBN_SELCHANGE(IDC_DECTYPE, OnSelchangeDectype)
	ON_EN_CHANGE(IDC_Edit_AccK, OnChangeEditAccK)
	ON_EN_CHANGE(IDC_EDIT_HandDecNum, OnChangeEDITHandDecNum)
	ON_BN_CLICKED(IDC_CHECK_SingleStepInterp, OnCHECKSingleStepInterp)
	ON_BN_CLICKED(IDC_RADIO_SingleStepInterpCom, OnRADIOSingleStepInterpCom)
	ON_BN_CLICKED(IDC_RADIO_SingleStepInterpExt, OnRADIOSingleStepInterpExt)
	ON_BN_CLICKED(IDC_BUTTON_StartSingleStepInterp, OnBUTTONStartSingleStepInterp)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageInterpolation message handlers
void CPageInterpolation::EnableWindows(BOOL bEnable)
{	
	m_Combo_InterMode.EnableWindow(bEnable);
	m_Check_LineCurve.EnableWindow(bEnable);
	m_Check_Constand.EnableWindow(bEnable);
	m_Combo_FirstAxisNum.EnableWindow(bEnable);
	m_Combo_SecondAxisNum.EnableWindow(bEnable);
	if(m_Combo_InterMode.GetCurSel() != 1) // 不是三轴直线插补
	{
		m_Combo_ThirdAxisNum.EnableWindow(FALSE);
	}
	else
	{
		m_Combo_ThirdAxisNum.EnableWindow(bEnable);
	}
	if(m_Check_LineCurve.GetCheck()) // S曲线运动
		m_Edit_AccK.EnableWindow(bEnable);
	
	if(m_Combo_InterMode.GetCurSel() < 2) // 直线插补
	{
		m_Static_HandDecNum.EnableWindow(FALSE);
		m_Edit_HandDecNum.EnableWindow(FALSE);
	}
	else
	{
		m_Static_HandDecNum.EnableWindow(TRUE);
		m_Edit_HandDecNum.EnableWindow(bEnable);
	}
	m_Combo_DecType.EnableWindow(FALSE); // 减速方式不可选
	m_Edit_LongAxisPulseNum.EnableWindow(bEnable);
	m_Edit_ShortAxisPulseNUm.EnableWindow(bEnable);
	m_Button_StartInterpolation.EnableWindow(bEnable);
	m_Button_DecStop.EnableWindow(!bEnable);
	m_Button_InstStop.EnableWindow(!bEnable);

}

void CPageInterpolation::OnSelchangeCOMBOInterMode() 
{
	// TODO: Add your control notification handler code here
	int nIndex = m_Combo_InterMode.GetCurSel();
	switch(nIndex)
	{
	case 0: // 两轴直线插补
		m_Check_LineCurve.EnableWindow(TRUE);
		m_Combo_ThirdAxisNum.EnableWindow(FALSE);
		m_Static_ThirdAxis.EnableWindow(FALSE);
		m_Static_LongAxisCenter.EnableWindow(FALSE);
		m_Edit_LongAxisCenter.EnableWindow(FALSE);
		m_Static_ShortAxisCenter.EnableWindow(FALSE);
		m_Edit_ShortAxisCenter.EnableWindow(FALSE);
		m_Edit_HandDecNum.EnableWindow(FALSE);
		m_Static_HandDecNum.EnableWindow(FALSE);
		m_Combo_DecType.SetCurSel(0);
		gl_pADView->m_nFunction = 1;
		break;
	case 1: // 三轴直线插补
		m_Check_LineCurve.EnableWindow(TRUE);
		m_Combo_ThirdAxisNum.EnableWindow(TRUE);
		m_Static_ThirdAxis.EnableWindow(TRUE);
		m_Static_LongAxisCenter.EnableWindow(FALSE);
		m_Edit_LongAxisCenter.EnableWindow(FALSE);
		m_Static_ShortAxisCenter.EnableWindow(FALSE);
		m_Edit_ShortAxisCenter.EnableWindow(FALSE);
		m_Static_HandDecNum.EnableWindow(FALSE);
		m_Edit_HandDecNum.EnableWindow(FALSE);
		m_Combo_DecType.SetCurSel(0);
		gl_pADView->m_nFunction = 1;
		break;
	case 2: // 正方向圆弧插补
		m_Check_LineCurve.EnableWindow(FALSE);
		m_Combo_ThirdAxisNum.EnableWindow(FALSE);
		m_Static_ThirdAxis.EnableWindow(FALSE);
		m_Static_LongAxisCenter.EnableWindow(TRUE);
		m_Edit_LongAxisCenter.EnableWindow(TRUE);
		m_Static_ShortAxisCenter.EnableWindow(TRUE);
		m_Edit_ShortAxisCenter.EnableWindow(TRUE);
		m_Static_HandDecNum.EnableWindow(TRUE);
		m_Edit_HandDecNum.EnableWindow(TRUE);
		m_Combo_DecType.SetCurSel(1);
		gl_pADView->m_nFunction = 2;
		break;
	case 3: // 反方向圆弧插补
		m_Check_LineCurve.EnableWindow(FALSE);
		m_Combo_ThirdAxisNum.EnableWindow(FALSE);
		m_Static_ThirdAxis.EnableWindow(FALSE);
		m_Static_LongAxisCenter.EnableWindow(TRUE);
		m_Edit_LongAxisCenter.EnableWindow(TRUE);
		m_Static_ShortAxisCenter.EnableWindow(TRUE);
		m_Edit_ShortAxisCenter.EnableWindow(TRUE);
		m_Static_HandDecNum.EnableWindow(TRUE);
		m_Edit_HandDecNum.EnableWindow(TRUE);
		m_Combo_DecType.SetCurSel(1);
		gl_pADView->m_nFunction = 2;
		break;
	default:
		break;
	}
	InitCfg(); // 重新初始化设置
	if(gl_pADView->m_nFunction == 1) // 直线插补方式
		::PostMessage(gl_pADView->m_hWnd, WM_DRAW_LINEINTERPOLATION, NULL, NULL);
	if(gl_pADView->m_nFunction == 2) // 圆弧插补方式
        ::PostMessage(gl_pADView->m_hWnd, WM_DRAW_CIRCLE, NULL, NULL);

}

void CPageInterpolation::OnAkordecnum() 
{
	// TODO: Add your control notification handler code here
	int nFlag = m_Check_LineCurve.GetCheck();

}


BOOL CPageInterpolation::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	// TODO: Add extra initialization here
	m_Combo_InterMode.SetCurSel(0); // 两轴直线插补
	OnSelchangeCOMBOInterMode();
	m_Combo_DecType.SetCurSel(0); 
	m_Combo_FirstAxisNum.SetCurSel(0);
	m_Combo_DecType.SetCurSel(0);
	OnSelchangeDectype();
	EnableWindows(TRUE);

	InitCfg();

	CButton* pButtonCom = (CButton*)GetDlgItem(IDC_RADIO_SingleStepInterpCom);
	CButton* pButtonExt = (CButton*)GetDlgItem(IDC_RADIO_SingleStepInterpExt);
	pButtonCom->SetCheck(1);
	pButtonCom->EnableWindow(FALSE);
	pButtonExt->EnableWindow(FALSE);
	m_Button_StartSingleStepInterp.EnableWindow(FALSE);
	m_Combo_FirstAxisNum.SetCurSel(gl_pADView->m_InterpAxis.Axis1);
	m_Combo_SecondAxisNum.SetCurSel(gl_pADView->m_InterpAxis.Axis2);
	m_Combo_ThirdAxisNum.SetCurSel(gl_pADView->m_InterpAxis.Axis3);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CPageInterpolation::OnSTARTInterpolation() 
{
	// TODO: Add your control notification handler code here
	if(m_Combo_InterMode.GetCurSel() == 1) // 三轴直线插补
	{
		if(gl_pADView->m_InterpAxis.Axis1 == gl_pADView->m_InterpAxis.Axis2 ||
			gl_pADView->m_InterpAxis.Axis1 == gl_pADView->m_InterpAxis.Axis3 || 
			gl_pADView->m_InterpAxis.Axis2 == gl_pADView->m_InterpAxis.Axis3)
		{
			AfxMessageBox(L"三个轴中存在两相同的轴号，请重新选择!");
			return;
		}
	}
	else
	{
		if(gl_pADView->m_InterpAxis.Axis1 == gl_pADView->m_InterpAxis.Axis2)
		{
			AfxMessageBox(L"两个轴中的轴号相同，请重新选择");
			return;
		}
	}

	switch(m_Combo_InterMode.GetCurSel())
	{
	case 0: // 两轴直线插补
		gl_pADView->StartLineInterpMovement(2);
		break;
	case 1: // 三轴直线插补
		gl_pADView->StartLineInterpMovement(3);
		break;
	case 2: // 两轴正方向圆弧插补
		gl_pADView->StartCircleInterpMovement(0);
		break;
	case 3: // 两轴反方向圆弧插补
		gl_pADView->StartCircleInterpMovement(1);
		break;
	default:
		break;
	}
 	m_Button_StartSingleStepInterp.EnableWindow(m_bSingleStep);
	EnableWindows(FALSE);
}

// 选择是直线还是S曲线
void CPageInterpolation::OnCHECKLineCurve() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_LineData.Line_Curve = m_Check_LineCurve.GetCheck();
	if(gl_pADView->m_LineData.Line_Curve) // 如果是S曲线
	{
		m_Edit_AccK.EnableWindow(TRUE);
	}
	else
	{
		m_Edit_AccK.EnableWindow(FALSE);
	}
}

// 选择主轴
void CPageInterpolation::OnSelchangeCOMBOFirstAxisNum() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_InterpAxis.Axis1 = m_Combo_FirstAxisNum.GetCurSel(); 
	gl_pADView->m_TabComSet.SetCurSel(gl_pADView->m_InterpAxis.Axis1);
	gl_pADView->m_PageComSet.SetCurrentAxisNum(gl_pADView->m_InterpAxis.Axis1); // 设置公用参数页的当前轴号
	gl_pADView->m_PageComSet.InitCfg(gl_pADView->m_InterpAxis.Axis1);	        // 初始化对应的轴号的参数
	gl_pADView->m_PageLine.InitCfg(gl_pADView->m_InterpAxis.Axis1);             // 初始化直线运动参数
	if(m_Combo_InterMode.GetCurSel() >= 2)
		::PostMessage(gl_pADView->m_hWnd, WM_DRAW_CIRCLE, NULL, NULL);
}

// 选择第二轴
void CPageInterpolation::OnSelchangeCOMBOSecondAxisNum() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_InterpAxis.Axis2 = m_Combo_SecondAxisNum.GetCurSel();
	if(m_Combo_InterMode.GetCurSel() >= 2)
		::PostMessage(gl_pADView->m_hWnd, WM_DRAW_CIRCLE, NULL, NULL);
}

// 选择第三轴
void CPageInterpolation::OnSelchangeCOMBOThirdAxisNum()
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_InterpAxis.Axis3 = m_Combo_ThirdAxisNum.GetCurSel();
}

void CPageInterpolation::InitCfg()
{
	CString str;
	if(m_Combo_InterMode.GetCurSel() < 2) // 两轴(三轴)直线插补
	{
		m_Check_LineCurve.SetCheck(gl_pADView->m_LineData.Line_Curve); // 直线|S曲线
		m_Check_Constand.SetCheck(gl_pADView->m_LineData.ConstantSpeed); // 固定线速度
		if(gl_pADView->m_LineData.Line_Curve) // 如果是S曲线
		{
			m_Edit_AccK.EnableWindow(TRUE);
		}
		else
		{
			m_Edit_AccK.EnableWindow(FALSE);
		}
		str.Format(L"%d", gl_pADView->m_LineData.n1AxisPulseNum); // 主轴插补脉冲数
		m_Edit_LongAxisPulseNum.SetWindowText(str);
		str.Format(L"%d", gl_pADView->m_LineData.n2AxisPulseNum); // 第二轴插补脉冲数
		m_Edit_ShortAxisPulseNUm.SetWindowText(str);
		str.Format(L"%d", gl_pADView->m_LineData.n3AxisPulseNum); // 第三轴插补脉冲数
		m_Edit_ThirdAxisPulseNum.SetWindowText(str);
		str.Format(L"%d", gl_pADView->m_DataList[gl_pADView->m_InterpAxis.Axis1].AccIncRate);
		m_Edit_AccK.SetWindowText(str); // 设置主轴的加速度变化率
		if(m_Combo_InterMode.GetCurSel() == 1)
		{
			m_Static_ThirdPulseNum.EnableWindow(TRUE);
			m_Edit_ThirdAxisPulseNum.EnableWindow(TRUE);
		}
		else
		{
			m_Static_ThirdPulseNum.EnableWindow(FALSE);
			m_Edit_ThirdAxisPulseNum.EnableWindow(FALSE);
		}
		
	}
	else // 圆弧插补
	{
		m_Static_ThirdPulseNum.EnableWindow(FALSE);
		m_Edit_ThirdAxisPulseNum.EnableWindow(FALSE);
		m_Check_Constand.SetCheck(gl_pADView->m_CircleData.ConstantSpeed); // 固定线速度
		str.Format(L"%d", gl_pADView->m_CircleData.Pulse1); // X轴的终点坐标
		m_Edit_LongAxisPulseNum.SetWindowText(str);
		str.Format(L"%d", gl_pADView->m_CircleData.Pulse2); // Y轴的终点坐标
		m_Edit_ShortAxisPulseNUm.SetWindowText(str);
		str.Format(L"%d", gl_pADView->m_CircleData.Center1); // X轴圆心坐标
		m_Edit_LongAxisCenter.SetWindowText(str);           
		str.Format(L"%d", gl_pADView->m_CircleData.Center2); // Y轴圆心坐标
		m_Edit_ShortAxisCenter.SetWindowText(str);   
        str.Format(L"%d", gl_pADView->m_OtherPara[gl_pADView->m_InterpAxis.Axis1].HandDecPulse);
		m_Edit_HandDecNum.SetWindowText(str);
	}
}

// 设置主轴的终点脉冲数
void CPageInterpolation::OnChangeLongAxisPulseNum() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_LongAxisPulseNum.GetWindowText(str);
	if(m_Combo_InterMode.GetCurSel() < 2) // 如果是直线插补
		gl_pADView->m_LineData.n1AxisPulseNum = wcstol(str, NULL, 10);
	else // 如果是圆弧插补
		gl_pADView->m_CircleData.Pulse1 = wcstol(str, NULL, 10);
	if(gl_pADView->m_nFunction == 1)
		::PostMessage(gl_pADView->m_hWnd, WM_DRAW_LINEINTERPOLATION, NULL, NULL);
	if(gl_pADView->m_nFunction == 2)
        ::PostMessage(gl_pADView->m_hWnd, WM_DRAW_CIRCLE, NULL, NULL);
}

// 设置第二轴的终点脉冲数
void CPageInterpolation::OnChangeShortAxisPulseNum() 
{	
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_ShortAxisPulseNUm.GetWindowText(str);
	if(m_Combo_InterMode.GetCurSel() < 2) // 如果是直线插补
		gl_pADView->m_LineData.n2AxisPulseNum = wcstol(str, NULL, 10);
	else // 如果是圆弧插补
		gl_pADView->m_CircleData.Pulse2 = wcstol(str, NULL, 10);
	if(gl_pADView->m_nFunction == 1)
		::PostMessage(gl_pADView->m_hWnd, WM_DRAW_LINEINTERPOLATION, NULL, NULL);
	if(gl_pADView->m_nFunction == 2)
        ::PostMessage(gl_pADView->m_hWnd, WM_DRAW_CIRCLE, NULL, NULL);
}


// 第三轴的终点脉冲数
void CPageInterpolation::OnChangeThirdAxisPulseNum() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_ThirdAxisPulseNum.GetWindowText(str);
	gl_pADView->m_LineData.n3AxisPulseNum = wcstol(str, NULL, 10);
}

// 长轴的圆心脉冲数
void CPageInterpolation::OnChangeEDITXCenter() 
{	
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_LongAxisCenter.GetWindowText(str);
	gl_pADView->m_CircleData.Center1 = wcstol(str, NULL, 10);
}

// 第二轴的圆心脉冲数
void CPageInterpolation::OnChangeEDITYCenter() 
{	
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_ShortAxisCenter.GetWindowText(str);
	gl_pADView->m_CircleData.Center2 = wcstol(str, NULL, 10);
}


void CPageInterpolation::OnSTOPDecStop() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->DecStop(WTNMC4A_ALLAXIS);
	m_Button_DecStop.EnableWindow(FALSE);
	//EnableWindows(TRUE);
}

void CPageInterpolation::OnSTOPInstStop() 
{
	// TODO: Add your control notification handler code here
	gl_bInstStop = TRUE;
	gl_pADView->ImmediateStop(WTNMC4A_ALLAXIS);
	m_Button_StartSingleStepInterp.EnableWindow(FALSE);
	EnableWindows(TRUE);
	
}

void CPageInterpolation::OnCHECKConstand() 
{
	// TODO: Add your control notification handler code here
	if(m_Combo_InterMode.GetCurSel() < 2) // 直线插补
		gl_pADView->m_LineData.ConstantSpeed = m_Check_Constand.GetCheck();
	else
		gl_pADView->m_CircleData.ConstantSpeed = m_Check_Constand.GetCheck();
}

void CPageInterpolation::OnSelchangeDectype() 
{
	// TODO: Add your control notification handler code here
/*	int nIndex = m_Combo_DecType.GetCurSel();
	if(nIndex == 0)
	{
		m_Static_HandDecNum.EnableWindow(FALSE);
		m_Edit_HandDecNum.EnableWindow(FALSE);
	}
	else
	{
		m_Static_HandDecNum.EnableWindow(TRUE);
		m_Edit_HandDecNum.EnableWindow(TRUE);
	}
	*/
//	if(nIndex == 0) // 自动减速
//	{
//		m_Static_AkOrDecNum.SetWindowText(L"加速度变化率");
//	}
//	else // 手动减速
//	{
//		m_Static_AkOrDecNum.SetWindowText(L"手动减速点");
//	}
}

void CPageInterpolation::OnChangeEditAccK() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_AccK.GetWindowText(str);
	gl_pADView->m_DataList[gl_pADView->m_InterpAxis.Axis1].AccIncRate = wcstol(str, NULL, 10);
}

void CPageInterpolation::OnChangeEDITHandDecNum() 
{	
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_HandDecNum.GetWindowText(str);
	gl_pADView->m_OtherPara[gl_pADView->m_InterpAxis.Axis1].HandDecPulse = wcstol(str, NULL, 10);
}

 

void CPageInterpolation::OnCHECKSingleStepInterp() 
{
	// TODO: Add your control notification handler code here
	m_bSingleStep = m_Button_SingleSetpInterp.GetCheck();
	if (m_bSingleStep)
	{
		m_Check_LineCurve.EnableWindow(FALSE);
		m_Edit_AccK.EnableWindow(FALSE);
	}
	else
	{
		m_Check_LineCurve.EnableWindow(TRUE);
		if(gl_pADView->m_LineData.Line_Curve) // 如果是S曲线
		{
			m_Edit_AccK.EnableWindow(TRUE);
		}
		else
		{
			m_Edit_AccK.EnableWindow(FALSE);
		}
	//	m_Edit_AccK.EnableWindow(TRUE);
	}
	CButton* pButtonCom = (CButton*)GetDlgItem(IDC_RADIO_SingleStepInterpCom);
	CButton* pButtonExt = (CButton*)GetDlgItem(IDC_RADIO_SingleStepInterpExt);
	pButtonCom->EnableWindow(m_bSingleStep);
	pButtonExt->EnableWindow(m_bSingleStep);
//	m_Button_StartSingleStepInterp.EnableWindow(m_bSingleStep);
//	m_Button_StartInterpolation.EnableWindow(!m_bSingleStep);
//	m_Button_StartSingleStepInterp.EnableWindow(m_bSingleStep);
	if(m_bSingleStep)
	{
		if(pButtonCom->GetCheck()) // 内部命令
		{
			WTNMC4A_SingleStepInterpolationCom(gl_pADView->m_hDevice);// 设置命令控制单步插补运动
		}
		else
		{
			WTNMC4A_SingleStepInterpolationExt(gl_pADView->m_hDevice);// 设置外部控制单步插补运动
		}
	}
	else
	{
		WTNMC4A_ClearSingleStepInterpolation(gl_pADView->m_hDevice); // 清除单步插补
	}
}

void CPageInterpolation::OnRADIOSingleStepInterpCom() 
{
	// TODO: Add your control notification handler code here
	WTNMC4A_SingleStepInterpolationCom(gl_pADView->m_hDevice);// 设置命令控制单步插补运动
}

 

void CPageInterpolation::OnRADIOSingleStepInterpExt() 
{
	// TODO: Add your control notification handler code here
	WTNMC4A_SingleStepInterpolationExt(gl_pADView->m_hDevice);
}

void CPageInterpolation::OnBUTTONStartSingleStepInterp() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->StartSingleStepInterpMovement();
}
