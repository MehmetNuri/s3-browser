package main

import (
	"fmt"
	"sync/atomic"
)

// Backend messages shown in the UI. The frontend picks the language and
// reports it through SetLanguage; unknown languages fall back to English.
var messages = map[string]map[string]string{
	"tr": {
		"unsafeDownloadPath":       "Nesne adı seçilen indirme klasörünün dışına çıkamaz",
		"transferNotFound":         "Transfer geçmişte bulunamadı",
		"transferAlreadyRunning":   "Transfer zaten çalışıyor",
		"transferCannotRetry":      "Yalnızca başarısız veya iptal edilmiş transferler tekrar denenebilir",
		"profileBackupDestination": "Dışa aktarım için uygulamanın profiles.json dosyasından farklı bir dosya seçin",
		"exportProfiles":           "Profilleri dışa aktar",
		"importProfiles":           "Profilleri içe aktar",
		"invalidProfileBackup":     "Geçersiz profil dosyası veya desteklenmeyen sürüm",
		"profileBackupTooLarge":    "Profil dosyası 1 MB'den büyük olamaz",
		"profileNotFound":          "profil bulunamadı",
		"renameTarget":             "Farklı ve boş olmayan bir ad girin",
		"entityTooLarge":           "Sunucu dosyayı reddetti: depolama servisinin izin verdiği en büyük dosya boyutunu aşıyor (HTTP 413). Supabase'de bu sınır proje ayarlarında Storage → \"Upload file size limit\" altındadır.",
		"mirrorEmptySource":        "Kaynak klasör boş veya erişilemiyor; %d nesnenin silinmesi engellendi",
		"dlgBackupFolder":          "Yedeklenecek klasör",
		"backupNotFound":           "Yedekleme işi bulunamadı",
		"backupRunning":            "Bu yedekleme zaten çalışıyor",
		"backupSummary":            "%d yüklendi, %d güncel, %d silindi",
		"backupNameRequired":       "Yedekleme adı gerekli",
		"editTooLarge":             "Yalnızca 64 KB'den küçük, sıkıştırılmamış metin nesneleri düzenlenebilir",
		"objectChanged":            "Nesne, açıldıktan sonra değişti. Yeniden açıp tekrar deneyin.",
		"secretsLocked":            "Kayıtlı erişim anahtarı çözülemedi (sistem anahtar zinciri kilitli veya değişmiş). Profili düzenleyip anahtarı yeniden girin.",
		"copyFailed":               "%d nesne kopyalanamadı",
		"copySameLocation":         "Hedef, kaynakla aynı konum olamaz",
		"bucketRequired":           "Hedef bucket gerekli",
		"dlgSyncFolder":            "Eşitlenecek klasör",
		"dlgSaveVersion":           "Sürümü kaydet",
		"invalidHeader":            "Başlık değeri satır sonu veya denetim karakteri içeremez",
		"searchQueryRequired":      "Aranacak metni girin",
		"versionRequired":          "Sürüm kimliği gerekli",
		"previewTooLarge":          "Görsel önizleme için çok büyük (en fazla 4 MB)",
		"previewUnsupported":       "Bu dosya desteklenen bir görsel değil",
		"notConnected":             "önce bir profile bağlanın",
		"nameRequired":             "profil adı gerekli",
		"connOK":                   "Bağlantı başarılı — %d bucket",
		"connOKPinned":             "Bağlantı başarılı — %d bucket erişilebilir (bucket listeleme yetkisi yok)",
		"listDeniedHint":           "%v\n\nBu kimlik bucket listeleyemiyor. Erişebildiği bucket adlarını \"Bucket'lar\" alanına yazın.",
		"uploadFailed":             "%d dosya yüklenemedi",
		"downloadFailed":           "%d dosya indirilemedi",
		"dlgUploadFiles":           "Yüklenecek dosyalar",
		"dlgUploadFolder":          "Yüklenecek klasör",
		"dlgDownloadDir":           "İndirme klasörü",
		"catService":               "Servis",
		"catBucket":                "Bucket",
		"catConfig":                "Bucket yapılandırması",
		"catObject":                "Nesne",
		"catMultipart":             "Multipart",
		"catPresigned":             "Presigned URL",
		"configMissing":            "API var, yapılandırma yok (%s)",
		"failed":                   "%s başarısız",
		"nBuckets":                 "%d bucket",
		"locEmpty":                 "us-east-1 (boş)",
		"objsPrefixes":             "%d nesne, %d prefix",
		"openUploads":              "%d açık upload",
		"metaKept":                 "metadata korundu",
		"metaLost":                 "metadata korunmadı",
		"headInfo":                 "%d bayt, %s, %s",
		"contentMismatch":          "içerik uyuşmuyor",
		"contentOK":                "içerik doğrulandı",
		"rangeIgnored":             "range yok sayıldı (%d bayt döndü)",
		"condIgnored":              "koşullu istek yok sayıldı (200 döndü)",
		"tagsIgnored":              "API yanıt veriyor ama tag'ler kaydedilmiyor (PutObjectTagging yok sayılıyor)",
		"nTags":                    "%d tag",
		"nParts":                   "%d part",
		"nDeleted":                 "%d nesne silindi",
		"nErrors":                  "%d hata: %s",
		"keySpace":                 "Key: boşluk",
		"keySpecial":               "Key: özel karakter (+ & = ,)",
		"keyUnicode":               "Key: Unicode (ü ş ğ ı)",
		"objectTagging":            "Nesne tag'leri",
	},
	"en": {
		"unsafeDownloadPath":       "Object name must stay within the selected download directory",
		"transferNotFound":         "Transfer no longer exists in history",
		"transferAlreadyRunning":   "Transfer is already running",
		"transferCannotRetry":      "Only failed or cancelled transfers can be retried",
		"profileBackupDestination": "Choose a destination other than the application's profiles.json file",
		"exportProfiles":           "Export profiles",
		"importProfiles":           "Import profiles",
		"invalidProfileBackup":     "Invalid profile file or unsupported version",
		"profileBackupTooLarge":    "Profile file must not exceed 1 MB",
		"profileNotFound":          "profile not found",
		"renameTarget":             "Enter a different, non-empty name",
		"entityTooLarge":           "The server rejected the file: it exceeds the largest file size the storage service allows (HTTP 413). On Supabase this limit is in the project settings under Storage → \"Upload file size limit\".",
		"mirrorEmptySource":        "The source folder is empty or unavailable; deleting %d objects was refused",
		"dlgBackupFolder":          "Folder to back up",
		"backupNotFound":           "Backup job not found",
		"backupRunning":            "This backup is already running",
		"backupSummary":            "%d uploaded, %d up to date, %d deleted",
		"backupNameRequired":       "A backup name is required",
		"editTooLarge":             "Only uncompressed text objects up to 64 KB can be edited",
		"objectChanged":            "The object changed after it was opened. Open it again and retry.",
		"secretsLocked":            "The stored secret key could not be decrypted (the system keychain is locked or has changed). Edit the profile and enter the key again.",
		"copyFailed":               "%d objects failed to copy",
		"copySameLocation":         "The destination must differ from the source location",
		"bucketRequired":           "A destination bucket is required",
		"dlgSyncFolder":            "Folder to synchronize",
		"dlgSaveVersion":           "Save version",
		"invalidHeader":            "Header values must not contain line breaks or control characters",
		"searchQueryRequired":      "Enter the text to search for",
		"versionRequired":          "A version ID is required",
		"previewTooLarge":          "Image is too large to preview (4 MB at most)",
		"previewUnsupported":       "This file is not a supported image",
		"notConnected":             "connect to a profile first",
		"nameRequired":             "profile name is required",
		"connOK":                   "Connected — %d buckets",
		"connOKPinned":             "Connected — %d buckets reachable (no permission to list buckets)",
		"listDeniedHint":           "%v\n\nThese credentials cannot list buckets. Enter the bucket names they can access in the \"Buckets\" field.",
		"uploadFailed":             "%d files failed to upload",
		"downloadFailed":           "%d files failed to download",
		"dlgUploadFiles":           "Files to upload",
		"dlgUploadFolder":          "Folder to upload",
		"dlgDownloadDir":           "Download folder",
		"catService":               "Service",
		"catBucket":                "Bucket",
		"catConfig":                "Bucket configuration",
		"catObject":                "Object",
		"catMultipart":             "Multipart",
		"catPresigned":             "Presigned URL",
		"configMissing":            "API available, no configuration set (%s)",
		"failed":                   "%s failed",
		"nBuckets":                 "%d buckets",
		"locEmpty":                 "us-east-1 (empty)",
		"objsPrefixes":             "%d objects, %d prefixes",
		"openUploads":              "%d open uploads",
		"metaKept":                 "metadata preserved",
		"metaLost":                 "metadata not preserved",
		"headInfo":                 "%d bytes, %s, %s",
		"contentMismatch":          "content mismatch",
		"contentOK":                "content verified",
		"rangeIgnored":             "range ignored (%d bytes returned)",
		"condIgnored":              "conditional request ignored (200 returned)",
		"tagsIgnored":              "API responds but tags are not stored (PutObjectTagging is ignored)",
		"nTags":                    "%d tags",
		"nParts":                   "%d parts",
		"nDeleted":                 "%d objects deleted",
		"nErrors":                  "%d errors: %s",
		"keySpace":                 "Key: space",
		"keySpecial":               "Key: special characters (+ & = ,)",
		"keyUnicode":               "Key: Unicode (ü ş ğ ı)",
		"objectTagging":            "Object tagging",
	},
}

var currentLang atomic.Value

func init() { currentLang.Store("tr") }

// T returns the message for key in the current language, formatted with args.
func T(key string, args ...any) string {
	m, ok := messages[currentLang.Load().(string)]
	if !ok {
		m = messages["en"]
	}
	s, ok := m[key]
	if !ok {
		if s, ok = messages["en"][key]; !ok {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// SetLanguage switches backend messages ("tr" or "en").
func (a *App) SetLanguage(lang string) {
	if _, ok := messages[lang]; ok {
		currentLang.Store(lang)
		if a.emitEvent != nil {
			a.emitEvent("language-changed", lang)
		}
	}
}
