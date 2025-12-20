package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.esd.cc/imlonghao/mtr.sb/pkgs/bgptools"
	"git.esd.cc/imlonghao/mtr.sb/proto"
	ratelimit "github.com/JGLTechnologies/gin-rate-limit"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/go-viper/encoding/hcl"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/ip2location/ip2location-go/v9"
	"github.com/ipinfo/go/v2/ipinfo"
	"github.com/ipinfo/go/v2/ipinfo/cache"
	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"github.com/likexian/whois"
	"github.com/meyskens/go-turnstile"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	protobuf "google.golang.org/protobuf/proto"
)

type Result struct {
	Node string
	Data interface{}
}

type Server struct {
	Name      string
	Provider  string
	Country   string
	Location  string
	AffLink   string
	Latitude  float64
	Longitude float64
	Url       string                  `json:"-"`
	Conn      proto.MtrSbWorkerClient `json:"-"`
	// WebSocket related
	IsWebSocket bool                                `json:"-"`
	WsConn      *websocket.Conn                     `json:"-"`
	WsLock      sync.Mutex                          `json:"-"`
	TaskChans   map[string]chan *proto.TaskResponse `json:"-"`
	TaskLock    sync.RWMutex                        `json:"-"`
}

type IPGeo struct {
	Country      string
	CountryShort string
	Region       string
	City         string
	Asn          int
	AsnName      string
	Rdns         string
}

var (
	serverList    map[string]*Server
	serverListMux sync.RWMutex
	json          = jsoniter.ConfigCompatibleWithStandardLibrary
	ipDB          *ip2location.DB
	Version       = "N/A"
	z             *zap.Logger
	ipio          *ipinfo.Client
	ipioWhiteList = []string{}
	wsUpgrader    = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	v *viper.Viper
)

func init() {
	serverList = make(map[string]*Server)
	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
}

func serverListHandler(c *gin.Context) {
	z.Info("server", zap.String("ip", c.ClientIP()))

	serverListMux.RLock()
	defer serverListMux.RUnlock()
	b, _ := json.Marshal(serverList)
	c.String(http.StatusOK, "%s", b)
}

func agentRegisterHandler(c *gin.Context) {
	clientIP := c.ClientIP()
	z.Info("agent register", zap.String("ip", clientIP))

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		z.Error("websocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Read registration message
	_, message, err := conn.ReadMessage()
	if err != nil {
		z.Error("read registration message failed", zap.Error(err))
		return
	}

	var regReq proto.AgentRegisterRequest
	if err := protobuf.Unmarshal(message, &regReq); err != nil {
		z.Error("unmarshal registration message failed", zap.Error(err))
		return
	}

	agentName := regReq.Name
	agentProvider := regReq.Provider

	z.Info("agent registration",
		zap.String("name", agentName),
		zap.String("provider", agentProvider),
		zap.String("ip", clientIP))

	// Query location from IP
	location := ""
	country := ""
	lat := 0.0
	lon := 0.0

	results, err := ipDB.Get_all(clientIP)
	if err == nil {
		country = results.Country_short
		location = fmt.Sprintf("%s, %s", results.City, results.Region)
		lat = float64(results.Latitude)
		lon = float64(results.Longitude)
	}

	// Send registration response
	regResp := &proto.AgentRegisterResponse{
		Success:   true,
		Message:   "Registration successful",
		Country:   country,
		Location:  location,
		Latitude:  float32(lat),
		Longitude: float32(lon),
	}

	respData, err := protobuf.Marshal(regResp)
	if err != nil {
		z.Error("marshal registration response failed", zap.Error(err))
		return
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, respData); err != nil {
		z.Error("write registration response failed", zap.Error(err))
		return
	}

	// Create server entry
	server := &Server{
		Name:        agentName,
		Provider:    agentProvider,
		Country:     country,
		Location:    location,
		Latitude:    lat,
		Longitude:   lon,
		IsWebSocket: true,
		WsConn:      conn,
		TaskChans:   make(map[string]chan *proto.TaskResponse),
	}

	// Add to server list
	serverListMux.Lock()
	serverList[agentName] = server
	serverListMux.Unlock()

	z.Info("agent registered", zap.String("name", agentName))

	// Start handling messages
	go handleAgentMessages(server, agentName)

	// Keep connection alive with ping/pong
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		select {
		case <-ticker.C:
			server.WsLock.Lock()
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				server.WsLock.Unlock()
				z.Info("agent disconnected", zap.String("name", agentName))
				goto cleanup
			}
			server.WsLock.Unlock()
		}
	}

cleanup:
	serverListMux.Lock()
	delete(serverList, agentName)
	serverListMux.Unlock()
	z.Info("agent removed", zap.String("name", agentName))
}

func handleAgentMessages(server *Server, agentName string) {
	for {
		_, message, err := server.WsConn.ReadMessage()
		if err != nil {
			z.Error("read message from agent failed",
				zap.String("agent", agentName),
				zap.Error(err))
			return
		}

		var taskResp proto.TaskResponse
		if err := protobuf.Unmarshal(message, &taskResp); err != nil {
			z.Error("unmarshal task response failed",
				zap.String("agent", agentName),
				zap.Error(err))
			continue
		}

		// Send response to waiting channel
		server.TaskLock.RLock()
		if ch, ok := server.TaskChans[taskResp.TaskId]; ok {
			select {
			case ch <- &taskResp:
			default:
				z.Warn("task response channel full",
					zap.String("agent", agentName),
					zap.String("task_id", taskResp.TaskId))
			}
		}
		server.TaskLock.RUnlock()
	}
}

func sendTaskToAgent(server *Server, taskReq *proto.TaskRequest) error {
	server.WsLock.Lock()
	defer server.WsLock.Unlock()

	data, err := protobuf.Marshal(taskReq)
	if err != nil {
		return fmt.Errorf("marshal task request failed: %w", err)
	}

	if err := server.WsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write task request failed: %w", err)
	}

	return nil
}

func ipHandler(c *gin.Context) {
	ip, ok := c.GetQuery("t")
	if !ok {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	z.Info("ip", zap.String("ip", c.ClientIP()), zap.String("target", ip))

	inWhiteList := false
	netIP, _ := netip.ParseAddr(c.ClientIP())
	for _, whiteList := range ipioWhiteList {
		if strings.Contains(whiteList, "/") {
			thisNet, _ := netip.ParsePrefix(whiteList)
			if !thisNet.Contains(netIP) {
				continue
			}
		} else {
			if whiteList != c.ClientIP() {
				continue
			}
		}
		inWhiteList = true
		break
	}

	asn := bgptools.Ip2Asn(ip)
	r := ""
	rdns, err := net.LookupAddr(ip)
	if err == nil && len(rdns) > 0 {
		r = rdns[0]
	}

	ipgeo := IPGeo{
		Asn:     asn,
		AsnName: bgptools.Asn2Name(asn),
		Rdns:    r,
	}

	if inWhiteList {
		results, err := ipio.GetIPInfo(net.ParseIP(ip))
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		ipgeo.Country = results.CountryName
		ipgeo.CountryShort = results.Country
		ipgeo.Region = results.Region
		ipgeo.City = results.City
	} else {
		results, err := ipDB.Get_all(ip)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		ipgeo.Country = results.Country_long
		ipgeo.CountryShort = results.Country_short
		ipgeo.Region = results.Region
		ipgeo.City = results.City
	}

	b, _ := json.Marshal(ipgeo)
	c.String(http.StatusOK, "%s", b)
}

func versionHandler(c *gin.Context) {
	z.Info("version", zap.String("ip", c.ClientIP()))

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second*15)
	defer cancel()

	clientChan := make(chan string)
	doneChan := make(chan bool)

	var wg sync.WaitGroup

	a, _ := json.Marshal(Result{
		Node: "Web",
		Data: Version,
	})
	log.Printf("%s", a)
	go func() { clientChan <- string(a) }()

	serverListMux.RLock()
	for hostname, server := range serverList {
		wg.Add(1)
		if server.IsWebSocket {
			go func(hostname string, srv *Server) {
				defer wg.Done()
				taskID := uuid.New().String()

				respChan := make(chan *proto.TaskResponse, 1)
				srv.TaskLock.Lock()
				srv.TaskChans[taskID] = respChan
				srv.TaskLock.Unlock()

				defer func() {
					srv.TaskLock.Lock()
					delete(srv.TaskChans, taskID)
					srv.TaskLock.Unlock()
					close(respChan)
				}()

				taskReq := &proto.TaskRequest{
					TaskId:   taskID,
					TaskType: proto.TaskType_TASK_VERSION,
					Request: &proto.TaskRequest_Version{
						Version: &proto.VersionRequest{},
					},
				}

				if err := sendTaskToAgent(srv, taskReq); err != nil {
					log.Printf("send task to agent %s failed: %v", hostname, err)
					responses, _ := json.Marshal(Result{
						Node: hostname,
						Data: "OFFLINE",
					})
					clientChan <- string(responses)
					return
				}

				select {
				case <-ctx.Done():
					responses, _ := json.Marshal(Result{
						Node: hostname,
						Data: "TIMEOUT",
					})
					clientChan <- string(responses)
				case taskResp := <-respChan:
					if taskResp == nil {
						responses, _ := json.Marshal(Result{
							Node: hostname,
							Data: "OFFLINE",
						})
						clientChan <- string(responses)
					} else if versionResp := taskResp.GetVersion(); versionResp != nil {
						responses, _ := json.Marshal(Result{
							Node: hostname,
							Data: versionResp.GetVersion(),
						})
						log.Printf("%s", responses)
						clientChan <- string(responses)
					}
				}
			}(hostname, server)
		} else {
			go func(hostname string, connection proto.MtrSbWorkerClient) {
				defer wg.Done()
				var responses []byte
				r, err := connection.Version(ctx, &proto.VersionRequest{})
				if err != nil {
					log.Printf("could not greet: %v", err)
					responses, _ = json.Marshal(Result{
						Node: hostname,
						Data: "OFFLINE",
					})
				} else {
					responses, _ = json.Marshal(Result{
						Node: hostname,
						Data: r.GetVersion(),
					})
				}
				log.Printf("%s", responses)
				clientChan <- string(responses)
			}(hostname, server.Conn)
		}
	}
	serverListMux.RUnlock()

	go func() {
		wg.Wait()
		doneChan <- true
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg := <-clientChan:
			c.SSEvent("message", msg)
			return true
		case <-doneChan:
			return false
		}
	})
}

func pingHandler(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	target := c.Query("t")
	protocolStr := c.Query("p")
	remoteDNS := c.Query("rd")

	z.Info("ping", zap.String("ip", c.ClientIP()),
		zap.String("target", target), zap.String("protocol", protocolStr), zap.String("remoteDNS", remoteDNS))

	protocol, err := strconv.Atoi(protocolStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if remoteDNS == "0" {
		newTarget := ""
		ips, _ := net.LookupIP(target)
		for _, ip := range ips {
			switch proto.Protocol(protocol) {
			case proto.Protocol_ANY:
				newTarget = ip.String()
				goto END
			case proto.Protocol_IPV4:
				if ipv4 := ip.To4(); ipv4 != nil {
					newTarget = ip.String()
					goto END
				}
			case proto.Protocol_IPV6:
				if ipv4 := ip.To4(); ipv4 == nil {
					newTarget = ip.String()
					goto END
				}
			}
		}
	END:
		if newTarget == "" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("domain can't resolve"))
			return
		}
		target = newTarget
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second*15)
	defer cancel()

	clientChan := make(chan string)
	doneChan := make(chan bool)

	var wg sync.WaitGroup

	serverListMux.RLock()
	for hostname, server := range serverList {
		wg.Add(1)
		if server.IsWebSocket {
			go func(hostname string, srv *Server) {
				defer wg.Done()
				taskID := uuid.New().String()

				// Create response channel
				respChan := make(chan *proto.TaskResponse, 100)
				srv.TaskLock.Lock()
				srv.TaskChans[taskID] = respChan
				srv.TaskLock.Unlock()

				defer func() {
					srv.TaskLock.Lock()
					delete(srv.TaskChans, taskID)
					srv.TaskLock.Unlock()
					close(respChan)
				}()

				// Send task
				taskReq := &proto.TaskRequest{
					TaskId:   taskID,
					TaskType: proto.TaskType_TASK_PING,
					Request: &proto.TaskRequest_Ping{
						Ping: &proto.PingRequest{
							Host:     target,
							Protocol: proto.Protocol(protocol),
						},
					},
				}

				if err := sendTaskToAgent(srv, taskReq); err != nil {
					log.Printf("send task to agent %s failed: %v", hostname, err)
					return
				}

				// Receive responses
				for {
					select {
					case <-ctx.Done():
						return
					case taskResp := <-respChan:
						if taskResp == nil {
							return
						}
						if pingResp := taskResp.GetPing(); pingResp != nil {
							a, _ := json.Marshal(Result{
								Node: hostname,
								Data: pingResp.Response,
							})
							log.Printf("%s", a)
							clientChan <- string(a)
						}
					}
				}
			}(hostname, server)
		} else {
			go func(hostname string, connection proto.MtrSbWorkerClient) {
				defer wg.Done()
				r, err := connection.Ping(ctx, &proto.PingRequest{
					Host:     target,
					Protocol: proto.Protocol(protocol),
				})
				if err != nil {
					log.Printf("could not greet: %v", err)
					return
				}
				for {
					pingResponse, err := r.Recv()
					if err == io.EOF {
						log.Printf("EOF!")
						break
					}
					if err != nil {
						log.Printf("could not greet: %v", err)
						break
					}
					if pingResponse == nil {
						continue
					}
					a, _ := json.Marshal(Result{
						Node: hostname,
						Data: pingResponse.Response,
					})
					log.Printf("%s", a)
					clientChan <- string(a)
				}
			}(hostname, server.Conn)
		}
	}
	serverListMux.RUnlock()

	go func() {
		wg.Wait()
		doneChan <- true
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg := <-clientChan:
			c.SSEvent("message", msg)
			return true
		case <-doneChan:
			return false
		}
	})
}

func tracerouteHandler(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	node := c.Query("n")
	target := c.Query("t")
	protocolStr := c.Query("p")
	remoteDNS := c.Query("rd")

	z.Info("traceroute", zap.String("ip", c.ClientIP()),
		zap.String("node", node), zap.String("target", target),
		zap.String("protocol", protocolStr), zap.String("remoteDNS", remoteDNS))

	var match bool
	for key, _ := range serverList {
		if key == node {
			match = true
			break
		}
	}
	if !match {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("node not found"))
		return
	}

	protocol, err := strconv.Atoi(protocolStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if remoteDNS == "0" {
		newTarget := ""
		ips, _ := net.LookupIP(target)
		for _, ip := range ips {
			switch proto.Protocol(protocol) {
			case proto.Protocol_ANY:
				newTarget = ip.String()
				goto END
			case proto.Protocol_IPV4:
				if ipv4 := ip.To4(); ipv4 != nil {
					newTarget = ip.String()
					goto END
				}
			case proto.Protocol_IPV6:
				if ipv4 := ip.To4(); ipv4 == nil {
					newTarget = ip.String()
					goto END
				}
			}
		}
	END:
		if newTarget == "" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("domain can't resolve"))
			return
		}
		target = newTarget
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second*15)
	defer cancel()

	clientChan := make(chan string)
	doneChan := make(chan bool)

	var wg sync.WaitGroup

	serverListMux.RLock()
	for hostname, server := range serverList {
		if hostname != node {
			continue
		}
		wg.Add(1)
		if server.IsWebSocket {
			go func(hostname string, srv *Server) {
				defer wg.Done()
				taskID := uuid.New().String()

				respChan := make(chan *proto.TaskResponse, 100)
				srv.TaskLock.Lock()
				srv.TaskChans[taskID] = respChan
				srv.TaskLock.Unlock()

				defer func() {
					srv.TaskLock.Lock()
					delete(srv.TaskChans, taskID)
					srv.TaskLock.Unlock()
					close(respChan)
				}()

				taskReq := &proto.TaskRequest{
					TaskId:   taskID,
					TaskType: proto.TaskType_TASK_TRACEROUTE,
					Request: &proto.TaskRequest_Traceroute{
						Traceroute: &proto.TracerouteRequest{
							Host:     target,
							Protocol: proto.Protocol(protocol),
						},
					},
				}

				if err := sendTaskToAgent(srv, taskReq); err != nil {
					log.Printf("send task to agent %s failed: %v", hostname, err)
					return
				}

				for {
					select {
					case <-ctx.Done():
						return
					case taskResp := <-respChan:
						if taskResp == nil {
							return
						}
						if traceResp := taskResp.GetTraceroute(); traceResp != nil {
							a, _ := json.Marshal(traceResp.Response)
							log.Printf("%s", a)
							clientChan <- string(a)
						}
					}
				}
			}(hostname, server)
		} else {
			go func(connection proto.MtrSbWorkerClient) {
				defer wg.Done()
				r, err := connection.Traceroute(ctx, &proto.TracerouteRequest{
					Host:     target,
					Protocol: proto.Protocol(protocol),
				})
				if err != nil {
					log.Printf("could not greet: %v", err)
					return
				}
				for {
					response, err := r.Recv()
					if err == io.EOF {
						log.Printf("EOF!")
						break
					}
					if err != nil {
						log.Printf("could not greet: %v", err)
						break
					}
					if response == nil {
						continue
					}
					a, _ := json.Marshal(response.Response)
					log.Printf("%s", a)
					clientChan <- string(a)
				}
			}(server.Conn)
		}
	}
	serverListMux.RUnlock()

	go func() {
		wg.Wait()
		doneChan <- true
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg := <-clientChan:
			c.SSEvent("message", msg)
			return true
		case <-doneChan:
			return false
		}
	})
}

func mtrHandler(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	node := c.Query("n")
	target := c.Query("t")
	protocolStr := c.Query("p")
	remoteDNS := c.Query("rd")

	z.Info("mtr", zap.String("ip", c.ClientIP()),
		zap.String("node", node), zap.String("target", target),
		zap.String("protocol", protocolStr), zap.String("remoteDNS", remoteDNS))

	var match bool
	for key, _ := range serverList {
		if key == node {
			match = true
			break
		}
	}
	if !match {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("node not found"))
		return
	}

	protocol, err := strconv.Atoi(protocolStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if remoteDNS == "0" {
		newTarget := ""
		ips, _ := net.LookupIP(target)
		for _, ip := range ips {
			switch proto.Protocol(protocol) {
			case proto.Protocol_ANY:
				newTarget = ip.String()
				goto END
			case proto.Protocol_IPV4:
				if ipv4 := ip.To4(); ipv4 != nil {
					newTarget = ip.String()
					goto END
				}
			case proto.Protocol_IPV6:
				if ipv4 := ip.To4(); ipv4 == nil {
					newTarget = ip.String()
					goto END
				}
			}
		}
	END:
		if newTarget == "" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("domain can't resolve"))
			return
		}
		target = newTarget
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second*20)
	defer cancel()

	clientChan := make(chan string)
	doneChan := make(chan bool)

	var wg sync.WaitGroup

	serverListMux.RLock()
	for hostname, server := range serverList {
		if hostname != node {
			continue
		}
		wg.Add(1)
		if server.IsWebSocket {
			go func(hostname string, srv *Server) {
				defer wg.Done()
				taskID := uuid.New().String()

				respChan := make(chan *proto.TaskResponse, 100)
				srv.TaskLock.Lock()
				srv.TaskChans[taskID] = respChan
				srv.TaskLock.Unlock()

				defer func() {
					srv.TaskLock.Lock()
					delete(srv.TaskChans, taskID)
					srv.TaskLock.Unlock()
					close(respChan)
				}()

				taskReq := &proto.TaskRequest{
					TaskId:   taskID,
					TaskType: proto.TaskType_TASK_MTR,
					Request: &proto.TaskRequest_Mtr{
						Mtr: &proto.MtrRequest{
							Host:     target,
							Protocol: proto.Protocol(protocol),
						},
					},
				}

				if err := sendTaskToAgent(srv, taskReq); err != nil {
					log.Printf("send task to agent %s failed: %v", hostname, err)
					return
				}

				for {
					select {
					case <-ctx.Done():
						return
					case taskResp := <-respChan:
						if taskResp == nil {
							return
						}
						if mtrResp := taskResp.GetMtr(); mtrResp != nil {
							a, _ := json.Marshal(mtrResp)
							log.Printf("%s", a)
							clientChan <- string(a)
						}
					}
				}
			}(hostname, server)
		} else {
			go func(connection proto.MtrSbWorkerClient) {
				defer wg.Done()
				r, err := connection.Mtr(ctx, &proto.MtrRequest{
					Host:     target,
					Protocol: proto.Protocol(protocol),
				})
				if err != nil {
					log.Printf("could not greet: %v", err)
					return
				}
				for {
					response, err := r.Recv()
					if err == io.EOF {
						log.Printf("EOF!")
						break
					}
					if err != nil {
						log.Printf("could not greet: %v", err)
						break
					}
					if response == nil {
						continue
					}
					a, _ := json.Marshal(response)
					log.Printf("%s", a)
					clientChan <- string(a)
				}
			}(server.Conn)
		}
	}
	serverListMux.RUnlock()

	go func() {
		wg.Wait()
		doneChan <- true
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg := <-clientChan:
			c.SSEvent("message", msg)
			return true
		case <-doneChan:
			return false
		}
	})
}

func whoisHandler(c *gin.Context) {
	target := c.Query("t")
	server := c.Query("s")
	token := c.Query("token")

	ts := turnstile.New(v.GetString("turnstile_secret"))
	resp, err := ts.Verify(token, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "data": err.Error()})
		return
	}

	verified := false
	if resp.Success {
		verified = true
	}

	z.Info("whois", zap.String("ip", c.ClientIP()), zap.Bool("turnstile", verified),
		zap.String("target", target), zap.String("server", server))

	if !verified {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "data": "turnstile verify code error"})
		return
	}

	whoisServerTrusted := false
	for _, trustedWhoisServer := range v.GetStringSlice("trusted_whois_server") {
		if server == trustedWhoisServer {
			whoisServerTrusted = true
			break
		}
	}
	if !whoisServerTrusted {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "data": "whois server not trusted"})
		return
	}

	result, err := whois.Whois(target, server)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "data": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func getParam(m map[string]interface{}, k string) string {
	if s, ok := m[k]; ok {
		return s.(string)
	} else {
		return ""
	}
}

func getParamFloat(m map[string]interface{}, k string) float64 {
	if s, ok := m[k]; ok {
		if f, ok := s.(float64); ok {
			return f
		} else {
			return 0
		}
	} else {
		return 0
	}
}

func initServerList() {
	serverList = make(map[string]*Server)
	cert, err := tls.LoadX509KeyPair(v.GetString("cert_path"), v.GetString("key_path"))
	if err != nil {
		log.Fatalf("tls.LoadX509KeyPair err: %v", err)
	}
	certPool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatalf("x509.SystemCertPool err: %v", err)
	}
	for _, caPath := range v.GetStringSlice("worker_ca_path") {
		ca, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatalf("ioutil.ReadFile err: %v", err)
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			log.Fatalf("certPool.AppendCertsFromPEM err")
		}
	}
	c := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	})
	nodes := v.Get("nodes").([]map[string]any)
	for _, node := range nodes {
		n := Server{
			Name:      getParam(node, "name"),
			Provider:  getParam(node, "provider"),
			Country:   getParam(node, "country"),
			Location:  getParam(node, "location"),
			AffLink:   getParam(node, "aff"),
			Url:       getParam(node, "url"),
			Latitude:  getParamFloat(node, "lat"),
			Longitude: getParamFloat(node, "lon"),
			Conn:      nil,
		}
		conn, err := grpc.NewClient(n.Url, grpc.WithTransportCredentials(c))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		client := proto.NewMtrSbWorkerClient(conn)
		n.Conn = client
		serverList[n.Name] = &n
	}
}

func catchAllPath(c *gin.Context) {
	p := filepath.Join("build", filepath.Clean("/"+c.Request.URL.Path))
	if _, err := os.Stat(p); err != nil {
		c.Request.URL.Path = "/"
	}
}

func main() {
	codecRegistry := viper.NewCodecRegistry()
	codec := hcl.Codec{}
	codecRegistry.RegisterCodec("hcl", codec)
	v = viper.NewWithOptions(
		viper.WithCodecRegistry(codecRegistry),
	)
	v.SetConfigName("server")
	v.SetConfigType("hcl")
	v.AddConfigPath("/etc/mtr.sb/")
	v.AddConfigPath("./")
	err := v.ReadInConfig()
	if err != nil {
		log.Fatalf("fatal error config file: %s", err)
	}
	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
		initServerList()
		ipioWhiteList = v.GetStringSlice("ipinfo_whitelist")
		fmt.Println("ipioWhiteList", ipioWhiteList)
	})
	v.WatchConfig()

	// logger
	cfg := zap.NewProductionConfig()
	z, err = cfg.Build()
	if err != nil {
		log.Fatalf("fatal error building logger, %v", err)
	}

	// Set up a connection to the server.
	initServerList()

	// init bgp.tools table.txt
	bgptools.InitNetwork(Version)
	bgptools.AsnNameInitNetwork(Version)

	// ip2location
	ipDB, err = ip2location.OpenDB(v.GetString("ip2location_db_path"))
	if err != nil {
		log.Fatalf("fail to load ip2location db: %v", err)
	}

	// ipinfo
	ipio = ipinfo.NewClient(nil, ipinfo.NewCache(cache.NewInMemory()), v.GetString("ipinfo_token"))
	ipioWhiteList = v.GetStringSlice("ipinfo_whitelist")
	fmt.Println("ipioWhiteList", ipioWhiteList)

	store := ratelimit.InMemoryStore(&ratelimit.InMemoryOptions{
		Rate:  time.Second * 10,
		Limit: 1,
	})
	mw := ratelimit.RateLimiter(store, &ratelimit.Options{
		ErrorHandler: func(c *gin.Context, info ratelimit.Info) {
			c.String(429, "Too many requests. Try again in "+time.Until(info.ResetTime).String())
		},
		KeyFunc: func(c *gin.Context) string {
			return c.ClientIP()
		},
	})

	router := gin.Default()
	router.RemoteIPHeaders = []string{"X-Real-IP"}
	api := router.Group("/api")

	api.GET("/ping", mw, pingHandler)
	api.GET("/traceroute", mw, tracerouteHandler)
	api.GET("/mtr", mw, mtrHandler)
	api.GET("/whois", mw, whoisHandler)

	api.GET("/servers", serverListHandler)
	api.GET("/ip", ipHandler)
	api.GET("/version", versionHandler)

	// WebSocket endpoint for agent registration
	api.GET("/agent/register", agentRegisterHandler)

	router.NoRoute(catchAllPath, gin.WrapH(http.FileServer(gin.Dir("build", false))))
	router.Run(":8085")
}
