// 定长环形缓冲区：push O(1)，无数组搬移。
// 用于实时波形图历史数据存储，替代 Array.shift() 的 O(n) 行为。
// toArray() 内置版本号缓存：仅在 push/clear 后首次调用重建数组，后续直接返回缓存引用。
// peekLast() 提供 O(1) 访问最新元素，不触发缓存重建——用于时间戳去重等只需尾元素的场景。
export interface RingBuffer<T> {
  /** 追加一个元素；缓冲区满时覆盖最旧元素，并使 toArray 缓存失效 */
  push(item: T): void
  /** 按时间顺序返回当前全部元素（ oldest → newest ）。版本号未变时返回缓存引用 */
  toArray(): readonly T[]
  /**
   * O(1) 返回最新（最后写入）的元素，不触发 toArray 缓存重建。
   * 用于只需访问尾部元素的场景（如时间戳去重），避免每帧 O(N) 数组重建。
   * 缓冲区为空时返回 undefined。
   */
  peekLast(): T | undefined
  /** 当前元素数量（≤ capacity） */
  readonly length: number
  /** 容量上限 */
  readonly capacity: number
  /** 清空缓冲区并使缓存失效 */
  clear(): void
}

export function createRingBuffer<T>(capacity: number): RingBuffer<T> {
  if (capacity <= 0) {
    throw new Error(`RingBuffer capacity must be > 0, got ${capacity}`)
  }
  const buf: T[] = new Array<T>(capacity)
  let head = 0       // 下一个写入位置
  let size = 0       // 当前有效元素数
  let version = 0    // 版本号：push/clear 时递增，toArray 据此判断是否需要重建缓存
  let cached: readonly T[] | null = null  // 缓存结果，避免每次 toArray 都新建数组

  return {
    capacity,

    get length() {
      return size
    },

    push(item: T) {
      buf[head] = item
      head = (head + 1) % capacity
      if (size < capacity) {
        size += 1
      }
      version += 1
      cached = null
    },

    toArray() {
      if (cached !== null) {
        return cached
      }
      if (size < capacity) {
        // 尚未绕环：直接切片
        cached = buf.slice(0, size) as readonly T[]
      } else {
        // 已绕环：head 指向最旧元素的下一个写入位
        // head 之前的段是最旧，head 之后的段是次新
        cached = [...buf.slice(head), ...buf.slice(0, head)] as readonly T[]
      }
      return cached
    },

    peekLast() {
      if (size === 0) return undefined
      // head 指向"下一个写入位"，所以最新元素在 head-1（注意绕环取模）
      const lastIndex = head === 0 ? capacity - 1 : head - 1
      return buf[lastIndex]
    },

    clear() {
      head = 0
      size = 0
      version += 1
      cached = null
    },
  }
}
