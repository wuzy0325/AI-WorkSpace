<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { NButton, NDataTable } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import type { TraversalPoint } from '@shared/types/traversal'

/**
 * 自定义布点表格组件（P0 改造）
 *
 * 设计动机：原 .pt-list 直接 v-for 渲染所有点位，上百点时 DOM 节点过多导致
 * 画面拉长且滚动卡顿；删除只能一个个点。本组件用 n-data-table 虚拟滚动
 * （仅渲染可见 ~20 行）+ 多选批量删除/清空全部解决这两个痛点。
 *
 * 父组件的 customPoints 是 TraversalPoint[]（含可选 per-point 配置字段），
 * 无稳定 id。本组件用原数组索引 __idx 作为 row-key：搜索过滤后仍能映射回原数组；
 * 批量删除按 __idx 从大到小 splice 避免索引漂移。
 *
 * per-point 字段（dwellMs/samples/test）编辑：
 * - dwellMs/samples 用 UiInputNumber，留空（null）回写为 undefined（用全局默认）
 * - test 用 UiCheckbox，undefined 与 true 在 UI 上均显示为勾选（与"默认测试"语义对齐），
 *   用户取消勾选后变 false；再次勾选变 true（无法回到 undefined，符合直觉）
 */

/** 表格行数据：原索引 + 坐标 + per-point 配置字段。__idx 不写入父组件 customPoints。 */
interface TableRow {
  __idx: number
  x: number
  y: number
  z: number
  u: number
  /** per-point 稳定时间（ms），undefined = 用全局 dwellTimeMs */
  dwellMs?: number
  /** per-point 采样点数，undefined = 用全局 samplesPerPoint */
  samples?: number
  /** per-point 是否测试，undefined = 用全局默认 true；false = 跳过采集 */
  test?: boolean
}

const points = defineModel<TraversalPoint[]>({ required: true })

const i18n = useI18nStore()
const t = computed(() => i18n.t)
const feedbackStore = useFeedbackStore()

// 搜索关键字：按坐标值模糊匹配（转字符串后 includes），空字符串 = 不过滤
const searchKeyword = ref('')

/** 派生表格行数据：附加 __idx 作为稳定 row-key，便于过滤后映射回原数组 */
const tableRows = computed<TableRow[]>(() =>
  points.value.map((p, i) => ({
    __idx: i,
    x: p.x,
    y: p.y,
    z: p.z,
    u: p.u,
    // per-point 字段透传：undefined 表示用全局默认，与 TraversalPoint 类型对齐
    dwellMs: p.dwellMs,
    samples: p.samples,
    test: p.test,
  })),
)

/** 搜索过滤：关键字 trim 后匹配任一轴坐标值（小写比较，数值与字符串都可匹配） */
const filteredRows = computed<TableRow[]>(() => {
  const kw = searchKeyword.value.trim().toLowerCase()
  if (!kw) return tableRows.value
  return tableRows.value.filter((r) =>
    [r.x, r.y, r.z, r.u].some((v) => String(v).toLowerCase().includes(kw)),
  )
})

// 多选状态：存的是 __idx（原数组索引），不是过滤后的索引
const checkedKeys = ref<number[]>([])

/** 当父组件 points 长度变化（导入/清空/批量删除）时，剔除已失效的选中项避免脏数据 */
watch(
  () => points.value.length,
  (newLen) => {
    if (checkedKeys.value.some((k) => k >= newLen)) {
      checkedKeys.value = checkedKeys.value.filter((k) => k < newLen)
    }
  },
)

/** row-key 函数：用 __idx（原数组索引），过滤后仍稳定 */
function rowKey(row: TableRow): number | string {
  return row.__idx
}

/** n-data-table 多选回调：keys 类型为 Array<string | number>，转 number 后存 */
function onCheckedRowKeysChange(keys: Array<string | number>): void {
  checkedKeys.value = keys.map((k) => Number(k))
}

/** 全选/取消全选：仅作用于当前过滤结果，避免搜索状态下误操作未显示项 */
const allFilteredSelected = computed(
  () => filteredRows.value.length > 0 && filteredRows.value.every((r) => checkedKeys.value.includes(r.__idx)),
)
function toggleSelectAll(): void {
  if (allFilteredSelected.value) {
    // 取消选中当前过滤结果
    const filteredKeySet = new Set(filteredRows.value.map((r) => r.__idx))
    checkedKeys.value = checkedKeys.value.filter((k) => !filteredKeySet.has(k))
  } else {
    // 选中当前过滤结果（与已有选中去重合并）
    const merged = new Set([...checkedKeys.value, ...filteredRows.value.map((r) => r.__idx)])
    checkedKeys.value = Array.from(merged).sort((a, b) => a - b)
  }
}

/**
 * 批量删除选中的点位
 *
 * 索引处理：从大到小 splice，避免删除过程中后续索引漂移
 * 二次确认：弹 feedbackStore.confirm，避免误删
 */
async function deleteSelected(): Promise<void> {
  if (checkedKeys.value.length === 0) return
  const msg = t.value.customPointsDeleteConfirm.replace('{n}', String(checkedKeys.value.length))
  const ok = await feedbackStore.confirm(msg, { title: t.value.customPointsDeleteSelected })
  if (!ok) return
  const sortedKeys = [...checkedKeys.value].sort((a, b) => b - a) // 从大到小
  for (const k of sortedKeys) {
    points.value.splice(k, 1)
  }
  checkedKeys.value = []
}

/** 清空全部点位：保留二次确认，避免误操作丢失全部数据 */
async function clearAll(): Promise<void> {
  if (points.value.length === 0) return
  const msg = t.value.customPointsClearConfirm.replace('{n}', String(points.value.length))
  const ok = await feedbackStore.confirm(msg, { title: t.value.customPointsClearAll })
  if (!ok) return
  points.value.splice(0, points.value.length)
  checkedKeys.value = []
}

/**
 * 删除单行：通过 __idx 反向映射到原数组索引后 splice
 *
 * 设计决策：与 deleteSelected/clearAll 不同，单行删除不走 feedbackStore.confirm——
 * 单点操作小、可立即重新添加，二次确认反而拖慢节奏。批量删除/清空全部因影响面大
 * 才保留确认。后续维护者不要误以为是遗漏而加上确认弹窗。
 *
 * 选中项漂移修正：splice 后所有 __idx > deletedIdx 的行都会前移一位，
 * 选中集合中这些项必须同步 -1，否则 UI 选中标记会跳到下一行（用户原选第 N 行，
 * 删除第 M 行后选中错位到 N+1）。同时剔除被删除项自身的 __idx。
 */
function deleteRow(row: TableRow): void {
  const deletedIdx = row.__idx
  points.value.splice(deletedIdx, 1)
  checkedKeys.value = checkedKeys.value
    .filter((k) => k !== deletedIdx)
    .map((k) => (k > deletedIdx ? k - 1 : k))
}

/** 统计信息：总数 + 各轴范围（min~max），空数组时显示 '—' */
const stats = computed(() => {
  const len = points.value.length
  if (len === 0) return { total: 0, xRange: '—', yRange: '—', zRange: '—', uRange: '—' }
  let xMin = Infinity, xMax = -Infinity
  let yMin = Infinity, yMax = -Infinity
  let zMin = Infinity, zMax = -Infinity
  let uMin = Infinity, uMax = -Infinity
  for (const p of points.value) {
    xMin = Math.min(xMin, p.x); xMax = Math.max(xMax, p.x)
    yMin = Math.min(yMin, p.y); yMax = Math.max(yMax, p.y)
    zMin = Math.min(zMin, p.z); zMax = Math.max(zMax, p.z)
    uMin = Math.min(uMin, p.u); uMax = Math.max(uMax, p.u)
  }
  const fmt = (min: number, max: number) => (min === max ? String(min) : `${min} ~ ${max}`)
  return {
    total: len,
    xRange: fmt(xMin, xMax),
    yRange: fmt(yMin, yMax),
    zRange: fmt(zMin, zMax),
    uRange: fmt(uMin, uMax),
  }
})

// 已选数量文案：搜索过滤时不影响"已选 N/M"——M 始终是全量，避免用户以为选中丢了
const selectedCountText = computed(() =>
  t.value.customPointsSelectedCount
    .replace('{n}', String(checkedKeys.value.length))
    .replace('{total}', String(points.value.length)),
)

/**
 * 更新某点位的 per-point dwellMs 字段
 *
 * UiInputNumber 在清空时 emit null，需转回 undefined 以保持"用全局默认"语义
 * （TraversalPoint.dwellMs 类型为 number | undefined，不接受 null）。
 * 通过 __idx 反向映射回原数组，避免过滤视图下索引错位。
 */
function updateDwellMs(row: TableRow, value: number | null): void {
  const point = points.value[row.__idx]
  if (!point) return
  // null（清空）→ undefined（用全局默认）；非空 → 写入数值
  point.dwellMs = value ?? undefined
}

/** 更新某点位的 per-point samples 字段，语义同 updateDwellMs */
function updateSamples(row: TableRow, value: number | null): void {
  const point = points.value[row.__idx]
  if (!point) return
  point.samples = value ?? undefined
}

/**
 * 更新某点位的 per-point test 字段
 *
 * UiCheckbox emit boolean，直接写入；用户一旦交互即从 undefined 切换为
 * 显式 true/false，无法回退到 undefined（符合"用户已显式选择"的直觉）。
 */
function updateTest(row: TableRow, checked: boolean): void {
  const point = points.value[row.__idx]
  if (!point) return
  point.test = checked
}

/** n-data-table 列定义：选择列 + 序号 + X/Y/Z/U + per-point 配置 + 操作 */
const columns = computed<DataTableColumns<TableRow>>(() => [
  {
    type: 'selection',
    cellClassName: 'cp-cell-selection',
  },
  {
    title: t.value.customPointsIndexColumn,
    key: '__idx',
    width: 56,
    render: (row) => row.__idx + 1,
  },
  { title: 'X', key: 'x', width: 80 },
  { title: 'Y', key: 'y', width: 80 },
  { title: 'Z', key: 'z', width: 80 },
  { title: 'U', key: 'u', width: 80 },
  // per-point 稳定时间列：留空显示"用默认"占位，min/max 与全局 dwellTimeMs 输入对齐
  {
    title: t.value.customPointsDwellMsColumn,
    key: 'dwellMs',
    width: 110,
    render: (row) =>
      h(UiInputNumber, {
        modelValue: row.dwellMs ?? null,
        min: 100,
        max: 60000,
        step: 100,
        placeholder: t.value.customPointsUseDefaultHint,
        'onUpdate:modelValue': (v: number | null) => updateDwellMs(row, v),
      }),
  },
  // per-point 采样点数列：留空显示"用默认"占位，min=1 与 samplesPerPoint 语义对齐
  {
    title: t.value.customPointsSamplesColumn,
    key: 'samples',
    width: 110,
    render: (row) =>
      h(UiInputNumber, {
        modelValue: row.samples ?? null,
        min: 1,
        max: 1000,
        step: 1,
        placeholder: t.value.customPointsUseDefaultHint,
        'onUpdate:modelValue': (v: number | null) => updateSamples(row, v),
      }),
  },
  // per-point 是否测试列：undefined/true 显示勾选，false 显示未勾选
  {
    title: t.value.customPointsTestColumn,
    key: 'test',
    width: 90,
    align: 'center',
    render: (row) =>
      h(
        UiCheckbox,
        {
          checked: row.test !== false,
          'onUpdate:checked': (v: boolean) => updateTest(row, v),
        },
      ),
  },
  {
    title: t.value.customPointsActionColumn,
    key: 'actions',
    width: 70,
    // 用 h 函数渲染操作按钮，避免在 template 里写复杂的动态列插槽
    render: (row) =>
      h(
        NButton,
        {
          size: 'tiny',
          quaternary: true,
          type: 'error',
          onClick: () => deleteRow(row),
        },
        { default: () => t.value.del },
      ),
  },
])

// n-data-table 虚拟滚动行高：固定 36px，与 :row-height 对齐避免行高测量误差
const ROW_HEIGHT = 36
// 表格最大高度：超过则内部虚拟滚动，避免上百点撑爆面板
const TABLE_MAX_HEIGHT = 320
</script>

<template>
  <div class="custom-points-table">
    <!-- 顶部工具栏：搜索 + 全选 + 批量删除 + 清空 + 已选统计 -->
    <div class="cpt-toolbar">
      <UiInput
        v-model="searchKeyword"
        :placeholder="t.customPointsSearchPlaceholder"
        class="cpt-search"
        :aria-label="t.customPointsSearchPlaceholder"
      />
      <UiButton size="sm" variant="secondary" :disabled="points.length === 0" @click="toggleSelectAll">
        {{ allFilteredSelected ? '☑' : '☐' }} {{ selectedCountText }}
      </UiButton>
      <UiButton
        size="sm"
        variant="danger"
        :disabled="checkedKeys.length === 0"
        @click="deleteSelected"
      >
        {{ t.customPointsDeleteSelected }}
      </UiButton>
      <UiButton
        size="sm"
        variant="danger"
        :disabled="points.length === 0"
        @click="clearAll"
      >
        {{ t.customPointsClearAll }}
      </UiButton>
    </div>

    <!-- 统计条：总数 + 各轴范围，100+ 点时快速校验数据是否在预期区间 -->
    <div class="cpt-stats">
      <span class="cpt-stat-item"><span class="cpt-stat-label">{{ t.customPointsTotalLabel }}:</span> {{ stats.total }}</span>
      <span class="cpt-stat-item"><span class="cpt-stat-label">X:</span> {{ stats.xRange }}</span>
      <span class="cpt-stat-item"><span class="cpt-stat-label">Y:</span> {{ stats.yRange }}</span>
      <span class="cpt-stat-item"><span class="cpt-stat-label">Z:</span> {{ stats.zRange }}</span>
      <span class="cpt-stat-item"><span class="cpt-stat-label">U:</span> {{ stats.uRange }}</span>
    </div>

    <!-- 表格主体：虚拟滚动仅渲染可见行；空数据时显示提示 -->
    <div v-if="points.length === 0" class="cpt-empty">
      {{ t.customPointsEmptyHint }}
    </div>
    <NDataTable
      v-else
      size="small"
      :columns="columns"
      :data="filteredRows"
      :row-key="rowKey"
      :checked-row-keys="checkedKeys"
      :virtual-scroll="true"
      :max-height="TABLE_MAX_HEIGHT"
      :row-height="ROW_HEIGHT"
      :bordered="false"
      :single-line="false"
      @update:checked-row-keys="onCheckedRowKeysChange"
    />
  </div>
</template>

<style scoped>
/* 容器：纵向排列工具栏 / 统计 / 表格，gap 与 UiPanel 内边距对齐 */
.custom-points-table {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

/* 工具栏：搜索框 flex 1 占满剩余空间，按钮固定宽度 */
.cpt-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.cpt-search {
  flex: 1;
  min-width: 160px;
}

/* 统计条：水平排列，灰字小号，避免占用主视觉 */
.cpt-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: var(--font-size-micro);
  color: var(--text-muted);
  padding: 2px 4px;
}
.cpt-stat-item {
  white-space: nowrap;
}
.cpt-stat-label {
  color: var(--text-secondary, var(--text-primary));
  margin-right: 2px;
}

/* 空状态：与表格等高，灰字提示，避免空白让用户以为没加载 */
.cpt-empty {
  padding: 16px 8px;
  text-align: center;
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
  border: 1px dashed var(--border-default);
  border-radius: var(--radius-md);
}

/* n-data-table 紧凑化：缩小单元格内边距，让虚拟滚动行高更接近 36px */
.custom-points-table :deep(.n-data-table .n-data-table-th),
.custom-points-table :deep(.n-data-table .n-data-table-td) {
  padding: 4px 8px;
  font-size: var(--font-size-xs);
}
</style>
