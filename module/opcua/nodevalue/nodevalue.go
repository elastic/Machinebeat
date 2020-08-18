package nodevalue

import (
	"time"

	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/common/cfgwarn"
	"github.com/elastic/beats/v7/metricbeat/mb"

	"context"
	"errors"

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
}

type Browse struct {
	Enabled          bool `config:"enabled"`
	MaxLevel         int  `config:"maxLevel"`
	MaxNodePerParent int  `config:"maxNodePerParent"`
}

type Node struct {
	ID       string `config:"id"`
	Label    string `config:"label"`
	Name     string
	DataType string
}

var browseDefaults = Browse{
	Enabled:          true,
	MaxLevel:         3,
	MaxNodePerParent: 5,
}

var DefaultConfig = MetricSet{
	Endpoint:            "opc.tcp://localhost:4840",
	RetryOnErrorCount:   5,
	MaxThreads:          50,
	Subscribe:           true,
	Policy:              "",
	Mode:                "",
	Username:            "",
	Password:            "",
	ClientCert:          "",
	ClientKey:           "",
	AppName:             "machinebeat",
	Nodes:               []Node{},
	Browse:              browseDefaults,
	MaxTriesToReconnect: 5,
}

var (
	sem     *semaphore.Weighted
	counter int
)

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
	}

	counter = metricset.MaxTriesToReconnect
	newConnection, err := establishConnection(metricset, 1)
	if err != nil {
		return nil, err
	}

	if !newConnection {
		logp.Warn("A new connection attempt was made. This gets ignored from this module")
		return metricset, nil
	}

	//Check if browsing is activated in general.
	//	If yes the collection will be started after browsing
	//	If no the collection will be started with the configured nodes directly
	if metricset.Browse.Enabled {
		logp.Info("Browsing is enabled. Data collection will start after discovery. Based on your server and browsing configuration this can take some time.")

		//Implements the browsing service of OPC UA.
		nodesToCollect = startBrowse()

		logp.Debug("Browse", "Nodes to collect data from")
		for _, nodeConfig := range nodesToCollect {
			logp.Debug("Browse", "Node: %v", nodeConfig.ID)
		}

		logp.Info("Browsing finished")
	} else {
		//If browsing is disabled we will collect directly from the configured nodes
		nodesToCollect = metricset.Nodes
	}

	if len(nodesToCollect) == 0 {
		logp.Info("Found 0 nodes to collect data from.")
	} else {
		if metricset.Subscribe {
			startSubscription()
		} else {
			sem = semaphore.NewWeighted(int64(metricset.MaxThreads))
		}
	}
	return metricset, nil
}

func establishConnection(config *MetricSet, retryCounter int) (bool, error) {
	for i := retryCounter; i > 0; i-- {
		newConnection, err := connect(config)
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
			closeConnection()
		}
	}()

	data, err := collectData()
	if err != nil {
		logp.Info("error: %v", err)
		logp.Error(err)
		closeConnection()
		return err
	}

	publishResponses(data, report, m)
	logp.Debug("Collector", "Event collector instance finished sucessfully.")
	return nil
}

func handleCounter(eventCount int, resetValue int) {
	if eventCount == 0 {
		counter = counter - 1
		if counter == 0 {
			logp.Info("[OPCUA] Too much zero publish attempts.")
			closeConnection()
		}
	} else {
		counter = resetValue
	}
}

func publishResponses(data []*ResponseObject, report mb.ReporterV2, config *MetricSet) {
	logp.Info("[OPCUA] Publishing %v new events", len(data))
	handleCounter(len(data), config.MaxTriesToReconnect)
	for _, response := range data {
		var mbEvent mb.Event
		event := make(common.MapStr)
		module := make(common.MapStr)
		if response.value.Status == 0 {
			event.Put("state", "OK")
		} else {
			event.Put("state", "ERROR")
		}
		event.Put("created", response.value.SourceTimestamp.String())

		if response.value.Value != nil {
			if response.node.DataType != "" {
				event.Put(response.node.DataType, response.value.Value.Value())
			} else {
				event.Put("value", response.value.Value.Value())
			}
		}
		module.Put("node", response.node)
		module.Put("endpoint", config.Endpoint)
		mbEvent.ModuleFields = module
		mbEvent.MetricSetFields = event
		report.Event(mbEvent)
	}
}

// Fetch methods implements the data gathering and data conversion to the right
// format. It publishes the event which is then forwarded to the output. In case
// of an error set the Error field of mb.Event or simply call report.Error().
func (m *MetricSet) Fetch(report mb.ReporterV2) error {
	if connected {
		if m.Subscribe {
			var data []*ResponseObject
			for {
				select {
				case response := <-subscription:
					data = append(data, response)
				default:
					publishResponses(data, report, m)
					return nil
				}
			}
		} else {
			ctx := context.Background()
			if err := sem.Acquire(ctx, 1); err != nil {
				logp.Err("[OPCUA] Max threads reached. This means that it takes too long to get the data from your OPC UA server. You should consider to increase the max Thread counter or the period of getting the data.")
			} else {
				go func() {
					collect(m, report)
					sem.Release(1)
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
	}
	return nil
}
