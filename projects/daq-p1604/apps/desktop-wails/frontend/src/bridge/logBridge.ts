import { StartLogFile, StopLogFile, GetLogFileState, PickDirectory } from '../../bindings/daq-p1604/backend/app'

export interface LogFileState {
  active: boolean
  outputDir: string
}

export function startLogFile(outputDir: string, prefix: string): Promise<void> {
  return StartLogFile(outputDir, prefix) as Promise<void>
}

export function stopLogFile(): Promise<void> {
  return StopLogFile() as Promise<void>
}

export function getLogFileState(): Promise<LogFileState> {
  return GetLogFileState() as any
}

export function pickDirectory(): Promise<string> {
  return PickDirectory() as any
}
