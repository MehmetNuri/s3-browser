import type { main } from './models'
import { desktop } from './runtime'

export const AddProfileBucket = (arg1:string): Promise<void> => desktop.call('AddProfileBucket', [arg1]) as Promise<void>
export const Connect = (arg1:string): Promise<main.Profile> => desktop.call('Connect', [arg1]) as Promise<main.Profile>
export const CancelTransfer = (arg1:number): Promise<void> => desktop.call('CancelTransfer', [arg1]) as Promise<void>
export const ClearTransfers = (): Promise<void> => desktop.call('ClearTransfers', []) as Promise<void>
export const RetryTransfer = (arg1:number): Promise<void> => desktop.call('RetryTransfer', [arg1]) as Promise<void>
export type Transfer = { id: number; kind: string; name: string; done: number; total: number; state: string; error: string; speed: number; order: number }
export type TransferQueue = { limit: number; paused: boolean; queued: number; running: number; failed: number; done: number; speed: number; bandwidthKBps: number }
export const GetTransfers = (): Promise<Transfer[]> => desktop.call('GetTransfers', []) as Promise<Transfer[]>
export const GetTransferQueue = (): Promise<TransferQueue> => desktop.call('GetTransferQueue', []) as Promise<TransferQueue>
export const CancelQueuedTransfers = (): Promise<number> => desktop.call('CancelQueuedTransfers', []) as Promise<number>
export const RetryFailedTransfers = (): Promise<number> => desktop.call('RetryFailedTransfers', []) as Promise<number>
export const PrioritizeTransfer = (arg1:number): Promise<void> => desktop.call('PrioritizeTransfer', [arg1]) as Promise<void>
export const ReorderTransfer = (arg1:number, arg2:number): Promise<void> => desktop.call('ReorderTransfer', [arg1, arg2]) as Promise<void>
export const RemoveTransfer = (arg1:number): Promise<void> => desktop.call('RemoveTransfer', [arg1]) as Promise<void>
export const PauseTransfers = (arg1:boolean): Promise<void> => desktop.call('PauseTransfers', [arg1]) as Promise<void>
export const SetBandwidthLimit = (arg1:number): Promise<void> => desktop.call('SetBandwidthLimit', [arg1]) as Promise<void>
export const SetTransferLimit = (arg1:number): Promise<void> => desktop.call('SetTransferLimit', [arg1]) as Promise<void>
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
export type SyncResult = { uploaded: number; downloaded: number; skipped: number; failed: number; deleted: number }
export const SyncToFolder = (arg1:string, arg2:string, arg3:boolean): Promise<SyncResult> => desktop.call('SyncToFolder', [arg1, arg2, arg3]) as Promise<SyncResult>
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
export type PublicAccessBlock = { blockPublicAcls: boolean; ignorePublicAcls: boolean; blockPublicPolicy: boolean; restrictPublicBuckets: boolean }
export type BucketSettings = {
  region: string; versioning: string; policy: string; cors: string; lifecycle: string; tags: Record<string, string>
  encryption: string; kmsKey: string; publicAccessBlock: PublicAccessBlock | null; unsupported: Record<string, string>
  websiteIndex: string; websiteError: string; loggingBucket: string; loggingPrefix: string; requesterPays: boolean
  objectLock: ObjectLock | null
}
export type AclGrant = { type: string; grantee: string; name: string; permission: string }
export type Acl = { ownerId: string; ownerName: string; grants: AclGrant[] }
export const GetObjectAcl = (arg1:string, arg2:string): Promise<Acl> => desktop.call('GetObjectAcl', [arg1, arg2]) as Promise<Acl>
export const SetObjectAcl = (arg1:string, arg2:string, arg3:Acl): Promise<void> => desktop.call('SetObjectAcl', [arg1, arg2, arg3]) as Promise<void>
export const GetBucketAcl = (arg1:string): Promise<Acl> => desktop.call('GetBucketAcl', [arg1]) as Promise<Acl>
export const SetBucketAcl = (arg1:string, arg2:Acl): Promise<void> => desktop.call('SetBucketAcl', [arg1, arg2]) as Promise<void>
export type Distribution = { id: string; domain: string; status: string; enabled: boolean; comment: string; aliases: string[]; origin: string }
export const ListDistributions = (arg1:string): Promise<Distribution[]> => desktop.call('ListDistributions', [arg1]) as Promise<Distribution[]>
export const InvalidatePaths = (arg1:string, arg2:Array<string>): Promise<string> => desktop.call('InvalidatePaths', [arg1, arg2]) as Promise<string>
export type ObjectLock = { enabled: boolean; mode: string; days: number; years: number }
export type ObjectRetention = { mode: string; until: string; legalHold: boolean }
export const SetObjectLock = (arg1:string, arg2:string, arg3:number, arg4:number): Promise<void> => desktop.call('SetObjectLock', [arg1, arg2, arg3, arg4]) as Promise<void>
export const SetObjectRetention = (arg1:string, arg2:string, arg3:string, arg4:string, arg5:boolean): Promise<void> => desktop.call('SetObjectRetention', [arg1, arg2, arg3, arg4, arg5]) as Promise<void>
export const SetObjectLegalHold = (arg1:string, arg2:string, arg3:boolean): Promise<void> => desktop.call('SetObjectLegalHold', [arg1, arg2, arg3]) as Promise<void>
export const SetBucketWebsite = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('SetBucketWebsite', [arg1, arg2, arg3]) as Promise<void>
export const SetBucketLogging = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('SetBucketLogging', [arg1, arg2, arg3]) as Promise<void>
export const SetRequesterPays = (arg1:string, arg2:boolean): Promise<void> => desktop.call('SetRequesterPays', [arg1, arg2]) as Promise<void>
export const GetBucketSettings = (arg1:string): Promise<BucketSettings> => desktop.call('GetBucketSettings', [arg1]) as Promise<BucketSettings>
export const SetBucketPolicy = (arg1:string, arg2:string): Promise<void> => desktop.call('SetBucketPolicy', [arg1, arg2]) as Promise<void>
export const SetBucketCors = (arg1:string, arg2:string): Promise<void> => desktop.call('SetBucketCors', [arg1, arg2]) as Promise<void>
export const SetBucketLifecycle = (arg1:string, arg2:string): Promise<void> => desktop.call('SetBucketLifecycle', [arg1, arg2]) as Promise<void>
export const SetBucketTags = (arg1:string, arg2:Record<string, string>): Promise<void> => desktop.call('SetBucketTags', [arg1, arg2]) as Promise<void>
export const SetBucketEncryption = (arg1:string, arg2:string, arg3:string): Promise<void> => desktop.call('SetBucketEncryption', [arg1, arg2, arg3]) as Promise<void>
export const SetPublicAccessBlock = (arg1:string, arg2:PublicAccessBlock): Promise<void> => desktop.call('SetPublicAccessBlock', [arg1, arg2]) as Promise<void>
export type ObjectSettings = {
  storageClass: string; tags: Record<string, string>; metadata: Record<string, string>; public: boolean
  restore: string; encryption: string; retention: ObjectRetention; unsupported: Record<string, string>
}
export const GetObjectSettings = (arg1:string, arg2:string): Promise<ObjectSettings> => desktop.call('GetObjectSettings', [arg1, arg2]) as Promise<ObjectSettings>
export const SetObjectTags = (arg1:string, arg2:string, arg3:Record<string, string>): Promise<void> => desktop.call('SetObjectTags', [arg1, arg2, arg3]) as Promise<void>
export const SetObjectMetadata = (arg1:string, arg2:string, arg3:Record<string, string>): Promise<void> => desktop.call('SetObjectMetadata', [arg1, arg2, arg3]) as Promise<void>
export const SetStorageClass = (arg1:string, arg2:Array<string>, arg3:string): Promise<number> => desktop.call('SetStorageClass', [arg1, arg2, arg3]) as Promise<number>
export const SetObjectPublic = (arg1:string, arg2:string, arg3:boolean): Promise<void> => desktop.call('SetObjectPublic', [arg1, arg2, arg3]) as Promise<void>
export const RestoreObject = (arg1:string, arg2:string, arg3:number, arg4:string): Promise<void> => desktop.call('RestoreObject', [arg1, arg2, arg3, arg4]) as Promise<void>
export type Mount = { id: string; profileId: string; profile: string; bucket: string; prefix: string; path: string; readOnly: boolean }
export type MountEvent = { id: string; state: string; error: string }
export const MountBucket = (arg1:string, arg2:string, arg3:boolean): Promise<Mount> => desktop.call('MountBucket', [arg1, arg2, arg3]) as Promise<Mount>
export const UnmountBucket = (arg1:string): Promise<void> => desktop.call('UnmountBucket', [arg1]) as Promise<void>
export const ListMounts = (): Promise<Mount[]> => desktop.call('ListMounts', []) as Promise<Mount[]>
export const OpenMountFolder = (arg1:string): Promise<void> => desktop.call('OpenMountFolder', [arg1]) as Promise<void>
export type EditEvent = { key: string; state: string; error: string }
export const EditExternally = (arg1:string, arg2:string): Promise<void> => desktop.call('EditExternally', [arg1, arg2]) as Promise<void>
