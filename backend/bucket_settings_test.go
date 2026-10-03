package main

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// configServer stores bucket and object sub-resources (policy, cors, tagging,
// …) as the raw documents the SDK sends, since the PUT bodies and GET
// responses of these resources share the same schema. Everything else goes
// to gofakes3.
type configServer struct {
	mu   sync.Mutex
	docs map[string][]byte
	next http.Handler
	// Resources answered with 501, to imitate providers that lack them.
	unsupported map[string]bool
}

var configResources = []string{"policy", "cors", "lifecycle", "tagging", "encryption", "publicAccessBlock", "acl", "restore", "website", "logging", "requestPayment", "object-lock", "retention", "legal-hold"}

// The error codes S3 answers with while a resource was never configured.
var missingCodes = map[string]string{
	"policy": "NoSuchBucketPolicy", "cors": "NoSuchCORSConfiguration", "lifecycle": "NoSuchLifecycleConfiguration",
	"tagging": "NoSuchTagSet", "encryption": "ServerSideEncryptionConfigurationNotFoundError",
	"publicAccessBlock": "NoSuchPublicAccessBlockConfiguration", "website": "NoSuchWebsiteConfiguration",
	"object-lock": "ObjectLockConfigurationNotFoundError", "retention": "NoSuchObjectLockConfiguration", "legal-hold": "NoSuchObjectLockConfiguration",
}

func newConfigServer() *configServer {
	return &configServer{docs: map[string][]byte{}, next: gofakes3.New(s3mem.New()).Server(), unsupported: map[string]bool{}}
}

func (s *configServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, name := range configResources {
		if !r.URL.Query().Has(name) {
			continue
		}
		if s.unsupported[name] {
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = io.WriteString(w, `<Error><Code>NotImplemented</Code><Message>not here</Message></Error>`)
			return
		}
		key := name + " " + r.URL.Path
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method {
		case http.MethodPut, http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			// A canned ACL arrives as a header; store the grants it stands for.
			if name == "acl" && len(body) == 0 {
				grants := ""
				if r.Header.Get("x-amz-acl") == "public-read" {
					grants = `<Grant><Grantee xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="Group"><URI>http://acs.amazonaws.com/groups/global/AllUsers</URI></Grantee><Permission>READ</Permission></Grant>`
				}
				body = []byte(`<AccessControlPolicy><Owner><ID>me</ID></Owner><AccessControlList>` + grants + `</AccessControlList></AccessControlPolicy>`)
			}
			s.docs[key] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(s.docs, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			doc, ok := s.docs[key]
			if !ok && name == "logging" {
				doc, ok = []byte(`<BucketLoggingStatus xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></BucketLoggingStatus>`), true
			}
			if !ok && name == "requestPayment" {
				doc, ok = []byte(`<RequestPaymentConfiguration><Payer>BucketOwner</Payer></RequestPaymentConfiguration>`), true
			}
			if !ok {
				if name == "acl" {
					_, _ = io.WriteString(w, `<AccessControlPolicy><Owner><ID>me</ID></Owner><AccessControlList></AccessControlList></AccessControlPolicy>`)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<Error><Code>`+missingCodes[name]+`</Code></Error>`)
				return
			}
			_, _ = w.Write(doc)
		}
		return
	}
	s.next.ServeHTTP(w, r)
}

func TestBucketSettingsRoundTrip(t *testing.T) {
	srv := newConfigServer()
	a := connectedApp(t, srv)
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::test/*"}]}`
	if err := a.SetBucketPolicy("test", policy); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketPolicy("test", "{not json"); err == nil {
		t.Fatal("invalid policy was accepted")
	}
	cors := `[{"AllowedMethods":["GET"],"AllowedOrigins":["https://example.com"],"MaxAgeSeconds":300}]`
	if err := a.SetBucketCors("test", cors); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketCors("test", `[{"Bogus":1}]`); err == nil {
		t.Fatal("unknown rule field was accepted")
	}
	lifecycle := `[{"ID":"expire-logs","Status":"Enabled","Filter":{"Prefix":"logs/"},"Expiration":{"Days":30}}]`
	if err := a.SetBucketLifecycle("test", lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketTags("test", map[string]string{"env": "prod", "team": "data"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketTags("test", map[string]string{"": "x"}); err == nil {
		t.Fatal("empty tag key was accepted")
	}
	if err := a.SetBucketEncryption("test", "aws:kms", "alias/app"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketEncryption("test", "ROT13", ""); err == nil {
		t.Fatal("unknown algorithm was accepted")
	}
	if err := a.SetPublicAccessBlock("test", PublicAccessBlock{BlockPublicAcls: true, RestrictPublicBuckets: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketWebsite("test", "index.html", "404.html"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetBucketWebsite("test", "docs/index.html", ""); err == nil {
		t.Fatal("index document with a path was accepted")
	}
	if err := a.SetBucketLogging("test", "logs-bucket", "access/"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetRequesterPays("test", true); err != nil {
		t.Fatal(err)
	}
	s, err := a.GetBucketSettings("test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Policy, `"s3:GetObject"`) || !strings.Contains(s.Policy, "\n  ") {
		t.Errorf("policy: %q", s.Policy)
	}
	if !strings.Contains(s.Cors, `"https://example.com"`) || !strings.Contains(s.Cors, `"MaxAgeSeconds": 300`) {
		t.Errorf("cors: %q", s.Cors)
	}
	if !strings.Contains(s.Lifecycle, `"expire-logs"`) || !strings.Contains(s.Lifecycle, `"Days": 30`) {
		t.Errorf("lifecycle: %q", s.Lifecycle)
	}
	if s.Tags["env"] != "prod" || s.Tags["team"] != "data" {
		t.Errorf("tags: %v", s.Tags)
	}
	if s.Encryption != "aws:kms" || s.KMSKey != "alias/app" {
		t.Errorf("encryption: %q %q", s.Encryption, s.KMSKey)
	}
	if s.PublicAccessBlock == nil || !s.PublicAccessBlock.BlockPublicAcls || s.PublicAccessBlock.BlockPublicPolicy || !s.PublicAccessBlock.RestrictPublicBuckets {
		t.Errorf("public access block: %+v", s.PublicAccessBlock)
	}
	if s.WebsiteIndex != "index.html" || s.WebsiteError != "404.html" || s.LoggingBucket != "logs-bucket" || s.LoggingPrefix != "access/" || !s.RequesterPays {
		t.Errorf("website/logging/payer: %+v", s)
	}
	if len(s.Unsupported) != 0 {
		t.Errorf("unsupported: %v", s.Unsupported)
	}
	// Clearing removes the documents instead of storing empty ones.
	for name, clear := range map[string]func() error{
		"policy":     func() error { return a.SetBucketPolicy("test", " ") },
		"cors":       func() error { return a.SetBucketCors("test", "") },
		"lifecycle":  func() error { return a.SetBucketLifecycle("test", "[]") },
		"tags":       func() error { return a.SetBucketTags("test", nil) },
		"encryption": func() error { return a.SetBucketEncryption("test", "", "") },
		"website":    func() error { return a.SetBucketWebsite("test", "", "") },
		"logging":    func() error { return a.SetBucketLogging("test", "", "") },
		"payer":      func() error { return a.SetRequesterPays("test", false) },
	} {
		if err := clear(); err != nil {
			t.Fatalf("clear %s: %v", name, err)
		}
	}
	s, err = a.GetBucketSettings("test")
	if err != nil {
		t.Fatal(err)
	}
	if s.Policy != "" || s.Cors != "" || s.Lifecycle != "" || len(s.Tags) != 0 || s.Encryption != "" || s.WebsiteIndex != "" || s.LoggingBucket != "" || s.RequesterPays {
		t.Errorf("not cleared: %+v", s)
	}
}

func TestBucketSettingsReportUnsupportedFeatures(t *testing.T) {
	srv := newConfigServer()
	srv.unsupported["lifecycle"], srv.unsupported["publicAccessBlock"] = true, true
	a := connectedApp(t, srv)
	s, err := a.GetBucketSettings("test")
	if err != nil {
		t.Fatal(err)
	}
	if s.Unsupported["lifecycle"] == "" || s.Unsupported["publicAccessBlock"] == "" || s.PublicAccessBlock != nil {
		t.Fatalf("unsupported: %v, block %+v", s.Unsupported, s.PublicAccessBlock)
	}
	if _, ok := s.Unsupported["policy"]; ok {
		t.Fatal("a missing policy is not an unsupported feature")
	}
	if err := a.SetBucketLifecycle("test", `[{"ID":"x","Status":"Enabled","Expiration":{"Days":1}}]`); err == nil {
		t.Fatal("expected the provider's error")
	}
}

func TestObjectSettings(t *testing.T) {
	srv := newConfigServer()
	a := connectedApp(t, srv)
	putObject(t, a, "docs/readme.txt", "hello")
	if err := a.SetObjectTags("test", "docs/readme.txt", map[string]string{"kind": "doc"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetObjectMetadata("test", "docs/readme.txt", map[string]string{"Author": "Mehmet", "bad key": "x"}); err == nil {
		t.Fatal("metadata name with a space was accepted")
	}
	if err := a.SetObjectMetadata("test", "docs/readme.txt", map[string]string{"Author": "Mehmet"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetObjectPublic("test", "docs/readme.txt", true); err != nil {
		t.Fatal(err)
	}
	// Rewriting the object in place must keep it public.
	if err := a.SetObjectMetadata("test", "docs/readme.txt", map[string]string{"author": "Mehmet", "rev": "2"}); err != nil {
		t.Fatal(err)
	}
	s, err := a.GetObjectSettings("test", "docs/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if s.Tags["kind"] != "doc" || s.Metadata["author"] != "Mehmet" || !s.Public || s.StorageClass != "STANDARD" {
		t.Fatalf("settings: %+v", s)
	}
	info, err := a.HeadObject("test", "docs/readme.txt")
	if err != nil || info.Size != 5 || info.ContentType == "" {
		t.Fatalf("object after metadata copy: %+v, %v", info, err)
	}
	if _, err := a.SetStorageClass("test", []string{"docs/readme.txt"}, "SILLY"); err == nil {
		t.Fatal("invalid class was accepted")
	}
	if err := a.RestoreObject("test", "docs/readme.txt", 0, ""); err == nil {
		t.Fatal("zero days was accepted")
	}
	if err := a.RestoreObject("test", "docs/readme.txt", 3, "Bulk"); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	restore := string(srv.docs["restore /test/docs/readme.txt"])
	srv.mu.Unlock()
	if !strings.Contains(restore, "<Days>3</Days>") || !strings.Contains(restore, "<Tier>Bulk</Tier>") {
		t.Fatalf("restore request: %s", restore)
	}
}

func TestObjectLockSettings(t *testing.T) {
	srv := newConfigServer()
	a := connectedApp(t, srv)
	putObject(t, a, "ledger.csv", "1,2,3")
	s, err := a.GetObjectSettings("test", "ledger.csv")
	if err != nil || s.Unsupported["retention"] == "" {
		t.Fatalf("without object lock: %+v, %v", s, err)
	}
	if err := a.SetObjectLock("test", "GOVERNANCE", 30, 1); err == nil {
		t.Fatal("days and years together were accepted")
	}
	if err := a.SetObjectLock("test", "GOVERNANCE", 30, 0); err != nil {
		t.Fatal(err)
	}
	b, err := a.GetBucketSettings("test")
	if err != nil || b.ObjectLock == nil || !b.ObjectLock.Enabled || b.ObjectLock.Mode != "GOVERNANCE" || b.ObjectLock.Days != 30 {
		t.Fatalf("bucket lock: %+v, %v", b.ObjectLock, err)
	}
	if err := a.SetObjectRetention("test", "ledger.csv", "COMPLIANCE", "2000-01-01", false); err == nil {
		t.Fatal("past date was accepted")
	}
	if err := a.SetObjectRetention("test", "ledger.csv", "GOVERNANCE", "2099-12-31", false); err != nil {
		t.Fatal(err)
	}
	if err := a.SetObjectLegalHold("test", "ledger.csv", true); err != nil {
		t.Fatal(err)
	}
	s, err = a.GetObjectSettings("test", "ledger.csv")
	if err != nil {
		t.Fatal(err)
	}
	if s.Retention.Mode != "GOVERNANCE" || !strings.HasPrefix(s.Retention.Until, "2099-12-31") || !s.Retention.LegalHold || len(s.Unsupported) != 0 {
		t.Fatalf("retention: %+v, unsupported %v", s.Retention, s.Unsupported)
	}
}

func TestAclRoundTrip(t *testing.T) {
	srv := newConfigServer()
	a := connectedApp(t, srv)
	putObject(t, a, "pub.txt", "x")
	acl, err := a.GetObjectAcl("test", "pub.txt")
	if err != nil || acl.OwnerID != "me" || len(acl.Grants) != 0 {
		t.Fatalf("initial acl: %+v, %v", acl, err)
	}
	acl.Grants = []AclGrant{{Type: "Group", Grantee: groupAllUsers, Permission: "READ"}}
	if err := a.SetObjectAcl("test", "pub.txt", acl); err != nil {
		t.Fatal(err)
	}
	acl, err = a.GetObjectAcl("test", "pub.txt")
	if err != nil || len(acl.Grants) != 2 {
		t.Fatalf("acl after put: %+v, %v", acl, err)
	}
	// The owner keeps full control even when the list omitted it.
	if acl.Grants[0].Grantee != "me" || acl.Grants[0].Permission != "FULL_CONTROL" || acl.Grants[1].Grantee != groupAllUsers || acl.Grants[1].Permission != "READ" {
		t.Fatalf("grants: %+v", acl.Grants)
	}
	if err := a.SetObjectAcl("test", "pub.txt", Acl{OwnerID: "me", Grants: []AclGrant{{Type: "Group", Grantee: groupAllUsers, Permission: "ADMIN"}}}); err == nil {
		t.Fatal("unknown permission was accepted")
	}
	if err := a.SetBucketAcl("test", Acl{OwnerID: "me", Grants: []AclGrant{{Type: "Group", Grantee: groupLogDelivery, Permission: "WRITE"}}}); err != nil {
		t.Fatal(err)
	}
	bucketAcl, err := a.GetBucketAcl("test")
	if err != nil || len(bucketAcl.Grants) != 2 || bucketAcl.Grants[1].Grantee != groupLogDelivery {
		t.Fatalf("bucket acl: %+v, %v", bucketAcl, err)
	}
}

func TestSaveTextKeepsPublicAccess(t *testing.T) {
	srv := newConfigServer()
	a := connectedApp(t, srv)
	putObject(t, a, "site/index.html", "<h1>old</h1>")
	if err := a.SetObjectPublic("test", "site/index.html", true); err != nil {
		t.Fatal(err)
	}
	info, err := a.HeadObject("test", "site/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SaveText("test", "site/index.html", "<h1>new</h1>", info.ETag); err != nil {
		t.Fatal(err)
	}
	s, err := a.GetObjectSettings("test", "site/index.html")
	if err != nil || !s.Public {
		t.Fatalf("object lost public access after saving text: %+v, %v", s, err)
	}
	if text, _ := a.PreviewText("test", "site/index.html"); text != "<h1>new</h1>" {
		t.Fatalf("content: %q", text)
	}
}
