//go:build requires_docker
// +build requires_docker

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore/providers/s3"
	"gopkg.in/yaml.v3"

	"github.com/cortexproject/cortex/integration/e2e"
	e2edb "github.com/cortexproject/cortex/integration/e2e/db"
	"github.com/cortexproject/cortex/integration/e2ecortex"
	"github.com/cortexproject/cortex/pkg/cortex"
)

func TestLoadRuntimeConfigFromStorageBackend(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, copyFileToSharedDir(s, "docs/configuration/runtime-config.yaml", runtimeConfigFile))
	require.NoError(t, copyFileToSharedDir(s, "docs/configuration/single-process-config-blocks-local.yaml", cortexConfigFile))

	filePath := filepath.Join(e2e.ContainerSharedDir, runtimeConfigFile)
	tests := []struct {
		name    string
		flags   map[string]string
		workDir string
	}{
		{
			name: "no storage backend provided",
			flags: map[string]string{
				"-runtime-config.file": filePath,
				// alert manager
				"-alertmanager.web.external-url":   "http://localhost/alertmanager",
				"-alertmanager-storage.backend":    "local",
				"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
			},
		},
		{
			name: "filesystem as storage backend",
			flags: map[string]string{
				"-runtime-config.file":    filePath,
				"-runtime-config.backend": "filesystem",
				// alert manager
				"-alertmanager.web.external-url":   "http://localhost/alertmanager",
				"-alertmanager-storage.backend":    "local",
				"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
			},
		},
		{
			name: "runtime-config.file is a relative path",
			flags: map[string]string{
				"-runtime-config.file": runtimeConfigFile,
				// alert manager
				"-alertmanager.web.external-url":   "http://localhost/alertmanager",
				"-alertmanager-storage.backend":    "local",
				"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
			},
			workDir: e2e.ContainerSharedDir,
		},
		{
			name: "runtime-config.file is an absolute path but working directory is not /",
			flags: map[string]string{
				"-runtime-config.file": filePath,
				// alert manager
				"-alertmanager.web.external-url":   "http://localhost/alertmanager",
				"-alertmanager-storage.backend":    "local",
				"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
			},
			workDir: "/var/lib/cortex",
		},
	}
	// make alert manager config dir
	require.NoError(t, writeFileToSharedDir(s, "alertmanager_configs", []byte{}))

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cortexSvc := e2ecortex.NewSingleBinaryWithConfigFile(fmt.Sprintf("cortex-%d", i), cortexConfigFile, tt.flags, "", 9009, 9095)
			cortexSvc.SetWorkDir(tt.workDir)

			require.NoError(t, s.StartAndWaitReady(cortexSvc))

			assertRuntimeConfigLoadedCorrectly(t, cortexSvc)

			require.NoError(t, s.Stop(cortexSvc))
		})
	}
}

func TestLoadRuntimeConfigFromCloudStorage(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	configFileName := "runtime-config.yaml"
	bucketName := "cortex"
	flags := map[string]string{
		"-runtime-config.backend":              "s3",
		"-runtime-config.s3.access-key-id":     e2edb.MinioAccessKey,
		"-runtime-config.s3.secret-access-key": e2edb.MinioSecretKey,
		"-runtime-config.s3.bucket-name":       bucketName,
		"-runtime-config.s3.endpoint":          fmt.Sprintf("%s-minio-9000:9000", networkName),
		"-runtime-config.s3.insecure":          "true",
		"-runtime-config.file":                 configFileName,
		"-runtime-config.reload-period":        "2s",
		// alert manager
		"-alertmanager.web.external-url":   "http://localhost/alertmanager",
		"-alertmanager-storage.backend":    "local",
		"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
	}
	// make alert manager config dir
	require.NoError(t, writeFileToSharedDir(s, "alertmanager_configs", []byte{}))

	// create s3 storage backend
	minio := e2edb.NewMinio(9000, bucketName)
	require.NoError(t, s.StartAndWaitReady(minio))

	client, err := s3.NewBucketWithConfig(nil, s3.Config{
		Endpoint:  minio.HTTPEndpoint(),
		Insecure:  true,
		Bucket:    bucketName,
		AccessKey: e2edb.MinioAccessKey,
		SecretKey: e2edb.MinioSecretKey,
	}, "runtime-config-test", nil)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(getCortexProjectDir(), "docs/configuration/runtime-config.yaml"))
	require.NoError(t, err)

	require.NoError(t, client.Upload(context.Background(), configFileName, bytes.NewReader(content)))
	require.NoError(t, copyFileToSharedDir(s, "docs/configuration/single-process-config-blocks-local.yaml", cortexConfigFile))

	// start cortex and assert runtime-config is loaded correctly
	cortexSvc := e2ecortex.NewSingleBinaryWithConfigFile("cortex-svc", cortexConfigFile, flags, "", 9009, 9095)
	require.NoError(t, s.StartAndWaitReady(cortexSvc))
	assertRuntimeConfigLoadedCorrectly(t, cortexSvc)

	// update runtime config
	newRuntimeConfig := []byte(`overrides:
  tenant3:
    ingestion_rate: 30000
    max_exemplars: 3`)
	require.NoError(t, client.Upload(context.Background(), configFileName, bytes.NewReader(newRuntimeConfig)))
	time.Sleep(2 * time.Second)

	runtimeConfig := getRuntimeConfig(t, cortexSvc)
	require.Nil(t, runtimeConfig.TenantLimits["tenant1"])
	require.Nil(t, runtimeConfig.TenantLimits["tenant2"])
	require.NotNil(t, runtimeConfig.TenantLimits["tenant3"])
	require.Equal(t, float64(30000), (*runtimeConfig.TenantLimits["tenant3"]).IngestionRate)
	require.Equal(t, 3, (*runtimeConfig.TenantLimits["tenant3"]).MaxExemplars)

	require.NoError(t, s.Stop(cortexSvc))
}

// Verify components are successfully started with `-distributor.shard-by-all-labels=false`
// except for "distributor", "querier", and "ruler"
// refer to https://github.com/cortexproject/cortex/issues/6741#issuecomment-3067244929
func Test_VerifyComponentsAreSuccessfullyStarted_WithRuntimeConfigLoad(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	runtimeConfigYamlFile := `
overrides:
  'user-1':
    max_global_series_per_user: 15000
`

	require.NoError(t, writeFileToSharedDir(s, runtimeConfigFile, []byte(runtimeConfigYamlFile)))
	filePath := filepath.Join(e2e.ContainerSharedDir, runtimeConfigFile)

	flags := mergeFlags(BlocksStorageFlags(), map[string]string{
		"-runtime-config.file":    filePath,
		"-runtime-config.backend": "filesystem",

		// alert manager
		"-alertmanager.web.external-url":   "http://localhost/alertmanager",
		"-alertmanager-storage.backend":    "local",
		"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),

		// store-gateway
		"-querier.store-gateway-addresses": "localhost:12345",

		// distributor.shard-by-all-labels is false
		"-distributor.shard-by-all-labels": "false",
	})

	// Start dependencies.
	consul := e2edb.NewConsul()
	minio := e2edb.NewMinio(9000, flags["-blocks-storage.s3.bucket-name"])
	require.NoError(t, s.StartAndWaitReady(consul, minio))

	// make alert manager config dir
	require.NoError(t, writeFileToSharedDir(s, "alertmanager_configs", []byte{}))

	// Ingester and Store gateway start
	ingester := e2ecortex.NewIngester("ingester", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), flags, "")
	storeGateway := e2ecortex.NewStoreGateway("store-gateway", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), flags, "")
	require.NoError(t, s.StartAndWaitReady(ingester, storeGateway))

	// Querier start, but fail with "-distributor.shard-by-all-labels": "false"
	querier := e2ecortex.NewQuerier("querier", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), mergeFlags(flags, map[string]string{
		"-querier.store-gateway-addresses": strings.Join([]string{storeGateway.NetworkGRPCEndpoint()}, ","),
	}), "")
	require.Error(t, s.StartAndWaitReady(querier))

	// Start Query frontend
	queryFrontend := e2ecortex.NewQueryFrontendWithConfigFile("query-frontend", "", flags, "")
	require.NoError(t, s.Start(queryFrontend))

	// Querier start, should success with "-distributor.shard-by-all-labels": "true"
	querier = e2ecortex.NewQuerier("querier", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), mergeFlags(flags, map[string]string{
		"-querier.store-gateway-addresses": strings.Join([]string{storeGateway.NetworkGRPCEndpoint()}, ","),
		"-distributor.shard-by-all-labels": "true",
		"-querier.frontend-address":        queryFrontend.NetworkGRPCEndpoint(),
	}), "")
	require.NoError(t, s.StartAndWaitReady(querier))

	// Ruler start, but fail with "-distributor.shard-by-all-labels": "false"
	ruler := e2ecortex.NewRuler("ruler", consul.NetworkHTTPEndpoint(), mergeFlags(flags, RulerFlags()), "")
	require.Error(t, s.StartAndWaitReady(ruler))

	// Ruler start, should success with "-distributor.shard-by-all-labels": "true"
	ruler = e2ecortex.NewRuler("ruler", consul.NetworkHTTPEndpoint(), mergeFlags(flags, RulerFlags(), map[string]string{
		"-distributor.shard-by-all-labels": "true",
	}), "")
	require.NoError(t, s.StartAndWaitReady(ruler))

	// Start the query-scheduler
	queryScheduler := e2ecortex.NewQueryScheduler("query-scheduler", flags, "")
	require.NoError(t, s.StartAndWaitReady(queryScheduler))

	// Start Alertmanager
	alertmanager := e2ecortex.NewAlertmanager("alertmanager", mergeFlags(flags, AlertmanagerFlags()), "")
	require.NoError(t, s.StartAndWaitReady(alertmanager))

	// Distributor start, but fail with "-distributor.shard-by-all-labels": "false"
	distributor := e2ecortex.NewQuerier("distributor", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), mergeFlags(flags, map[string]string{}), "")
	require.Error(t, s.StartAndWaitReady(distributor))

	// Distributor start, should success with "-distributor.shard-by-all-labels": "true"
	distributor = e2ecortex.NewQuerier("distributor", e2ecortex.RingStoreConsul, consul.NetworkHTTPEndpoint(), mergeFlags(flags, map[string]string{
		"-distributor.shard-by-all-labels": "true",
	}), "")
	require.NoError(t, s.StartAndWaitReady(distributor))
}

func assertRuntimeConfigLoadedCorrectly(t *testing.T, cortexSvc *e2ecortex.CortexService) {
	runtimeConfig := getRuntimeConfig(t, cortexSvc)

	require.NotNil(t, runtimeConfig.TenantLimits["tenant1"])
	require.Equal(t, float64(10000), (*runtimeConfig.TenantLimits["tenant1"]).IngestionRate)
	require.Equal(t, 1, (*runtimeConfig.TenantLimits["tenant1"]).MaxExemplars)
	require.NotNil(t, runtimeConfig.TenantLimits["tenant2"])
	require.Equal(t, float64(10000), (*runtimeConfig.TenantLimits["tenant2"]).IngestionRate)
	require.Equal(t, 0, (*runtimeConfig.TenantLimits["tenant2"]).MaxExemplars)
	require.Equal(t, false, *runtimeConfig.Multi.Mirroring)
	require.Equal(t, "memberlist", runtimeConfig.Multi.PrimaryStore)
	require.Equal(t, float64(42000), (*runtimeConfig.IngesterLimits).MaxIngestionRate)
	require.Equal(t, int64(10000), (*runtimeConfig.IngesterLimits).MaxInflightPushRequests)
}

func getRuntimeConfig(t *testing.T, cortexSvc *e2ecortex.CortexService) cortex.RuntimeConfigValues {
	res, err := e2e.GetRequest("http://" + cortexSvc.HTTPEndpoint() + "/runtime_config")
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	runtimeConfig := cortex.RuntimeConfigValues{}
	require.NoError(t, yaml.Unmarshal(body, &runtimeConfig))
	return runtimeConfig
}
