/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{
	LONG A,LP,speed;
	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	WTNMC4A_PARA_DataList DL;		// 定义公共参数结构体
	WTNMC4A_PARA_LCData LC;			// 定义直线曲线参数结构体
	LC.AxisNum = WTNMC4A_YAXIS;		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴)
	LC.LV_DV= WTNMC4A_DV;			// 驱动方式 WTNMC4A_DV:定长驱动 WTNMC4A_LV:连续驱动
	LC.PulseMode = WTNMC4A_CWCCW;	// 脉冲方式 WTNMC4A_CWCCW:CW/CCW方式,WTNMC4A_CPDIR:CP/DIR方式 
	LC.Line_Curve = WTNMC4A_LINE;	// 运动方式WTNMC4A_LINE:直线加/减速; WTNMC4A_CURVE:S曲线加/减速)
	DL.Multiple =1;
	DL.Acceleration = 500;			// 加速度(125~1000,000)(直线加减速驱动中加速度一直不变)
	DL.Deceleration = 500;			// 减速度(125~1000,000)(直线加减速驱动中加速度一直不变)
	//	DL.AccIncRate = 8000;			// 加速度变化率(仅S曲线驱动时有效)
	//	DL.DecIncRate = 2000; 
	DL.StartSpeed = 100 ;			// 初始速度(1~8000)
	DL.DriveSpeed = 8000;			// 驱动速度	(1~8000)	
	//	LC.DecMode = WTNMC4A_AUTO;		// 减速模式 WTNMC4A_AUTO :自动减速 WTNMC4A_HAND:手动减速
	LC.nPulseNum = 100000;			// 定量输出脉冲数(0~268435455)
	LC.Direction = WTNMC4A_PDIRECTION;// 运动方向 WTNMC4A_PDIRECTION: 正转  WTNMC4A_MDIRECTION:反转		

	//	WTNMC4A_SetAccofst(hDevice,LC.AxisNum,0);
	/*	WTNMC4A_HanDec(				 // 手动减速点设定
	hDevice,		// 设备句柄
	LC.AxisNum,		// 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
	84249);			// 手动减速点数据，范围(0 - 4294967295)*/

	WTNMC4A_InitLVDV(				//	初始化连续,定长脉冲驱动
		hDevice,
		&DL, 
		&LC);
	WTNMC4A_OutSwitch(				 // 设置输出切换
		hDevice,			 // 设备句柄
		WTNMC4A_YAXIS,	 // 轴号(WTNMC4A_XAXIS:X轴,WTNMC4A_YAXIS:Y轴, WTNMC4A_ZAXIS:Z轴,WTNMC4A_UAXIS:U轴) 
		WTNMC4A_STATUSOUT);	
	WTNMC4A_StartLVDV(				// 启动定长脉冲驱动
		hDevice,		// 设备句柄
		LC.AxisNum);	
	//	WTNMC4A_SetDec(hDevice,LCD.AxisNum,2500); // 不设置减速度时，减速时使用加速度的值，设置时按设定值
	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, LC.AxisNum));	// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, LC.AxisNum));	// 读当前加速度
		LP = WTNMC4A_ReadLP(hDevice, LC.AxisNum);		// 读逻辑计数器
		printf("%d轴速度 = %d    ",LC.AxisNum,speed);
		printf("%d轴加速度 = %d  ", LC.AxisNum,A);
		printf("%d轴逻辑位置计数器 = %ld \n", LC.AxisNum,LP);
	}	         

	_getch();
	WTNMC4A_DecStop(hDevice, WTNMC4A_ALLAXIS);		// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

