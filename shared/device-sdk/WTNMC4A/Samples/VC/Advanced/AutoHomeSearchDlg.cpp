// AutoHomeSearchDlg.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "AutoHomeSearchDlg.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CAutoHomeSearchDlg dialog


CAutoHomeSearchDlg::CAutoHomeSearchDlg(CWnd* pParent /*=NULL*/)
: CDialog(CAutoHomeSearchDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CAutoHomeSearchDlg)
	//}}AFX_DATA_INIT
}


void CAutoHomeSearchDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CAutoHomeSearchDlg)
	DDX_Control(pDX, IDC_EDIT_LowSpeed, m_Edit_LowSpeed);
	DDX_Control(pDX, IDC_EDIT_HighSpeed, m_Edit_HighSpeed);
//	DDX_Control(pDX, IDC_CHECK_LIMIT, m_Button_LIMIT);
	DDX_Control(pDX, IDC_CHECK_PCLR, m_Button_PCLR);
	DDX_Control(pDX, IDC_CHECK_SAND, m_Button_SAND);
	DDX_Control(pDX, IDC_EDIT_OffsetPulseNum, m_Edit_OffsetPulseNum);
	DDX_Control(pDX, IDC_CHECK_ST4E, m_Button_ST4E);
	DDX_Control(pDX, IDC_CHECK_ST3E, m_Button_ST3E);
	DDX_Control(pDX, IDC_CHECK_ST2E, m_Button_ST2E);
	DDX_Control(pDX, IDC_CHECK_ST1E, m_Button_ST1E);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CAutoHomeSearchDlg, CDialog)
	//{{AFX_MSG_MAP(CAutoHomeSearchDlg)
	ON_BN_CLICKED(IDC_BUTTON_OnOK, OnBUTTONOnOK)
	ON_BN_CLICKED(IDC_CHECK_ST1E, OnCheckSt1e)
	ON_BN_CLICKED(IDC_CHECK_ST2E, OnCheckSt2e)
	ON_BN_CLICKED(IDC_CHECK_ST3E, OnCheckSt3e)
	ON_BN_CLICKED(IDC_CHECK_ST4E, OnCheckSt4e)
	ON_BN_CLICKED(IDC_RADIO_ST1DP, OnRadioSt1dp)
	ON_BN_CLICKED(IDC_RADIO_ST1DN, OnRadioSt1dn)
	ON_BN_CLICKED(IDC_RADIO_ST2DP, OnRadioSt2dp)
	ON_BN_CLICKED(IDC_RADIO_ST2DN, OnRadioSt2dn)
	ON_BN_CLICKED(IDC_RADIO_ST3DP, OnRadioSt3dp)
	ON_BN_CLICKED(IDC_RADIO_ST3DN, OnRadioSt3dn)
	ON_BN_CLICKED(IDC_RADIO_ST4DP, OnRadioSt4dp)
	ON_BN_CLICKED(IDC_RADIO_ST4DN, OnRadioSt4dn)
	ON_EN_CHANGE(IDC_EDIT_OffsetPulseNum, OnChangeEDITOffsetPulseNum)
	ON_BN_CLICKED(IDC_CHECK_SAND, OnCheckSand)
	ON_BN_CLICKED(IDC_CHECK_PCLR, OnCheckPclr)
//	ON_BN_CLICKED(IDC_CHECK_LIMIT, OnCheckLimit)
	ON_EN_CHANGE(IDC_EDIT_HighSpeed, OnChangeEDITHighSpeed)
	ON_EN_CHANGE(IDC_EDIT_LowSpeed, OnChangeEDITLowSpeed)
	ON_BN_CLICKED(IDC_RADIO_IN0H, OnRadioIn0h)
	ON_BN_CLICKED(IDC_RADIO_IN0L, OnRadioIn0l)
	ON_BN_CLICKED(IDC_RADIO_IN1H, OnRadioIn1h)
	ON_BN_CLICKED(IDC_RADIO_IN1L, OnRadioIn1l)
	ON_BN_CLICKED(IDC_RADIO_IN2H, OnRadioIn2h)
	ON_BN_CLICKED(IDC_RADIO_IN2L, OnRadioIn2l)
	ON_WM_CLOSE()
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CAutoHomeSearchDlg message handlers

void CAutoHomeSearchDlg::OnBUTTONOnOK() 
{
	// TODO: Add your control notification handler code here
	
	CDialog::OnOK();
}

void CAutoHomeSearchDlg::OnCheckSt1e() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1E = m_Button_ST1E.GetCheck();
}

BOOL CAutoHomeSearchDlg::OnInitDialog() 
{	
	CDialog::OnInitDialog();
	// TODO: Add extra initialization here
	m_nCurrentAxis = gl_pADView->m_nCurrentAxis;	
	m_Button_ST1E.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1E);
	CButton* pButtonSTDP = (CButton*)GetDlgItem(IDC_RADIO_ST1DP); 
	CButton* pButtonSTDN = (CButton*)GetDlgItem(IDC_RADIO_ST1DN);
	pButtonSTDP->SetCheck(!gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1D); // 正方向
	pButtonSTDN->SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1D);  // 反方向
	m_Button_ST2E.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2E);
	pButtonSTDP = (CButton*)GetDlgItem(IDC_RADIO_ST2DP); 
	pButtonSTDN = (CButton*)GetDlgItem(IDC_RADIO_ST2DN);
	pButtonSTDP->SetCheck(!gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2D); // 正方向
	pButtonSTDN->SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2D);  // 反方向
	m_Button_ST3E.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3E);
	pButtonSTDP = (CButton*)GetDlgItem(IDC_RADIO_ST3DP); 
	pButtonSTDN = (CButton*)GetDlgItem(IDC_RADIO_ST3DN);
	pButtonSTDP->SetCheck(!gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3D); // 正方向
	pButtonSTDN->SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3D);  // 反方向
	m_Button_ST4E.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4E);
	pButtonSTDP = (CButton*)GetDlgItem(IDC_RADIO_ST4DP); 
	pButtonSTDN = (CButton*)GetDlgItem(IDC_RADIO_ST4DN);
	pButtonSTDP->SetCheck(!gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4D); // 正方向
	pButtonSTDN->SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4D);  // 反方向
	CButton *pButtonINH = (CButton*)GetDlgItem(IDC_RADIO_IN0H);
	CButton *pButtonINL = (CButton*)GetDlgItem(IDC_RADIO_IN0L);
	pButtonINH->SetCheck(gl_pADView->m_nInData[m_nCurrentAxis][0]);  // 高电平
 	pButtonINL->SetCheck(1); // 低电平
	pButtonINH = (CButton*)GetDlgItem(IDC_RADIO_IN1H);
	pButtonINL = (CButton*)GetDlgItem(IDC_RADIO_IN1L);
	pButtonINH->SetCheck(gl_pADView->m_nInData[m_nCurrentAxis][1]);  // 高电平
 	pButtonINL->SetCheck(1); // 低电平
	pButtonINH = (CButton*)GetDlgItem(IDC_RADIO_IN2H);
	pButtonINL = (CButton*)GetDlgItem(IDC_RADIO_IN2L);
	pButtonINH->SetCheck(gl_pADView->m_nInData[m_nCurrentAxis][2]);  // 高电平
 	pButtonINL->SetCheck(1); // 低电平
	m_Button_SAND.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].SAND);
	m_Button_PCLR.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].PCLR);
//	m_Button_LIMIT.SetCheck(gl_pADView->m_HomeSearchPara[m_nCurrentAxis].LIMIT);
	CString str;
	str.Format(L"%d", gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum);
	m_Edit_OffsetPulseNum.SetWindowText(str);
	str.Format(L"%d", gl_pADView->m_DataList[m_nCurrentAxis].DriveSpeed);
	m_Edit_HighSpeed.SetWindowText(str);
	str.Format(L"%d", gl_pADView->m_nHomeLowSpeed);
	m_Edit_LowSpeed.SetWindowText(str);

// 	(CButton*)GetDlgItem(IDC_RADIO_IN0H)->ShowWindow(SW_HIDE);
// 	(CButton*)GetDlgItem(IDC_RADIO_IN1H)->ShowWindow(SW_HIDE);
// 	(CButton*)GetDlgItem(IDC_RADIO_IN2H)->ShowWindow(SW_HIDE);


	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

void CAutoHomeSearchDlg::OnCheckSt2e() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2E = m_Button_ST2E.GetCheck();
}

void CAutoHomeSearchDlg::OnCheckSt3e() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3E = m_Button_ST3E.GetCheck();	
}

void CAutoHomeSearchDlg::OnCheckSt4e() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4E = m_Button_ST4E.GetCheck();	
}

void CAutoHomeSearchDlg::OnRadioSt1dp() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1D = 0; // 正方向
}

void CAutoHomeSearchDlg::OnRadioSt1dn() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST1D = 1; // 反方向
}

void CAutoHomeSearchDlg::OnRadioSt2dp() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2D = 0; // 正方向	
}

void CAutoHomeSearchDlg::OnRadioSt2dn() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST2D = 1; // 反方向	
}

void CAutoHomeSearchDlg::OnRadioSt3dp() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3D = 0; // 正方向	
}

void CAutoHomeSearchDlg::OnRadioSt3dn() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST3D = 1; // 反方向		
}

void CAutoHomeSearchDlg::OnRadioSt4dp() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4D = 0; // 正方向	
}

void CAutoHomeSearchDlg::OnRadioSt4dn() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].ST4D = 1; // 反方向	
}

// 脉冲计数器偏移
void CAutoHomeSearchDlg::OnChangeEDITOffsetPulseNum() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_OffsetPulseNum.GetWindowText(str);
	gl_pADView->m_LCData[m_nCurrentAxis].nPulseNum = wcstol(str, NULL, 10);
}

// 原点信号和Z相信号有效时停止第三步操作
void CAutoHomeSearchDlg::OnCheckSand() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].SAND = m_Button_SAND.GetCheck();
}

// 第四步结束时清除逻辑计数器和实位计数器 
void CAutoHomeSearchDlg::OnCheckPclr() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].PCLR = m_Button_PCLR.GetCheck();
}

// 利用硬件限位信号(nLMTP或nLMPM)进行原点搜寻
void CAutoHomeSearchDlg::OnCheckLimit() 
{
	// TODO: Add your control notification handler code here
//	gl_pADView->m_HomeSearchPara[m_nCurrentAxis].LIMIT = m_Button_LIMIT.GetCheck();
}

void CAutoHomeSearchDlg::OnChangeEDITHighSpeed() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_HighSpeed.GetWindowText(str);
	gl_pADView->m_DataList[m_nCurrentAxis].DriveSpeed = wcstol(str, NULL, 10);
}

void CAutoHomeSearchDlg::OnChangeEDITLowSpeed() 
{
	// TODO: Add your control notification handler code here
	CString str;
	m_Edit_LowSpeed.GetWindowText(str);
	gl_pADView->m_nHomeLowSpeed = wcstol(str, NULL, 10);
//	WTNMC4A_SetHV(gl_pADView->m_hDevice, m_nCurrentAxis, nData);
}

void CAutoHomeSearchDlg::OnRadioIn0h() 
{
	// TODO: Add your control notification handler code here
//	WTNMC4A_SetInEnable(gl_pADView->m_hDevice, m_nCurrentAxis, 0, 1);
	gl_pADView->m_nInData[m_nCurrentAxis][0] = 1;
}

void CAutoHomeSearchDlg::OnRadioIn0l() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_nInData[m_nCurrentAxis][0] = 0;
}

void CAutoHomeSearchDlg::OnRadioIn1h() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_nInData[m_nCurrentAxis][1] = 1;
}

void CAutoHomeSearchDlg::OnRadioIn1l() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_nInData[m_nCurrentAxis][1] = 0;
}

void CAutoHomeSearchDlg::OnRadioIn2h() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_nInData[m_nCurrentAxis][2] = 1;
}

void CAutoHomeSearchDlg::OnRadioIn2l() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_nInData[m_nCurrentAxis][2] = 0;
}

void CAutoHomeSearchDlg::OnClose() 
{
	// TODO: Add your message handler code here and/or call default
	CDialog::OnClose();
}
