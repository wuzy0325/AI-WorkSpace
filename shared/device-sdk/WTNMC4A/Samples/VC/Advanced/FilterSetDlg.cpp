// FilterSetDlg.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "FilterSetDlg.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CFilterSetDlg dialog


CFilterSetDlg::CFilterSetDlg(CWnd* pParent /*=NULL*/)
	: CDialog(CFilterSetDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CFilterSetDlg)
	//}}AFX_DATA_INIT
}


void CFilterSetDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CFilterSetDlg)
	DDX_Control(pDX, IDC_STATIC_SignalDelay, m_Static_SignalDelay);
	DDX_Control(pDX, IDC_COMBO_TimeConst, m_Combo_TimeConst);
	DDX_Control(pDX, IDC_CHECK_FE4, m_Button_FE4);
	DDX_Control(pDX, IDC_CHECK_FE3, m_Button_FE3);
	DDX_Control(pDX, IDC_CHECK_FE2, m_Button_FE2);
	DDX_Control(pDX, IDC_CHECK_FE1, m_Button_FE1);
	DDX_Control(pDX, IDC_CHECK_FE0, m_Button_FE0);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CFilterSetDlg, CDialog)
	//{{AFX_MSG_MAP(CFilterSetDlg)
	ON_CBN_SELCHANGE(IDC_COMBO_TimeConst, OnSelchangeCOMBOTimeConst)
	ON_BN_CLICKED(IDC_CHECK_FE0, OnCheckFe0)
	ON_BN_CLICKED(IDC_CHECK_FE1, OnCheckFe1)
	ON_BN_CLICKED(IDC_CHECK_FE2, OnCheckFe2)
	ON_BN_CLICKED(IDC_CHECK_FE3, OnCheckFe3)
	ON_BN_CLICKED(IDC_CHECK_FE4, OnCheckFe4)
	ON_WM_CLOSE()
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CFilterSetDlg message handlers

BOOL CFilterSetDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	m_nCurrentAxis = gl_pADView->m_nCurrentAxis;
	m_Button_FE0.SetCheck(gl_pADView->m_FilterPara[m_nCurrentAxis].FE0);
	m_Button_FE1.SetCheck(gl_pADView->m_FilterPara[m_nCurrentAxis].FE1);
	m_Button_FE2.SetCheck(gl_pADView->m_FilterPara[m_nCurrentAxis].FE2);
	m_Button_FE3.SetCheck(gl_pADView->m_FilterPara[m_nCurrentAxis].FE3);
	m_Button_FE4.SetCheck(gl_pADView->m_FilterPara[m_nCurrentAxis].FE4);
	int nIndex = gl_pADView->m_FilterPara[m_nCurrentAxis].FL0 + 
			gl_pADView->m_FilterPara[m_nCurrentAxis].FL1*2 + 
			gl_pADView->m_FilterPara[m_nCurrentAxis].FL2*4;
	m_Combo_TimeConst.SetCurSel(nIndex);
	CString str = GetSignalDelay(nIndex); // 通过索引值取得信号延迟时间
	m_Static_SignalDelay.SetWindowText(str);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

CString CFilterSetDlg::GetSignalDelay(int nIndex)
{
	CString str;
	switch(nIndex)
	{
	case 0:
		str = "2uS";
		break;
	case 1:
		str = "256uS";
		break;
	case 2:
		str = "512uS";
		break;
	case 3:
		str = "1.024mS";
		break;
	case 4:
		str = "2.048mS";
		break;
	case 5:
		str = "4.096mS";
		break;
	case 6:
		str = "8.012mS";
		break;
	case 7:
		str = "16.384mS";
		break;
	default:
		break;	
	}
	return str;
}

// 时间常量选择
void CFilterSetDlg::OnSelchangeCOMBOTimeConst() 
{
	// TODO: Add your control notification handler code here
	int nIndex = m_Combo_TimeConst.GetCurSel();
	CString str = GetSignalDelay(nIndex); // 通过索引值取得信号延迟时间
	m_Static_SignalDelay.SetWindowText(str);
	int FL2 = nIndex/4;
	int FL1 = (nIndex%4)/2;
	int FL0 = nIndex%2;
	gl_pADView->m_FilterPara[m_nCurrentAxis].FL2 = FL2;
	gl_pADView->m_FilterPara[m_nCurrentAxis].FL1 = FL1;
	gl_pADView->m_FilterPara[m_nCurrentAxis].FL0 = FL0;
}

void CFilterSetDlg::OnCheckFe0() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_FilterPara[m_nCurrentAxis].FE0 = m_Button_FE0.GetCheck();
}

void CFilterSetDlg::OnCheckFe1() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_FilterPara[m_nCurrentAxis].FE1 = m_Button_FE1.GetCheck();
}

void CFilterSetDlg::OnCheckFe2() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_FilterPara[m_nCurrentAxis].FE2 = m_Button_FE2.GetCheck();
}

void CFilterSetDlg::OnCheckFe3() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_FilterPara[m_nCurrentAxis].FE3 = m_Button_FE3.GetCheck();
}

void CFilterSetDlg::OnCheckFe4() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_FilterPara[m_nCurrentAxis].FE4 = m_Button_FE4.GetCheck();	
}

void CFilterSetDlg::OnClose() 
{
	// TODO: Add your message handler code here and/or call default
	WTNMC4A_ExtMode(			
		gl_pADView->m_hDevice,			// 设备句柄
		gl_pADView->m_nCurrentAxis,			// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)
		&gl_pADView->m_FilterPara[m_nCurrentAxis]);// 滤波器参数结构体指针	
	CDialog::OnClose();
}

void CFilterSetDlg::OnOK() 
{
	// TODO: Add extra validation here
	OnClose();
	CDialog::OnOK();
}
