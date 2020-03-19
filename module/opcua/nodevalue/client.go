package nodevalue

import (
	"github.com/elastic/beats/libbeat/logp"

	"github.com/gopcua/opcua"
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
	endpoint     = ""
	connected    = false
	subscribedTo = make(map[string]bool)
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

func connect(config MetricSet) error {
	var err error

	if !connected {
		logp.Info("[OPCUA] Get all endpoints from %v", config.Endpoint)
		endpoints, err := opcua.GetEndpoints(config.Endpoint)
		if err != nil {
			logp.Error(err)
		}

		for _, endp := range endpoints {
			logp.Debug("Endpoints", "Found Endpoint: %v", endp.EndpointURL)
			logp.Debug("Endpoints", "Security Mode: %v", endp.SecurityMode.String())
			logp.Debug("Endpoints", "Security Policy: %v", endp.SecurityPolicyURI)
		}

		ep := opcua.SelectEndpoint(endpoints, config.Policy, ua.MessageSecurityModeFromString(config.Mode))
		if ep == nil {
			logp.Err("[OPCUA] Failed to find suitable endpoint. Will switch to default.")
			endpoint = config.Endpoint
		} else {
			endpoint = ep.EndpointURL
		}

		logp.Info("[OPCUA] Policy URI: %v with security mode %v", ep.SecurityPolicyURI, ep.SecurityMode)

		opts := []opcua.Option{
			opcua.SecurityPolicy(config.Policy),
			opcua.SecurityModeString(config.Mode),
		}

		if config.ClientCert != "" {
			logp.Info("[OPCUA] Set ApplicationDescription (SAN DNS and SAN URL) to %v", config.CN)
			opts = append(opts, opcua.ApplicationURI(config.CN))
			opts = append(opts, opcua.CertificateFile(config.ClientCert), opcua.PrivateKeyFile(config.ClientKey))
		}

		if config.Username != "" {
			logp.Info("[OPCUA] Set authentication information")
			logp.Info("[OPCUA] User: %v", config.Username)
			opts = append(opts, opcua.AuthUsername(config.Username, config.Password), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeUserName))
		} else {
			logp.Info("[OPCUA] Set to anonymous login")
			opts = append(opts, opcua.AuthAnonymous(), opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous))
		}

		ctx := context.Background()
		client = opcua.NewClient(endpoint, opts...)
		if err := client.Connect(ctx); err != nil {
			return err
		}
		connected = true
		logp.Info("[OPCUA] Connection established")
	}
	return err
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
		logp.Warn("[OPCUA] Configured node id %v has not a valid type. int and string is allowed. %v provided. ID will be ignored", id, v)
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
	logp.Info("[OPCUA] Starting subscribe process")
	var node string

	ctx := context.Background()
	subscription = make(chan *ResponseObject, 5000)

	subInterval, err := time.ParseDuration("10ms")
	if err != nil {
		logp.Error(err)
	}

	// start channel-based subscription
	ch := make(chan *opcua.PublishNotificationData)

	sub, err := client.Subscribe(&opcua.SubscriptionParameters{
		Interval: subInterval,
	}, ch)
	if err != nil {
		logp.Info("Error occured")
		logp.Error(err)
		return
	}

	logp.Info("[OPCUA] Created subscription with id %v", sub.SubscriptionID)

	for _, nodeConfig := range nodeCollection {

		if subscribedTo[nodeConfig.ID.(string)] {
			continue
		}
		node = "ns=" + strconv.Itoa(int(nodeConfig.Namespace)) + ";s=" + nodeConfig.ID.(string)
		logp.Info("[OPCUA] Subscribe to node: %v", node)

		id, err := ua.ParseNodeID(node)
		if err != nil {
			logp.Error(err)
		}

		// arbitrary client handle for the monitoring item
		handle := uint32(42)
		miCreateRequest := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, handle)
		res, err := sub.Monitor(ua.TimestampsToReturnBoth, miCreateRequest)
		if err != nil || res.Results[0].StatusCode != ua.StatusOK {
			logp.Error(err)
		}

		subscribedTo[nodeConfig.ID.(string)] = true
		logp.Info("[OPCUA] Subscribe to node done")

	}
	go sub.Run(ctx) // start Publish loop
	logp.Info("[OPCUA] Start listening")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if msg.Error != nil {
				logp.Warn("[OPCUA] error=%s", msg.Error)
				continue

			}

			switch x := msg.Value.(type) {
			case *ua.DataChangeNotification:
				for _, item := range x.MonitoredItems {
					var response ResponseObject
					//response.node.ID = rspMsg.NodeID.String()
					//response.node.Namespace = rspMsg.NodeID.Namespace()
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

func cleanup(sub *monitor.Subscription) {
	log.Printf("[OPCUA] Subscribe Stats: sub=%d delivered=%d dropped=%d", sub.SubscriptionID(), sub.Delivered(), sub.Dropped())
	sub.Unsubscribe()
}

func closeConnection() {
	client.Close()
	logp.Debug("Collect", "Successfully shutdown connection")
	connected = false
}
