/**
 * Pinia Store ID 统一管理
 * 
 * 解决共享模块和本地模块之间的 store ID 冲突问题。
 * 
 * 问题背景：
 * - 共享模块使用 'motion' 作为 store ID
 * - 本地模块也使用 'motion' 作为 store ID
 * 
 * 解决方案：
 * - 本地 store 使用带前缀的 ID: 'WINDLABX4-motion'
 * - 共享模块使用 'motion' 作为 ID
 * - 提供兼容层，允许通过两种方式访问
 */

export const STORE_IDS = {
  // 原始 ID（保持向后兼容）
  MOTION: 'motion',
  FEEDBACK: 'feedback',
  
  // 新的带前缀 ID
  MOTION_WINDLABX4: 'WINDLABX4-motion',
  FEEDBACK_WINDLABX4: 'WINDLABX4-feedback',
  
  // 其他 store ID
  DEVICE: 'device',
  CALIBRATION: 'calibration',
  TRAVERSAL: 'traversal',
  THEME: 'theme',
  STORAGE: 'storage',
  I18N: 'i18n',
  LOG: 'log',
} as const;

export type StoreId = typeof STORE_IDS[keyof typeof STORE_IDS];

/**
 * 获取 store ID 的兼容版本
 * 
 * 如果指定了新的 ID 格式，返回新格式；
 * 否则返回原始 ID。
 */
export function resolveStoreId(id: string, useNewFormat = false): string {
  // 如果已经是新格式，直接返回
  if (id.startsWith('WINDLABX4-')) {
    return id;
  }
  
  // 转换为新格式
  if (useNewFormat) {
    return `WINDLABX4-${id}`;
  }
  
  // 返回原始 ID
  return id;
}

/**
 * Store 访问配置
 * 
 * 定义每个 store 的访问策略：
 * - primaryId: 主要使用的 ID
 * - aliasIds: 别名 ID（向后兼容）
 * - migrationStatus: 迁移状态
 */
export const STORE_CONFIG: Record<string, {
  primaryId: string;
  aliasIds: string[];
  description: string;
}> = {
  motion: {
    primaryId: STORE_IDS.MOTION_WINDLABX4,
    aliasIds: [STORE_IDS.MOTION],
    description: '运动控制状态管理',
  },
  feedback: {
    primaryId: STORE_IDS.FEEDBACK_WINDLABX4,
    aliasIds: [STORE_IDS.FEEDBACK],
    description: '用户反馈（Toast、确认对话框）',
  },
  device: {
    primaryId: STORE_IDS.DEVICE,
    aliasIds: [],
    description: '设备状态管理',
  },
  calibration: {
    primaryId: STORE_IDS.CALIBRATION,
    aliasIds: [],
    description: '校准流程状态管理',
  },
  traversal: {
    primaryId: STORE_IDS.TRAVERSAL,
    aliasIds: [],
    description: '遍历扫描状态管理',
  },
  theme: {
    primaryId: STORE_IDS.THEME,
    aliasIds: [],
    description: '主题配置管理',
  },
  storage: {
    primaryId: STORE_IDS.STORAGE,
    aliasIds: [],
    description: '数据存储状态管理',
  },
  i18n: {
    primaryId: STORE_IDS.I18N,
    aliasIds: [],
    description: '国际化配置管理',
  },
  log: {
    primaryId: STORE_IDS.LOG,
    aliasIds: [],
    description: '日志状态管理',
  },
};

/**
 * 迁移检查工具
 * 
 * 用于检查代码中是否还在使用旧的 store ID，
 * 帮助识别需要迁移的代码位置。
 */
export function checkStoreIdUsage(code: string): {
  oldIds: string[];
  newIds: string[];
  suggestions: string[];
} {
  const oldIds: string[] = [];
  const newIds: string[] = [];
  const suggestions: string[] = [];
  
  // 检查 motion store
  if (code.includes("defineStore('motion'") && !code.includes('WINDLABX4-motion')) {
    oldIds.push('motion');
    suggestions.push(
      "将 defineStore('motion' 迁移到 defineStore('WINDLABX4-motion'，并更新所有使用 useMotionStore() 的地方"
    );
  }
  
  // 检查 feedback store
  if (code.includes("defineStore('feedback'") && !code.includes('WINDLABX4-feedback')) {
    oldIds.push('feedback');
    suggestions.push(
      "将 defineStore('feedback' 迁移到 defineStore('WINDLABX4-feedback'，并更新所有使用 useFeedbackStore() 的地方"
    );
  }
  
  // 检查新 ID
  if (code.includes('WINDLABX4-motion')) {
    newIds.push('WINDLABX4-motion');
  }
  if (code.includes('WINDLABX4-feedback')) {
    newIds.push('WINDLABX4-feedback');
  }
  
  return { oldIds, newIds, suggestions };
}
