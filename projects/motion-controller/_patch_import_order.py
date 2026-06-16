"""临时脚本：修复 import 顺序"""
import pathlib

f = pathlib.Path(r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\device-sdk\go\motion\adapters\hardware\wtnmc4a_motion.go')
content = f.read_text(encoding='utf-8')

old = '\t"sync"\n\t"sync/atomic"\n\t"os"\n\t"path/filepath"\n\t"syscall"'
new = '\t"os"\n\t"path/filepath"\n\t"sync"\n\t"sync/atomic"\n\t"syscall"'
content = content.replace(old, new)

f.write_text(content, encoding='utf-8')
print('Done')
