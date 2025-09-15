package plc4xvalue

import (
	"time"

	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/elastic/beats/v7/libbeat/common/cfgwarn"
	"github.com/elastic/beats/v7/metricbeat/mb"

	"errors"
	_ "fmt"

	"github.com/elastic/beats/v7/libbeat/common"
)

// init registers the MetricSet with the central registry as soon as the program
// starts. The New function will be called later to instantiate an instance of
// the MetricSet for each host defined in the module's configuration. After the
// MetricSet has been created then Fetch will begin to be called periodically.
func init() {
	mb.Registry.MustAddMetricSet("plc4x", "value", New)
}

// MetricSet holds any configuration or state information. It must implement
// the mb.MetricSet interface. And this is best achieved by embedding
// mb.BaseMetricSet because it implements all of the required mb.MetricSet
// interface methods except for Fetch.
type MetricSet struct {
	mb.BaseMetricSet
	Endpoint            string `config:"endpoint"`
	Client              Client
	MaxTriesToReconnect int    `config:"maxTriesToReconnect"`
	RetryOnErrorCount   int    `config:"retryOnError"`
	Nodes               []Node `config:"nodes"`
}

var clientDefaults = Client{
	connected: false,
}

var DefaultConfig = MetricSet{
	Endpoint:            "modbus-tcp://178.128.239.15",
	Client:              clientDefaults,
	MaxTriesToReconnect: 5,
	RetryOnErrorCount:   5,
	Nodes:               []Node{},
}

// New creates a new instance of the MetricSet. New is responsible for unpacking
// any MetricSet specific configuration options if there are any.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	cfgwarn.Beta("The PLC4X metricset is experimental.")

	config := DefaultConfig
	if err := base.Module().UnpackConfig(&config); err != nil {
		return nil, err
	}

	metricset := &MetricSet{
		BaseMetricSet:       base,
		Endpoint:            config.Endpoint,
		Client:              config.Client,
		RetryOnErrorCount:   config.RetryOnErrorCount,
		MaxTriesToReconnect: config.MaxTriesToReconnect,
		Nodes:               config.Nodes,
	}

	metricset.Client.counter = metricset.MaxTriesToReconnect
	metricset.Client.config = metricset

	_, err := establishConnection(metricset, 1)
	if err != nil {
		return nil, err
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
	logp.Critical("[PLC4X] Tried to connect to endpoint %v time(s). Without success.", retryCounter)
	return false, errors.New("Connection was not possible")
}

func publishResponses(data []*ResponseObject, report mb.ReporterV2, config *MetricSet) {
	logp.Info("[PLC4X] Publishing %v new events", len(data))
	for _, response := range data {
		var mbEvent mb.Event
		root := make(common.MapStr)

		root.Put("event.provider", "plc4x")
		root.Put("event.url", config.Endpoint)
		root.Put("event.creation", time.Now())
		root.Put("event.dataset", response.node.ID)

		root.Put("sensor.id", response.node.ID)

		event := make(common.MapStr)
		event.Put("type", response.value.GetPlcValueType().String())
		event.Put("value", response.value.GetString())

		mbEvent.RootFields = root
		mbEvent.MetricSetFields = event
		report.Event(mbEvent)
	}
}

// Fetch methods implements the data gathering and data conversion to the right
// format. It publishes the event which is then forwarded to the output. In case
// of an error set the Error field of mb.Event or simply call report.Error().
func (m *MetricSet) Fetch(report mb.ReporterV2) error {
	if m.Client.connected {
		resp, err := m.Client.read()
		if err != nil {
			logp.Info("[PLC4X] Data Collection failed")
			return err
		}
		publishResponses(resp, report, m)

	} else {
		//It seems that there was an error, we will try to reconnect
		logp.Info("[PLC4X] Lets wait a while before reconnect happens")
		time.Sleep(5 * time.Second)
		_, err := establishConnection(m, m.RetryOnErrorCount)
		if err != nil {
			logp.Info("[PLC4X] Reconnect was not successful")
			return err
		}
	}
	return nil
}
