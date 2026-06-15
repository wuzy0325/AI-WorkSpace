// PageSynchronX.cpp : implementation file
//

#include "stdafx.h"
#include "sys.h"
#include "PageSynchron.h"
#include "ADView.h"
#ifdef _DEBUG
#define new DEBUG_NEW
#undef THIS_FILE
static char THIS_FILE[] = __FILE__;
#endif
extern CADView* gl_pADView;
/////////////////////////////////////////////////////////////////////////////
// CPageSynchron property page

IMPLEMENT_DYNCREATE(CPageSynchron, CPropertyPage)

CPageSynchron::CPageSynchron() : CPropertyPage(CPageSynchron::IDD)
{
	//{{AFX_DATA_INIT(CPageSynchron)
		// NOTE: the ClassWizard will add member initialization here
	m_nAxisNum = 0;
	//}}AFX_DATA_INIT
}

CPageSynchron::~CPageSynchron()
{
}

void CPageSynchron::DoDataExchange(CDataExchange* pDX)
{
	CPropertyPage::DoDataExchange(pDX);
	//{{AFX_DATA_MAP(CPageSynchron)
		// NOTE: the ClassWizard will add DDX and DDV calls here
	//}}AFX_DATA_MAP
}


BEGIN_MESSAGE_MAP(CPageSynchron, CPropertyPage)
	//{{AFX_MSG_MAP(CPageSynchron)
	ON_BN_CLICKED(IDC_CHECK_AXIS1_X, OnCheckAxis1)
	ON_BN_CLICKED(IDC_CHECK_AXIS2_X, OnCheckAxis2)
	ON_BN_CLICKED(IDC_CHECK_AXIS3_X, OnCheckAxis3)
	ON_BN_CLICKED(IDC_CHECK_CMD_X, OnCheckCmdX)
	ON_BN_CLICKED(IDC_CHECK_LPRD_X, OnCheckLprdX)
	ON_BN_CLICKED(IDC_CHECK_DSTA_X, OnCheckDstaX)
	ON_BN_CLICKED(IDC_CHECK_DEND_X, OnCheckDendX)
	ON_BN_CLICKED(IDC_RADIO_LPX, OnRadioLpx)
	ON_BN_CLICKED(IDC_RADIO_EPX, OnRadioEpx)
	ON_BN_CLICKED(IDC_CHECK_PBCP_X, OnCheckPbcpX)
	ON_BN_CLICKED(IDC_CHECK_PSCP_X, OnCheckPscpX)
	ON_BN_CLICKED(IDC_CHECK_PBCM_X, OnCheckPbcmX)
	ON_BN_CLICKED(IDC_CHECK_PSCM_X, OnCheckPscmX)
	ON_BN_CLICKED(IDC_CHECK_IN3LH_X, OnCheckIn3lhX)
	ON_BN_CLICKED(IDC_CHECK_IN3HL_X, OnCheckIn3hlX)
	ON_BN_CLICKED(IDC_CHECK_FDRVP_X, OnCheckFdrvpX)
	ON_BN_CLICKED(IDC_CHECK_FDRVM_X, OnCheckFdrvmX)
	ON_BN_CLICKED(IDC_CHECK_CDRVP_X, OnCheckCdrvpX)
	ON_BN_CLICKED(IDC_CHECK_CDRVM_X, OnCheckCdrvmX)
	ON_BN_CLICKED(IDC_CHECK_SSTOP_X, OnCheckSstopX)
	ON_BN_CLICKED(IDC_CHECK_ISTOP_X, OnCheckIstopX)
	ON_BN_CLICKED(IDC_CHECK_LPSAV_X, OnCheckLpsavX)
	ON_BN_CLICKED(IDC_CHECK_EPSAV_X, OnCheckEpsavX)
	ON_BN_CLICKED(IDC_CHECK_LPSET_X, OnCheckLpsetX)
	ON_BN_CLICKED(IDC_CHECK_OPSET_X, OnCheckOpsetX)
	ON_BN_CLICKED(IDC_CHECK_VLSET_X, OnCheckVlsetX)
	ON_BN_CLICKED(IDC_CHECK_OUTN_X, OnCheckOutnX)
	ON_BN_CLICKED(IDC_CHECK_INTN_X, OnCheckIntnX)
	ON_BN_CLICKED(IDC_CHECK_EPSET_X, OnCheckEpsetX)
	//}}AFX_MSG_MAP
END_MESSAGE_MAP()

/////////////////////////////////////////////////////////////////////////////
// CPageSynchron message handlers
// 指定Y轴为同步轴
void CPageSynchron::OnCheckAxis1() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = (CButton*)GetDlgItem(IDC_CHECK_AXIS1_X);
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].AXIS1 = pButton->GetCheck();
}

// 指定Z轴为同步轴
void CPageSynchron::OnCheckAxis2() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = (CButton*)GetDlgItem(IDC_CHECK_AXIS2_X);
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].AXIS2 = pButton->GetCheck();
}

// 指定U轴为同步轴
void CPageSynchron::OnCheckAxis3() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = (CButton*)GetDlgItem(IDC_CHECK_AXIS3_X);
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].AXIS3 = pButton->GetCheck();
	
}

// 当启动当前轴同步动作时,启动同步
void CPageSynchron::OnCheckCmdX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = (CButton*)GetDlgItem(IDC_CHECK_CMD_X);
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].CMD = pButton->GetCheck();

}

// 读逻辑位置计数器LP时,启动同步
void CPageSynchron::OnCheckLprdX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButton = (CButton*)GetDlgItem(IDC_CHECK_LPRD_X);
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].LPRD = pButton->GetCheck();
	
}

// 当驱动开始时,启动同步动作
void CPageSynchron::OnCheckDstaX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonDSTA = (CButton*)GetDlgItem(IDC_CHECK_DSTA_X);
	CButton* pButtonDEND = (CButton*)GetDlgItem(IDC_CHECK_DEND_X);
	int nCheck = pButtonDSTA->GetCheck();
	if(nCheck)
	{
		pButtonDEND->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].DEND = 0;
		
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].DSTA = nCheck;
	
}

// 当驱动结束时,启动同步动作
void CPageSynchron::OnCheckDendX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonDSTA = (CButton*)GetDlgItem(IDC_CHECK_DSTA_X);
	CButton* pButtonDEND = (CButton*)GetDlgItem(IDC_CHECK_DEND_X);
	int nCheck = pButtonDEND->GetCheck();
	if(nCheck)
	{
		pButtonDSTA->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].DSTA = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].DEND = nCheck;
}

// 以逻辑计数器LP为比较对象
void CPageSynchron::OnRadioLpx() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonLP = (CButton*)GetDlgItem(IDC_RADIO_LPX);
	CButton* pButtonEP = (CButton*)GetDlgItem(IDC_RADIO_EPX);
	pButtonLP->SetCheck(1); 
	pButtonEP->SetCheck(0);
	gl_pADView->m_nLPEP[m_nAxisNum] = WTNMC4A_LOGIC;
}

// 以实位计数器EP为比较对象
void CPageSynchron::OnRadioEpx() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonLP = (CButton*)GetDlgItem(IDC_RADIO_LPX);
	CButton* pButtonEP = (CButton*)GetDlgItem(IDC_RADIO_EPX);
	pButtonLP->SetCheck(0); 
	pButtonEP->SetCheck(1);
	gl_pADView->m_nLPEP[m_nAxisNum] = WTNMC4A_FACT;
}

// 当LP/EP值 >= COMP+值时,启动同步
void CPageSynchron::OnCheckPbcpX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonPBCP = (CButton*)GetDlgItem(IDC_CHECK_PBCP_X); // LP/EP > COMP+
	CButton* pButtonPSCP = (CButton*)GetDlgItem(IDC_CHECK_PSCP_X); // LP/EP < COMP+
	int nCheck = pButtonPBCP->GetCheck();
	if(nCheck)
	{
		pButtonPSCP->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PSCP = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PBCP = nCheck;
}

// 当LP/EP值 < COMP+值时,启动同步
void CPageSynchron::OnCheckPscpX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonPBCP = (CButton*)GetDlgItem(IDC_CHECK_PBCP_X); // LP/EP > COMP+
	CButton* pButtonPSCP = (CButton*)GetDlgItem(IDC_CHECK_PSCP_X); // LP/EP < COMP+
	int nCheck = pButtonPSCP->GetCheck();
	if(nCheck)
	{
		pButtonPBCP->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PBCP = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PSCP = nCheck;
	
}

// 当LP/EP值 >= COMP-值,启动同步
void CPageSynchron::OnCheckPbcmX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonPBCM = (CButton*)GetDlgItem(IDC_CHECK_PBCM_X); // LP/EP > COMP-
	CButton* pButtonPSCM = (CButton*)GetDlgItem(IDC_CHECK_PSCM_X); // LP/EP < COMP-
	int nCheck = pButtonPBCM->GetCheck();
	if(nCheck)
	{
		pButtonPSCM->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PSCM = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PBCM = nCheck;
	
}

// 当LP/EP值 < COMP-值,启动同步
void CPageSynchron::OnCheckPscmX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonPBCM = (CButton*)GetDlgItem(IDC_CHECK_PBCM_X); // LP/EP > COMP-
	CButton* pButtonPSCM = (CButton*)GetDlgItem(IDC_CHECK_PSCM_X); // LP/EP < COMP-
	int nCheck = pButtonPSCM->GetCheck();
	if(nCheck)
	{
		pButtonPBCM->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PBCM = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].PSCM = nCheck;
	
}

// 当IN3出现上升沿时,启动同步动作
void CPageSynchron::OnCheckIn3lhX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonIN3LH = (CButton*)GetDlgItem(IDC_CHECK_IN3LH_X); // 上升沿
	CButton* pButtonIN3HL = (CButton*)GetDlgItem(IDC_CHECK_IN3HL_X); // 下降沿
	int nCheck = pButtonIN3LH->GetCheck();
	if(nCheck)
	{
		pButtonIN3HL->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].IN3HL = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].IN3LH = nCheck;
}

// 当IN3出现下降沿时,启动同步动作
void CPageSynchron::OnCheckIn3hlX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonIN3LH = (CButton*)GetDlgItem(IDC_CHECK_IN3LH_X); // 上升沿
	CButton* pButtonIN3HL = (CButton*)GetDlgItem(IDC_CHECK_IN3HL_X); // 下降沿
	int nCheck = pButtonIN3HL->GetCheck();
	if(nCheck)
	{
		pButtonIN3LH->SetCheck(0);
		gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].IN3LH = 0;
	}
	gl_pADView->m_SynchronActionOwnAxis[m_nAxisNum].IN3HL = nCheck;	
}

BOOL CPageSynchron::OnInitDialog() 
{
	CPropertyPage::OnInitDialog();
	
	// TODO: Add extra initialization here
	CButton* pButtonOUTN = (CButton*)GetDlgItem(IDC_CHECK_OUTN_X);
	pButtonOUTN->ShowWindow(SW_HIDE);
	CButton* pButtonLP = (CButton*)GetDlgItem(IDC_RADIO_LPX);
	CButton* pButtonEP = (CButton*)GetDlgItem(IDC_RADIO_EPX);
	pButtonLP->SetCheck(1); 
	pButtonEP->SetCheck(0);

	return TRUE;  // return TRUE unless you set the focus to a control
	              // EXCEPTION: OCX Property Pages should return FALSE
}

// 启动正方向定长驱动
void CPageSynchron::OnCheckFdrvpX() 
{
	// TODO: Add your control notification handler code here
	CButton *pButtonFDRVP = (CButton*)GetDlgItem(IDC_CHECK_FDRVP_X); // 正方向定长
	CButton *pButtonFDRVM = (CButton*)GetDlgItem(IDC_CHECK_FDRVM_X); // 反方向定长
	CButton *pButtonCDRVP = (CButton*)GetDlgItem(IDC_CHECK_CDRVP_X); // 正方向连续
	CButton *pButtonCDRVM = (CButton*)GetDlgItem(IDC_CHECK_CDRVM_X); // 反方向连续
	int nCheck = pButtonFDRVP->GetCheck();
	if(nCheck)
	{
		pButtonFDRVM->SetCheck(0);
		pButtonCDRVP->SetCheck(0);
		pButtonCDRVM->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVM = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVP = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVM = 0;
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVP = nCheck;
}

// 启动反方向定长驱动
void CPageSynchron::OnCheckFdrvmX() 
{
	// TODO: Add your control notification handler code here
	CButton *pButtonFDRVP = (CButton*)GetDlgItem(IDC_CHECK_FDRVP_X); // 正方向定长
	CButton *pButtonFDRVM = (CButton*)GetDlgItem(IDC_CHECK_FDRVM_X); // 反方向定长
	CButton *pButtonCDRVP = (CButton*)GetDlgItem(IDC_CHECK_CDRVP_X); // 正方向连续
	CButton *pButtonCDRVM = (CButton*)GetDlgItem(IDC_CHECK_CDRVM_X); // 反方向连续
	int nCheck = pButtonFDRVM->GetCheck();
	if(nCheck)
	{
		pButtonFDRVP->SetCheck(0);
		pButtonCDRVP->SetCheck(0);
		pButtonCDRVM->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVP = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVP = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVM = 0;
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVM = nCheck;	
}

// 启动正方向连续驱动
void CPageSynchron::OnCheckCdrvpX() 
{
	// TODO: Add your control notification handler code here
	CButton *pButtonFDRVP = (CButton*)GetDlgItem(IDC_CHECK_FDRVP_X); // 正方向定长
	CButton *pButtonFDRVM = (CButton*)GetDlgItem(IDC_CHECK_FDRVM_X); // 反方向定长
	CButton *pButtonCDRVP = (CButton*)GetDlgItem(IDC_CHECK_CDRVP_X); // 正方向连续
	CButton *pButtonCDRVM = (CButton*)GetDlgItem(IDC_CHECK_CDRVM_X); // 反方向连续
	int nCheck = pButtonCDRVP->GetCheck();
	if(nCheck)
	{
		pButtonFDRVP->SetCheck(0);
		pButtonFDRVM->SetCheck(0);
		pButtonCDRVM->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVP = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVM = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVM = 0;
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVP = nCheck;	
	
}

// 启动反方向连续驱动
void CPageSynchron::OnCheckCdrvmX() 
{
	// TODO: Add your control notification handler code here
	CButton *pButtonFDRVP = (CButton*)GetDlgItem(IDC_CHECK_FDRVP_X); // 正方向定长
	CButton *pButtonFDRVM = (CButton*)GetDlgItem(IDC_CHECK_FDRVM_X); // 反方向定长
	CButton *pButtonCDRVP = (CButton*)GetDlgItem(IDC_CHECK_CDRVP_X); // 正方向连续
	CButton *pButtonCDRVM = (CButton*)GetDlgItem(IDC_CHECK_CDRVM_X); // 反方向连续
	int nCheck = pButtonCDRVM->GetCheck();
	if(nCheck)
	{
		pButtonFDRVP->SetCheck(0);
		pButtonFDRVM->SetCheck(0);
		pButtonCDRVP->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVP = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].FDRVM = 0;
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVP = 0;
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].CDRVM = nCheck;		
}

// 减速停止
void CPageSynchron::OnCheckSstopX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonSSTOP = (CButton*)GetDlgItem(IDC_CHECK_SSTOP_X);
	CButton* pButtonISTOP = (CButton*)GetDlgItem(IDC_CHECK_ISTOP_X);
	int nCheck = pButtonSSTOP->GetCheck();
	if(nCheck)
	{
		pButtonISTOP->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].ISTOP = 0;
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].SSTOP = nCheck;		
}

// 立即停止
void CPageSynchron::OnCheckIstopX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonSSTOP = (CButton*)GetDlgItem(IDC_CHECK_SSTOP_X);
	CButton* pButtonISTOP = (CButton*)GetDlgItem(IDC_CHECK_ISTOP_X);
	int nCheck = pButtonISTOP->GetCheck();
	if(nCheck)
	{
		pButtonSSTOP->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].SSTOP = 0;	
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].ISTOP = nCheck;
}

// 把当前LP值保存到同步缓冲寄存器BR
void CPageSynchron::OnCheckLpsavX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonLPSAV = (CButton*)GetDlgItem(IDC_CHECK_LPSAV_X); // LP->BR
	CButton* pButtonEPSAV = (CButton*)GetDlgItem(IDC_CHECK_EPSAV_X); // EP->BR
	int nCheck = pButtonLPSAV->GetCheck();
	if(nCheck)
	{
		pButtonEPSAV->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].EPSAV = 0;	
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].LPSAV = nCheck;
	
}

// 把当前EP值保存到同步缓冲寄存器BR
void CPageSynchron::OnCheckEpsavX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonLPSAV = (CButton*)GetDlgItem(IDC_CHECK_LPSAV_X); // LP->BR
	CButton* pButtonEPSAV = (CButton*)GetDlgItem(IDC_CHECK_EPSAV_X); // EP->BR
	int nCheck = pButtonEPSAV->GetCheck();
	if(nCheck)
	{
		pButtonLPSAV->SetCheck(0);
		gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].LPSAV = 0;	
	}
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].EPSAV = nCheck;
	
}

// 把WR6和WR7的值设定到逻辑寄存器LP中
void CPageSynchron::OnCheckLpsetX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonLPSET = (CButton*)GetDlgItem(IDC_CHECK_LPSET_X); // WR6&WR7->LP
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].LPSET = pButtonLPSET->GetCheck();
	
}

// 把WR6和WR7的值设定到逻辑寄存器EP中
void CPageSynchron::OnCheckEpsetX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonEPSET = (CButton*)GetDlgItem(IDC_CHECK_EPSET_X); // WR6&WR7->EP
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].EPSET = pButtonEPSET->GetCheck();
	
}

// 把WR6和WR7的值设定到脉冲寄存器P中
void CPageSynchron::OnCheckOpsetX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonOPSET = (CButton*)GetDlgItem(IDC_CHECK_OPSET_X); // WR6&WR7->P
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].OPSET = pButtonOPSET->GetCheck();
}

// 把WR6的值设定为驱动速度V
void CPageSynchron::OnCheckVlsetX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonVLSET = (CButton*)GetDlgItem(IDC_CHECK_VLSET_X); // WR6->V
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].VLSET = pButtonVLSET->GetCheck();
}

// 用nDCC的引脚输出同步脉冲
void CPageSynchron::OnCheckOutnX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonOUTN = (CButton*)GetDlgItem(IDC_CHECK_OUTN_X); // nDCC
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].OUTN = pButtonOUTN->GetCheck();
}

// 产生中断
void CPageSynchron::OnCheckIntnX() 
{
	// TODO: Add your control notification handler code here
	CButton* pButtonINTN = (CButton*)GetDlgItem(IDC_CHECK_INTN_X); // nDCC
	gl_pADView->m_SynchronActionOtherAxis[m_nAxisNum].INTN = pButtonINTN->GetCheck();
}

// 设置轴号
void CPageSynchron::SetAxisNum(int nAxisNum)
{
	m_nAxisNum = nAxisNum;
	CButton* pButtonAxis1 = (CButton*)GetDlgItem(IDC_CHECK_AXIS1_X);
	CButton* pButtonAxis2 = (CButton*)GetDlgItem(IDC_CHECK_AXIS2_X);
	CButton* pButtonAxis3 = (CButton*)GetDlgItem(IDC_CHECK_AXIS3_X);
	switch(m_nAxisNum)
	{
	case WTNMC4A_XAXIS: // X轴
		pButtonAxis1->SetWindowText(L"指定Y轴为同步轴");
		pButtonAxis2->SetWindowText(L"指定Z轴为同步轴");
		pButtonAxis3->SetWindowText(L"指定U轴为同步轴");
		break;
	case WTNMC4A_YAXIS: // Y轴
		pButtonAxis1->SetWindowText(L"指定Z轴为同步轴");
		pButtonAxis2->SetWindowText(L"指定U轴为同步轴");
		pButtonAxis3->SetWindowText(L"指定X轴为同步轴");
		break;
	case WTNMC4A_ZAXIS: // Z轴
		pButtonAxis1->SetWindowText(L"指定U轴为同步轴");
		pButtonAxis2->SetWindowText(L"指定X轴为同步轴");
		pButtonAxis3->SetWindowText(L"指定Y轴为同步轴");
		break;
	case WTNMC4A_UAXIS: // U轴
		pButtonAxis1->SetWindowText(L"指定X轴为同步轴");
		pButtonAxis2->SetWindowText(L"指定Y轴为同步轴");
		pButtonAxis3->SetWindowText(L"指定Z轴为同步轴");
		break;
	default:
		break;
	}
}
