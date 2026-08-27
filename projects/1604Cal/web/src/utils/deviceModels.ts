import type { DeviceDTO } from '@/types/device'

/**
 * 设备型号归类工具。
 * 统一各组件对 model 字符串的判型逻辑，避免正则/字符串比较在
 * 多个组件里各自漂移（Primitive Obsession / Repeated Switches）。
 */

/** 归一化型号：去空白、转小写（与后端 factory normalizeModel 语义一致）。 */
export function normalizeDeviceModel(model: string | undefined | null): string {
  return (model ?? '').replace(/\s+/g, '').toLowerCase()
}

/** DAQ-P-1603 无阀门协议命令（DLL FFI 路径）：阀门切换/复位控件需隐藏，改用软件校零。 */
export function isValvelessModel(model: string | undefined | null): boolean {
  const normalized = normalizeDeviceModel(model)
  return normalized === 'daq-p-1603' || normalized === 'p1603'
}

/** ConST820 仅支持部分压力单位，单位下拉需按型号收窄选项。 */
export function isConst820Model(model: string | undefined | null): boolean {
  return normalizeDeviceModel(model).includes('820')
}

/** 取指定设备配置中启用的通道序号列表（升序）。P1603 校零等"按启用通道执行"的场景共用。 */
export function enabledChannelIndexes(devices: DeviceDTO[], deviceId: string): number[] {
  const dto = devices.find(device => device.id === deviceId)
  return (dto?.channels ?? [])
    .filter(channel => channel.enabled)
    .map(channel => channel.index)
    .sort((a, b) => a - b)
}
