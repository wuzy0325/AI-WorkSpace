import { describe, it, expect } from 'vitest'
import { createRingBuffer, type RingBuffer } from '@composables/ringBuffer'

describe('createRingBuffer', () => {
  it('capacity=0 抛 Error', () => {
    expect(() => createRingBuffer(0)).toThrow('RingBuffer capacity must be > 0')
    expect(() => createRingBuffer(-1)).toThrow('RingBuffer capacity must be > 0')
  })

  it('capacity=4，push 3 次 → toArray 长度 3，顺序 oldest→newest', () => {
    const rb = createRingBuffer<string>(4)
    rb.push('a')
    rb.push('b')
    rb.push('c')
    expect(rb.length).toBe(3)
    expect(rb.toArray()).toEqual(['a', 'b', 'c'])
  })

  it('capacity=4，push 6 次 → 覆盖最旧 2 个，toArray 长度=4', () => {
    const rb = createRingBuffer<number>(4)
    for (let i = 0; i < 6; i++) {
      rb.push(i)
    }
    expect(rb.length).toBe(4)
    expect(rb.toArray()).toEqual([2, 3, 4, 5])
  })

  it('capacity=1，连续 push → 始终只保留最新', () => {
    const rb = createRingBuffer<string>(1)
    rb.push('a')
    expect(rb.toArray()).toEqual(['a'])
    rb.push('b')
    expect(rb.toArray()).toEqual(['b'])
    rb.push('c')
    expect(rb.length).toBe(1)
    expect(rb.toArray()).toEqual(['c'])
  })

  it('capacity=4，push 4 次刚好填满 → 未绕环', () => {
    const rb = createRingBuffer<number>(4)
    for (let i = 0; i < 4; i++) {
      rb.push(i)
    }
    expect(rb.length).toBe(4)
    expect(rb.toArray()).toEqual([0, 1, 2, 3])
  })

  it('clear() 后 length=0、toArray=[]', () => {
    const rb = createRingBuffer<number>(3)
    rb.push(1)
    rb.push(2)
    rb.clear()
    expect(rb.length).toBe(0)
    expect(rb.toArray()).toEqual([])
  })

  it('容量公共属性 capacity 与传入值一致', () => {
    const rb = createRingBuffer<number>(128)
    expect(rb.capacity).toBe(128)
  })

  it('泛型支持自定义对象类型', () => {
    interface Point {
      ts: number
      v: number
    }
    const rb = createRingBuffer<Point>(3)
    rb.push({ ts: 1, v: 10 })
    rb.push({ ts: 2, v: 20 })
    expect(rb.length).toBe(2)
    const arr = rb.toArray()
    expect(arr[0].ts).toBe(1)
    expect(arr[0].v).toBe(10)
    expect(arr[1].ts).toBe(2)
    expect(arr[1].v).toBe(20)
  })

  // ===== 版本号缓存测试 =====

  it('toArray 版本号缓存：push 后连续两次 toArray 返回同一引用', () => {
    const rb = createRingBuffer<number>(4)
    rb.push(1)
    rb.push(2)
    const arr1 = rb.toArray()
    const arr2 = rb.toArray()
    expect(arr1).toBe(arr2) // 同一引用
  })

  it('toArray 版本号缓存：再次 push 后缓存失效，返回新引用', () => {
    const rb = createRingBuffer<number>(4)
    rb.push(1)
    const arr1 = rb.toArray()
    rb.push(2)
    const arr2 = rb.toArray()
    expect(arr1).not.toBe(arr2) // 不同引用
    expect(arr2).toEqual([1, 2])
  })

  it('toArray 版本号缓存：clear 后返回空数组，与之前引用不同', () => {
    const rb = createRingBuffer<number>(4)
    rb.push(1)
    const arr1 = rb.toArray()
    rb.clear()
    const arr2 = rb.toArray()
    expect(arr1).not.toBe(arr2)
    expect(arr2).toEqual([])
  })

  it('toArray 版本号缓存：绕环后再次 push，缓存正确重建', () => {
    const rb = createRingBuffer<number>(3)
    rb.push(1)
    rb.push(2)
    rb.push(3)
    const arr1 = rb.toArray()
    rb.push(4)
    const arr2 = rb.toArray()
    expect(arr1).not.toBe(arr2)
    expect(arr2).toEqual([2, 3, 4])
  })

  // ===== 边界 =====

  it('push 空缓冲区后 length 从 0 逐步递增', () => {
    const rb = createRingBuffer<number>(10)
    expect(rb.length).toBe(0)
    rb.push(1)
    expect(rb.length).toBe(1)
    rb.push(2)
    expect(rb.length).toBe(2)
  })

  it('多次绕环后数据顺序仍正确（压迫测试）', () => {
    const rb = createRingBuffer<number>(5)
    // 写 13 次 → 绕环 2 圈 + 3 个位置
    for (let i = 0; i < 13; i++) {
      rb.push(i)
    }
    expect(rb.length).toBe(5)
    expect(rb.toArray()).toEqual([8, 9, 10, 11, 12])
  })

  // ===== peekLast 测试 =====

  it('peekLast() 空缓冲区返回 undefined', () => {
    const rb = createRingBuffer<number>(4)
    expect(rb.peekLast()).toBeUndefined()
  })

  it('peekLast() 返回最新写入的元素（未绕环）', () => {
    const rb = createRingBuffer<string>(4)
    rb.push('a')
    rb.push('b')
    rb.push('c')
    expect(rb.peekLast()).toBe('c')
  })

  it('peekLast() 返回最新写入的元素（绕环后）', () => {
    const rb = createRingBuffer<number>(3)
    rb.push(1)
    rb.push(2)
    rb.push(3)
    expect(rb.peekLast()).toBe(3)
    // 绕环一次，4 覆盖了 1 的位置
    rb.push(4)
    expect(rb.peekLast()).toBe(4)
    expect(rb.toArray()).toEqual([2, 3, 4])
  })

  it('peekLast() capacity=1 边界：始终返回最新', () => {
    const rb = createRingBuffer<string>(1)
    expect(rb.peekLast()).toBeUndefined()
    rb.push('a')
    expect(rb.peekLast()).toBe('a')
    rb.push('b')
    expect(rb.peekLast()).toBe('b')
  })

  it('peekLast() 不触发 toArray 缓存重建：连续调用不影响缓存引用', () => {
    const rb = createRingBuffer<number>(4)
    rb.push(1)
    rb.push(2)
    const arr1 = rb.toArray()
    // 在 push 之间调用 peekLast 不应让 toArray 缓存失效
    const peeked = rb.peekLast()
    expect(peeked).toBe(2)
    const arr2 = rb.toArray()
    expect(arr1).toBe(arr2) // 同一引用，未重建
  })

  it('peekLast() 多次绕环后仍正确', () => {
    const rb = createRingBuffer<number>(5)
    for (let i = 0; i < 13; i++) {
      rb.push(i)
    }
    expect(rb.peekLast()).toBe(12)
  })
})
