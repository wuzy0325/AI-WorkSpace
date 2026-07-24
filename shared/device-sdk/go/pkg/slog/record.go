package slog

import "time"

// Record 是一条日志记录，由 Logger 在调用 Log/LogAttrs 时构造，
// 传给 Handler.Handle 处理输出。
type Record struct {
	Time    time.Time
	Level   Level
	Message string
	PC      uintptr // 源码位置（业务侧未使用，占位以兼容标准库字段）
	attrs   []Attr
}

// NewRecord 构造一条 Record。多数业务代码用 Logger.LogAttrs 间接构造。
func NewRecord(t time.Time, level Level, msg string, pc uintptr) Record {
	return Record{
		Time:    t,
		Level:   level,
		Message: msg,
		PC:      pc,
	}
}

// AddAttrs 向 Record 追加多个 Attr。
func (r *Record) AddAttrs(attrs ...Attr) {
	r.attrs = append(r.attrs, attrs...)
}

// Add 向 Record 追加单个 key-value 对。
func (r *Record) Add(key string, value any) {
	r.attrs = append(r.attrs, Any(key, value))
}

// Attrs 遍历 Record 的所有 Attr，f 返回 false 时停止。
// 与标准库 slog.Record.Attrs 签名一致。
func (r *Record) Attrs(f func(Attr) bool) {
	for _, a := range r.attrs {
		if !f(a) {
			return
		}
	}
}

// NumAttrs 返回 Attr 数量。
func (r *Record) NumAttrs() int {
	return len(r.attrs)
}

// Clone 复制一份 Record。
// fanoutHandler 给每个 sink 一份独立拷贝，避免某个 sink 修改 attrs 影响其他。
func (r Record) Clone() Record {
	r.attrs = append([]Attr(nil), r.attrs...)
	return r
}