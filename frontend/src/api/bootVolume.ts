import { post } from './http'

export const bootVolumeApi = {
  update: (req: { userId: string; bootVolumeId: string; sizeInGBs: number; vpusPerGB: number }) =>
    post('/bootVolume/update', req),
  delete: (req: { userId: string; bootVolumeId: string }) => post('/bootVolume/delete', req)
}
