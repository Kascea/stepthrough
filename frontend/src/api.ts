import { Call } from '@wailsio/runtime'
import { SessionData } from './types'

const PIPELINE = 'github.com/kascea/stepthrough/service.PipelineService'
const UI = 'github.com/kascea/stepthrough/service.UIService'

export const api = {
  checkDockerReady: (): Promise<boolean> =>
    Call.ByName(`${PIPELINE}.CheckDockerReady`),

  getSession: (): Promise<SessionData> =>
    Call.ByName(`${PIPELINE}.GetSession`),

  selectAndAdd: (): Promise<void> =>
    Call.ByName(`${UI}.SelectAndAdd`),

  removeTab: (file: string): Promise<void> =>
    Call.ByName(`${PIPELINE}.RemoveTab`, file),

  setActiveTab: (file: string): Promise<void> =>
    Call.ByName(`${PIPELINE}.SetActiveTab`, file),

  runPipeline: (file: string, stageIndex: number): Promise<void> =>
    Call.ByName(`${PIPELINE}.RunPipeline`, file, stageIndex),

  cancelPipeline: (file: string): Promise<void> =>
    Call.ByName(`${PIPELINE}.CancelPipeline`, file),

  relocateAndWatch: (file: string): Promise<void> =>
    Call.ByName(`${UI}.RelocateAndWatch`, file),
}
