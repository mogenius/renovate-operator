package objectstore

// S3Config holds the configuration for connecting to an S3-compatible object store.
// Either a real AWS bucket or a custom endpoint for S3-compatible stores is supported.
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string // optional; set for S3-compatible self-hosted stores
	ForcePathStyle  bool   // optional; enable path-style addressing (required by some S3-compatible stores)
	AccessKeyID     string // optional; uses the default credential chain when empty
	SecretAccessKey string // optional; uses the default credential chain when empty
}

// IsConfigured reports whether the minimum required field (Bucket) is set.
func (c S3Config) IsConfigured() bool {
	return c.Bucket != ""
}

// HasStaticCredentials reports whether explicit access-key credentials are set.
func (c S3Config) HasStaticCredentials() bool {
	return c.AccessKeyID != "" && c.SecretAccessKey != ""
}
