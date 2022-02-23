package nodevalue

import (
	"time"

	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/common/cfgwarn"
	"github.com/elastic/beats/v7/metricbeat/mb"

	"context"
	"errors"
	_ "fmt"
	"math"
	"reflect"

	"golang.org/x/sync/semaphore"
)

// init registers the MetricSet with the central registry as soon as the program
// starts. The New function will be called later to instantiate an instance of
// the MetricSet for each host defined in the module's configuration. After the
// MetricSet has been created then Fetch will begin to be called periodically.
func init() {
	mb.Registry.MustAddMetricSet("opcua", "nodevalue", New)
}

// MetricSet holds any configuration or state information. It must implement
// the mb.MetricSet interface. And this is best achieved by embedding
// mb.BaseMetricSet because it implements all of the required mb.MetricSet
// interface methods except for Fetch.
type MetricSet struct {
	mb.BaseMetricSet
	Endpoint            string `config:"endpoint"`
	Nodes               []Node `config:"nodes"`
	Browse              Browse `config:"browse"`
	RetryOnErrorCount   int    `config:"retryOnError"`
	MaxThreads          int    `config:"maxThreads"`
	MaxTriesToReconnect int    `config:"maxTriesToReconnect"`
	Subscribe           bool   `config:"subscribe"`
	Username            string `config:"username"`
	Password            string `config:"password"`
	Policy              string `config:"policy"`
	Mode                string `config:"securityMode"`
	ClientCert          string `config:"clientCert"`
	ClientKey           string `config:"clientKey"`
	AppName             string `config:"appName"`
	Client              Client
	LegacyFields        bool `config:"legacyFields"`
	ECSFields           bool `config:"ECSFields"`
}

type Browse struct {
	Enabled          bool `config:"enabled"`
	MaxLevel         int  `config:"maxLevel"`
	MaxNodePerParent int  `config:"maxNodePerParent"`
}

var browseDefaults = Browse{
	Enabled:          true,
	MaxLevel:         0,
	MaxNodePerParent: 0,
}

var clientDefaults = Client{
	connected: false,
}

var DefaultConfig = MetricSet{
	Endpoint:            "opc.tcp://localhost:4840",
	RetryOnErrorCount:   5,
	MaxThreads:          50,
	Subscribe:           true,
	Policy:              "None",
	Mode:                "None",
	Username:            "",
	Password:            "",
	ClientCert:          "",
	ClientKey:           "",
	AppName:             "machinebeat",
	Nodes:               []Node{},
	Browse:              browseDefaults,
	MaxTriesToReconnect: 5,
	Client:              clientDefaults,
	LegacyFields:        false,
	ECSFields:           true,
}

// New creates a new instance of the MetricSet. New is responsible for unpacking
// any MetricSet specific configuration options if there are any.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	cfgwarn.Beta("The OPCUA metricset is beta.")

	config := DefaultConfig
	if err := base.Module().UnpackConfig(&config); err != nil {
		return nil, err
	}

	metricset := &MetricSet{
		BaseMetricSet:       base,
		Endpoint:            config.Endpoint,
		RetryOnErrorCount:   config.RetryOnErrorCount,
		MaxThreads:          config.MaxThreads,
		Subscribe:           config.Subscribe,
		Username:            config.Username,
		Password:            config.Password,
		Policy:              config.Policy,
		Mode:                config.Mode,
		ClientCert:          config.ClientCert,
		ClientKey:           config.ClientKey,
		AppName:             config.AppName,
		Nodes:               config.Nodes,
		Browse:              config.Browse,
		MaxTriesToReconnect: config.MaxTriesToReconnect,
		Client:              config.Client,
		LegacyFields:        config.LegacyFields,
		ECSFields:           config.ECSFields,
	}

	metricset.Client.counter = metricset.MaxTriesToReconnect
	metricset.Client.config = metricset

	_, err := establishConnection(metricset, 1)
	if err != nil {
		return nil, err
	}

	//Check if browsing is activated in general.
	//	If yes the collection will be started after browsing
	//	If no the collection will be started with the configured nodes directly
	if metricset.Browse.Enabled {
		logp.Info("Browsing is enabled. Data collection will start after discovery. Based on your server and browsing configuration this can take some time.")

		//Implements the browsing service of OPC UA.
		metricset.Client.startBrowse()

		logp.Debug("Browse", "Nodes to collect data from")
		for _, nodeConfig := range metricset.Client.nodesToCollect {
			logp.Debug("Browse", "Node: %v", nodeConfig.ID)
		}

		logp.Info("Browsing finished")
	} else {
		//If browsing is disabled we will collect directly from the configured nodes
		for i := range metricset.Nodes {
			metricset.Client.nodesToCollect = append(metricset.Client.nodesToCollect, &metricset.Nodes[i])
		}
		err := metricset.Client.appendNodeInformation()
		if err != nil {
			return nil, err
		}
	}

	if len(metricset.Client.nodesToCollect) == 0 {
		logp.Info("Found 0 nodes to collect data from.")
	} else {
		if metricset.Subscribe {
			metricset.Client.startSubscription()
		} else {
			metricset.Client.sem = semaphore.NewWeighted(int64(metricset.MaxThreads))
		}
	}
	return metricset, nil
}

func establishConnection(config *MetricSet, retryCounter int) (bool, error) {
	for i := retryCounter; i > 0; i-- {
		newConnection, err := config.Client.connect()
		if err == nil {
			return newConnection, err
		}
		logp.Error(err)
		time.Sleep(1 * time.Second)
	}
	logp.Critical("[OPCUA] Tried to connect to OPC UA server %v time(s). Without success.", retryCounter)
	return false, errors.New("Connection was not possible")
}

func collect(m *MetricSet, report mb.ReporterV2) error {
	logp.Debug("Collector", "Event collector instance started")

	defer func() {
		if r := recover(); r != nil {
			logp.Info("Recovered from panic. The beat will reconnect now")
			logp.Info("Panic message: %v", r)
			m.Client.closeConnection()
		}
	}()

	data, err := m.Client.collectData()
	if err != nil {
		logp.Info("error: %v", err)
		logp.Error(err)
		m.Client.closeConnection()
		return err
	}

	publishResponses(data, report, m)
	logp.Debug("Collector", "Event collector instance finished sucessfully.")
	return nil
}

func handleCounter(eventCount int, resetValue int, config *MetricSet) {
	if eventCount == 0 {
		config.Client.counter = config.Client.counter - 1
		if config.Client.counter == 0 {
			logp.Info("[OPCUA] Too much zero publish attempts.")
			config.Client.closeConnection()
		}
	} else {
		config.Client.counter = resetValue
	}
}

func publishResponses(data []*ResponseObject, report mb.ReporterV2, config *MetricSet) {
	logp.Info("[OPCUA] Publishing %v new events", len(data))
	handleCounter(len(data), config.MaxTriesToReconnect, config)
	for _, response := range data {
		var mbEvent mb.Event
		event := make(common.MapStr)
		module := make(common.MapStr)
		root := make(common.MapStr)

		//Publish the event with the legacy field schema
		if config.LegacyFields {
			if response.value.Status == 0 {
				event.Put("state", "OK")
			} else {
				event.Put("state", "ERROR")
			}
			event.Put("created", response.value.SourceTimestamp.String())

			if response.value.Value != nil {
				if response.node.DataType != "" {
					if response.node.DataType == "float64" {
						if !isArray(response.value.Value.Value()) {
							if !math.IsNaN(response.value.Value.Value().(float64)) {
								root.Put(response.node.DataType, response.value.Value.Value())
							}
						}
					} else {
						root.Put(response.node.DataType, response.value.Value.Value())
					}
				} else {
					event.Put("value", response.value.Value.Value())
				}
			}
			module.Put("node", response.node)
			module.Put("endpoint", config.Endpoint)

		}

		//Publish the event with ECS field schema
		if config.ECSFields {
			root.Put("event.provider", "opcua")
			root.Put("event.url", config.Endpoint)
			root.Put("event.creation", time.Now())
			root.Put("event.dataset", response.node.Path)

			root.Put("sensor.id", response.node.ID)
			root.Put("sensor.name", response.node.Name)
			root.Put("sensor.label", response.node.Label)

			root.Put("value.source_timestamp", response.value.SourceTimestamp.String())
			if response.value.Value != nil {
				if response.node.DataType != "" {
					root.Put("value.datatype", response.node.DataType)
					if response.node.DataType == "float64" {
						if !isArray(response.value.Value.Value()) {
							if !math.IsNaN(response.value.Value.Value().(float64)) {
								root.Put("value.value_"+response.node.DataType, response.value.Value.Value())
							}
						}
					} else {
						root.Put("value.value_"+response.node.DataType, response.value.Value.Value())
					}

				} else {
					root.Put("value.value", response.value.Value.Value())
				}
			}
		}

		mbEvent.RootFields = root
		mbEvent.ModuleFields = module
		mbEvent.MetricSetFields = event
		report.Event(mbEvent)
	}
}

// Fetch methods implements the data gathering and data conversion to the right
// format. It publishes the event which is then forwarded to the output. In case
// of an error set the Error field of mb.Event or simply call report.Error().
func (m *MetricSet) Fetch(report mb.ReporterV2) error {
	if m.Client.connected {
		if m.Subscribe {
			var data []*ResponseObject
			for {
				select {
				case response := <-m.Client.subscription:
					data = append(data, response)
				default:
					publishResponses(data, report, m)
					return nil
				}
			}
		} else {
			ctx := context.Background()
			if err := m.Client.sem.Acquire(ctx, 1); err != nil {
				logp.Err("[OPCUA] Max threads reached. This means that it takes too long to get the data from your OPC UA server. You should consider to increase the max Thread counter or the period of getting the data.")
			} else {
				go func() {
					collect(m, report)
					m.Client.sem.Release(1)
				}()
			}
		}
	} else {
		//It seems that there was an error, we will try to reconnect
		logp.Info("[OPCUA] Lets wait a while before reconnect happens")
		time.Sleep(5 * time.Second)
		_, err := establishConnection(m, m.RetryOnErrorCount)
		if err != nil {
			logp.Info("[OPCUA] Reconnect was not successful")
			return err
		}
		if m.Subscribe {
			m.Client.startSubscription()
		}
	}
	return nil
}

func isArray(v interface{}) bool {

	rt := reflect.TypeOf(v)
	switch rt.Kind() {
	case reflect.Slice:
		//fmt.Println(v, "is a slice with element type", rt.Elem())
		return true
	case reflect.Array:
		//fmt.Println(v, "is an array with element type", rt.Elem())
		return true
	default:
		//fmt.Println(v, "is something else entirely")
		return false
	}
	return false
}
