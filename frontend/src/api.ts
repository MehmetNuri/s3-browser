import type { main } from './models'
import { desktop } from './runtime'

export const AddProfileBucket = (arg1:string): Promise<void> => desktop.call('AddProfileBucket', [arg1]) as Promise<void>
export const Connect = (arg1:string): Promise<main.Profile> => desktop.call('Connect', [arg1]) as Promise<main.Profile>
export const CancelTransfer = (arg1:number): Promise<void> => desktop.call('CancelTransfer', [arg1]) as Promise<void>
export const ClearTransfers = (): Promise<void> => desktop.call('ClearTransfers', []) as Promise<void>
export const RetryTransfer = (arg1:number): Promise<void> => desktop.call('RetryTransfer', [arg1]) as Promise<void>
export const CopyObject = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('CopyObject', [arg1, arg2, arg3]) as Promise<void>
export const CopyToClipboard = (arg1:string): Promise<void> => desktop.call('CopyToClipboard', [arg1]) as Promise<void>
export const CreateBucket = (arg1:string): Promise<void> => desktop.call('CreateBucket', [arg1]) as Promise<void>
export const CreateFolder = (arg1:string, arg2:string): Promise<void> => desktop.call('CreateFolder', [arg1, arg2]) as Promise<void>
export const DeleteBucket = (arg1:string, arg2:boolean): Promise<void> => desktop.call('DeleteBucket', [arg1, arg2]) as Promise<void>
export const DeleteKeys = (arg1:string, arg2:Array<string>): Promise<void> => desktop.call('DeleteKeys', [arg1, arg2]) as Promise<void>
export const DeleteProfile = (arg1:string): Promise<void> => desktop.call('DeleteProfile', [arg1]) as Promise<void>
export const Download = (arg1:string, arg2:string, arg3:Array<string>): Promise<number> => desktop.call('Download', [arg1, arg2, arg3]) as Promise<number>
export const ExportProfiles = (): Promise<boolean> => desktop.call('ExportProfiles', []) as Promise<boolean>
export const ImportProfiles = (): Promise<number> => desktop.call('ImportProfiles', []) as Promise<number>
export const GetAppInfo = (): Promise<main.AppInfo> => desktop.call('GetAppInfo', []) as Promise<main.AppInfo>
export const HeadObject = (arg1:string, arg2:string): Promise<main.ObjectInfo> => desktop.call('HeadObject', [arg1, arg2]) as Promise<main.ObjectInfo>
export const ListBuckets = (): Promise<main.BucketList> => desktop.call('ListBuckets', []) as Promise<main.BucketList>
export const ListObjects = (arg1:string, arg2:string, arg3:string): Promise<main.Listing> => desktop.call('ListObjects', [arg1, arg2, arg3]) as Promise<main.Listing>
export const ListProfiles = (): Promise<Array<main.Profile>> => desktop.call('ListProfiles', []) as Promise<Array<main.Profile>>
export const OpenConfigDir = (): Promise<void> => desktop.call('OpenConfigDir', []) as Promise<void>
export const PickAndUpload = (arg1:string, arg2:string): Promise<number> => desktop.call('PickAndUpload', [arg1, arg2]) as Promise<number>
export const PickFolderAndUpload = (arg1:string, arg2:string): Promise<number> => desktop.call('PickFolderAndUpload', [arg1, arg2]) as Promise<number>
export const Presign = (arg1:string, arg2:string, arg3:number): Promise<string> => desktop.call('Presign', [arg1, arg2, arg3]) as Promise<string>
export const PreviewImage = (arg1:string, arg2:string): Promise<string> => desktop.call('PreviewImage', [arg1, arg2]) as Promise<string>
export const FolderStats = (arg1:string, arg2:string): Promise<{ objects: number; size: number; truncated: boolean }> => desktop.call('FolderStats', [arg1, arg2]) as Promise<{ objects: number; size: number; truncated: boolean }>
export type ObjectVersion = { versionId: string; modified: string; size: number; isLatest: boolean; deleteMarker: boolean }
export type ObjectHeaders = { contentType: string; cacheControl: string; contentDisposition: string; contentEncoding: string }
export type SearchResult = { items: main.S3Object[]; scanned: number; truncated: boolean }
export type SyncResult = { uploaded: number; skipped: number; failed: number; deleted: number }
export const SyncFolder = (arg1:string, arg2:string, arg3:boolean): Promise<SyncResult> => desktop.call('SyncFolder', [arg1, arg2, arg3]) as Promise<SyncResult>
export type BackupJob = { id: string; name: string; profileId: string; bucket: string; prefix: string; dir: string; mirror: boolean; intervalMinutes: number; lastRun: string; lastStatus: string; lastMessage: string; running: boolean }
export const ListBackupJobs = (): Promise<BackupJob[]> => desktop.call('ListBackupJobs', []) as Promise<BackupJob[]>
export const CreateBackupJob = (arg1:string, arg2:string, arg3:string, arg4:boolean, arg5:number): Promise<BackupJob> => desktop.call('CreateBackupJob', [arg1, arg2, arg3, arg4, arg5]) as Promise<BackupJob>
export const DeleteBackupJob = (arg1:string): Promise<void> => desktop.call('DeleteBackupJob', [arg1]) as Promise<void>
export const RunBackupJob = (arg1:string): Promise<SyncResult> => desktop.call('RunBackupJob', [arg1]) as Promise<SyncResult>
export const SearchObjects = (arg1:string, arg2:string, arg3:string): Promise<SearchResult> => desktop.call('SearchObjects', [arg1, arg2, arg3]) as Promise<SearchResult>
export const ListVersions = (arg1:string, arg2:string): Promise<ObjectVersion[]> => desktop.call('ListVersions', [arg1, arg2]) as Promise<ObjectVersion[]>
export const RestoreVersion = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('RestoreVersion', [arg1, arg2, arg3]) as Promise<void>
export const DeleteVersion = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('DeleteVersion', [arg1, arg2, arg3]) as Promise<void>
export const DownloadVersion = (arg1:string, arg2:string, arg3:string): Promise<boolean> => desktop.call('DownloadVersion', [arg1, arg2, arg3]) as Promise<boolean>
export const UpdateObjectHeaders = (arg1:string, arg2:string, arg3:ObjectHeaders): Promise<void> => desktop.call('UpdateObjectHeaders', [arg1, arg2, arg3]) as Promise<void>
export type AnalysisEntry = { name: string; size: number; objects: number }
export type DuplicateGroup = { size: number; copies: number; wasted: number; keys: string[] }
export type BucketAnalysis = {
  objects: number; size: number; truncated: boolean; wasted: number
  folders: AnalysisEntry[]; types: AnalysisEntry[]; classes: AnalysisEntry[]; ages: AnalysisEntry[]
  largest: main.S3Object[]; duplicates: DuplicateGroup[]
}
export const AnalyzeBucket = (arg1:string, arg2:string): Promise<BucketAnalysis> => desktop.call('AnalyzeBucket', [arg1, arg2]) as Promise<BucketAnalysis>
export const ListProfileBuckets = (arg1:string): Promise<main.BucketList> => desktop.call('ListProfileBuckets', [arg1]) as Promise<main.BucketList>
export const CopyToProfile = (arg1:string, arg2:string, arg3:Array<string>, arg4:string, arg5:string, arg6:string, arg7:boolean): Promise<number> => desktop.call('CopyToProfile', [arg1, arg2, arg3, arg4, arg5, arg6, arg7]) as Promise<number>
export const SaveText = (arg1:string, arg2:string, arg3:string, arg4:string): Promise<void> => desktop.call('SaveText', [arg1, arg2, arg3, arg4]) as Promise<void>
export type IncompleteUpload = { key: string; uploadId: string; initiated: string; size: number; stale: boolean }
export const ListIncompleteUploads = (arg1:string): Promise<IncompleteUpload[]> => desktop.call('ListIncompleteUploads', [arg1]) as Promise<IncompleteUpload[]>
export const AbortStaleUploads = (arg1:string): Promise<number> => desktop.call('AbortStaleUploads', [arg1]) as Promise<number>
export const BucketVersioning = (arg1:string): Promise<string> => desktop.call('BucketVersioning', [arg1]) as Promise<string>
export const SetBucketVersioning = (arg1:string, arg2:boolean): Promise<void> => desktop.call('SetBucketVersioning', [arg1, arg2]) as Promise<void>
export const PreviewText = (arg1:string, arg2:string): Promise<string> => desktop.call('PreviewText', [arg1, arg2]) as Promise<string>
export const RemoveProfileBucket = (arg1:string): Promise<void> => desktop.call('RemoveProfileBucket', [arg1]) as Promise<void>
export const RenameObject = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('RenameObject', [arg1, arg2, arg3]) as Promise<void>
export const RunCapabilities = (arg1:main.CapOptions): Promise<Array<main.CapResult>> => desktop.call('RunCapabilities', [arg1]) as Promise<Array<main.CapResult>>
export const SaveProfile = (arg1:main.Profile): Promise<main.Profile> => desktop.call('SaveProfile', [arg1]) as Promise<main.Profile>
export const SetLanguage = (arg1:string): Promise<void> => desktop.call('SetLanguage', [arg1]) as Promise<void>
export const TestProfile = (arg1:main.Profile): Promise<string> => desktop.call('TestProfile', [arg1]) as Promise<string>
// Local paths are resolved by the preload from the dropped files, never by the renderer.
export const UploadDropped = (arg1:string, arg2:string, arg3:Array<File>): Promise<number> => desktop.uploadDropped(arg1, arg2, arg3)
