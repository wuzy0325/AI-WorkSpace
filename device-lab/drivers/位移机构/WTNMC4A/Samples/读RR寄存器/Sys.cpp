/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{
	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	LONG A,LP,speed;					
	WTNMC4A_PARA_DataList DL;		// 公用参数
	WTNMC4A_PARA_LCData LC;			// 直线和S曲线参数
	WTNMC4A_PARA_RR0 RR0;			// 状态寄存器RR0
	LC.AxisNum = WTNMC4A_XAXIS;					// 轴号(WTNMC4A_XAXIS:X轴; WTNMC4A_YAXIS:Y轴；WTNMC4A_ZAXIS:Z轴; WTNMC4A_UAXIS:U轴)
	LC.LV_DV= WTNMC4A_DV;						// 驱动方式 WTNMC4A_LV:连续驱动；WTNMC4A_DV:定长驱动
	LC.PulseMode = WTNMC4A_CWCCW;				// 模式0：CW/CCW方式，1：CP/DIR方式 
	LC.Line_Curve = WTNMC4A_LINE;				// 直线曲线(0:直线加/减速; 1:S曲线加/减速)
	DL.Multiple=1;
	DL.Acceleration = 2500;						// 加速度(125~1000,000)(直线加减速驱动中加速度一直不变）
	DL.Deceleration = 2500;						// 加速度(125~1000,000)(直线加减速驱动中加速度一直不变）
	DL.AccIncRate = 2000;						// 加速度变化率(仅S曲线驱动时有效)
	DL.StartSpeed= 100 ;						// 初始速度(1~8000)
	DL.DriveSpeed = 8000 ;						// 驱动速度	(1~8000)	
	LC.nPulseNum = 150000 ;						// 定量输出脉冲数(0~268435455)
	LC.Direction = WTNMC4A_PDIRECTION ;			// 转动方向 WTNMC4A_PDirection: 正转  WTNMC4A_MDirection:反转		
	WTNMC4A_SetLP(hDevice, WTNMC4A_XAXIS, 0);	// 设置逻辑位置计数器
	WTNMC4A_SetLP(hDevice, WTNMC4A_YAXIS, 0);	// 设置逻辑位置计数器
	WTNMC4A_InitLVDV(				//	初始化连续,定长脉冲驱动
		hDevice,
		&DL, 
		&LC);
	WTNMC4A_StartLVDV(				// 启动定长脉冲驱动
		hDevice,		// 设备句柄
		LC.AxisNum);			

	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, LC.AxisNum));	// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, LC.AxisNum));	// 读当前加速度
		LP = WTNMC4A_ReadLP(hDevice, LC.AxisNum);	// 读逻辑计数器
		WTNMC4A_GetRR0Status(
			hDevice,			// 设备句柄
			&RR0);				// RR2寄存器状态			


		printf("ZDRV=%d ",RR0.ZDRV);	// Z轴驱动状态
		printf("XDRV=%d ",RR0.XDRV);	// X轴驱动状态
		//		printf("XIN2=%d ",RR3.XIN2);
		printf("%d轴速度 = %d ",LC.AxisNum,speed);
		//		printf("%d轴加速度 = %d  ", LC.AxisNum,A);
		printf("%d轴逻辑位置计数器 = %ld\n", LC.AxisNum,LP);
	}
	WTNMC4A_DecStop( hDevice,  WTNMC4A_ALLAXIS);	// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

