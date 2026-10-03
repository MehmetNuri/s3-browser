package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Profile is a saved S3 connection.
type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"` // aws | supabase | minio | r2 | a preset name | custom
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	AccessKey    string `json:"accessKey"`
	SecretKey    string `json:"secretKey"`
	SessionToken string `json:"sessionToken"`
	PathStyle    bool   `json:"pathStyle"`
	SkipTLS      bool   `json:"skipTLS"`
	// Supabase helper: project ref, used to build the endpoint.
	ProjectRef string `json:"projectRef"`
	// Buckets added by hand, for credentials without s3:ListAllMyBuckets.
	Buckets []string `json:"buckets"`
	// Defaults for new uploads and copies into this profile; empty keeps the provider's default.
	StorageClass string `json:"storageClass"`
	Encryption   string `json:"encryption"` // "" | AES256 | aws:kms
	KMSKey       string `json:"kmsKey"`
}

type profileStore struct {
	mu     sync.Mutex
	path   string
	cipher secretCipher // nil stores secrets as plaintext
}

func newProfileStore() *profileStore {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, "s3browser")
	_ = os.MkdirAll(dir, 0o700)
	return &profileStore{path: filepath.Join(dir, "profiles.json")}
}

func (s *profileStore) load() ([]Profile, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []Profile
	if err := json.Unmarshal(b, &ps); err != nil {
		return nil, err
	}
	sort.Slice(ps, func(i, j int) bool { return strings.ToLower(ps[i].Name) < strings.ToLower(ps[j].Name) })
	// Secrets written before encryption was available are protected on first use.
	if s.unseal(ps) && s.cipher != nil && s.cipher.Available() {
		_ = s.save(ps)
	}
	return ps, nil
}

func (s *profileStore) save(ps []Profile) error {
	ps, err := s.seal(ps)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(s.path, b)
}

func (s *profileStore) list() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *profileStore) upsert(p Profile) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return p, err
	}
	if p.ID == "" {
		p.ID = randID()
	}
	found := false
	for i := range ps {
		if ps[i].ID == p.ID {
			ps[i] = p
			found = true
		}
	}
	if !found {
		ps = append(ps, p)
	}
	return p, s.save(ps)
}

func (s *profileStore) remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.load()
	if err != nil {
		return err
	}
	out := ps[:0]
	for _, p := range ps {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return s.save(out)
}

func (s *profileStore) get(id string) (Profile, error) {
	ps, err := s.list()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, errors.New(T("profileNotFound"))
}

func randID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// normalize fills provider-specific defaults.
func normalize(p Profile) Profile {
	seen := map[string]bool{}
	bs := []string{}
	for _, b := range p.Buckets {
		b = strings.TrimSpace(b)
		if b != "" && !seen[b] {
			seen[b] = true
			bs = append(bs, b)
		}
	}
	p.Buckets = bs
	// Pasted values often carry spaces or a line break, which breaks request signing.
	for _, field := range []*string{&p.Name, &p.Region, &p.AccessKey, &p.SecretKey, &p.SessionToken, &p.ProjectRef} {
		*field = strings.TrimSpace(*field)
	}
	p.Endpoint = strings.TrimRight(strings.TrimSpace(p.Endpoint), "/")
	p.StorageClass, p.Encryption, p.KMSKey = strings.TrimSpace(p.StorageClass), strings.TrimSpace(p.Encryption), strings.TrimSpace(p.KMSKey)
	if !storageClasses[p.StorageClass] {
		p.StorageClass = ""
	}
	if p.Encryption != string(types.ServerSideEncryptionAes256) && p.Encryption != string(types.ServerSideEncryptionAwsKms) {
		p.Encryption, p.KMSKey = "", ""
	}
	if !validHeaderValue(p.KMSKey) {
		p.KMSKey = ""
	}
	switch p.Provider {
	case "supabase":
		if p.Endpoint == "" && p.ProjectRef != "" {
			p.Endpoint = "https://" + p.ProjectRef + ".storage.supabase.co/storage/v1/s3"
		}
		p.PathStyle = true
	case "minio", "storj", "gcs", "backblaze":
		p.PathStyle = true
	case "r2":
		if p.Region == "" {
			p.Region = "auto"
		}
	}
	if p.Region == "" {
		p.Region = "us-east-1"
	}
	return p
}

func newClient(ctx context.Context, p Profile) (*s3.Client, error) {
	if isSealed(p.SecretKey) || isSealed(p.SessionToken) {
		return nil, errors.New(T("secretsLocked"))
	}
	p = normalize(p)
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(p.Region),
		// Many S3-compatible backends (Supabase, MinIO, R2...) reject the
		// default CRC checksums added by newer SDKs.
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	}
	if p.AccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(p.AccessKey, p.SecretKey, p.SessionToken)))
	}
	if p.SkipTLS {
		opts = append(opts, config.WithHTTPClient(awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		})))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if p.Endpoint != "" {
			o.BaseEndpoint = aws.String(p.Endpoint)
		}
		o.UsePathStyle = p.PathStyle
	}), nil
}

// uploadOptions are the per-profile defaults applied to objects this
// application writes.
type uploadOptions struct {
	storageClass types.StorageClass
	encryption   types.ServerSideEncryption
	kmsKey       string
	// sameStorage marks a copy whose source and destination profiles reach the same service.
	sameStorage bool
}

func (p Profile) uploadOptions() uploadOptions {
	return uploadOptions{storageClass: types.StorageClass(p.StorageClass), encryption: types.ServerSideEncryption(p.Encryption), kmsKey: p.KMSKey}
}

// apply sets the defaults on a put request; empty values leave the request alone.
func (o uploadOptions) apply(in *s3.PutObjectInput) {
	if o.storageClass != "" {
		in.StorageClass = o.storageClass
	}
	if o.encryption != "" {
		in.ServerSideEncryption = o.encryption
		if o.encryption == types.ServerSideEncryptionAwsKms && o.kmsKey != "" {
			in.SSEKMSKeyId = aws.String(o.kmsKey)
		}
	}
}

// activeUploadOptions are the defaults of the connected profile.
func (a *App) activeUploadOptions() uploadOptions {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.profile.uploadOptions()
}
