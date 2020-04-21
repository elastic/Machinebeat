package nodevalue

import (
	"github.com/elastic/beats/libbeat/logp"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/monitor"
	"github.com/gopcua/opcua/ua"

	"context"
	"log"
	"time"
)

var (
	client       *opcua.Client
	subscription chan *ResponseObject
	endpoint     = ""
	connected    = false
	subscribedTo = make(map[string]bool)
	cfg          *MetricSet
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

func printEndpoints(endpoints []*ua.EndpointDescription) {
	for _, endp := range endpoints {
		logp.Info("[OPCUA] Endpoint: %v", endp.EndpointURL)
		logp.Info("[OPCUA] Security Mode: %v", endp.SecurityMode.String())
		logp.Info("[OPCUA] Security Policy: %v", endp.SecurityPolicyURI)
	}
}

func connect(config *MetricSet) (bool, error) {
	var err error
	cfg = config
	if connected {
		return false, nil
	}
	logp.Info("[OPCUA] Get all endpoints from %v", config.Endpoint)
	endpoints, err := opcua.GetEndpoints(config.Endpoint)
	if err != nil {
		logp.Error(err)
		logp.Debug("Connect", err.Error())
	}

	ep := opcua.SelectEndpoint(endpoints, config.Policy, ua.MessageSecurityModeFromString(config.Mode))
	if ep == nil {
		logp.Err("[OPCUA] Failed to find suitable endpoint. Will try to switch to default [No security settings]. The following configurations are available for security0000000000000000:")
		printEndpoints(endpoints)
		endpoint = config.Endpoint
	} else {
		endpoint = ep.EndpointURL
		logp.Info("[OPCUA] Policy URI: %v with security mode %v", ep.SecurityPolicyURI, ep.SecurityMode)
	}

	opts := []opcua.Option{
		opcua.SecurityPolicy(config.Policy),
		opcua.SecurityModeString(config.Mode),
	}

	if config.ClientCert != "" {
		logp.Info("[OPCUA] Set ApplicationDescription (SAN DNS and SAN URL) to %v", config.AppName)
		opts = append(opts, opcua.ApplicationURI(config.AppName))
		opts = append(opts, opcua.CertificateFile(config.ClientCert), opcua.PrivateKeyFile(config.ClientKey))
	}

	if ep != nil {
		if config.Username != "" {
			logp.Info("[OPCUA] Set authentication information")
			logp.Info("[OPCUA] User: %v", config.Username)
			opts = append(opts, opcua.AuthUsername(config.Username, config.Password), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName))
		} else {
			logp.Info("[OPCUA] Set to anonymous login")
			opts = append(opts, opcua.AuthAnonymous(), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous))
		}
	}

	ctx := context.Background()
	client = opcua.NewClient(endpoint, opts...)
	if err := client.Connect(ctx); err != nil {
		return false, err
	}
	connected = true
	logp.Info("[OPCUA] Connection established")
	return true, err
}

func collectData() ([]*ResponseObject, error) {

	var retVal []*ResponseObject
	var nodesToRead []*ua.ReadValueID
	var nodes []Node

	logp.Debug("Collect", "Building the request")
	for _, nodeConfig := range cfg.Nodes {
		logp.Debug("Collect", "Collecting data from Node %v", nodeConfig.ID)
		nodeId, err := ua.ParseNodeID(nodeConfig.ID)
		if err != nil {
			return nil, err
		}
		nodesToRead = append(nodesToRead, &ua.ReadValueID{NodeID: nodeId})

		node := client.Node(nodeId)
		name, err := node.DisplayName()
		if err == nil {
			nodeConfig.Name = name.Text
		} else {
			logp.Debug("Collect", err.Error())
		}

		attrs, err := node.Attributes(ua.AttributeIDDataType)
		if err != nil {
			logp.Error(err)
			logp.Debug("Collect", err.Error())
		} else {
			nodeConfig.DataType = getDataType(attrs[0])
		}

		//Adding more meta information to the nodes
		nodes = append(nodes, nodeConfig)
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

	for index, node := range nodes {
		logp.Debug("Collect", "Add response from %v", node.ID)
		logp.Debug("Collect", "Current result %v", m.Results[index])
		var response ResponseObject
		response.node = node
		response.value = m.Results[index]
		retVal = append(retVal, &response)
	}
	logp.Debug("Collect", "Data collection done")
	return retVal, nil
}

func prepareSubscription() {
	if subscription == nil {
		subscription = make(chan *ResponseObject, 50000)
	}
}

func startSubscription() {
	logp.Info("[OPCUA] Starting subscribe process")
	prepareSubscription()

	for _, nodeConfig := range cfg.Nodes {
		nodeId, err := ua.ParseNodeID(nodeConfig.ID)
		if err != nil {
			logp.Info("Error occured, will skip node: %v", nodeConfig.ID)
			logp.Error(err)
			logp.Debug("Subscribe", err.Error())
			continue
		}
		go subscribeTo(nodeId)
	}

	logp.Info("[OPCUA] Subscribe process done")
}

func subscribeTo(nodeId *ua.NodeID) {

	if subscribedTo[nodeId.String()] {
		return
	}
	logp.Info("[OPCUA] Subscribe to node: %v", nodeId.String())

	//Prepare the response
	var nodeInformation *Node
	var found = false

	for _, nodeCfg := range cfg.Nodes {
		if nodeId.String() == nodeCfg.ID {
			nodeInformation = &nodeCfg
			found = true
			break
		}
	}

	if !found {
		nodeInformation = &Node{
			ID:    nodeId.String(),
			Label: nodeId.String(),
		}
	}

	node := client.Node(nodeId)
	name, err := node.DisplayName()
	nodeInformation.Name = name.Text

	attrs, err := node.Attributes(ua.AttributeIDDataType)
	if err != nil {
		logp.Error(err)
		logp.Debug("Subscribe", err.Error())
	}
	nodeInformation.DataType = getDataType(attrs[0])

	ctx := context.Background()
	subInterval, err := time.ParseDuration("10ms")
	if err != nil {
		logp.Error(err)
		logp.Debug("Subscribe", err.Error())
	}

	// start channel-based subscription
	ch := make(chan *opcua.PublishNotificationData)

	sub, err := client.Subscribe(&opcua.SubscriptionParameters{
		Interval: subInterval,
	}, ch)
	if err != nil {
		logp.Info("Error occured")
		logp.Error(err)
		logp.Debug("Subscribe", err.Error())
		return
	}

	logp.Info("[OPCUA] Created subscription with id %v", sub.SubscriptionID)

	// arbitrary client handle for the monitoring item
	handle := uint32(42)
	miCreateRequest := opcua.NewMonitoredItemCreateRequestWithDefaults(nodeId, ua.AttributeIDValue, handle)
	res, err := sub.Monitor(ua.TimestampsToReturnBoth, miCreateRequest)
	if err != nil || res.Results[0].StatusCode != ua.StatusOK {
		logp.Error(err)
	}

	subscribedTo[nodeId.String()] = true
	logp.Debug("Subscribe", "[OPCUA] Subscribe to node done")

	go sub.Run(ctx) // start Publish loop
	logp.Debug("Subscribe", "[OPCUA] Start listening")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg.Error != nil {
				logp.Debug("Subscribe", "[OPCUA] subscription=%d error=%s", msg.SubscriptionID, msg.Error)
				continue
			}

			switch x := msg.Value.(type) {
			case *ua.DataChangeNotification:
				for _, item := range x.MonitoredItems {
					//Create response object. This will be collected for every subscription and send during fetch phase to elastic
					var response ResponseObject
					response.node = *nodeInformation
					response.value = item.Value
					subscription <- &response
				}

			default:
				logp.Err("what's this publish result? %T", msg.Value)
			}

		}
	}
	logp.Info("[OPCUA] Stopped listening")
}

//startBrowse is starting browsing through all configured nodes
//if no node is configured it will start at the root node
func startBrowse() {

	var nodeObjsToBrowse []*opcua.Node

	prepareSubscription()

	if len(cfg.Nodes) > 0 {
		for _, nodeConfig := range cfg.Nodes {
			logp.Info("[OPCUA] Start browsing node: %v", nodeConfig.ID)
			nodeId, err := ua.ParseNodeID(nodeConfig.ID)
			if err != nil {
				logp.Err("Invalid node id: %s", err)
				continue
			}
			nodeObj := client.Node(nodeId)
			nodeObjsToBrowse = append(nodeObjsToBrowse, nodeObj)
		}
	} else {
		logp.Info("[OPCUA] Start browsing from node: root")

		nodeObj := client.Node(ua.NewTwoByteNodeID(id.ObjectsFolder))
		nodeObjsToBrowse = append(nodeObjsToBrowse, nodeObj)
	}

	//For each configured Node start browsing.
	for _, nodeObj := range nodeObjsToBrowse {
		//This will browse through nodes and subscribe to every node that we found
		_, err := browse(nodeObj, 0)
		if err != nil {
			logp.Error(err)
			logp.Debug("Browse", err.Error())
		}
	}
}

//browse() is a recursive function to iterate through the node tree
// it returns the node ids of every node that produces values to subscribe to
func browse(node *opcua.Node, level int) ([]*ua.NodeID, error) {
	logp.Debug("Browse", "Start browsing")
	if level > cfg.Browse.MaxLevel {
		return nil, nil
	}

	var nodes []*ua.NodeID

	//Collect attributes of the current node
	attrs, err := node.Attributes(ua.AttributeIDDataType, ua.AttributeIDDisplayName)
	if err != nil {
		logp.Error(err)
		logp.Debug("Browse", err.Error())
	}
	logp.Info("Analyse node id %v", node.ID.String())

	//Only add nodes that have data
	if getDataType(attrs[0]) != "" {
		logp.Info("Add new node to subscription list: ID: %v| Type %v| Name %v", node.ID.String(), getDataType(attrs[0]), attrs[1].Value.String())
		go subscribeTo(node.ID)
		nodes = append(nodes, node.ID)
	}

	//Collect children of the node and iterate through them
	children := findChildren(node, 0)

	for i, child := range children {
		n, err := browse(child, level+1)
		if err != nil {
			logp.Error(err)
			logp.Debug("Browse", err.Error())
		}
		//Append everything that comes back
		nodes = append(nodes, n...)
		if i > cfg.Browse.MaxNodePerParent {
			break
		}
	}
	return nodes, nil
}

func findChildren(node *opcua.Node, refs uint32) []*opcua.Node {
	children, err := node.Children(refs, ua.NodeClassAll)
	if err != nil {
		logp.Error(err)
		logp.Debug("Browse", err.Error())
		return nil
	}
	logp.Debug("Browse", "Found %v new nodes for browsing with ref id %v", len(children), refs)
	return children
}

func getDataType(value *ua.DataValue) string {
	switch err := value.Status; err {
	case ua.StatusOK:
		switch v := value.Value.NodeID().IntID(); v {
		case id.DateTime:
			return "time.Time"
		case id.Boolean:
			return "bool"
		case id.SByte:
			return "int8"
		case id.Int16:
			return "int16"
		case id.Int32:
			return "int32"
		case id.Byte:
			return "byte"
		case id.UInt16:
			return "uint16"
		case id.UInt32:
			return "uint32"
		case id.UtcTime:
			return "time.Time"
		case id.String:
			return "string"
		case id.Float:
			return "float32"
		case id.Double:
			return "float64"
		}
	default:
		logp.Debug("Get DataType", "Status not okay")
	}

	return ""
}

func cleanup(sub *monitor.Subscription) {
	log.Printf("[OPCUA] Subscribe Stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe()
}

func closeConnection() {
	logp.Debug("Shutdown", "Will shutdown connection savely")
	connected = false

	//Fetch panic during shutdown. So that the beat can reconnect
	defer func() {
		if r := recover(); r != nil {
			logp.Info("The connection was already closed / terminated")
		}
	}()

	client.CloseSession()
	client.Close()
	logp.Debug("Shutdown", "Shutdown successfully")
}
