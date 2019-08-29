package topic

import (
	"github.com/elastic/beats/libbeat/common/cfgwarn"
	_ "github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/metricbeat/mb"
)

// init registers the MetricSet with the central registry as soon as the program
// starts. The New function will be called later to instantiate an instance of
// the MetricSet for each host defined in the module's configuration. After the
// MetricSet has been created then Fetch will begin to be called periodically.
func init() {
	mb.Registry.MustAddMetricSet("mqtt", "topic", New)
}

// MetricSet holds any configuration or state information. It must implement
// the mb.MetricSet interface. And this is best achieved by embedding
// mb.BaseMetricSet because it implements all of the required mb.MetricSet
// interface methods except for Fetch.
type MetricSet struct {
	mb.BaseMetricSet
	BrokerURL       string   `config:"host"`
	BrokerUsername  string   `config:"user"`
	BrokerPassword  string   `config:"password"`
	TopicsSubscribe []string `config:"topics"`
	QoS             int      `config:"QoS"`
	DecodePaylod    bool     `config:"decode_payload"`
}

var (
	DefaultConfig = MetricSet{
		BrokerURL:       "localhost",
		BrokerUsername:  "",
		BrokerPassword:  "",
		TopicsSubscribe: []string{"#"},
		DecodePaylod:    true,
		QoS:             0,
	}
)

// New creates a new instance of the MetricSet. New is responsible for unpacking
// any MetricSet specific configuration options if there are any.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	cfgwarn.Beta("The MQTT metricset is beta.")

	config := DefaultConfig
	if err := base.Module().UnpackConfig(&config); err != nil {
		return nil, err
	}

	metricset := &MetricSet{
		BaseMetricSet:   base,
		BrokerURL:       config.BrokerURL,
		BrokerUsername:  config.BrokerUsername,
		BrokerPassword:  config.BrokerPassword,
		TopicsSubscribe: config.TopicsSubscribe,
		DecodePaylod:    config.DecodePaylod,
		QoS:             config.QoS,
	}

	setupMqttClient(metricset)

	return metricset, nil
}

// Fetch methods implements the data gathering and data conversion to the right
// format. It publishes the event which is then forwarded to the output. In case
// of an error set the Error field of mb.Event or simply call report.Error().
func (m *MetricSet) Fetch(report mb.ReporterV2) error {
	// we are working in a subscriber mode
	// we send the all collected data after the configured timeframe
	for {
		select {
		case event := <-events:
			report.Event(event)
		default:
			return nil
		}
	}
	return nil
}
