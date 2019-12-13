package nodevalue

import (
	"strconv"
	"time"

	"github.com/elastic/beats/libbeat/logp"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/common/cfgwarn"
	"github.com/elastic/beats/metricbeat/mb"

	"context"

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
	Endpoint          string `config:"endpoint"`
	Nodes             []Node `config:"nodes"`
	RetryOnErrorCount int    `config:"retryOnError"`
	MaxThreads        int    `config:"maxThreads"`
	Subscribe         bool   `config:"subscribe"`
	Username          string `config:"username"`
	Password          string `config:"password"`
	Policy            string `config:"policy"`
	Mode              string `config:"securityMode"`
	ClientCert        string `config:"clientCert"`
	ClientKey         string `config:"clientKey"`
}

type Node struct {
	Namespace uint16      `config:"ns"`
	ID        interface{} `config:"id"`
	Label     string      `config:"label"`
}

var DefaultConfig = MetricSet{
	Endpoint:          "opc.tcp://localhost:4840",
	RetryOnErrorCount: 5,
	MaxThreads:        50,
	Subscribe:         true,
	Policy:            "",
	Mode:              "",
	Username:          "",
	Password:          "",
	ClientCert:        "",
	ClientKey:         "",
}

var (
	sem *semaphore.Weighted
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
		BaseMetricSet:     base,
		Endpoint:          config.Endpoint,
		Nodes:             config.Nodes,
		RetryOnErrorCount: config.RetryOnErrorCount,
		MaxThreads:        config.MaxThreads,
		Subscribe:         config.Subscribe,
		Username:          config.Username,
		Password:          config.Password,
		Policy:            config.Policy,
		Mode:              config.Mode,
		ClientCert:        config.ClientCert,
		ClientKey:         config.ClientKey,
	}

	err := establishConnection(*metricset, 1)
	if err != nil {
		return nil, err
	}

	//Implements the browsing service of OPC UA. Currently not working well
	//startBrowse(metricset)

	if metricset.Subscribe {
		startSubscription(metricset.Nodes)
	} else {
		sem = semaphore.NewWeighted(int64(metricset.MaxThreads))
	}

	return metricset, nil
}

func establishConnection(config MetricSet, retryCounter int) error {
	var err error
	for i := retryCounter; i > 0; i-- {
		err = connect(config)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	logp.Critical("Tried to connect to OPC UA server %v time(s). Without success.", retryCounter)
	return err
}

func collect(m *MetricSet, report mb.ReporterV2) error {
	logp.Debug("Collector", "Event collector instance started")

	data, err := collectData(m.Nodes)
	if err != nil {
		logp.Info("error: %v", err)
		logp.Error(err)
		connected = false
		return err
	}
	publishResponses(data, report, m)
	logp.Debug("Collector", "Event collector instance finished sucessfully.")
	return nil
}

func publishResponses(data []*ResponseObject, report mb.ReporterV2, config *MetricSet) {
	for _, response := range data {
		var mbEvent mb.Event
		event := make(common.MapStr)
		if response.value.Status == 0 {
			event.Put("state", "OK")
		} else {
			event.Put("state", "ERROR")
		}
		event.Put("created", response.value.SourceTimestamp.String())
		event.Put("value", response.value.Value.Value())

		//Map the configured label if its not set already
		if response.node.Label == "" {
			for _, nodeConfig := range config.Nodes {
				if response.node.ID == "ns="+strconv.Itoa(int(nodeConfig.Namespace))+";s="+nodeConfig.ID.(string) {
					response.node.Label = nodeConfig.Label
				}
			}
		}
		event.Put("node", response.node.Label)
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
				logp.Err("Max threads reached. This means that it takes too long to get the data from your OPC UA server. You should consider to increase the max Thread counter or the period of getting the data.")
			} else {
				go func() {
					collect(m, report)
					sem.Release(1)
				}()
			}
		}
	} else {
		//It seems that there was an error, we will try to reconnect
		logp.Info("Lets wait a while before reconnect happens")
		time.Sleep(5 * time.Second)
		err := establishConnection(*m, m.RetryOnErrorCount)
		if err != nil {
			logp.Info("Reconnect was not successful")
			return err
		}
	}

	return nil
}
