package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	runtime "s3browser/internal/desktop"
)

// BackupJob is a saved sync of one local folder into a bucket. Its folder is
// always chosen in a native dialog; the renderer cannot supply the path.
type BackupJob struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ProfileID       string `json:"profileId"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	Dir             string `json:"dir"`
	Mirror          bool   `json:"mirror"`          // also delete objects that left the folder
	IntervalMinutes int    `json:"intervalMinutes"` // 0 runs only on request
	LastRun         string `json:"lastRun"`         // RFC 3339
	LastStatus      string `json:"lastStatus"`      // "" | ok | error
	LastMessage     string `json:"lastMessage"`
	Running         bool   `json:"running"` // not stored
}

// Scheduled jobs run only while the application is open, including in the tray.
const minBackupInterval = 5

type backupStore struct {
	mu      sync.Mutex
	path    string
	running map[string]bool
}

func (a *App) backups() *backupStore {
	a.backupOnce.Do(func() {
		if a.backupStore == nil {
			a.backupStore = &backupStore{path: filepath.Join(filepath.Dir(a.store.path), "backups.json")}
		}
	})
	return a.backupStore
}

func (s *backupStore) load() ([]BackupJob, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupJob{}, nil
	}
	if err != nil {
		return nil, err
	}
	jobs := []BackupJob{}
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	sort.SliceStable(jobs, func(i, j int) bool { return strings.ToLower(jobs[i].Name) < strings.ToLower(jobs[j].Name) })
	return jobs, nil
}

func (s *backupStore) save(jobs []BackupJob) error {
	stored := make([]BackupJob, len(jobs))
	for i, job := range jobs {
		job.Running = false
		stored[i] = job
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(s.path, data)
}

// update loads the jobs, lets change edit them and stores the result.
func (s *backupStore) update(change func([]BackupJob) ([]BackupJob, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs, err := s.load()
	if err != nil {
		return err
	}
	if jobs, err = change(jobs); err != nil {
		return err
	}
	return s.save(jobs)
}

func (s *backupStore) removeProfile(profileID string) error {
	return s.update(func(jobs []BackupJob) ([]BackupJob, error) {
		kept := jobs[:0]
		for _, job := range jobs {
			if job.ProfileID != profileID {
				kept = append(kept, job)
			}
		}
		return kept, nil
	})
}

func (a *App) ListBackupJobs() ([]BackupJob, error) {
	s := a.backups()
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs, err := s.load()
	for i := range jobs {
		jobs[i].Running = s.running[jobs[i].ID]
	}
	return jobs, err
}

// CreateBackupJob asks for a local folder and saves a job that backs it up
// into the given bucket of the active profile.
func (a *App) CreateBackupJob(name, bucket, prefix string, mirror bool, intervalMinutes int) (BackupJob, error) {
	if _, err := a.cli(); err != nil {
		return BackupJob{}, err
	}
	a.mu.RLock()
	profileID := a.profile.ID
	a.mu.RUnlock()
	name, bucket = strings.TrimSpace(name), strings.TrimSpace(bucket)
	if name == "" {
		return BackupJob{}, errors.New(T("backupNameRequired"))
	}
	if bucket == "" {
		return BackupJob{}, errors.New(T("bucketRequired"))
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: T("dlgBackupFolder")})
	if err != nil || dir == "" {
		return BackupJob{}, err
	}
	return a.addBackupJob(BackupJob{Name: name, ProfileID: profileID, Bucket: bucket, Prefix: prefix, Dir: dir, Mirror: mirror, IntervalMinutes: intervalMinutes})
}

func (a *App) addBackupJob(job BackupJob) (BackupJob, error) {
	job.ID = randID()
	job.Prefix = strings.TrimLeft(strings.TrimSpace(job.Prefix), "/")
	if job.Prefix != "" && !strings.HasSuffix(job.Prefix, "/") {
		job.Prefix += "/"
	}
	if job.IntervalMinutes < 0 {
		job.IntervalMinutes = 0
	} else if job.IntervalMinutes > 0 && job.IntervalMinutes < minBackupInterval {
		job.IntervalMinutes = minBackupInterval
	}
	err := a.backups().update(func(jobs []BackupJob) ([]BackupJob, error) { return append(jobs, job), nil })
	return job, err
}

func (a *App) DeleteBackupJob(id string) error {
	return a.backups().update(func(jobs []BackupJob) ([]BackupJob, error) {
		kept := jobs[:0]
		for _, job := range jobs {
			if job.ID != id {
				kept = append(kept, job)
			}
		}
		if len(kept) == len(jobs) {
			return nil, errors.New(T("backupNotFound"))
		}
		return kept, nil
	})
}

// RunBackupJob runs a job now, with its own connection, so it works for any
// saved profile and does not disturb the one being browsed.
func (a *App) RunBackupJob(id string) (SyncResult, error) {
	s := a.backups()
	s.mu.Lock()
	jobs, err := s.load()
	var job *BackupJob
	for i := range jobs {
		if jobs[i].ID == id {
			job = &jobs[i]
		}
	}
	switch {
	case err != nil:
	case job == nil:
		err = errors.New(T("backupNotFound"))
	case s.running[id]:
		err = errors.New(T("backupRunning"))
	default:
		if s.running == nil {
			s.running = make(map[string]bool)
		}
		s.running[id] = true
	}
	s.mu.Unlock()
	if err != nil {
		return SyncResult{}, err
	}
	a.emitEvent("backup", id)

	result, runErr := a.runBackup(*job)
	status, message := "ok", T("backupSummary", result.Uploaded, result.Skipped, result.Deleted)
	if runErr != nil {
		status, message = "error", describeErr(runErr).Error()
	}
	finished := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	delete(s.running, id)
	s.mu.Unlock()
	_ = s.update(func(jobs []BackupJob) ([]BackupJob, error) {
		for i := range jobs {
			if jobs[i].ID == id {
				jobs[i].LastRun, jobs[i].LastStatus, jobs[i].LastMessage = finished, status, message
			}
		}
		return jobs, nil
	})
	a.emitEvent("backup", id)
	return result, runErr
}

func (a *App) runBackup(job BackupJob) (SyncResult, error) {
	profile, err := a.store.get(job.ProfileID)
	if err != nil {
		return SyncResult{}, err
	}
	c, err := newClient(a.ctx, profile)
	if err != nil {
		return SyncResult{}, err
	}
	return a.syncDirectory(c, job.Bucket, job.Prefix, job.Dir, job.Mirror)
}

// backupDue reports whether a scheduled job should run at the given time.
func backupDue(job BackupJob, now time.Time) bool {
	if job.IntervalMinutes <= 0 {
		return false
	}
	last, err := time.Parse(time.RFC3339, job.LastRun)
	if err != nil {
		return true // never ran
	}
	return !now.Before(last.Add(time.Duration(job.IntervalMinutes) * time.Minute))
}

// runBackupSchedule starts due jobs, one after another, until ctx ends.
func (a *App) runBackupSchedule(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			jobs, err := a.ListBackupJobs()
			if err != nil {
				continue
			}
			for _, job := range jobs {
				if ctx.Err() == nil && !job.Running && backupDue(job, now) {
					_, _ = a.RunBackupJob(job.ID)
				}
			}
		}
	}
}
