@echo off
del *.exp
del *.bsc
del *.obj
del *.res
del *.sbr
del *.idb
del *.pch
del *.aps
del *.opt
del *.ilk
del *.ncb
del *.suo
del *.pdb
del *.sdf
del *.user

del x64\*.exp
del x64\*.pdb
del x86\*.exp
del x86\*.pdb

del Debug\*.* /q
del release\*.* /q

rd Debug
rd release

rd X86 /s /q
rd x64 /s /q

rd ipch /s /q