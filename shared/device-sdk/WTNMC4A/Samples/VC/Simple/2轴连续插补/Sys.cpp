/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{


	LONG A,LPX,LPY,speed;
	HANDLE hDevice = WTNMC4A_DEV_CreateA("192.168.1.4");
	if (hDevice == INVALID_HANDLE_VALUE)
	{
		printf("Create Device Error...\n");
		//		return 0;
	}
	
	WTNMC4A_PARA_DataList  DL;
	WTNMC4A_PARA_InterpolationAxis IA;	// 插补轴
	WTNMC4A_PARA_LineData LD;		// 直线插补和固定线速度直线插补参数
	IA.Axis1 = WTNMC4A_XAXIS;		// X轴
	IA.Axis2 = WTNMC4A_YAXIS;		// Y轴
	LD.ConstantSpeed=1;				// 固定线速度 (不固定线速度 | 固定线速度)
	LD.Line_Curve = WTNMC4A_LINE;	// 直线运动
	DL.Multiple =1;		// 倍率 (1~500)
	DL.Acceleration=1250;			// 加速度(125~1000000)
	DL.StartSpeed = 1000;			// 初始速度(1~8000)
	DL.DriveSpeed = 1000;			// 驱动速度(1~8000)
	LD.n1AxisPulseNum=10000;		// 主轴终点脉冲数 (-8388608~8388607)
	LD.n2AxisPulseNum= 0;			// 第二轴轴终点脉冲数 (-8388608~8388607)
	WTNMC4A_InitLineInterpolation_2D(			// 初始化任意2轴直线插补运动 
		hDevice,					// 设备句柄
		&DL,						// 公共参数结构体指针
		&IA,						// 插补轴结构体指针
		&LD);						// 直线插补结构体指针
	WTNMC4A_StartLineInterpolation_2D(hDevice);	// 启动任意2轴直线插补运动 

	WTNMC4A_NextWait( hDevice);					// 等待写入下一个节点的参数和命令
		
	WTNMC4A_PARA_CircleData  CD;		// 正反方向圆弧插补参数
	CD.Direction = 0;		// 运动方向 (正方向 | 反方向)
	CD.Center1=0;		// 主轴圆心坐标(脉冲数-8388608~8388607)
	CD.Center2=2000;	// 第二轴轴圆心坐标(脉冲数-8388608~8388607)
	CD.Pulse1=0;		// 主轴终点坐标(脉冲数-8388608~8388607)	
	CD.Pulse2=4000;		// 第二轴轴终点坐标(脉冲数-8388608~8388607)
	CD.ConstantSpeed = WTNMC4A_CONSTAND;	// 固定线速度
	WTNMC4A_InitCWInterpolation_2D(				// 初始化任意2轴正反方向圆弧插补运动 
		hDevice,			// 设备句柄
		&DL,				// 公共参数结构体指针
		&IA,				// 插补轴结构体指针
		&CD);				// 圆弧插补结构体指针
	WTNMC4A_StartCWInterpolation_2D(hDevice, CD.Direction);

	WTNMC4A_NextWait( hDevice);					// 等待写入下一个节点的参数和命令
		
	LD.n1AxisPulseNum = -10000;
	LD.n2AxisPulseNum = 0;

	WTNMC4A_InitLineInterpolation_2D(			// 初始化任意2轴直线插补运动 
		hDevice,					// 设备句柄
		&DL,						// 公共参数结构体指针
		&IA,						// 插补轴结构体指针
		&LD);						// 直线插补结构体指针
	WTNMC4A_StartLineInterpolation_2D(hDevice);	// 启动任意2轴直线插补运动 

	WTNMC4A_NextWait( hDevice);					// 等待写入下一个节点的参数和命令


	CD.Center1=0;
	CD.Center2=-2000;
	CD.Pulse1=0;
	CD.Pulse2=-4000;
	WTNMC4A_InitCWInterpolation_2D(				// 初始化任意2轴正反方向圆弧插补运动 
		hDevice,			// 设备句柄
		&DL,				// 公共参数结构体指针
		&IA,				// 插补轴结构体指针
		&CD);				// 圆弧插补结构体指针
	WTNMC4A_StartCWInterpolation_2D(hDevice, CD.Direction);
	while(!_kbhit())		
	{
		speed= WTNMC4A_ReadCV(hDevice, IA.Axis1);	// 读当前速度
		A = WTNMC4A_ReadCA(hDevice, IA.Axis1);	// 读当前加速度
		LPX = WTNMC4A_ReadLP(hDevice, IA.Axis1);	// 读逻辑计数器
		LPY = WTNMC4A_ReadLP(hDevice, IA.Axis2);	// 读逻辑计数器
		printf("%d轴speed = %d ",IA.Axis1,speed);
		printf("%d轴A = %d ",IA.Axis1, A);
		printf("%d轴LP = %ld  ",IA.Axis1, LPX);
		printf("%d轴LP = %ld \n",IA.Axis2, LPY);
	}  	
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

