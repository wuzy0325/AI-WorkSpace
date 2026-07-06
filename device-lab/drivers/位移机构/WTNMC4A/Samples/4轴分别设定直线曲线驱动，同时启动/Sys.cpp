/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	LONG A[4],LP[4],speed[4];								
	int i;
	WTNMC4A_PARA_DataList DL[4];	// 公用参数 
	WTNMC4A_PARA_LCData LC[4];		// 直线和S曲线参数
	WTNMC4A_SetLP(hDevice, WTNMC4A_ALLAXIS, 0); // 设置逻辑位置计数器
	WTNMC4A_SetEP(hDevice, WTNMC4A_ALLAXIS,	0);	// 设置实位计数器 		 	
	WTNMC4A_SetAccofst(hDevice, WTNMC4A_ALLAXIS, 0);	// 设置加速计数器偏移
	for(i=0; i<4; i++)
	{

		LC[i].AxisNum = i;						// 轴号(WTNMC4A_XAXIS:X轴; WTNMC4A_YAXIS:Y轴;;WTNMC4A_ZAXIS:Z轴; WTNMC4A_UAXIS:U轴)
		LC[i].LV_DV= WTNMC4A_DV;				// 驱动方式 WTNMC4A_DV:定长驱动 WTNMC4A_LV: 连续驱动
		LC[i].PulseMode = WTNMC4A_CWCCW;		// 模式0：CW/CCW方式，1：CP/DIR方式 
		LC[i].Line_Curve = WTNMC4A_LINE;		// 直线曲线(0:直线加/减速; 1:S曲线加/减速)

		DL[i].Multiple=1;
		DL[i].Acceleration = 2500;				// 加速度(125~1000,000)(直线加减速驱动中加速度一直不变）
		DL[i].Deceleration = 1250;				// 减速度(125~1000000)
		DL[i].AccIncRate = 1000;				// 加速度变化率(仅S曲线驱动时有效)
		DL[i].StartSpeed = 1 ;					// 初始速度(1~8000)
		DL[i].DriveSpeed = 8000 ;				// 驱动速度	(1~8000)	
		LC[i].nPulseNum = 100000 ;				// 定量输出脉冲数(0~268435455)
		LC[i].Direction = WTNMC4A_PDIRECTION ;	// 转动方向 WTNMC4A_PDirection: 正转  WTNMC4A_MDirection:反转		
		WTNMC4A_InitLVDV(						//	初始单轴化连续,定长脉冲驱动
			hDevice,
			&DL[i], 
			&LC[i]);
	}	
	WTNMC4A_Start4D( hDevice);				// 4轴同时启动
	while(!_kbhit())
	{
		for(i=0; i<4; i++)
		{
			speed[i]= (WTNMC4A_ReadCV(hDevice, i));		// 读当前速度
			A[i] = (WTNMC4A_ReadCA(hDevice, i));		// 读当前加速度
			LP[i] = WTNMC4A_ReadLP(hDevice, i);		// 读逻辑计数器
		}
		printf("%dSV=%d ",WTNMC4A_XAXIS,speed[0]);
		printf("%dSV=%d ",WTNMC4A_YAXIS,speed[1]);
		printf("%dSV=%d ",WTNMC4A_ZAXIS,speed[2]);
		printf("%dSV=%d ",WTNMC4A_UAXIS,speed[3]);

		printf("%dLP=%ld ", WTNMC4A_XAXIS,LP[0]);
		printf("%dLP=%ld ", WTNMC4A_YAXIS,LP[1]);		
		printf("%dLP=%ld ", WTNMC4A_ZAXIS,LP[2]);
		printf("%dLP=%ld\n", WTNMC4A_UAXIS,LP[3]);	
	}         
	WTNMC4A_DecStop(			 // 减速停止
		hDevice,			 // 设备句柄
		WTNMC4A_ALLAXIS);		

	WTNMC4A_DEV_Release(hDevice);
	_getch();
	return 1;
}

