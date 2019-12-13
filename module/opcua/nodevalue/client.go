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
	endpoint     string
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

	ctx := context.Background()
	subscription = make(chan *ResponseObject, 5000)

	m, err := monitor.NewNodeMonitor(client)
	if err != nil {
		log.Fatal(err)
	}

	m.SetErrorHandler(func(_ *opcua.Client, sub *monitor.Subscription, err error) {
		logp.Warn("[OPCUA] Error on monitoring channel: sub=%d err=%s", sub.SubscriptionID(), err.Error())
	})

	// start channel-based subscription
	var nodes []string
	for _, nodeConfig := range nodeCollection {
		if subscribedTo[nodeConfig.ID.(string)] {
			continue
		}
		nodes = append(nodes, "ns="+strconv.Itoa(int(nodeConfig.Namespace))+";s="+nodeConfig.ID.(string))
		subscribedTo[nodeConfig.ID.(string)] = true

	}
	if len(nodes) > 0 {
		go startChanSub(ctx, m, 0, nodes...)
	}
}

func startChanSub(ctx context.Context, m *monitor.NodeMonitor, lag time.Duration, nodes ...string) {
	logp.Info("[OPCUA] Subscribe to nodes: %v", nodes)

	ch := make(chan *monitor.DataChangeMessage, 16)

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
				logp.Warn("[OPCUA] channel-sub=%d error=%s", sub.SubscriptionID(), msg.Error)
			} else {
				var response ResponseObject

				response.node.ID = msg.NodeID.String()
				response.node.Namespace = msg.NodeID.Namespace()
				response.value = msg.DataValue
				subscription <- &response
			}
			time.Sleep(lag)
		}
	}
	logp.Info("[OPCUA] Subscribe to nodes done")
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
