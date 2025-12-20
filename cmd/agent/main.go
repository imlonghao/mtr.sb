package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"git.esd.cc/imlonghao/mtr.sb/pkgs/worker"
	"git.esd.cc/imlonghao/mtr.sb/proto"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	protobuf "google.golang.org/protobuf/proto"
)

var (
	Version = "N/A"
	z       *zap.Logger
	w       *worker.Worker
)

type TaskStreamWriter struct {
	taskID   string
	taskType proto.TaskType
	conn     *websocket.Conn
}

func (tsw *TaskStreamWriter) Send(resp interface{}) error {
	var taskResp proto.TaskResponse
	taskResp.TaskId = tsw.taskID
	taskResp.TaskType = tsw.taskType

	switch tsw.taskType {
	case proto.TaskType_TASK_PING:
		if pingResp, ok := resp.(*proto.PingResponse); ok {
			taskResp.Response = &proto.TaskResponse_Ping{Ping: pingResp}
		}
	case proto.TaskType_TASK_MTR:
		if mtrResp, ok := resp.(*proto.MtrResponse); ok {
			taskResp.Response = &proto.TaskResponse_Mtr{Mtr: mtrResp}
		}
	case proto.TaskType_TASK_TRACEROUTE:
		if traceResp, ok := resp.(*proto.TracerouteResponse); ok {
			taskResp.Response = &proto.TaskResponse_Traceroute{Traceroute: traceResp}
		}
	case proto.TaskType_TASK_VERSION:
		if versionResp, ok := resp.(*proto.VersionResponse); ok {
			taskResp.Response = &proto.TaskResponse_Version{Version: versionResp}
		}
	}

	data, err := protobuf.Marshal(&taskResp)
	if err != nil {
		return fmt.Errorf("marshal task response failed: %w", err)
	}

	if err := tsw.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("write task response failed: %w", err)
	}

	return nil
}

func handleTask(conn *websocket.Conn, taskReq *proto.TaskRequest) {
	z.Info("received task",
		zap.String("task_id", taskReq.TaskId),
		zap.String("task_type", taskReq.TaskType.String()))

	tsw := &TaskStreamWriter{
		taskID:   taskReq.TaskId,
		taskType: taskReq.TaskType,
		conn:     conn,
	}

	switch taskReq.TaskType {
	case proto.TaskType_TASK_PING:
		if pingReq := taskReq.GetPing(); pingReq != nil {
			handlePingTask(tsw, pingReq)
		}
	case proto.TaskType_TASK_MTR:
		if mtrReq := taskReq.GetMtr(); mtrReq != nil {
			handleMtrTask(tsw, mtrReq)
		}
	case proto.TaskType_TASK_TRACEROUTE:
		if traceReq := taskReq.GetTraceroute(); traceReq != nil {
			handleTracerouteTask(tsw, traceReq)
		}
	case proto.TaskType_TASK_VERSION:
		handleVersionTask(tsw)
	}
}

type PingStreamServer struct {
	writer *TaskStreamWriter
}

func (p *PingStreamServer) Send(resp *proto.PingResponse) error {
	return p.writer.Send(resp)
}

func (p *PingStreamServer) Context() context.Context {
	return context.Background()
}

func (p *PingStreamServer) SetHeader(m metadata.MD) error {
	return nil
}

func (p *PingStreamServer) SendHeader(m metadata.MD) error {
	return nil
}

func (p *PingStreamServer) SetTrailer(m metadata.MD) {
}

func (p *PingStreamServer) SendMsg(m interface{}) error {
	return nil
}

func (p *PingStreamServer) RecvMsg(m interface{}) error {
	return nil
}

func handlePingTask(tsw *TaskStreamWriter, req *proto.PingRequest) {
	server := &PingStreamServer{writer: tsw}
	if err := w.Ping(req, server); err != nil {
		z.Error("ping task failed", zap.Error(err))
	}
}

type MtrStreamServer struct {
	writer *TaskStreamWriter
}

func (m *MtrStreamServer) Send(resp *proto.MtrResponse) error {
	return m.writer.Send(resp)
}

func (m *MtrStreamServer) Context() context.Context {
	return context.Background()
}

func (m *MtrStreamServer) SetHeader(md metadata.MD) error {
	return nil
}

func (m *MtrStreamServer) SendHeader(md metadata.MD) error {
	return nil
}

func (m *MtrStreamServer) SetTrailer(md metadata.MD) {
}

func (m *MtrStreamServer) SendMsg(msg interface{}) error {
	return nil
}

func (m *MtrStreamServer) RecvMsg(msg interface{}) error {
	return nil
}

func handleMtrTask(tsw *TaskStreamWriter, req *proto.MtrRequest) {
	server := &MtrStreamServer{writer: tsw}
	if err := w.Mtr(req, server); err != nil {
		z.Error("mtr task failed", zap.Error(err))
	}
}

type TracerouteStreamServer struct {
	writer *TaskStreamWriter
}

func (t *TracerouteStreamServer) Send(resp *proto.TracerouteResponse) error {
	return t.writer.Send(resp)
}

func (t *TracerouteStreamServer) Context() context.Context {
	return context.Background()
}

func (t *TracerouteStreamServer) SetHeader(md metadata.MD) error {
	return nil
}

func (t *TracerouteStreamServer) SendHeader(md metadata.MD) error {
	return nil
}

func (t *TracerouteStreamServer) SetTrailer(md metadata.MD) {
}

func (t *TracerouteStreamServer) SendMsg(msg interface{}) error {
	return nil
}

func (t *TracerouteStreamServer) RecvMsg(msg interface{}) error {
	return nil
}

func handleTracerouteTask(tsw *TaskStreamWriter, req *proto.TracerouteRequest) {
	server := &TracerouteStreamServer{writer: tsw}
	if err := w.Traceroute(req, server); err != nil {
		z.Error("traceroute task failed", zap.Error(err))
	}
}

func handleVersionTask(tsw *TaskStreamWriter) {
	resp := &proto.VersionResponse{
		Version: Version,
	}
	if err := tsw.Send(resp); err != nil {
		z.Error("send version response failed", zap.Error(err))
	}
}

func main() {
	// Load configuration
	viper.SetEnvPrefix("MTRSB")
	viper.BindEnv("url")
	viper.BindEnv("name")
	viper.BindEnv("provider")
	viper.AutomaticEnv()

	// Initialize logger
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{
		"stdout",
	}
	var err error
	z, err = cfg.Build()
	if err != nil {
		log.Fatalf("fatal error building logger: %v", err)
	}
	defer z.Sync()

	// Initialize worker
	w = &worker.Worker{V: Version}

	serverURL := viper.GetString("url")
	agentName := viper.GetString("name")
	agentProvider := viper.GetString("provider")

	if serverURL == "" {
		log.Fatalf("server url is not set")
	}
	if agentName == "" {
		log.Fatalf("agent name is not set")
	}
	if agentProvider == "" {
		log.Fatalf("agent provider is not set")
	}

	z.Info("connecting to server",
		zap.String("url", serverURL),
		zap.String("name", agentName),
		zap.String("provider", agentProvider))

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	z.Info("connected to server")

	// Send registration message
	regReq := &proto.AgentRegisterRequest{
		Name:     agentName,
		Provider: agentProvider,
	}

	regData, err := protobuf.Marshal(regReq)
	if err != nil {
		log.Fatalf("marshal registration request failed: %v", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, regData); err != nil {
		log.Fatalf("write registration request failed: %v", err)
	}

	z.Info("registration sent")

	// Read registration response
	_, regRespData, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("read registration response failed: %v", err)
	}

	var regResp proto.AgentRegisterResponse
	if err := protobuf.Unmarshal(regRespData, &regResp); err != nil {
		log.Fatalf("unmarshal registration response failed: %v", err)
	}

	if !regResp.Success {
		log.Fatalf("registration failed: %s", regResp.Message)
	}

	z.Info("registration successful",
		zap.String("country", regResp.Country),
		zap.String("location", regResp.Location),
		zap.Float64("latitude", float64(regResp.Latitude)),
		zap.Float64("longitude", float64(regResp.Longitude)))

	// Set up ping/pong handlers
	conn.SetPongHandler(func(string) error {
		z.Debug("received pong")
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Handle interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})

	// Start reading messages
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				z.Error("read message failed", zap.Error(err))
				return
			}

			// Check if it's a ping message
			if len(message) == 0 {
				continue
			}

			var taskReq proto.TaskRequest
			if err := protobuf.Unmarshal(message, &taskReq); err != nil {
				z.Error("unmarshal task request failed", zap.Error(err))
				continue
			}

			// Handle task in a goroutine
			go handleTask(conn, &taskReq)
		}
	}()

	z.Info("agent started, waiting for tasks")

	// Wait for interrupt or done
	select {
	case <-interrupt:
		z.Info("interrupt received, shutting down")
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	case <-done:
		z.Info("connection closed")
	}
}
