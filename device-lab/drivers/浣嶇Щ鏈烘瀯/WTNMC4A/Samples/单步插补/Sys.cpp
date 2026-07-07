/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{


	LONG A,LP1,LP2,speed;	
	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	WTNMC4A_PARA_DataList DL;		// 公用参数 
	WTNMC4A_PARA_LineData LD;		// 直线插补和固定线速度直线插补参数
	WTNMC4A_PARA_InterpolationAxis IA;	// 插补轴
	IA.Axis1= WTNMC4A_XAXIS;		// X轴
	IA.Axis2 = WTNMC4A_YAXIS;		// Y轴
	LD.Line_Curve = WTNMC4A_LINE;			// 直线曲线(WTNMC4A_Line:直线加/减速; WTNMC4A_Curve:S曲线加/减速)
	LD.ConstantSpeed = 0;					// 固定线速度 (不固定线速度 | 固定线速度)
	DL.Multiple=1;
	DL.Acceleration = 1250;					// 加速度(1~1000,000)
	DL.AccIncRate=1000;						// 加速度变化率(954~62500000)
	DL.StartSpeed = 1000;					// 初始速度(1~8000)
	DL.DriveSpeed = 1000;					// 驱动速度	(1~8000)
	LD.n1AxisPulseNum = 60000;				// X轴终点坐标脉冲数(-8388608~+8388607) 
	LD.n2AxisPulseNum = 10000;				// Y轴终点坐标脉冲数(-8388608~+8388607)
	WTNMC4A_SetLP(hDevice, IA.Axis1, 0);	// 设置逻辑位置计数器
	WTNMC4A_SetLP(hDevice, IA.Axis2, 0);	// 设置逻辑位置计数器
	/*	WTNMC4A_ExSingleStepInterpolation(		// 外部控制单步插补运动
	hDevice);		// */
	WTNMC4A_SingleStepInterpolationCom(		// 命令控制单步插补运动
		hDevice);			// 设备句柄	
	WTNMC4A_InitLineInterpolation_2D(		// 初始化直线插补运动 
		hDevice,			// 设备句柄
		&DL,
		&IA,
		&LD); 

	WTNMC4A_StartLineInterpolation_2D(hDevice);		// 启动任意2轴直线插补运动

	while(!_kbhit())
	{
		WTNMC4A_StartSingleStepInterpolation(hDevice);		// 发单步插补命令，执行一次输出一个脉冲				 
		speed= (WTNMC4A_ReadCV(hDevice, IA.Axis1));		// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, IA.Axis1));	// 读当前加速度
		LP1 = WTNMC4A_ReadLP(hDevice, IA.Axis1);		// 读逻辑计数器
		LP2 = WTNMC4A_ReadLP(hDevice, IA.Axis2); 
		printf("%d轴Speed = %d ",IA.Axis1, speed);
		printf("%d轴A = %d ", IA.Axis1, A);
		printf("%d轴LP = %ld ", IA.Axis1, LP1);
		printf("%d轴LP = %ld\n", IA.Axis2, LP2);
	}
	WTNMC4A_DecStop( hDevice,  WTNMC4A_ALLAXIS);
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

