// SelDevNetDlg.cpp : implementation file
//

#include "stdafx.h"
#include "Sys.h"
#include "SelDevNetDlg.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CSelDevNetDlg dialog


extern CSysApp theApp;
CSelDevNetDlg::CSelDevNetDlg(CWnd* pParent /*=NULL*/)
	: CDialog(CSelDevNetDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CSelDevNetDlg)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT


}


void CSelDevNetDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CSelDevNetDlg)
	DDX_Control(pDX, IDC_IPADDRESS1, m_IPAddr);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CSelDevNetDlg, CDialog)
	//{{AFX_MSG_MAP(CSelDevNetDlg)
	ON_BN_CLICKED(IDC_BUTTON_Link, OnBUTTONLink)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CSelDevNetDlg message handlers
extern CSysApp theApp;
void CSelDevNetDlg::OnBUTTONLink() 
{
	// TODO: Add your control notification handler code here
	CString strIPAddr;

	CEdit* pEdit = (CEdit*)GetDlgItem(IDC_EDIT_RS_Timeout);
	pEdit->GetWindowText(strIPAddr);
	ULONG nRSTimeout = wcstol(strIPAddr, NULL, 10);

	m_IPAddr.GetWindowText(strIPAddr);



	GetDlgItem(IDOK)->EnableWindow(FALSE);
	GetDlgItem(IDC_BUTTON_Link)->SetWindowText(L"正在连接...");
	HANDLE hDevice =WTNMC4A_DEV_CreateW(strIPAddr, nRSTimeout, nRSTimeout);
	if (hDevice != INVALID_HANDLE_VALUE)
	{
		WTNMC4A_DEV_Release(hDevice);
		GetDlgItem(IDOK)->EnableWindow(TRUE);
		GetDlgItem(IDOK)->SetWindowText(L"OK");
		
		theApp.WriteProfileString(L"ParaSave", L"IPAddr", strIPAddr);
		theApp.m_IPAddr = strIPAddr;
		theApp.m_nRSTimeout = nRSTimeout;
		theApp.m_bLinkSuccess = TRUE;	
	}
	else
	{
		GetDlgItem(IDOK)->EnableWindow(TRUE);
		GetDlgItem(IDOK)->SetWindowText(L"进入半模拟状态");
		AfxMessageBox(L"无法连接指定设备，请确保IP地址设置正确以及设备与电脑正确连接，与外部电源正确连接");
		
		theApp.m_bLinkSuccess = FALSE;	
	}
	GetDlgItem(IDC_BUTTON_Link)->SetWindowText(L"连接");
}

BOOL CSelDevNetDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	
	
	CString strTmp;	
	// 取得IPAddr
	strTmp = theApp.GetProfileString(L"ParaSave", L"IPAddr", L"192.168.1.4");
	m_IPAddr.SetWindowText(strTmp);
	CEdit* pEdit = (CEdit*)GetDlgItem(IDC_EDIT_RS_Timeout);
	pEdit->SetWindowText(L"500");
	//OnBUTTONLink();
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}
