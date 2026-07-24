package slog

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// Kind 表示 Value 的类型，避免 any 装箱开销。
type Kind int

const (
	KindAny Kind = iota
	KindBool
	KindDuration
	KindFloat64
	KindInt64
	KindString
	KindTime
	KindUint64
	KindGroup
)

// Value 是 Attr 的值，封装各种类型。
// 与标准库 slog.Value 字段不同（标准库用 any + 字段标签），
// 本实现用 kind + num + str + any 四字段，
// 兼顾常见类型的零分配输出和任意类型的兜底存储。
type Value struct {
	kind Kind
	num  uint64 // Int64/Uint64/Float64/Bool/Duration/Time 的位模式
	str  string // KindString 的值
	any  any    // KindAny/KindGroup 的原始值
}

// Kind 返回 Value 类型。
func (v Value) Kind() Kind { return v.kind }

// StringValue 构造 string 类型的 Value。
func StringValue(v string) Value {
	return Value{kind: KindString, str: v}
}

// IntValue 构造 int 类型的 Value。
func IntValue(v int) Value {
	return Int64Value(int64(v))
}

// Int64Value 构造 int64 类型的 Value。
func Int64Value(v int64) Value {
	return Value{kind: KindInt64, num: uint64(v)}
}

// Uint64Value 构造 uint64 类型的 Value。
func Uint64Value(v uint64) Value {
	return Value{kind: KindUint64, num: v}
}

// BoolValue 构造 bool 类型的 Value。
func BoolValue(v bool) Value {
	var b uint64
	if v {
		b = 1
	}
	return Value{kind: KindBool, num: b}
}

// Float64Value 构造 float64 类型的 Value。
func Float64Value(v float64) Value {
	return Value{kind: KindFloat64, num: math.Float64bits(v)}
}

// DurationValue 构造 time.Duration 类型的 Value。
func DurationValue(v time.Duration) Value {
	return Value{kind: KindDuration, num: uint64(int64(v))}
}

// TimeValue 构造 time.Time 类型的 Value。
// 时区信息不保留（Time() 返回 UTC），业务侧未在日志中需要时区。
func TimeValue(v time.Time) Value {
	return Value{kind: KindTime, num: uint64(v.UnixNano())}
}

// AnyValue 构造任意类型的 Value。
func AnyValue(v any) Value {
	return Value{kind: KindAny, any: v}
}

// String 返回 Value 的字符串形式。
// 与标准库 slog.Value.String 行为一致：复杂类型用 fmt.Sprintf 兜底。
func (v Value) String() string {
	switch v.kind {
	case KindString:
		return v.str
	case KindInt64:
		return strconv.FormatInt(int64(v.num), 10)
	case KindUint64:
		return strconv.FormatUint(v.num, 10)
	case KindFloat64:
		return strconv.FormatFloat(math.Float64frombits(v.num), 'g', -1, 64)
	case KindBool:
		if v.num == 1 {
			return "true"
		}
		return "false"
	case KindDuration:
		return time.Duration(int64(v.num)).String()
	case KindTime:
		return time.Unix(0, int64(v.num)).Format(time.RFC3339Nano)
	case KindAny:
		if v.any == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%v", v.any)
	case KindGroup:
		return ""
	default:
		return ""
	}
}
// Any 返回原始值。
func (v Value) Any() any {
	switch v.kind {
	case KindString:
		return v.str
	case KindInt64:
		return int64(v.num)
	case KindUint64:
		return v.num
	case KindFloat64:
		return math.Float64frombits(v.num)
	case KindBool:
		return v.num == 1
	case KindDuration:
		return time.Duration(int64(v.num))
	case KindTime:
		return time.Unix(0, int64(v.num))
	case KindAny, KindGroup:
		return v.any
	default:
		return nil
	}
}

// Int64 返回 int64 形式（仅 KindInt64 / KindDuration 有效）。
func (v Value) Int64() int64 { return int64(v.num) }

// Uint64 返回 uint64 形式（仅 KindUint64 有效）。
func (v Value) Uint64() uint64 { return v.num }

// Bool 返回 bool 形式（仅 KindBool 有效）。
func (v Value) Bool() bool { return v.num == 1 }

// Float64 返回 float64 形式（仅 KindFloat64 有效）。
func (v Value) Float64() float64 { return math.Float64frombits(v.num) }

// Duration 返回 Duration 形式（仅 KindDuration 有效）。
func (v Value) Duration() time.Duration { return time.Duration(int64(v.num)) }

// Time 返回 Time 形式（仅 KindTime 有效）。
func (v Value) Time() time.Time { return time.Unix(0, int64(v.num)) }

// GroupValue 返回 Group 内嵌的 Attr 列表（仅 KindGroup 有效）。
func (v Value) GroupValue() []Attr {
	if v.kind != KindGroup {
		return nil
	}
	attrs, _ := v.any.([]Attr)
	return attrs
}
// Attr 是 key-value 对。
type Attr struct {
	Key   string
	Value Value
}

// Equal 比较两个 Attr 是否相等。空 Attr（Key="") 视为相等。
// 用于 RingHandler 检测空 Attr（slog.Attr{}）。
func (a Attr) Equal(b Attr) bool {
	if a.Key != b.Key {
		return false
	}
	if a.Value.kind != b.Value.kind {
		return false
	}
	switch a.Value.kind {
	case KindString:
		return a.Value.str == b.Value.str
	case KindInt64, KindUint64, KindBool, KindDuration, KindTime:
		return a.Value.num == b.Value.num
	case KindFloat64:
		return math.Float64frombits(a.Value.num) == math.Float64frombits(b.Value.num)
	case KindAny, KindGroup:
		return fmt.Sprintf("%v", a.Value.any) == fmt.Sprintf("%v", b.Value.any)
	default:
		return false
	}
}

// String 返回 Attr 的字符串表示，格式 key=value。
func (a Attr) String() string {
	return a.Key + "=" + a.Value.String()
}

// Resolve 立即求值 LogValuer。本实现不支持 LogValuer，直接返回自身。
// 保留该方法以兼容标准库 slog.Attr.Resolve 签名。
func (a Attr) Resolve() Attr { return a }

// ---- Attr 构造函数 ----

// String 构造 string 类型 Attr。
func String(key, val string) Attr {
	return Attr{Key: key, Value: StringValue(val)}
}

// Int 构造 int 类型 Attr。
func Int(key string, val int) Attr {
	return Attr{Key: key, Value: IntValue(val)}
}

// Int64 构造 int64 类型 Attr。
func Int64(key string, val int64) Attr {
	return Attr{Key: key, Value: Int64Value(val)}
}

// Uint64 构造 uint64 类型 Attr。
func Uint64(key string, val uint64) Attr {
	return Attr{Key: key, Value: Uint64Value(val)}
}

// Bool 构造 bool 类型 Attr。
func Bool(key string, val bool) Attr {
	return Attr{Key: key, Value: BoolValue(val)}
}

// Float64 构造 float64 类型 Attr。
func Float64(key string, val float64) Attr {
	return Attr{Key: key, Value: Float64Value(val)}
}

// Duration 构造 time.Duration 类型 Attr。
func Duration(key string, val time.Duration) Attr {
	return Attr{Key: key, Value: DurationValue(val)}
}

// Time 构造 time.Time 类型 Attr。
func Time(key string, val time.Time) Attr {
	return Attr{Key: key, Value: TimeValue(val)}
}

// Any 构造任意类型 Attr。
func Any(key string, val any) Attr {
	return Attr{Key: key, Value: AnyValue(val)}
}

// Group 构造分组 Attr。
// 标准 slog 中 Group 在输出中嵌套为 key.key2=val2，
// 本实现用 KindGroup 存储 Attr 列表，TextHandler 输出时展开为扁平的 group.key=val。
func Group(key string, attrs ...Attr) Attr {
	return Attr{Key: key, Value: Value{kind: KindGroup, any: attrs}}
}