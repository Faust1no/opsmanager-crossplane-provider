package clients

import (
	"context"
	stderrors "errors"
	"net/http"
	"time"

	"go.mongodb.org/ops-manager/opsmngr"
)

// s3ValidationErrorCode is the Ops Manager error code returned when a blockstore
// or oplog-store Update fails its post-write S3 smoke test
// (validateHeadAfterWrite, validatePutCopy, validatePutOverwrite,
// validateDeleteAfterPut). The first probe after a config change often hits a
// transient 403 from S3 / MinIO; subsequent probes succeed.
const s3ValidationErrorCode = "BACKUP-S3-VALIDATION_FAILED"

// IsS3ValidationConflict reports whether err is an Ops Manager 409 with the
// S3 validation error code.
func IsS3ValidationConflict(err error) bool {
	if err == nil {
		return false
	}
	var e *opsmngr.ErrorResponse
	if !stderrors.As(err, &e) || e.Response == nil {
		return false
	}
	return e.Response.StatusCode == http.StatusConflict && e.ErrorCode == s3ValidationErrorCode
}

// RetryOnS3Validation runs op, and on a BACKUP-S3-VALIDATION_FAILED 409 retries
// up to maxAttempts (total tries, including the first) with a fixed backoff.
// Any other error or a successful call returns immediately. The context's
// deadline is honored — if it fires during a backoff the most recent error is
// returned.
func RetryOnS3Validation(ctx context.Context, maxAttempts int, backoff time.Duration, op func() error) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = op()
		if err == nil || !IsS3ValidationConflict(err) {
			return err
		}
		if attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff):
		}
	}
	return err
}
