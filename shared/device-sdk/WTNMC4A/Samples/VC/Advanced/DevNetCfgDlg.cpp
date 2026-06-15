// DevNetCfgDlg.cpp : implementation file
//

#include "stdafx.h"
#include "Sys.h"
#include "DevNetCfgDlg.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif

/////////////////////////////////////////////////////////////////////////////
// CDevNetCfgDlg dialog


extern CSysApp theApp;
CDevNetCfgDlg::CDevNetCfgDlg(CWnd* pParent /*=NULL*/)
	: CDialog(CDevNetCfgDlg::IDD, pParent)
{
	//{{AFX_DATA_INIT(CDevNetCfgDlg)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT
}


void CDevNetCfgDlg::DoDataExchange(CDataExchange* pDX)
{
	CDialog::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CDevNetCfgDlg)
		// NOTE: the ClassWizard will add DDX and DDV calls here
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CDevNetCfgDlg, CDialog)
	//{{AFX_MSG_MAP(CDevNetCfgDlg)
	ON_BN_CLICKED(IDC_BUTTON_Modify, OnBUTTONModify)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CDevNetCfgDlg message handlers

void CDevNetCfgDlg::OnBUTTONModify() 
{
	// TODO: Add your control notification handler code here
	
	DEVICE_NET_INFO DeviceInfo;
	GetDlgItem(IDC_IPADDRESS_IP)->GetWindowText(DeviceInfo.strIP, 16);
	GetDlgItem(IDC_IPADDRESS_Mask)->GetWindowText(DeviceInfo.strSubnetMask, 16);
	GetDlgItem(IDC_IPADDRESS_Gate)->GetWindowText(DeviceInfo.strGateway, 16);
	GetDlgItem(IDC_EDIT_MAC)->GetWindowText(DeviceInfo.strMAC, 20);

	
	if(!WTNMC4A_SetNetCfg(theApp.m_hDeviceApp, &DeviceInfo))
	{
		AfxMessageBox(L"修改失败!\n");
	}
	else
	{
		AfxMessageBox(L"修改成功,请断电重新连接设备!\n");
	}
}

BOOL CDevNetCfgDlg::OnInitDialog() 
{
	CDialog::OnInitDialog();
	
	// TODO: Add extra initialization here
	DEVICE_NET_INFO DeviceInfo;
	if (theApp.m_hDeviceApp != INVALID_HANDLE_VALUE)
	{
		if(!WTNMC4A_GetNetCfg(theApp.m_hDeviceApp, &DeviceInfo))
		{
			return TRUE;
		}
		else
		{
			GetDlgItem(IDC_IPADDRESS_IP)->SetWindowText(DeviceInfo.strIP);
			GetDlgItem(IDC_IPADDRESS_Mask)->SetWindowText(DeviceInfo.strSubnetMask);
			GetDlgItem(IDC_IPADDRESS_Gate)->SetWindowText(DeviceInfo.strGateway);
			GetDlgItem(IDC_EDIT_MAC)->SetWindowText(DeviceInfo.strMAC);
			GetDlgItem(IDC_BUTTON_Modify)->EnableWindow(TRUE);
		}
	}
	
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}
