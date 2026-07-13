package traversal

import (
	"path/filepath"
	"testing"
)

// TestResolveOutputPath 锁定遍历 CSV 输出路径拼接规则：
// SavePath 是"目录"，必须与 SaveFileName（文件名）拼接，绝不能把目录当文件创建。
// 回归背景：v2 存储初始化曾直接用 config.SavePath（目录）作为文件创建路径，
// 导致 O_EXCL 在目录上报 "is a directory"。
//
// 关键不变量：
//   - 扩展名比较大小写不敏感（Windows 文件系统不区分 .csv / .CSV / .Csv）
//   - 所有返回值过 filepath.Clean，跨平台分隔符一致
func TestResolveOutputPath(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "目录+带后缀文件名 → 拼接",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data", SaveFileName: "Traversal-2026-07-13.csv"},
			want: filepath.Join("D:/data", "Traversal-2026-07-13.csv"),
		},
		{
			name: "目录+无后缀文件名 → 自动补 .csv",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data", SaveFileName: "Traversal-2026-07-13"},
			want: filepath.Join("D:/data", "Traversal-2026-07-13.csv"),
		},
		{
			name: "SavePath 已带 .csv → 视为完整路径直接使用（Clean 后跨平台一致）",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/my.csv"},
			want: filepath.Clean("D:/data/my.csv"),
		},
		{
			name: "SavePath 已带 .CSV 大写 → 大小写不敏感，视为完整路径",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/my.CSV"},
			want: filepath.Clean("D:/data/my.CSV"),
		},
		{
			name: "SavePath 已带 .Csv 混合大小写 → 大小写不敏感，视为完整路径",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/my.Csv"},
			want: filepath.Clean("D:/data/my.Csv"),
		},
		{
			name: "SavePath 为空 → 回退到默认名（无目录）",
			cfg:  Config{TaskID: "abc123"},
			want: "traversal_abc123.csv",
		},
		{
			name: "SavePath 为空 + 有文件名 → 直接用文件名",
			cfg:  Config{TaskID: "abc123", SaveFileName: "foo.csv"},
			want: "foo.csv",
		},
		{
			name: "SavePath 为空 + 文件名无后缀 → 自动补 .csv",
			cfg:  Config{TaskID: "abc123", SaveFileName: "foo"},
			want: "foo.csv",
		},
		{
			name: "SavePath 末尾带斜杠 → filepath.Join 自动归一",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/", SaveFileName: "result.csv"},
			want: filepath.Join("D:/data/", "result.csv"),
		},
		{
			name: "SaveFileName 含多点 → 仅最后一个 .csv 视为扩展名",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data", SaveFileName: "2026.07.13.csv"},
			want: filepath.Join("D:/data", "2026.07.13.csv"),
		},
		{
			name: "TaskID 为空 + 无 SaveFileName → 退化为 traversal_",
			cfg:  Config{TaskID: "", SavePath: "D:/data"},
			want: filepath.Join("D:/data", "traversal_.csv"),
		},
		{
			name: "Windows 反斜杠路径 → Clean 后与正斜杠等价",
			cfg:  Config{TaskID: "t1", SavePath: `D:\data`, SaveFileName: "result.csv"},
			want: filepath.Join(`D:\data`, "result.csv"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveOutputPath(c.cfg)
			if got != c.want {
				t.Errorf("ResolveOutputPath() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestResolveResultLogPath 锁定结果日志与 CSV 同目录、同 stem。
// 覆盖三个分支：目录+文件名、SavePath 已带 .csv、SavePath 为空。
func TestResolveResultLogPath(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "目录+文件名 → 同 stem .results.jsonl",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data", SaveFileName: "Traversal-2026-07-13.csv"},
			want: filepath.Join("D:/data", "Traversal-2026-07-13.results.jsonl"),
		},
		{
			name: "SavePath 已带 .csv → 同 stem .results.jsonl",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/my.csv"},
			want: filepath.Clean("D:/data/my.results.jsonl"),
		},
		{
			name: "SavePath 已带 .CSV 大写 → 同 stem .results.jsonl（保留原大小写 stem）",
			cfg:  Config{TaskID: "t1", SavePath: "D:/data/my.CSV"},
			want: filepath.Clean("D:/data/my.results.jsonl"),
		},
		{
			name: "SavePath 为空 → 相对路径 .results.jsonl",
			cfg:  Config{TaskID: "abc123"},
			want: "traversal_abc123.results.jsonl",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveResultLogPath(c.cfg)
			if got != c.want {
				t.Errorf("ResolveResultLogPath() = %q, want %q", got, c.want)
			}
		})
	}
}
