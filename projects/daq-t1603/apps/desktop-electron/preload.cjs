const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  showOpenDialog: () => ipcRenderer.invoke('dialog:pick-directory'),
})
