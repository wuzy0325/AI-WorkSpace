// PageDIO.cpp : implementation file
//
#include "stdafx.h"
#include "sys.h"
#include "PageDIO.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageDIO property page
///////////////////////X轴////////////////////////////////////////////
UINT BUTTONID_DOX[8]={
	IDC_DOX0,IDC_DOX1, IDC_DOX2,IDC_DOX3,IDC_DOX4,IDC_DOX5,IDC_DOX6,IDC_DOX7
};
UINT BUTTONID_DIX[8]={ 
	IDC_DIX0,IDC_DIX1, IDC_DIX2,IDC_DIX3,IDC_DIX4,IDC_DIX5,IDC_DIX6,IDC_DIX7
};
//////////////////////Y轴///////////////////////////////////////////////////
UINT BUTTONID_DOY[8]={ 
	IDC_DOY0,IDC_DOY1, IDC_DOY2,IDC_DOY3,IDC_DOY4,IDC_DOY5,IDC_DOY6,IDC_DOY7
};
UINT BUTTONID_DIY[8]={ 
	IDC_DIY0,IDC_DIY1, IDC_DIY2,IDC_DIY3,IDC_DIY4,IDC_DIY5,IDC_DIY6,IDC_DIY7
};
/////////////////////Z轴////////////////////////////////////////////////////
UINT BUTTONID_DOZ[8]={ 
	IDC_DOZ0,IDC_DOZ1, IDC_DOZ2,IDC_DOZ3,IDC_DOZ4,IDC_DOZ5,IDC_DOZ6,IDC_DOZ7
};
UINT BUTTONID_DIZ[8]={ 
	IDC_DIZ0,IDC_DIZ1, IDC_DIZ2,IDC_DIZ3,IDC_DIZ4,IDC_DIZ5,IDC_DIZ6,IDC_DIZ7
};
//////////////////////U轴////////////////////////////////////////////////////
UINT BUTTONID_DOU[8]={ 
	IDC_DOU0,IDC_DOU1, IDC_DOU2,IDC_DOU3,IDC_DOU4,IDC_DOU5,IDC_DOU6,IDC_DOU7
};
UINT BUTTONID_DIU[8]={
	IDC_DIU0,IDC_DIU1, IDC_DIU2,IDC_DIU3,IDC_DIU4,IDC_DIU5,IDC_DIU6,IDC_DIU7
};
//////////////////////////////////////////////////////////////////////////////
IMPLEMENT_DYNCREATE(CPageDIO, CPropertyPage)

CPageDIO::CPageDIO() : CPropertyPage(CPageDIO::IDD)
{
	//{{AFX_DATA_INIT(CPageDIO)


	//}}AFX_DATA_INIT
}

CPageDIO::~CPageDIO()
{
}

void CPageDIO::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageDIO)
 
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageDIO, CPropertyPage)
	//{{AFX_MSG_MAP(CPageDIO)
	ON_BN_CLICKED(IDC_START_DIO, OnStartDio)
	ON_BN_CLICKED(IDC_STOP_DIO, OnStopDio)
	ON_BN_CLICKED(IDC_DOX0, OnDox0)
	ON_BN_CLICKED(IDC_DOX1, OnDox1)
	ON_BN_CLICKED(IDC_DOX2, OnDox2)
	ON_BN_CLICKED(IDC_DOX3, OnDox3)
	ON_BN_CLICKED(IDC_DOX4, OnDox4)
	ON_BN_CLICKED(IDC_DOX5, OnDox5)
	ON_BN_CLICKED(IDC_DOX6, OnDox6)
	ON_BN_CLICKED(IDC_DOX7, OnDox7)
	ON_BN_CLICKED(IDC_DOY0, OnDoy0)
	ON_BN_CLICKED(IDC_DOY1, OnDoy1)
	ON_BN_CLICKED(IDC_DOY2, OnDoy2)
	ON_BN_CLICKED(IDC_DOY3, OnDoy3)
	ON_BN_CLICKED(IDC_DOY4, OnDoy4)
	ON_BN_CLICKED(IDC_DOY5, OnDoy5)
	ON_BN_CLICKED(IDC_DOY6, OnDoy6)
	ON_BN_CLICKED(IDC_DOY7, OnDoy7)
	ON_BN_CLICKED(IDC_DOZ0, OnDoz0)
	ON_BN_CLICKED(IDC_DOU1, OnDou1)
	ON_BN_CLICKED(IDC_DOU2, OnDou2)
	ON_BN_CLICKED(IDC_DOU3, OnDou3)
	ON_BN_CLICKED(IDC_DOU4, OnDou4)
	ON_BN_CLICKED(IDC_DOU5, OnDou5)
	ON_BN_CLICKED(IDC_DOU6, OnDou6)
	ON_BN_CLICKED(IDC_DOU7, OnDou7)
	ON_BN_CLICKED(IDC_DOZ1, OnDoz1)
	ON_BN_CLICKED(IDC_DOZ2, OnDoz2)
	ON_BN_CLICKED(IDC_DOZ3, OnDoz3)
	ON_BN_CLICKED(IDC_DOZ4, OnDoz4)
	ON_BN_CLICKED(IDC_DOZ5, OnDoz5)
	ON_BN_CLICKED(IDC_DOZ6, OnDoz6)
	ON_BN_CLICKED(IDC_DOZ7, OnDoz7)
	ON_BN_CLICKED(IDC_DOU0, OnDou0)
	ON_BN_CLICKED(IDC_RADIO_ComOutX, OnRADIOComOutX)
	ON_BN_CLICKED(IDC_RADIO_ComOutY, OnRADIOComOutY)
	ON_BN_CLICKED(IDC_RADIO_StatOutX, OnRADIOStatOutX)
	ON_BN_CLICKED(IDC_RADIO_StatOutY, OnRADIOStatOutY)
	ON_BN_CLICKED(IDC_RADIO_ComOutZ, OnRADIOComOutZ)
	ON_BN_CLICKED(IDC_RADIO_StatOutZ, OnRADIOStatOutZ)
	ON_BN_CLICKED(IDC_RADIO_ComOutU, OnRADIOComOutU)
	ON_BN_CLICKED(IDC_RADIO_StatOutU, OnRADIOStatOutU)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageDIO message handlers
CButton* CPageDIO::GetButtonDOX(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DOX[nIndex]); // X轴出	
}

CButton* CPageDIO::GetButtonDIX(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DIX[nIndex]); // X轴入
}


CButton* CPageDIO::GetButtonDOY(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DOY[nIndex]); // Y轴出	
}

CButton* CPageDIO::GetButtonDIY(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DIY[nIndex]); // Y轴入	
}


CButton* CPageDIO::GetButtonDOZ(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DOZ[nIndex]); // Z轴出	
}

CButton* CPageDIO::GetButtonDIZ(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DIZ[nIndex]); // Z轴入	
}

CButton* CPageDIO::GetButtonDOU(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DOU[nIndex]); // U轴出	
}

CButton* CPageDIO::GetButtonDIU(int nIndex)
{
	return (CButton*)GetDlgItem(BUTTONID_DIU[nIndex]); // U轴入	
}
BOOL CPageDIO::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	// TODO: Add extra initialization here
	CButton* pStartDIO = (CButton*)GetDlgItem(IDC_START_DIO);
	pStartDIO->EnableWindow(TRUE);
	CButton* pStopDIO = (CButton*)GetDlgItem(IDC_STOP_DIO);
	pStopDIO->EnableWindow(FALSE);	
	CButton* pOutComX = (CButton*)GetDlgItem(IDC_RADIO_ComOutX);
	CButton* pOutComY = (CButton*)GetDlgItem(IDC_RADIO_ComOutY);
	CButton* pOutComZ = (CButton*)GetDlgItem(IDC_RADIO_ComOutZ);
	CButton* pOutComU = (CButton*)GetDlgItem(IDC_RADIO_ComOutU);
//	CButton* pOutStatX = (CButton*)GetDlgItem(IDC_RADIO_StatOutX);
	pOutComX->SetCheck(1);
	pOutComY->SetCheck(1);
	pOutComZ->SetCheck(1);
	pOutComU->SetCheck(1);
	gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS] = WTNMC4A_GENERALOUT;
	gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS] = WTNMC4A_GENERALOUT;
	gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS] = WTNMC4A_GENERALOUT;
	gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS] = WTNMC4A_GENERALOUT;
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_XAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS]);
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_YAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS]);
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS]);
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_UAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS]);
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);


	
	int index;

	ULONG ulDOX[8];
	ULONG ulDOY[8];
	ULONG ulDOZ[8];
	ULONG ulDOU[8];
	memcpy(ulDOX, &gl_pADView->m_ParaDO[0], sizeof(ulDOX));
	memcpy(ulDOY, &gl_pADView->m_ParaDO[1], sizeof(ulDOY));
	memcpy(ulDOZ, &gl_pADView->m_ParaDO[2], sizeof(ulDOZ));
	memcpy(ulDOU, &gl_pADView->m_ParaDO[3], sizeof(ulDOU));
	for (index=0; index<8; index++)
	{		
		CButton* pButton = GetButtonDOX(index);
		pButton->SetCheck(ulDOX[index]);				
		SetButtonText(pButton, pButton->GetCheck());
		
		pButton = GetButtonDOY(index);
		pButton->SetCheck(ulDOY[index]);				
		SetButtonText(pButton, pButton->GetCheck());
		
		pButton = GetButtonDOZ(index);
		pButton->SetCheck(ulDOZ[index]);			
		SetButtonText(pButton, pButton->GetCheck());
		
		pButton = GetButtonDOU(index);
		pButton->SetCheck(ulDOU[index]);		 		
		SetButtonText(pButton, pButton->GetCheck());
	}
// 	for (index=0; index<4; index++)
// 	{		
// 		gl_pADView->m_ParaDO[index].OUT0 = 1;
// 		gl_pADView->m_ParaDO[index].OUT1 = 1;
// 		gl_pADView->m_ParaDO[index].OUT2 = 1;
// 		gl_pADView->m_ParaDO[index].OUT3 = 1;
// 		gl_pADView->m_ParaDO[index].OUT4 = 1;
// 		gl_pADView->m_ParaDO[index].OUT5 = 1;
// 		gl_pADView->m_ParaDO[index].OUT6 = 1;		
// 		gl_pADView->m_ParaDO[index].OUT7 = 1;
// 	}
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}


void CPageDIO::OnStartDio() 
{
	gl_pADView->SetTimer(2, 500, NULL); // 启动定时器
    CButton* pStartDIO = (CButton*)GetDlgItem(IDC_START_DIO);
	pStartDIO->EnableWindow(FALSE);
	CButton* pStopDIO = (CButton*)GetDlgItem(IDC_STOP_DIO);
	pStopDIO->EnableWindow(TRUE);

}

void CPageDIO::OnStopDio() 
{
	
	// TODO: Add your control notification handler code here
	gl_pADView->KillTimer(2);	
	CButton* pStartDIO = (CButton*)GetDlgItem(IDC_START_DIO);
	pStartDIO->EnableWindow(TRUE);
	CButton* pStopDIO = (CButton*)GetDlgItem(IDC_STOP_DIO);
	pStopDIO->EnableWindow(FALSE);
}

//-------------------------X轴的开关量输出---------------------------------------------------------
void CPageDIO::OnDox0()
{
	CButton* pButton = GetButtonDOX(0);
//	m_bDOX[0] = pButton->GetCheck();
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT0 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}
void CPageDIO::OnDox1()
{
	CButton* pButton = GetButtonDOX(1);
//	m_bDOX[1] = pButton->GetCheck();
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT1 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox2()
{
	CButton* pButton = GetButtonDOX(2);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT2 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox3()
{
	CButton* pButton = GetButtonDOX(3);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT3 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox4()
{
	CButton* pButton = GetButtonDOX(4);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT4 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox5()
{
	CButton* pButton = GetButtonDOX(5);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT5 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox6()
{
	CButton* pButton = GetButtonDOX(6);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT6 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDox7()
{
	CButton* pButton = GetButtonDOX(7);
	gl_pADView->m_ParaDO[WTNMC4A_XAXIS].OUT7 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_XAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

//-------------------------Y轴的开关量输出---------------------------------------------------------
void CPageDIO::OnDoy0() 
{
	CButton* pButton = GetButtonDOY(0);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT0 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy1() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOY(1);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT1 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy2() 
{
	CButton* pButton = GetButtonDOY(2);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT2 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy3() 
{
	CButton* pButton = GetButtonDOY(3);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT3 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy4() 
{
	CButton* pButton = GetButtonDOY(4);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT4 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy5() 
{
	CButton* pButton = GetButtonDOY(5);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT5 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy6() 
{
	CButton* pButton = GetButtonDOY(6);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT6 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoy7() 
{
	CButton* pButton = GetButtonDOY(7);
	gl_pADView->m_ParaDO[WTNMC4A_YAXIS].OUT7 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_YAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

//-------------------------Z轴的开关量输出---------------------------------------------------------
void CPageDIO::OnDoz0() 
{
	CButton* pButton = GetButtonDOZ(0);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT0 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz1() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(1);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT1 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz2() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(2);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT2 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz3() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(3);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT3 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz4() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(4);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT4 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz5() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(5);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT5 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz6() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(6);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT6 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDoz7() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOZ(7);
	gl_pADView->m_ParaDO[WTNMC4A_ZAXIS].OUT7 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_ZAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou0() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = GetButtonDOU(0);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT0 = pButton->GetCheck();
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou1() 
{
	CButton* pButton = GetButtonDOU(1);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT1 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou2() 
{
	CButton* pButton = GetButtonDOU(2);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT2 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou3() 
{
	CButton* pButton = GetButtonDOU(3);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT3 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou4() 
{
	CButton* pButton = GetButtonDOU(4);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT4 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou5() 
{
	CButton* pButton = GetButtonDOU(5);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT5 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou6() 
{
	CButton* pButton = GetButtonDOU(6);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT6 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);	
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::OnDou7() 
{
	CButton* pButton = GetButtonDOU(7);
	gl_pADView->m_ParaDO[WTNMC4A_UAXIS].OUT7 = pButton->GetCheck(); 
	gl_pADView->SetSwitchOut(WTNMC4A_UAXIS);
	SetButtonText(pButton, pButton->GetCheck());
}

void CPageDIO::RefreshButton(PWTNMC4A_PARA_RR3 RR3, PWTNMC4A_PARA_RR4 RR4)
{
	GetButtonDIX(0)->SetCheck(RR3->XIN0);   // 外部停止信号XIN0的电平状态
	SetButtonText(GetButtonDIX(0), RR3->XIN0);
	GetButtonDIX(1)->SetCheck(RR3->XIN1);   // 外部停止信号XIN1的电平状态
	SetButtonText(GetButtonDIX(1), RR3->XIN1);
	GetButtonDIX(2)->SetCheck(RR3->XIN2);   // 外部停止信号XIN2的电平状态
	SetButtonText(GetButtonDIX(2), RR3->XIN2);
	GetButtonDIX(3)->SetCheck(RR3->XIN3);   // 外部停止信号XIN3的电平状态
	SetButtonText(GetButtonDIX(3), RR3->XIN3);
	GetButtonDIX(4)->SetCheck(RR3->XEXPP);  // 外部正方向点动输入信号XEXPP的电平状态
	SetButtonText(GetButtonDIX(4), RR3->XEXPP);
	GetButtonDIX(5)->SetCheck(RR3->XEXPM);  // 外部反方向点动输入信号XEXPM的电平状态
	SetButtonText(GetButtonDIX(5), RR3->XEXPM);
	GetButtonDIX(6)->SetCheck(RR3->XINPOS); // 外部伺服电机到位信号XINPOS的电平状态
	SetButtonText(GetButtonDIX(6), RR3->XINPOS);
	GetButtonDIX(7)->SetCheck(RR3->XALARM); // 外部伺服马达报警信号XALARM的电平状态 
	SetButtonText(GetButtonDIX(7), RR3->XALARM);

	GetButtonDIY(0)->SetCheck(RR3->YIN0);   // 外部停止信号YIN0的电平状态
	SetButtonText(GetButtonDIY(0), RR3->XIN0);
	GetButtonDIY(1)->SetCheck(RR3->YIN1);   // 外部停止信号YIN1的电平状态
	SetButtonText(GetButtonDIY(1), RR3->YIN1);
	GetButtonDIY(2)->SetCheck(RR3->YIN2);   // 外部停止信号YIN2的电平状态
	SetButtonText(GetButtonDIY(2), RR3->YIN2);
	GetButtonDIY(3)->SetCheck(RR3->YIN3);   // 外部停止信号YIN3的电平状态
	SetButtonText(GetButtonDIY(3), RR3->YIN3);
	GetButtonDIY(4)->SetCheck(RR3->YEXPP);  // 外部正方向点动输入信号YEXPP的电平状态
	SetButtonText(GetButtonDIY(4), RR3->YEXPP);
	GetButtonDIY(5)->SetCheck(RR3->YEXPM);  // 外部反方向点动输入信号YEXPM的电平状态
	SetButtonText(GetButtonDIY(5), RR3->YEXPM);
	GetButtonDIY(6)->SetCheck(RR3->YINPOS); // 外部伺服电机到位信号YINPOS的电平状态
	SetButtonText(GetButtonDIY(6), RR3->YINPOS);
	GetButtonDIY(7)->SetCheck(RR3->YALARM); // 外部伺服马达报警信号YALARM的电平状态 
	SetButtonText(GetButtonDIY(7), RR3->YALARM);

	GetButtonDIZ(0)->SetCheck(RR4->ZIN0);   // 外部停止信号ZIN0的电平状态
	SetButtonText(GetButtonDIZ(0), RR4->ZIN0);
	GetButtonDIZ(1)->SetCheck(RR4->ZIN1);   // 外部停止信号ZIN1的电平状态
	SetButtonText(GetButtonDIZ(1), RR4->ZIN1);
	GetButtonDIZ(2)->SetCheck(RR4->ZIN2);   // 外部停止信号ZIN2的电平状态
	SetButtonText(GetButtonDIZ(2), RR4->ZIN2);
	GetButtonDIZ(3)->SetCheck(RR4->ZIN3);   // 外部停止信号ZIN3的电平状态
	SetButtonText(GetButtonDIZ(3), RR4->ZIN3);
	GetButtonDIZ(4)->SetCheck(RR4->ZEXPP);  // 外部正方向点动输入信号ZEXPP的电平状态
	SetButtonText(GetButtonDIZ(4), RR4->ZEXPP);
	GetButtonDIZ(5)->SetCheck(RR4->ZEXPM);  // 外部反方向点动输入信号ZEXPM的电平状态
	SetButtonText(GetButtonDIZ(5), RR4->ZEXPM);
	GetButtonDIZ(6)->SetCheck(RR4->ZINPOS); // 外部伺服电机到位信号ZINPOS的电平状态
	SetButtonText(GetButtonDIZ(6), RR4->ZINPOS);
	GetButtonDIZ(7)->SetCheck(RR4->ZALARM); // 外部伺服马达报警信号ZALARM的电平状态 
	SetButtonText(GetButtonDIZ(7), RR4->ZALARM);

	GetButtonDIU(0)->SetCheck(RR4->UIN0);   // 外部停止信号UIN0的电平状态
	SetButtonText(GetButtonDIU(0), RR4->UIN0);
	GetButtonDIU(1)->SetCheck(RR4->UIN1);   // 外部停止信号UIN1的电平状态
	SetButtonText(GetButtonDIU(1), RR4->UIN1);
	GetButtonDIU(2)->SetCheck(RR4->UIN2);   // 外部停止信号UIN2的电平状态
	SetButtonText(GetButtonDIU(2), RR4->UIN2);
	GetButtonDIU(3)->SetCheck(RR4->UIN3);   // 外部停止信号UIN3的电平状态
	SetButtonText(GetButtonDIU(3), RR4->UIN3);
	GetButtonDIU(4)->SetCheck(RR4->UEXPP);  // 外部正方向点动输入信号UEXPP的电平状态
	SetButtonText(GetButtonDIU(4), RR4->UEXPP);
	GetButtonDIU(5)->SetCheck(RR4->UEXPM);  // 外部反方向点动输入信号UEXPM的电平状态
	SetButtonText(GetButtonDIU(5), RR4->UEXPM);
	GetButtonDIU(6)->SetCheck(RR4->UINPOS); // 外部伺服电机到位信号UINPOS的电平状态
	SetButtonText(GetButtonDIU(6), RR4->UINPOS);
	GetButtonDIU(7)->SetCheck(RR4->UALARM); // 外部伺服马达报警信号UALARM的电平状态 
	SetButtonText(GetButtonDIU(7), RR4->UALARM);
}


void CPageDIO::SetButtonText(CButton* pButton, BOOL bFlag)
{
	CString str;
	pButton->GetWindowText(str);
	if(bFlag) // 高电平
	{
		str.Replace(L"低", L"高");
		pButton->SetWindowText(str);
	}
	else
	{
		str.Replace(L"高", L"低");
		pButton->SetWindowText(str);
	}
}

void CPageDIO::OnRADIOComOutX() // X轴通用输出
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS] = WTNMC4A_GENERALOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOX(Index)->ShowWindow(SW_SHOW);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_XAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS]);

}

void CPageDIO::OnRADIOStatOutX()  // X轴状态输出
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS] = WTNMC4A_STATUSOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOX(Index)->ShowWindow(SW_HIDE);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_XAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_XAXIS]);
}

void CPageDIO::OnRADIOComOutY() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS] = WTNMC4A_GENERALOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOY(Index)->ShowWindow(SW_SHOW);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_YAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS]);	
}

void CPageDIO::OnRADIOStatOutY() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS] = WTNMC4A_STATUSOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOY(Index)->ShowWindow(SW_HIDE);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_YAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_YAXIS]);		
}

void CPageDIO::OnRADIOComOutZ() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS] = WTNMC4A_GENERALOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOZ(Index)->ShowWindow(SW_SHOW);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS]);	
}

void CPageDIO::OnRADIOStatOutZ() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS] = WTNMC4A_STATUSOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOZ(Index)->ShowWindow(SW_HIDE);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_ZAXIS]);	
}

void CPageDIO::OnRADIOComOutU() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS] = WTNMC4A_GENERALOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOU(Index)->ShowWindow(SW_SHOW);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_UAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS]);	
}

void CPageDIO::OnRADIOStatOutU() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS] = WTNMC4A_STATUSOUT;
	for(int Index=4; Index<8; Index++)
	{
		GetButtonDOU(Index)->ShowWindow(SW_HIDE);
	}
	WTNMC4A_OutSwitch(gl_pADView->m_hDevice, WTNMC4A_UAXIS, gl_pADView->m_StatusGeneralOut[WTNMC4A_UAXIS]);	
}
