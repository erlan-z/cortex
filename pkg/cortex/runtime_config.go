package cortex

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/cortexproject/cortex/pkg/ingester"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/runtimeconfig"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

var (
	errMultipleDocuments    = errors.New("the provided runtime configuration contains multiple documents")
	tenantLimitCheckTargets = []string{All, Distributor, Querier, Ruler}
)

// RuntimeConfigValues are values that can be reloaded from configuration file while Cortex is running.
// Reloading is done by runtime_config.Manager, which also keeps the currently loaded config.
// These values are then pushed to the components that are interested in them.
type RuntimeConfigValues struct {
	TenantLimits map[string]*validation.Limits `yaml:"overrides"`

	Multi kv.MultiRuntimeConfig `yaml:"multi_kv_config"`

	IngesterChunkStreaming *bool `yaml:"ingester_stream_chunks_when_using_blocks"`

	IngesterLimits *ingester.InstanceLimits `yaml:"ingester_limits"`
}

// runtimeConfigTenantLimits provides per-tenant limit overrides based on a runtimeconfig.Manager
// that reads limits from a configuration file on disk and periodically reloads them.
type runtimeConfigTenantLimits struct {
	manager *runtimeconfig.Manager
}

// newTenantLimits creates a new validation.TenantLimits that loads per-tenant limit overrides from
// a runtimeconfig.Manager
func newTenantLimits(manager *runtimeconfig.Manager) validation.TenantLimits {
	return &runtimeConfigTenantLimits{
		manager: manager,
	}
}

func (l *runtimeConfigTenantLimits) ByUserID(userID string) *validation.Limits {
	return l.AllByUserID()[userID]
}

func (l *runtimeConfigTenantLimits) AllByUserID() map[string]*validation.Limits {
	cfg, ok := l.manager.GetConfig().(*RuntimeConfigValues)
	if cfg != nil && ok {
		return cfg.TenantLimits
	}

	return nil
}

type runtimeConfigLoader struct {
	cfg Config
}

func (l runtimeConfigLoader) load(r io.Reader) (interface{}, error) {
	var overrides = &RuntimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.SetStrict(true)

	// Decode the first document. An empty document (EOF) is OK.
	if err := decoder.Decode(&overrides); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	// Ensure the provided YAML config is not composed of multiple documents,
	if err := decoder.Decode(&RuntimeConfigValues{}); !errors.Is(err, io.EOF) {
		return nil, errMultipleDocuments
	}

	targetStr := l.cfg.Target.String()
	for _, target := range tenantLimitCheckTargets {
		if strings.Contains(targetStr, target) {
			// only check if target is `all`, `distributor`, "querier", and "ruler"
			// refer to https://github.com/cortexproject/cortex/issues/6741#issuecomment-3067244929
			for _, ul := range overrides.TenantLimits {
				if err := ul.Validate(l.cfg.Distributor.ShardByAllLabels, l.cfg.Ingester.ActiveSeriesMetricsEnabled); err != nil {
					return nil, err
				}
			}
		}
	}

	return overrides, nil
}

func multiClientRuntimeConfigChannel(manager *runtimeconfig.Manager) func() <-chan kv.MultiRuntimeConfig {
	if manager == nil {
		return nil
	}
	// returns function that can be used in MultiConfig.ConfigProvider
	return func() <-chan kv.MultiRuntimeConfig {
		outCh := make(chan kv.MultiRuntimeConfig, 1)

		// push initial config to the channel
		val := manager.GetConfig()
		if cfg, ok := val.(*RuntimeConfigValues); ok && cfg != nil {
			outCh <- cfg.Multi
		}

		ch := manager.CreateListenerChannel(1)
		go func() {
			for val := range ch {
				if cfg, ok := val.(*RuntimeConfigValues); ok && cfg != nil {
					outCh <- cfg.Multi
				}
			}
		}()

		return outCh
	}
}

func ingesterInstanceLimits(manager *runtimeconfig.Manager) func() *ingester.InstanceLimits {
	if manager == nil {
		return nil
	}

	return func() *ingester.InstanceLimits {
		val := manager.GetConfig()
		if cfg, ok := val.(*RuntimeConfigValues); ok && cfg != nil {
			return cfg.IngesterLimits
		}
		return nil
	}
}

func runtimeConfigHandler(runtimeCfgManager *runtimeconfig.Manager, defaultLimits validation.Limits) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, ok := runtimeCfgManager.GetConfig().(*RuntimeConfigValues)
		if !ok || cfg == nil {
			util.WriteTextResponse(w, "runtime config file doesn't exist")
			return
		}

		var output interface{}
		switch r.URL.Query().Get("mode") {
		case "diff":
			// Default runtime config is just empty struct, but to make diff work,
			// we set defaultLimits for every tenant that exists in runtime config.
			defaultCfg := RuntimeConfigValues{}
			defaultCfg.TenantLimits = map[string]*validation.Limits{}
			for k, v := range cfg.TenantLimits {
				if v != nil {
					defaultCfg.TenantLimits[k] = &defaultLimits
				}
			}

			cfgYaml, err := util.YAMLMarshalUnmarshal(cfg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			defaultCfgYaml, err := util.YAMLMarshalUnmarshal(defaultCfg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			output, err = util.DiffConfig(defaultCfgYaml, cfgYaml)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

		default:
			output = cfg
		}
		util.WriteYAMLResponse(w, output)
	}
}
