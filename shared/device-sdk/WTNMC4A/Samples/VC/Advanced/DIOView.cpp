// IOView.cpp : implementation file
//

#include "stdafx.h"
#include "Sys.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
/////////////////////////////////////////////////////////////////////////////
// CDIOView

IMPLEMENT_DYNCREATE(CDIOView, CFormView)

CDIOView::CDIOView()
	: CFormView(CDIOView::IDD)
{
	//{{AFX_DATA_INIT(CDIOView)
	m_bDO0 = FALSE;
	m_bDO1 = FALSE;
	m_bDO2 = FALSE;
	m_bDO3 = FALSE;
	m_bDO4 = FALSE;
	m_bDO5 = FALSE;
	m_bDO6 = FALSE;
	m_bDO7 = FALSE;
	m_bDO8 = FALSE;
	m_bDO9 = FALSE;
	m_bDO10 = FALSE;
	m_bDO11 = FALSE;
	m_bDO12 = FALSE;
	m_bDO13 = FALSE;
	m_bDO14 = FALSE;
	m_bDO15 = FALSE;
	//}}AFX_DATA_INIT
	bCreateDevice = FALSE;
}

CDIOView::~CDIOView()
{
}

void CDIOView::DoDataExchange(CDataExchange* pDX)
{
	CFormView::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CDIOView)
	DDX_Control(pDX, IDC_CHECK_DI7, m_Button_In7);
	DDX_Control(pDX, IDC_CHECK_DI6, m_Button_In6);
	DDX_Control(pDX, IDC_CHECK_DI5, m_Button_In5);
	DDX_Control(pDX, IDC_CHECK_DI4, m_Button_In4);
	DDX_Control(pDX, IDC_CHECK_DI3, m_Button_In3);
	DDX_Control(pDX, IDC_CHECK_DI2, m_Button_In2);
	DDX_Control(pDX, IDC_CHECK_DI1, m_Button_In1);
	DDX_Control(pDX, IDC_CHECK_DI0, m_Button_In0);
	DDX_Control(pDX, IDC_CHECK_DO7, m_Check_DO7);
	DDX_Control(pDX, IDC_CHECK_DO6, m_Check_DO6);
	DDX_Control(pDX, IDC_CHECK_DO5, m_Check_DO5);
	DDX_Control(pDX, IDC_CHECK_DO4, m_Check_DO4);
	DDX_Control(pDX, IDC_CHECK_DO3, m_Check_DO3);
	DDX_Control(pDX, IDC_CHECK_DO2, m_Check_DO2);
	DDX_Control(pDX, IDC_CHECK_DO1, m_Check_DO1);
	DDX_Control(pDX, IDC_CHECK_DO0, m_Check_DO0);
	DDX_Check(pDX, IDC_CHECK_DO0, m_bDO0);
	DDX_Check(pDX, IDC_CHECK_DO1, m_bDO1);
	DDX_Check(pDX, IDC_CHECK_DO2, m_bDO2);
	DDX_Check(pDX, IDC_CHECK_DO3, m_bDO3);
	DDX_Check(pDX, IDC_CHECK_DO4, m_bDO4);
	DDX_Check(pDX, IDC_CHECK_DO5, m_bDO5);
	DDX_Check(pDX, IDC_CHECK_DO6, m_bDO6);
	DDX_Check(pDX, IDC_CHECK_DO7, m_bDO7);
	//}}AFX_DATA_MAP
}

BEGIN_MESSAGE_MAP(CDIOView, CFormView)
	//{{AFX_MSG_MAP(CDIOView)
	ON_BN_CLICKED(IDC_CHECK_DO0, OnCheckDo0)
	ON_BN_CLICKED(IDC_CHECK_DO1, OnCheckDo1)
	ON_BN_CLICKED(IDC_CHECK_DO2, OnCheckDo2)
	ON_BN_CLICKED(IDC_CHECK_DO3, OnCheckDo3)
	ON_BN_CLICKED(IDC_CHECK_DO4, OnCheckDo4)
	ON_BN_CLICKED(IDC_CHECK_DO5, OnCheckDo5)
	ON_BN_CLICKED(IDC_CHECK_DO6, OnCheckDo6)
	ON_BN_CLICKED(IDC_CHECK_DO7, OnCheckDo7)
	ON_WM_TIMER()
	ON_WM_DESTROY()
	ON_BN_CLICKED(IDC_CHECK_AutoRead, OnCHECKAutoRead)
	ON_BN_CLICKED(IDC_BUTTON_Read, OnBUTTONRead)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CDIOView diagnostics

#ifdef _DEBUG
void CDIOView::AssertValid() const
{
	CFormView::AssertValid();
}

void CDIOView::Dump(CDumpContext& dc) const
{
	CFormView::Dump(dc);
}
#endif //_DEBUG

/////////////////////////////////////////////////////////////////////////////
// CDIOView message handlers

void CDIOView::SetSwitchStatus(void)
{
	if (!bCreateDevice)
		return;
	 BYTE DISts[8];
	// 读入各路开关量状态
	if (!PCI8603_GetDeviceDI(hDevice, DISts))
	{
		MessageBox("读开关量数据失败...");
		return;
	}
	
	if (DISts[0])  // 开
	{
			m_Button_In0.SetState(TRUE);
			m_Button_In0.SetWindowText("开关0 (开)");
			m_Button_In0.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In0.SetState(FALSE);
		m_Button_In0.SetWindowText("开关0 (关)");
		m_Button_In0.SetCheck(0); 
		
	}
	
	if (DISts[1])  // 开
	{
		m_Button_In1.SetState(TRUE);
		m_Button_In1.SetWindowText("开关1 (开)");
		m_Button_In1.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In1.SetState(FALSE);
		m_Button_In1.SetWindowText("开关1 (关)");
		m_Button_In1.SetCheck(0); 
		
	}
	
	if (DISts[2])  // 开
	{
		m_Button_In2.SetState(TRUE);
		m_Button_In2.SetWindowText("开关2 (开)");
		m_Button_In2.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In2.SetState(FALSE);
		m_Button_In2.SetWindowText("开关2 (关)");
		m_Button_In2.SetCheck(0); 
		
	}
	
	if (DISts[3])  // 开
	{
		m_Button_In3.SetState(TRUE);
		m_Button_In3.SetWindowText("开关3 (开)");
		m_Button_In3.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In3.SetState(FALSE);
		m_Button_In3.SetWindowText("开关3 (关)");
		m_Button_In3.SetCheck(0); 
		
	}
	
	if (DISts[4])  // 开
	{
		m_Button_In4.SetState(TRUE);
		m_Button_In4.SetWindowText("开关4 (开)");
		m_Button_In4.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In4.SetState(FALSE);
		m_Button_In4.SetWindowText("开关4 (关)");
		m_Button_In4.SetCheck(0); 
		
	}
	
	if (DISts[5])  // 开
	{
		m_Button_In5.SetState(TRUE);
		m_Button_In5.SetWindowText("开关5 (开)");
		m_Button_In5.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In5.SetState(FALSE);
		m_Button_In5.SetWindowText("开关5 (关)");
		m_Button_In5.SetCheck(0); 
		
	}
	
	if (DISts[6])  // 开
	{
		m_Button_In6.SetState(TRUE);
		m_Button_In6.SetWindowText("开关6 (开)");
		m_Button_In6.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In6.SetState(FALSE);
		m_Button_In6.SetWindowText("开关6 (关)");
		m_Button_In6.SetCheck(0); 
		
	}
	
	if (DISts[7])  // 开
	{
		m_Button_In7.SetState(TRUE);
		m_Button_In7.SetWindowText("开关7 (开)");
		m_Button_In7.SetCheck(1); 
	}
	else
	{              // 关
		m_Button_In7.SetState(FALSE);
		m_Button_In7.SetWindowText("开关7 (关)");
		m_Button_In7.SetCheck(0); 
	}	
}

void CDIOView::OnTimer(UINT nIDEvent) 
{
	SetSwitchStatus();
	CFormView::OnTimer(nIDEvent);
}

void CDIOView::SetSwitchOut(void)
{
	if (!bCreateDevice)
		return;
	BYTE DOSts[8];
	DOSts[0] = BYTE(m_bDO0);
	DOSts[1] = BYTE(m_bDO1);
	DOSts[2] = BYTE(m_bDO2);
	DOSts[3] = BYTE(m_bDO3);
	DOSts[4] = BYTE(m_bDO4);
	DOSts[5] = BYTE(m_bDO5);
	DOSts[6] = BYTE(m_bDO6);
	DOSts[7] = BYTE(m_bDO7);
	Flag = true;
	// 输出各路开关量状态
	PCI8603_SetDeviceDO(hDevice, DOSts); 
}

void CDIOView::OnCheckDo0() 
{
	UpdateData(TRUE);
	if (m_bDO0)
	{
		m_Check_DO0.SetWindowText("开关0 (开)");
	}
	else
	{
		m_Check_DO0.SetWindowText("开关0 (关)");
	}
	SetSwitchOut();
}

void CDIOView::OnCheckDo1() 
{
	UpdateData(TRUE);
	if (m_bDO1)
	{
		m_Check_DO1.SetWindowText("开关1 (开)");
	}
	else
	{
		m_Check_DO1.SetWindowText("开关1 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo2() 
{
	UpdateData(TRUE);
	if (m_bDO2)
	{
		m_Check_DO2.SetWindowText("开关2 (开)");
	}
	else
	{
		m_Check_DO2.SetWindowText("开关2 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo3() 
{
	UpdateData(TRUE);
	if (m_bDO3)
	{
		m_Check_DO3.SetWindowText("开关3 (开)");
	}
	else
	{
		m_Check_DO3.SetWindowText("开关3 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo4() 
{
	UpdateData(TRUE);
	if (m_bDO4)
	{
		m_Check_DO4.SetWindowText("开关4 (开)");
	}
	else
	{
		m_Check_DO4.SetWindowText("开关4 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo5() 
{
	UpdateData(TRUE);
	if (m_bDO5)
	{
		m_Check_DO5.SetWindowText("开关5 (开)");
	}
	else
	{
		m_Check_DO5.SetWindowText("开关5 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo6() 
{
	UpdateData(TRUE);
	if (m_bDO6)
	{
		m_Check_DO6.SetWindowText("开关6 (开)");
	}
	else
	{
		m_Check_DO6.SetWindowText("开关6 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnCheckDo7() 
{
	UpdateData(TRUE);
	if (m_bDO7)
	{
		m_Check_DO7.SetWindowText("开关7 (开)");
	}
	else
	{
		m_Check_DO7.SetWindowText("开关7 (关)");
	}
	
	SetSwitchOut();
}

void CDIOView::OnInitialUpdate()
{
	CFormView::OnInitialUpdate();

	CSysApp *pApp = (CSysApp *)AfxGetApp();
	DeviceID = pApp->m_CurrentDeviceID;  // 取得当前应用程序使用的设备ID号	
	Flag = true;
	hDevice = INVALID_HANDLE_VALUE;
//	hDevice = PCI8603_CreateDevice(DeviceID);

	if (hDevice != INVALID_HANDLE_VALUE)
	{	
		bCreateDevice = TRUE;
	}
	else
	{
		return;
	}
	SetTimer(1, 350, NULL);
	m_CHECK_AutoRead()->SetCheck(TRUE);
	CButton* pButton = (CButton*)GetDlgItem(IDC_BUTTON_Read);
	pButton->ShowWindow(SW_HIDE);

	BYTE DOSts[8];
	//PCI8603_RetDeviceDO( hDevice, DOSts);
	memset(DOSts, NULL, FALSE);
	m_bDO0 = DOSts[0];
	m_bDO1 = DOSts[1];
	m_bDO2 = DOSts[2];
	m_bDO3 = DOSts[3];
	m_bDO4 = DOSts[4];
	m_bDO5 = DOSts[5];
	m_bDO6 = DOSts[6];
	m_bDO7 = DOSts[7];

	UpdateData(FALSE);

	OnCheckDo0();
	OnCheckDo1();
	OnCheckDo2();
	OnCheckDo3();
	OnCheckDo4();
	OnCheckDo5();
	OnCheckDo6();
	OnCheckDo7();
}


void CDIOView::OnDestroy() 
{
	if (hDevice == INVALID_HANDLE_VALUE)
	{
		return;
	}
	
	if (!PCI8603_ReleaseDevice(hDevice))
		AfxMessageBox("设备释放失败");
	CFormView::OnDestroy();	
}

void CDIOView::OnCHECKAutoRead() 
{
	if (m_CHECK_AutoRead()->GetCheck())
	{
		SetTimer(1, 350, NULL);
		m_BUTTON_Read()->ShowWindow(SW_HIDE);
	}
	else
	{
		KillTimer(1);
		m_BUTTON_Read()->ShowWindow(SW_SHOW);
	}
}

void CDIOView::OnBUTTONRead() 
{
	SetSwitchStatus();
}

// void CDIOView::OnPaint() 
// {
// // 	CPaintDC dc(this); // device context for painting	
// }
