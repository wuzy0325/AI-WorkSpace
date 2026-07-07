/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice = WTNMC4A_DEV_CreateA("192.168.1.4");							 
	LONG A,LP,speed;
	WTNMC4A_PARA_DataList DL;		// 公用参数
	WTNMC4A_PARA_LCData LC;			// 直线和S曲线参数
	LC.AxisNum = WTNMC4A_XAXIS;		// 轴号(WTNMC4A_XAXIS:X轴; WTNMC4A_YAXIS:Y轴)
	LC.LV_DV= WTNMC4A_LV;			// 驱动方式  (连续 | 定长 )
	LC.PulseMode = WTNMC4A_CWCCW;	// 模式0：CW/CCW方式，1：CP/DIR方式 
	LC.Line_Curve = WTNMC4A_LINE;	// 直线曲线(0:直线加/减速; 1:S曲线加/减速)
	DL.Multiple =1;
	DL.Acceleration = 2500;			// 加速度(125~1000,000)(直线加减速驱动中加速度一直不变）
	DL.AccIncRate = 2000;			// 加速度变化率(仅S曲线驱动时有效)
	DL.StartSpeed = 100 ;			// 初始速度(1~8000)
	DL.DriveSpeed = 1000 ;			// 驱动速度	(1~8000)	
	LC.nPulseNum = 100000 ;			// 定量输出脉冲数(0~268435455)
	LC.Direction = WTNMC4A_PDIRECTION;	// 转动方向 WTNMC4A_PDirection: 正转  WTNMC4A_MDirection:反转		
	WTNMC4A_SetLP(hDevice, LC.AxisNum,0);  
	WTNMC4A_SetEP(					// 设置实位计数器 
		hDevice,				// 设备句柄
		LC.AxisNum,			// 轴号(WTNMC4A_XAxis:X轴; WTNMC4A_YAxis:Y轴)
		0);	
	WTNMC4A_SetAccofst(hDevice,LC.AxisNum,0);
	WTNMC4A_SetPDirSoftwareLimit(	// 设置正方向软件限位
		hDevice,		// 设备句柄
		LC.AxisNum,		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		WTNMC4A_LOGIC,	// 逻辑/实位计数器选择 WTNMC4A_LOGIC：逻辑位置计数器 WTNMC4A_FACT：实位计数器	
		5000);			// 软件限位数据*/
	WTNMC4A_InitLVDV(				//	初始化连续,定长脉冲驱动
		hDevice,
		&DL, 
		&LC);
	WTNMC4A_StartLVDV(				// 启动定长脉冲驱动
		hDevice,		// 设备句柄
		LC.AxisNum);	
	//	WTNMC4A_SetDec(hDevice,LCD.AxisNum,2500); // 不设置减速度时，减速时使用加速度的值，设置时按设定值
	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, LC.AxisNum));		// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, LC.AxisNum));		// 读当前加速度
		LP = WTNMC4A_ReadLP(hDevice, LC.AxisNum);						// 读逻辑计数器
		printf("%d轴速度 = %d    ",LC.AxisNum,speed);
		printf("%d轴加速度 = %d  ", LC.AxisNum,A);
		printf("%d轴逻辑位置计数器 = %ld \n", LC.AxisNum,LP);
	}	         

	_getch();
	WTNMC4A_DecStop(hDevice, WTNMC4A_ALLAXIS);		// 减速停止

	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

