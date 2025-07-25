---
title: "v1.x Guarantees"
linkTitle: "v1.x Guarantees"
weight: 6
slug: v1guarantees
---

For the v1.0 release, we want to provide the following guarantees:

## Flags, Config and minor version upgrades

Upgrading cortex from one minor version to the next should "just work"; that being said, we don't want to bump the major version every time we remove a flag, so we will will keep deprecated flags around for 2 minor release.  There is a metric (`deprecated_flags_inuse_total`) you can alert on to find out if you're using a deprecated  flag.

Similarly to flags, minor version upgrades using config files should "just work".  If we do need to change config, we will keep the old way working for two minor version.  There will be a metric you can alert on for this too.

These guarantees don't apply for [experimental features](#experimental-features).

## Reading old data

The Cortex maintainers commit to ensuring future version of Cortex can read data written by versions up to two years old. In practice we expect to be able to read more, but this is our guarantee.

## API Compatibility

Cortex strives to be 100% API compatible with Prometheus (under `/prometheus/*` and `/api/prom/*`); any deviation from this is considered a bug, except:

- Additional API endpoints for creating, removing and modifying alerts and recording rules.
- Additional API around pushing metrics (under `/api/push`).
- Additional API endpoints for management of Cortex itself, such as the ring.  These APIs are not part of the any compatibility guarantees.

_For more information, please refer to the [limitations](../guides/limitations.md) doc._

## Experimental features

Cortex is an actively developed project and we want to encourage the introduction of new features and capability.  As such, not everything in each release of Cortex is considered "production-ready". We don't provide any backwards compatibility guarantees on these and the config and flags might break.

Currently experimental features are:

- Ruler
  - Evaluate rules to query frontend instead of ingesters (enabled via `-ruler.frontend-address`).
  - When `-ruler.frontend-address` is specified, the response format can be specified (via `-ruler.query-response-format`).
- S3 Server Side Encryption (SSE) using KMS (including per-tenant KMS config overrides).
- Azure blob storage.
- Zone awareness based replication.
- Ruler API (to PUT rules).
- Alertmanager:
  - API (enabled via `-experimental.alertmanager.enable-api`)
  - Sharding of tenants across multiple instances (enabled via `-alertmanager.sharding-enabled`)
  - Receiver integrations firewall (configured via `-alertmanager.receivers-firewall.*`)
- Memcached client DNS-based service discovery.
- In-memory (FIFO) and Redis cache.
- gRPC Store.
- TLS configuration in gRPC and HTTP clients.
- TLS configuration in Etcd client.
- Blocksconvert tools
- OpenStack Swift storage support.
- Metric relabeling in the distributor.
- Scalable query-frontend (when using query-scheduler)
- Ingester: do not unregister from ring on shutdown (`-ingester.unregister-on-shutdown=false`)
- Distributor:
  - Do not extend writes on unhealthy ingesters (`-distributor.extend-writes=false`)
  - Accept multiple HA pairs in the same request (enabled via `-experimental.distributor.ha-tracker.mixed-ha-samples=true`)
- Tenant Deletion in Purger, for blocks storage.
- Query-frontend: query stats tracking (`-frontend.query-stats-enabled`)
- Blocks storage bucket index
  - The bucket index support in the querier and store-gateway (enabled via `-blocks-storage.bucket-store.bucket-index.enabled=true`) is experimental
  - The block deletion marks migration support in the compactor (`-compactor.block-deletion-marks-migration-enabled`) is temporarily and will be removed in future versions
- Blocks storage user index
- Querier: tenant federation
  - `-tenant-federation.enabled`
  - `-tenant-federation.regex-matcher-enabled`
  - `-tenant-federation.user-sync-interval`
- The thanosconvert tool for converting Thanos block metadata to Cortex
- HA Tracker: cleanup of old replicas from KV Store.
- Instance limits in ingester and distributor
- Exemplar storage, currently in-memory only within the Ingester based on Prometheus exemplar storage (`-blocks-storage.tsdb.max-exemplars`)
- Querier limits:
  - `-querier.max-fetched-chunks-per-query`
  - `-querier.max-fetched-chunk-bytes-per-query`
  - `-querier.max-fetched-series-per-query`
- Alertmanager limits
  - notification rate (`-alertmanager.notification-rate-limit` and `-alertmanager.notification-rate-limit-per-integration`)
  - dispatcher groups (`-alertmanager.max-dispatcher-aggregation-groups`)
  - user config size (`-alertmanager.max-config-size-bytes`)
  - templates count in user config (`-alertmanager.max-templates-count`)
  - max template size (`-alertmanager.max-template-size-bytes`)
- Disabling ring heartbeat timeouts
  - `-distributor.ring.heartbeat-timeout=0`
  - `-ring.heartbeat-timeout=0`
  - `-ruler.ring.heartbeat-timeout=0`
  - `-alertmanager.sharding-ring.heartbeat-timeout=0`
  - `-compactor.ring.heartbeat-timeout=0`
  - `-store-gateway.sharding-ring.heartbeat-timeout=0`
- Disabling ring heartbeats
  - `-distributor.ring.heartbeat-period=0`
  - `-ingester.heartbeat-period=0`
  - `-ruler.ring.heartbeat-period=0`
  - `-alertmanager.sharding-ring.heartbeat-period=0`
  - `-compactor.ring.heartbeat-period=0`
  - `-store-gateway.sharding-ring.heartbeat-period=0`
- Compactor shuffle sharding
  - Enabled via `-compactor.sharding-enabled=true`, `-compactor.sharding-strategy=shuffle-sharding`, and `-compactor.tenant-shard-size` set to a value larger than 0.
- Vertical sharding at query frontend for range/instant queries
  - `-frontend.query-vertical-shard-size` (int) CLI flag
  - `query_vertical_shard_size` (int) field in runtime config file
- Snapshotting of in-memory TSDB on disk during shutdown
  - `-blocks-storage.tsdb.memory-snapshot-on-shutdown` (boolean) CLI flag
- Out of order samples support
  - `-blocks-storage.tsdb.out-of-order-cap-max` (int) CLI flag
  - `-ingester.out-of-order-time-window` (duration) CLI flag
  - `out_of_order_time_window` (duration) field in runtime config file
- Store Gateway Zone Stable Shuffle Sharding
  - `-store-gateway.sharding-ring.zone-stable-shuffle-sharding` CLI flag
  - `zone_stable_shuffle_sharding` (boolean) field in config file
- Basic Lifecycler (Storegateway, Alertmanager, Ruler) Final Sleep on shutdown, which tells the pod wait before shutdown, allowing a delay to propagate ring changes.
  - `-ruler.ring.final-sleep` (duration) CLI flag
  - `store-gateway.sharding-ring.final-sleep` (duration) CLI flag
  - `alertmanager-sharding-ring.final-sleep` (duration) CLI flag
- OTLP Receiver
- Persistent tokens in the Ruler Ring:
  - `-ruler.ring.tokens-file-path` (path) CLI flag
- Native Histograms
  - Ingestion can be enabled by setting `-blocks-storage.tsdb.enable-native-histograms=true` on Ingester.
- String interning for metrics labels
  - Enable string interning for metrics labels by setting `-ingester.labels-string-interning-enabled` on Ingester.
- Query-frontend: query rejection (`-frontend.query-rejection.enabled`)
- Querier: protobuf codec (`-api.querier-default-codec`)
- Query-frontend: dynamic query splits
  - `querier.max-shards-per-query` (int) CLI flag
  - `querier.max-fetched-data-duration-per-query` (duration) CLI flag
- Ingester/Store-Gateway: Query rejection
  - `-ingester.query-protection.rejection`
  - `-store-gateway.query-protection.rejection`
- Distributor/Ingester: Stream push connection
  - Enable stream push connection between distributor and ingester by setting `-distributor.use-stream-push=true` on Distributor.
