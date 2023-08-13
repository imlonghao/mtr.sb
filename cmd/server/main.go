package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"git.esd.cc/imlonghao/mtr.sb/pkgs/bgptools"
	"git.esd.cc/imlonghao/mtr.sb/proto"
	"github.com/JGLTechnologies/gin-rate-limit"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
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
	json          = jsoniter.ConfigCompatibleWithStandardLibrary
	ipDB          *ip2location.DB
	Version       = "N/A"
	z             *zap.Logger
	ipio          *ipinfo.Client
	ipioWhiteList = []string{}
)

func init() {
	serverList = make(map[string]*Server)
	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
}

func serverListHandler(c *gin.Context) {
	z.Info("server", zap.String("ip", c.ClientIP()))

	b, _ := json.Marshal(serverList)
	c.String(http.StatusOK, "%s", b)
}

func ipHandler(c *gin.Context) {
	ip, ok := c.GetQuery("t")
	if !ok {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	z.Info("ip", zap.String("ip", c.ClientIP()), zap.String("target", ip))

	inWhiteList := false
	netIP, _ := netip.ParseAddr(ip)
	for _, whiteList := range ipioWhiteList {
		if strings.Contains(whiteList, "/") {
			thisNet, _ := netip.ParsePrefix(whiteList)
			if !thisNet.Contains(netIP) {
				continue
			}
		} else {
			if whiteList != ip {
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
		results, err := ipio.GetIPInfo(net.IP(ip))
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

	for hostname, server := range serverList {
		wg.Add(1)
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

	for hostname, server := range serverList {
		wg.Add(1)
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

	for hostname, server := range serverList {
		if hostname != node {
			continue
		}
		wg.Add(1)
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

	for hostname, server := range serverList {
		if hostname != node {
			continue
		}
		wg.Add(1)
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

	ts := turnstile.New(viper.GetString("turnstile_secret"))
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
	for _, trustedWhoisServer := range viper.GetStringSlice("trusted_whois_server") {
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
	cert, err := tls.LoadX509KeyPair(viper.GetString("cert_path"), viper.GetString("key_path"))
	if err != nil {
		log.Fatalf("tls.LoadX509KeyPair err: %v", err)
	}
	certPool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatalf("x509.SystemCertPool err: %v", err)
	}
	for _, caPath := range viper.GetStringSlice("worker_ca_path") {
		ca, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatalf("ioutil.ReadFile err: %v", err)
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			log.Fatalf("certPool.AppendCertsFromPEM err")
		}
	}
	c := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
	})
	nodes := viper.Get("nodes").([]map[string]interface{})
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
		conn, err := grpc.Dial(n.Url, grpc.WithTransportCredentials(c))
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
	viper.SetConfigName("server")
	viper.SetConfigType("hcl")
	viper.AddConfigPath("/etc/mtr.sb/")
	viper.AddConfigPath("./")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("fatal error config file: %w", err)
	}
	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
		initServerList()
		ipioWhiteList = viper.GetStringSlice("ipinfo_whitelist")
		fmt.Println("ipioWhiteList", ipioWhiteList)
	})
	viper.WatchConfig()

	// logger
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{
		"/var/log/mtr.sb/server.log",
	}
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
	ipDB, err = ip2location.OpenDB(viper.GetString("ip2location_db_path"))
	if err != nil {
		log.Fatalf("fail to load ip2location db: %v", err)
	}

	// ipinfo
	ipio = ipinfo.NewClient(nil, ipinfo.NewCache(cache.NewInMemory()), viper.GetString("ipinfo_token"))
	ipioWhiteList = viper.GetStringSlice("ipinfo_whitelist")
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

	router.NoRoute(catchAllPath, gin.WrapH(http.FileServer(gin.Dir("build", false))))
	router.Run(":8085")
}
