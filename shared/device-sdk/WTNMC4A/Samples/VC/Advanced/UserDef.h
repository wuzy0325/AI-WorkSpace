// Usedef.h
typedef struct _OTHERSPARA 
{
	USHORT  LogicFact;		// 逻辑或实际
	LONG	UpperLimit;		// 限位上限
	LONG	LowerLimit;		// 限位下限
	LONG    HandDecPulse;   // 手动减速点
	LONG    AccOffset;      // 加速计数偏移值
}OTHERPARA, *POTHERPARA;


#define WM_DRAW_LINE WM_USER + 1
#define WM_DRAW_LINEINTERPOLATION WM_USER + 2
#define WM_DRAW_CIRCLE WM_USER + 3
#define	WM_DRAW_SEQUENCE WM_USER + 4
#define WM_DRAW_BIT WM_USER + 5
#define WM_INIT_LINEMODE   WM_USER + 101
#define WM_LINEDECTYPE_CHANGE  WM_USER + 102
#define WM_LINEDRIVERTYPE_CHANGE WM_USER + 103

#define LINECURVE_FUNC  0
#define SYNCHRON_FUNC   1
#define HOMESEARCH_FUNC 2 
#define INTERP_FUNC     3
