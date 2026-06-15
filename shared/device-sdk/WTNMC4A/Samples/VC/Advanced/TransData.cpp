#include "stdafx.h"
#include "Sys.h"
#include "TransData.h"
#include "ADWaveView.h"

// 主要用于精度分析，将非标准数据格式转换为标准数据格式(即线性格式),返回半码值(如16位返回32768, 14位为8192)
int TransToStdDataAD(SHORT DataDestBuffer[], SHORT DataSrcBuffer[], int iMiddleLsb, int iTriggerLsb, int Size) 
{	
	for (int i = 0; i < Size; i ++)
	{		
		DataDestBuffer[i] = (int)(DataSrcBuffer[i]-LSB_HALF - iMiddleLsb);
		if (DataDestBuffer[i] > iTriggerLsb)
		{
			gl_OverLimitCount++;
		}
		if (DataDestBuffer[i] < (-iTriggerLsb))
		{
			gl_OverLimitCount++;
		}
	}
	return LSB_HALF;
}

