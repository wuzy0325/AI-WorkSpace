/**
 * 共享运动控制模块初始化示例
 * 
 * 此文件仅供参考，不应直接导入使用。
 * 各项目需要在 main.ts 中自行调用 setMotionApi() 和 setToastService()。
 * 
 * @example
 * // 在项目 main.ts 中
 * import { setMotionApi, setToastService } from '@shared/motion';
 * 
 * setToastService({
 *   pushToast: (message, type) => { ... }
 * });
 * 
 * setMotionApi({
 *   getProfiles: () => yourApi.getProfiles(),
 *   getStatusAll: () => yourApi.getStatusAll(),
 *   upsertProfile: (profile) => yourApi.upsertProfile(profile),
 *   deleteProfile: (id) => yourApi.deleteProfile(id),
 *   connect: (id) => yourApi.connect(id).then(() => {}),
 *   disconnect: (id) => yourApi.disconnect(id),
 *   moveTo: (id, axis, position) => yourApi.moveTo(id, axis, position).then(() => {}),
 *   moveBy: (id, axis, delta) => yourApi.moveBy(id, axis, delta).then(() => {}),
 *   jog: (id, axis, direction, speed) => yourApi.jog(id, axis, direction, speed).then(() => {}),
 *   home: (id, axis) => yourApi.home(id, axis).then(() => {}),
 *   stop: (id, axis) => yourApi.stop(id, axis).then(() => {}),
 *   emergencyStop: (id) => yourApi.emergencyStop(id).then(() => {}),
 *   definePosition: (id, axis, position) => yourApi.definePosition(id, axis, position).then(() => {}),
 *   onStatusUpdated: (cb) => yourApi.onStatusUpdated(cb),
 * });
 */

export {};
