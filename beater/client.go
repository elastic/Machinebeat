package beater

import (
	"errors"

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

func connect(endpointURL string) error {
	var err error
	endpoint = endpointURL
	if !connected {
		logp.Info("Connecting to %v", endpoint)
		client = opcua.NewClient(endpoint, nil)
		if err := client.Open(); err != nil {
			return err
		}
		connected = true
		logp.Info("Connection established")
	}
	return err
}

func collectData(nodeConfig config.Node) (map[string]interface{}, error) {
	logp.Debug("Collect", "Collecting data from Node %v (NS = %v)", nodeConfig.ID, nodeConfig.Namespace)

	var retVal = make(map[string]interface{})
	var nodeID *ua.NodeID
	var node *opcua.Node

	switch v := nodeConfig.ID.(type) {
	case int:
		nodeID = ua.NewNumericNodeID(nodeConfig.Namespace, nodeConfig.ID.(uint32))
	case string:
		nodeID = ua.NewStringNodeID(nodeConfig.Namespace, nodeConfig.ID.(string))
	default:
		logp.Debug("Collect", "Configured node id %v has not a valid type. int and string is allowed. %v provided", node.ID, v)
	}
	node = client.Node(nodeID)

	rh := ua.RequestHeader{}

	rv := ua.ReadValueID{}

	rv.NodeID = nodeID
	rv.AttributeID = 0
	rv.IndexRange = ""

	qn, err := node.BrowseName()
	if err != nil {
		return nil, err
	}
	rv.DataEncoding = qn

	rr := ua.ReadRequest{}
	rr.MaxAge = 2000
	rr.NodesToRead = []*ua.ReadValueID{&rv}
	rr.TimestampsToReturn = ua.TimestampsToReturnBoth
	rr.RequestHeader = &rh

	m, err := client.Read(&rr)
	if err != nil {
		return nil, err
	}
	value, status := handleReadResponse(m)
	if value == nil {
		return nil, errors.New("It looks like there was an error while getting the last chunk of data. Let's try to reconnect.")
	}
	retVal["Node"] = node.ID
	retVal["Value"] = value.Value
	retVal["Status"] = status
	retVal["Value_Timestamp"] = m.ResponseHeader.Timestamp
	logp.Debug("Collect", "Data collection done")

	return retVal, nil
}

func handleReadResponse(resp *ua.ReadResponse) (value *ua.Variant, status uint32) {
	//TODO: Return array of values not only the first one
	//logp.Info("Results: %v", resp.ResponseHeader)
	for _, r := range resp.Results {
		logp.Info("Result: %v", r)
		logp.Info("Value: %v", r.Value)
		return r.Value, r.Status
	}
	return nil, 0
}

func closeConnection() {
	client.Close()
	logp.Debug("Collect", "Successfully shutdown connection")
	connected = false
}
