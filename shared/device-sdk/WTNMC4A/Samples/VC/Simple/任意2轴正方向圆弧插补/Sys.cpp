/***************************************************************************************/
#include "stdafx.h"
#include <stdio.h>
#include <conio.h>
#include "WTNMC4A.h"
int main(int argc, char* argv[])
{

	HANDLE hDevice =  WTNMC4A_DEV_CreateA("192.168.1.4");
	LONG A,LP1,LP2,speed;	
	WTNMC4A_PARA_DataList DL;			// 公用参数
	WTNMC4A_PARA_InterpolationAxis IA;	// 插补轴
	WTNMC4A_PARA_CircleData CD;			// 正反方向圆弧插补参数
	CD.ConstantSpeed = 1;				// WTNMC4A_CONSTAND:固定   WTNMC4A_NOCONSTAND：不固定 
	IA.Axis1 = WTNMC4A_XAXIS;			// 选择X轴为插补主轴
	IA.Axis2 = WTNMC4A_UAXIS;			// 选择U轴为插补副轴
	CD.Direction = 1;					// 方向：WTNMC4A_PDIRECTION：正转 WTNMC4A_MDIRECTION：反转
	CD.Center1 =5000;					// 主轴圆心坐标（脉冲数）
	CD.Center2 = 0;						// 副轴圆心坐标（脉冲数）
	CD.Pulse1 =0;						// 主轴终点坐标（脉冲数）		
	CD.Pulse2 =0;						// 副轴终点脉冲数
	DL.Multiple =1;
	DL.Acceleration=5000;				// 加速度(125~1000000)
	DL.StartSpeed = 1000;				// 初始速度(1~8000)
	DL.DriveSpeed = 1000;				// 驱动速度(1~8000)	
	/*	WTNMC4A_HanDec(						// 手动减速点设定
	hDevice,			// 设备句柄
	WTNMC4A_XAXIS,	// 轴号(WTNMC4A_XAXIS:X轴; WTNMC4A_YAXIS:Y轴)
	54200);			// 手动减速点数据，范围(0 - 268435455)*/
	WTNMC4A_InitCWInterpolation_2D(hDevice,// 初始化圆弧插补 
		&DL,
		&IA,
		&CD);
	WTNMC4A_StartCWInterpolation_2D(	// 启动反方向圆弧插补
		hDevice, CD.Direction);// 设备句柄					  
	while(!_kbhit())
	{
		speed= (WTNMC4A_ReadCV(hDevice, IA.Axis1));	// 读当前速度
		A = (WTNMC4A_ReadCA(hDevice, IA.Axis1));	// 读当前加速度
		LP1 = WTNMC4A_ReadLP(hDevice, IA.Axis1);		// 读逻辑计数器
		LP2 = WTNMC4A_ReadLP(hDevice, IA.Axis2);
		printf(" %d轴speed= %d ",IA.Axis1, speed);
		printf(" %d轴A= %d ",IA.Axis1, A);
		printf("%d轴LP = %ld  ", IA.Axis1, LP1);
		printf("%d轴LP = %ld \n",IA.Axis2, LP2);   
	}
	WTNMC4A_DecStop( hDevice,  WTNMC4A_ALLAXIS);		// 减速停止
	WTNMC4A_DEV_Release(hDevice);
	return 0;
}

