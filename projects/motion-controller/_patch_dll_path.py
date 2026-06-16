"""临时脚本：修改 wtnmc4a_motion.go 的 DLL 查找逻辑"""
import pathlib

f = pathlib.Path(r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\device-sdk\go\motion\adapters\hardware\wtnmc4a_motion.go')
content = f.read_text(encoding='utf-8')

# 1. Add os and path/filepath imports
old_import = '\t"syscall"\n\t"unsafe"'
new_import = '\t"os"\n\t"path/filepath"\n\t"syscall"\n\t"unsafe"'
content = content.replace(old_import, new_import)

# 2. Replace wtnmc4aFindDLL function
old_func = 'func wtnmc4aFindDLL() string {\n\treturn "WTNMC4A_64.dll"\n}'
new_func = '''// wtnmc4aFindDLL 查找 WTNMC4A_64.dll 的路径。
// 优先从可执行文件所在目录查找，确保安装包部署后能正确定位 DLL。
func wtnmc4aFindDLL() string {
\tdllName := "WTNMC4A_64.dll"
\t// 获取可执行文件所在目录
\texePath, err := os.Executable()
\tif err != nil {
\t\tslog.Warn("获取可执行文件路径失败，使用默认DLL名", "error", err)
\t\treturn dllName
\t}
\tdllPath := filepath.Join(filepath.Dir(exePath), dllName)
\tif _, err := os.Stat(dllPath); err == nil {
\t\treturn dllPath
\t}
\t// 回退到系统搜索路径（PATH、exe同目录等）
\tslog.Debug("exe目录下未找到DLL，回退到系统搜索路径", "dll", dllName, "checked", dllPath)
\treturn dllName
}'''
content = content.replace(old_func, new_func)

f.write_text(content, encoding='utf-8')
print('Done')
