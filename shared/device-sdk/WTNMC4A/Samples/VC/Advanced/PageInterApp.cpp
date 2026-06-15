// PageInterApp.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageInterApp.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
extern BOOL gl_bSequenceRun;
extern BOOL gl_bBitProc;
extern BOOL gl_bBitDataProc;

extern BOOL gl_hExitEvent;
extern BOOL gl_bStop;
extern BOOL gl_bBitStop;
extern BOOL gl_bSequenceStop;
/////////////////////////////////////////////////////////////////////////////
// CPageInterApp property page

IMPLEMENT_DYNCREATE(CPageInterApp, CPropertyPage)

CPageInterApp::CPageInterApp() : CPropertyPage(CPageInterApp::IDD)
{
	//{{AFX_DATA_INIT(CPageInterApp)
		// NOTE: the ClassWizard will add member initialization here
	//}}AFX_DATA_INIT
}

CPageInterApp::~CPageInterApp()
{
}

void CPageInterApp::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageInterApp)
	DDX_Control(pDX, IDC_IntStartBit, m_IntStartBit);
	DDX_Control(pDX, IDC_IntSttopBit, m_IntStopBit);
	DDX_Control(pDX, IDC_Stop_Sequence, m_StopSequence);
	DDX_Control(pDX, IDC_Start_Sequence, m_StartSequence);
	DDX_Control(pDX, IDC_Stop_Bit, m_StopBit);
	DDX_Control(pDX, IDC_Start_Bit, m_StartBit);
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageInterApp, CPropertyPage)
	//{{AFX_MSG_MAP(CPageInterApp)
	ON_BN_CLICKED(IDC_Start_Bit, OnStartBit)
	ON_BN_CLICKED(IDC_Start_Sequence, OnStartSequence)
	ON_BN_CLICKED(IDC_IntStartBit, OnIntStartBit)
	ON_BN_CLICKED(IDC_Stop_Bit, OnStopBit)
	ON_BN_CLICKED(IDC_Stop_Sequence, OnStopSequence)
	ON_BN_CLICKED(IDC_IntSttopBit, OnIntSttopBit)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageInterApp message handlers

BOOL CPageInterApp::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	// TODO: Add extra initialization here
	m_StopBit.EnableWindow(FALSE);
	m_StopSequence.EnableWindow(FALSE);
	m_IntStopBit.EnableWindow(FALSE);
	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}
//开始位插补（六边形）

void CPageInterApp::OnStartBit() 
{
	// TODO: Add your control notification handler code here
	gl_pADView->StartINTBitInterpMovement();

// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	m_StartBit.EnableWindow(FALSE);
	m_StopBit.EnableWindow(TRUE);
}

// 开始连续插补
void CPageInterApp::OnStartSequence() 
{
	// TODO: Add your control notification handler code here
//	WTNMC4A_EnableSerInterpolation(gl_pADView->m_hDevice, TRUE);// 连续插补使能

// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	gl_pADView->StartSequenceMovement();
	m_StartSequence.EnableWindow(FALSE);
	m_StopSequence.EnableWindow(TRUE);
}

// 开始位插补（心形）
void CPageInterApp::OnIntStartBit() 
{
	// TODO: Add your control notification handler code here
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice,WTNMC4A_XAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_YAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_ZAXIS, 1, 0);		// 上下脉冲方式
// 	WTNMC4A_SetEncoderSignalType(gl_pADView->m_hDevice, WTNMC4A_UAXIS, 1, 0);		// 上下脉冲方式
	gl_pADView->StartBitInterpMovement();
	m_IntStartBit.EnableWindow(FALSE);
	m_IntStopBit.EnableWindow(TRUE);
}

void CPageInterApp::OnStopBit() 
{
	// TODO: Add your control notification handler code here
//	gl_pADView->DecStop(WTNMC4A_ALLAXIS);
	gl_bBitStop = FALSE;
	gl_bBitDataProc = FALSE;
	while (!gl_bBitStop)
	{
		Sleep(1);
	}
	
	gl_bBitStop = FALSE;

	gl_pADView->ImmediateStop(WTNMC4A_ALLAXIS);
	m_StartBit.EnableWindow(TRUE);
	m_StopBit.EnableWindow(FALSE);

		
}

void CPageInterApp::OnStopSequence() 
{
	// TODO: Add your control notification handler code here
//	gl_pADView->DecStop(WTNMC4A_ALLAXIS);
	gl_bSequenceRun = FALSE;
	gl_bSequenceStop = FALSE;
	
	while (!gl_bSequenceStop)
	{
		Sleep(1);
	}
	gl_bSequenceStop = FALSE;

	gl_pADView->ImmediateStop(WTNMC4A_ALLAXIS);
	m_StartSequence.EnableWindow(TRUE);
	m_StopSequence.EnableWindow(FALSE);
//	WTNMC4A_EnableSerInterpolation(gl_pADView->m_hDevice, FALSE);
}

void CPageInterApp::OnIntSttopBit() 
{
	// TODO: Add your control notification handler code here
//	gl_pADView->DecStop(WTNMC4A_ALLAXIS);
	gl_bStop = FALSE;
	gl_bBitProc = FALSE;
	//USB5953_ReleaseSystemEvent(gl_hEvent); // 释放消息
//	WaitForSingleObject(gl_hExitEvent, 100);

 	while (!gl_bStop)
 	{
 		Sleep(1);
 	}

	gl_bStop = FALSE;

//	WTNMC4A_SetBP_2D(gl_pADView->m_hDevice, 0, 0, 0, 0);// 设置任意2轴位插补数据
	gl_pADView->ImmediateStop(WTNMC4A_ALLAXIS);
	m_IntStartBit.EnableWindow(TRUE);
	m_IntStopBit.EnableWindow(FALSE);
}
