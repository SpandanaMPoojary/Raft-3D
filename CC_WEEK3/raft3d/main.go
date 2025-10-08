package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"raft3d/raftstore"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

var store *raftstore.RaftStore

func main() {
	httpPort := flag.String("http-port", "8080", "HTTP server port")
	raftPort := flag.String("raft-port", "12000", "Raft TCP port")
	nodeID := flag.String("node-id", "node1", "Node ID")
	joinAddr := flag.String("join", "", "Leader HTTP address to join")
	flag.Parse()

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(*nodeID)
	config.SnapshotThreshold = 1024
	config.SnapshotInterval = 30 * time.Second
	config.LeaderLeaseTimeout = 500 * time.Millisecond
	config.ElectionTimeout = 1000 * time.Millisecond

	bindAddr := fmt.Sprintf("127.0.0.1:%s", *raftPort)
	advertiseAddr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		log.Fatalf("Failed to resolve address: %v", err)
	}

	transport, err := raft.NewTCPTransport(
		bindAddr,
		advertiseAddr,
		3,
		10*time.Second,
		os.Stderr,
	)
	if err != nil {
		log.Fatal("Failed to create transport:", err)
	}

	dataDir := filepath.Join("data", *nodeID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatal("Failed to create data directory:", err)
	}

	snapshots, err := raft.NewFileSnapshotStore(dataDir, 3, os.Stderr)
	if err != nil {
		log.Fatal("Failed to create snapshot store:", err)
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	if err != nil {
		log.Fatal("Failed to create log store:", err)
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "stable.db"))
	if err != nil {
		log.Fatal("Failed to create stable store:", err)
	}

	fsm := &raftstore.FSM{
		Printers:  make(map[string]raftstore.Printer),
		Filaments: make(map[string]raftstore.Filament),
		PrintJobs: make(map[string]raftstore.PrintJob),
	}

	raftNode, err := raft.NewRaft(
		config,
		fsm,
		logStore,
		stableStore,
		snapshots,
		transport,
	)
	if err != nil {
		log.Fatal("Failed to create Raft instance:", err)
	}

	if *joinAddr != "" {
		err = joinCluster(*joinAddr, *nodeID, *raftPort)
		if err != nil {
			log.Fatal("Failed to join cluster:", err)
		}
	} else {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	}

	store = raftstore.NewRaftStore(raftNode, fsm)
	router := setupRouter(raftNode)
	log.Printf("Starting node %s on HTTP port %s", *nodeID, *httpPort)
	log.Fatal(router.Run(":" + *httpPort))
}

func joinCluster(leaderAddr, nodeID, raftPort string) error {
	joinRequest := struct {
		NodeID      string `json:"node_id"`
		RaftAddress string `json:"raft_address"`
	}{
		NodeID:      nodeID,
		RaftAddress: fmt.Sprintf("127.0.0.1:%s", raftPort),
	}

	body, err := json.Marshal(joinRequest)
	if err != nil {
		return fmt.Errorf("failed to encode join request: %w", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("http://%s/join", leaderAddr),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to send join request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("join failed: %s", string(body))
	}
	return nil
}

func setupRouter(raftNode *raft.Raft) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Filament Routes
	router.POST("/api/v1/filaments", handleCreateFilament)
	router.GET("/api/v1/filaments", handleGetFilaments)
	router.GET("/api/v1/filaments/:id", handleGetFilament)

	// Printer Routes
	router.POST("/api/v1/printers", handleCreatePrinter)
	router.GET("/api/v1/printers", handleGetPrinters)
	router.DELETE("/api/v1/printers/:id", handleDeletePrinter)

	// Print Job Routes
	router.POST("/api/v1/print_jobs", handleCreatePrintJob)
	router.GET("/api/v1/print_jobs", handleGetPrintJobs)
	router.POST("/api/v1/print_jobs/:id/status", handleUpdateJobStatus)
	// Health Check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"leader": string(raftNode.Leader()),
		})
	})

	// Join cluster
	router.POST("/join", func(c *gin.Context) {
		if raftNode.State() != raft.Leader {
			c.JSON(http.StatusBadRequest, gin.H{"error": "not the leader"})
			return
		}
		var req struct {
			NodeID      string `json:"node_id"`
			RaftAddress string `json:"raft_address"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		future := raftNode.AddVoter(
			raft.ServerID(req.NodeID),
			raft.ServerAddress(req.RaftAddress),
			0,
			30*time.Second,
		)
		if err := future.Error(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "node joined"})
	})

	return router
}

func handleCreatePrinter(c *gin.Context) {
	var printer raftstore.Printer
	if err := c.ShouldBindJSON(&printer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := store.CreatePrinter(printer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, printer)
}

func handleGetPrinters(c *gin.Context) {
	c.JSON(http.StatusOK, store.GetPrinters())
}

func handleDeletePrinter(c *gin.Context) {
	printerID := c.Param("id")

	if _, exists := store.GetPrinter(printerID); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "printer not found"})
		return
	}

	if err := store.DeletePrinter(printerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "printer deleted"})
}

func handleCreateFilament(c *gin.Context) {
	var filament raftstore.Filament
	if err := c.ShouldBindJSON(&filament); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if filament.TotalWeightInGrams <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid total weight"})
		return
	}

	filament.RemainingWeightInGrams = filament.TotalWeightInGrams

	if err := store.CreateFilament(filament); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, filament)
}

func handleGetFilaments(c *gin.Context) {
	c.JSON(http.StatusOK, store.GetFilaments())
}

func handleGetFilament(c *gin.Context) {
	filamentID := c.Param("id")
	filament, exists := store.GetFilament(filamentID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "filament not found"})
		return
	}
	c.JSON(http.StatusOK, filament)
}

func handleCreatePrintJob(c *gin.Context) {
	var job raftstore.PrintJob
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, exists := store.GetPrinter(job.PrinterID); !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid printer ID"})
		return
	}

	filament, exists := store.GetFilament(job.FilamentID)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filament ID"})
		return
	}

	availableWeight := filament.RemainingWeightInGrams - store.GetPendingFilamentUsage(job.FilamentID)
	if job.PrintWeightInGrams > availableWeight {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient filament available"})
		return
	}

	job.Status = "Queued"
	if err := store.CreatePrintJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

func handleGetPrintJobs(c *gin.Context) {
	c.JSON(http.StatusOK, store.GetPrintJobs())
}

func handleUpdateJobStatus(c *gin.Context) {
	jobID := c.Param("id")
	var req struct {
		Status string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	job, exists := store.GetPrintJob(jobID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if !isValidStatusTransition(job.Status, req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status transition"})
		return
	}

	if req.Status == "Done" {
		if err := store.ReduceFilamentWeight(job.FilamentID, job.PrintWeightInGrams); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := store.UpdateJobStatus(jobID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "status updated successfully"})
}

func isValidStatusTransition(oldStatus, newStatus string) bool {
	validTransitions := map[string][]string{
		"Queued":    {"Running", "Cancelled"},
		"Running":   {"Done", "Cancelled"},
		"Done":      {},
		"Cancelled": {},
	}

	allowed, exists := validTransitions[oldStatus]
	if !exists {
		return false
	}

	for _, s := range allowed {
		if s == newStatus {
			return true
		}
	}
	return false
}
