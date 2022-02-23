package nodevalue

import (
	"github.com/elastic/beats/v7/libbeat/logp"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"context"
	"time"

	"golang.org/x/sync/semaphore"
)

type Client struct {
	opcua            *opcua.Client
	subscription     chan *ResponseObject
	openSubscription *opcua.Subscription
	endpoint         string
	connected        bool
	nodesToCollect   []*Node
	sem              *semaphore.Weighted
	counter          int
	config           *MetricSet
	ctx              context.Context
}

type ResponseObject struct {
	node  Node
	value *ua.DataValue
}

type Node struct {
	ID       string `config:"id"`
	Label    string `config:"label"`
	NodeId   *ua.NodeID
	Object   *opcua.Node
	Name     string
	Path     string
	DataType string
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

func printEndpoints(endpoints []*ua.EndpointDescription) {
	if len(endpoints) == 0 {
		logp.Info("[OPCUA] This server has no endpoints. This can happen when the OPC UA server can't be reached. Are you sure that the endpoint is right?")
	}
	for _, endp := range endpoints {
		logp.Info("[OPCUA] Endpoint: %v", endp.EndpointURL)
		logp.Info("[OPCUA] Security Mode: %v", endp.SecurityMode.String())
		logp.Info("[OPCUA] Security Policy: %v", endp.SecurityPolicyURI)
	}
}

func (client *Client) connect() (bool, error) {
	var err error
	var config = client.config

	if client.connected {
		return false, nil
	}
	logp.Info("[OPCUA] Get all endpoints from %v", config.Endpoint)
	client.ctx = context.Background()
	endpoints, err := opcua.GetEndpoints(client.ctx, config.Endpoint)
	if err != nil {
		logp.Error(err)
		logp.Debug("Connect", err.Error())
	}

	var policy = ua.FormatSecurityPolicyURI(config.Policy)
	var mode = ua.MessageSecurityModeFromString(config.Mode)
	logp.Info("[OPCUA] Your selected policy: %v and security mode: %v", policy, mode)

	ep := opcua.SelectEndpoint(endpoints, config.Policy, ua.MessageSecurityModeFromString(config.Mode))
	if ep == nil {

		logp.Err("[OPCUA] Failed to find suitable endpoint. Will try to switch to default [No security settings]. The following configurations are available for security:")
		printEndpoints(endpoints)
		client.endpoint = config.Endpoint
	} else {
		client.endpoint = ep.EndpointURL
		logp.Info("[OPCUA] Policy URI: %v with security mode %v", ep.SecurityPolicyURI, ep.SecurityMode)
	}

	opts := []opcua.Option{
		opcua.SecurityPolicy(config.Policy),
		opcua.SecurityModeString(config.Mode),
	}

	logp.Info("[OPCUA] Set ApplicationName to %v", config.AppName)
	opts = append(opts, opcua.ApplicationName(config.AppName))
	logp.Info("[OPCUA] Set ApplicationDescription (SAN DNS and SAN URL) to %v", config.AppName)
	opts = append(opts, opcua.ApplicationURI(config.AppName))

	if config.ClientCert != "" {
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
	client.opcua = opcua.NewClient(client.endpoint, opts...)
	if err := client.opcua.Connect(ctx); err != nil {
		return false, err
	}
	client.connected = true
	logp.Info("[OPCUA] Connection established")
	return true, err
}

func (client *Client) appendNodeInformation() error {

	var opcuaClient = client.opcua

	for _, nodeCfg := range client.nodesToCollect {
		logp.Debug("Append Information", "Collecting data from Node %v", nodeCfg.ID)

		logp.Debug("Append Information", "Collect internal ID")
		nodeId, err := ua.ParseNodeID(nodeCfg.ID)
		if err != nil {
			return err
		}

		logp.Debug("Append Information", "Collect internal Object")
		node := opcuaClient.Node(nodeId)

		if nodeCfg.Name == "" {
			logp.Debug("Append Information", "Collect display name")
			name, err := node.DisplayName()
			if err == nil {
				nodeCfg.Name = name.Text
			} else {
				logp.Debug("Collect", err.Error())
			}
		}
		if nodeCfg.DataType == "" {
			logp.Debug("Append Information", "Collect data type")
			attrs, err := node.Attributes(ua.AttributeIDDataType)
			if err != nil {
				logp.Error(err)
				logp.Debug("Collect", err.Error())
			} else {
				nodeCfg.DataType = getDataType(attrs[0])
			}
		}
	}
	return nil
}

func (client *Client) collectData() ([]*ResponseObject, error) {

	var retVal []*ResponseObject
	var nodesToRead []*ua.ReadValueID

	var opcuaClient = client.opcua

	logp.Debug("Collect", "Building the request")
	for _, nodeCfg := range client.nodesToCollect {
		logp.Debug("Collect", "Add node to request %v", nodeCfg.ID)
		nodesToRead = append(nodesToRead, &ua.ReadValueID{NodeID: nodeCfg.NodeId})
	}

	req := &ua.ReadRequest{
		MaxAge:             2000,
		NodesToRead:        nodesToRead,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
	}

	logp.Debug("Collect", "Sending request")
	m, err := opcuaClient.Read(req)
	if err != nil {
		return retVal, err
	}

	logp.Debug("Collect", "Evaluating response")

	for index, node := range client.nodesToCollect {
		logp.Debug("Collect", "Add response from %v", node.ID)
		logp.Debug("Collect", "Current result %v", m.Results[index])
		var response ResponseObject
		response.node = *node
		response.value = m.Results[index]
		retVal = append(retVal, &response)
	}
	logp.Debug("Collect", "Data collection done")
	return retVal, nil
}

func (client *Client) startSubscription() {
	logp.Info("[OPCUA] Starting subscribe process")
	if client.subscription == nil {
		client.subscription = make(chan *ResponseObject, 50000)
	}

	go client.subscribeTo()

	logp.Info("[OPCUA] Subscribe process done")
}

func (client *Client) subscribeTo() {

	var opcuaClient = client.opcua

	//Create subscription
	ctx := context.Background()
	subInterval, err := time.ParseDuration("10ms")
	if err != nil {
		logp.Error(err)
		logp.Debug("Subscribe", err.Error())
	}

	// start channel-based subscription
	ch := make(chan *opcua.PublishNotificationData)

	sub, err := opcuaClient.Subscribe(&opcua.SubscriptionParameters{
		Interval: subInterval,
	}, ch)
	if err != nil {
		logp.Info("Error occured")
		logp.Error(err)
		logp.Debug("Subscribe", err.Error())
		return
	}

	logp.Info("[OPCUA] Created subscription with id %v", sub.SubscriptionID)

	for i, nodeCfg := range client.nodesToCollect {
		logp.Info("[OPCUA] Add node to subscription: %v", nodeCfg.ID)

		//Parse Node ID
		nodeId, err := ua.ParseNodeID(nodeCfg.ID)
		if err != nil {
			logp.Info("Error occured, will skip node: %v", nodeCfg.ID)
			logp.Error(err)
			logp.Debug("Subscribe", err.Error())
			continue
		}

		// arbitrary client handle for the monitoring item
		handle := uint32(i)
		miCreateRequest := opcua.NewMonitoredItemCreateRequestWithDefaults(nodeId, ua.AttributeIDValue, handle)
		res, err := sub.Monitor(ua.TimestampsToReturnBoth, miCreateRequest)
		if err != nil || res.Results[0].StatusCode != ua.StatusOK {
			logp.Info("Error occured, will skip node: %v", nodeCfg.ID)
			if err != nil {
				logp.Error(err)
				logp.Debug("Subscribe", err.Error())
			}
			continue
		}

		logp.Debug("Subscribe", "[OPCUA] Added node to subscription")
	}

	client.openSubscription = sub

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
					//Create response object. This will be collected for every subscribed node and send during fetch phase to elastic
					var response ResponseObject
					response.node = *client.nodesToCollect[item.ClientHandle]
					response.value = item.Value
					client.subscription <- &response
				}

			default:
				logp.Err("what's this publish result? %T", msg.Value)
			}

		}
	}
	logp.Info("[OPCUA] Stopped listening")
}

//startBrowse is starting browsing through all configured nodes
//if no node is configured it will start at root node(s)
func (client *Client) startBrowse() {

	var nodeObjsToBrowse []*opcua.Node
	var opcuaClient = client.opcua

	if len(client.config.Nodes) > 0 {
		for _, nodeCfg := range client.config.Nodes {
			logp.Info("[OPCUA] Start browsing node: %v", nodeCfg.ID)
			nodeId, err := ua.ParseNodeID(nodeCfg.ID)
			if err != nil {
				logp.Info("Error occured, will skip node: %v", nodeCfg.ID)
				logp.Error(err)
				logp.Debug("Subscribe", err.Error())
				continue
			}
			nodeObj := opcuaClient.Node(nodeId)
			nodeObjsToBrowse = append(nodeObjsToBrowse, nodeObj)
		}
	} else {
		logp.Info("[OPCUA] No custom browse root node configuration found. Start browsing from Objects and Views folder")

		objFolderObj := opcuaClient.Node(ua.NewTwoByteNodeID(id.ObjectsFolder))
		nodeObjsToBrowse = append(nodeObjsToBrowse, objFolderObj)

		viewFolderObj := opcuaClient.Node(ua.NewTwoByteNodeID(id.ViewsFolder))
		nodeObjsToBrowse = append(nodeObjsToBrowse, viewFolderObj)
	}

	//For each configured Node start browsing.
	for _, nodeObj := range nodeObjsToBrowse {
		//This will browse through nodes and subscribe to every node that we found
		err := client.browse(nodeObj, 0, "")
		if err != nil {
			logp.Info("Error occured")
			logp.Error(err)
			logp.Debug("Browse", err.Error())
		}

		logp.Debug("Browse", "Found %v nodes to collect data from so far", len(client.nodesToCollect))
	}
	logp.Info("Found %v nodes in total to collect data from", len(client.nodesToCollect))
}

//browse() is a recursive function to iterate through the node tree
// it returns the node ids of every node that produces values to subscribe to
func (client *Client) browse(node *opcua.Node, level int, path string) error {

	var opcuaClient = client.opcua
	var config = client.config
	var browseName string

	logp.Debug("Browse", "Start browsing at %v", path)
	if config.Browse.MaxLevel > 0 && level > config.Browse.MaxLevel {
		logp.Debug("Browse", "Max level reached. Increase browse.maxLevel to increase this limit")
		return nil
	}

	logp.Info("Analyse node id %v", node.ID.String())

	//Collect attributes of the current node
	attrs, err := node.Attributes(ua.AttributeIDDataType, ua.AttributeIDDisplayName, ua.AttributeIDBrowseName)
	if err != nil {
		logp.Error(err)
		logp.Debug("Browse", err.Error())
	}
	if len(attrs) > 0 {
		switch err := attrs[1].Status; err {
		case ua.StatusOK:
			browseName = attrs[1].Value.String()
		default:
			logp.Debug("Get BrowseName", "Could get BrowseName to build path.")
			browseName = ""
		}

		path = join(path, browseName)

		//Only add nodes that have data
		if getDataType(attrs[0]) != "" {
			nodeObject := &Node{}
			logp.Info("Add new node to list: ID: %v| Type %v| Name %v", node.ID.String(), getDataType(attrs[0]), attrs[1].Value.String())

			nodeObject.Object = opcuaClient.Node(node.ID)
			nodeObject.Path = path
			nodeObject.Name = attrs[1].Value.String()
			nodeObject.DataType = getDataType(attrs[0])
			nodeObject.NodeId = node.ID
			nodeObject.ID = node.ID.String()
			nodeObject.Label = nodeObject.Name

			client.nodesToCollect = append(client.nodesToCollect, nodeObject)
		}
	}
	//Collect children of the node and iterate through them
	children := findChildren(node, 0)

	for i, child := range children {
		err := client.browse(child, level+1, path)
		if err != nil {
			logp.Error(err)
			logp.Debug("Browse", err.Error())
		}

		if config.Browse.MaxNodePerParent > 0 && i > config.Browse.MaxNodePerParent {
			logp.Debug("Browse", "Max node per parent reached. Increase browse.maxNodePerParent to increase this limit")
			break
		}
	}
	return nil
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
		if value.Value != nil {
			logp.Debug("Get DataType", "Using default data type: %v", value.Value.NodeID().String())
			return value.Value.NodeID().String()
		}
		logp.Debug("Get DataType", "Default data type is not applicable")
	}
	logp.Debug("Get DataType", "Was not able to detect DataType")
	return ""
}

func (client *Client) closeConnection() {
	logp.Debug("Shutdown", "Will shutdown connection savely")
	client.connected = false

	//Fetch panic during shutdown. So that the beat can reconnect
	defer func() {
		if r := recover(); r != nil {
			logp.Info("The connection was already closed / terminated")
		}
	}()

	client.openSubscription.Cancel(client.ctx)
	client.openSubscription = nil
	client.opcua.CloseSession()
	client.opcua.Close()
	logp.Debug("Shutdown", "Shutdown successfully")
}
