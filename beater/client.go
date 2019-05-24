package beater

import (
	"github.com/elastic/beats/libbeat/logp"
	"github.com/felix-lessoer/machinebeat/config"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

var (
	client    *opcua.Client
	endpoint  string
	connected = false
)

type ResponseObject struct {
	node  config.Node
	value *ua.DataValue
}

func connect(endpointURL string) error {
	var err error
	endpoint = endpointURL
	if !connected {
		logp.Info("Connecting to %v", endpoint)
		client = opcua.NewClient(endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))
		if err := client.Connect(); err != nil {
			return err
		}
		connected = true
		logp.Info("Connection established")
	}
	return err
}

func collectData(nodeCollection []config.Node) ([]ResponseObject, error) {

	var retVal []ResponseObject
	var nodesToRead []*ua.ReadValueID
	var readValueID *ua.ReadValueID

	logp.Debug("Collect", "Building the request")
	for _, nodeConfig := range nodeCollection {
		logp.Debug("Collect", "Collecting data from Node %v (NS = %v)", nodeConfig.ID, nodeConfig.Namespace)
		readValueID = new(ua.ReadValueID)
		switch v := nodeConfig.ID.(type) {
		case int:
			nodeID := *ua.NewNumericNodeID(nodeConfig.Namespace, nodeConfig.ID.(uint32))
			readValueID.NodeID = &nodeID
		case string:
			nodeID := *ua.NewStringNodeID(nodeConfig.Namespace, nodeConfig.ID.(string))
			readValueID.NodeID = &nodeID
		default:
			logp.Warn("Configured node id %v has not a valid type. int and string is allowed. %v provided. ID will be ignored", nodeConfig.ID, v)
			continue
		}

		nodesToRead = append(nodesToRead, readValueID)

	}

	req := &ua.ReadRequest{
		MaxAge:             2000,
		NodesToRead:        nodesToRead,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}

	logp.Debug("Collect", "Sending request")
	m, err := client.Read(req)
	if err != nil {
		return retVal, err
	}

	logp.Debug("Collect", "Evaluating response")

	for index, node := range nodeCollection {
		var response ResponseObject
		response.node = node
		response.value = m.Results[index]
		retVal = append(retVal, response)
	}

	logp.Debug("Collect", "Data collection done")

	return retVal, nil
}

func closeConnection() {
	client.Close()
	logp.Debug("Collect", "Successfully shutdown connection")
	connected = false
}
