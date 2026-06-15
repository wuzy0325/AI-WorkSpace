// SysDoc.cpp :  CSysDoc 类的实现
//

#include "stdafx.h"
#include "Sys.h"

#include "SysDoc.h"
#include ".\sysdoc.h"

#include "ChildFrm.h"

#ifdef _DEBUG
#define new DEBUG_NEW
#endif


// CSysDoc

IMPLEMENT_DYNCREATE(CSysDoc, CDocument)

BEGIN_MESSAGE_MAP(CSysDoc, CDocument)
	ON_COMMAND(ID_FILE_SAVE, OnFileSave)
END_MESSAGE_MAP()


// CSysDoc 构造/析构

CSysDoc::CSysDoc()
{
	// TODO: 在此添加一次性构造代码

}

CSysDoc::~CSysDoc()
{
}

BOOL CSysDoc::OnNewDocument()
{
	if (!CDocument::OnNewDocument())
		return FALSE;
	// TODO: 在此添加重新初始化代码
	// (SDI 文档将重用该文档)	
	return TRUE;
}




// CSysDoc 序列化

void CSysDoc::Serialize(CArchive& ar)
{	
	if (ar.IsStoring())
	{	

	}
	else
	{	

	}
}


// CSysDoc 诊断

#ifdef _DEBUG
void CSysDoc::AssertValid() const
{
	CDocument::AssertValid();
}

void CSysDoc::Dump(CDumpContext& dc) const
{
	CDocument::Dump(dc);
}
#endif //_DEBUG


// CSysDoc 命令

BOOL CSysDoc::OnSaveDocument(LPCTSTR lpszPathName)
{
	// TODO: 在此添加专用代码和/或调用基类
	POSITION pos;
	pos = this->GetFirstViewPosition();
	CChildFrame* pFrm  = NULL;	
	if (pos != NULL)
	{
		CView* pView = this->GetNextView(pos);
		pFrm = (CChildFrame*)pView->GetParentFrame();		

		CStdioFile fileSave(lpszPathName, CFile::modeCreate|CFile::modeWrite);		
		this->SetModifiedFlag(FALSE);
		return pFrm->m_Channel.SaveToFile(fileSave);	
	}

	return FALSE;
}

BOOL CSysDoc::OnOpenDocument(LPCTSTR lpszPathName)
{
	POSITION pos;
	pos = this->GetFirstViewPosition();
	CChildFrame* pFrm  = NULL;	
	if (pos != NULL)
	{
		CView* pView = this->GetNextView(pos);
		pFrm = (CChildFrame*)pView->GetParentFrame();		

		CStdioFile fileSave(lpszPathName, CFile::modeRead);

		if (fileSave.m_pStream)
		{
			pFrm->m_Channel.LoadTxtFileData(fileSave);	
			// 更新波形视图
			pFrm->UpdataSegmentViewScrSizes();
			// 更新段信息列表
			pFrm->m_pChCfgView->InitListSegment();
			this->SetModifiedFlag(FALSE);
			theApp.AddToRecentFileList(lpszPathName);
			return TRUE;
		}
		else
			return FALSE;		
	}	

	return FALSE;
}

void CSysDoc::OnFileSave()
{
	CDocument::OnFileSave();
}
