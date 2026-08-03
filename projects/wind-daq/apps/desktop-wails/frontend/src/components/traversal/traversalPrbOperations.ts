import type {
  CalibrationCsvFileInfo,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  SevenHolePrbFileInfo,
  TraversalProbeType,
} from '@shared/types/traversal'

export interface TraversalPrbOperations {
  getError(): string | null
  importPrbFile(filePath: string): Promise<PrbFileInfo | null>
  importMultiPrbFiles(
    filePaths: string[],
    machNumbers?: number[],
    interpolationMode?: MultiPrbInterpolationMode,
  ): Promise<{ files: PrbFileInfo[]; machNumbers: number[]; warnings: string[] } | null>
  importCalibrationCsvFile(filePath: string): Promise<CalibrationCsvFileInfo | null>
  importSevenHolePrbFiles(
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null>
  importSevenHoleCalibrationCsvFiles(
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null>
  clearInterpolator(probeType: TraversalProbeType): Promise<boolean> | boolean
}
