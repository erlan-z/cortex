package bucketindex

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/parquet"
)

const (
	MarkersPathname = "markers"
)

var (
	MarkersMap = map[string]func(ulid.ULID) string{
		metadata.DeletionMarkFilename:   BlockDeletionMarkFilepath,
		metadata.NoCompactMarkFilename:  NoCompactMarkFilenameMarkFilepath,
		parquet.ConverterMarkerFileName: ConverterMarkFilePath,
	}
)

// BlockDeletionMarkFilepath returns the path, relative to the tenant's bucket location,
// of a block deletion mark in the bucket markers location.
func BlockDeletionMarkFilepath(blockID ulid.ULID) string {
	return fmt.Sprintf("%s/%s-%s", MarkersPathname, blockID.String(), metadata.DeletionMarkFilename)
}

// NoCompactMarkFilenameMarkFilepath returns the path, relative to the tenant's bucket location,
// of a block no compact mark in the bucket markers location.
func NoCompactMarkFilenameMarkFilepath(blockID ulid.ULID) string {
	return fmt.Sprintf("%s/%s-%s", MarkersPathname, blockID.String(), metadata.NoCompactMarkFilename)
}

func ConverterMarkFilePath(blockID ulid.ULID) string {
	return fmt.Sprintf("%s/%s-%s", parquet.ConverterMarkerPrefix, blockID.String(), parquet.ConverterMarkerFileName)
}

// IsBlockDeletionMarkFilename returns whether the input filename matches the expected pattern
// of block deletion markers stored in the markers location.
func IsBlockDeletionMarkFilename(name string) (ulid.ULID, bool) {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return ulid.ULID{}, false
	}

	// Ensure the 2nd part matches the block deletion mark filename.
	if parts[1] != metadata.DeletionMarkFilename {
		return ulid.ULID{}, false
	}

	// Ensure the 1st part is a valid block ID.
	id, err := ulid.Parse(filepath.Base(parts[0]))
	return id, err == nil
}

// IsBlockNoCompactMarkFilename returns whether the input filename matches the expected pattern
// of block no compact markers stored in the markers location.
func IsBlockNoCompactMarkFilename(name string) (ulid.ULID, bool) {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return ulid.ULID{}, false
	}

	// Ensure the 2nd part matches the block deletion mark filename.
	if parts[1] != metadata.NoCompactMarkFilename {
		return ulid.ULID{}, false
	}

	// Ensure the 1st part is a valid block ID.
	id, err := ulid.Parse(filepath.Base(parts[0]))
	return id, err == nil
}

// IsBlockParquetConverterMarkFilename returns whether the input filename matches the expected pattern
// of block parquet converter markers stored in the markers location.
func IsBlockParquetConverterMarkFilename(name string) (ulid.ULID, bool) {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return ulid.ULID{}, false
	}

	// Ensure the 2nd part matches the parquet converter mark filename.
	if parts[1] != parquet.ConverterMarkerFileName {
		return ulid.ULID{}, false
	}

	// Ensure the 1st part is a valid block ID.
	id, err := ulid.Parse(filepath.Base(parts[0]))
	return id, err == nil
}

// MigrateBlockDeletionMarksToGlobalLocation list all tenant's blocks and, for each of them, look for
// a deletion mark in the block location. Found deletion marks are copied to the global markers location.
// The migration continues on error and returns once all blocks have been checked.
func MigrateBlockDeletionMarksToGlobalLocation(ctx context.Context, bkt objstore.Bucket, userID string, cfgProvider bucket.TenantConfigProvider) error {
	bucket := bucket.NewUserBucketClient(userID, bkt, cfgProvider)
	userBucket := bucket.WithExpectedErrs(bucket.IsObjNotFoundErr)

	// Find all blocks in the storage.
	var blocks []ulid.ULID
	err := userBucket.Iter(ctx, "", func(name string) error {
		if id, ok := block.IsBlockDir(name); ok {
			blocks = append(blocks, id)
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "list blocks")
	}

	errs := tsdb_errors.NewMulti()

	for _, blockID := range blocks {
		for mark, globalFilePath := range MarkersMap {
			// Look up mark (if any).
			reader, err := userBucket.Get(ctx, path.Join(blockID.String(), mark))
			if userBucket.IsObjNotFoundErr(err) {
				continue
			} else if err != nil {
				errs.Add(err)
				continue
			}

			// Upload it to the global markers location.
			uploadErr := userBucket.Upload(ctx, globalFilePath(blockID), reader)
			if closeErr := reader.Close(); closeErr != nil {
				errs.Add(closeErr)
			}
			if uploadErr != nil {
				errs.Add(uploadErr)
			}
		}
	}

	return errs.Err()
}
