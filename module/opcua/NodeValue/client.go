package nodevalue

import (
	"github.com/elastic/beats/libbeat/logp"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"

	"context"
	"log"
	"strconv"
	"time"
)

var (
	client       *opcua.Client
	subscription chan *ResponseObject
	endpoint     string
	connected    = false
)

type ResponseObject struct {
	node  Node
	value *ua.DataValue
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

func connect(endpointURL string) error {
	var err error
	endpoint = endpointURL
	if !connected {
		logp.Info("Connecting to %v", endpoint)
		ctx := context.Background()
		client = opcua.NewClient(endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))
		if err := client.Connect(ctx); err != nil {
			return err
		}
		connected = true
		logp.Info("Connection established")
	}
	return err
}

func browse(nodeCollection []Node) ([]string, error) {
	logp.Info("Start browsing")

	var nodeList []string

	for _, nodeConfig := range nodeCollection {
		nodeID, _ := getNodeID(nodeConfig.Namespace, nodeConfig.ID)
		node := client.Node(nodeID)

		nodeList, err := doBrowse(node, "", 0)
		if err != nil {
			logp.Error(err)
			return nil, err
		}
		for _, s := range nodeList {
			logp.Info(s)
		}
	}
	return nodeList, nil
}

func doBrowse(n *opcua.Node, path string, level int) ([]string, error) {
	if level > 10 {
		return nil, nil
	}

	logp.Info("Extract browseName")
	browseName, err := n.BrowseName()
	if err != nil {
		logp.Info("Error at browsing: %v", err)
		logp.Error(err)
		return nil, err
	}
	path = join(path, browseName.Name)

	logp.Info("Browsing %v under %v", browseName.Name, path)

	typeDefs := ua.NewTwoByteNodeID(id.HasTypeDefinition)
	refs, err := n.References(typeDefs)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs.Results {
		for _, refDesc := range ref.References {
			logp.Info("New node detected: %v", refDesc.DisplayName.Text)
			if refDesc.NodeClass != ua.NodeClassVariable {
				doBrowse(client.Node(refDesc.ReferenceTypeID), path, level+1)
			}
		}
	}
	return nil, nil
}

func getNodeID(ns uint16, id interface{}) (*ua.NodeID, *ua.ReadValueID) {
	var readValueID *ua.ReadValueID
	readValueID = new(ua.ReadValueID)

	convId, ok := id.(uint32)
	if ok {
		id = convId
	}
	nodeID, err := ua.ParseNodeID(id.(string))
	if err != nil {
		logp.Error(err)
	} else {
		readValueID.NodeID = nodeID
		return nodeID, readValueID
	}

	switch v := id.(type) {
	case int:
		nodeID := *ua.NewNumericNodeID(ns, id.(uint32))
		readValueID.NodeID = &nodeID
		return &nodeID, readValueID
	case string:
		nodeID := *ua.NewStringNodeID(ns, id.(string))
		readValueID.NodeID = &nodeID
		return &nodeID, readValueID
	default:
		logp.Warn("Configured node id %v has not a valid type. int and string is allowed. %v provided. ID will be ignored", id, v)
	}

	return nil, nil
}

func collectData(nodeCollection []Node) ([]*ResponseObject, error) {

	var retVal []*ResponseObject
	var nodesToRead []*ua.ReadValueID

	logp.Debug("Collect", "Building the request")
	for _, nodeConfig := range nodeCollection {
		logp.Debug("Collect", "Collecting data from Node %v (NS = %v)", nodeConfig.ID, nodeConfig.Namespace)
		_, readValueID := getNodeID(nodeConfig.Namespace, nodeConfig.ID)
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
		var response *ResponseObject
		response.node = node
		response.value = m.Results[index]
		retVal = append(retVal, response)
	}

	logp.Debug("Collect", "Data collection done")

	return retVal, nil
}

func startSubscription(nodeCollection []Node) {
	logp.Info("Starting subscribe process")
	ctx := context.Background()

	m, err := monitor.NewNodeMonitor(client)
	if err != nil {
		log.Fatal(err)
	}

	m.SetErrorHandler(func(_ *opcua.Client, sub *monitor.Subscription, err error) {
		logp.Warn("Error on monitoring channel: sub=%d err=%s", sub.SubscriptionID(), err.Error())
	})

	// start channel-based subscription
	var nodes []string
	for _, nodeConfig := range nodeCollection {
		nodes = append(nodes, "ns="+strconv.Itoa(int(nodeConfig.Namespace))+";s="+nodeConfig.ID.(string))
	}
	go startChanSub(ctx, m, 0, nodes...)

	logp.Info("Finished subscribe process")
}

func startChanSub(ctx context.Context, m *monitor.NodeMonitor, lag time.Duration, nodes ...string) {
	logp.Info("Subscribe to nodes: %v", nodes)

	ch := make(chan *monitor.DataChangeMessage, 16)
	subscription = make(chan *ResponseObject, 500)

	//TODO: Save sub to unsubscribe on closing the beat
	sub, err := m.ChanSubscribe(ctx, ch, nodes...)

	if err != nil {
		log.Fatal(err)
	}
	defer cleanup(sub)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg.Error != nil {
				logp.Err("[channel ] sub=%d error=%s", sub.SubscriptionID(), msg.Error)
			} else {
				var response ResponseObject
				response.node.ID = msg.NodeID.String()
				response.node.Namespace = msg.NodeID.Namespace()
				response.node.Label = msg.NodeID.String()
				response.value = msg.DataValue
				subscription <- &response

			}
			time.Sleep(lag)
		}
	}
	logp.Info("Subscribe to nodes done")
}

func cleanup(sub *monitor.Subscription) {
	log.Printf("stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe()
}

func closeConnection() {
	client.Close()
	logp.Debug("Collect", "Successfully shutdown connection")
	connected = false
}
